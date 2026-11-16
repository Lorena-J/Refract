package main

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/ponytojas/Refract/internal/mode"
)

// checkDeployment refuses configurations that are unsafe for the hosted
// service. A self-hosted install is never refused here. authMode is the
// REFRACT_AUTH value after its default is applied.
func checkDeployment(m mode.Mode, authMode, publicURL, sandbox string, maxWait, compileTimeout time.Duration) error {
	if m != mode.SaaS {
		return nil
	}
	if authMode == "none" {
		return errors.New("REFRACT_MODE=saas can't run with REFRACT_AUTH=none: every visitor would be the same account. Set REFRACT_AUTH=builtin")
	}
	if publicURL == "" {
		return errors.New("REFRACT_MODE=saas needs REFRACT_PUBLIC_URL, the address users open Refract at (e.g. https://refract.example.com)")
	}
	u, err := url.Parse(publicURL)
	if err != nil || u.Host == "" || (u.Path != "" && u.Path != "/") || !secureOrigin(u) {
		return fmt.Errorf("REFRACT_PUBLIC_URL=%q: the hosted service needs an https address", publicURL)
	}
	if sandbox != "bwrap" {
		return errors.New("REFRACT_MODE=saas needs REFRACT_SANDBOX=bwrap: without it a document can read other accounts' files")
	}
	if maxWait <= 0 {
		return errors.New("REFRACT_COMPILE_MAX_WAIT=0 (no limit) isn't allowed on the hosted service: a waiting request would outlast the proxy's limit")
	}
	// A compile is one request, and Cloudflare cuts a request with no response at about 100 s.
	if maxWait+compileTimeout > proxyLimit {
		hint := "Lower REFRACT_COMPILE_MAX_WAIT or REFRACT_COMPILE_TIMEOUT"
		if maxWait+60*time.Second <= proxyLimit {
			hint = "Set REFRACT_COMPILE_TIMEOUT=60"
		}
		return fmt.Errorf("REFRACT_COMPILE_MAX_WAIT (%ds) plus REFRACT_COMPILE_TIMEOUT (%ds) is over %ds: a proxy would cut the request first. %s", int(maxWait.Seconds()), int(compileTimeout.Seconds()), int(proxyLimit.Seconds()), hint)
	}
	return nil
}

// proxyLimit is the longest a compile request may take (its wait plus the
// compile), under the roughly 100 s after which a proxy gives up on it.
const proxyLimit = 95 * time.Second

// secureOrigin reports whether u is https, or http on this machine, so the
// hosted mode can be tried locally.
func secureOrigin(u *url.URL) bool {
	switch u.Scheme {
	case "https":
		return true
	case "http":
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			return true
		}
	}
	return false
}

// parseSignup reads REFRACT_SIGNUP: whether people may create their own
// account. The default is closed in every mode until the hosted service has
// terms and a privacy policy to show at sign-up (S8 makes it open in saas).
func parseSignup(_ mode.Mode, v string) (bool, error) {
	switch v {
	case "", "closed":
		return false, nil
	case "open":
		return true, nil
	}
	return false, fmt.Errorf("REFRACT_SIGNUP=%q: want open or closed", v)
}

// checkSignup refuses open sign-up on the hosted service when it can't be run
// properly. A self-hosted install, or one that doesn't sign people in itself,
// is never refused here.
func checkSignup(m mode.Mode, authKind string, signup, mailReady bool) error {
	if m != mode.SaaS || authKind != "builtin" || !signup {
		return nil
	}
	if !mailReady {
		return errors.New("REFRACT_MODE=saas with REFRACT_SIGNUP=open needs email: set REFRACT_SMTP_URL and REFRACT_MAIL_FROM, or REFRACT_SIGNUP=closed")
	}
	return errors.New(`REFRACT_MODE=saas can't open sign-up yet: this version can't show the terms and privacy policy people accept when they sign up. Set REFRACT_SIGNUP=closed and add accounts with "refract users add"`)
}
