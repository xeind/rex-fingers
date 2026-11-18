package hints

import (
	"strings"
	"testing"
)

// checkHints fails if hs is not n unique, nonempty, prefix-free hints made
// of letters from the alphabet. Two letters is the longest hint, so a list
// holds at most len(alphabet)^2 hints; ui leaves the matches past that
// without a hint.
func checkHints(t *testing.T, name, alphabet string, n int, hs []string) {
	t.Helper()
	want := n
	if k := len(alphabet); want > k*k {
		want = k * k
	}
	if len(hs) != want {
		t.Fatalf("%s n=%d: got %d hints, want %d", name, n, len(hs), want)
	}
	for i, a := range hs {
		if a == "" || strings.Trim(a, alphabet) != "" {
			t.Fatalf("%s n=%d: hint %q is empty or uses letters outside %q", name, n, a, alphabet)
		}
		for j, b := range hs {
			if i != j && strings.HasPrefix(b, a) {
				t.Fatalf("%s n=%d: %q is a prefix of or equals %q", name, n, a, b)
			}
		}
	}
}

func TestEveryLayoutGeneratesPrefixFreeHints(t *testing.T) {
	for name := range Layouts {
		alphabet, ok := Alphabet(name)
		if !ok || len(alphabet) < 2 {
			t.Fatalf("layout %s: alphabet %q", name, alphabet)
		}
		for _, n := range []int{0, 1, 2, len(alphabet), len(alphabet) + 1, len(alphabet) * 2, 300} {
			checkHints(t, name, alphabet, n, Generate(alphabet, n))
		}
	}
}

func TestShortCustomAlphabetsStayPrefixFree(t *testing.T) {
	for _, alphabet := range []string{"ab", "asd", "asdf", "jkl;"} {
		for _, n := range []int{1, 2, 3, 5, 9, 30} {
			checkHints(t, alphabet, alphabet, n, Generate(alphabet, n))
		}
	}
}
