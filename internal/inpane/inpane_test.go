package inpane

import (
	"os"
	"strings"
	"testing"

	"github.com/xeind/rex-fingers/internal/screen"
)

func TestRestoreRepaintsEveryRowAndUndoesBegin(t *testing.T) {
	scr := screen.Parse("\x1b[1mone\x1b[0m\r\n\r\nthree", 3)
	got := Restore(scr, true, true)
	want := "\x1b[1;1H\x1b[0m\x1b[2K\x1b[1mone\x1b[0m" +
		"\x1b[2;1H\x1b[0m\x1b[2K" +
		"\x1b[3;1H\x1b[0m\x1b[2Kthree" +
		"\x1b[0m\x1b[?7h\x1b8\x1b[?25h"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestRestoreKeepsModesThatWereOff(t *testing.T) {
	got := Restore(screen.Parse("x", 1), false, false)
	if strings.Contains(got, "\x1b[?7h") || strings.Contains(got, "\x1b[?25h") {
		t.Fatalf("restored a mode that was off: %q", got)
	}
}

func TestTTYOfThisProcess(t *testing.T) {
	path, err := TTY(os.Getpid())
	if err != nil {
		t.Skipf("no terminal in this test run: %v", err)
	}
	if !strings.HasPrefix(path, "/dev/") {
		t.Fatalf("got %q", path)
	}
}
