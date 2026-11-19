package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ponytojas/Refract/internal/store"
)

func TestRemoteOrigin(t *testing.T) {
	for raw, want := range map[string]string{
		"https://github.com/a/b.git":         "https://github.com:443",
		"https://GitHub.COM:443/c/d":         "https://github.com:443",
		"https://git.lab.org:8443/x.git":     "https://git.lab.org:8443",
		"https://[2001:db8::1]/x.git":        "https://[2001:db8::1]:443",
		"file:///tmp/remote.git":             "file://",
		"https:///no-host":                   "",
		"https://git.lab.org.evil.example/x": "https://git.lab.org.evil.example:443",
		" https://git.lab.org/padded.git\n":  "https://git.lab.org:443",
	} {
		if got := remoteOrigin(raw); got != want {
			t.Errorf("remoteOrigin(%q) = %q, want %q", raw, got, want)
		}
	}
}

// sealRemote stores a remote record as is, as an older release saved it.
func sealRemote(t *testing.T, s *Service, id, record string) {
	t.Helper()
	sealed, err := store.Seal(s.Key, []byte(record), remoteKey(id))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(context.Background(), s.DB, remoteKey(id), sealed); err != nil {
		t.Fatal(err)
	}
}

func TestTokenStaysWithItsOrigin(t *testing.T) {
	ctx := ownerCtx
	s, id := newTurnProject(t)
	s.Key = make([]byte, 32)
	token := func() (string, string) {
		t.Helper()
		r, err := s.Remote(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return r.Token, r.TokenOrigin
	}
	if err := s.SetRemote(ctx, id, Remote{URL: "https://git.lab.org/a/b.git", Token: "tok.lab"}, true); err != nil {
		t.Fatal(err)
	}
	if tok, origin := token(); tok != "tok.lab" || origin != "https://git.lab.org:443" {
		t.Fatalf("saved = %q for %q", tok, origin)
	}

	// Same origin, written differently, another repository and branch: kept.
	if err := s.SetRemote(ctx, id, Remote{URL: "https://GIT.lab.org:443/a/c.git", Branch: "draft", AutoPush: true}, true); err != nil {
		t.Fatalf("same-origin edit: %v", err)
	}
	if tok, _ := token(); tok != "tok.lab" {
		t.Errorf("same-origin edit dropped the token: %q", tok)
	}

	// Another host, another port, a look-alike host: refused, nothing changes.
	for _, u := range []string{"https://evil.example/a/b.git", "https://git.lab.org:8443/a/b.git", "https://git.lab.org.evil.example/a/b.git"} {
		if err := s.SetRemote(ctx, id, Remote{URL: u}, true); !errors.Is(err, ErrTokenOrigin) {
			t.Errorf("%s with the saved token: %v", u, err)
		}
	}
	if r, _ := s.Remote(ctx, id); r.URL != "https://GIT.lab.org:443/a/c.git" || r.Token != "tok.lab" {
		t.Errorf("a refused save changed the remote: %+v", r)
	}

	// The client can't bind a token to an origin of its choosing.
	if err := s.SetRemote(ctx, id, Remote{URL: "https://evil.example/a/b.git", TokenOrigin: "https://git.lab.org:443"}, true); !errors.Is(err, ErrTokenOrigin) {
		t.Errorf("client-supplied origin: %v", err)
	}

	// Another host with a new token binds that one to the new host.
	if err := s.SetRemote(ctx, id, Remote{URL: "https://code.example/a/b.git", Token: "tok.code"}, true); err != nil {
		t.Fatal(err)
	}
	if tok, origin := token(); tok != "tok.code" || origin != "https://code.example:443" {
		t.Errorf("new host = %q for %q", tok, origin)
	}

	// Another host without a token, dropping the saved one: saved, no token.
	if err := s.SetRemote(ctx, id, Remote{URL: "https://public.example/a/b.git"}, false); err != nil {
		t.Fatal(err)
	}
	if tok, origin := token(); tok != "" || origin != "" {
		t.Errorf("cleared = %q for %q", tok, origin)
	}
}

func TestLegacyRemoteKeepsWorking(t *testing.T) {
	ctx := ownerCtx
	s, id, bare := remoteProject(t)
	// As saved before tokens recorded their origin.
	sealRemote(t, s, id, `{"url":"file://`+bare+`","branch":"main","auto_push":false,"github":false,"token":"legacy.tok"}`)
	r, err := s.Remote(ctx, id)
	if err != nil || r.Token != "legacy.tok" || r.TokenOrigin != "file://" {
		t.Fatalf("legacy = %+v, %v", r, err)
	}
	os.WriteFile(filepath.Join(s.Path(id), "main.tex"), []byte("legacy push\n"), 0o644)
	if err := s.Push(ctx, id); err != nil {
		t.Fatalf("push: %v", err)
	}
	if res, err := s.Pull(ctx, id); err != nil || len(res.Conflicts) != 0 {
		t.Fatalf("pull = %+v, %v", res, err)
	}

	// A legacy https record: same-origin edits keep it, another host doesn't.
	sealRemote(t, s, id, `{"url":"https://git.lab.org/a/b.git","branch":"main","token":"legacy.tok"}`)
	if err := s.SetRemote(ctx, id, Remote{URL: "https://git.lab.org/a/renamed.git"}, true); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.Remote(ctx, id); r.Token != "legacy.tok" || r.TokenOrigin != "https://git.lab.org:443" {
		t.Errorf("same-origin edit of a legacy record: %+v", r)
	}
	sealRemote(t, s, id, `{"url":"https://git.lab.org/a/b.git","branch":"main","token":"legacy.tok"}`)
	if err := s.SetRemote(ctx, id, Remote{URL: "https://evil.example/a/b.git"}, true); !errors.Is(err, ErrTokenOrigin) {
		t.Errorf("legacy record to another host: %v", err)
	}
}

// A record whose token and URL disagree (edited by hand, or by a bug) is
// refused before git runs, for manual and automatic sync alike.
func TestMismatchedTokenIsNeverSent(t *testing.T) {
	ctx := ownerCtx
	s, id, _ := remoteProject(t)
	sealRemote(t, s, id, `{"url":"https://evil.example/a/b.git","branch":"main","token":"tok.lab","token_origin":"https://git.lab.org:443"}`)
	if err := s.Push(ctx, id); !errors.Is(err, ErrTokenOrigin) {
		t.Errorf("push: %v", err)
	}
	if _, err := s.Pull(ctx, id); !errors.Is(err, ErrTokenOrigin) {
		t.Errorf("pull: %v", err)
	}
}

func TestGitHubTokenOnlyGoesToGitHub(t *testing.T) {
	ctx := ownerCtx
	s, id := newTurnProject(t)
	s.Key = make([]byte, 32)
	var asked int
	s.GitHubToken = func(context.Context, string) (string, error) {
		asked++
		return "gh.tok", nil
	}
	if err := s.SetRemote(ctx, id, Remote{URL: "https://evil.example/a/b.git", GitHub: true}, true); !errors.Is(err, ErrGitHubRemote) {
		t.Errorf("GitHub mode on another host: %v", err)
	}
	if err := s.SetRemote(ctx, id, Remote{URL: "https://github.com/a/b.git", GitHub: true, Token: "ignored", TokenOrigin: "https://evil.example:443"}, true); err != nil {
		t.Fatal(err)
	}
	r, err := s.Remote(ctx, id)
	if err != nil || r.Token != "gh.tok" || r.TokenOrigin != "https://github.com:443" {
		t.Fatalf("github remote = %+v, %v", r, err)
	}
	if _, _, err := s.remoteArgs(r, nil); err != nil {
		t.Errorf("args for GitHub: %v", err)
	}

	// Saved by an older release on another host: no token is looked up, and
	// push and pull refuse before running git.
	sealRemote(t, s, id, `{"url":"https://evil.example/a/b.git","branch":"main","github":true}`)
	asked = 0
	if r, err := s.Remote(ctx, id); err != nil || r.Token != "" {
		t.Errorf("legacy GitHub remote elsewhere = %+v, %v", r, err)
	}
	if err := s.Push(ctx, id); !errors.Is(err, ErrGitHubRemote) {
		t.Errorf("push: %v", err)
	}
	if _, err := s.Pull(ctx, id); !errors.Is(err, ErrGitHubRemote) {
		t.Errorf("pull: %v", err)
	}
	if asked != 0 {
		t.Errorf("GitHub token looked up %d times for another host", asked)
	}

	// A GitHub Enterprise origin, when configured, replaces github.com.
	s.GitHubOrigin = "https://github.lab.org"
	if err := s.SetRemote(ctx, id, Remote{URL: "https://github.com/a/b.git", GitHub: true}, true); !errors.Is(err, ErrGitHubRemote) {
		t.Errorf("github.com with another GitHub origin: %v", err)
	}
}

func TestRemoteArgs(t *testing.T) {
	s := &Service{}
	header := func(env []string) string {
		for _, e := range env {
			if v, ok := strings.CutPrefix(e, "GIT_CONFIG_VALUE_0="); ok {
				return v
			}
		}
		return ""
	}
	opts, env, err := s.remoteArgs(&Remote{URL: "https://public.example/a.git"}, nil)
	if err != nil || header(env) != "" || slices.Contains(opts, "http.followRedirects=false") {
		t.Errorf("without a token: %v %v %v", opts, env, err)
	}
	opts, env, err = s.remoteArgs(&Remote{URL: "https://git.lab.org/a.git", Token: "t", TokenOrigin: "https://git.lab.org:443"}, nil)
	if err != nil || !strings.HasPrefix(header(env), "Authorization: Basic ") || !slices.Contains(opts, "http.followRedirects=false") {
		t.Errorf("with a token: %v %v %v", opts, env, err)
	}
	if !slices.Contains(opts, "protocol.allow=never") || slices.Contains(opts, "protocol.file.allow=always") {
		t.Errorf("transports: %v", opts)
	}
	// pinRemote's own refusal isn't repeated.
	pin := []string{"-c", "http.followRedirects=false", "-c", "http.curloptResolve=git.lab.org:443:93.184.215.14"}
	opts, _, _ = s.remoteArgs(&Remote{URL: "https://git.lab.org/a.git", Token: "t", TokenOrigin: "https://git.lab.org:443"}, pin)
	n := 0
	for _, o := range opts {
		if o == "http.followRedirects=false" {
			n++
		}
	}
	if n != 1 || pin[len(pin)-1] != "http.curloptResolve=git.lab.org:443:93.184.215.14" {
		t.Errorf("with a pin: %v", opts)
	}
	for _, r := range []Remote{
		{URL: "https://evil.example/a.git", Token: "t", TokenOrigin: "https://git.lab.org:443"},
		{URL: "https://git.lab.org:8443/a.git", Token: "t", TokenOrigin: "https://git.lab.org:443"},
		{URL: "https://git.lab.org/a.git", Token: "t"}, // no origin: never sent
	} {
		if _, env, err := s.remoteArgs(&r, nil); !errors.Is(err, ErrTokenOrigin) || env != nil {
			t.Errorf("%+v: %v, %v", r, env, err)
		}
	}
	if _, _, err := s.remoteArgs(&Remote{URL: "https://evil.example/a.git", GitHub: true}, nil); !errors.Is(err, ErrGitHubRemote) {
		t.Errorf("GitHub elsewhere: %v", err)
	}
}
