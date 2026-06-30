package api

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Acme Inc":          "acme-inc",
		"  Web   App  ":     "web-app",
		"My_Cool.Service!":  "my-cool-service",
		"already-a-slug":    "already-a-slug",
		"UPPER":             "upper",
		"123 Numbers 456":   "123-numbers-456",
		"!!!":               "",
		"trailing dashes--": "trailing-dashes",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
