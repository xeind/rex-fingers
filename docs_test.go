package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var optionLine = regexp.MustCompile(`^  (\w+) += (.*?),\s*(--.*)?$`)

// luaDefaults returns the option names and values in init.lua's defaults.
func luaDefaults(t *testing.T) map[string]string {
	src := readFile(t, "lua/rex-fingers/init.lua")
	start := strings.Index(src, "local defaults = {")
	end := strings.Index(src[start:], "\n}\n")
	if start < 0 || end < 0 {
		t.Fatal("no defaults table in init.lua")
	}
	return parseOptionLines(src[start : start+end])
}

func parseOptionLines(block string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(block, "\n") {
		if m := optionLine.FindStringSubmatch(line); m != nil {
			out[m[1]] = strings.TrimSpace(m[2])
		}
	}
	return out
}

// section returns the README text from a heading to the next heading.
func section(t *testing.T, md, heading string) string {
	start := strings.Index(md, "\n## "+heading+"\n")
	if start < 0 {
		t.Fatalf("README has no %q section", heading)
	}
	body := md[start+1:]
	if next := strings.Index(body[3:], "\n## "); next >= 0 {
		body = body[:next+3]
	}
	return body
}

// AGENTS.md rule: the README lists every option with the default of init.lua.
func TestReadmeOptionsMatchLuaDefaults(t *testing.T) {
	lua := luaDefaults(t)
	cfg := section(t, readFile(t, "README.md"), "Configuration")
	block := cfg[strings.Index(cfg, "```lua"):]
	block = block[:strings.Index(block[6:], "```")+6]
	readme := parseOptionLines(block)
	if len(lua) == 0 || len(readme) == 0 {
		t.Fatalf("parsed %d Lua options and %d README options", len(lua), len(readme))
	}
	for name, def := range lua {
		got, ok := readme[name]
		if !ok {
			t.Errorf("README lacks option %s", name)
		} else if got != def {
			t.Errorf("option %s: README default %s, init.lua default %s", name, got, def)
		}
	}
	for name := range readme {
		if _, ok := lua[name]; !ok {
			t.Errorf("README lists %s, which init.lua does not define", name)
		}
	}
}

// Every Lua option has a CLI flag or is a Lua-only setting. A new option
// that reaches neither would do nothing.
func TestLuaOptionsReachTheProgram(t *testing.T) {
	src := readFile(t, "lua/rex-fingers/init.lua")
	luaOnly := map[string]bool{"binary": true, "key": true, "window_key": true, "overlay": true}
	for name := range luaDefaults(t) {
		if luaOnly[name] {
			continue
		}
		if !strings.Contains(src, "opts."+name) && !strings.Contains(src, `pick("`+name+`")`) {
			t.Errorf("init.lua defines %s but reads it nowhere", name)
		}
	}
}

// The block a reader pastes into a coding agent must stay short and name
// the file the agent has to read.
func TestReadmeCopyToAgentBlock(t *testing.T) {
	sec := section(t, readFile(t, "README.md"), "Copy to agent")
	start := strings.Index(sec, "```text\n")
	if start < 0 {
		t.Fatal("no text block in the Copy to agent section")
	}
	body := sec[start+len("```text\n"):]
	body = body[:strings.Index(body, "```")]
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) > 8 {
		t.Errorf("the block has %d lines, want 8 or fewer", len(lines))
	}
	for _, want := range []string{
		"https://raw.githubusercontent.com/xeind/rex-fingers/main/AGENTS.md",
		"Install it for a user",
		"absolute path",
		"rex config check",
		"ask me before",
		"alt+f",
		"alt+shift+f",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the block lacks %q", want)
		}
	}
}

// The section the block names must exist in AGENTS.md.
func TestAgentsHasInstallSection(t *testing.T) {
	if !strings.Contains(readFile(t, "AGENTS.md"), "\n## Install it for a user\n") {
		t.Error("AGENTS.md has no section \"Install it for a user\"")
	}
}

var linkTarget = regexp.MustCompile(`\]\(([^)\s]+)\)|(?:src|srcset)="([^"]+)"`)

var notInAnchor = regexp.MustCompile(`[^\p{L}\p{N} _-]`)

// anchors returns the anchor GitHub gives each heading of a Markdown file.
func anchors(md string) map[string]bool {
	out := map[string]bool{}
	fenced := false
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		text := strings.TrimLeft(line, "#")
		if fenced || text == line || !strings.HasPrefix(text, " ") {
			continue
		}
		text = notInAnchor.ReplaceAllString(strings.ToLower(strings.TrimSpace(text)), "")
		out[strings.ReplaceAll(text, " ", "-")] = true
	}
	return out
}

// Every local link in the README and in docs/ names a file that exists,
// and every #anchor names a heading of that file.
func TestLocalLinksExist(t *testing.T) {
	files, err := filepath.Glob("docs/*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range append([]string{"README.md"}, files...) {
		for _, m := range linkTarget.FindAllStringSubmatch(readFile(t, file), -1) {
			target := m[1] + m[2]
			if strings.Contains(target, "://") {
				continue
			}
			path, anchor, _ := strings.Cut(target, "#")
			if path == "" {
				path = file
			} else {
				path = filepath.Join(filepath.Dir(file), filepath.FromSlash(path))
			}
			if _, err := os.Stat(path); err != nil {
				t.Errorf("%s links to %s: %v", file, target, err)
				continue
			}
			if anchor != "" && !anchors(readFile(t, path))[anchor] {
				t.Errorf("%s links to %s: %s has no such heading", file, target, path)
			}
		}
	}
}
