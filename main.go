// rex-fingers shows letter hints over the paths, URLs, hashes, and other
// strings on a Rex pane, and copies the one you pick, like tmux-fingers.
//
// It runs inside a floating pane that the Lua plugin opens over the target
// pane. Usage:
//
//	rex-fingers show --session SESSION --block BLOCK [flags]
//	rex-fingers patterns
package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/xeind/rex-fingers/internal/patterns"
	"github.com/xeind/rex-fingers/internal/rexcli"
	"github.com/xeind/rex-fingers/internal/screen"
	"github.com/xeind/rex-fingers/internal/ui"
)

const version = "0.1.0"

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
	case "version":
		fmt.Println(version)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rex-fingers show --session ID --block ID [--patterns a,b] [--extra RE] [--copy-command CMD] [--alphabet S] [--align top|bottom]\n       rex-fingers patterns\n       rex-fingers version")
}

func show(args []string) error {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	session := fs.String("session", "", "session ID of the pane to read")
	block := fs.String("block", "", "block ID of the pane to read")
	names := fs.String("patterns", "", "comma-separated built-in pattern names; empty means all")
	var extra multiFlag
	fs.Var(&extra, "extra", "an extra regular expression; repeatable")
	copyCmd := fs.String("copy-command", "", "command that reads the text to copy on stdin; defaults to pbcopy, wl-copy, or xclip")
	alphabet := fs.String("alphabet", "", "letters used for hints")
	align := fs.String("align", "bottom", "edge of the pane to keep when the overlay is shorter: top or bottom")
	hintStyle := fs.String("hint-style", "", "SGR parameters for hints, such as 1;30;43")
	highlightStyle := fs.String("highlight-style", "", "SGR parameters for the rest of a match, such as 1;33;4")
	noDim := fs.Bool("no-dim", false, "keep the screen at full brightness instead of dimming non-matches")
	vtFile := fs.String("vt-file", "", "file holding the pane's VT capture; it is deleted after reading. Without it the pane is read through the rex CLI")
	rows := fs.Int("rows", 0, "the pane's row count, used with --vt-file")
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
		Alphabet: *alphabet,
		Align:    *align,
		Style:    ui.Style{Hint: *hintStyle, Highlight: *highlightStyle},
		NoDim:    *noDim,
		Timeout:  1500 * time.Millisecond,
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
	text := sel.Target.Text
	switch sel.Modifier {
	case ui.Shift:
		return pane.Write(text)
	case ui.Ctrl:
		return openTarget(text)
	default:
		return copyText(text, *copyCmd)
	}
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
