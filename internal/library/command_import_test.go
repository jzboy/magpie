package library

import (
	"os"
	"path/filepath"
	"testing"
)

// piPrompts puts Pi's own prompt files (testdata/pi-prompts, from Pi's
// repository .pi/prompts) in the sandbox's ~/.pi/agent/prompts, and says
// each one's bytes by path.
func piPrompts(t *testing.T, h string) map[string]string {
	t.Helper()
	ents, err := os.ReadDir("testdata/pi-prompts")
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join("testdata/pi-prompts", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		f := filepath.Join(h, ".pi/agent/prompts", e.Name())
		write(t, f, string(b))
		m[f] = string(b)
	}
	return m
}

func probeCommands(t *testing.T) *CommandProbe {
	t.Helper()
	p, err := ProbeCommands()
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func candidate(p *CommandProbe, name string) []CommandCandidate {
	var r []CommandCandidate
	for _, c := range p.Candidates {
		if c.Name == name {
			r = append(r, c)
		}
	}
	return r
}

func candidatePaths(p *CommandProbe) []string {
	var r []string
	for _, c := range p.Candidates {
		r = append(r, c.Path)
	}
	return r
}
