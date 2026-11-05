// Package hints generates short, prefix-free key sequences, so that typing
// a hint selects one target without a confirming key.
package hints

import "strings"

// DefaultAlphabet is the tmux-fingers QWERTY alphabet without the letters
// c, i, m, q, and n, which tmux-fingers reserves.
const DefaultAlphabet = "asdfwerzxvjkluopghtyb"

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
