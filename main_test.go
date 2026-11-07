package main

import (
	"strings"
	"testing"
)

func TestCommandEnvAddsUTF8Locale(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		add  bool
	}{
		{"empty, as under the Rex server", []string{"HOME=/x"}, true},
		{"LANG utf-8", []string{"LANG=en_US.UTF-8"}, false},
		{"LANG utf8 lowercase", []string{"LANG=C.utf8"}, false},
		{"LC_CTYPE utf-8", []string{"LC_CTYPE=de_DE.UTF-8"}, false},
		{"LC_ALL overrides a utf-8 LANG", []string{"LANG=en_US.UTF-8", "LC_ALL=C"}, true},
		{"LANG without encoding", []string{"LANG=C"}, true},
	}
	for _, c := range cases {
		got := commandEnv(c.env)
		added := len(got) == len(c.env)+1 && got[len(got)-1] == "LANG=en_US.UTF-8"
		if added != c.add {
			t.Errorf("%s: env %v, got %v", c.name, c.env, got)
		}
		if !c.add && strings.Join(got, "\x00") != strings.Join(c.env, "\x00") {
			t.Errorf("%s: env changed: %v", c.name, got)
		}
	}
}
