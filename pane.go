package main

// The in-pane subcommands draw the hints into the target pane's own tty.
// The Lua plugin runs them through rex.exec and keeps the state between
// key presses: the VT file, the tty, and the letters typed so far. Each
// prints one JSON object on standard output.
//
//	draw     saves the cursor and draws the hints; prints the tty and the
//	         number of hints
//	press    applies one key; repaints, or restores the pane and runs the
//	         action
//	restore  repaints the pane as it was and deletes the VT file
//
// A run covers one pane, given by --pid or --tty with --vt-file, --rows,
// and --cols, or every pane of a window, given by one --pane spec each.
// The panes share one hint list. The first pane is the one the key was
// pressed in: a paste goes there and a shell action runs in its --cwd.
//
// A run can end without restore: a config reload or an app restart drops
// the key mode, and the hints stay painted. So draw records each pane's
// run in a file named after its tty, with the text it painted. The next
// draw --begin, or restore --recover, reads it: when the pane still shows
// that text, the saved capture is the pane as it was and replaces the new
// one; otherwise the run is dropped.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xeind/rex-fingers/internal/inpane"
	"github.com/xeind/rex-fingers/internal/screen"
	"github.com/xeind/rex-fingers/internal/ui"
)

// paneSpec is one pane of a run: how to reach its tty, its capture, its
// size, and the terminal modes to put back.
type paneSpec struct {
	tty    string
	pid    int
	vtFile string
	// textFile is the same screen as unwrapped text; it says which rows
	// the terminal wrapped. Optional.
	textFile string
	rows     int
	cols     int
	cursor   bool
	wrap     bool
	scr      screen.Screen
	// recovered is set when the pane still showed a leftover run's hints
	// and that run's capture now stands in for the new one.
	recovered bool
	// currentFile is the pane's VT as the caller captured it just now,
	// for press and restore. Optional. When it differs from the painted
	// frame the pane wrote output during the run: moved is set and
	// softRows maps each screen row that still shows the frame to the
	// snapshot row to repaint it from.
	currentFile string
	moved       bool
	softRows    map[int]int
}

// runState is what draw leaves beside a pane's capture, so a later call
// can tell whether the pane still shows the hints and put it back.
type runState struct {
	VT      string   `json:"vt"`
	Text    string   `json:"text,omitempty"`
	Rows    int      `json:"rows"`
	Cols    int      `json:"cols"`
	Cursor  bool     `json:"cursor"`
	Wrap    bool     `json:"wrap"`
	Typed   string   `json:"typed"`
	Painted []string `json:"painted"`
}

// runFile names the state file of the run on a tty. One tty has one pane,
// so one run at a time.
func runFile(tty string) string {
	return filepath.Join(os.TempDir(), "rex-fingers-"+filepath.Base(tty)+".run.json")
}

func (p *paneSpec) saveRun(painted []string, typed string) error {
	data, err := json.Marshal(runState{VT: p.vtFile, Text: p.textFile, Rows: p.rows, Cols: p.cols,
		Cursor: p.cursor, Wrap: p.wrap, Typed: typed, Painted: painted})
	if err != nil {
		return err
	}
	return os.WriteFile(runFile(p.tty), data, 0o600)
}

// remove deletes the pane's capture and run state.
func (p *paneSpec) remove() {
	os.Remove(p.vtFile)
	if p.textFile != "" {
		os.Remove(p.textFile)
	}
	os.Remove(runFile(p.tty))
}

