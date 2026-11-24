// Package rexcli calls the rex command-line tool for the few server
// operations the overlay needs: reading a pane, typing into it, and
// giving it focus back.
package rexcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Pane names a terminal block on a Rex server.
type Pane struct {
	Session string
	Block   string
}

// Size is a terminal's size in cells.
type Size struct {
	Columns int `json:"columns"`
	Rows    int `json:"rows"`
}

// binary returns the rex to run. A program the server starts for a
// config action may have no rex on its PATH: the app puts it there only
// for its own terminals. REX_BIN_DIR names the server's own; after that
// come PATH and the places Rex installs it.
func binary() string {
	candidates := []string{}
	if dir := os.Getenv("REX_BIN_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "rex"))
	}
	if path, err := exec.LookPath("rex"); err == nil {
		candidates = append(candidates, path)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, "Library/Application Support/com.superlogical.rex/RexServer/Versions/Current/Rex Server.app/Contents/MacOS/rex"),
			filepath.Join(home, ".local/bin/rex"))
	}
	candidates = append(candidates, "/usr/local/bin/rex", "/usr/bin/rex")
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	return "rex"
}

func run(args ...string) ([]byte, error) {
	cmd := exec.Command(binary(), args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rex %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// Perform asks the app client to perform one of its actions, such as
// pane.copy_mode.enter, on its focused pane, and waits two seconds at
// most. args are name=value pairs.
func Perform(client, action string, args ...string) error {
	_, err := run(append([]string{"-C", client, "--timeout", "2s", "do", action}, args...)...)
	return err
}

func (p Pane) call(method string, args string, result any) error {
	argv := []string{"block", "call", "-s", p.Session, "-b", p.Block, method}
	if args != "" {
		argv = append(argv, args)
	}
	out, err := run(argv...)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(out, result)
}

// Size returns the pane's size.
func (p Pane) Size() (Size, error) {
	var s Size
	err := p.call("size", "", &s)
	return s, err
}

// Capture returns the pane's screen and scrollback as VT sequences, one
// row per CR LF. The CLI reads long output in parts itself.
func (p Pane) Capture() (string, error) {
	out, err := run("capture", "-s", p.Session, "-b", p.Block, "--format=vt")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Write types text into the pane as if the user had typed it.
func (p Pane) Write(text string) error {
	args, _ := json.Marshal(map[string]string{"data": text})
	return p.call("write", string(args), nil)
}

// Pwd returns the working directory the pane's shell last reported, as
// Rex gives it: a file:// URL or a plain path. It is empty when none was
// reported.
func (p Pane) Pwd() string {
	var r struct {
		Pwd string `json:"pwd"`
	}
	if err := p.call("pwd", "", &r); err != nil {
		return ""
	}
	return r.Pwd
}

// Focus gives the pane focus in the session.
func (p Pane) Focus() error {
	_, err := run("focus", "-s", p.Session, p.Block)
	return err
}
