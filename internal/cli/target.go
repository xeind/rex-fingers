package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
