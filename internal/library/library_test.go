package library

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/pelletier/go-toml/v2"

	"github.com/yetone/magpie/internal/agentenv"
	"github.com/yetone/magpie/internal/testenv"
)

// sandbox is a home with every agent magpie can give the library to, and
// nothing on PATH, so only these are found.
func sandbox(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(h, ".config"))
	t.Setenv("XDG_STATE_HOME", "")
	// Gemini CLI is found by its binary alone
	bin := filepath.Join(h, "bin")
	testenv.Program(t, filepath.Join(bin, "gemini"), "#!/bin/sh\n")
	write(t, filepath.Join(bin, "gemini.exe"), "")
	// the other fixture agents' CLIs beside their folders, as on a machine
	// where they are really installed: with the folder alone each would be
	// gone (CLIMissing, #2177) and no target of the library's. Only these:
	// a stand-in for an agent no folder here keeps would have it detected
	// by its binary alone in every test.
	for _, name := range []string{
		"claude", "codex", "opencode", "mimo", "deveco", "pi", "goose",
		"cursor-agent", "copilot", "crush", "dsh",
	} {
		testenv.Program(t, filepath.Join(bin, name), "#!/bin/sh\n")
		write(t, filepath.Join(bin, name+".exe"), "")
	}
	t.Setenv("PATH", bin)
	// never the machine's global node_modules
	roots := piGlobalRoots
	piGlobalRoots = func() []string { return nil }
	t.Cleanup(func() { piGlobalRoots = roots })
	// a terminal's PATH is the test's, never the developer's login shell's
	up := userPath
	userPath = func() []string { return filepath.SplitList(os.Getenv("PATH")) }
	t.Cleanup(func() { userPath = up })
	for _, k := range agentenv.Vars {
		t.Setenv(k, "")
	}
	t.Setenv("APPDATA", "")
	t.Setenv("LOCALAPPDATA", "")
	// never the developer's own GitHub token, sent to a fake GitHub
	for _, k := range GitHubTokenEnv {
		t.Setenv(k, "")
	}
	for _, f := range []string{
		".claude/settings.json", ".codex/config.toml", ".gemini/settings.json",
		".config/opencode/opencode.json", ".config/mimocode/mimocode.json", ".config/deveco/deveco.jsonc", ".pi/agent/settings.json", ".config/goose/config.yaml",
		".cursor/cli-config.json", ".copilot/settings.json", ".config/crush/crush.json", ".dsh/profiles/web/cordis.patch.yml",
	} {
		write(t, filepath.Join(h, f), "")
	}
	return h
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// holdCLI puts a CLI of the given name in the sandbox's bin, beside the
// agent folder the test wrote: the folder alone is an uninstall that left
// it behind (#2177), and no target of the library's. An empty name is
// nothing, for an agent with no install command, which is never gone.
func holdCLI(t *testing.T, h, bin string) {
	t.Helper()
	if bin == "" {
		return
	}
	testenv.Program(t, filepath.Join(h, "bin", bin), "#!/bin/sh\n")
	write(t, filepath.Join(h, "bin", bin+".exe"), "")
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// ok fails the test on an error or a problem: ok(t)(SaveServer(…)).
func ok(t *testing.T) func(*Result, error) *Result {
	return func(r *Result, err error) *Result {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Problems) > 0 {
			t.Fatalf("problems: %+v", r.Problems)
		}
		return r
	}
}

func ids(ts []*Target) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Agent.ID)
	}
	return out
}

// almaHere makes Alma's data folder, which is what has it detected, and
// returns its USER.md.
func almaHere(t *testing.T, h string) string {
	t.Helper()
	t.Setenv("APPDATA", filepath.Join(h, "AppData", "Roaming"))
	cfg, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(cfg, "alma"), 0o755)
	return filepath.Join(h, ".config", "alma", "USER.md")
}

// almaFront is how Alma reads its owner from USER.md (getOwnerInfo in
// 0.4.164's out/main/index.js): the YAML front matter, from the file's
// first line, unanchored otherwise.
var almaFront = regexp.MustCompile(`^---\n([\s\S]*?)\n---`)

func TestServerKeepsUsersKeys(t *testing.T) {
	h := sandbox(t)
	// 浩哥AI keeps Codex, whose mcp_servers entry has keys of the user's own
	// (startup_timeout_sec) beside magpie's; copilot and gemini are gone.
	ok(t)(SaveServer("", Server{Name: "fs", Transport: "stdio", Command: "npx", Agents: []string{"codex"}}))
	cfg := filepath.Join(h, ".codex/config.toml")
	write(t, cfg, strings.Replace(read(t, cfg), "command = ", "startup_timeout_sec = 30\ncommand = ", 1))

	ok(t)(SaveServer("fs", Server{Name: "fs", Transport: "stdio", Command: "uvx", Agents: []string{"codex"}}))
	if s := read(t, cfg); !strings.Contains(s, "startup_timeout_sec = 30") || !strings.Contains(s, `"uvx"`) {
		t.Errorf("codex:\n%s", s)
	}
}

func TestDelCodexRemovesServerSubtables(t *testing.T) {
	const before = `[user]
note = '''
[mcp_servers.x.fake]
'''

`
	const server = "[mcp_servers.x]\ncommand = \"runner\"\n\n"
	const env = "[mcp_servers.x.env]\nTOKEN = \"value\"\n\n"
	const arrays = `[[mcp_servers.x.env_vars]]
name = "FIRST"
source = "local"

[[mcp_servers.x.env_vars]]
name = "SECOND"
source = "local"

`
	const after = `[[skills.config]]
path = "/keep-the-skill"

[mcp_servers.xy]
command = "same prefix but another server"

[mcp_servers.y]
command = "keep"
`
	for _, self := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, before+server+env+arrays+after)
		if err := delCodex(path, "x", self); err != nil {
			t.Fatal(err)
		}
		want := before + after
		if !self {
			want = before + server + after
		}
		if got := read(t, path); got != want {
			t.Fatalf("self=%v, got:\n%s\nwant:\n%s", self, got, want)
		}
		var document map[string]any
		if err := toml.Unmarshal([]byte(read(t, path)), &document); err != nil {
			t.Fatal(err)
		}
		if self && document["mcp_servers"].(map[string]any)["x"] != nil {
			t.Fatal("removed server was implicitly recreated by a child table")
		}
	}
}

