package screen

import "testing"

// FuzzParse feeds Parse, Join, Column, and CellAt arbitrary bytes. None may
// panic, and the cells must agree with the text they came from.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"",
		"plain\r\nrows\r\nhere",
		"\x1b[1;31mred\x1b[0m /tmp/a\r\n\x1b[2Kx",
		"\xf0\x9f\x87\xaf\xf0\x9f\x87\xb5 flag\r\n",
		"\x1b[38;5;12m\x1b[K\x1b[",
		"漢字 wide\r\n\x00\x07\x1b",
		"\xff\xfe invalid utf-8",
	} {
		f.Add(s, 3, 10)
	}
	f.Fuzz(func(t *testing.T, vt string, rows, cols int) {
		if rows > 1000 {
			rows = 1000
		}
		s := Parse(vt, rows)
		if rows > 0 && len(s.Lines) > rows {
			t.Fatalf("Parse kept %d lines, rows is %d", len(s.Lines), rows)
		}
		s.Join(cols, vt)
		for _, l := range s.Lines {
			if got := l.Column(len(l.Cells)); got < len(l.Cells) {
				t.Fatalf("Column(%d) = %d, less than the cell count", len(l.Cells), got)
			}
			start, count := l.CellAt(0, len(l.Text))
			if count > len(l.Cells) || start > len(l.Cells) {
				t.Fatalf("CellAt(0, %d) = %d, %d on %d cells", len(l.Text), start, count, len(l.Cells))
			}
			// A row differs from its own text nowhere, and a slice of the
			// whole row draws that text.
			if runs, kept, total := l.Diff(l.Text); len(runs) != 0 || kept != total {
				t.Fatalf("Diff of a row with its own text: runs %v, kept %d of %d", runs, kept, total)
			}
			if width := l.Column(len(l.Cells)); width <= 1000 {
				if got := Parse(l.Slice(0, width), 1).Lines[0].Text; got != l.Text {
					t.Fatalf("Slice(0, %d) draws %q, the row is %q", width, got, l.Text)
				}
			}
		}
	})
}
