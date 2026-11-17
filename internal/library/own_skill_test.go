package library

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// After Bring in all (StringKe, JasonLeeForOnly on Discord): "Claude Code
// already has a skill of its own called impeccable", and nothing to do
// about it. An agent's folder by the library skill's name that holds the
// very same files — a copy CC Switch made into each agent, or one put back
// by what installed it — gives way to the library's, kept aside; one that
// differs is said, and settled either way from the warning.
func skillAgents(t *testing.T, name string) []string {
	t.Helper()
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range v.Skills {
		if s.Name == name {
			return s.Agents
		}
	}
	t.Fatalf("no skill %s", name)
	return nil
}

func TestSameTree(t *testing.T) {
	d := t.TempDir()
	a, b := filepath.Join(d, "a"), filepath.Join(d, "b")
	skill(t, a, "x", "X")
	skill(t, b, "x", "X")
	write(t, filepath.Join(a, ".DS_Store"), "1")
	write(t, filepath.Join(b, ".git/HEAD"), "ref")
	write(t, filepath.Join(b, marker), "copied")
	if !sameTree(a, b) {
		t.Fatal("the same files aren't the same")
	}
	write(t, filepath.Join(b, "scripts/run.sh"), "echo ho\n") // same size
	if sameTree(a, b) {
		t.Error("other bytes are the same")
	}
	write(t, filepath.Join(b, "scripts/run.sh"), "echo hi\n")
	write(t, filepath.Join(b, "more.md"), "")
	if sameTree(a, b) {
		t.Error("another file is the same")
	}
	if sameTree(a, filepath.Join(d, "gone")) {
		t.Error("a folder that isn't there is the same")
	}
}

// A backup holding a skill folder set aside is its only copy: it is never
// pruned with the others.
func TestPruneKeepsSkillsSetAside(t *testing.T) {
	sandbox(t)
	write(t, filepath.Join(BackupDir(), "2000-01-01_00-00-00.000", "gemini", "skills", "x", "SKILL.md"), "mine")
	for i := range keepBackups + 3 {
		write(t, filepath.Join(BackupDir(), fmt.Sprintf("2026-01-01_00-00-%02d.000", i), "claude", "CLAUDE.md"), "")
	}
	pruneBackups()
	if _, err := os.Stat(filepath.Join(BackupDir(), "2000-01-01_00-00-00.000", "gemini", "skills", "x", "SKILL.md")); err != nil {
		t.Error("the skill set aside was pruned")
	}
	es, _ := os.ReadDir(BackupDir())
	if len(es) != keepBackups+1 {
		t.Errorf("%d backups", len(es))
	}
}
