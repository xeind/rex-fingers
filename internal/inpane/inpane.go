// Package inpane draws into the target pane's own tty instead of a
// floating pane. Rex has no API that draws over a pane, but the pane's tty
// belongs to the user, and what is written to it shows in the pane.
//
// Begin saves the cursor and stops autowrap before the hints are drawn.
// Restore repaints every screen row from the VT snapshot and puts the
// cursor, autowrap, and cursor visibility back. Nothing scrolls, so the
// scrollback is never touched.
package inpane

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/xeind/rex-fingers/internal/screen"
)

// Begin saves the cursor and its style (DECSC), hides it, and turns off
// autowrap so that drawing the last cell of the last row cannot scroll.
const Begin = "\x1b7\x1b[?25l\x1b[?7l"

// BeginAlt switches to the alternate screen, which saves the cursor and
// clears that screen, then hides the cursor and turns off autowrap. A pane
// scrolled up shows its scrollback, which no tty write reaches; the
// alternate screen has no scrollback, so the pane shows it wherever it was
// scrolled, and goes back to that place when RestoreAlt leaves it.
const BeginAlt = "\x1b[?1049h\x1b[?25l\x1b[?7l"

// RestoreAlt leaves the alternate screen and undoes BeginAlt. The screen
// and scrollback under it were never written, so nothing is repainted.
func RestoreAlt(cursorVisible, wraparound bool) string {
	var b strings.Builder
	b.WriteString("\x1b[0m")
	if wraparound {
		b.WriteString("\x1b[?7h")
	}
	b.WriteString("\x1b[?1049l")
	if cursorVisible {
		b.WriteString("\x1b[?25h")
	}
	return b.String()
}

// Restore returns the bytes that repaint the screen rows from the snapshot
// and undo Begin. cursorVisible and wraparound are the pane's modes as
// they were before Begin.
func Restore(scr screen.Screen, cursorVisible, wraparound bool) string {
	var b strings.Builder
	for i, line := range scr.Lines {
		fmt.Fprintf(&b, "\x1b[%d;1H\x1b[0m\x1b[2K%s", i+1, line.VT)
	}
	b.WriteString("\x1b[0m")
	if wraparound {
		b.WriteString("\x1b[?7h")
	}
	b.WriteString("\x1b8")
	if cursorVisible {
		b.WriteString("\x1b[?25h")
	}
	return b.String()
}

// RestoreRows repaints only the screen rows in rows, each from the
// snapshot row it maps to, and undoes Begin without moving the cursor.
// It is for a pane that wrote output while the hints were up: the rows
// that still show hints are put back, the new output is left alone, and
// the cursor stays where the pane's program put it.
//
// over holds, for a row the program wrote into, VT that positions the
// cursor itself and puts back only the cells that still show the hints.
// Such a row is not cleared, so what the program wrote there stays.
func RestoreRows(scr screen.Screen, rows map[int]int, over map[int]string, cursorVisible, wraparound bool) string {
	var b strings.Builder
	for row := 0; row < len(scr.Lines)+len(rows)+len(over); row++ {
		if src, ok := rows[row]; ok && src >= 0 && src < len(scr.Lines) {
			fmt.Fprintf(&b, "\x1b7\x1b[%d;1H\x1b[0m\x1b[2K%s\x1b8", row+1, scr.Lines[src].VT)
		} else if seq := over[row]; seq != "" {
			fmt.Fprintf(&b, "\x1b7%s\x1b8", seq)
		}
	}
	b.WriteString("\x1b[0m")
	if wraparound {
		b.WriteString("\x1b[?7h")
	}
	if cursorVisible {
		b.WriteString("\x1b[?25h")
	}
	return b.String()
}

// TTY returns the path of the terminal that process pid runs on, such as
// /dev/ttys003.
func TTY(pid int) (string, error) {
	out, err := exec.Command("ps", "-o", "tty=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", fmt.Errorf("ps -p %d: %w", pid, err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" || name == "?" || name == "??" {
		return "", fmt.Errorf("process %d has no terminal", pid)
	}
	return "/dev/" + name, nil
}

// Writer markers: the writer prints one of them on its own screen, which
// the caller reads to learn whether it reached the pane's tty.
const (
	WriterReady  = "rex-fingers-writer-ready"
	WriterFailed = "rex-fingers-writer-failed"
)

// WriterCommand is the argv of a program that copies its input to the tty
// of a process, for a pane on another machine. The caller appends the
// process ID. It finds the tty as TTY does, puts its own terminal in raw
// mode so the bytes pass unchanged, prints WriterReady, and runs cat. It
// needs only sh, ps, tr, stty, and cat, which every Unix host has.
func WriterCommand() []string {
	script := `t=$(ps -o tty= -p "$1" | tr -d ' ')
case "$t" in ""|"?"|"??") printf ` + WriterFailed + `; exit 1 ;; esac
[ -w "/dev/$t" ] || { printf ` + WriterFailed + `; exit 1; }
stty raw -echo || { printf ` + WriterFailed + `; exit 1; }
printf ` + WriterReady + `
exec cat > "/dev/$t"`
	return []string{"sh", "-c", script, "sh"}
}

// ReaderMark is what the reader prints before the first key, so that its
// screen is told apart from one that has not started.
const ReaderMark = "R"

// ReaderTitle is the terminal title of the reader's pane.
const ReaderTitle = "fingers"

// ReaderCommand is the argv of a program that reports the keys typed into
// its terminal, for a pane on another machine: it sets the terminal raw,
// prints ReaderMark, and then prints each byte it reads as two hex digits
// and a newline. It ends itself after lifetime seconds, so a reader left
// behind does not keep the focus for good. It needs only sh, stty, sleep,
// and hexdump.
//
// The app names a pane after its foreground program, hexdump, unless
// that program set a title. Rex takes the program in the foreground when
// the title arrives as the one that set it, so the title ReaderTitle is
// written 0.3 s after the start, when sh has become hexdump. It goes to
// stderr, which leaves stdout to the mark and the keys.
func ReaderCommand(lifetime int) []string {
	script := `stty raw -echo
printf ` + ReaderMark + `
(sleep 0.3; printf '\033]2;` + ReaderTitle + `\007' >&2; exec 2>/dev/null; sleep "$1"; kill $$) >/dev/null &
exec hexdump -v -e '1/1 "%02x\n"'`
	return []string{"sh", "-c", script, "sh", strconv.Itoa(lifetime)}
}
