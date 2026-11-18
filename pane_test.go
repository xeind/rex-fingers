package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/xeind/rex-fingers/internal/screen"
)

// A run left on a tty is taken over when the pane still shows what it
// painted: its capture replaces the new one under the new paths, with the
// modes the pane had. A changed pane drops the run and its files.
func TestRecoverLeftoverRun(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dir := t.TempDir()
	tty := "/dev/ttys-test"
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	begin := func() (paneSpec, paneSpec) {
		old := paneSpec{tty: tty, vtFile: write("old.vt", "see /tmp/a\r\nend"), textFile: write("old.txt", "see /tmp/a\nend"),
			rows: 2, cols: 20, cursor: true, wrap: true}
		if err := old.saveRun([]string{"see stmp/a", "end"}, ""); err != nil {
			t.Fatal(err)
		}
		// The new capture is the stale frame, read with the modes Begin left.
		fresh := paneSpec{tty: tty, vtFile: write("new.vt", "see stmp/a\r\nend"), textFile: write("new.txt", "see stmp/a\nend"),
			rows: 2, cols: 20, cursor: false, wrap: false}
		if err := fresh.load(); err != nil {
			t.Fatal(err)
		}
		return old, fresh
	}

	old, fresh := begin()
	if err := fresh.recover(); err != nil {
		t.Fatal(err)
	}
	if !fresh.recovered || !fresh.cursor || !fresh.wrap {
		t.Fatalf("recovered %v cursor %v wrap %v", fresh.recovered, fresh.cursor, fresh.wrap)
	}
	if got := fresh.scr.Lines[0].Text; got != "see /tmp/a" {
		t.Fatalf("screen after recovery: %q", got)
	}
	for _, gone := range []string{old.vtFile, old.textFile, runFile(tty)} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s still exists", gone)
		}
	}

	old, fresh = begin()
	fresh.scr = screen.Parse("see stmp/a\r\n$ ls", 2)
	if err := fresh.recover(); err != nil {
		t.Fatal(err)
	}
	if fresh.recovered || fresh.cursor {
		t.Fatalf("a changed pane must drop the run: %+v", fresh)
	}
	if _, err := os.Stat(old.vtFile); err == nil {
		t.Error("dropped run's capture still exists")
	}
	if _, err := os.Stat(fresh.vtFile); err != nil {
		t.Error("new capture was removed")
	}
}

// A pane that scrolled two rows while the hints were up: the rows that
// still show the frame are found at the offset, the new output is not.
func TestAlignRowsFindsTheScroll(t *testing.T) {
	painted := []string{"$ ls", "atmp/a", "stmp/b", "dtmp/c", "$ ", ""}
	cur := []string{"stmp/b", "dtmp/c", "$ echo hi", "hi", "$ ", ""}
	rows, k := alignRows(cur, painted)
	if len(rows) != 2 || rows[0] != 2 || rows[1] != 3 || k != 2 {
		t.Fatalf("rows %v at offset %d, want {0:2 1:3} at 2", rows, k)
	}
	if rows, _ := alignRows(painted, painted); len(rows) != 5 || rows[1] != 1 {
		t.Fatalf("unchanged screen: %v", rows)
	}
	if rows, _ := alignRows([]string{"x", "y"}, painted); len(rows) != 0 {
		t.Fatalf("replaced screen: %v", rows)
	}
}

// Rows the pane's program wrote into while the hints were up. A prompt
// row with a character typed after it still shows its hint: the captured
// cells go back over the columns that show the paint, and the typed
// character is not written over. A row the program redrew, hint cell
// included, and a row of new output are left alone.
func TestWrittenRowsPutBackOnlyThePaintedCells(t *testing.T) {
	scr := screen.Parse("/tmp/a\r\n\x1b[34m~/dev/x\x1b[0m main > \r\nold line\r\nstays", 4)
	painted := []string{"atmp/a", "s/dev/x main > ", "old line", "stays"}
	lines := screen.Parse("/tmp/a!\r\n"+
		"\x1b[2m\x1b[7ms\x1b[0;2m/dev/x main > \x1b[0m\x1b[31m1\x1b[0m\r\n"+
		"something else\r\n"+
		"stays", 4).Lines
	cur := make([]string, len(lines))
	for i, l := range lines {
		cur[i] = l.Text
	}
	rows, k := alignRows(cur, painted)
	if len(rows) != 1 || rows[3] != 3 || k != 0 {
		t.Fatalf("rows %v at offset %d, want {3:3} at 0", rows, k)
	}
	over := writtenRows(lines, painted, scr, k, rows)
	want := "\x1b[2;1H\x1b[0m\x1b[34m~/dev/x\x1b[0m main > \x1b[0m"
	if over[1] != want {
		t.Errorf("the prompt row: %q, want %q", over[1], want)
	}
	if len(over) != 1 {
		t.Errorf("rows patched: %v, want only the prompt row", over)
	}
}

