package ui

import (
	"strings"
	"testing"

	"github.com/xeind/rex-fingers/internal/patterns"
	"github.com/xeind/rex-fingers/internal/screen"
)

// Two screens share one hint list: no hint repeats across them, the same
// text gets the same hint everywhere, and a press finds the match in the
// screen that holds it.
func TestSetSharesHintsAndRoutesPress(t *testing.T) {
	set, err := patterns.Compile([]string{"path"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ovs := NewSet([]screen.Screen{
		screen.Parse("/tmp/a /tmp/b", 0),
		screen.Parse("/tmp/c /tmp/a", 0),
	}, set, Options{Alphabet: "asdf"})
	seen := map[string]string{}
	for i, o := range ovs {
		for _, tg := range o.Targets() {
			if prev, ok := seen[tg.Hint]; ok && prev != tg.Text {
				t.Fatalf("hint %q names %q and %q", tg.Hint, prev, tg.Text)
			}
			seen[tg.Hint] = tg.Text
			if tg.Text == "/tmp/a" && tg.Hint != ovs[0].Targets()[0].Hint {
				t.Fatalf("screen %d: /tmp/a got hint %q, want %q", i, tg.Hint, ovs[0].Targets()[0].Hint)
			}
		}
	}
	if len(seen) != 3 {
		t.Fatalf("want 3 distinct hints for 3 texts, got %v", seen)
	}
	var cHint string
	for _, tg := range ovs[1].Targets() {
		if tg.Text == "/tmp/c" {
			cHint = tg.Hint
		}
	}
	pane, sel, typed, done := PressSet(ovs, []byte(cHint), "")
	if !done || pane != 1 || sel.Target == nil || sel.Target.Text != "/tmp/c" {
		t.Fatalf("press %q: pane %d done %v typed %q sel %+v", cHint, pane, done, typed, sel)
	}
	pane, _, typed, done = PressSet(ovs, []byte("z"), "")
	if done || pane != -1 || typed != "" {
		t.Fatalf("letter outside the alphabet: pane %d done %v typed %q", pane, done, typed)
	}
}

// The heavier pattern gets the first hint, whatever the screen order: a
// file:line below a path takes "a".
func TestHeavierPatternGetsFirstHint(t *testing.T) {
	set, err := patterns.Compile([]string{"path", "file-line"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	o := New(screen.Parse("/tmp/a\r\nmain.go:3", 0), set, Options{Alphabet: "asdf"})
	got := map[string]string{}
	for _, tg := range o.Targets() {
		got[tg.Text] = tg.Hint
	}
	if got["main.go:3"] != "a" || got["/tmp/a"] != "s" {
		t.Fatalf("hints %v", got)
	}
}

// A path the terminal wrapped is one target: the full text, one segment
// per row, the hint on the first row, and a press copies the whole path.
func TestWrappedMatchIsOneTarget(t *testing.T) {
	set, err := patterns.Compile([]string{"path"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	scr := screen.Parse("see local/codex-\r\nfindings.md now", 0)
	scr.Join(16, "see local/codex-findings.md now")
	if !scr.Lines[0].Wrap {
		t.Fatal("row 0 should wrap")
	}
	o := New(scr, set, Options{Alphabet: "asdf"})
	ts := o.Targets()
	if len(ts) != 1 || ts[0].Text != "local/codex-findings.md" {
		t.Fatalf("targets %+v", ts)
	}
	want := []Segment{{0, 4, 12}, {1, 0, 11}}
	if len(ts[0].Segments) != 2 || ts[0].Segments[0] != want[0] || ts[0].Segments[1] != want[1] {
		t.Fatalf("segments %+v, want %+v", ts[0].Segments, want)
	}
	if ts[0].Row != 0 || ts[0].Cell != 4 {
		t.Fatalf("first segment fields: %+v", ts[0])
	}
	var b strings.Builder
	o.Paint(&b, 2, 16, "")
	if !strings.Contains(b.String(), "\x1b[2;1H") {
		t.Fatal("second row of the match was not highlighted")
	}
}

// Painted gives the screen text with the hints in place, for each hint
// position, so the in-pane mode can tell a pane that still shows them.
func TestPaintedText(t *testing.T) {
	scr := screen.Parse("ab /tmp/x  y /tmp/z", 0)
	targets := []Target{
		{Hint: "s", Row: 0, Cell: 3, Span: 6},  // /tmp/x
		{Hint: "d", Row: 0, Cell: 11, Span: 1}, // y
		{Hint: "f", Row: 0, Cell: 13, Span: 6}, // /tmp/z
	}
	cases := []struct{ position, typed, want string }{
		{"left", "", "ab stmp/x  d ftmp/z"},
		{"right", "", "ab /tmp/s  d /tmp/f"},
		{"gutter", "", "abs/tmp/x dyf/tmp/z"},
		{"left", "x", "ab /tmp/x  y /tmp/z"},
	}
	for _, c := range cases {
		o := &Overlay{scr: scr, targets: targets, opts: Options{HintPosition: c.position}}
		var b strings.Builder
		o.Paint(&b, 2, 19, c.typed)
		got := o.Painted()
		if len(got) != 2 || got[0] != c.want || got[1] != "" {
			t.Errorf("%s typed %q: got %q, want [%q \"\"]", c.position, c.typed, got, c.want)
		}
	}
	// Gutter with no blank before the match covers its end instead, unless
	// the match reaches the right edge, where it covers the start.
	tight := &Overlay{scr: screen.Parse("ab/tmp/x /tmp/y", 0), opts: Options{HintPosition: "gutter"},
		targets: []Target{{Hint: "s", Row: 0, Cell: 2, Span: 6}, {Hint: "d", Row: 0, Cell: 9, Span: 6}}}
	var tb strings.Builder
	tight.Paint(&tb, 1, 15, "")
	if got := tight.Painted(); got[0] != "ab/tmp/sd/tmp/y" {
		t.Errorf("gutter fallback: got %q", got)
	}
	edge := &Overlay{scr: screen.Parse("ab/tmp/x", 0), opts: Options{HintPosition: "gutter"},
		targets: []Target{{Hint: "s", Row: 0, Cell: 2, Span: 6}}}
	edge.Paint(&tb, 1, 8, "")
	if got := edge.Painted(); got[0] != "abstmp/x" {
		t.Errorf("gutter fallback at the right edge: got %q", got)
	}
	// A hint over a wide character is padded to its width.
	wide := &Overlay{scr: screen.Parse("日本 /tmp/x", 0), targets: []Target{{Hint: "s", Row: 0, Cell: 0, Span: 2}},
		opts: Options{HintPosition: "left"}}
	var b strings.Builder
	wide.Paint(&b, 1, 20, "")
	if got := wide.Painted(); got[0] != "s 本 /tmp/x" {
		t.Errorf("wide: got %q", got)
	}
}

// A row that fills the width is not erased to its end: with autowrap off
// the cursor stays on the last column and the erase would take that cell.
func TestPaintKeepsLastCellOfFullRow(t *testing.T) {
	o := &Overlay{scr: screen.Parse("12345\r\n123", 0), opts: Options{NoDim: true}}
	var b strings.Builder
	o.Paint(&b, 2, 5, "")
	got := b.String()
	if strings.Contains(got, "12345\x1b[0m\x1b[K") {
		t.Fatalf("full row erased to end of line: %q", got)
	}
	if !strings.Contains(got, "123\x1b[0m\x1b[K") {
		t.Fatalf("short row not erased to end of line: %q", got)
	}
}

func TestFlashWritesReverseVideoOverTheMatch(t *testing.T) {
	scr := screen.Parse("see /tmp/a", 0)
	o := &Overlay{scr: scr, opts: Options{}}
	var b strings.Builder
	o.Paint(&b, 1, 20, "")
	b.Reset()
	o.Flash(&b, &Target{Hint: "s", Row: 0, Cell: 4, Span: 6})
	if want := "\x1b[1;5H\x1b[0m\x1b[7m/tmp/a\x1b[0m"; b.String() != want {
		t.Fatalf("got  %q\nwant %q", b.String(), want)
	}
}

func TestGutter(t *testing.T) {
	scr := screen.Parse("ab /tmp/x  y /tmp/z", 0)
	path := Target{Hint: "s", Row: 0, Cell: 3, Span: 6}    // /tmp/x
	y := Target{Hint: "d", Row: 0, Cell: 11, Span: 1}      // y
	second := Target{Hint: "f", Row: 0, Cell: 13, Span: 6} // /tmp/z
	o := &Overlay{scr: scr, targets: []Target{path, y, second}, opts: Options{HintPosition: "gutter"}}

	if start, ok := o.gutter(path); !ok || start != 2 {
		t.Fatalf("one blank before the path: want 2,true got %d,%v", start, ok)
	}
	if start, ok := o.gutter(second); !ok || start != 12 {
		t.Fatalf("blank after y: want 12,true got %d,%v", start, ok)
	}
	if _, ok := o.gutter(Target{Hint: "sd", Row: 0, Cell: 3, Span: 6}); ok {
		t.Fatal("two-letter hint with one blank must fall back")
	}
	if _, ok := o.gutter(Target{Hint: "s", Row: 0, Cell: 0, Span: 2}); ok {
		t.Fatal("match at the left edge must fall back")
	}
	o.targets = []Target{{Hint: "a", Row: 0, Cell: 1, Span: 2}, path} // "b " is a match
	if _, ok := o.gutter(path); ok {
		t.Fatal("blank inside another match must fall back")
	}
	o.opts.HintPosition = "left"
	if _, ok := o.gutter(second); ok {
		t.Fatal("left mode never uses the gutter")
	}
}
