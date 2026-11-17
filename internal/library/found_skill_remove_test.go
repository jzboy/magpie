package library

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// wongxy on Discord (资源库技能支持批量删除): the skills the agents have of
// their own go several at once, each as RemoveFoundSkill takes one. One
// that can't be taken out (its agent's folder can't be written) stays, in
// Unremoved, as it was; the others go all the same, with their backups.
// When none can go the change fails and nothing is touched.
func TestRemoveFoundSkills(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a folder made read-only still lets this user (or root) move what's in it")
	}
	h := sandbox(t)
	claude := filepath.Join(h, ".claude/skills")
	skill(t, filepath.Join(claude, "notes"), "notes", "Mine")
	skill(t, filepath.Join(h, ".codex/skills/notes"), "notes", "Mine") // a byte copy
	dev := filepath.Join(h, "src/dev")
	skill(t, dev, "dev", "Work in progress")
	if err := os.Symlink(dev, filepath.Join(claude, "dev")); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(claude, "keep")
	skill(t, keep, "keep", "Not picked")
	// Gemini's skill folder is read-only: it can't be moved, nor its files
	// taken out of it
	gem := filepath.Join(h, ".gemini/skills")
	stuck := filepath.Join(gem, "stuck")
	skill(t, stuck, "stuck", "Held")
	if err := os.Chmod(stuck, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(stuck, 0o755) })

	res, err := RemoveFoundSkills([]string{"notes", "stuck", "dev", "notes"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unremoved) != 1 || res.Unremoved[0].What != "skill:stuck" || res.Unremoved[0].Error == "" {
		t.Fatalf("unremoved: %+v", res.Unremoved)
	}
	for _, p := range []string{filepath.Join(claude, "notes"), filepath.Join(h, ".codex/skills/notes"), filepath.Join(claude, "dev")} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Errorf("%s is still there", p)
		}
	}
	for _, g := range []string{"*/claude/skills/notes/SKILL.md", "*/codex/skills/notes/SKILL.md"} {
		if m, _ := filepath.Glob(filepath.Join(BackupDir(), g)); len(m) != 1 {
			t.Errorf("%s isn't kept with the backups: %v", g, m)
		}
	}
	if read(t, filepath.Join(dev, "SKILL.md")) == "" {
		t.Error("the folder Claude Code linked to is gone")
	}
	if read(t, filepath.Join(gem, "stuck/SKILL.md")) == "" {
		t.Error("the skill that couldn't be removed lost its files")
	}
	found := func() []string {
		v, _ := Read(nil)
		var names []string
		for _, f := range v.FoundSkills {
			names = append(names, f.Name+":"+strings.Join(slices.Concat(f.Agents, f.Copies), ","))
		}
		return names
	}
	if got := found(); !slices.Equal(got, []string{"keep:claude", "stuck:gemini"}) {
		t.Errorf("found after: %v", got)
	}

	// none can go: the change fails with the reason, and nothing goes
	if _, err := RemoveFoundSkills([]string{"stuck", "nothing"}); err == nil {
		t.Error("removing none of them didn't fail")
	}
	if got := found(); !slices.Equal(got, []string{"keep:claude", "stuck:gemini"}) {
		t.Errorf("found after a failed change: %v", got)
	}
	if _, err := RemoveFoundSkills(nil); err == nil {
		t.Error("no names wasn't refused")
	}
	if v, _ := Read(nil); len(v.Skills) != 0 {
		t.Errorf("the library got skills: %+v", v.Skills)
	}
}