// The pane's capture runs from the top of the scrollback to the last
// screen row, blank rows included. A screen with scrollback above and
// blank rows below, unchanged since the paint, has not moved, so restore
// repaints all of it.
func TestCheckMovedReadsTheScreenRows(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	tty := "/dev/ttys-test"
	p := paneSpec{tty: tty, rows: 5}
	if err := p.saveRun([]string{"atmp/a", "$", "", "", ""}, ""); err != nil {
		t.Fatal(err)
	}
	p.currentFile = filepath.Join(t.TempDir(), "current.vt")
	vt := "1\r\n2\r\n3\r\n\x1b[2m\x1b[1;38;5;0;48;5;9ma\x1b[0;2mtmp/a\x1b[0m\r\n$ \r\n\r\n\r\n"
	if err := os.WriteFile(p.currentFile, []byte(vt), 0o600); err != nil {
		t.Fatal(err)
	}
	p.checkMoved()
	if p.moved {
		t.Fatalf("unchanged screen read as moved; rows %v", p.softRows)
	}
}

func TestShowsPainted(t *testing.T) {
	scr := screen.Parse("see stmp/a   \r\n\r\n", 3)
	if !showsPainted(scr, []string{"see stmp/a", "", ""}) {
		t.Error("trailing blanks must not matter")
	}
	if !showsPainted(scr, []string{"see stmp/a"}) {
		t.Error("missing trailing rows must count as blank")
	}
	if showsPainted(scr, []string{"see /tmp/a", "", ""}) || showsPainted(scr, nil) {
		t.Error("a different frame, or none, must not match")
	}
}

func TestParsePaneSpec(t *testing.T) {
	p, err := parsePaneSpec("pid=42,vt=/tmp/x.vt,rows=24,cols=80,cursor=false,wrap=true")
	if err != nil {
		t.Fatal(err)
	}
	if p.pid != 42 || p.vtFile != "/tmp/x.vt" || p.rows != 24 || p.cols != 80 || p.cursor || !p.wrap {
		t.Fatalf("got %+v", p)
	}
	p, err = parsePaneSpec("tty=/dev/ttys003,vt=/tmp/y.vt,rows=10")
	if err != nil || p.tty != "/dev/ttys003" || !p.cursor || !p.wrap {
		t.Fatalf("defaults: %+v, %v", p, err)
	}
	for _, bad := range []string{"rows", "pid=x,vt=a,rows=1", "size=3,vt=a,rows=1"} {
		if _, err := parsePaneSpec(bad); err == nil {
			t.Errorf("parsePaneSpec(%q) accepted", bad)
		}
	}
}

// The rows a scrolled pane shows are found in the capture by their text.
// No match, or more than one, means the live screen: a guessed page would
// hint text the user is not looking at.
func TestFindOffset(t *testing.T) {
	lines := []string{"a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8"}
	wide := []string{"start", "0123456789", "tail   ", "", "x y", "end", "last"}
	for _, c := range []struct {
		name  string
		lines []string
		view  string
		rows  int
		want  int
	}{
		{"the live screen", lines, "a6\na7\na8", 3, 0},
		{"one row up", lines, "a5\na6\na7", 3, 1},
		{"the top page", lines, "a1\na2\na3", 3, 5},
		{"a trailing newline", lines, "a2\na3\na4\n", 3, 4},
		{"not in the capture", lines, "b1\nb2\nb3", 3, 0},
		{"an empty view", lines, "", 3, 0},
		{"a blank view", lines, "\n\n", 3, 0},
		{"a capture no taller than the pane", lines[:3], "a1\na2\na3", 3, 0},
		{"a wrapped line copied as one line", wide, "0123456789tail\n\nx y", 4, 2},
		{"blank rows at the foot dropped by the copy", wide, "start\n0123456789\ntail", 4, 3},
		{"spaces differ", wide, "tail\n\nxy\nend", 4, 1},
		// screen.Parse drops zero-width characters; the app's copy keeps them.
		{"a variation selector and a combining mark in the copy", []string{"top", "warn ⚠ cafe", "mid", "end", "last"}, "warn ⚠️ café\nmid", 2, 2},
	} {
		if got := findOffset(c.lines, c.view, c.rows); got != c.want {
			t.Errorf("%s: findOffset = %d, want %d", c.name, got, c.want)
		}
	}
	// The same rows in several places read the same wherever they are
	// painted from, so the nearest page is taken. A pane that kept such a
	// page fell back to the live screen and looked as if it had jumped to
	// the bottom.
	twice := []string{"same", "rows", "same", "rows", "same", "rows", "tail"}
	if got := findOffset(twice, "same\nrows", 2); got != 1 {
		t.Errorf("a view found in several places: findOffset = %d, want 1, the nearest", got)
	}
	// When the live screen is one of them, the pane is not scrolled.
	if got := findOffset([]string{"same", "rows", "same", "rows"}, "same\nrows", 2); got != 0 {
		t.Errorf("a view that is also the live screen: findOffset = %d, want 0", got)
	}
	// A blank row at the edge lets the page start a row early or late; the
	// page whose rows line up with the copied lines is the one in view.
	edge := []string{"top", "", "a", "b", "", "end", "last"}
	if got := findOffset(edge, "\na\nb", 3); got != 3 {
		t.Errorf("a blank row above the view: findOffset = %d, want 3", got)
	}
	if got := findOffset(edge, "a\nb\n", 3); got != 2 {
		t.Errorf("a blank row below the view: findOffset = %d, want 2", got)
	}
}

