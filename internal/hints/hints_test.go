package hints

import (
	"strings"
	"testing"
)

func TestGenerateIsPrefixFree(t *testing.T) {
	for _, n := range []int{1, 5, 21, 22, 40, 100, 400} {
		hs := Generate(DefaultAlphabet, n)
		if len(hs) != n {
			t.Fatalf("n=%d: got %d hints", n, len(hs))
		}
		seen := map[string]bool{}
		for _, h := range hs {
			if seen[h] {
				t.Fatalf("n=%d: duplicate hint %q", n, h)
			}
			seen[h] = true
		}
		for _, a := range hs {
			for _, b := range hs {
				if a != b && strings.HasPrefix(b, a) {
					t.Fatalf("n=%d: %q is a prefix of %q", n, a, b)
				}
			}
		}
	}
}

func TestGenerateKeepsMostHintsShort(t *testing.T) {
	hs := Generate(DefaultAlphabet, 22)
	singles := 0
	for _, h := range hs {
		if len(h) == 1 {
			singles++
		}
	}
	if singles != 20 {
		t.Fatalf("22 hints: want 20 single letters, got %d", singles)
	}
}