func TestDelCodexParseErrorLeavesFileUntouched(t *testing.T) {
	const input = "[mcp_servers.x]\ncommand = \"runner\"\n\n[mcp_servers.x.env]\nTOKEN = \"value\"\n\n[other]\ninvalid = [\n"
	for _, self := range []bool{false, true} {
		path := filepath.Join(t.TempDir(), "config.toml")
		write(t, path, input)
		if err := delCodex(path, "x", self); err == nil || !strings.HasPrefix(err.Error(), path+": ") {
			t.Fatalf("expected a parse error naming the file, got %v", err)
		}
		if got := read(t, path); got != input {
			t.Fatalf("changed file after a parse error:\n%s", got)
		}
	}
}

func TestPutCodexPreservesChildArrayValues(t *testing.T) {
	const other = "[mcp_servers.other]\ncommand = \"keep\"\n"
	const input = `[mcp_servers.x]
command = "old"

[[mcp_servers.x.env_vars]]
name = "FIRST"
source = "local"

[[mcp_servers.x.env_vars]]
name = "SECOND"
source = "local"

`
	path := filepath.Join(t.TempDir(), "config.toml")
	write(t, path, input+other)
	f := &mcpFile{Path: path, Format: fmtCodex}
	before, err := f.entries()
	if err != nil {
		t.Fatal(err)
	}
	if err := f.put(&Server{Name: "x", Transport: "stdio", Command: "new"}, before["x"]); err != nil {
		t.Fatal(err)
	}
	after, err := f.entries()
	if err != nil {
		t.Fatal(err)
	}
	if after["x"]["command"] != "new" || !reflect.DeepEqual(after["x"]["env_vars"], before["x"]["env_vars"]) {
		t.Fatalf("lost the user's array values while saving: %v", after["x"])
	}
	if !strings.HasSuffix(read(t, path), other) {
		t.Fatal("changed the other server")
	}
	if err := f.del("x"); err != nil {
		t.Fatal(err)
	}
	after, err = f.entries()
	if err != nil || after["x"] != nil || read(t, path) != other {
		t.Fatalf("server was not completely removed: %v, %v\n%s", after, err, read(t, path))
	}
}

// Every key Codex reads for a server (codex-rs/config/src/mcp_types.rs,
// RawMcpServerConfig) is as the user wrote it after the server is taken
// into the library and saved again for Codex and Claude Code: the value and
// its TOML type, a float written as one among them.
func TestCodexServerKeysSurviveTheLibrary(t *testing.T) {
	h := sandbox(t)
	cfg := filepath.Join(h, ".codex/config.toml")
	write(t, cfg, `model = "gpt-5"

[mcp_servers.inner]
command = "node"
args = ["/srv/mcp.js", "--tool-args", "{\"strict\":true}", "a b"]
cwd = "/srv"
env_vars = ["HOME", { name = "TOKEN", source = "local" }]
startup_timeout_sec = 20.5
startup_timeout_ms = 30000
tool_timeout_sec = 120.0
enabled = true
required = false
supports_parallel_tool_calls = true
tool_input_schema_max_bytes = 50000
default_tools_approval_mode = "approve"
enabled_tools = ["search", "fetch"]
disabled_tools = ["drop"]
startup_readiness = "cached"
omit_tools_from = ["code_mode"]
environment_id = "local"
name = "Inner"

[mcp_servers.inner.env]
API_KEY = "k"
PORT = "8080"

[mcp_servers.inner.tools.search]
approval_mode = "approve"
output_token_limit = 4000

[mcp_servers.web]
url = "https://x.example/mcp"
bearer_token = "b"
bearer_token_env_var = "WEB_TOKEN"
http_headers = { "X-Team" = "a" }
env_http_headers = { "X-Key" = "WEB_KEY" }
http_headers_helper = "/bin/helper"
auth = "oauth"
scopes = ["read", "write"]
oauth_resource = "https://x.example"
startup_timeout_sec = 1e1

[mcp_servers.web.oauth]
client_id = "cid"
client_secret = "s"
callback_url = "http://127.0.0.1:8765/cb"
callback_port = 8765
authorization_server_issuer = "https://id.example"
`)
	f := &mcpFile{Path: cfg, Format: fmtCodex}
	before, err := f.entries()
	if err != nil {
		t.Fatal(err)
	}
	ok(t)(ImportServer("inner"))
	ok(t)(ImportServer("web"))
	l, err := load()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"inner", "web"} {
		s := *l.server(name)
		if !s.Remote() {
			s.Args = append(slices.Clone(s.Args), "--more")
		}
		s.Agents = []string{"claude", "codex"}
		ok(t)(SaveServer(name, s))
	}
	after, err := f.entries()
	if err != nil {
		t.Fatal(err)
	}
	for name, m := range before {
		for k, v := range m {
			if k == "args" {
				continue
			}
			if got := after[name][k]; !reflect.DeepEqual(got, v) {
				t.Errorf("%s.%s: %#v (%T) came back %#v (%T)", name, k, v, v, got, got)
			}
		}
	}
	if a := after["inner"]["args"]; !reflect.DeepEqual(a, []any{"/srv/mcp.js", "--tool-args", `{"strict":true}`, "a b", "--more"}) {
		t.Errorf("args: %#v", a)
	}
	if t.Failed() {
		t.Log(read(t, cfg))
	}
}

// What a TOML value magpie keeps reads back as the same value and type.
func TestTOMLValueKeepsTypes(t *testing.T) {
	src := "f = 120.0\ng = 1e21\nh = -0.5\ni = 7\nd = 2026-10-06\nlt = 07:32:00.5\nldt = 2026-10-06T07:32:00\nodt = 2026-10-06T07:32:00.123+08:00\ninf = inf\n"
	var want map[string]any
	if err := toml.Unmarshal([]byte(src), &want); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(want)) {
		b.WriteString(k + " = " + tomlValue(want[k]) + "\n")
	}
	var got map[string]any
	if err := toml.Unmarshal([]byte(b.String()), &got); err != nil {
		t.Fatalf("%v\n%s", err, b.String())
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v\nwant %#v\n%s", got, want, b.String())
	}
}

