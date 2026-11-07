// Package patterns holds the built-in regular expressions, ported from
// tmux-fingers, and compiles the set a run uses into one expression.
package patterns

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Builtin maps a pattern name to its expression. A named group "match"
// limits what is highlighted and copied to that part of the match.
var Builtin = map[string]string{
	"ip":    `\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}`,
	"uuid":  `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`,
	"sha":   `[0-9a-f]{7,128}`,
	"digit": `[0-9]{4,}`,
	"url":   `((https?://|git@|git://|ssh://|ftp://|file:///)[^\s()"']+)`,
	"path":  `(([.\w\-~\$@]+)?(/[.\w\-@]+)+/?)`,
	// A path with spaces, as shells print it inside quotes.
	"quoted-path": `["'](?<match>([.\w\-~\$@ ]+)?(/[.\w\-@ ]+)+/?)["']`,
	// A file with a line and optional column, as compilers and test
	// runners print it: src/app.ts:10:5 or main.go:42.
	"file-line": `(([.\w\-~\$@]+)?(/[.\w\-@]+)+|[\w\-]+\.\w+):\d+(:\d+)?`,
	// Added from tmux-thumbs and WezTerm quick select.
	"markdown-url": `\[[^\]]*\]\((?<match>[^)]+)\)`,
	"color":        `#[0-9a-fA-F]{6}\b`,
	// Full form with four or more groups, or the compressed form with "::".
	// Three groups alone are not enough, so a time like 21:27:05 is not an
	// address.
	"ipv6":              `(([0-9a-fA-F]{1,4}:){3,7}[0-9a-fA-F]{1,4}|([0-9a-fA-F]{1,4}:)*[0-9a-fA-F]{0,4}::([0-9a-fA-F]{1,4}:)*[0-9a-fA-F]{1,4})(%\w+)?`,
	"email":             `[\w.+-]+@[\w-]+(\.[\w-]+)+`,
	"semver":            `\bv?\d+\.\d+\.\d+([-+][\w.]+)?\b`,
	"hex":               `(0x[0-9a-fA-F]+)`,
	"kubernetes-pod":    `[a-z][a-z0-9-]*[a-z0-9]-[bcdfghjklmnpqrstvwxz2456789]{5,10}-[bcdfghjklmnpqrstvwxz2456789]{5}`,
	"git-status":        `(modified|deleted|deleted by us|new file): +(?<match>.+)`,
	"git-status-branch": `Your branch is up to date with '(?<match>.*)'.`,
	"diff":              `(---|\+\+\+) [ab]/(?<match>.*)`,
}

// Names returns the built-in pattern names, sorted.
func Names() []string {
	names := make([]string, 0, len(Builtin))
	for n := range Builtin {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Match is one thing found on a line: the byte range of the whole match
// and of the part to copy.
type Match struct {
	Start, End         int
	CopyStart, CopyEnd int
}

// Set is a compiled group of patterns.
type Set struct {
	exprs  []*regexp.Regexp
	checks []func(text string, start, end int) bool
}

// checks are tests a regular expression cannot express, by pattern name.
// A match that fails its check is dropped.
var checks = map[string]func(text string, start, end int) bool{
	// Rust and C++ scope operators such as Pane::Annotate look like a
	// compressed address. A real one has a digit and does not touch a
	// word character on either side; \b cannot say that before a colon.
	"ipv6": standaloneWithDigit,
	// Seven letters from a to f make a word more often than a hash:
	// "feedbac" in feedback, "defaced", "deadbeef". A real hash stands
	// alone and, in better than 999 of 1000 cases, has a digit.
	"sha": standaloneWithDigit,
}

// standaloneWithDigit holds when the match touches no word character on
// either side and contains a digit.
func standaloneWithDigit(text string, start, end int) bool {
	if start > 0 && isWord(text[start-1]) {
		return false
	}
	if end < len(text) && isWord(text[end]) {
		return false
	}
	return strings.ContainsAny(text[start:end], "0123456789")
}

func isWord(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// Compile builds a Set from built-in names and extra expressions. An empty
// names list selects every built-in pattern.
func Compile(names []string, extra []string) (*Set, error) {
	if len(names) == 0 {
		names = Names()
	}
	var s Set
	for _, n := range names {
		src, ok := Builtin[n]
		if !ok {
			return nil, fmt.Errorf("unknown pattern %q", n)
		}
		re, err := regexp.Compile(src)
		if err != nil {
			return nil, fmt.Errorf("pattern %s: %w", n, err)
		}
		s.exprs = append(s.exprs, re)
		s.checks = append(s.checks, checks[n])
	}
	for i, src := range extra {
		re, err := regexp.Compile(src)
		if err != nil {
			return nil, fmt.Errorf("extra pattern %d: %w", i, err)
		}
		s.exprs = append(s.exprs, re)
		s.checks = append(s.checks, nil)
	}
	return &s, nil
}

// Find returns the matches on a line, left to right, with overlaps
// resolved in favor of the leftmost, then the longest, match.
func (s *Set) Find(text string) []Match {
	var all []Match
	for i, re := range s.exprs {
		mi := re.SubexpIndex("match")
		for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
			if check := s.checks[i]; check != nil && !check(text, loc[0], loc[1]) {
				continue
			}
			m := Match{Start: loc[0], End: loc[1], CopyStart: loc[0], CopyEnd: loc[1]}
			if mi > 0 && loc[2*mi] >= 0 {
				m.CopyStart, m.CopyEnd = loc[2*mi], loc[2*mi+1]
			}
			if m.CopyEnd > m.CopyStart && strings.TrimSpace(text[m.CopyStart:m.CopyEnd]) != "" {
				all = append(all, m)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Start != all[j].Start {
			return all[i].Start < all[j].Start
		}
		return all[i].End > all[j].End
	})
	var out []Match
	last := -1
	for _, m := range all {
		if m.Start < last {
			continue
		}
		out = append(out, m)
		last = m.End
	}
	return out
}
