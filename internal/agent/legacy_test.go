package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

func TestRenameLegacy(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	write := func(rel, body string) {
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := provider.Save(provider.Provider{ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k", Models: []string{"deepseek-flash"}}); err != nil {
		t.Fatal(err)
	}
	write(".codex/config.toml", "model_reasoning_effort = \"high\"\nmodel = \"deepseek/deepseek-flash\"\nmodel_provider = \"dial\"\nmodel_catalog_json = '"+filepath.Join(home, ".codex", "dial-models.json")+"'\n\n[projects.\"/x\"]\ntrust_level = \"trusted\"\n\n[model_providers.dial]\nname = \"dial\"\nbase_url = \"http://127.0.0.1:3425/v1\"\nwire_api = \"responses\"\nexperimental_bearer_token = \"dial\"\n")
	write(".codex/dial-models.json", "{}")
	write(".claude/settings.json", `{"env": {"ANTHROPIC_BASE_URL": "`+gateway.URL()+`", "ANTHROPIC_AUTH_TOKEN": "dial", "ANTHROPIC_MODEL": "deepseek/deepseek-flash"}, "theme": "dark"}`)

	RenameLegacy()

	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(home, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	cx := read(".codex/config.toml")
	if strings.Contains(cx, "dial") || !strings.Contains(cx, "[model_providers.magpie]") || !strings.Contains(cx, `model_provider = "magpie"`) || !strings.Contains(cx, `[projects."/x"]`) {
		t.Fatalf("codex:\n%s", cx)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "dial-models.json")); err == nil {
		t.Fatal("dial-models.json still there")
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "magpie-models.json")); err != nil {
		t.Fatal("magpie-models.json missing")
	}
	if b := stashLoad(); b["codex.provider"] == "dial" {
		t.Fatal("dial stashed as the provider to go back to")
	}
	cl := read(".claude/settings.json")
	if strings.Contains(cl, `"dial"`) || !strings.Contains(cl, `"ANTHROPIC_AUTH_TOKEN": "magpie"`) || !strings.Contains(cl, `"theme": "dark"`) {
		t.Fatalf("claude:\n%s", cl)
	}

	// a second run leaves every file alone
	before := map[string]string{}
	for _, rel := range []string{".codex/config.toml", ".claude/settings.json"} {
		before[rel] = read(rel)
	}
	RenameLegacy()
	for rel, b := range before {
		if read(rel) != b {
			t.Fatalf("%s changed on the second run", rel)
		}
	}
}

// RenameLegacy reads a thinking level as it is: an effort field, read
// against the models the agent is offered, had the whole catalog built for
// it at every start (~52 MB with a real account's providers), though a
// level is never a dial/ model.
func TestRenameLegacyBuildsNoModels(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	if err := provider.Save(provider.Provider{ID: "deepseek", Name: "DeepSeek", Chat: "https://api.deepseek.com/v1", Key: "k", Models: []string{"deepseek-flash"}}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"env":{"ANTHROPIC_MODEL":"deepseek/deepseek-flash"},"defaultThinkingLevel":"high"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	builtModels.Lock()
	builtModels.m = nil
	builtModels.Unlock()
	RenameLegacy()
	builtModels.Lock()
	defer builtModels.Unlock()
	for k := range builtModels.m {
		t.Errorf("RenameLegacy built %s's models", k)
	}
}
