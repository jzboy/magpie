package main

import "testing"

func TestDerive(t *testing.T) {
	for desc, want := range map[string]string{
		"haoge-v1.4-0-gabc1234\n":       "1.4.0+haoge",   // at the tag
		"haoge-v1.4-3-g0bcb2cc":         "1.4.0-3-g0bcb2cc+haoge", // past it
		"haoge-v2.0.10-12-gdeadbee":     "2.0.10-12-gdeadbee+haoge",
		"":                              "0.0.0-dev+haoge", // no tag at all
		"v0.1.4000-3-gabc":              "0.0.0-dev+haoge", // upstream's tag
		"fatal: no names found, cannot describe anything.": "0.0.0-dev+haoge",
	} {
		if got := derive(desc); got != want {
			t.Errorf("derive(%q) = %q, want %q", desc, got, want)
		}
	}
}
