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
func RestoreRows(scr screen.Screen, rows map[int]int, cursorVisible, wraparound bool) string {
	var b strings.Builder
	for row := 0; row < len(scr.Lines)+len(rows); row++ {
		src, ok := rows[row]
		if !ok || src < 0 || src >= len(scr.Lines) {
			continue
		}
		fmt.Fprintf(&b, "\x1b7\x1b[%d;1H\x1b[0m\x1b[2K%s\x1b8", row+1, scr.Lines[src].VT)
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
