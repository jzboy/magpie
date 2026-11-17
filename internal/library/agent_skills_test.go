package library

import (
	"os"
	"path/filepath"
	"testing"
)

// A skill kept in ~/.agents/skills reaches ZCode, which reads that folder
// beside its own ~/.zcode/skills: giving it to zcode ticks the chip the
// shared copy already answers, and no second link lands in its own folder
// (#4987).
func TestSharedSkillReachesZCode(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".zcode/cli/config.json"), "")
	write(t, filepath.Join(h, ".zcode/v2/config.json"), "{}")
	shared := filepath.Join(h, ".agents/skills/orca")
	skill(t, shared, "orca", "Orca")
	ok(t)(ImportSkill("orca"))
	own := filepath.Join(h, ".zcode/skills/orca")

	ok(t)(SkillAgents("orca", []string{"kimi", "zcode"}))
	if _, err := os.Lstat(own); !os.IsNotExist(err) {
		t.Error("zcode has a second link to it in its own folder")
	}
	ok(t)(Sync())
	if _, err := os.Lstat(own); !os.IsNotExist(err) {
		t.Error("zcode's own folder got it on the sync too")
	}
}
