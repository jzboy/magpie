package library

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func symlink(t *testing.T, to, at string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(to, at); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
}

// #227: the user-wide ~/.agents/skills is found, links to it (symlinks,
// junctions on Windows) are the same skill, and bringing one in leaves the
// shared folder as it is.
func TestRealDir(t *testing.T) {
	d := t.TempDir()
	skill(t, filepath.Join(d, "a/real"), "real", "")
	symlink(t, filepath.Join(d, "a/real"), filepath.Join(d, "b/abs"))
	symlink(t, filepath.Join("..", "b", "abs"), filepath.Join(d, "c/rel"))
	symlink(t, filepath.Join(d, "c"), filepath.Join(d, "dir"))
	want, _ := filepath.EvalSymlinks(filepath.Join(d, "a/real"))
	for _, p := range []string{"a/real", "b/abs", "c/rel", "dir/rel", "dir/rel/scripts/.."} {
		if got := realDir(filepath.Join(d, p)); got != want {
			t.Errorf("%s: %s, want %s", p, got, want)
		}
	}
	if got := realDir(filepath.Join(d, "dir/rel/scripts")); got != filepath.Join(want, "scripts") {
		t.Errorf("inside: %s", got)
	}
	// a loop, and what isn't there: the path as it is
	symlink(t, filepath.Join(d, "loop2"), filepath.Join(d, "loop1"))
	symlink(t, filepath.Join(d, "loop1"), filepath.Join(d, "loop2"))
	for _, p := range []string{filepath.Join(d, "loop1"), filepath.Join(d, "gone/x")} {
		if got := realDir(p); got != p {
			t.Errorf("%s: %s", p, got)
		}
	}
	if linked(filepath.Join(d, "a/real")) || !linked(filepath.Join(d, "c/rel")) {
		t.Error("isLink")
	}
	if !within(filepath.Join(d, "a", "x"), filepath.Join(d, "a")) || within(filepath.Join(d, "ab"), filepath.Join(d, "a")) || within(d, filepath.Join(d, "a")) {
		t.Error("within")
	}
}

// A junction (what Windows makes without the right to symlink, and what
// the issue's ZCode and Pi folders were) is a link to the folder it points to.
func TestJunction(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are Windows'")
	}
	d := t.TempDir()
	skill(t, filepath.Join(d, "shared/orchestration"), "orchestration", "")
	j := filepath.Join(d, "zcode", "orchestration")
	os.MkdirAll(filepath.Dir(j), 0o755)
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", j, filepath.Join(d, "shared", "orchestration")).CombinedOutput(); err != nil {
		t.Skipf("mklink /J: %v %s", err, out)
	}
	if !linked(j) {
		t.Error("a junction isn't a link")
	}
	if realDir(j) != realDir(filepath.Join(d, "shared", "orchestration")) {
		t.Errorf("%s != %s", realDir(j), realDir(filepath.Join(d, "shared", "orchestration")))
	}
}
