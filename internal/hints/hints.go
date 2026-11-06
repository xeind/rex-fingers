// Package hints generates short, prefix-free key sequences, so that typing
// a hint selects one target without a confirming key.
package hints

import "strings"

// DefaultAlphabet is the tmux-fingers QWERTY alphabet without the letters
// c, i, m, q, and n, which tmux-fingers reserves.
const DefaultAlphabet = "asdfwerzxvjkluopghtyb"

// Layouts maps a keyboard layout name to its hint alphabet, as in
// tmux-fingers. Alphabet strips the reserved letters from them.
var Layouts = map[string]string{
	"qwerty":             "asdfqwerzxcvjklmiuopghtybn",
	"qwerty-homerow":     "asdfjklgh",
	"qwerty-left-hand":   "asdfqwerzcxv",
	"qwerty-right-hand":  "jkluiopmyhn",
	"azerty":             "qsdfazerwxcvjklmuiopghtybn",
	"azerty-homerow":     "qsdfjkmgh",
	"azerty-left-hand":   "qsdfazerwxcv",
	"azerty-right-hand":  "jklmuiophyn",
	"qwertz":             "asdfqweryxcvjkluiopmghtzbn",
	"qwertz-homerow":     "asdfghjkl",
	"qwertz-left-hand":   "asdfqweryxcv",
	"qwertz-right-hand":  "jkluiopmhzn",
	"dvorak":             "aoeuqjkxpyhtnsgcrlmwvzfidb",
	"dvorak-homerow":     "aoeuhtnsid",
	"dvorak-left-hand":   "aoeupqjkyix",
	"dvorak-right-hand":  "htnsgcrlmwvz",
	"colemak":            "arstqwfpzxcvneioluymdhgjbk",
	"colemak-homerow":    "arstneiodh",
	"colemak-left-hand":  "arstqwfpzxcv",
	"colemak-right-hand": "neioluymjhk",
}

// reserved are the letters tmux-fingers never uses as hints: c, m, and q
// are exit or control keys, i and n are easy to misread.
const reserved = "cimqn"

// Alphabet returns the hint alphabet for a layout name, without the
// reserved letters. It returns false for an unknown layout.
func Alphabet(layout string) (string, bool) {
	src, ok := Layouts[layout]
	if !ok {
		return "", false
	}
	var b strings.Builder
	for _, r := range src {
		if !strings.ContainsRune(reserved, r) {
			b.WriteRune(r)
		}
	}
	return b.String(), true
}

// Generate returns n hints over the alphabet. Each hint is one letter when
// n fits in the alphabet. Otherwise some letters become prefixes of
// two-letter hints, as few as needed, so most hints stay one key. The
// result is sorted shortest first.
func Generate(alphabet string, n int) []string {
	letters := strings.Split(alphabet, "")
	k := len(letters)
	if n <= 0 || k == 0 {
		return nil
	}
	if n <= k {
		return letters[:n]
	}
	// p letters become prefixes; each yields k hints, and k-p stay single.
	p := (n - k + (k - 1) - 1) / (k - 1)
	if p > k {
		p = k
	}
	singles := letters[:k-p]
	out := make([]string, 0, n)
	out = append(out, singles...)
	for _, prefix := range letters[k-p:] {
		for _, l := range letters {
			out = append(out, prefix+l)
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}
