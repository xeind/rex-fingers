// Package screen turns a terminal's VT dump into a grid of cells so that
// matches can be located by row and column, while the original escape
// sequences are kept for redrawing the screen with its colors.
package screen

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Cell is one character cell of the screen. A wide character occupies one
// Cell with Width 2; the terminal column after it is skipped.
type Cell struct {
	Rune  rune
	Width int
}

// Line is one terminal row: the raw VT sequence that redraws it, and the
// plain cells it shows.
type Line struct {
	VT    string
	Cells []Cell
	Text  string
	// Wrap is set when the row is the first part of a longer line the
	// terminal wrapped, so the next row continues it. See Join.
	Wrap bool
}

// Screen is the visible part of a terminal, oldest row first.
type Screen struct {
	Lines []Line
}

// Join marks the rows that the terminal wrapped, using the same screen
// captured as text with the format method's unwrap option, where each
// wrapped line is one line. A row joins the next when it is exactly cols
// wide and some unwrapped line contains the two rows run together; the
// second test keeps a line that happens to be cols wide from joining
// whatever follows it. A row that wrapped at a wide character ends one
// cell short and is not joined.
func (s *Screen) Join(cols int, unwrapped string) {
	if cols <= 0 {
		return
	}
	lines := strings.Split(unwrapped, "\n")
	for r := 0; r+1 < len(s.Lines); r++ {
		row := &s.Lines[r]
		if row.Column(len(row.Cells)) != cols || strings.TrimRight(row.Text, " ") != row.Text {
			continue
		}
		pair := row.Text + strings.TrimRight(s.Lines[r+1].Text, " ")
		if pair == row.Text {
			continue
		}
		for _, u := range lines {
			if strings.Contains(u, pair) {
				row.Wrap = true
				break
			}
		}
	}
}

// Parse reads a VT dump as the Rex terminal format method returns it, and
// keeps only the last rows of it. The method writes the scrollback, then
// every screen row, blank ones included, with CR LF between rows and none
// after the last, so the last rows parts are exactly the screen.
func Parse(vt string, rows int) Screen {
	return ParseAt(vt, rows, 0)
}

// ParseAt reads the rows of a VT dump that end offset rows above its last
// row: the page a pane shows when it is scrolled up by offset rows. An
// offset past the top of the dump gives its first rows.
func ParseAt(vt string, rows, offset int) Screen {
	parts := strings.Split(vt, "\r\n")
	if rows > 0 && len(parts) > rows {
		if offset < 0 {
			offset = 0
		}
		if offset > len(parts)-rows {
			offset = len(parts) - rows
		}
		end := len(parts) - offset
		parts = parts[end-rows : end]
	}
	s := Screen{Lines: make([]Line, 0, len(parts))}
	for _, p := range parts {
		cells := cellsOf(p)
		s.Lines = append(s.Lines, Line{VT: p, Cells: cells, Text: textOf(cells)})
	}
	return s
}

// Texts returns the text of every row of a VT dump, scrollback included,
// without keeping the cells or the VT of each row. A search over a long
// scrollback needs only the text: Parse held 181 MB for 100,000 rows.
func Texts(vt string) []string {
	parts := strings.Split(vt, "\r\n")
	texts := make([]string, len(parts))
	for i, p := range parts {
		texts[i] = textOf(cellsOf(p))
	}
	return texts
}

// Column returns the terminal column, counted from 0, where cell i of the
// line starts.
func (l Line) Column(i int) int {
	col := 0
	for j := 0; j < i && j < len(l.Cells); j++ {
		col += l.Cells[j].Width
	}
	return col
}

// CellAt returns the index of the cell that holds byte offset off of
// l.Text, and the number of cells that cover the next length bytes.
func (l Line) CellAt(off, length int) (start, count int) {
	pos := 0
	start = -1
	for i, c := range l.Cells {
		n := len(string(c.Rune))
		if start < 0 && pos >= off {
			start = i
		}
		if start >= 0 {
			if pos >= off+length {
				return start, i - start
			}
		}
		pos += n
	}
	if start < 0 {
		return len(l.Cells), 0
	}
	return start, len(l.Cells) - start
}

// Diff compares the line with want, the text the row is expected to show,
// column by column. It returns the column ranges where they differ, each
// as a start and an end one past the last column, and how many of want's
// non-blank cells the line still shows, out of how many.
func (l Line) Diff(want string) (runs [][2]int, kept, total int) {
	have, expect := columns(l.Cells), columns(cellsOf(want))
	n := len(have)
	if len(expect) > n {
		n = len(expect)
	}
	at := func(cols []rune, i int) rune {
		if i < len(cols) {
			return cols[i]
		}
		return ' '
	}
	start := -1
	for i := 0; i < n; i++ {
		h, w := at(have, i), at(expect, i)
		if w != ' ' && w != 0 {
			total++
			if h == w {
				kept++
			}
		}
		if h != w && start < 0 {
			start = i
		}
		if h == w && start >= 0 {
			runs = append(runs, [2]int{start, i})
			start = -1
		}
	}
	if start >= 0 {
		runs = append(runs, [2]int{start, n})
	}
	return runs, kept, total
}

