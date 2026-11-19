package cli

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/xeind/rex-fingers/internal/ui"
)

// TestResolveTarget runs one row per rule of resolveTarget over a temp
// directory. TestMain runs the tests from the module root, so every
// relative name here exists only in the temp directory.
func TestResolveTarget(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", tmp)
	dir := filepath.Join(tmp, "fixture-dir")
	file := filepath.Join(dir, "fixture-file.txt")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "fixture.go"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, text, cwd string
		path            string
		kind            targetKind
	}{
		{"url", "https://example.com/a?b=c", tmp, "https://example.com/a?b=c", targetURL},
		{"git url", "git@github.com:x/y.git", tmp, "git@github.com:x/y.git", targetURL},
		{"file url", "file://" + dir, tmp, dir, targetDir},
		{"home", "~", "/", tmp, targetDir},
		{"under home", "~/fixture-dir/fixture-file.txt", "/", file, targetFile},
		{"absolute folder", dir, "/", dir, targetDir},
		{"relative file", "fixture-dir/fixture-file.txt", tmp, file, targetFile},
		{"bare name that exists", "fixture.go", tmp, filepath.Join(tmp, "fixture.go"), targetFile},
		{"trailing period", "fixture-dir/fixture-file.txt.", tmp, file, targetFile},
		{"trailing comma", "fixture-dir/fixture-file.txt,", tmp, file, targetFile},
		{"line and column", "fixture.go:12:3", tmp, filepath.Join(tmp, "fixture.go"), targetFile},
		{"line and colon", "fixture-dir/fixture-file.txt:322:", tmp, file, targetFile},
		{"diff a/", "a/fixture-dir/fixture-file.txt", tmp, file, targetFile},
		{"diff b/ and a line", "b/fixture-dir/fixture-file.txt:7.", tmp, file, targetFile},
		{"missing file in a folder", "fixture-dir/gone/x.go", tmp, dir, targetMissing},
		{"missing deeper", "gone/deeper/x", tmp, tmp, targetMissing},
		{"missing under home", "~/gone.txt", "/", tmp, targetMissing},
		{"sha", "a3f9c21", tmp, "", targetNone},
		{"semver", "v1.2.3", tmp, "", targetNone},
		{"ip", "192.168.0.1", tmp, "", targetNone},
		{"relative with no pane directory", "fixture-dir/fixture-file.txt", "", "", targetNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, kind := resolveTarget(tc.text, tc.cwd)
			if path != tc.path || kind != tc.kind {
				t.Errorf("resolveTarget(%q, %q) = %q, %v; want %q, %v", tc.text, tc.cwd, path, kind, tc.path, tc.kind)
			}
		})
	}
}

// TestOpenActions runs :open: and :reveal: through runAction with a
// recorder as the opener, so nothing opens.
func TestOpenActions(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "fixture-dir")
	file := filepath.Join(dir, "fixture-file.txt")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var called []string
	saved := runOpener
	runOpener = func(args ...string) error { called = args; return nil }
	t.Cleanup(func() { runOpener = saved })

	revealFile := []string{"-R", file}
	if runtime.GOOS != "darwin" {
		revealFile = []string{dir}
	}
	tests := []struct {
		name, action, text string
		want               []string // nil: the opener is not called
		fails              bool
	}{
		{"open a URL", actionOpen, "https://example.com", []string{"https://example.com"}, false},
		{"reveal a URL", actionReveal, "https://example.com", []string{"https://example.com"}, false},
		{"open a folder", actionOpen, dir, []string{dir}, false},
		{"reveal a folder", actionReveal, dir, []string{dir}, false},
		{"open a file", actionOpen, file, []string{file}, false},
		{"reveal a file", actionReveal, file, revealFile, false},
		{"open a relative file", actionOpen, "fixture-dir/fixture-file.txt:3", []string{file}, false},
		{"open a missing path", actionOpen, "fixture-dir/gone.txt", nil, true},
		{"reveal a missing path", actionReveal, "fixture-dir/gone.txt", []string{dir}, false},
		{"open a sha", actionOpen, "a3f9c21", nil, true},
		{"reveal a sha", actionReveal, "a3f9c21", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called = nil
			sel := ui.Selection{Target: &ui.Target{Text: tc.text}}
			err := runAction(tc.action, sel, "true", io.Discard, nil, func() string { return tmp })
			if (err != nil) != tc.fails {
				t.Errorf("error = %v, want failure %v", err, tc.fails)
			}
			if !slices.Equal(called, tc.want) {
				t.Errorf("opener got %q, want %q", called, tc.want)
			}
		})
	}
}
