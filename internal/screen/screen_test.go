package screen

import (
	"strings"
	"testing"
)

func TestParseExpandsCursorForwardAndDropsStyles(t *testing.T) {
	vt := "\x1b[0m\x1b[1m\x1b[38;5;11m~\x1b[0m/x\x1b[3Cend\x1b[0m\r\n" +
		"\x1b]8;;http://a\x07link\x1b]8;;\x07 日本"
	s := Parse(vt, 0)
	if len(s.Lines) != 2 {
		t.Fatalf("want 2 lines, got %d", len(s.Lines))
	}
	if got := s.Lines[0].Text; got != "~/x   end" {
		t.Fatalf("line 0 text %q", got)
	}
	if got := s.Lines[1].Text; got != "link 日本" {
		t.Fatalf("line 1 text %q", got)
	}
	if col := s.Lines[1].Column(6); col != 7 {
		t.Fatalf("wide char column: want 7, got %d", col)
	}
}

func TestParseKeepsLastRows(t *testing.T) {
	s := Parse("a\r\nb\r\nc", 2)
	if len(s.Lines) != 2 || s.Lines[0].Text != "b" {
		t.Fatalf("want b,c got %+v", s.Lines)
	}
}

// A screen whose bottom row is blank ends in CR LF and an empty part. That
// part is the bottom row, so with scrollback above it the screen must not
// shift up by one.
func TestParseKeepsBlankBottomRow(t *testing.T) {
	s := Parse("old\r\ntop\r\nmid\r\n", 3)
	if len(s.Lines) != 3 || s.Lines[0].Text != "top" || s.Lines[2].Text != "" {
		t.Fatalf("want top,mid,\"\" got %+v", s.Lines)
	}
}

// A flag is two regional indicators. Ghostty draws each one 2 columns wide,
// so the path after "🇵🇭 " starts at column 5, not 3.
func TestRegionalIndicatorIsWide(t *testing.T) {
	l := Parse("🇵🇭 /tmp/x", 0).Lines[0]
	if l.Text != "🇵🇭 /tmp/x" {
		t.Fatalf("text %q", l.Text)
	}
	start, count := l.CellAt(len("🇵🇭 "), len("/tmp/x"))
	if start != 3 || count != 6 {
		t.Fatalf("CellAt: want 3,6 got %d,%d", start, count)
	}
	if col := l.Column(start); col != 5 {
		t.Fatalf("flag column: want 5, got %d", col)
	}
}

// A full-width row joins the next only when the unwrapped text shows the
// two were one line. An exactly-cols line followed by unrelated text, or
// a short row, stays on its own.
func TestJoin(t *testing.T) {
	s := Parse("abcdefgh\r\nij rest\r\n12345678\r\nother\r\nshort\r\nmore", 0)
	s.Join(8, "abcdefghij rest\n12345678\nother\nshort\nmore")
	want := []bool{true, false, false, false, false, false}
	for i, l := range s.Lines {
		if l.Wrap != want[i] {
			t.Errorf("row %d %q: wrap %v, want %v", i, l.Text, l.Wrap, want[i])
		}
	}
}

func TestCellAt(t *testing.T) {
	l := Parse("ab日本cd", 0).Lines[0]
	start, count := l.CellAt(2, 6) // bytes of 日本
	if start != 2 || count != 2 {
		t.Fatalf("want 2,2 got %d,%d", start, count)
	}
}

func TestParseAtReadsAPageAboveTheLastRows(t *testing.T) {
	vt := "one\r\ntwo\r\nthree\r\nfour\r\nfive"
	for _, c := range []struct {
		rows, offset int
		want         []string
	}{
		{2, 0, []string{"four", "five"}},
		{2, 1, []string{"three", "four"}},
		{2, 3, []string{"one", "two"}},
		{2, 9, []string{"one", "two"}},    // past the top: the first rows
		{2, -1, []string{"four", "five"}}, // a negative offset is the last rows
		{9, 2, []string{"one", "two", "three", "four", "five"}},
	} {
		s := ParseAt(vt, c.rows, c.offset)
		var got []string
		for _, l := range s.Lines {
			got = append(got, l.Text)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("ParseAt(rows %d, offset %d) = %v, want %v", c.rows, c.offset, got, c.want)
		}
	}
}