func TestServerRenameAndAgents(t *testing.T) {
	h := sandbox(t)
	ok(t)(SaveServer("", Server{Name: "a", Transport: "stdio", Command: "x", Agents: []string{"claude", "cursor"}}))
	ok(t)(SaveServer("a", Server{Name: "b", Transport: "stdio", Command: "x", Agents: []string{"claude", "cursor"}}))
	c := read(t, filepath.Join(h, ".claude.json"))
	if strings.Contains(c, `"a"`) || !strings.Contains(c, `"b"`) {
		t.Errorf("rename:\n%s", c)
	}
	ok(t)(ServerAgents("b", []string{"claude"}))
	if s := read(t, filepath.Join(h, ".cursor/mcp.json")); strings.Contains(s, `"b"`) {
		t.Errorf("cursor still has it:\n%s", s)
	}
	if _, err := SaveServer("", Server{Name: "b", Transport: "stdio", Command: "y"}); err == nil {
		t.Error("a second server by the same name")
	}
	if _, err := SaveServer("", Server{Name: "../x", Transport: "stdio", Command: "y"}); err == nil {
		t.Error("a name that isn't one")
	}
}

// Codex and Goose can't reach a server over SSE: they're told so, the
// others get it.
func TestServerSSE(t *testing.T) {
	sandbox(t)
	r, err := SaveServer("", Server{Name: "s", Transport: "sse", URL: "http://localhost:9/sse", Agents: []string{"claude", "codex"}})
	if err != nil {
		t.Fatal(err)
	}
	var who []string
	for _, p := range r.Problems {
		who = append(who, p.Agent)
	}
	slices.Sort(who)
	if !slices.Equal(who, []string{"codex"}) {
		t.Errorf("problems: %+v", r.Problems)
	}
	got, _ := targetByID("claude").MCP.read()
	if got["s"] == nil || got["s"].Transport != "sse" {
		t.Errorf("claude: %+v", got["s"])
	}
}
func TestInstructions(t *testing.T) {
	h := sandbox(t)
	cl := filepath.Join(h, ".claude/CLAUDE.md")
	write(t, cl, "# Mine\n\nBe brief.\n")
	shared := "Use tabs."
	ok(t)(SaveInstructions(InstructionsChange{Shared: &shared, Agents: []string{"claude", "codex"}}))
	s := read(t, cl)
	if !strings.HasPrefix(s, "# Mine\n\nBe brief.\n\n"+blockBegin+"\nUse tabs.\n"+blockEnd) {
		t.Errorf("claude:\n%s", s)
	}
	cx := filepath.Join(h, ".codex/AGENTS.md")
	if s := read(t, cx); s != blockBegin+"\nUse tabs.\n"+blockEnd+"\n" {
		t.Errorf("codex:\n%q", s)
	}
	extra := "Codex only."
	ok(t)(SaveInstructions(InstructionsChange{Extra: map[string]*string{"codex": &extra}}))
	if s := read(t, cx); !strings.Contains(s, "Use tabs.\n\nCodex only.") {
		t.Errorf("codex extra:\n%s", s)
	}

	// edited in the file: left alone until the library changes
	write(t, cl, strings.Replace(read(t, cl), "Use tabs.", "Use spaces.", 1))
	ok(t)(Sync())
	if !strings.Contains(read(t, cl), "Use spaces.") {
		t.Error("an edit in the file was undone")
	}
	iv, _ := ReadInstructions()
	for _, a := range iv.Agents {
		if a.Agent == "claude" && (!a.Edited || a.Own != 3) {
			t.Errorf("claude: %+v", a)
		}
	}
	ok(t)(SaveInstructions(InstructionsChange{Rewrite: []string{"claude"}}))
	if !strings.Contains(read(t, cl), "Use tabs.") {
		t.Error("rewrite didn't")
	}

	// off: magpie's part goes, the user's stays; a file only magpie wrote goes
	ok(t)(SaveInstructions(InstructionsChange{Agents: []string{}}))
	if s := read(t, cl); s != "# Mine\n\nBe brief.\n" {
		t.Errorf("claude after:\n%q", s)
	}
	if _, err := os.Stat(cx); !os.IsNotExist(err) {
		t.Error("codex's AGENTS.md, only magpie's, is still there")
	}
	if es, _ := os.ReadDir(BackupDir()); len(es) == 0 {
		t.Error("nothing was backed up")
	}
}

// TestInstructionsSetLinkedToAgentsFile is #2349: instructions.md linked to
// the file Claude Code reads. The set was read with magpie's part in it, so
// each sync wrote the whole file into its own part again and it doubled.
func TestInstructionsSetLinkedToAgentsFile(t *testing.T) {
	h := sandbox(t)
	cl := filepath.Join(h, ".claude/CLAUDE.md")
	write(t, cl, "# Mine\n\nBe brief.\n")
	set := filepath.Join(Dir(), "instructions.md")
	if err := os.MkdirAll(filepath.Dir(set), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(cl, set); err != nil {
		t.Skip("no symlinks here:", err)
	}
	ok(t)(SaveInstructions(InstructionsChange{Agents: []string{"claude", "codex"}}))
	for range 3 {
		ok(t)(Sync())
	}
	if s := read(t, cl); s != "# Mine\n\nBe brief.\n" {
		t.Errorf("claude's file, the set's own, was written into:\n%q", s)
	}
	cx := filepath.Join(h, ".codex/AGENTS.md")
	if s := read(t, cx); s != blockBegin+"\n# Mine\n\nBe brief.\n"+blockEnd+"\n" {
		t.Errorf("codex:\n%q", s)
	}
	iv, _ := ReadInstructions()
	if iv.Shared != "# Mine\n\nBe brief." {
		t.Errorf("shared: %q", iv.Shared)
	}
}

func tarball(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: "owner-repo-abc123/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.WriteHeader(&tar.Header{Name: "owner-repo-abc123/../evil", Mode: 0o644, Typeflag: tar.TypeReg})
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestSkillsFromGitHub(t *testing.T) {
	h := sandbox(t)
	version := "one"
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		if strings.Contains(r.URL.Path, "missing") {
			http.NotFound(w, r)
			return
		}
		w.Write(tarball(t, map[string]string{
			"README.md":            "hi",
			"skills/pdf/SKILL.md":  "---\nname: pdf\ndescription: PDFs " + version + "\n---\n",
			"skills/pdf/forms.md":  "forms",
			"skills/docx/SKILL.md": "---\nname: docx\ndescription: Word\n---\n",
			"template/SKILL.md":    "---\nname: template\n---\n",
		}))
	}))
	defer srv.Close()
	old := tarballURL
	tarballURL = func(repo, ref string) string { return srv.URL + "/" + repo + "/" + ref }
	defer func() { tarballURL = old }()

	if _, err := ProbeSkills("owner/missing"); err == nil || !strings.Contains(err.Error(), "no repository") {
		t.Errorf("missing: %v", err)
	}
	in := "https://github.com/owner/repo/tree/main/skills"
	p, err := ProbeSkills(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Candidates) != 2 {
		t.Fatalf("candidates: %+v", p.Candidates)
	}
	if asked[len(asked)-1] != "/owner/repo/main" {
		t.Errorf("asked %v", asked)
	}
	ok(t)(InstallSkills(in, []string{"skills/pdf"}, []string{"claude"}))
	if s := read(t, filepath.Join(h, ".claude/skills/pdf/forms.md")); s != "forms" {
		t.Errorf("forms: %q", s)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p.root), "evil")); err == nil {
		t.Error("the tarball wrote outside its folder")
	}
	version = "two"
	ok(t)(UpdateSkill("pdf", false))
	v, _ := Read(nil)
	if v.Skills[0].Description != "PDFs two" || v.Skills[0].Source != in+"/pdf" {
		t.Errorf("after update: %+v", v.Skills[0])
	}
	if !ours(filepath.Join(h, ".claude/skills/pdf"), "pdf") {
		t.Error("claude's link went in the update")
	}
	// installed again, it is had already: not copied a second time
	if r := ok(t)(InstallSkills(in, []string{"skills/pdf"}, nil)); len(r.Installed) != 0 || !slices.Equal(r.Had, []string{"pdf"}) {
		t.Errorf("installed twice: %v, had %v", r.Installed, r.Had)
	}
}

