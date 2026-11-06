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
  --hint-position left|right
  --hint-style SGR        default 1;38;5;0;48;5;12
  --highlight-style SGR   default 1;38;5;12
  --no-dim                keep non-matches at full brightness
  --align top|bottom      rows to keep when the overlay is shorter

An ACTION is :copy:, :paste:, :open:, or a shell command that reads the
match on stdin with MODIFIER and HINT in its environment. Every pick is
copied to the clipboard first, as in tmux-fingers.`)
}

func show(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	session := fs.String("session", "", "")
	block := fs.String("block", "", "")
	vtFile := fs.String("vt-file", "", "")
	rows := fs.Int("rows", 0, "")
	names := fs.String("patterns", "", "")
	var extra multiFlag
	fs.Var(&extra, "extra", "")
	mainAction := fs.String("main-action", actionCopy, "")
	shiftAction := fs.String("shift-action", actionPaste, "")
	ctrlAction := fs.String("ctrl-action", actionOpen, "")
	altAction := fs.String("alt-action", "", "")
	copyCmd := fs.String("copy-command", "", "")
	layout := fs.String("keyboard-layout", "", "")
	alphabet := fs.String("alphabet", "", "")
	hintPosition := fs.String("hint-position", "left", "")
	hintStyle := fs.String("hint-style", "", "")
	highlightStyle := fs.String("highlight-style", "", "")
	noDim := fs.Bool("no-dim", false, "")
	align := fs.String("align", "bottom", "")
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *session == "" || *block == "" {
		return fmt.Errorf("--session and --block are required")
	}
	if *hintPosition != "left" && *hintPosition != "right" {
		return fmt.Errorf("--hint-position must be left or right")
	}
	letters := *alphabet
	if letters == "" && *layout != "" {
		a, ok := hints.Alphabet(*layout)
		if !ok {
			return fmt.Errorf("unknown keyboard layout %q; see rex-fingers layouts", *layout)
		}
		letters = a
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
	var selected []string
	if *names != "" {
		selected = strings.Split(*names, ",")
	}
	set, err := patterns.Compile(selected, extra)
	if err != nil {
		return err
	}
	scr := screen.Parse(vt, size.Rows)
	overlay := ui.New(scr, set, ui.Options{
		Alphabet:     letters,
		Align:        *align,
		Style:        ui.Style{Hint: *hintStyle, Highlight: *highlightStyle},
		NoDim:        *noDim,
		HintPosition: *hintPosition,
		Timeout:      1500 * time.Millisecond,
	})

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

	action := *mainAction
	switch sel.Modifier {
	case ui.Shift:
		action = *shiftAction
	case ui.Ctrl:
		action = *ctrlAction
	case ui.Alt:
		action = *altAction
	}
	return runAction(action, sel, pane, *copyCmd)
}

// runAction copies the match, then performs the action for the modifier:
// nothing more for :copy:, typing for :paste:, the system opener for
// :open:, or a shell command with the match on its standard input.
func runAction(action string, sel ui.Selection, pane rexcli.Pane, copyCmd string) error {
	text := sel.Target.Text
	copyErr := copyText(text, copyCmd)
	switch action {
	case "", actionCopy:
		return copyErr
	case actionPaste:
		return pane.Write(text)
	case actionOpen:
		return openTarget(text)
	}
	cmd := exec.Command("sh", "-c", action)
	cmd.Stdin = strings.NewReader(text)
	cmd.Dir = pane.Pwd()
	cmd.Env = append(os.Environ(), "MODIFIER="+sel.Modifier.String(), "HINT="+sel.Target.Hint)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
// writes it with OSC 52 for terminals that forward that to the clipboard.
func copyText(text, command string) error {
	fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
	argv := copyCommand(command)
	if argv == nil {
		return fmt.Errorf("no clipboard command found; pass --copy-command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
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
	return exec.Command(opener, text).Start()
}
