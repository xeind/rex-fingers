package inpane

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

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

// RestoreRows repaints the given rows around the cursor, which it saves
// and restores for each, and resets the modes without a final DECRC.
func TestRestoreRowsLeavesTheCursorAlone(t *testing.T) {
	scr := screen.Parse("one\r\ntwo\r\nthree", 3)
	got := RestoreRows(scr, map[int]int{0: 1}, nil, true, false)
	want := "\x1b7\x1b[1;1H\x1b[0m\x1b[2Ktwo\x1b8\x1b[0m\x1b[?25h"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// A process with no terminal gives the writer nothing to write to: it
// prints WriterFailed and exits before it runs cat.
func TestWriterFailsWithoutATerminal(t *testing.T) {
	sleep := exec.Command("sleep", "5")
	sleep.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := sleep.Start(); err != nil {
		t.Fatal(err)
	}
	defer sleep.Process.Kill()
	argv := append(WriterCommand(), strconv.Itoa(sleep.Process.Pid))
	out, err := exec.Command(argv[0], argv[1:]...).Output()
	if err == nil {
		t.Fatalf("writer exited 0; output %q", out)
	}
	if string(out) != WriterFailed {
		t.Fatalf("got %q, want %q", out, WriterFailed)
	}
}

// The reader prints its mark, then each byte as two hex digits on a row.
// Without a terminal stty fails and the rest still runs.
func TestReaderPrintsKeysAsHex(t *testing.T) {
	argv := ReaderCommand(5)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader("aS\x1b")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("reader: %v", err)
	}
	if want := ReaderMark + "61\n53\n1b\n"; string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}

// The substitute writes a first line, its terminal's path, and then each
// byte as it is. Without a terminal that line says so.
func TestSubstituteWritesKeysToItsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "keys")
	argv := append(SubstituteCommand(5), file)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader("aS\x1b")
	if err := cmd.Run(); err != nil {
		t.Fatalf("substitute: %v", err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	_, keys, found := strings.Cut(string(got), "\n")
	if !found || keys != "aS\x1b" {
		t.Fatalf("got %q, want a first line and then %q", got, "aS\x1b")
	}
}

// A reader nobody closes ends itself when its lifetime is over.
func TestReaderEndsItself(t *testing.T) {
	argv := ReaderCommand(1)
	cmd := exec.Command(argv[0], argv[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("the reader was still alive 5 s after a lifetime of 1 s")
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
