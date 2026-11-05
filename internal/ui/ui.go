// Package ui draws the captured screen with hints over it in the overlay
// terminal and turns key presses into a selection.
package ui

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/xeind/rex-fingers/internal/hints"
	"github.com/xeind/rex-fingers/internal/patterns"
	"github.com/xeind/rex-fingers/internal/screen"
)

// Target is one highlighted match.
type Target struct {
	Hint string
	Text string
	Row  int // 0-based row on the captured screen
	Cell int // first cell of the copied part
	Span int // cells the copied part covers
}

// Modifier tells which key family selected the target.
type Modifier int

const (
	Plain Modifier = iota
	Shift
	Ctrl
)

// Selection is what the user picked, or Cancelled.
type Selection struct {
	Target    *Target
	Modifier  Modifier
	Cancelled bool
}

// Style holds SGR parameter strings, such as "1;33;7".
type Style struct {
	Hint      string
	Highlight string
}

// Options configure a run.
type Options struct {
	Alphabet string
	Style    Style
	// Align says which edge of the captured screen to keep when the overlay
	// has fewer rows than the pane: "bottom" or "top".
	Align string
	// NoDim keeps the screen at full brightness instead of dimming
	// everything that is not a match.
	NoDim bool
	// Timeout bounds the wait for the overlay to reach its final size.
	Timeout time.Duration
}

// Timing is called with a label at each step toward the first frame. It
// is nil unless the caller wants measurements.
var Timing = func(string) {}

func (o *Overlay) Timing(label string) { Timing(label) }

// Overlay owns the overlay terminal for one run.
type Overlay struct {
	in      *os.File
	out     io.Writer
	scr     screen.Screen
	targets []Target
	opts    Options
	rows    int
	cols    int
	typed   string
}

// New prepares an overlay for a captured screen and its matches.
func New(scr screen.Screen, set *patterns.Set, opts Options) *Overlay {
	if opts.Alphabet == "" {
		opts.Alphabet = hints.DefaultAlphabet
	}
	// The defaults name palette slots, not fixed colors, so they follow the
	// theme: hints are palette 0 on bright blue (12), the rest of a match
	// is bright blue. The screen around them is dimmed, as tmux-fingers does.
	if opts.Style.Hint == "" {
		opts.Style.Hint = "1;38;5;0;48;5;12"
	}
	if opts.Style.Highlight == "" {
		opts.Style.Highlight = "1;38;5;12"
	}
	if opts.Align == "" {
		opts.Align = "bottom"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 1500 * time.Millisecond
	}
	o := &Overlay{in: os.Stdin, out: os.Stdout, scr: scr, opts: opts}
	o.findTargets(set)
	return o
}

func (o *Overlay) findTargets(set *patterns.Set) {
	var found []Target
	for row, line := range o.scr.Lines {
		for _, m := range set.Find(line.Text) {
			cell, span := line.CellAt(m.CopyStart, m.CopyEnd-m.CopyStart)
			if span == 0 {
				continue
			}
			found = append(found, Target{
				Text: line.Text[m.CopyStart:m.CopyEnd],
				Row:  row, Cell: cell, Span: span,
			})
		}
	}
	// The same text gets the same hint, so fewer keys are needed.
	byText := map[string]int{}
	order := []string{}
	for _, t := range found {
		if _, ok := byText[t.Text]; !ok {
			byText[t.Text] = len(order)
			order = append(order, t.Text)
		}
	}
	hs := hints.Generate(o.opts.Alphabet, len(order))
	for _, t := range found {
		i := byText[t.Text]
		if i >= len(hs) {
			continue
		}
		t.Hint = hs[i]
		if len(t.Hint) > t.Span {
			continue
		}
		o.targets = append(o.targets, t)
	}
}

// Targets returns the hinted matches.
func (o *Overlay) Targets() []Target { return o.targets }

// Run draws the screen, waits for a hint, and returns the selection. It
// restores the terminal before returning.
func (o *Overlay) Run(paneRows int) (Selection, error) {
	fd := int(o.in.Fd())
	if !term.IsTerminal(fd) {
		return Selection{}, fmt.Errorf("stdin is not a terminal")
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return Selection{}, err
	}
	defer term.Restore(fd, state)
	fmt.Fprint(o.out, "\x1b[?1049h\x1b[?25l")
	defer fmt.Fprint(o.out, "\x1b[0m\x1b[?25h\x1b[?1049l")

	// Subscribe before the size wait, so a resize that lands between the
	// wait and the first draw still triggers a redraw.
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)

	o.waitForSize(fd, paneRows)
	o.Timing(fmt.Sprintf("sized %dx%d", o.cols, o.rows))
	o.draw()
	o.Timing("drawn")
	keys := make(chan []byte)
	go func() {
		buf := make([]byte, 16)
		for {
			n, err := o.in.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			keys <- append([]byte(nil), buf[:n]...)
		}
	}()

	for {
		select {
		case <-winch:
			o.cols, o.rows, _ = term.GetSize(fd)
			o.draw()
		case k, ok := <-keys:
			if !ok {
				return Selection{Cancelled: true}, nil
			}
			before := o.typed
			sel, done := o.handle(k)
			if done {
				return sel, nil
			}
			if o.typed != before {
				o.draw()
			}
		}
	}
}