// recover looks for a run left on the pane's tty. When the pane still
// shows what that run painted, its capture is the pane as it was: it
// takes the place of the new capture, under the new paths, with the modes
// the pane had, and recovered is set. Otherwise the leftover is dropped.
func (p *paneSpec) recover() error {
	path := runFile(p.tty)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	os.Remove(path)
	var st runState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil
	}
	// A capture written over the old one, which only a hand-run test
	// does, is the painted frame and cannot stand for the pane as it was.
	if st.VT == p.vtFile {
		return nil
	}
	if !showsPainted(p.scr, st.Painted) {
		os.Remove(st.VT)
		if st.Text != "" {
			os.Remove(st.Text)
		}
		return nil
	}
	if err := os.Rename(st.VT, p.vtFile); err != nil {
		return err
	}
	if st.Text != "" && p.textFile != "" {
		if err := os.Rename(st.Text, p.textFile); err != nil {
			return err
		}
	} else {
		os.Remove(st.Text)
		os.Remove(p.textFile)
		p.textFile = ""
	}
	// The stale frame hides the cursor and stops autowrap, so the modes
	// read now are the run's, not the pane's.
	p.cursor, p.wrap = st.Cursor, st.Wrap
	p.recovered = true
	return p.load()
}

// modesFromRun takes the pane's cursor and autowrap modes from its run
// state. The flags carry what the caller read when the run began, which is
// the painted frame's modes when a recovered run began it; the state holds
// the pane's own.
func (p *paneSpec) modesFromRun() {
	data, err := os.ReadFile(runFile(p.tty))
	if err != nil {
		return
	}
	var st runState
	if json.Unmarshal(data, &st) == nil {
		p.cursor, p.wrap = st.Cursor, st.Wrap
	}
}

// showsPainted reports whether the screen's text is the painted frame,
// row for row, trailing blanks aside.
func showsPainted(scr screen.Screen, painted []string) bool {
	rows := make([]string, len(scr.Lines))
	for i, l := range scr.Lines {
		rows[i] = l.Text
	}
	return sameRows(rows, painted)
}

func sameRows(have, want []string) bool {
	if len(want) == 0 {
		return false
	}
	for i := 0; i < len(want) || i < len(have); i++ {
		var h, w string
		if i < len(have) {
			h = strings.TrimRight(have[i], " ")
		}
		if i < len(want) {
			w = strings.TrimRight(want[i], " ")
		}
		if h != w {
			return false
		}
	}
	return true
}

// current returns the text of the pane's screen rows from the VT the
// caller captured, and deletes the file. Nil without one. The text format
// drops blank rows at the foot of the screen, so its last rows would reach
// into the scrollback; the VT keeps every screen row.
func (p *paneSpec) current() []string {
	if p.currentFile == "" {
		return nil
	}
	data, err := os.ReadFile(p.currentFile)
	os.Remove(p.currentFile)
	if err != nil {
		return nil
	}
	scr := screen.Parse(string(data), p.rows)
	lines := make([]string, len(scr.Lines))
	for i, l := range scr.Lines {
		lines[i] = l.Text
	}
	return lines
}

// checkMoved compares the pane's current text with the frame the run
// painted. When they differ the pane wrote output during the run: moved
// is set and softRows says which rows can still be put back.
func (p *paneSpec) checkMoved() {
	cur := p.current()
	if cur == nil {
		return
	}
	data, err := os.ReadFile(runFile(p.tty))
	if err != nil {
		return
	}
	var st runState
	if json.Unmarshal(data, &st) != nil || sameRows(cur, st.Painted) {
		return
	}
	p.moved = true
	p.softRows = alignRows(cur, st.Painted)
}

// alignRows finds how far the pane scrolled, the offset k at which the
// most non-blank painted rows reappear in the current rows, and returns
// the screen rows that still show a painted row, mapped to that row's
// index in the snapshot. Blank rows take no part: they match anywhere.
func alignRows(cur, painted []string) map[int]int {
	trim := func(s string) string { return strings.TrimRight(s, " ") }
	best, bestK := 0, 0
	for k := 0; k < len(painted); k++ {
		n := 0
		for i := 0; i+k < len(painted) && i < len(cur); i++ {
			if w := trim(painted[i+k]); w != "" && w == trim(cur[i]) {
				n++
			}
		}
		if n > best {
			best, bestK = n, k
		}
	}
	rows := map[int]int{}
	for i := 0; i+bestK < len(painted) && i < len(cur); i++ {
		if w := trim(painted[i+bestK]); w != "" && w == trim(cur[i]) {
			rows[i] = i + bestK
		}
	}
	return rows
}

