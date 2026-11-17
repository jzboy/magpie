package agent

import (
	"testing"

	"github.com/yetone/magpie/internal/gateway"
)

// ecalpal_ on Discord (magpie 0.1.2689, WSL 3.0.3.0, Ubuntu 22.04.5,
// .wslconfig vmSwitch=WSL_Bridge_Switch, networkingMode=Bridged,
// dhcp=false): in bridged networking the distro's default route is the
// LAN's router, 192.168.9.1, while Windows is at 192.168.9.147/24 on
// vEthernet (WSL_Bridge_Switch). magpie wrote the router into wsl.json and
// into the agent's config, where nothing listens. Windows' address on the
// bridge is the distro's way to it, and its key is the WSL key that
// address takes. NAT and mirrored keep their own values.
func TestWSLBridgedReachesWindowsOnTheBridge(t *testing.T) {
	for _, c := range []struct {
		net, cidr, route, want string
	}{
		{"bridged", "192.168.9.147/24", "192.168.9.1", "192.168.9.147"},
		{"nat", "172.20.0.1/20", "172.20.0.1", "172.20.0.1"},
		{"mirrored", "192.168.9.147/24", "192.168.9.1", ""},
	} {
		fakeWSLNet(t, c.cidr)
		d := parseProbe("Ubuntu-22.04", "home:/home/me\ndir:.codex\nroute:default via "+c.route+" dev eth0 proto static\nnet:"+c.net+"\n")
		if d == nil || d.Gateway != c.want {
			t.Fatalf("%s: %+v", c.net, d)
		}
		d.mirror("")
		want := gateway.URL()
		if c.want != "" {
			want = "http://" + c.want + ":" + gateway.Port()
		}
		if d.base() != want {
			t.Errorf("%s: %s, want %s", c.net, d.base(), want)
		}
	}
}