// columns returns one rune per terminal column: a wide character is its
// rune, then 0 for the column it also covers.
func columns(cells []Cell) []rune {
	var cols []rune
	for _, c := range cells {
		cols = append(cols, c.Rune)
		for i := 1; i < c.Width; i++ {
			cols = append(cols, 0)
		}
	}
	return cols
}

// Slice returns VT that draws columns from up to, not including, to of the
// line as l.VT styles them, for a cursor already at column from. The SGR
// sequences before those columns are replayed first, so the slice does not
// depend on the style in force. A cell the line never wrote, and a column
// past its end, is drawn as a blank in the default style.
func (l Line) Slice(from, to int) string {
	var b, sgr strings.Builder
	started := false
	begin := func() {
		if !started {
			started = true
			b.WriteString("\x1b[0m")
			b.WriteString(sgr.String())
		}
	}
	blank := func() {
		begin()
		b.WriteString("\x1b[0m ")
		b.WriteString(sgr.String())
	}
	rs := []rune(l.VT)
	col := 0
	for i := 0; i < len(rs) && col < to; i++ {
		r := rs[i]
		if r == 0x1b {
			last, blanks := escape(rs, i)
			if i+1 < len(rs) && rs[i+1] == '[' && last < len(rs) && rs[last] == 'm' {
				seq := string(rs[i : last+1])
				sgr.WriteString(seq)
				if started {
					b.WriteString(seq)
				}
			}
			for ; blanks > 0 && col < to; blanks-- {
				if col >= from {
					blank()
				}
				col++
			}
			i = last
			continue
		}
		w := width(r)
		if r == '\t' {
			r = ' '
		}
		if w == 0 {
			// A combining mark belongs to the cell before it.
			if started && r >= 0x20 && r != 0x7f {
				b.WriteRune(r)
			}
			continue
		}
		if col >= from {
			begin()
			b.WriteRune(r)
		}
		col += w
	}
	if col < from {
		col = from
	}
	for ; col < to; col++ {
		blank()
	}
	if started {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

func textOf(cells []Cell) string {
	var b strings.Builder
	for _, c := range cells {
		b.WriteRune(c.Rune)
	}
	return b.String()
}

// cellsOf walks a VT line and keeps the printable characters, expanding a
// cursor-forward sequence (CSI n C) into spaces. Every other escape
// sequence is dropped.
func cellsOf(vt string) []Cell {
	var cells []Cell
	rs := []rune(vt)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r != 0x1b {
			if r == '\t' {
				cells = append(cells, Cell{' ', 1})
				continue
			}
			if w := width(r); w > 0 {
				cells = append(cells, Cell{r, w})
			}
			continue
		}
		last, blanks := escape(rs, i)
		for k := 0; k < blanks; k++ {
			cells = append(cells, Cell{' ', 1})
		}
		i = last
	}
	return cells
}

// width returns the number of columns r takes: 0 for a control character
// or a combining mark, 1 for a tab, which the capture holds as one cell.
func width(r rune) int {
	if r == '\t' {
		return 1
	}
	if r < 0x20 || r == 0x7f {
		return 0
	}
	w := runewidth.RuneWidth(r)
	// go-runewidth gives a regional indicator, half of a flag, width 1.
	// Ghostty, Rex's emulator, draws each one 2 columns wide while
	// grapheme clustering (mode 2027) is off.
	if w > 0 && r >= 0x1F1E6 && r <= 0x1F1FF {
		w = 2
	}
	return w
}

// escape reads the escape sequence that starts at rs[i]. It returns the
// index of the sequence's last rune, which is len(rs) for one cut short,
// and the number of blank cells it stands for: n for cursor-forward
// (CSI n C), 0 for any other.
func escape(rs []rune, i int) (last, blanks int) {
	if i+1 >= len(rs) {
		return len(rs), 0
	}
	switch rs[i+1] {
	case '[':
		j := i + 2
		for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
			j++
		}
		if j < len(rs) && rs[j] == 'C' {
			for _, d := range rs[i+2 : j] {
				if d >= '0' && d <= '9' {
					blanks = blanks*10 + int(d-'0')
				}
			}
			if blanks == 0 {
				blanks = 1
			}
		}
		return j, blanks
	case ']':
		j := i + 2
		for j < len(rs) {
			if rs[j] == 0x07 {
				break
			}
			if rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\' {
				j++
				break
			}
			j++
		}
		return j, 0
	case '(', ')', '#':
		return i + 2, 0
	default:
		return i + 1, 0
	}
}
