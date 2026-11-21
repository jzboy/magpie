// version prints the version this build carries, from the newest haoge-v
// tag: haoge-v1.4 → 1.4.0+haoge. A commit since that tag is not a release
// and says so ("1.4.0-3-g0bcb2cc+haoge"), which update.Released() turns
// down; the "+haoge" keeps the fork off upstream's update feed. The
// Makefile and the CI jobs read this once per build.
//
// Usage: go run ./build/version [git describe output]
package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// tag matches what `git describe --tags --match haoge-v* --long` prints:
// haoge-v1.4-0-gabc1234 at the tag, haoge-v1.4-3-gabc1234 past it.
var tag = regexp.MustCompile(`^haoge-v(\d+\.\d+(?:\.\d+)?)-(\d+)-g([0-9a-f]+)`)

// derive turns git describe's line into the build's version; anything it
// doesn't recognise is a build from source and says so.
func derive(desc string) string {
	m := tag.FindStringSubmatch(strings.TrimSpace(desc))
	if m == nil {
		return "0.0.0-dev+haoge"
	}
	// a tag of two numbers (haoge-v1.4) is a release of x.y.0
	ver := m[1]
	if strings.Count(ver, ".") == 1 {
		ver += ".0"
	}
	if m[2] == "0" {
		return ver + "+haoge"
	}
	return ver + "-" + m[2] + "-g" + m[3] + "+haoge"
}

func main() {
	if len(os.Args) > 1 {
		fmt.Println(derive(os.Args[1]))
		return
	}
	out, err := exec.Command("git", "describe", "--tags", "--match", "haoge-v*", "--long").Output()
	if err != nil {
		fmt.Println(derive(""))
		return
	}
	fmt.Println(derive(string(out)))
}