func TestUpdateEverySkill(t *testing.T) {
	h := sandbox(t)
	version, gone := "one", false
	asked := map[string]int{}
	var amu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		amu.Lock()
		asked[r.URL.Path]++
		amu.Unlock()
		if gone && strings.Contains(r.URL.Path, "other") {
			http.NotFound(w, r)
			return
		}
		w.Write(tarball(t, map[string]string{
			"skills/pdf/SKILL.md":  "---\nname: pdf\ndescription: PDFs " + version + "\n---\n",
			"skills/docx/SKILL.md": "---\nname: docx\ndescription: Word " + version + "\n---\n",
			"x/SKILL.md":           "---\nname: x\ndescription: X " + version + "\n---\n",
		}))
	}))
	defer srv.Close()
	old := tarballURL
	tarballURL = func(repo, ref string) string { return srv.URL + "/" + repo + "/" + ref }
	defer func() { tarballURL = old }()

	ok(t)(InstallSkills("owner/repo", []string{"skills/pdf", "skills/docx"}, []string{"claude"}))
	ok(t)(InstallSkills("owner/other", []string{"x"}, []string{"claude"}))
	version, gone = "two", true
	clear(asked)
	res, err := UpdateSkills()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.Updated, []string{"docx", "pdf"}) {
		t.Errorf("updated %v", res.Updated)
	}
	if len(res.Unupdated) != 1 || res.Unupdated[0].What != "skill:x" || !strings.Contains(res.Unupdated[0].Error, "no repository") {
		t.Errorf("unupdated %+v", res.Unupdated)
	}
	if asked["/owner/repo/"] != 1 {
		t.Errorf("owner/repo fetched %d times, not once", asked["/owner/repo/"])
	}
	v, _ := Read(nil)
	for _, s := range v.Skills {
		want := map[string]string{"pdf": "PDFs two", "docx": "Word two", "x": "X one"}[s.Name]
		if s.Description != want {
			t.Errorf("%s: %q, want %q", s.Name, s.Description, want)
		}
	}
	if !ours(filepath.Join(h, ".claude/skills/docx"), "docx") {
		t.Error("claude's link went in the update")
	}
}