// waitForSize waits until the overlay is as wide as the pane, or until the
// timeout. A new floating pane starts at 80 by 24 and is resized by the
// app a moment later.
func (o *Overlay) waitForSize(fd int, paneRows int) {
	deadline := time.Now().Add(o.opts.Timeout)
	for {
		o.cols, o.rows, _ = term.GetSize(fd)
		if (o.cols != 80 || o.rows != 24) || time.Now().After(deadline) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// offset is the first captured row shown on the overlay.
func (o *Overlay) offset() int {
	extra := len(o.scr.Lines) - o.rows
	if extra <= 0 || o.opts.Align == "top" {
		return 0
	}
	return extra
}

func (o *Overlay) draw() {
	// One write, no full-screen clear: each row is overwritten in place
	// and cleared to its end, so a redraw never shows a blank frame.
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[0m")
	off := o.offset()
	for i := off; i < len(o.scr.Lines) && i-off < o.rows; i++ {
		if i > off {
			b.WriteString("\r\n")
		}
		b.WriteString("\x1b[0m")
		b.WriteString(o.backdrop(o.scr.Lines[i].VT))
		b.WriteString("\x1b[0m\x1b[K")
	}
	b.WriteString("\x1b[0m\x1b[J")
	for _, t := range o.targets {
		if !strings.HasPrefix(t.Hint, o.typed) {
			continue
		}
		row := t.Row - off
		if row < 0 || row >= o.rows {
			continue
		}
		line := o.scr.Lines[t.Row]
		col := line.Column(t.Cell)
		fmt.Fprintf(&b, "\x1b[%d;%dH", row+1, col+1)
		// The hint covers the first cells of the match; the rest of the
		// match is drawn highlighted. Widths are kept so nothing shifts.
		hintCells := 0
		covered := 0
		for hintCells < len(t.Hint) && covered < t.Span {
			hintCells += line.Cells[t.Cell+covered].Width
			covered++
		}
		fmt.Fprintf(&b, "\x1b[0m\x1b[%sm%s", o.opts.Style.Hint, t.Hint)
		for p := len(t.Hint); p < hintCells; p++ {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "\x1b[0m\x1b[%sm", o.opts.Style.Highlight)
		for _, c := range line.Cells[t.Cell+covered : t.Cell+t.Span] {
			b.WriteRune(c.Rune)
		}
		b.WriteString("\x1b[0m")
	}
	io.WriteString(o.out, b.String())
}

// backdrop dims a captured line. Every style reset in the line is replaced
// by a reset that turns dim back on, so the whole line stays faint and
// the hints drawn over it stand out.
func (o *Overlay) backdrop(vt string) string {
	if o.NoDim() {
		return vt
	}
	vt = strings.ReplaceAll(vt, "\x1b[0m", "\x1b[0;2m")
	vt = strings.ReplaceAll(vt, "\x1b[m", "\x1b[0;2m")
	return "\x1b[2m" + vt
}

// NoDim reports whether the backdrop is left at full brightness.
func (o *Overlay) NoDim() bool { return o.opts.NoDim }

// handle applies one key press. It returns the selection and true when the
// run is over.
func (o *Overlay) handle(k []byte) (Selection, bool) {
	if len(k) == 0 {
		return Selection{}, false
	}
	c := k[0]
	switch {
	case c == 0x1b || c == 0x03 || c == 'q':
		return Selection{Cancelled: true}, true
	case c == 0x7f || c == 0x08:
		if o.typed != "" {
			o.typed = o.typed[:len(o.typed)-1]
		}
		return Selection{}, false
	}
	mod := Plain
	var letter byte
	switch {
	case c >= 'a' && c <= 'z':
		letter = c
	case c >= 'A' && c <= 'Z':
		letter, mod = c+('a'-'A'), Shift
	case c >= 1 && c <= 26:
		letter, mod = c+('a'-1), Ctrl
	default:
		return Selection{}, false
	}
	if !strings.ContainsRune(o.opts.Alphabet, rune(letter)) {
		return Selection{}, false
	}
	o.typed += string(letter)
	for i := range o.targets {
		if o.targets[i].Hint == o.typed {
			return Selection{Target: &o.targets[i], Modifier: mod}, true
		}
	}
	any := false
	for _, t := range o.targets {
		if strings.HasPrefix(t.Hint, o.typed) {
			any = true
			break
		}
	}
	if !any {
		o.typed = ""
	}
	return Selection{}, false
}
