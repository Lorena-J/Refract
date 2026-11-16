package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ponytojas/Refract/internal/mode"
)

func TestCheckDeployment(t *testing.T) {
	for _, tc := range []struct {
		mode      mode.Mode
		auth      string
		publicURL string
		sandbox   string // "" is the default, none
		wait      time.Duration
		timeout   time.Duration
		want      string // part of the refusal; empty means the start is allowed
	}{
		// A self-hosted install is never refused, whatever the rest says.
		{mode.SelfHost, "none", "", "", time.Hour, time.Hour, ""},
		{mode.SelfHost, "builtin", "not a url", "", time.Hour, time.Hour, ""},
		{mode.SelfHost, "none", "http://r.example.com/app", "", time.Hour, time.Hour, ""},

		{mode.SaaS, "none", "https://r.example.com", "bwrap", 20 * time.Second, 60 * time.Second, "REFRACT_AUTH=none"},
		{mode.SaaS, "builtin", "", "bwrap", 20 * time.Second, 60 * time.Second, "REFRACT_PUBLIC_URL"},
		{mode.SaaS, "builtin", "http://r.example.com", "bwrap", 20 * time.Second, 60 * time.Second, "https address"},
		{mode.SaaS, "builtin", "not a url", "bwrap", 20 * time.Second, 60 * time.Second, "https address"},
		{mode.SaaS, "builtin", "https://", "bwrap", 20 * time.Second, 60 * time.Second, "https address"},
		{mode.SaaS, "builtin", "r.example.com", "bwrap", 20 * time.Second, 60 * time.Second, "https address"},
		{mode.SaaS, "builtin", "https://r.example.com/app", "bwrap", 20 * time.Second, 60 * time.Second, "https address"},
		{mode.SaaS, "builtin", "http://localhost.example.com", "bwrap", 20 * time.Second, 60 * time.Second, "https address"},
		{mode.SaaS, "builtin", "https://r.example.com", "bwrap", 20 * time.Second, 60 * time.Second, ""},
		{mode.SaaS, "builtin", "https://r.example.com/", "bwrap", 20 * time.Second, 60 * time.Second, ""},
		{mode.SaaS, "builtin", "http://localhost:8080", "bwrap", 20 * time.Second, 60 * time.Second, ""},
		{mode.SaaS, "builtin", "http://127.0.0.1:8080", "bwrap", 20 * time.Second, 60 * time.Second, ""},
		{mode.SaaS, "builtin", "http://[::1]:8080", "bwrap", 20 * time.Second, 60 * time.Second, ""},
		{mode.SaaS, "cloudflare", "https://r.example.com", "bwrap", 20 * time.Second, 60 * time.Second, ""},

		// The sandbox is required, and a compile must finish before a proxy cuts the request.
		{mode.SaaS, "builtin", "https://r.example.com", "", 20 * time.Second, 60 * time.Second, "REFRACT_SANDBOX=bwrap"},
		{mode.SaaS, "builtin", "https://r.example.com", "none", 20 * time.Second, 60 * time.Second, "REFRACT_SANDBOX=bwrap"},
		{mode.SaaS, "builtin", "https://r.example.com", "bwrap", 20 * time.Second, 90 * time.Second, "is over 95s: a proxy would cut the request first. Set REFRACT_COMPILE_TIMEOUT=60"},
		{mode.SaaS, "builtin", "https://r.example.com", "bwrap", 20 * time.Second, 75 * time.Second, ""},
		{mode.SaaS, "builtin", "https://r.example.com", "bwrap", 0, 60 * time.Second, "REFRACT_COMPILE_MAX_WAIT=0"},
		{mode.SaaS, "builtin", "https://r.example.com", "bwrap", 40 * time.Second, 60 * time.Second, "Lower REFRACT_COMPILE_MAX_WAIT"},
	} {
		err := checkDeployment(tc.mode, tc.auth, tc.publicURL, tc.sandbox, tc.wait, tc.timeout)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("checkDeployment(%s, %q, %q) = %v; want it allowed", tc.mode, tc.auth, tc.publicURL, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("checkDeployment(%s, %q, %q) = %v; want a refusal naming %q", tc.mode, tc.auth, tc.publicURL, err, tc.want)
		}
	}
}

func TestParseSignup(t *testing.T) {
	for _, tc := range []struct {
		mode mode.Mode
		in   string
		want bool
		bad  bool
	}{
		{mode.SelfHost, "", false, false},
		{mode.SaaS, "", false, false}, // closed until the terms exist (S8)
		{mode.SelfHost, "open", true, false},
		{mode.SaaS, "open", true, false},
		{mode.SelfHost, "closed", false, false},
		{mode.SaaS, "closed", false, false},
		{mode.SelfHost, "yes", false, true},
		{mode.SelfHost, "Open", false, true},
	} {
		got, err := parseSignup(tc.mode, tc.in)
		if (err != nil) != tc.bad || got != tc.want {
			t.Errorf("parseSignup(%s, %q) = %v, %v", tc.mode, tc.in, got, err)
		}
	}
}

func TestCheckSignup(t *testing.T) {
	for _, tc := range []struct {
		mode              mode.Mode
		auth              string
		signup, mailReady bool
		want              string // part of the refusal; empty means allowed
	}{
		{mode.SelfHost, "builtin", true, false, ""},
		{mode.SelfHost, "builtin", true, true, ""},
		{mode.SaaS, "cloudflare", true, false, ""},
		{mode.SaaS, "builtin", false, false, ""}, // the default: closed
		{mode.SaaS, "builtin", false, true, ""},
		{mode.SaaS, "builtin", true, false, "REFRACT_SMTP_URL"},
		{mode.SaaS, "builtin", true, true, "REFRACT_SIGNUP=closed"}, // the S8 gate
	} {
		err := checkSignup(tc.mode, tc.auth, tc.signup, tc.mailReady)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("checkSignup(%s, %s, %v, %v) = %v; want it allowed", tc.mode, tc.auth, tc.signup, tc.mailReady, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("checkSignup(%s, %s, %v, %v) = %v; want a refusal naming %q", tc.mode, tc.auth, tc.signup, tc.mailReady, err, tc.want)
		}
	}
}
