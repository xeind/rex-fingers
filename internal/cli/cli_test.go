package cli

import (
	"bytes"
	"flag"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xeind/rex-fingers/internal/hints"
	"github.com/xeind/rex-fingers/internal/patterns"
	"github.com/xeind/rex-fingers/internal/screen"
	"github.com/xeind/rex-fingers/internal/ui"
)

var binary string

// TestMain builds the program once, so the CLI tests run the real binary.
// The tests run from the module root, two directories up: the program is
// built from there, and docs_test.go reads README.md, AGENTS.md, docs/,
// and lua/ by paths from there.
func TestMain(m *testing.M) {
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		panic(err)
	}
	dir, err := os.MkdirTemp("", "rex-fingers-cli-")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "rex-fingers")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		os.Stderr.Write(out)
		os.RemoveAll(dir)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// run executes the binary and returns standard output, standard error, and
// the exit code.
func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

// The version comes from the build, so the test checks its form: a tag or
// pseudo-version in a repository, "(devel)" outside one.
var versionLine = regexp.MustCompile(`^(v\d+\.\d+\.\d+\S*|\(devel\))\n$`)

func TestCLIVersion(t *testing.T) {
	out, _, code := run(t, "version")
	if code != 0 || !versionLine.MatchString(out) {
		t.Errorf("version: code %d, output %q, want a version such as v0.3.0", code, out)
	}
}

