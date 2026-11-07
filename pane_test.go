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
