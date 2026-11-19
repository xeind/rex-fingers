package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// targetKind is what a picked match names, for :open: and :reveal:.
type targetKind int

const (
	targetNone    targetKind = iota // not a path: a hash, a version, an address
	targetURL                       // a URL, returned as it is
	targetDir                       // a folder that exists
	targetFile                      // a file that exists
	targetMissing                   // a path that does not exist; the nearest folder that does
)

var (
	urlScheme  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://`)
	lineSuffix = regexp.MustCompile(`(:\d+){1,2}$`)
)

// resolveTarget turns the text of a match into what it names, with dir,
// the pane's directory, for a relative path. The open command runs in the
// Rex server's directory, so a path must be absolute by then.
//
// A URL is returned as it is; file:///x is the path /x. Then ~ is the home
// directory and a relative path is under dir. If the path does not exist,
// it is tried without a trailing ".", ",", or ":" and a line and column
// (cli.go:12:3), then also without a leading a/ or b/ (git diff). Text
// that still names nothing is a missing path, with the nearest folder that
// exists, if it contains "/" or starts with "~" or "."; otherwise it is
// not a path. With no dir, a relative path resolves to nothing.
func resolveTarget(text, dir string) (string, targetKind) {
	if rest, ok := strings.CutPrefix(text, "file://"); ok {
		text = rest
	} else if urlScheme.MatchString(text) || strings.HasPrefix(text, "git@") {
		return text, targetURL
	}
	trimmed := lineSuffix.ReplaceAllString(strings.TrimRight(text, ".,:"), "")
	candidates := []string{text, trimmed}
	if rest, ok := strings.CutPrefix(trimmed, "a/"); ok {
		candidates = append(candidates, rest)
	} else if rest, ok := strings.CutPrefix(trimmed, "b/"); ok {
		candidates = append(candidates, rest)
	}
	last := ""
	for _, c := range candidates {
		p := absolute(c, dir)
		if p == "" {
			continue
		}
		last = p
		if info, err := os.Stat(p); err == nil {
			if info.IsDir() {
				return p, targetDir
			}
			return p, targetFile
		}
	}
	pathLike := strings.Contains(text, "/") || strings.HasPrefix(text, "~") || strings.HasPrefix(text, ".")
	if last == "" || !pathLike {
		return "", targetNone
	}
	for p := filepath.Dir(last); ; p = filepath.Dir(p) {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p, targetMissing
		}
		if p == filepath.Dir(p) {
			return "", targetNone
		}
	}
}

// openTarget opens what text names with the system opener. :open: opens
// a URL, a folder, or a file with its default app. :reveal: is the same
// for a URL and a folder; a file is selected in Finder, or on Linux its
// folder is opened, and a missing path opens the nearest folder that
// exists. Text that names nothing to open is an error, so the pick says so
// in the server log.
func openTarget(text, dir string, reveal bool) error {
	path, kind := resolveTarget(text, dir)
	switch {
	case kind == targetNone:
		return fmt.Errorf("%q is not a path or a URL", text)
	case kind == targetMissing && !reveal:
		return fmt.Errorf("%s: no such file or folder", text)
	case kind == targetFile && reveal && runtime.GOOS == "darwin":
		return runOpener("-R", path)
	case kind == targetFile && reveal:
		return runOpener(filepath.Dir(path))
	}
	return runOpener(path)
}

// runOpener runs open, or xdg-open off a Mac, with args. It waits up to
// 2 seconds for a failure: open returns at once, but xdg-open may run the
// app itself and not return. A test sets it to a recorder.
var runOpener = func(args ...string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	cmd := exec.Command(name, args...)
	cmd.Env = commandEnv(os.Environ())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s %s: %v: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
	case <-time.After(2 * time.Second):
	}
	return nil
}

// absolute returns p as an absolute path, with ~ expanded and a relative
// path joined to dir; or "" when it cannot.
func absolute(p, dir string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, strings.TrimPrefix(p[1:], "/"))
	}
	if p == "" || (!filepath.IsAbs(p) && dir == "") {
		return ""
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(dir, p)
}
