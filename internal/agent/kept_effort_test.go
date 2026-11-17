package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// keptEffortHome is a sandbox HOME with a provider of thinking models: deep
// (none, low, high: magpie's default high) and wide (low, medium, high:
// magpie's default medium).
func keptEffortHome(t *testing.T, models ...string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("WORKBUDDY_CONFIG_DIR", "")
	keptEffortModels(t, models...)
	return home
}

func keptEffortModels(t *testing.T, models ...string) {
	t.Helper()
	if len(models) == 0 {
		models = []string{"deep", "wide"}
	}
	if err := provider.Save(provider.Provider{ID: "think", Name: "Think", Chat: "https://example.test/v1", Key: "k", Models: models}); err != nil {
		t.Fatal(err)
	}
	if err := catalog.SaveLive("think", "https://example.test/v1", []catalog.Model{
		{ID: "deep", Efforts: []string{"none", "low", "high"}},
		{ID: "wide", Efforts: []string{"low", "medium", "high"}},
		{ID: "fresh", Efforts: []string{"low", "medium", "high"}},
	}); err != nil {
		t.Fatal(err)
	}
}

// 悠悠哥 on Discord: the default thinking effort set on one of magpie's
// models in WorkBuddy was put back to magpie's on every sync. WorkBuddy's
// Settings > Models saves an edited model whole into models.json, with
// reasoning.defaultEffort (empty for its Auto) and the vendor of the
// provider it matches, "Custom" for magpie's (WorkBuddy 5.x's
// inferProviderIdFromLocalCustomModel and the form's save); one set by hand
// keeps vendor magpie. Either way a sync keeps the user's effort while the
// model offers it, a new model gets magpie's default, and a model magpie no
// longer has is taken out, with no second entry of any model.
func TestWorkBuddyKeepsTheUsersDefaultEffort(t *testing.T) {
	home := keptEffortHome(t)
	path := filepath.Join(home, ".workbuddy", "models.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	a := workbuddy(home)
	if err := a.Field("provider").Set(magpieID); err != nil {
		t.Fatal(err)
	}
	read := func() map[string]map[string]any {
		t.Helper()
		var ms []map[string]any
		b, _ := os.ReadFile(path)
		if err := json.Unmarshal(b, &ms); err != nil {
			t.Fatalf("%v\n%s", err, b)
		}
		out := map[string]map[string]any{}
		for _, m := range ms {
			id := m["id"].(string)
			if out[id] != nil {
				t.Fatalf("%s twice: %s", id, b)
			}
			out[id] = m
		}
		return out
	}
	write := func(ms map[string]map[string]any) {
		t.Helper()
		var list []map[string]any
		for _, id := range []string{"think/deep", "think/wide", "think/fresh"} {
			if ms[id] != nil {
				list = append(list, ms[id])
			}
		}
		b, _ := json.Marshal(list)
		os.WriteFile(path, b, 0o600)
	}
	effort := func(m map[string]any) any {
		r, _ := m["reasoning"].(map[string]any)
		if r == nil {
			return "no reasoning"
		}
		return r["defaultEffort"]
	}

	ms := read()
	if effort(ms["think/deep"]) != "high" || effort(ms["think/wide"]) != "medium" {
		t.Fatalf("magpie's defaults: %v %v", effort(ms["think/deep"]), effort(ms["think/wide"]))
	}

	// deep set by hand; wide saved from WorkBuddy's model settings
	ms["think/deep"]["reasoning"].(map[string]any)["defaultEffort"] = "low"
	wide := ms["think/wide"]
	wide["vendor"] = "Custom"
	wide["useCustomProtocol"] = false
	wide["reasoning"] = map[string]any{"defaultEffort": "high", "supportedEfforts": []string{"low", "medium", "high"}, "canDisableThinking": false}
	write(ms)
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	ms = read()
	if len(ms) != 2 || effort(ms["think/deep"]) != "low" || effort(ms["think/wide"]) != "high" || ms["think/wide"]["vendor"] != magpieID {
		t.Fatalf("after sync: %v", ms)
	}

	// WorkBuddy's Auto: no defaultEffort, which stays so
	delete(ms["think/wide"]["reasoning"].(map[string]any), "defaultEffort")
	ms["think/wide"]["vendor"] = "Custom"
	write(ms)
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if ms = read(); effort(ms["think/wide"]) != nil {
		t.Fatalf("Auto: %v", ms["think/wide"])
	}

	// an effort the model no longer offers gives way to magpie's default
	ms["think/deep"]["reasoning"].(map[string]any)["defaultEffort"] = "xhigh"
	write(ms)
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	if ms = read(); effort(ms["think/deep"]) != "high" {
		t.Fatalf("unoffered effort: %v", effort(ms["think/deep"]))
	}

	// a new model gets magpie's default; a model gone is taken out, the
	// one saved from WorkBuddy's settings too
	ms["think/deep"]["reasoning"].(map[string]any)["defaultEffort"] = "low"
	ms["think/wide"]["vendor"] = "Custom"
	write(ms)
	keptEffortModels(t, "deep", "fresh")
	if err := a.Sync(); err != nil {
		t.Fatal(err)
	}
	ms = read()
	if len(ms) != 2 || ms["think/wide"] != nil || effort(ms["think/fresh"]) != "medium" || effort(ms["think/deep"]) != "low" {
		t.Fatalf("after the catalog changed: %v", ms)
	}

	// off takes a model saved from WorkBuddy's settings out too
	ms["think/deep"]["vendor"] = "Custom"
	write(ms)
	if err := a.Field("provider").Set(""); err != nil {
		t.Fatal(err)
	}
	if ms = read(); len(ms) != 0 {
		t.Fatalf("after off: %v", ms)
	}
}