// parsePaneSpec reads a --pane value: comma-separated key=value pairs
// with the keys tty, pid, vt, rows, cols, cursor, and wrap. Temp file
// names from os.tmpname hold no commas.
func parsePaneSpec(s string) (paneSpec, error) {
	p := paneSpec{cursor: true, wrap: true}
	for _, kv := range strings.Split(s, ",") {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			return p, fmt.Errorf("--pane: %q is not key=value", kv)
		}
		var err error
		switch key {
		case "tty":
			p.tty = value
		case "pid":
			p.pid, err = strconv.Atoi(value)
		case "vt":
			p.vtFile = value
		case "text":
			p.textFile = value
		case "current":
			p.currentFile = value
		case "rows":
			p.rows, err = strconv.Atoi(value)
		case "cols":
			p.cols, err = strconv.Atoi(value)
		case "cursor":
			p.cursor, err = strconv.ParseBool(value)
		case "wrap":
			p.wrap, err = strconv.ParseBool(value)
		default:
			return p, fmt.Errorf("--pane: unknown key %q", key)
		}
		if err != nil {
			return p, fmt.Errorf("--pane: %s: %v", key, err)
		}
	}
	return p, nil
}

// load resolves the tty and parses the capture.
func (p *paneSpec) load() error {
	if p.vtFile == "" || p.rows <= 0 {
		return fmt.Errorf("a pane needs vt and rows")
	}
	if p.tty == "" {
		if p.pid <= 0 {
			return fmt.Errorf("a pane needs tty or pid")
		}
		path, err := inpane.TTY(p.pid)
		if err != nil {
			return err
		}
		p.tty = path
	}
	data, err := os.ReadFile(p.vtFile)
	if err != nil {
		return err
	}
	p.scr = screen.Parse(string(data), p.rows)
	if p.textFile != "" {
		if text, err := os.ReadFile(p.textFile); err == nil {
			p.scr.Join(p.cols, string(text))
		}
	}
	return nil
}

func (p *paneSpec) open() (*os.File, error) {
	return os.OpenFile(p.tty, os.O_WRONLY, 0)
}

// paneRun holds the flags of one in-pane subcommand.
type paneRun struct {
	opts     *options
	panes    multiFlag
	single   paneSpec
	typed    *string
	key      *string
	modifier *string
	cwd      *string
	begin    *bool
	recover  *bool
}

func paneCommand(name string, args []string) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	r := paneRun{
		opts:     addOptions(fs),
		typed:    fs.String("typed", "", ""),
		key:      fs.String("key", "", ""),
		modifier: fs.String("modifier", "main", ""),
		cwd:      fs.String("cwd", "", ""),
		begin:    fs.Bool("begin", false, ""),
		recover:  fs.Bool("recover", false, ""),
	}
	fs.Var(&r.panes, "pane", "")
	fs.StringVar(&r.single.tty, "tty", "", "")
	fs.IntVar(&r.single.pid, "pid", 0, "")
	fs.StringVar(&r.single.vtFile, "vt-file", "", "")
	fs.StringVar(&r.single.textFile, "text-file", "", "")
	fs.StringVar(&r.single.currentFile, "current-file", "", "")
	fs.IntVar(&r.single.rows, "rows", 0, "")
	fs.IntVar(&r.single.cols, "cols", 0, "")
	fs.BoolVar(&r.single.cursor, "cursor-visible", true, "")
	fs.BoolVar(&r.single.wrap, "wraparound", true, "")
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return err
	}
	var specs []paneSpec
	if len(r.panes) == 0 {
		if r.single.vtFile == "" || r.single.rows <= 0 {
			return fmt.Errorf("--vt-file and --rows are required")
		}
		if r.single.tty == "" && r.single.pid <= 0 {
			return fmt.Errorf("--tty or --pid is required")
		}
		specs = []paneSpec{r.single}
	}
	for _, s := range r.panes {
		p, err := parsePaneSpec(s)
		if err != nil {
			return err
		}
		specs = append(specs, p)
	}
	for i := range specs {
		if err := specs[i].load(); err != nil {
			return err
		}
		// A first draw, or a restore asked to, picks up a run left on the
		// pane before the hints are found, so they come from the clean
		// screen.
		if (name == "draw" && *r.begin) || (name == "restore" && *r.recover) {
			if err := specs[i].recover(); err != nil {
				return err
			}
		} else {
			specs[i].modesFromRun()
			specs[i].checkMoved()
		}
	}
	switch name {
	case "draw":
		return r.draw(specs)
	case "press":
		return r.press(specs)
	}
	return r.restore(specs)
}

