package library

import (
	"testing"
)

// A server turned off goes from the agents that run it only once no agent's
// config runs that command: another agent's entry, or the user's own under
// another name, keeps it (#3885).
// launchedBy reads the processes a server's command line starts, from the
// lines of #3885's Windows machine and their macOS and Linux forms.
func TestLaunchedBy(t *testing.T) {
	tavily := launchedBy(&Server{Command: "npx", Args: []string{"-y", "tavily-mcp"}})
	for line, want := range map[string]bool{
		`C:\WINDOWS\system32\cmd.exe /d /s /c "npx -y tavily-mcp"`:                                                   true,
		`"C:\Program Files\nodejs\node.exe" "C:\Program Files\nodejs\node_modules\npm\bin\npx-cli.js" -y tavily-mcp`: true,
		`node /usr/local/lib/node_modules/npm/bin/npx-cli.js -y tavily-mcp`:                                          true,
		`npm exec tavily-mcp`: false, // what npx runs is under the npx it is ended with
		`node C:\Users\u\AppData\Local\npm-cache\_npx\1\node_modules\tavily-mcp\build\index.js`: false,
		`cmd.exe /d /s /c "npx -y tavily-mcp-two"`:                                              false,
		`bunx -y tavily-mcp`:      false,
		`npx -y other tavily-mcp`: false,
		`tavily-mcp`:              false,
	} {
		if got := tavily(line); got != want {
			t.Errorf("%s: %v, want %v", line, got, want)
		}
	}
	bin := launchedBy(&Server{Command: `C:\tools\firecrawl-mcp.cmd`})
	if !bin(`C:\WINDOWS\system32\cmd.exe /d /s /c "C:\tools\firecrawl-mcp.cmd"`) || bin(`firecrawl-mcp-x`) {
		t.Error("a server with no arguments is told by its program")
	}
	// an interpreter with nothing after it is anything it runs
	for _, s := range []*Server{{Command: "node"}, {Command: "npx"}, {URL: "https://x", Command: "npx", Args: []string{"x"}}, {}} {
		if launchKey(s) != "" || launchedBy(s)(`node index.js`) || launchedBy(s)(`node`) {
			t.Errorf("%+v picks", s)
		}
	}
}
