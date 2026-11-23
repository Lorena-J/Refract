package main

import "testing"

func TestParseOutbound(t *testing.T) {
	for in, want := range map[string]bool{"": false, "any": false, "public": true} {
		if got, err := parseOutbound(in); err != nil || got != want {
			t.Errorf("parseOutbound(%q) = %v, %v", in, got, err)
		}
	}
	for _, in := range []string{"other", "Public", "private"} {
		if _, err := parseOutbound(in); err == nil {
			t.Errorf("parseOutbound(%q) accepted", in)
		}
	}
}

func TestGitAtLeast237(t *testing.T) {
	for in, want := range map[string]bool{
		"git version 2.37.0":                 true,
		"git version 2.36.9":                 false,
		"git version 2.50.1 (Apple Git-155)": true,
		"git version 3.0.0":                  true,
		"git version 1.9.5":                  false,
		"nonsense":                           false,
	} {
		if got := gitAtLeast237(in); got != want {
			t.Errorf("gitAtLeast237(%q) = %v", in, got)
		}
	}
}
