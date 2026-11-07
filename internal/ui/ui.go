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
	// Segments is the copied part row by row; the first is Row, Cell,
	// Span. A match on a line the terminal wrapped has one per row.
	Segments []Segment
}

// Segment is the part of a match on one row.
type Segment struct {
	Row, Cell, Span int
}

// Modifier tells which key family selected the target.
type Modifier int

const (
	Plain Modifier = iota
	Shift
	Ctrl
	Alt
)

// String names the modifier as tmux-fingers does in the MODIFIER
// environment variable.
func (m Modifier) String() string {
	switch m {
	case Shift:
		return "shift"
	case Ctrl:
		return "ctrl"
	case Alt:
		return "alt"
	}
	return "main"
}

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
	// HintPosition puts the hint over the start ("left") or the end
	// ("right") of the match, or in the blank cells before it ("gutter",
	// which falls back to "left" when there are not enough).
	HintPosition string
	// Timeout bounds the wait for the overlay to reach its final size.
	Timeout time.Duration
	// Flash is how long a picked match is shown in reverse video before
	// the overlay closes. Zero shows nothing.
	Flash time.Duration
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
	if opts.HintPosition == "" {
		opts.HintPosition = "left"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 1500 * time.Millisecond
	}
	o := &Overlay{in: os.Stdin, out: os.Stdout, scr: scr, opts: opts}
	o.findTargets(set)
	return o
}

func (o *Overlay) findTargets(set *patterns.Set) {
	assignHints([]*Overlay{o}, set)
}

// NewSet prepares one overlay per captured screen, with one hint list
// over all of them, so a hint names one match across every pane of a
// window. Each overlay is otherwise the same as New gives.
func NewSet(scrs []screen.Screen, set *patterns.Set, opts Options) []*Overlay {
	ovs := make([]*Overlay, len(scrs))
	for i, scr := range scrs {
		ovs[i] = New(scr, nil, opts)
	}
	assignHints(ovs, set)
	return ovs
}

// assignHints finds the matches on every overlay's screen and gives them
// hints from one list. The same text gets the same hint, so fewer keys
// are needed.
func assignHints(ovs []*Overlay, set *patterns.Set) {
	if set == nil {
		return
	}
	type found struct {
		o *Overlay
		t Target
	}
	var all []found
	for _, o := range ovs {
		lines := o.scr.Lines
		// Rows the terminal wrapped are matched as one line, so a path
		// or URL that spans rows is one match and copies whole.
		for first := 0; first < len(lines); {
			last := first
			for last+1 < len(lines) && lines[last].Wrap {
				last++
			}
			var joined strings.Builder
			offs := make([]int, 0, last-first+2)
			for r := first; r <= last; r++ {
				offs = append(offs, joined.Len())
				joined.WriteString(lines[r].Text)
			}
			offs = append(offs, joined.Len())
			text := joined.String()
			for _, m := range set.Find(text) {
				var segs []Segment
				for i, r := 0, first; r <= last; i, r = i+1, r+1 {
					lo, hi := max(m.CopyStart, offs[i]), min(m.CopyEnd, offs[i+1])
					if lo >= hi {
						continue
					}
					cell, span := lines[r].CellAt(lo-offs[i], hi-lo)
					if span > 0 {
						segs = append(segs, Segment{Row: r, Cell: cell, Span: span})
					}
				}
				if len(segs) == 0 {
					continue
				}
				all = append(all, found{o, Target{
					Text: text[m.CopyStart:m.CopyEnd],
					Row:  segs[0].Row, Cell: segs[0].Cell, Span: segs[0].Span,
					Segments: segs,
				}})
			}
			first = last + 1
		}
	}
	byText := map[string]int{}
	order := []string{}
	for _, f := range all {
		if _, ok := byText[f.t.Text]; !ok {
			byText[f.t.Text] = len(order)
			order = append(order, f.t.Text)
		}
	}
	hs := hints.Generate(ovs[0].opts.Alphabet, len(order))
	for _, f := range all {
		i := byText[f.t.Text]
		if i >= len(hs) {
			continue
		}
		f.t.Hint = hs[i]
		if len(f.t.Hint) > f.t.Span {
			continue
		}
		f.o.targets = append(f.o.targets, f.t)
	}
}

// Targets returns the hinted matches.
func (o *Overlay) Targets() []Target { return o.targets }

// Paint draws the screen with the hints that start with typed onto w, a
// terminal of rows by cols cells, without reading keys. The in-pane mode
// uses it to draw into the target pane's own tty.
func (o *Overlay) Paint(w io.Writer, rows, cols int, typed string) {
	o.out, o.rows, o.cols, o.typed = w, rows, cols, typed
	o.draw()
}

