// rex-fingers shows letter hints over the paths, URLs, hashes, and other
// strings on a Rex pane, and copies the one you pick, like tmux-fingers.
//
// It runs inside a floating pane that the Lua plugin opens over the target
// pane. Usage:
//
//	rex-fingers show --session SESSION --block BLOCK [flags]
//	rex-fingers patterns
//	rex-fingers layouts
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/xeind/rex-fingers/internal/hints"
	"github.com/xeind/rex-fingers/internal/patterns"
	"github.com/xeind/rex-fingers/internal/rexcli"
	"github.com/xeind/rex-fingers/internal/screen"
	"github.com/xeind/rex-fingers/internal/ui"
)

const version = "0.2.0"

// The special action values, as in tmux-fingers.
const (
	actionCopy  = ":copy:"
	actionPaste = ":paste:"
	actionOpen  = ":open:"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "show":
		if err := show(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "rex-fingers:", err)
			os.Exit(1)
		}
	case "draw", "press", "restore":
		if err := paneCommand(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "rex-fingers:", err)
			os.Exit(1)
		}
	case "patterns":
		for _, n := range patterns.Names() {
			fmt.Printf("%-18s %s\n", n, patterns.Builtin[n])
		}
	case "layouts":
		names := make([]string, 0, len(hints.Layouts))
		for n := range hints.Layouts {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			a, _ := hints.Alphabet(n)
			fmt.Printf("%-20s %s\n", n, a)
		}
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: rex-fingers show --session ID --block ID [flags]
       rex-fingers draw|press|restore --vt-file FILE --rows N --tty PATH|--pid N [flags]
       rex-fingers patterns
       rex-fingers layouts
       rex-fingers version

flags of show:
  --vt-file FILE          the pane's VT capture; read and deleted
  --rows N                the pane's row count, with --vt-file
  --patterns a,b          built-in pattern names; empty means all
  --extra RE              an extra pattern; repeatable
  --main-action ACTION    on a hint; default :copy:
  --shift-action ACTION   on Shift+hint; default :paste:
  --ctrl-action ACTION    on Ctrl+hint; default :open:
  --alt-action ACTION     on Alt+hint; default none
  --copy-command CMD      what :copy: runs; default pbcopy, wl-copy, xclip, or xsel
  --keyboard-layout NAME  qwerty, colemak, dvorak, ...; see layouts
  --alphabet LETTERS      hint letters, overrides the layout
  --hint-position left|right|gutter
  --hint-style SGR        default 1;38;5;0;48;5;12
  --highlight-style SGR   default 1;38;5;12
  --no-dim                keep non-matches at full brightness
  --flash MS              show the picked match in reverse video for MS ms
  --align top|bottom      rows to keep when the overlay is shorter

flags of draw, press, and restore, which draw in the pane's own tty and
also take the flags of show that pick and style the hints:
  --tty PATH              the pane's tty; or --pid N of a process on it
  --cols N                the pane's column count
  --typed LETTERS         hint letters typed so far
  --begin                 (draw) save the cursor before the first draw, and
                          pick up a run left on the pane without restore
  --recover               (restore) repaint only if the pane still shows a
                          run left without restore
  --current-file FILE     (press, restore) the pane's VT captured now; if
                          it differs from what was drawn, the pane wrote
                          output during the run, so only rows that still
                          show the hints are repainted and press picks
                          nothing
  --key a --modifier M    (press) the key; M is main, shift, ctrl, or alt
  --cwd DIR               (press) where an action command runs
  --cursor-visible=B --wraparound=B  the pane's modes before draw --begin

