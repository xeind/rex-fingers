package cli

// draw --begin --await-views hints the page each pane is scrolled to. Rex
// reports no scroll position, so the Lua plugin has the app copy the rows
// each pane shows, in copy mode, one pane after the other in the order of
// the --pane specs. Those copies arrive on the clipboard after the Lua
// action has returned, and the app has no way to call Lua back that does
// not move a scrolled pane to its last row: a key sent to a pane does.
// So this program waits for them itself.
//
// The call the plugin makes saves the clipboard, puts a marker text on it,
// starts a copy of itself in the background, and returns at once with the
// ttys. The background copy takes each pane's rows as the clipboard
// changes, puts the saved clipboard back, and draws.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// awaitEnv names the file with the saved clipboard, and marks the
// background copy.
const awaitEnv = "REX_FINGERS_AWAIT"

// awaitTimeout is how long the background copy waits for one pane's rows.
// The app took 0.5 to 1.5 seconds a pane.
const awaitTimeout = 3 * time.Second

// waitFile marks a pane whose run is still waiting for its rows. press
// ignores keys while it exists, and restore deletes it to call the run off.
func waitFile(tty string) string {
	return filepath.Join(os.TempDir(), "rex-fingers-"+filepath.Base(tty)+".wait")
}

// marker is the text put on the clipboard before pane i is copied. The
// clipboard holding anything else means the app has copied that pane.
func marker(saved string, i int) string {
	return fmt.Sprintf("rex-fingers-waiting-%s-%d", filepath.Base(saved), i)
}

// awaitStart saves the clipboard, starts the background copy, and replies
// with the ttys, which the plugin needs for press and restore.
func awaitStart(specs []paneSpec) error {
	for i := range specs {
		if err := specs[i].resolve(); err != nil {
			return err
		}
	}
	text, err := readClipboard()
	if err != nil {
		return err
	}
	f, err := os.CreateTemp("", "rex-fingers-clipboard-")
	if err != nil {
		return err
	}
	saved := f.Name()
	_, err = f.WriteString(text)
	f.Close()
	if err == nil {
		err = writeClipboard(marker(saved, 0))
	}
	if err != nil {
		os.Remove(saved)
		return err
	}
	for _, p := range specs {
		os.WriteFile(waitFile(p.tty), nil, 0o600)
	}
	self, err := os.Executable()
	if err == nil {
		cmd := exec.Command(self, os.Args[1:]...)
		cmd.Env = append(os.Environ(), awaitEnv+"="+saved)
		// Its own session and no inherited output, so the caller's wait
		// for this process ends when this process does.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err = cmd.Start(); err == nil {
			cmd.Process.Release()
		}
	}
	if err != nil {
		writeClipboard(text)
		os.Remove(saved)
		for _, p := range specs {
			os.Remove(waitFile(p.tty))
		}
		return err
	}
	return reply(map[string]any{"waiting": true, "tty": specs[0].tty, "panes": ttys(specs)})
}

// awaitViews runs in the background copy. It takes the rows of the first
// count panes from the clipboard, each into its pane's view file, then
// puts the saved clipboard back. The panes after them are hinted on their
// live screens. It reports false when the run was called off meanwhile; the
// captures are then deleted and nothing is drawn. A pane whose rows never
// come, as when the app refused a step, is hinted on its live screen, and
// so are the panes after it.
func awaitViews(specs []paneSpec, count int, saved string) bool {
	for i := range specs {
		specs[i].resolve()
	}
	if count > len(specs) {
		count = len(specs)
	}
	called := func() bool {
		_, err := os.Stat(waitFile(specs[0].tty))
		return err != nil
	}
wait:
	for i := range specs[:count] {
		mark := marker(saved, i)
		if i > 0 && writeClipboard(mark) != nil {
			break
		}
		deadline := time.Now().Add(awaitTimeout)
		empty := 0
		for {
			if called() || time.Now().After(deadline) {
				break wait
			}
			text, err := readClipboard()
			// A writer clears the clipboard, then fills it; a read in
			// between finds it empty. Empty counts as the pane's rows only
			// when it stays empty, which a blank pane gives.
			if err == nil && text == "" && empty < 8 {
				empty++
				time.Sleep(15 * time.Millisecond)
				continue
			}
			if err == nil && text != mark {
				path := specs[i].vtFile + ".view"
				if os.WriteFile(path, []byte(text), 0o600) == nil {
					specs[i].viewFile = path
				}
				break
			}
			time.Sleep(15 * time.Millisecond)
		}
	}
	if text, err := os.ReadFile(saved); err == nil {
		writeClipboard(string(text))
	}
	os.Remove(saved)
	off := called()
	for _, p := range specs {
		os.Remove(waitFile(p.tty))
	}
	if off {
		for _, p := range specs {
			os.Remove(p.viewFile)
			p.remove()
		}
		return false
	}
	return true
}
