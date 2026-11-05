// Package rexcli calls the rex command-line tool for the few server
// operations the overlay needs: reading a pane, typing into it, and
// giving it focus back.
package rexcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
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

func run(args ...string) ([]byte, error) {
	cmd := exec.Command("rex", args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rex %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
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

// Focus gives the pane focus in the session.
func (p Pane) Focus() error {
	_, err := run("focus", "-s", p.Session, p.Block)
	return err
}