// A page above the live screen is hinted on the alternate screen, and the
// run state carries the page so press and restore act on it.
func TestLocateScrolledPage(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	vt := "old /tmp/a\r\nold /tmp/b\r\nmid\r\nnew /tmp/c\r\n$ "
	p := paneSpec{tty: "/dev/ttys-scroll", vtFile: write("s.vt", vt), viewFile: write("s.view", "old /tmp/a\nold /tmp/b"),
		rows: 2, cols: 20, cursor: true, wrap: true}
	if err := p.load(); err != nil {
		t.Fatal(err)
	}
	if got := p.scr.Lines[0].Text; got != "new /tmp/c" {
		t.Fatalf("before locate the page is the live screen, got %q", got)
	}
	p.locate()
	if p.offset != 3 || !p.alt || p.scr.Lines[0].Text != "old /tmp/a" || p.scr.Lines[1].Text != "old /tmp/b" {
		t.Fatalf("offset %d alt %v page %q, %q", p.offset, p.alt, p.scr.Lines[0].Text, p.scr.Lines[1].Text)
	}
	if _, err := os.Stat(p.viewFile); err == nil {
		t.Error("locate left the view file")
	}
	if err := p.saveRun([]string{"old stmp/a", "old dtmp/b"}, ""); err != nil {
		t.Fatal(err)
	}

	// press and restore name the same capture and take the page from the
	// run state.
	later := paneSpec{tty: p.tty, vtFile: p.vtFile, rows: 2, cols: 20, cursor: true, wrap: true}
	if err := later.load(); err != nil {
		t.Fatal(err)
	}
	later.modesFromRun()
	if later.offset != 3 || !later.alt || later.scr.Lines[0].Text != "old /tmp/a" {
		t.Fatalf("from the run state: offset %d alt %v page %q", later.offset, later.alt, later.scr.Lines[0].Text)
	}

	// A view that is the live screen keeps the in-place path.
	live := paneSpec{tty: "/dev/ttys-live", vtFile: write("l.vt", vt), viewFile: write("l.view", "new /tmp/c\n$"),
		rows: 2, cols: 20, cursor: true, wrap: true}
	if err := live.load(); err != nil {
		t.Fatal(err)
	}
	live.locate()
	if live.offset != 0 || live.alt {
		t.Fatalf("the live screen: offset %d alt %v", live.offset, live.alt)
	}
}

func TestKeyBytes(t *testing.T) {
	cases := []struct {
		key, mod string
		want     []byte
	}{
		{"a", "main", []byte("a")},
		{"a", "shift", []byte("A")},
		{"a", "ctrl", []byte{0x01}},
		{"z", "alt", []byte{0x1b, 'z'}},
	}
	for _, c := range cases {
		got, err := keyBytes(c.key, c.mod)
		if err != nil || !bytes.Equal(got, c.want) {
			t.Errorf("keyBytes(%q, %q) = %q, %v; want %q", c.key, c.mod, got, err, c.want)
		}
	}
	for _, bad := range [][2]string{{"A", "main"}, {"ab", "main"}, {"a", "super"}} {
		if _, err := keyBytes(bad[0], bad[1]); err == nil {
			t.Errorf("keyBytes(%q, %q) accepted", bad[0], bad[1])
		}
	}
}
