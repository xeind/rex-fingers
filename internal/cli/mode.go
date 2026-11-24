package cli

// mode turns a key mode of the app on and off, for a run on a pane of a
// Rex host. The hint keys of such a run are bound in a key mode to an
// action: an action bound to a key runs in a host's tab, a Lua function
// does not. The app turns a mode on only when it is asked through this
// machine's server; an action run from a host's tab reaches the app by
// way of the host, where the mode does not exist. So the plugin runs
// this program, which asks as rex -C does. The app must allow remote
// control.
//
// The run's action ends once the hints are up, so nothing of the plugin
// is alive to turn the mode off when no key ever comes. --on starts a
// copy of this program in the background that turns it off after --limit
// seconds, unless --off came first: the token file is the sign that the
// run is still up.

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"

	"github.com/xeind/rex-fingers/internal/rexcli"
)

// modeEnv marks the background copy.
const modeEnv = "REX_FINGERS_MODE_WATCH"

func modeCommand(args []string) error {
	fs := flag.NewFlagSet("mode", flag.ExitOnError)
	client := fs.String("client", "", "")
	name := fs.String("name", "", "")
	on := fs.Bool("on", false, "")
	off := fs.Bool("off", false, "")
	token := fs.String("token", "", "")
	limit := fs.Int("limit", 16, "")
	fs.Usage = usage
	fs.Parse(args)
	if *client == "" || *on == *off || (*on && *name == "") {
		return fmt.Errorf("mode needs --client and one of --on --name NAME and --off")
	}
	if os.Getenv(modeEnv) != "" {
		time.Sleep(time.Duration(*limit) * time.Second)
		if _, err := os.Stat(*token); err != nil {
			return nil
		}
		os.Remove(*token)
		return rexcli.Perform(*client, "client.mode.exit")
	}
	if *off {
		if *token != "" {
			os.Remove(*token)
		}
		if err := rexcli.Perform(*client, "client.mode.exit"); err != nil {
			return err
		}
		return reply(map[string]any{"mode": false})
	}
	if err := rexcli.Perform(*client, "client.mode.enter", "name="+*name); err != nil {
		return err
	}
	f, err := os.CreateTemp("", "rex-fingers-mode-")
	if err != nil {
		rexcli.Perform(*client, "client.mode.exit")
		return err
	}
	f.Close()
	self, err := os.Executable()
	if err == nil {
		cmd := exec.Command(self, "mode", "--client", *client, "--off", "--token", f.Name(), "--limit", strconv.Itoa(*limit))
		cmd.Env = append(os.Environ(), modeEnv+"=1")
		// Its own session and no inherited output, so the caller's wait
		// for this process ends when this process does.
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err = cmd.Start(); err == nil {
			cmd.Process.Release()
		}
	}
	if err != nil {
		os.Remove(f.Name())
		rexcli.Perform(*client, "client.mode.exit")
		return err
	}
	return reply(map[string]any{"mode": true, "token": f.Name()})
}