// Press applies one key, as the bytes a terminal sends for it, after the
// letters already typed. It returns the selection, the letters typed now,
// and whether the run is over.
func (o *Overlay) Press(k []byte, typed string) (Selection, string, bool) {
	o.typed = typed
	sel, done := o.handle(k)
	return sel, o.typed, done
}

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
				// The flash must happen here: the deferred writes above
				// leave the alternate screen as soon as Run returns.
				if sel.Target != nil && o.opts.Flash > 0 {
					o.Flash(o.out, sel.Target)
					time.Sleep(o.opts.Flash)
				}
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
		line := o.scr.Lines[i]
		b.WriteString("\x1b[0m")
		b.WriteString(o.backdrop(line.VT))
		b.WriteString("\x1b[0m")
		// After a row that fills the width the cursor is still on the
		// last column while autowrap is off, so erasing to the end of
		// the line would take that cell. A full row has nothing to erase.
		if line.Column(len(line.Cells)) < o.cols {
			b.WriteString("\x1b[K")
		}
	}
	b.WriteString("\x1b[0m\x1b[J")
	for _, t := range o.targets {
		if !strings.HasPrefix(t.Hint, o.typed) {
			continue
		}
		if t.Row-off < 0 || t.Row-off >= o.rows {
			continue
		}
		// The hint covers the first or last cells of the match; the rest
		// of the match is drawn highlighted, one row at a time when the
		// match spans wrapped rows. Widths are kept so nothing shifts. In
		// gutter mode the hint takes the blank cells before the match
		// instead, when there are enough, and the whole match stays
		// readable.
		p := o.place(t)
		o.hint(&b, p.row-off, o.scr.Lines[p.row].Column(p.cell), t.Hint, p.width)
		for _, s := range t.segments() {
			from, to := p.uncovered(s)
			o.highlight(&b, off, s, from, to, o.opts.Style.Highlight)
		}
	}
	io.WriteString(o.out, b.String())
}

// segments returns the rows of the match; an older Target without them
// is its first row.
func (t Target) segments() []Segment {
	if len(t.Segments) == 0 {
		return []Segment{{t.Row, t.Cell, t.Span}}
	}
	return t.Segments
}

// placement says where the hint of a target is drawn: it replaces covered
// cells from cell on row, and is width columns wide.
type placement struct {
	row, cell, covered, width int
}

// uncovered returns the cells of segment s that the hint does not cover,
// as a range from, to. The hint sits at the start or the end of the
// match, or outside it, never in the middle.
func (p placement) uncovered(s Segment) (from, to int) {
	from, to = 0, s.Span
	if s.Row != p.row {
		return
	}
	end := p.cell + p.covered
	if p.cell <= s.Cell && end > s.Cell {
		from = end - s.Cell
	}
	if p.cell < s.Cell+s.Span && end >= s.Cell+s.Span {
		to = p.cell - s.Cell
	}
	return
}

// place decides where the hint of t goes for the hint position.
func (o *Overlay) place(t Target) placement {
	if start, ok := o.gutter(t); ok {
		return placement{t.Row, start, len(t.Hint), len(t.Hint)}
	}
	segs := t.segments()
	if o.opts.HintPosition == "right" {
		last := segs[len(segs)-1]
		line := o.scr.Lines[last.Row]
		width, covered := 0, 0
		for width < len(t.Hint) && covered < last.Span {
			width += line.Cells[last.Cell+last.Span-1-covered].Width
			covered++
		}
		return placement{last.Row, last.Cell + last.Span - covered, covered, width}
	}
	first := segs[0]
	line := o.scr.Lines[first.Row]
	width, covered := 0, 0
	for width < len(t.Hint) && covered < first.Span {
		width += line.Cells[first.Cell+covered].Width
		covered++
	}
	return placement{first.Row, first.Cell, covered, width}
}

// Painted returns the text of each row as the last Paint or draw left it:
// the screen's text with the hints that start with typed in place, one
// string per row of the terminal. The in-pane mode saves it to tell later
// whether a pane still shows the hints.
func (o *Overlay) Painted() []string {
	off := o.offset()
	rows := make([][]rune, o.rows)
	for i := range rows {
		if off+i < len(o.scr.Lines) {
			rows[i] = []rune(o.scr.Lines[off+i].Text)
		}
	}
	for _, t := range o.targets {
		if !strings.HasPrefix(t.Hint, o.typed) || t.Row-off < 0 || t.Row-off >= o.rows {
			continue
		}
		p := o.place(t)
		// The hint letters, padded to the width of the cells they cover,
		// replace those cells.
		cells := make([]rune, 0, p.width)
		cells = append(cells, []rune(t.Hint)...)
		for len(cells) < p.width {
			cells = append(cells, ' ')
		}
		row := rows[p.row-off]
		row = append(append(append([]rune{}, row[:p.cell]...), cells...), row[p.cell+p.covered:]...)
		rows[p.row-off] = row
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = strings.TrimRight(string(r), " ")
	}
	return out
}

