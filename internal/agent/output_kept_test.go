package agent

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/yetone/magpie/internal/catalog"
	"github.com/yetone/magpie/internal/provider"
)

// outputHome is syncHome with a model models.dev says replies 384000
// tokens, as it says of deepseek-v4.1-flash, served by the relay.
func outputHome(t *testing.T) string {
	t.Helper()
	home := syncHome(t)
	listOutput(t, 384000)
	if err := provider.Save(provider.Provider{ID: "relay", Name: "Relay", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"deepseek-v4.1-flash"}}); err != nil {
		t.Fatal(err)
	}
	return home
}

// listOutput is models.dev saying the model replies n tokens.
func listOutput(t *testing.T, n int) {
	t.Helper()
	os.WriteFile(catalog.CachePath(), []byte(`{"deepseek":{"models":{"deepseek-v4.1-flash":{"id":"deepseek-v4.1-flash","name":"DeepSeek V4.1 Flash","limit":{"context":1000000,"output":`+strconv.Itoa(n)+`}}}}}`), 0o644)
	catalog.Reset()
}

// Flandern1211 on X: a max output edited by hand in dsh or WorkBuddy was
// written back by the next sync. For each file magpie writes the models
// into: magpie's own value first, then one typed by hand stays through
// syncs and models.dev changing, a Max output set in magpie is written over
// it, and taken away, magpie's own follows models.dev again.
func checkOutputKept(t *testing.T, name string, wantFirst int, read func() string, typeIn func(n int), sync func()) {
	t.Helper()
	const model = "relay/deepseek-v4.1-flash"
	want := func(n int, why string) {
		t.Helper()
		if got := read(); got != strconv.Itoa(n) {
			t.Fatalf("%s: %s: output = %q, want %d", name, why, got, n)
		}
	}
	sync()
	want(wantFirst, "first sync")
	typeIn(64000)
	sync()
	want(64000, "typed by hand, synced")
	listOutput(t, 300000)
	sync()
	want(64000, "typed by hand, models.dev changed")
	if err := provider.SetModelOutput(model, 100000); err != nil {
		t.Fatal(err)
	}
	sync()
	want(100000, "Max output set in magpie")
	if err := provider.SetModelOutput(model, 0); err != nil {
		t.Fatal(err)
	}
	sync()
	if got := read(); got == "100000" || got == "64000" {
		t.Fatalf("%s: with no Max output set, output = %s, want models.dev's", name, got)
	}
}

func TestDshKeepsAMaxTokensTypedByHand(t *testing.T) {
	home := outputHome(t)
	web := filepath.Join(home, ".dsh", "profiles", "web", "cordis.patch.yml")
	os.MkdirAll(filepath.Dir(web), 0o755)
	os.WriteFile(web, []byte("# Your patch layer for this dsh profile.\n"), 0o644)
	a := dsh(home)
	if err := a.Field("model").Set("magpie/relay/deepseek-v4.1-flash"); err != nil {
		t.Fatal(err)
	}
	path := web
	re := regexp.MustCompile(`(?m)^(\s+maxTokens: )(\d+)$`)
	checkOutputKept(t, "dsh", 384000,
		func() string {
			m := re.FindStringSubmatch(readFile(path))
			if m == nil {
				return "none:\n" + readFile(path)
			}
			return m[2]
		},
		func(n int) {
			writeFile(t, path, re.ReplaceAllString(readFile(path), "${1}"+strconv.Itoa(n)))
		},
		func() {
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
		})
}

func TestWorkBuddyKeepsAMaxOutputTypedByHand(t *testing.T) {
	home := outputHome(t)
	path := filepath.Join(home, "models.json")
	re := regexp.MustCompile(`("maxOutputTokens": )(\d+)`)
	checkOutputKept(t, "workbuddy", 128000,
		func() string {
			m := re.FindStringSubmatch(readFile(path))
			if m == nil {
				return "none:\n" + readFile(path)
			}
			return m[2]
		},
		func(n int) {
			writeFile(t, path, re.ReplaceAllString(readFile(path), "${1}"+strconv.Itoa(n)))
		},
		func() {
			if err := workbuddyWrite(path, true); err != nil {
				t.Fatal(err)
			}
		})
}
