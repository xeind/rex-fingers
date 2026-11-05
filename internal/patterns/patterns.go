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
	exprs []*regexp.Regexp
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
	}
	for i, src := range extra {
		re, err := regexp.Compile(src)
		if err != nil {
			return nil, fmt.Errorf("extra pattern %d: %w", i, err)
		}
		s.exprs = append(s.exprs, re)
	}
	return &s, nil
}

// Find returns the matches on a line, left to right, with overlaps
// resolved in favor of the leftmost, then the longest, match.
func (s *Set) Find(text string) []Match {
	var all []Match
	for _, re := range s.exprs {
		mi := re.SubexpIndex("match")
		for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
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
