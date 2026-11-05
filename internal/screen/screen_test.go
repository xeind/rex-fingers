package screen

import "testing"

func TestParseExpandsCursorForwardAndDropsStyles(t *testing.T) {
	vt := "\x1b[0m\x1b[1m\x1b[38;5;11m~\x1b[0m/x\x1b[3Cend\x1b[0m\r\n" +
		"\x1b]8;;http://a\x07link\x1b]8;;\x07 日本\r\n"
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
	s := Parse("a\r\nb\r\nc\r\n", 2)
	if len(s.Lines) != 2 || s.Lines[0].Text != "b" {
		t.Fatalf("want b,c got %+v", s.Lines)
	}
}

func TestCellAt(t *testing.T) {
	l := Parse("ab日本cd", 0).Lines[0]
	start, count := l.CellAt(2, 6) // bytes of 日本
	if start != 2 || count != 2 {
		t.Fatalf("want 2,2 got %d,%d", start, count)
	}
}