func TestCLIPatternsListsEveryBuiltin(t *testing.T) {
	out, _, code := run(t, "patterns")
	if code != 0 {
		t.Fatalf("patterns: exit %d", code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != len(patterns.Builtin) {
		t.Errorf("patterns printed %d lines, want %d", len(lines), len(patterns.Builtin))
	}
	for name, src := range patterns.Builtin {
		want := false
		for _, l := range lines {
			if strings.HasPrefix(l, name+" ") && strings.HasSuffix(l, src) {
				want = true
			}
		}
		if !want {
			t.Errorf("patterns output has no line for %s", name)
		}
	}
}

func TestCLILayoutsListsEveryLayout(t *testing.T) {
	out, _, code := run(t, "layouts")
	if code != 0 {
		t.Fatalf("layouts: exit %d", code)
	}
	for name := range hints.Layouts {
		alphabet, _ := hints.Alphabet(name)
		if !strings.Contains(out, name) || !strings.Contains(out, alphabet) {
			t.Errorf("layouts output lacks %s %q", name, alphabet)
		}
	}
}

func TestCLIUsageErrorsExitTwo(t *testing.T) {
	for _, args := range [][]string{{}, {"nonsense"}, {"--help"}} {
		_, errOut, code := run(t, args...)
		if code != 2 || !strings.Contains(errOut, "usage:") {
			t.Errorf("rex-fingers %v: exit %d, stderr %q; want exit 2 and usage", args, code, errOut)
		}
	}
}

// Each of these fails before the program contacts Rex or a tty.
func TestCLIMissingArgumentsFailCleanly(t *testing.T) {
	for _, args := range [][]string{
		{"show"},
		{"show", "--session", "S"},
		{"show", "--block", "B"},
		{"draw"},
		{"press"},
		{"restore"},
		{"draw", "--bogus-flag"},
		{"show", "--bogus-flag"},
	} {
		_, errOut, code := run(t, args...)
		if code == 0 || !strings.Contains(errOut, "rex-fingers:") && !strings.Contains(errOut, "flag provided but not defined") {
			t.Errorf("rex-fingers %v: exit %d, stderr %q; want a failure with a message", args, code, errOut)
		}
	}
}

// parseOptions builds the options the way show and the in-pane commands do.
func parseOptions(t *testing.T, args ...string) *options {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	o := addOptions(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return o
}

func TestOptionsRejectBadValues(t *testing.T) {
	for _, args := range [][]string{
		{"--hint-position", "middle"},
		{"--keyboard-layout", "no-such-layout"},
		{"--patterns", "no-such-pattern"},
		{"--patterns", "path,no-such-pattern"},
		{"--extra", "(unclosed"},
	} {
		if _, err := parseOptions(t, args...).overlay(screen.Screen{}, "bottom"); err == nil {
			t.Errorf("%v: want an error, got none", args)
		}
	}
}

func TestOptionsAcceptEveryNamedChoice(t *testing.T) {
	var cases [][]string
	for _, pos := range []string{"left", "right", "gutter"} {
		cases = append(cases, []string{"--hint-position", pos})
	}
	for name := range hints.Layouts {
		cases = append(cases, []string{"--keyboard-layout", name})
	}
	for name := range patterns.Builtin {
		cases = append(cases, []string{"--patterns", name})
	}
	cases = append(cases,
		[]string{"--alphabet", "ab"},
		[]string{"--extra", `(?<match>\bTODO\b)`, "--extra", `FIXME`},
		[]string{"--no-dim", "--flash", "150"},
		[]string{"--hint-style", "1;38;5;0;48;5;12", "--highlight-style", "1;38;5;12"},
	)
	for _, args := range cases {
		if _, err := parseOptions(t, args...).overlay(screen.Screen{}, "top"); err != nil {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestOptionsDefaultActions(t *testing.T) {
	o := parseOptions(t)
	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"main", *o.mainAction, actionCopy},
		{"shift", *o.shiftAction, actionPaste},
		{"ctrl", *o.ctrlAction, actionOpen},
		{"alt", *o.altAction, ""},
	} {
		if c.got != c.want {
			t.Errorf("%s action default %q, want %q", c.name, c.got, c.want)
		}
	}
}

// An action gets the session and the pane of the run as REX_SESSION and
// REX_BLOCK, so `rex run` in it opens beside the pane. The server that
// runs the program sets neither.
func TestPressActionGetsRexTarget(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("REX_SESSION", "session:inherited")
	t.Setenv("REX_BLOCK", "block:inherited")
	dir := t.TempDir()
	tty := filepath.Join(dir, "tty")
	vt := filepath.Join(dir, "screen.vt")
	out := filepath.Join(dir, "env.out")
	for _, f := range []struct{ path, content string }{{tty, ""}, {vt, "see /tmp/a"}} {
		if err := os.WriteFile(f.path, []byte(f.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	common := []string{"--tty", tty, "--vt-file", vt, "--rows", "1", "--cols", "20",
		"--patterns", "path", "--alphabet", "asdf", "--copy-command", "true",
		"--main-action", `printf '%s %s' "$REX_SESSION" "$REX_BLOCK" > "` + out + `"`}
	run := func(args ...string) {
		if b, err := exec.Command(binary, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s: %v: %s", args[0], err, b)
		}
	}
	run(append([]string{"draw", "--begin"}, common...)...)
	run(append([]string{"press", "--key", "a", "--session", "session:s1", "--block", "block:b1"}, common...)...)
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("the action did not run: %v", err)
	}
	if string(got) != "session:s1 block:b1" {
		t.Errorf("action saw %q, want %q", got, "session:s1 block:b1")
	}
}

// Rex reports a pane's directory as a file:// URL when the shell sends
// OSC 7, and as a plain path otherwise. An action takes either.
func TestActionFolderFromPwd(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "a folder")
	file := filepath.Join(dir, "fixture.txt")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var opened []string
	saved := runOpener
	runOpener = func(args ...string) error { opened = args; return nil }
	t.Cleanup(func() { runOpener = saved })
	out := filepath.Join(tmp, "pwd.out")
	t.Setenv("RF_TEST_OUT", out)

	for _, pwd := range []string{
		dir,
		(&url.URL{Scheme: "file", Host: "colter.local", Path: dir}).String(),
		(&url.URL{Scheme: "file", Path: dir}).String(),
	} {
		t.Run(pwd, func(t *testing.T) {
			sel := ui.Selection{Target: &ui.Target{Text: "fixture.txt"}}
			opened = nil
			if err := runAction(actionOpen, sel, "true", io.Discard, nil, func() string { return pwd }); err != nil {
				t.Fatalf(":open: %v", err)
			}
			if len(opened) != 1 || opened[0] != file {
				t.Errorf(":open: opener got %q, want %q", opened, file)
			}
			if err := runAction(`pwd -P > "$RF_TEST_OUT"`, sel, "true", io.Discard, nil, func() string { return pwd }); err != nil {
				t.Fatalf("shell action: %v", err)
			}
			got, _ := os.ReadFile(out)
			if strings.TrimSpace(string(got)) != dir {
				t.Errorf("shell action ran in %q, want %q", strings.TrimSpace(string(got)), dir)
			}
		})
	}
}
