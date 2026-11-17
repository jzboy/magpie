package agent

import (
	"testing"
)

// Every agent whose magpie is a provider of its own config (Spelled) holds
// a provider magpie didn't put there as the user's: the row isn't connected
// on it, and Connect takes it on, Disconnect gives it up.
func TestHandWiredMagpieProviderSiblings(t *testing.T) {
	for _, id := range []string{"dsh"} {
		t.Run(id, func(t *testing.T) {
			syncHome(t)
			a, err := Find(id)
			if err != nil {
				t.Fatal(err)
			}
			if a.Spelled == nil {
				t.Fatal("no Spelled")
			}
			// written outside magpie: the field's own Set, nothing recorded
			if err := a.Fields[0].Set("magpie/relay/glm-4.6"); err != nil {
				t.Fatal(err)
			}
			if a.Wired() {
				t.Fatalf("a provider named magpie magpie didn't set reads as connected: %v", a.Values())
			}
			if d := a.Drift(); d != nil {
				t.Errorf("drift: %+v", d)
			}
			if err := a.Connect(); err != nil {
				t.Fatal(err)
			}
			if !a.Wired() {
				t.Fatalf("not connected after Connect: %v", a.Values())
			}
			if err := a.Disconnect(); err != nil {
				t.Fatal(err)
			}
			if a.Wired() {
				t.Fatalf("still connected after Disconnect: %v", a.Values())
			}
		})
	}
}

// setUpInMagpie marks agents as connected in magpie, as Connect leaves them
// (its connect.was stash), for a test that writes a connected agent's files
// itself.
func setUpInMagpie(ids ...string) {
	for _, id := range ids {
		stash(map[string]string{id + ".connect.was": "{}"})
	}
}
