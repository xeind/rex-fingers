package screen

import "testing"

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
