package agent

import (
	"testing"

	"github.com/yetone/magpie/internal/usage"
)

// Every agent's requests are told by what its own description says.
func TestAgentOf(t *testing.T) {
	cases := map[string]string{
		"claude-cli/2.1.0 (external, cli)":         "claude",
		"codex_cli_rs/0.40.0 (Mac OS 26.0; arm64)": "codex",
		"curl/8.4.0":              "curl",
		"deepseek-harness/0.3.1":  "dsh",
		"dsh":                     "dsh", // a record kept by an id already
		"kimi-code-desktop/1.2.1": "kimi-code-desktop",
		"":                        "other",
	}
	for ua, want := range cases {
		if got := usage.AgentOf(ua); got != want {
			t.Errorf("%q: got %q want %q", ua, got, want)
		}
	}
}

// Every agent's own gateway key (gateway.TokenFor) names it in usage: the
// ones that send no User-Agent of their own are told by that key alone
// (#1605's check of the class).
func TestEveryAgentNamedByItsKey(t *testing.T) {
	for _, a := range Clients() {
		if got := usage.AgentOf(a.ID); got != a.ID {
			t.Errorf("%s's key names %q", a.ID, got)
		}
	}
}
