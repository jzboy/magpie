package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// commandAgents puts every agent that takes slash commands in the
// sandbox, and says where each keeps one called name.
func commandAgents(t *testing.T, h, name string) map[string]string {
	t.Helper()
	write(t, filepath.Join(h, ".qwen/settings.json"), "{}\n")
	write(t, filepath.Join(h, ".factory/settings.json"), "")
	return map[string]string{
		"claude":   filepath.Join(h, ".claude/commands", name+".md"),
		"opencode": filepath.Join(h, ".config/opencode/commands", name+".md"),
		"pi":       filepath.Join(h, ".pi/agent/prompts", name+".md"),
		"gemini":   filepath.Join(h, ".gemini/commands", name+".toml"),
		"qwen":     filepath.Join(h, ".qwen/commands", name+".md"),
		"droid":    filepath.Join(h, ".factory/commands", name+".md"),
	}
}

func commandProblems(res *Result) map[string]string {
	m := map[string]string{}
	for _, p := range res.Problems {
		if strings.HasPrefix(p.What, "command:") {
			m[p.Agent] = p.Error
		}
	}
	return m
}

func TestCommandCheck(t *testing.T) {
	sandbox(t)
	for _, c := range []Command{
		{Name: "", Prompt: "x"},
		{Name: "Review", Prompt: "x"},
		{Name: "-review", Prompt: "x"},
		{Name: "re/view", Prompt: "x"},
		{Name: "re:view", Prompt: "x"},
		{Name: strings.Repeat("a", 65), Prompt: "x"},
		{Name: "review", Prompt: "  \n"},
		{Name: "review", Prompt: "x", Agents: []string{"cursor"}},
		{Name: "review", Prompt: "x", Description: strings.Repeat("字", maxCommandDescription+1)},
	} {
		if _, err := SaveCommand("", c); err == nil {
			t.Errorf("%+v was taken", c)
		}
	}
	for _, n := range []string{"r", "pr_review", "fix-2"} {
		if _, err := SaveCommand("", Command{Name: n, Prompt: "x"}); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
	if _, err := SaveCommand("", Command{Name: "r", Prompt: "y"}); err == nil {
		t.Error("a second command called r")
	}
}

// Carried to another computer (a backup, WebDAV sync), the commands come
// along; a bundle from a magpie before them keeps this computer's.
func TestCommandsCarried(t *testing.T) {
	h := sandbox(t)
	commandAgents(t, h, "review")
	ok(t)(SaveCommand("", Command{Name: "review", Description: "d", Prompt: "Review $ARGUMENTS.", Agents: []string{"claude"}}))
	b, err := Collect()
	if err != nil {
		t.Fatal(err)
	}
	if b.Commands == nil || len(*b.Commands) != 1 || (*b.Commands)[0].Name != "review" {
		t.Fatalf("collected: %+v", b.Commands)
	}
	old := *b
	old.Commands = nil
	ok(t)(RemoveCommands([]string{"review"}))
	ok(t)(SaveCommand("", Command{Name: "here", Prompt: "x", Agents: []string{"claude"}}))
	if _, err := Put(&old); err != nil {
		t.Fatal(err)
	}
	l, _ := load()
	if len(l.Commands) != 1 || l.Commands[0].Name != "here" {
		t.Errorf("an older bundle took this computer's commands: %+v", l.Commands)
	}
	if _, err := Put(b); err != nil {
		t.Fatal(err)
	}
	l, _ = load()
	if len(l.Commands) != 1 || l.Commands[0].Name != "review" {
		t.Errorf("after put: %+v", l.Commands)
	}
	if got := read(t, filepath.Join(h, ".claude/commands/review.md")); got != "---\ndescription: \"d\"\n---\n\nReview $ARGUMENTS.\n" {
		t.Errorf("claude's file after put: %q", got)
	}
	if _, err := os.Stat(filepath.Join(h, ".claude/commands/here.md")); !os.IsNotExist(err) {
		t.Errorf("here.md stayed: %v", err)
	}
}
