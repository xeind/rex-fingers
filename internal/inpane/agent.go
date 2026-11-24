package inpane

import (
	_ "embed"
	"strconv"
)

// agentLua is the script of a run on a Rex host; agent.lua says what it
// does and how the plugin talks to it.
//
//go:embed agent.lua
var agentLua string

// Agent markers: the agent prints them on its own screen, which the
// plugin reads. AgentFailed comes from the shell that starts it, when the
// host has no rex to run the script.
const (
	AgentGathered = "RXF1"
	AgentResult   = "RXF2"
	AgentFailed   = "RXF0"
)

// AgentCommand is the argv of the host agent, for a pane on another
// machine. The caller appends the address of the host's server, the
// session, and the agent's arguments as a JSON object. It runs the
// script with the host's own rex, which Rex names in REX_BIN_DIR for
// every block, so nothing is installed. The terminal is raw, so the
// frames the plugin writes to it pass unchanged. The script is told that
// terminal's path, to put a time limit on a read. It ends itself after
// lifetime seconds, so an agent the plugin never closes is not left for
// good. SIGTERM alone did not end rex while its script waited for input
// (0.1.0-preview.22), so SIGKILL follows. It needs sh, stty, sleep, ps,
// and tty; without tty the script's reads have no time limit.
func AgentCommand(lifetime int) []string {
	script := `rex="$REX_BIN_DIR/rex"
[ -x "$rex" ] || { printf ` + AgentFailed + `; exit 1; }
stty raw -echo || { printf ` + AgentFailed + `; exit 1; }
REX_FINGERS_TTY=$(tty 2>/dev/null)
export REX_FINGERS_TTY
(sleep "$2"; kill $$; sleep 1; kill -9 $$) >/dev/null 2>&1 &
exec "$rex" -S "${3:-$REX_SERVER}" --autostart=false do -e "$1" -s "$4" --args "$5"`
	return []string{"sh", "-c", script, "sh", agentLua, strconv.Itoa(lifetime)}
}
