package provider

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/catalog"
)

// Helpers the tests kept from upstream use, which lived in removed
// subscription test files. The Claude branches are skipped: 浩哥AI keeps no
// Claude subscription, only the Claude Code agent.

// modelIDs is a list's ids.
func modelIDs(ms []catalog.Model) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

// threeAccounts signs agent in to three accounts in a sandbox HOME, all on
// in magpie, and answers them as `next` takes them.
func threeAccounts(t *testing.T, agent string) []string {
	t.Helper()
	users := pickUpThree(t, agent)
	for _, u := range users {
		if err := SetLoginOn(agent, u, true); err != nil {
			t.Fatal(err)
		}
	}
	return users
}

// pickUpThree has agent sign itself in to three accounts in turn in a
// sandbox HOME, magpie picking each up, and answers them as threeAccounts
// does, on or off as magpie left them.
func pickUpThree(t *testing.T, agent string) []string {
	t.Helper()
	if agent != "codex" {
		t.Skip("the Claude subscription is removed in 浩哥AI")
	}
	home := signIn(t) // me@example.com
	rememberLogins(true)
	codexSignIn(t, home, "work@example.com", "r-work")
	rememberLogins(true)
	codexSignIn(t, home, "third@example.com", "r-third")
	rememberLogins(true)
	ls := Logins(agent)
	at := slices.IndexFunc(ls, func(l Login) bool { return l.Active })
	if len(ls) != 3 || at < 0 {
		t.Fatalf("not three accounts, one signed in: %+v", ls)
	}
	if at == 2 {
		t.Fatalf("the one signed in is the last, so nothing wraps round: %+v", ls)
	}
	var users []string
	for i := range ls {
		users = append(users, ls[(at+1+i)%3].User)
	}
	return users
}

// usedAt answers each account's used share, a quota reading.
func usedAt(agent string, used map[string]float64) func(context.Context, string) map[string]SubscriptionQuota {
	at := time.Now().Add(time.Hour)
	return func(context.Context, string) map[string]SubscriptionQuota {
		out := map[string]SubscriptionQuota{}
		for u, v := range used {
			out[u] = SubscriptionQuota{Provider: agent, User: u, Windows: []QuotaWindow{
				{Name: "5 hours", Used: v, ResetsAt: &at, Span: 5 * time.Hour},
				{Name: "Fable", Used: 100, Model: "fable", ResetsAt: &at}, // one model's own: never counted
			}}
		}
		return out
	}
}
