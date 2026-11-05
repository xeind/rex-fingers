package patterns

import "testing"

func copies(t *testing.T, set *Set, line string) []string {
	t.Helper()
	var out []string
	for _, m := range set.Find(line) {
		out = append(out, line[m.CopyStart:m.CopyEnd])
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuiltinPatterns(t *testing.T) {
	set, err := Compile(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		line string
		want []string
	}{
		{"one /usr/local/bin/rex two", []string{"/usr/local/bin/rex"}},
		{"see ~/.config/rex/init.lua:12", []string{"~/.config/rex/init.lua:12"}},
		{"open https://example.com/x?y=1&z=2 now", []string{"https://example.com/x?y=1&z=2"}},
		{"clone git@github.com:xeind/rex-fingers.git", []string{"git@github.com:xeind/rex-fingers.git"}},
		{"commit deadbeefcafe1234 by me", []string{"deadbeefcafe1234"}},
		{"id 3b241101-e2bb-4255-8caf-4136c566a962 ok", []string{"3b241101-e2bb-4255-8caf-4136c566a962"}},
		{"addr 0x7ffee3 and 10.0.0.42 port 8080", []string{"0x7ffee3", "10.0.0.42", "8080"}},
		{"\tmodified:   src/main.go", []string{"src/main.go"}},
		{"--- a/internal/ui/ui.go", []string{"internal/ui/ui.go"}},
		{"pod nginx-deployment-66b6c48dd5-7xb2r running", []string{"nginx-deployment-66b6c48dd5-7xb2r"}},
		{`cp "My Documents/notes.txt" '/tmp/a b/c'`, []string{"My Documents/notes.txt", "/tmp/a b/c"}},
		{"src/app.ts:10:5 - error: cannot find", []string{"src/app.ts:10:5"}},
		{"at 21:27:05 and ::1", []string{"::1"}},
		{"main.go:42: undefined: x", []string{"main.go:42"}},
		{"see [the docs](https://example.com/docs) first", []string{"https://example.com/docs"}},
		{"accent = #c6b18e and #FF9E3B", []string{"#c6b18e", "#FF9E3B"}},
		{"inet6 fe80::1c3a:ff:fe12:3456%en0 and 2001:db8::1", []string{"fe80::1c3a:ff:fe12:3456%en0", "2001:db8::1"}},
		{"Author: Xein <xeindeniel@gmail.com>", []string{"xeindeniel@gmail.com"}},
		{"go: added golang.org/x/term v0.47.0 and v2.1.318-beta.1", []string{"golang.org/x/term", "v0.47.0", "v2.1.318-beta.1"}},
		{"nothing here", nil},
	}
	for _, c := range cases {
		if got := copies(t, set, c.line); !equal(got, c.want) {
			t.Errorf("%q: want %q, got %q", c.line, c.want, got)
		}
	}
}

func TestOverlapsKeepLeftmostLongest(t *testing.T) {
	set, _ := Compile(nil, nil)
	// The sha pattern and the digit pattern both match inside this token;
	// the longer, leftmost match wins and the digits are not a second hit.
	got := copies(t, set, "cafe1234beef")
	if !equal(got, []string{"cafe1234beef"}) {
		t.Fatalf("got %q", got)
	}
}

func TestExtraPatternWithMatchGroup(t *testing.T) {
	set, err := Compile([]string{"url"}, []string{`Error: (?<match>.+)`})
	if err != nil {
		t.Fatal(err)
	}
	got := copies(t, set, "Error: disk full")
	if !equal(got, []string{"disk full"}) {
		t.Fatalf("got %q", got)
	}
	if _, err := Compile([]string{"nope"}, nil); err == nil {
		t.Fatal("unknown pattern name was accepted")
	}
}
