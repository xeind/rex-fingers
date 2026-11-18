package patterns

import "testing"

// FuzzFind runs the full default set over arbitrary text. Every match must
// lie inside the text, the copied part inside the match, and the matches
// must not overlap.
func FuzzFind(f *testing.F) {
	set, err := Compile(append(Default(), "digit"), []string{`(?<match>\bTODO\b)`})
	if err != nil {
		f.Fatal(err)
	}
	for _, s := range []string{
		"",
		"/usr/local/bin/rex ~/.config/rex/init.lua",
		"https://example.com/x?y=1 git@github.com:a/b.git [docs](https://example.com)",
		"src/app.ts:10:5 deadbeefcafe1234 0x7ffee3 #c6b18e 10.0.0.42 fe80::1c3a:ff:fe12:3456%en0",
		"modified:   src/main.go\n--- a/internal/ui/ui.go",
		"21:27:05 feedback Pane::Annotate",
		"'/tmp/a b/c' \"quoted\"",
		"\xff\xfe\x00 invalid",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		last := 0
		for _, m := range set.Find(text) {
			if m.Start < last || m.End > len(text) || m.Start >= m.End {
				t.Fatalf("match [%d,%d) out of order or range after %d in %q", m.Start, m.End, last, text)
			}
			if m.CopyStart < m.Start || m.CopyEnd > m.End || m.CopyStart >= m.CopyEnd {
				t.Fatalf("copy range [%d,%d) outside match [%d,%d) in %q", m.CopyStart, m.CopyEnd, m.Start, m.End, text)
			}
			last = m.End
		}
	})
}