An ACTION is :copy:, :paste:, :open:, or a shell command that reads the
match on stdin with MODIFIER and HINT in its environment. Every pick is
copied to the clipboard first, as in tmux-fingers.`)
}

// options are the flags that show and the in-pane subcommands share: what
// to match, how hints look, and what a hint does.
type options struct {
	names          *string
	extra          multiFlag
	mainAction     *string
	shiftAction    *string
	ctrlAction     *string
	altAction      *string
	copyCmd        *string
	layout         *string
	alphabet       *string
	hintPosition   *string
	hintStyle      *string
	highlightStyle *string
	noDim          *bool
	flash          *int
}

func addOptions(fs *flag.FlagSet) *options {
	o := &options{
		names:          fs.String("patterns", "", ""),
		mainAction:     fs.String("main-action", actionCopy, ""),
		shiftAction:    fs.String("shift-action", actionPaste, ""),
		ctrlAction:     fs.String("ctrl-action", actionOpen, ""),
		altAction:      fs.String("alt-action", "", ""),
		copyCmd:        fs.String("copy-command", "", ""),
		layout:         fs.String("keyboard-layout", "", ""),
		alphabet:       fs.String("alphabet", "", ""),
		hintPosition:   fs.String("hint-position", "left", ""),
		hintStyle:      fs.String("hint-style", "", ""),
		highlightStyle: fs.String("highlight-style", "", ""),
		noDim:          fs.Bool("no-dim", false, ""),
		flash:          fs.Int("flash", 0, ""),
	}
	fs.Var(&o.extra, "extra", "")
	return o
}

// overlay checks the options, compiles the patterns, and finds the hinted
// matches on scr.
func (o *options) overlay(scr screen.Screen, align string) (*ui.Overlay, error) {
	ovs, err := o.overlays([]screen.Screen{scr}, align)
	if err != nil {
		return nil, err
	}
	return ovs[0], nil
}

// overlays does the same for several screens that share one hint list,
// one per pane of a window.
func (o *options) overlays(scrs []screen.Screen, align string) ([]*ui.Overlay, error) {
	if *o.hintPosition != "left" && *o.hintPosition != "right" && *o.hintPosition != "gutter" {
		return nil, fmt.Errorf("--hint-position must be left, right, or gutter")
	}
	letters := *o.alphabet
	if letters == "" && *o.layout != "" {
		a, ok := hints.Alphabet(*o.layout)
		if !ok {
			return nil, fmt.Errorf("unknown keyboard layout %q; see rex-fingers layouts", *o.layout)
		}
		letters = a
	}
	var selected []string
	if *o.names != "" {
		selected = strings.Split(*o.names, ",")
	}
	set, err := patterns.Compile(selected, o.extra)
	if err != nil {
		return nil, err
	}
	return ui.NewSet(scrs, set, ui.Options{
		Alphabet:     letters,
		Align:        align,
		Style:        ui.Style{Hint: *o.hintStyle, Highlight: *o.highlightStyle},
		NoDim:        *o.noDim,
		HintPosition: *o.hintPosition,
		Timeout:      1500 * time.Millisecond,
		Flash:        time.Duration(*o.flash) * time.Millisecond,
	}), nil
}

// action returns the action configured for a modifier.
func (o *options) action(m ui.Modifier) string {
	switch m {
	case ui.Shift:
		return *o.shiftAction
	case ui.Ctrl:
		return *o.ctrlAction
	case ui.Alt:
		return *o.altAction
	}
	return *o.mainAction
}

func show(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	session := fs.String("session", "", "")
	block := fs.String("block", "", "")
	vtFile := fs.String("vt-file", "", "")
	textFile := fs.String("text-file", "", "")
	rows := fs.Int("rows", 0, "")
	cols := fs.Int("cols", 0, "")
	opts := addOptions(fs)
	align := fs.String("align", "bottom", "")
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *session == "" || *block == "" {
		return fmt.Errorf("--session and --block are required")
	}
	pane := rexcli.Pane{Session: *session, Block: *block}
	ui.Timing = timing
	timing("start")

	var vt string
	size := rexcli.Size{Rows: *rows}
	if *vtFile != "" {
		data, err := os.ReadFile(*vtFile)
		os.Remove(*vtFile)
		if err != nil {
			return err
		}
		vt = string(data)
	} else {
		var err error
		if size, err = pane.Size(); err != nil {
			return err
		}
		if vt, err = pane.Capture(); err != nil {
			return err
		}
	}
	scr := screen.Parse(vt, size.Rows)
	// The same screen captured as unwrapped text says which rows the
	// terminal wrapped, so a match can span them.
	if *textFile != "" {
		text, err := os.ReadFile(*textFile)
		os.Remove(*textFile)
		if err == nil {
			width := *cols
			if width == 0 {
				width = size.Columns
			}
			scr.Join(width, string(text))
		}
	}
	overlay, err := opts.overlay(scr, *align)
	if err != nil {
		return err
	}

	timing("parsed")
	sel, err := overlay.Run(size.Rows)
	// The pane gets focus back whatever happened; the floating pane closes
	// when this program exits. Rex keeps a pane open when its program exits
	// within a moment of starting, so a very fast hint press waits it out.
	defer pane.Focus()
	if lived := time.Since(started); lived < 400*time.Millisecond {
		time.Sleep(400*time.Millisecond - lived)
	}
	if err != nil {
		return err
	}
	if sel.Cancelled || sel.Target == nil {
		return nil
	}

	return runAction(opts.action(sel.Modifier), sel, *opts.copyCmd, os.Stdout, pane.Write, pane.Pwd)
}

// runAction copies the match, then performs the action for the modifier:
// nothing more for :copy:, paste for :paste:, the system opener for
// :open:, or a shell command with the match on its standard input. The
// OSC 52 copy goes to osc; a command runs in the directory dir returns.
func runAction(action string, sel ui.Selection, copyCmd string, osc io.Writer, paste func(string) error, dir func() string) error {
	text := sel.Target.Text
	copyErr := copyText(text, copyCmd, osc)
	switch action {
	case "", actionCopy:
		return copyErr
	case actionPaste:
		return paste(text)
	case actionOpen:
		return openTarget(text)
	}
	cmd := exec.Command("sh", "-c", action)
	cmd.Stdin = strings.NewReader(text)
	cmd.Dir = dir()
	cmd.Env = append(commandEnv(os.Environ()), "MODIFIER="+sel.Modifier.String(), "HINT="+sel.Target.Hint)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// commandEnv returns env with a UTF-8 locale added when none is set. The
// Rex server runs the in-pane subcommands with no LANG, and pbcopy then
// reads its input as Mac Roman, so "…" lands on the clipboard as "‚Ä¶".
func commandEnv(env []string) []string {
	vars := map[string]string{}
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok {
			vars[name] = value
		}
	}
	// The first of these that is set decides the character encoding.
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := strings.ToLower(vars[name]); v != "" {
			if strings.Contains(v, "utf-8") || strings.Contains(v, "utf8") {
				return env
			}
			break
		}
	}
	return append(append([]string(nil), env...), "LANG=en_US.UTF-8")
}

var started = time.Now()

// timing appends a timestamp to the file named by REX_FINGERS_TIMING, for
// measuring the time to the first frame.
func timing(label string) {
	path := os.Getenv("REX_FINGERS_TIMING")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s +%dms at %d\n", label, time.Since(started).Milliseconds(), time.Now().UnixMilli())
	f.Close()
}

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// copyText puts text on the clipboard with the copy command, and also
// writes it with OSC 52 to osc for terminals that forward that to the
// clipboard.
func copyText(text, command string, osc io.Writer) error {
	fmt.Fprintf(osc, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
	argv := copyCommand(command)
	if argv == nil {
		return fmt.Errorf("no clipboard command found; pass --copy-command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	cmd.Env = commandEnv(os.Environ())
	return cmd.Run()
}

func copyCommand(command string) []string {
	if command != "" {
		return []string{"sh", "-c", command}
	}
	candidates := [][]string{{"pbcopy"}, {"wl-copy"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}
	for _, c := range candidates {
		if _, err := exec.LookPath(c[0]); err == nil {
			return c
		}
	}
	return nil
}

func openTarget(text string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	cmd := exec.Command(opener, text)
	cmd.Env = commandEnv(os.Environ())
	return cmd.Start()
}
