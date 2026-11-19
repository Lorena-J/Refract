package main

import (
	"errors"
	"fmt"
)

// profile is REFRACT_PROFILE: how much Refract trusts the people who compile.
// "personal" (the default): one person or people who trust each other.
// "team": accounts that must not read each other's files, so every compile
// runs in the sandbox and the server refuses to start without it.
type profile string

const (
	profilePersonal profile = "personal"
	profileTeam     profile = "team"
)

func parseProfile(v string) (profile, error) {
	switch v {
	case "", string(profilePersonal):
		return profilePersonal, nil
	case string(profileTeam):
		return profileTeam, nil
	}
	return "", fmt.Errorf("REFRACT_PROFILE=%q: want personal or team", v)
}

// applyProfile makes the team profile's sandbox the default and refuses a
// team configuration that wouldn't isolate accounts. sandboxSet says whether
// REFRACT_SANDBOX was given. The sandbox's own startup check (compile.Sandbox.Check)
// then verifies bubblewrap, the bundle and a real compile.
func applyProfile(p profile, authKind string, cfg *compileConfig, sandboxSet bool) error {
	if p != profileTeam {
		return nil
	}
	if authKind == "none" {
		return errors.New("REFRACT_PROFILE=team needs people to sign in: with REFRACT_AUTH=none everyone is the same account. Set REFRACT_AUTH=builtin (or cloudflare), see \"Choose how people sign in\" in the README")
	}
	if !sandboxSet {
		cfg.sandbox = "bwrap"
	}
	if cfg.sandbox != "bwrap" {
		return errors.New("REFRACT_PROFILE=team needs the compile sandbox: without it a document can read other accounts' projects and the database. Remove REFRACT_SANDBOX=none (the team profile uses bwrap), see \"Run it for a team\" in the README")
	}
	return nil
}

// unisolatedWarning is logged when several accounts compile without the
// sandbox: allowed (people may trust each other), but worth saying.
func unisolatedWarning(p profile, authKind, sandbox string) string {
	if p == profileTeam || authKind == "none" || sandbox == "bwrap" {
		return ""
	}
	return "compiles are not isolated: a document can read other accounts' projects and the database. If the people signing in don't all trust each other, set REFRACT_PROFILE=team (see \"Run it for a team\" in the README)"
}