// Texts must give the same text per row as Parse, which the page search
// used before; only the cells are not kept.
func TestTextsMatchesParse(t *testing.T) {
	vt := "\x1b[1m~\x1b[0m/x\x1b[3Cend\r\n\x1b]8;;http://a\x07link\x1b]8;;\x07 日本\r\n\r\nwarn ⚠️\ttab"
	want := Parse(vt, 0)
	got := Texts(vt)
	if len(got) != len(want.Lines) {
		t.Fatalf("Texts gave %d rows, Parse %d", len(got), len(want.Lines))
	}
	for i, l := range want.Lines {
		if got[i] != l.Text {
			t.Errorf("row %d: Texts %q, Parse %q", i, got[i], l.Text)
		}
	}
}

// Diff reports the columns where a row no longer shows the expected text,
// and how much of that text is still there. A wide character takes two
// columns.
func TestDiffFindsTheChangedColumns(t *testing.T) {
	line := Parse("s/dev/x > 12  世x", 1).Lines[0]
	runs, kept, total := line.Diff("s/dev/x >     世y")
	if len(runs) != 2 || runs[0] != [2]int{10, 12} || runs[1] != [2]int{16, 17} {
		t.Errorf("runs %v, want [[10 12] [16 17]]", runs)
	}
	if kept != 9 || total != 10 {
		t.Errorf("kept %d of %d, want 9 of 10", kept, total)
	}
	if runs, kept, total := line.Diff(line.Text); len(runs) != 0 || kept != total {
		t.Errorf("the same text: runs %v, kept %d of %d", runs, kept, total)
	}
	// A row shorter than the expected text differs where it ends.
	if runs, _, _ := Parse("ab", 1).Lines[0].Diff("abcd"); len(runs) != 1 || runs[0] != [2]int{2, 4} {
		t.Errorf("short row: runs %v, want [[2 4]]", runs)
	}
}

// Slice draws a range of columns with the styles the row gives them. The
// styles set before the range are replayed; a cell the row skipped with
// cursor-forward, and a column past its end, is a blank in the default
// style.
func TestSliceKeepsTheStyleOfTheColumns(t *testing.T) {
	line := Parse("\x1b[2mab\x1b[0m\x1b[31mcd\x1b[0m\x1b[2Cz", 1).Lines[0]
	for _, c := range []struct {
		from, to int
		want     string
	}{
		{2, 4, "\x1b[0m\x1b[2m\x1b[0m\x1b[31mcd\x1b[0m"},
		{1, 3, "\x1b[0m\x1b[2mb\x1b[0m\x1b[31mc\x1b[0m"},
		{5, 7, "\x1b[0m\x1b[2m\x1b[0m\x1b[31m\x1b[0m\x1b[0m \x1b[2m\x1b[0m\x1b[31m\x1b[0mz\x1b[0m"},
		{7, 9, "\x1b[0m\x1b[2m\x1b[0m\x1b[31m\x1b[0m\x1b[0m \x1b[2m\x1b[0m\x1b[31m\x1b[0m\x1b[0m \x1b[2m\x1b[0m\x1b[31m\x1b[0m\x1b[0m"},
	} {
		if got := line.Slice(c.from, c.to); got != c.want {
			t.Errorf("Slice(%d, %d) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
	// What a slice draws is what the row shows in those columns.
	if got := Parse(line.Slice(1, 3), 1).Lines[0].Text; got != "bc" {
		t.Errorf("Slice(1, 3) draws %q, want \"bc\"", got)
	}
}
