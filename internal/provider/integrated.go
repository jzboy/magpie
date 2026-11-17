package provider

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/yetone/magpie/internal/plugin"
)

// integratedSubs are the subscriptions 浩哥AI ships through community
// plugins: WorkBuddy (China) and Qoder CN. Their built-in code is retired
// upstream (AGENTS.md); the plugin does sign-in, models, requests and
// usage, and the mover hands the built-in id to it.
var integratedSubs = []string{"workbuddy", QoderCNID}

// integratedState remembers which integrated plugins were installed once,
// so one the user removed later is not added again on the next start.
type integratedState struct {
	Done []string `json:"done"`
}

func integratedStatePath() string { return filepath.Join(plugin.Dir(), "integrated.json") }

// EnsureIntegratedPlugins installs the integrated subscriptions' plugins
// once, at the version each mover needs, through the Plugins page's own
// install path (npm, through Bun). A plugin already listed is left as the
// user has it; a failed install (offline, say) is tried again next start.
// It runs while the gateway serves, never holding a command up.
func EnsureIntegratedPlugins(ctx context.Context) {
	var st integratedState
	if b, err := os.ReadFile(integratedStatePath()); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	for _, id := range integratedSubs {
		mv := movers[id]
		if mv == nil || pluginReady(mv) {
			continue
		}
		done := false
		for _, d := range st.Done {
			if d == id {
				done = true
				break
			}
		}
		if done {
			continue // installed once: a later removal is the user's own
		}
		if _, err := addPlugin(ctx, mv.pkg+"@"+mv.min); err != nil {
			log.Println("integrated plugin", mv.pkg, err)
			continue
		}
		st.Done = append(st.Done, id)
		if b, err := json.Marshal(st); err == nil {
			_ = os.MkdirAll(plugin.Dir(), 0o755)
			_ = os.WriteFile(integratedStatePath(), b, 0o644)
		}
	}
}
