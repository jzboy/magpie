package cli

import "strings"

// lineWith is the first line of out that has s in it, "" when none has.
// It lived in a removed provider test file; the tests kept from upstream
// still use it.
func lineWith(out, s string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}
