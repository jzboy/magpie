package library

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/tidwall/jsonc"
)

// hooksOf is the list a hooks file has under hooks.<ev>, as JSON.
func hooksOf(t *testing.T, path, ev string) []string {
	t.Helper()
	var out []string
	for _, e := range gjson.GetBytes(jsonc.ToJSON([]byte(read(t, path))), "hooks."+ev).Array() {
		out = append(out, canonical([]byte(e.Raw)))
	}
	return out
}

// An entry the user changed after magpie wrote it is theirs: taking the
// hook out of the library, or changing it, leaves their entry be. One the
// user had already, the same as the library's, is neither written twice
// nor taken.
func TestHooksTheUsersEditsAreTheirs(t *testing.T) {
	h := sandbox(t)
	settings := filepath.Join(h, ".claude/settings.json")
	write(t, settings, `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"bark"}]}]}}`)
	ok(t)(SaveHook("", Hook{Name: "bark", Event: "turn-end", Command: "bark", Agents: []string{"claude"}}))
	if got := hooksOf(t, settings, "Stop"); len(got) != 1 {
		t.Fatalf("the user's own was written again: %v", got)
	}
	ok(t)(RemoveHooks([]string{"bark"}))
	if got := hooksOf(t, settings, "Stop"); len(got) != 1 {
		t.Fatalf("the user's own was taken out: %v", got)
	}

	ok(t)(SaveHook("", Hook{Name: "fmt", Event: "after-tool", Matcher: "edit", Command: "gofmt -w .", Agents: []string{"claude"}}))
	s := read(t, settings)
	write(t, settings, strings.Replace(s, "gofmt -w .", "gofmt -w ./cmd", 1))
	ok(t)(HookAgents("fmt", []string{}))
	if got := hooksOf(t, settings, "PostToolUse"); len(got) != 1 || !strings.Contains(got[0], "./cmd") {
		t.Errorf("the user's edit of magpie's entry was taken out: %v", got)
	}

	// a changed hook goes out as it was and in as it is
	ok(t)(SaveHook("", Hook{Name: "lint", Event: "before-tool", Matcher: "shell", Command: "lint-a", Agents: []string{"claude"}}))
	ok(t)(SaveHook("lint", Hook{Name: "lint", Event: "before-tool", Matcher: "shell", Command: "lint-b", Agents: []string{"claude"}}))
	if got := hooksOf(t, settings, "PreToolUse"); len(got) != 1 || !strings.Contains(got[0], "lint-b") || !strings.Contains(got[0], `"matcher":"Bash"`) {
		t.Errorf("claude PreToolUse after the change: %v", got)
	}
}

// A file that doesn't parse is unknown, never empty: it is left as it is,
// and the hook said not to be given.
func TestHooksUnreadableFileLeftAlone(t *testing.T) {
	h := sandbox(t)
	codexHooks := filepath.Join(h, ".codex/hooks.json")
	bad := `{"hooks": {"Stop": [ oops`
	write(t, codexHooks, bad)
	res, err := SaveHook("", Hook{Name: "bark", Event: "turn-end", Command: "bark", Agents: []string{"codex"}})
	if err != nil {
		t.Fatal(err)
	}
	if read(t, codexHooks) != bad {
		t.Error("a hooks.json magpie can't read was written over")
	}
	if !slices.ContainsFunc(res.Problems, func(p Problem) bool { return p.Agent == "codex" && p.What == "hook:bark" }) {
		t.Errorf("problems: %+v", res.Problems)
	}
	if a := mustLoad(t).Applied["codex"]; a != nil && len(a.Hooks) != 0 {
		t.Errorf("recorded as written: %+v", a.Hooks)
	}
}

// Codex trusts a hook by its place in hooks.json: a group of magpie's
// taken out from before the user's own moves the user's trust with it,
// and the record of the group taken out goes. The record's key holds
// hooks.json's path, spelled as Codex spells it (codexKey): on Windows the
// path's backslashes make it a literal string (#2275), and a folder with an
// apostrophe as well, an escaped basic one.
func TestHooksCodexTrustMovesWithTheGroups(t *testing.T) {
	for _, c := range []struct{ name, dir string }{
		{"default", ""},
		{"backslash", `co\dex`},
		{"apostrophe", `o'b\rien`},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := sandbox(t)
			dir := filepath.Join(h, ".codex")
			if c.dir != "" {
				dir = filepath.Join(h, c.dir)
				t.Setenv("CODEX_HOME", dir)
			}
			codexHooks := filepath.Join(dir, "hooks.json")
			cfg := filepath.Join(dir, "config.toml")
			write(t, cfg, "model = \"gpt-5\"\n")
			ok(t)(SaveHook("", Hook{Name: "bark", Event: "turn-end", Command: "bark", Agents: []string{"codex"}}))
			// the user adds one of their own after it and trusts both in /hooks
			raw := read(t, codexHooks)
			var doc map[string]map[string][]json.RawMessage
			if err := json.Unmarshal([]byte(raw), &doc); err != nil {
				t.Fatal(err)
			}
			doc["hooks"]["Stop"] = append(doc["hooks"]["Stop"], json.RawMessage(`{"hooks":[{"type":"command","command":"mine"}]}`))
			b, _ := json.Marshal(doc)
			write(t, codexHooks, string(b))
			key := func(g int) string { return codexKey(codexHooks + ":stop:" + string(rune('0'+g)) + ":0") }
			pre := codexKey(codexHooks + ":pre_tool_use:0:0")
			write(t, cfg, read(t, cfg)+"\n[hooks.state]\n\n[hooks.state."+key(0)+"]\ntrusted_hash = \"sha256:aaa\"\n\n[hooks.state."+key(1)+"]\ntrusted_hash = \"sha256:bbb\"\n\n[hooks.state."+pre+"]\nenabled = false\n")

			ok(t)(RemoveHooks([]string{"bark"}))
			if got := hooksOf(t, codexHooks, "Stop"); len(got) != 1 || !strings.Contains(got[0], "mine") {
				t.Fatalf("codex Stop after: %v", got)
			}
			c := read(t, cfg)
			want := "model = \"gpt-5\"\n\n[hooks.state]\n\n[hooks.state." + key(0) + "]\ntrusted_hash = \"sha256:bbb\"\n\n[hooks.state." + pre + "]\nenabled = false\n"
			if c != want {
				t.Errorf("config.toml after:\n%s\nwant:\n%s", c, want)
			}
		})
	}
}

