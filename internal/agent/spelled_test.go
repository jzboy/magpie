package agent

import (
	"testing"
)

// An agent that spells a model provider/model, magpie's as magpie/<ref>:
// one of its own providers named as one of magpie's (its own relay, and
// magpie's relay) is its own model, not magpie's. The row isn't connected
// on it, and Disconnect from magpie puts it back and is done, rather than
// leaving the row connected on the model it went back to (#835).
func TestOwnProviderNamedAsMagpies(t *testing.T) {
	for _, id := range []string{"dsh"} {
		t.Run(id, func(t *testing.T) {
			syncHome(t)
			a, err := Find(id)
			if err != nil {
				t.Fatal(err)
			}
			f := &a.Fields[0]
			if err := f.Set("relay/glm-4.6"); err != nil {
				t.Fatal(err)
			}
			if v := f.Get(); v != "relay/glm-4.6" {
				t.Fatalf("own model reads %q", v)
			}
			if a.Wired() {
				t.Fatal("its own relay/glm-4.6 reads as magpie's")
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
			if v := f.Get(); v != "relay/glm-4.6" {
				t.Errorf("Disconnect left %q, not its own relay/glm-4.6", v)
			}
		})
	}
}
