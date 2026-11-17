package library

import (
	"testing"
)

// Only the tool events take a tool, and only session-start takes a start:
// the validation of each stays as it was.
func TestHookMatcherKeptToItsEvent(t *testing.T) {
	sandbox(t)
	for _, c := range []Hook{
		{Event: "turn-end", Matcher: "compact"},
		{Event: "after-tool", Matcher: "compact"},
		{Event: "before-tool", Matcher: "startup"},
		{Event: "session-start", Matcher: "shell"},
		{Event: "session-start", Matcher: "compact|compact"},
		{Event: "session-start", Matcher: "compact|"},
		{Event: "session-start", Matcher: "Compact"},
		{Event: "prompt", Matcher: "resume"},
	} {
		c.Name, c.Command = "x", "x"
		if _, err := SaveHook("", c); err == nil {
			t.Errorf("%s was narrowed to %q", c.Event, c.Matcher)
			ok(t)(RemoveHooks([]string{"x"}))
		}
	}
	for _, m := range []string{"startup", "fork", "clear|fork", "startup|resume|clear|compact|fork"} {
		ok(t)(SaveHook("", Hook{Name: "x", Event: "session-start", Matcher: m, Command: "x"}))
		ok(t)(RemoveHooks([]string{"x"}))
	}
	ok(t)(SaveHook("", Hook{Name: "x", Event: "after-tool", Matcher: "shell", Command: "x"}))
}