// Flash redraws the match t in reverse video on w, as feedback that it
// was picked. Reverse video is an attribute, not a colour, so it follows
// the theme like the styles do.
func (o *Overlay) Flash(w io.Writer, t *Target) {
	var b strings.Builder
	off := o.offset()
	for _, s := range t.segments() {
		o.highlight(&b, off, s, 0, s.Span, "7")
	}
	io.WriteString(w, b.String())
}

// hint writes the hint letters at a screen row and column, padded to
// width cells so a wide character under it is fully covered.
func (o *Overlay) hint(b *strings.Builder, row, col int, hint string, width int) {
	fmt.Fprintf(b, "\x1b[%d;%dH\x1b[0m\x1b[%sm%s", row+1, col+1, o.opts.Style.Hint, hint)
	for p := len(hint); p < width; p++ {
		b.WriteByte(' ')
	}
	b.WriteString("\x1b[0m")
}

// highlight redraws cells from through to of a segment in an SGR style,
// skipping a segment that is off the visible rows.
func (o *Overlay) highlight(b *strings.Builder, off int, s Segment, from, to int, style string) {
	row := s.Row - off
	if row < 0 || row >= o.rows || from >= to {
		return
	}
	line := o.scr.Lines[s.Row]
	fmt.Fprintf(b, "\x1b[%d;%dH\x1b[0m\x1b[%sm", row+1, line.Column(s.Cell+from)+1, style)
	for _, c := range line.Cells[s.Cell+from : s.Cell+to] {
		b.WriteRune(c.Rune)
	}
	b.WriteString("\x1b[0m")
}

// gutter returns the cell where the hint of t starts when it fits in the
// blank cells just before the match. It needs HintPosition "gutter", one
// blank cell per hint letter, and none of those cells inside another
// match on the row. Otherwise the hint covers the match as in "left".
func (o *Overlay) gutter(t Target) (int, bool) {
	if o.opts.HintPosition != "gutter" {
		return 0, false
	}
	start := t.Cell - len(t.Hint)
	if start < 0 {
		return 0, false
	}
	cells := o.scr.Lines[t.Row].Cells
	for _, c := range cells[start:t.Cell] {
		if c.Rune != ' ' {
			return 0, false
		}
	}
	for _, u := range o.targets {
		if u.Row == t.Row && u.Cell < t.Cell && u.Cell+u.Span > start {
			return 0, false
		}
	}
	return start, true
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
	_, sel, typed, done := PressSet([]*Overlay{o}, k, o.typed)
	o.typed = typed
	return sel, done
}

// PressSet applies one key to a set of overlays that share one hint list,
// as NewSet builds. It returns the index of the overlay that holds the
// picked match, the selection, the letters typed now, and whether the
// run is over. A hint that no overlay continues clears the typed letters.
func PressSet(ovs []*Overlay, k []byte, typed string) (int, Selection, string, bool) {
	if len(k) == 0 {
		return -1, Selection{}, typed, false
	}
	c := k[0]
	mod := Plain
	// Alt sends an escape before the letter; a lone escape leaves.
	if c == 0x1b && len(k) == 2 && k[1] >= 'a' && k[1] <= 'z' {
		c, mod = k[1], Alt
	}
	switch {
	case c == 0x1b || c == 0x03 || c == 'q':
		return -1, Selection{Cancelled: true}, typed, true
	case c == 0x7f || c == 0x08:
		if typed != "" {
			typed = typed[:len(typed)-1]
		}
		return -1, Selection{}, typed, false
	}
	var letter byte
	switch {
	case c >= 'a' && c <= 'z':
		letter = c
	case c >= 'A' && c <= 'Z':
		letter, mod = c+('a'-'A'), Shift
	case c >= 1 && c <= 26:
		letter, mod = c+('a'-1), Ctrl
	default:
		return -1, Selection{}, typed, false
	}
	if !strings.ContainsRune(ovs[0].opts.Alphabet, rune(letter)) {
		return -1, Selection{}, typed, false
	}
	typed += string(letter)
	any := false
	for i, o := range ovs {
		for j := range o.targets {
			if o.targets[j].Hint == typed {
				return i, Selection{Target: &o.targets[j], Modifier: mod}, typed, true
			}
			if strings.HasPrefix(o.targets[j].Hint, typed) {
				any = true
			}
		}
	}
	if !any {
		typed = ""
	}
	return -1, Selection{}, typed, false
}