// overlays builds the shared hint set over the panes.
func (r paneRun) overlays(specs []paneSpec) ([]*ui.Overlay, error) {
	scrs := make([]screen.Screen, len(specs))
	for i, p := range specs {
		scrs[i] = p.scr
	}
	return r.opts.overlays(scrs, "top")
}

// paint draws every pane with the hints that continue typed.
func (r paneRun) paint(specs []paneSpec, ovs []*ui.Overlay, typed string, begin bool) error {
	for i, p := range specs {
		tty, err := p.open()
		if err != nil {
			return err
		}
		var b strings.Builder
		if begin {
			// A recovered pane has the cursor the old run saved; put it
			// back before this run saves it again.
			if p.recovered {
				b.WriteString("\x1b8")
			}
			b.WriteString(inpane.Begin)
		}
		ovs[i].Paint(&b, p.rows, p.cols, typed)
		// Back to the cursor Begin saved, so output the pane's program
		// writes while the hints are up lands where it would have anyway.
		b.WriteString("\x1b8")
		_, err = tty.WriteString(b.String())
		tty.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// restoreAll puts every pane back and deletes its capture and run state.
// It carries on past a pane that fails and returns the first error.
func restoreAll(specs []paneSpec) error {
	var first error
	for _, p := range specs {
		if err := p.restore(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// restore repaints the pane as it was and deletes its files. A pane that
// moved on gets back only the rows that still show the frame, and keeps
// its cursor where its program left it.
func (p *paneSpec) restore() error {
	tty, err := p.open()
	if err == nil {
		seq := inpane.Restore(p.scr, p.cursor, p.wrap)
		if p.moved {
			seq = inpane.RestoreRows(p.scr, p.softRows, p.cursor, p.wrap)
		}
		_, err = tty.WriteString(seq)
		tty.Close()
	}
	p.remove()
	return err
}

// anyMoved reports whether a pane of the run wrote output during it.
func anyMoved(specs []paneSpec) bool {
	for _, p := range specs {
		if p.moved {
			return true
		}
	}
	return false
}

// saveAll records every pane's run with the frame just painted.
func saveAll(specs []paneSpec, ovs []*ui.Overlay, typed string) error {
	for i := range specs {
		if err := specs[i].saveRun(ovs[i].Painted(), typed); err != nil {
			return err
		}
	}
	return nil
}

func ttys(specs []paneSpec) []map[string]string {
	out := make([]map[string]string, len(specs))
	for i, p := range specs {
		out[i] = map[string]string{"tty": p.tty}
	}
	return out
}

func (r paneRun) draw(specs []paneSpec) error {
	ovs, err := r.overlays(specs)
	if err != nil {
		return err
	}
	n := 0
	for _, o := range ovs {
		n += len(o.Targets())
	}
	if n > 0 {
		if err := r.paint(specs, ovs, *r.typed, *r.begin); err != nil {
			return err
		}
		if err := saveAll(specs, ovs, *r.typed); err != nil {
			return err
		}
	} else {
		// Nothing is painted, so a pane that still showed an old run is
		// put back here.
		for i := range specs {
			if specs[i].recovered {
				if err := specs[i].restore(); err != nil {
					return err
				}
			}
		}
	}
	return reply(map[string]any{"tty": specs[0].tty, "panes": ttys(specs), "hints": n})
}

func (r paneRun) press(specs []paneSpec) error {
	k, err := keyBytes(*r.key, *r.modifier)
	if err != nil {
		return err
	}
	ovs, err := r.overlays(specs)
	if err != nil {
		return err
	}
	// Output written during the run means the hints no longer line up
	// with the screen, so the key is not a pick: the run ends and the
	// panes get back what still shows the frame.
	if anyMoved(specs) {
		result := map[string]any{"done": true, "changed": true}
		if err := restoreAll(specs); err != nil {
			result["error"] = err.Error()
		}
		return reply(result)
	}
	pane, sel, typed, done := ui.PressSet(ovs, k, *r.typed)
	if !done {
		if typed != *r.typed {
			if err := r.paint(specs, ovs, typed, false); err != nil {
				return err
			}
			if err := saveAll(specs, ovs, typed); err != nil {
				return err
			}
		}
		return reply(map[string]any{"done": false, "typed": typed})
	}
	if sel.Target != nil && *r.opts.flash > 0 {
		if tty, err := specs[pane].open(); err == nil {
			ovs[pane].Flash(tty, sel.Target)
			tty.Close()
			time.Sleep(time.Duration(*r.opts.flash) * time.Millisecond)
		}
	}
	restoreErr := restoreAll(specs)
	result := map[string]any{"done": true}
	if sel.Cancelled || sel.Target == nil {
		if restoreErr != nil {
			result["error"] = restoreErr.Error()
		}
		return reply(result)
	}
	result["pane"] = pane
	// Lua pastes through Rex's write method, which needs no rex CLI on the
	// server's PATH. The OSC 52 copy goes to the pane the key was pressed
	// in; any pane of the window reaches the same clipboard.
	paste := func(text string) error { result["paste"] = text; return nil }
	dir := func() string { return *r.cwd }
	osc, err := specs[0].open()
	if err != nil {
		return err
	}
	defer osc.Close()
	if err := runAction(r.opts.action(sel.Modifier), sel, *r.opts.copyCmd, osc, paste, dir); err != nil {
		result["error"] = err.Error()
	}
	return reply(result)
}

func (r paneRun) restore(specs []paneSpec) error {
	if *r.recover {
		// Only a pane that still showed a leftover run is repainted; the
		// others are as they are, and their new captures go.
		kept := specs[:0]
		for _, p := range specs {
			if p.recovered {
				kept = append(kept, p)
			} else {
				p.remove()
			}
		}
		specs = kept
	}
	if err := restoreAll(specs); err != nil {
		return err
	}
	return reply(map[string]any{"done": true, "panes": len(specs), "changed": anyMoved(specs)})
}

// keyBytes turns a key mode binding, a letter and a modifier, into the
// bytes a terminal sends for it, which is what ui.Overlay reads.
func keyBytes(key, modifier string) ([]byte, error) {
	if len(key) != 1 || key[0] < 'a' || key[0] > 'z' {
		return nil, fmt.Errorf("--key must be one letter a-z, got %q", key)
	}
	c := key[0]
	switch modifier {
	case "main", "":
		return []byte{c}, nil
	case "shift":
		return []byte{c - 'a' + 'A'}, nil
	case "ctrl":
		return []byte{c - 'a' + 1}, nil
	case "alt":
		return []byte{0x1b, c}, nil
	}
	return nil, fmt.Errorf("--modifier must be main, shift, ctrl, or alt, got %q", modifier)
}

func reply(v any) error {
	return json.NewEncoder(os.Stdout).Encode(v)
}
