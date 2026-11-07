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
	parts := strings.Split(vt, "\r\n")
	if rows > 0 && len(parts) > rows {
		parts = parts[len(parts)-rows:]
	}
	s := Screen{Lines: make([]Line, 0, len(parts))}
	for _, p := range parts {
		cells := cellsOf(p)
		s.Lines = append(s.Lines, Line{VT: p, Cells: cells, Text: textOf(cells)})
	}
	return s
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
			if r < 0x20 || r == 0x7f {
				continue
			}
			w := runewidth.RuneWidth(r)
			if w == 0 {
				continue
			}
			// go-runewidth gives a regional indicator, half of a flag,
			// width 1. Ghostty, Rex's emulator, draws each one 2 columns
			// wide while grapheme clustering (mode 2027) is off.
			if r >= 0x1F1E6 && r <= 0x1F1FF {
				w = 2
			}
			cells = append(cells, Cell{r, w})
			continue
		}
		if i+1 >= len(rs) {
			break
		}
		switch rs[i+1] {
		case '[':
			j := i + 2
			for j < len(rs) && (rs[j] < 0x40 || rs[j] > 0x7e) {
				j++
			}
			if j < len(rs) && rs[j] == 'C' {
				n := 0
				for _, d := range rs[i+2 : j] {
					if d >= '0' && d <= '9' {
						n = n*10 + int(d-'0')
					}
				}
				if n == 0 {
					n = 1
				}
				for k := 0; k < n; k++ {
					cells = append(cells, Cell{' ', 1})
				}
			}
			i = j
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
			i = j
		case '(', ')', '#':
			i += 2
		default:
			i++
		}
	}
	return cells
}