func TestGitHubSource(t *testing.T) {
	for in, want := range map[string]Source{
		"owner/repo":                                           {Kind: "github", Repo: "owner/repo"},
		"https://github.com/owner/repo":                        {Kind: "github", Repo: "owner/repo"},
		"github.com/owner/repo.git":                            {Kind: "github", Repo: "owner/repo"},
		"https://github.com/o/r/tree/v1/skills/pdf":            {Kind: "github", Repo: "o/r", Ref: "v1", Path: "skills/pdf"},
		"https://github.com/o/r/blob/main/skills/pdf/SKILL.md": {Kind: "github", Repo: "o/r", Ref: "main", Path: "skills/pdf"},
	} {
		got, ok := githubSource(in)
		if !ok || got != want {
			t.Errorf("%s: %+v %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "not a repo", "https://gitlab.com/o/r", "/abs/path"} {
		if _, ok := githubSource(in); ok {
			t.Errorf("%q taken for GitHub", in)
		}
	}
}

// A skills CLI command pasted whole installs from its source
// (RockyShowlle on X: the npx line a skill's page gives), whatever runs
// it and wherever its flags are; anything else is left as it is.
func TestSkillsCommand(t *testing.T) {
	for in, want := range map[string]string{
		"npx skills add vercel-labs/agent-skills":                     "vercel-labs/agent-skills",
		"  npx -y skills@latest add owner/repo --skill pdf ":          "owner/repo",
		"npx skills add --skill pdf -a claude-code owner/repo":        "owner/repo",
		"bunx skills add https://github.com/o/r/tree/main/skills/pdf": "https://github.com/o/r/tree/main/skills/pdf",
		"pnpm dlx skills add o/r":                                     "o/r",
		"bun x skills add o/r -y":                                     "o/r",
		"owner/repo":                                                  "owner/repo",
		"npx skills find pdf":                                         "npx skills find pdf",
		"npx other add o/r":                                           "npx other add o/r",
		"npx skills add --skill pdf":                                  "npx skills add --skill pdf",
		"~/skills/pdf":                                                "~/skills/pdf",
	} {
		if got := skillsCommand(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	sandbox(t)
	newFakeGitHub(t)
	p, err := ProbeSkills("npx skills add someone/cli --skill pdf")
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != "github" || p.Source != "someone/cli" || len(p.Candidates) != 1 || p.Candidates[0].Name != "pdf" {
		t.Fatalf("probe: %+v", p)
	}
	ok(t)(InstallSkills("npx skills add someone/cli --skill pdf", []string{p.Candidates[0].Path}, nil))
	if l := mustLoad(t); l.skill("pdf") == nil || l.skill("pdf").Source.Repo != "someone/cli" {
		t.Fatalf("installed: %+v", l.skill("pdf"))
	}
}

// Claude Desktop is given only the servers it runs itself: a remote one is
// its Connectors', and the page says so.
func TestClaudeDesktopMCP(t *testing.T) {
	h := sandbox(t)
	if targetByID("claude-desktop") != nil {
		t.Fatal("Claude Desktop found without its folder")
	}
	d, _ := os.UserConfigDir()
	p := filepath.Join(d, "Claude", "claude_desktop_config.json")
	if !strings.HasPrefix(p, h) {
		t.Skip("config dir outside the sandbox: " + p)
	}
	write(t, p, `{"globalShortcut": "Alt+Space", "mcpServers": {"mine": {"command": "x"}}}`)
	tg := targetByID("claude-desktop")
	if tg == nil || tg.MCP == nil || tg.MCP.Path != p || tg.Instructions != "" || tg.Skills != "" {
		t.Fatalf("claude-desktop target: %+v", tg)
	}
	ok(t)(SaveServer("", Server{Name: "fs", Transport: "stdio", Command: "npx", Args: []string{"-y", "fs"}, Agents: []string{"claude-desktop"}}))
	if r, err := SaveServer("", Server{Name: "web", Transport: "http", URL: "https://example.com/mcp", Agents: []string{"claude-desktop"}}); err != nil || len(r.Problems) != 1 || r.Problems[0].Error != "no-remote" {
		t.Fatalf("remote server on Claude Desktop: %+v %v", r, err)
	}
	var doc struct {
		GlobalShortcut string
		MCPServers     map[string]map[string]any
	}
	if err := json.Unmarshal([]byte(read(t, p)), &doc); err != nil {
		t.Fatal(err)
	}
	if fs := doc.MCPServers["fs"]; fs["command"] != "npx" || fs["type"] != nil {
		t.Errorf("fs written as %v", fs)
	}
	if doc.MCPServers["web"] != nil || doc.MCPServers["mine"] == nil || doc.GlobalShortcut != "Alt+Space" {
		t.Errorf("file: %s", read(t, p))
	}
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range v.Agents {
		if a.ID == "claude-desktop" && !a.NoRemote {
			t.Error("claude-desktop not said to take no remote server")
		}
	}
	for _, s := range v.Servers {
		if s.Name == "web" && s.Problems["claude-desktop"] != "no-remote" {
			t.Errorf("web problems: %v", s.Problems)
		}
	}
}

// The servers Codex's app writes into its config itself are told apart:
// its own, not ones to bring in.
func TestFoundServersAppOwned(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".codex/config.toml"), `[mcp_servers.node_repl]
command = 'C:\Users\u\AppData\Local\OpenAI\Codex\runtimes\cua_node\1.0\node_repl.exe'

[mcp_servers.cua_repl]
command = 'C:\Program Files\WindowsApps\OpenAI.Codex_1.0_x64\ChatGPT.exe'
enabled = false

[mcp_servers.computer-use]
command = "./Codex Computer Use.app/Contents/SharedSupport/SkyComputerUseClient.app/Contents/MacOS/SkyComputerUseClient"

[mcp_servers.gh]
command = "gh-mcp"
`)
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	own := map[string]bool{}
	for _, f := range v.FoundServers {
		own[f.Server.Name] = f.Own
	}
	want := map[string]bool{"node_repl": true, "cua_repl": true, "computer-use": true, "gh": false}
	if !maps.Equal(own, want) {
		t.Errorf("own: %v, want %v", own, want)
	}
}

// Another app's server in Codex's config, run from inside that app's
// bundle, is one to bring in, its empty args and inline env table read as
// they are (iomect's #2635, the TOML as the report gave it). Only the
// agent's own app's servers are its own.
func TestFoundServerFromAnotherApp(t *testing.T) {
	h := sandbox(t)
	write(t, filepath.Join(h, ".codex/config.toml"), `[mcp_servers.netcatty-external]
command = "/Applications/Netcatty.app/Contents/Resources/app.asar.unpacked/electron/cli/netcatty-external-mcp"
args = []
env = { NETCATTY_EXTERNAL_MCP_DISCOVERY_FILE = "/Users/****/Library/Application Support/netcatty/external-mcp/discovery.json" }

[mcp_servers.computer-use]
command = "/Applications/Codex.app/Contents/Resources/SkyComputerUseClient"
`)
	v, err := Read(nil)
	if err != nil {
		t.Fatal(err)
	}
	var got *Found
	own := map[string]bool{}
	for i, f := range v.FoundServers {
		own[f.Server.Name] = f.Own
		if f.Server.Name == "netcatty-external" {
			got = &v.FoundServers[i]
		}
	}
	if got == nil || got.Own {
		t.Fatalf("netcatty-external found as %+v; own %v", got, own)
	}
	s := got.Server
	if s.Command != "/Applications/Netcatty.app/Contents/Resources/app.asar.unpacked/electron/cli/netcatty-external-mcp" || len(s.Args) != 0 ||
		s.Env["NETCATTY_EXTERNAL_MCP_DISCOVERY_FILE"] != "/Users/****/Library/Application Support/netcatty/external-mcp/discovery.json" || !slices.Contains(s.Agents, "codex") {
		t.Errorf("netcatty-external read as %+v", s)
	}
	if !own["computer-use"] {
		t.Errorf("the Codex app's own server isn't its own: %v", own)
	}
}

// TestInstructionSets: several sets are kept, one is on, and the agents
// read that one; switching rewrites their files (#106).
func TestInstructionSets(t *testing.T) {
	h := sandbox(t)
	cx := filepath.Join(h, ".codex/AGENTS.md")
	first := "Use tabs."
	ok(t)(SaveInstructions(InstructionsChange{Shared: &first, Agents: []string{"codex"}}))
	work := "Company rules."
	ok(t)(SaveInstructions(InstructionsChange{Create: &InstrSet{ID: "work", Name: "Work"}, Texts: map[string]*string{"work": &work}}))
	if s := read(t, cx); !strings.Contains(s, "Use tabs.") || strings.Contains(s, "Company") {
		t.Errorf("a new set was written before it was switched to:\n%s", s)
	}
	ok(t)(SaveInstructions(InstructionsChange{Activate: "work"}))
	if s := read(t, cx); !strings.Contains(s, "Company rules.") || strings.Contains(s, "tabs") {
		t.Errorf("after switching:\n%s", s)
	}
	iv, _ := ReadInstructions()
	if iv.Shared != work || len(iv.Sets) != 2 || iv.Sets[0].Text != first || !iv.Sets[1].Active || iv.Sets[1].Name != "Work" {
		t.Errorf("view: %+v", iv)
	}
	if _, err := SaveInstructions(InstructionsChange{Remove: "work"}); err == nil {
		t.Error("the set on was removed")
	}
	// the page's text is the set on's; a profile taken now switches back to it
	snap, _ := Snapshot()
	ok(t)(SaveInstructions(InstructionsChange{Activate: "default", Rename: &InstrSet{ID: "default", Name: "Home"}}))
	if s := read(t, cx); !strings.Contains(s, "Use tabs.") {
		t.Errorf("back to the first:\n%s", s)
	}
	ok(t)(Restore(snap))
	if s := read(t, cx); !strings.Contains(s, "Company rules.") {
		t.Errorf("a profile's set wasn't put back:\n%s", s)
	}
	ok(t)(SaveInstructions(InstructionsChange{Activate: "default"}))
	ok(t)(SaveInstructions(InstructionsChange{Remove: "work"}))
	iv, _ = ReadInstructions()
	if len(iv.Sets) != 1 || iv.Sets[0].Name != "Home" || !iv.Sets[0].Active {
		t.Errorf("after removing: %+v", iv.Sets)
	}
	if _, err := os.Stat(setPath("work")); !os.IsNotExist(err) {
		t.Error("a removed set's file stayed")
	}
	if _, err := SaveInstructions(InstructionsChange{Remove: "default"}); err == nil {
		t.Error("the first set was removed")
	}
}

// DeepSeek Harness takes instructions in $DSH_HOME/AGENTS.md, skills in
// $DSH_HOME/skills, and a server as a dsh-mcp-client row an insert of
// magpie's adds to every profile's patch list; the user's own entries,
// rows and !!js values stay as they are.
func TestDsh(t *testing.T) {
	h := sandbox(t)
	d := filepath.Join(h, "dsh-home")
	t.Setenv("DSH_HOME", d)
	web, desk := filepath.Join(d, "profiles/web/cordis.patch.yml"), filepath.Join(d, "profiles/desktop/cordis.patch.yml")
	head := "# Your patch layer for this dsh profile\n"
	user := head + "- id: llm-deepseek\n  config:\n    thinking: enabled\n" +
		"- insert:\n    - id: mcp-engram\n      name: '@deepseek-ai/dsh-mcp-client'\n      config:\n        serverName: engram\n        transport: stdio\n        command: engram\n        args: [mcp]\n        cwd: !!js process.cwd()\n" +
		"    - id: mcp-web\n      name: '@deepseek-ai/dsh-mcp-client'\n      config:\n        serverName: web\n        transport: streamable-http\n        url: http://localhost:3000/mcp\n        toolCallTimeoutMs: 5000\n"
	write(t, web, user)
	write(t, desk, head+"[]\n")
	write(t, filepath.Join(d, "AGENTS.md"), "# Mine\n")

	tg := targetByID("dsh")
	if tg == nil || tg.Instructions != filepath.Join(d, "AGENTS.md") || tg.Skills != filepath.Join(d, "skills") ||
		tg.MCP == nil || tg.MCP.Path != web || !slices.Equal(tg.MCP.Also, []string{desk}) {
		t.Fatalf("dsh target: %+v %+v", tg, tg.MCP)
	}
	got, err := tg.MCP.read()
	if err != nil {
		t.Fatal(err)
	}
	if s := got["engram"]; s == nil || s.Command != "engram" || !slices.Equal(s.Args, []string{"mcp"}) {
		t.Errorf("engram read as %+v", s)
	}
	if s := got["web"]; s == nil || s.Transport != "http" || s.URL != "http://localhost:3000/mcp" {
		t.Errorf("web read as %+v", s)
	}

	shared := "Use tabs."
	ok(t)(SaveInstructions(InstructionsChange{Shared: &shared, Agents: []string{"dsh"}}))
	if s := read(t, filepath.Join(d, "AGENTS.md")); !strings.HasPrefix(s, "# Mine\n\n"+blockBegin+"\nUse tabs.\n"+blockEnd) {
		t.Errorf("AGENTS.md:\n%s", s)
	}

	src := filepath.Join(h, "src/skills")
	skill(t, filepath.Join(src, "pdf"), "pdf", "Read PDFs")
	ok(t)(InstallSkills(src, []string{"pdf"}, []string{"dsh"}))
	if _, err := os.Stat(filepath.Join(d, "skills/pdf/SKILL.md")); err != nil {
		t.Error(err)
	}

	fs := Server{Name: "fs", Transport: "stdio", Command: "npx", Args: []string{"-y", "@mcp/fs"}, Env: map[string]string{"TOKEN": "t\"q"}, Agents: []string{"dsh"}}
	ok(t)(SaveServer("", fs))
	// the user's web, now magpie's: out of the user's insert, its timeout kept
	ok(t)(SaveServer("", Server{Name: "web", Transport: "http", URL: "https://example.com/mcp", Headers: map[string]string{"Authorization": "Bearer x"}, Agents: []string{"dsh"}}))
	// dsh-mcp-client has no SSE
	if r, err := SaveServer("", Server{Name: "ev", Transport: "sse", URL: "https://example.com/sse", Agents: []string{"dsh"}}); err != nil ||
		len(r.Problems) != 1 || r.Problems[0].Error != errNoSSE.Error() {
		t.Errorf("ev: %+v %v", r, err)
	}
	ok(t)(RemoveServer("ev"))
	for _, p := range []string{web, desk} {
		f := &mcpFile{Path: p, Format: fmtDsh}
		got, err := f.read()
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if s := got["fs"]; s == nil || !s.same(&fs) {
			t.Errorf("%s: fs read back as %+v\n%s", p, s, read(t, p))
		}
		if s := got["web"]; s == nil || s.URL != "https://example.com/mcp" || s.Headers["Authorization"] != "Bearer x" {
			t.Errorf("%s: web read back as %+v\n%s", p, s, read(t, p))
		}
		if got["ev"] != nil {
			t.Errorf("%s has ev over SSE", p)
		}
		if s := read(t, p); !strings.HasPrefix(s, head) {
			t.Errorf("%s lost its head:\n%s", p, s)
		}
	}
	s := read(t, web)
	for _, want := range []string{"- id: llm-deepseek\n  config:\n    thinking: enabled\n", "serverName: engram", "cwd: !!js process.cwd()",
		"toolCallTimeoutMs: 5000", "- insert: # magpie\n    - id: magpie-mcp-fs\n      name: \"@deepseek-ai/dsh-mcp-client\""} {
		if !strings.Contains(s, want) {
			t.Errorf("web profile lacks %q:\n%s", want, s)
		}
	}
	if strings.Count(s, "serverName") != 3 {
		t.Errorf("web profile:\n%s", s)
	}
	if es, _ := os.ReadDir(BackupDir()); len(es) == 0 {
		t.Error("nothing was backed up")
	}

	// off again: magpie's inserts go, the user's stay
	ok(t)(RemoveServer("fs"))
	ok(t)(RemoveServer("web"))
	ok(t)(SaveInstructions(InstructionsChange{Agents: []string{}}))
	ok(t)(RemoveSkill("pdf"))
	if s := read(t, desk); s != head+"[]\n" {
		t.Errorf("desktop:\n%q", s)
	}
	s = read(t, web)
	if strings.Contains(s, "# magpie") || !strings.Contains(s, "serverName: engram") || !strings.Contains(s, "cwd: !!js process.cwd()") {
		t.Errorf("web:\n%s", s)
	}
	if s := read(t, filepath.Join(d, "AGENTS.md")); s != "# Mine\n" {
		t.Errorf("AGENTS.md after: %q", s)
	}
	if _, err := os.Lstat(filepath.Join(d, "skills/pdf")); !os.IsNotExist(err) {
		t.Error("dsh still has the skill")
	}
}

// A server by a name dsh-mcp-client can't take isn't written.
func TestDshServerName(t *testing.T) {
	sandbox(t)
	r, err := SaveServer("", Server{Name: "a-name-that-is-longer-than-32-characters", Transport: "stdio", Command: "x", Agents: []string{"dsh"}})
	if err != nil || len(r.Problems) != 1 || r.Problems[0].Agent != "dsh" {
		t.Fatalf("%+v %v", r, err)
	}
}

// A server of the user's the library takes in keeps what dsh works out
// itself (!!js) when magpie writes it again.
func TestDshImportKeepsJS(t *testing.T) {
	h := sandbox(t)
	p := filepath.Join(h, ".dsh/profiles/web/cordis.patch.yml")
	write(t, p, "- insert:\n    - id: mcp-engram\n      name: '@deepseek-ai/dsh-mcp-client'\n      config:\n        serverName: engram\n        transport: stdio\n        command: engram\n        cwd: !!js process.cwd()\n        env:\n          HOME: !!js require('os').homedir()\n")
	v, _ := Read(nil)
	if len(v.FoundServers) != 0 {
		t.Errorf("an env dsh works out read as a server: %+v", v.FoundServers)
	}
	// a variable read whole is the library's reference (#1435)
	write(t, p, "- insert:\n    - id: mcp-engram\n      name: '@deepseek-ai/dsh-mcp-client'\n      config:\n        serverName: engram\n        transport: stdio\n        command: engram\n        cwd: !!js process.cwd()\n        env:\n          HOME: !!js process.env.HOME\n")
	v, _ = Read(nil)
	if len(v.FoundServers) != 1 || v.FoundServers[0].Server.Env["HOME"] != "${HOME}" {
		t.Errorf("found: %+v", v.FoundServers)
	}
	write(t, p, "- insert:\n    - id: mcp-engram\n      name: '@deepseek-ai/dsh-mcp-client'\n      config:\n        serverName: engram\n        transport: stdio\n        command: engram\n        cwd: !!js process.cwd()\n")
	ok(t)(ImportServer("engram"))
	ok(t)(SaveServer("engram", Server{Name: "engram", Transport: "stdio", Command: "engram", Args: []string{"mcp"}, Agents: []string{"dsh"}}))
	s := read(t, p)
	if !strings.Contains(s, "- insert: # magpie") || !strings.Contains(s, `cwd: !!js "process.cwd()"`) || strings.Count(s, "serverName") != 1 {
		t.Errorf("patch list:\n%s", s)
	}
}

// Every skill on for the agents named at once, and off again; an agent not
// named keeps what it has (#443).
func TestEverySkillAgents(t *testing.T) {
	h := sandbox(t)
	src := filepath.Join(h, "src/skills")
	skill(t, filepath.Join(src, "pdf"), "pdf", "Read PDFs")
	skill(t, filepath.Join(src, "xlsx"), "xlsx", "Sheets")
	ok(t)(InstallSkills(src, []string{"pdf"}, []string{"claude"}))
	ok(t)(InstallSkills(src, []string{"xlsx"}, nil))
	has := func(d string) bool { _, err := os.Stat(filepath.Join(h, d, "SKILL.md")); return err == nil }
	ok(t)(EverySkillAgents([]string{"codex"}, true))
	for _, d := range []string{".claude/skills/pdf", ".codex/skills/pdf", ".codex/skills/xlsx"} {
		if !has(d) {
			t.Errorf("%s isn't there", d)
		}
	}
	ok(t)(EverySkillAgents([]string{"codex"}, false))
	for _, d := range []string{".codex/skills/pdf", ".codex/skills/xlsx"} {
		if has(d) {
			t.Errorf("%s is still there", d)
		}
	}
	if !has(".claude/skills/pdf") {
		t.Error("claude, not named, lost pdf")
	}
	v, _ := Read(nil)
	for _, s := range v.Skills {
		if want := map[string][]string{"pdf": {"claude"}, "xlsx": {}}[s.Name]; !slices.Equal(s.Agents, want) {
			t.Errorf("%s: %v, want %v", s.Name, s.Agents, want)
		}
	}
	if _, err := EverySkillAgents(nil, true); err == nil {
		t.Error("no agents named was taken")
	}
}

// A repository's skills on for an agent at once, and off again, from its
// group's chips; a skill not named, and an agent not named, keep theirs (#787).
func TestSomeSkillsAgents(t *testing.T) {
	h := sandbox(t)
	src := filepath.Join(h, "src/skills")
	for _, n := range []string{"pdf", "xlsx", "docx"} {
		skill(t, filepath.Join(src, n), n, n)
	}
	ok(t)(InstallSkills(src, []string{"pdf", "xlsx"}, []string{"gemini"}))
	ok(t)(InstallSkills(src, []string{"docx"}, []string{"claude"}))
	has := func(d string) bool { _, err := os.Stat(filepath.Join(h, d, "SKILL.md")); return err == nil }
	agents := func() map[string][]string {
		v, _ := Read(nil)
		m := map[string][]string{}
		for _, s := range v.Skills {
			m[s.Name] = s.Agents
		}
		return m
	}
	ok(t)(SomeSkillsAgents([]string{"pdf", "xlsx"}, []string{"claude"}, true))
	if m := agents(); !slices.Equal(m["pdf"], []string{"claude", "gemini"}) || !slices.Equal(m["xlsx"], []string{"claude", "gemini"}) || !slices.Equal(m["docx"], []string{"claude"}) {
		t.Errorf("on for claude: %v", m)
	}
	if !has(".claude/skills/pdf") || !has(".claude/skills/xlsx") {
		t.Error("claude wasn't given pdf and xlsx")
	}
	ok(t)(SomeSkillsAgents([]string{"pdf", "xlsx"}, []string{"claude", "gemini"}, false))
	if m := agents(); len(m["pdf"]) != 0 || len(m["xlsx"]) != 0 || !slices.Equal(m["docx"], []string{"claude"}) {
		t.Errorf("off: %v", m)
	}
	if has(".claude/skills/pdf") || has(".gemini/skills/xlsx") || !has(".claude/skills/docx") {
		t.Error("the agents' folders don't match")
	}
	if _, err := SomeSkillsAgents([]string{"nope"}, []string{"claude"}, true); err == nil {
		t.Error("a skill not in the library was taken")
	}
	if _, err := SomeSkillsAgents(nil, []string{"claude"}, true); err == nil {
		t.Error("no skills named was taken as every skill")
	}
}

// Every server on for one agent at once, and off again; the others keep
// theirs, and an agent isn't given a server it can't reach (#475).
func TestEveryServerAgents(t *testing.T) {
	h := sandbox(t)
	ok(t)(SaveServer("", Server{Name: "fs", Transport: "stdio", Command: "fs", Agents: []string{"claude"}}))
	ok(t)(SaveServer("", Server{Name: "web", Transport: "sse", URL: "http://localhost:9/sse", Agents: []string{"claude"}}))
	ok(t)(EveryServerAgents([]string{"codex"}, true))
	agents := func() map[string][]string {
		v, _ := Read(nil)
		m := map[string][]string{}
		for _, s := range v.Servers {
			m[s.Name] = s.Agents
		}
		return m
	}
	if m := agents(); !slices.Equal(m["fs"], []string{"claude", "codex"}) || !slices.Equal(m["web"], []string{"claude"}) {
		t.Errorf("on for codex: %v", m)
	}
	if c := read(t, filepath.Join(h, ".codex/config.toml")); !strings.Contains(c, "[mcp_servers.fs]") || strings.Contains(c, "web") {
		t.Errorf("codex's config:\n%s", c)
	}
	ok(t)(EveryServerAgents([]string{"claude"}, false))
	if m := agents(); !slices.Equal(m["fs"], []string{"codex"}) || len(m["web"]) != 0 {
		t.Errorf("off for claude: %v", m)
	}
	if c := read(t, filepath.Join(h, ".claude.json")); strings.Contains(c, `"fs"`) || strings.Contains(c, `"web"`) {
		t.Errorf("claude still has them:\n%s", c)
	}
	if !strings.Contains(read(t, filepath.Join(h, ".codex/config.toml")), "[mcp_servers.fs]") {
		t.Error("codex, not named, lost fs")
	}
	if _, err := EveryServerAgents(nil, false); err == nil {
		t.Error("no agents named was taken")
	}
}

// Fate on Discord: with the Default set empty, a set made and written and
// Claude Code switched on left CLAUDE.md as it was, since the agents read
// the empty Default. A set added, or one given text, while the set in use
// is empty is the one the agents read; one in use with text stays so.
func TestInstructionSetTakesAnEmptyOnesPlace(t *testing.T) {
	h := sandbox(t)
	md := filepath.Join(h, ".claude/CLAUDE.md")
	write(t, md, "# Mine\n")
	ok(t)(SaveInstructions(InstructionsChange{Create: &InstrSet{ID: "work", Name: "Work"}}))
	work := "Company rules."
	ok(t)(SaveInstructions(InstructionsChange{Texts: map[string]*string{"work": &work}}))
	ok(t)(SaveInstructions(InstructionsChange{Agents: []string{"claude"}}))
	if s := read(t, md); !strings.HasPrefix(s, "# Mine\n\n"+blockBegin+"\nCompany rules.\n"+blockEnd) {
		t.Errorf("CLAUDE.md:\n%s", s)
	}
	// off: magpie's part out, the user's own as it was
	ok(t)(SaveInstructions(InstructionsChange{Agents: []string{}}))
	if s := read(t, md); s != "# Mine\n" {
		t.Errorf("switched off:\n%q", s)
	}
	ok(t)(SaveInstructions(InstructionsChange{Agents: []string{"claude"}}))

	// a set in use with text isn't left for a new one
	ok(t)(SaveInstructions(InstructionsChange{Create: &InstrSet{ID: "home", Name: "Home"}}))
	home := "Home rules."
	ok(t)(SaveInstructions(InstructionsChange{Texts: map[string]*string{"home": &home}}))
	if s := read(t, md); !strings.Contains(s, "Company rules.") || strings.Contains(s, "Home") {
		t.Errorf("a set with text was left for another:\n%s", s)
	}

	// an existing set given text while the set in use is emptied
	empty := ""
	ok(t)(SaveInstructions(InstructionsChange{Texts: map[string]*string{"work": &empty}}))
	ok(t)(SaveInstructions(InstructionsChange{Texts: map[string]*string{"home": &home}}))
	if s := read(t, md); !strings.Contains(s, "Home rules.") {
		t.Errorf("the set given text wasn't read:\n%s", s)
	}
}

// skill writes a skill folder's SKILL.md and a script (restored after the
// removed-agent tests that shared it were cut).
func skill(t *testing.T, dir, name, desc string) {
	write(t, filepath.Join(dir, "SKILL.md"), "---\nname: "+name+"\ndescription: "+desc+"\n---\n\n# "+name+"\n")
	write(t, filepath.Join(dir, "scripts/run.sh"), "echo hi\n")
}