// codexKey is a [hooks.state] key as Codex 0.162 writes it (toml_writer's
// TomlKeyBuilder::as_default, seen from `codex app-server`'s
// config/batchWrite): a basic string when nothing in it needs escaping, a
// literal one when a backslash or a double quote does and it has no
// apostrophe, else a basic string with its escapes.
func codexKey(k string) string {
	if !strings.ContainsAny(k, "\\\"") {
		return `"` + k + `"`
	}
	if !strings.Contains(k, "'") {
		return "'" + k + "'"
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(k) + `"`
}

// Hooks turned off in the agent are said: Claude Code's disableAllHooks.
func TestHooksOffSaid(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".claude/settings.json"), `{"disableAllHooks": true}`)
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	j := slices.IndexFunc(v.HookAgents, func(a HookAgentView) bool { return a.ID == "claude" })
	if j < 0 || !v.HookAgents[j].Off {
		t.Errorf("claude's view: %+v", v.HookAgents)
	}
}

// #2011 (emo172): a hook's name is letters, digits, - and _, so a hook
// says what it is for in a description, in any language. The library
// keeps it through a save, a rename and a change of agents, the page
// reads it, and no agent's file is given it.
func TestHookDescription(t *testing.T) {
	h := sandbox(t)
	ok(t)(SaveHook("", Hook{Name: "auto-format-queue", Description: "  编辑后\n格式化 ", Event: "after-tool", Matcher: "edit", Command: "fmtq", Agents: []string{"claude"}}))
	if d := mustLoad(t).hook("auto-format-queue").Description; d != "编辑后 格式化" {
		t.Errorf("kept %q, want the words on one line", d)
	}
	ok(t)(HookAgents("auto-format-queue", []string{"claude", "codex"}))
	ok(t)(SaveHook("auto-format-queue", Hook{Name: "fmtq", Description: "编辑后格式化", Event: "after-tool", Matcher: "edit", Command: "fmtq", Agents: []string{"claude", "codex"}}))
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Hooks) != 1 || v.Hooks[0].Name != "fmtq" || v.Hooks[0].Description != "编辑后格式化" {
		t.Errorf("the page's hooks: %+v", v.Hooks)
	}
	for _, f := range []string{".claude/settings.json", ".codex/hooks.json"} {
		if s := read(t, filepath.Join(h, f)); strings.Contains(s, "格式化") || strings.Contains(s, "description") {
			t.Errorf("%s was given the description: %s", f, s)
		}
	}
	// a line, not a document
	if _, err := SaveHook("fmtq", Hook{Name: "fmtq", Description: strings.Repeat("格", 201), Event: "after-tool", Command: "fmtq"}); err == nil {
		t.Error("a 201-character description was kept")
	}
	ok(t)(SaveHook("fmtq", Hook{Name: "fmtq", Description: strings.Repeat("格", 200), Event: "after-tool", Command: "fmtq"}))
}

// Claude Code's own events are all of its HOOK_EVENTS (claude 2.1.296),
// those added after magpie's hooks first came among them: one goes to
// settings.json under its own name, and the page offers every one (#2020,
// emo172).
func TestHooksClaudeEventsAll(t *testing.T) {
	h := sandbox(t)
	for _, ev := range []string{"StopFailure", "PostCompact", "FileChanged", "MessageDisplay", "PreModelSwitch", "PostToolBatch"} {
		ok(t)(SaveHook("", Hook{Name: "on-" + ev, Event: "claude:" + ev, Command: "x", Agents: []string{"claude"}}))
		if got := hooksOf(t, filepath.Join(h, ".claude/settings.json"), ev); len(got) != 1 {
			t.Errorf("settings.json %s: %v", ev, got)
		}
	}
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	j := slices.IndexFunc(v.HookAgents, func(a HookAgentView) bool { return a.ID == "claude" })
	if j < 0 || len(v.HookAgents[j].Events) != 33 {
		t.Fatalf("claude's events: %+v", v.HookAgents)
	}
}
