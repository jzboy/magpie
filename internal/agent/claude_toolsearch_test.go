package agent

import (
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
)

// TestClaudeToolSearchOn: Claude Code before 2.1.304 keeps tool search off
// behind an ANTHROPIC_BASE_URL other than Anthropic's, so every MCP tool's
// definition went on each request: magpie's MCP router's 245 tools were
// 124K tokens of a 341K context (cybercaius on Discord). While Claude Code
// is on magpie, magpie sets ENABLE_TOOL_SEARCH=true; it goes when Claude
// Code is taken off magpie, and a value of the user's, an empty one
// among them, is theirs through a switch, Sync and reset.
func TestClaudeToolSearchOn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := provider.Save(provider.Provider{ID: "v", Name: "V", Chat: "http://127.0.0.1:1/v1", Key: "k", Models: []string{"gpt-x"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "settings.json")
	writeFile(t, path, `{"theme":"dark"}`)
	a := claude(home)
	search := func() (string, bool) { return edit.GetJSON(path, "env."+claudeSearchEnv) }
	set := func(key, v string) {
		t.Helper()
		if err := a.Field(key).Set(v); err != nil {
			t.Fatal(err)
		}
	}
	sync := func() {
		t.Helper()
		if err := a.Sync(); err != nil {
			t.Fatal(err)
		}
	}

	set("model", "v/gpt-x")
	if v, _ := search(); v != "true" {
		t.Fatalf("on magpie: %q, want true", v)
	}
	sync()
	if v, _ := search(); v != "true" {
		t.Fatalf("Sync: %q", v)
	}
	// off magpie, onto Claude Code's own model, and reset: magpie's goes
	set("model", "claude-opus-4-8")
	if v, has := search(); has {
		t.Fatalf("Claude's own model kept magpie's value: %q", v)
	}
	set("model", "v/gpt-x")
	if v, _ := search(); v != "true" {
		t.Fatalf("back on magpie: %q", v)
	}
	set("model", "")
	if v, has := search(); has {
		t.Fatalf("reset kept magpie's value: %q", v)
	}

	// the user's own value is theirs
	for _, mine := range []string{"false", "auto:5", ""} {
		if err := edit.SetJSON(path, edit.KV{Path: "env." + claudeSearchEnv, Value: mine}); err != nil {
			t.Fatal(err)
		}
		set("model", "v/gpt-x")
		sync()
		if v, has := search(); v != mine || !has {
			t.Fatalf("the user's %q became %q (%v)", mine, v, has)
		}
		set("model", "")
		if v, has := search(); v != mine || !has {
			t.Fatalf("reset took the user's %q away: %q (%v)", mine, v, has)
		}
		if err := edit.DelJSON(path, "env."+claudeSearchEnv); err != nil {
			t.Fatal(err)
		}
	}
	// magpie's "true" taken out by the user, then a value of theirs: magpie
	// doesn't write over it
	set("model", "v/gpt-x")
	if err := edit.SetJSON(path, edit.KV{Path: "env." + claudeSearchEnv, Value: "false"}); err != nil {
		t.Fatal(err)
	}
	sync()
	if v, _ := search(); v != "false" {
		t.Fatalf("the user's false after magpie's true became %q", v)
	}
	if v, _ := edit.GetJSON(path, "theme"); v != "dark" {
		t.Fatal("theme lost")
	}
}
