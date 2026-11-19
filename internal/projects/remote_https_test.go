package projects

import (
	"context"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// gitServer is a local https server that records the Authorization header of
// every request git makes to it, and answers 404 (or redirects).
type gitServer struct {
	*httptest.Server
	mu   sync.Mutex
	auth []string
}

func (g *gitServer) seen() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.auth...)
}

func newGitServer(t *testing.T, redirectTo string) *gitServer {
	t.Helper()
	g := &gitServer{}
	g.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.auth = append(g.auth, r.Header.Get("Authorization"))
		g.mu.Unlock()
		if redirectTo != "" {
			http.Redirect(w, r, redirectTo+r.URL.RequestURI(), http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(g.Close)
	// git trusts the test certificate through the one GIT_ variable kept.
	ca := filepath.Join(t.TempDir(), "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: g.Certificate().Raw}), 0o644)
	t.Setenv("GIT_SSL_CAINFO", ca)
	return g
}

// httpsProject is a project whose remote is on target, with a synthetic token.
func httpsProject(t *testing.T, target string) (*Service, string) {
	t.Helper()
	s, id := newTurnProject(t)
	s.Key = make([]byte, 32)
	if err := s.SetRemote(ownerCtx, id, Remote{URL: target + "/paper.git", Token: "synthetic.tok"}, true); err != nil {
		t.Fatal(err)
	}
	return s, id
}

const wantAuth = "Basic eC1hY2Nlc3MtdG9rZW46c3ludGhldGljLnRvaw==" // x-access-token:synthetic.tok

func noSecret(t *testing.T, err error) {
	t.Helper()
	if err != nil && (strings.Contains(err.Error(), "synthetic.tok") || strings.Contains(err.Error(), basicAuth("synthetic.tok"))) {
		t.Errorf("the error shows the credential: %v", err)
	}
}

func TestHTTPSRemoteGetsItsToken(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git over https")
	}
	srv := newGitServer(t, "")
	s, id := httpsProject(t, srv.URL)
	err := s.Push(ownerCtx, id)
	if err == nil {
		t.Fatal("push to a 404 succeeded")
	}
	noSecret(t, err)
	if _, err := s.Pull(ownerCtx, id); err == nil {
		t.Fatal("pull from a 404 succeeded")
	}
	seen := srv.seen()
	if len(seen) < 2 {
		t.Fatalf("requests = %v", seen)
	}
	for _, a := range seen {
		if a != wantAuth {
			t.Errorf("Authorization = %q, want the saved token", a)
		}
	}
}

func TestHTTPSRedirectGetsNoToken(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git over https")
	}
	other := newGitServer(t, "")
	hop := newGitServer(t, other.URL) // same certificate: git would trust both
	s, id := httpsProject(t, hop.URL)
	noSecret(t, s.Push(ownerCtx, id))
	if _, err := s.Pull(ownerCtx, id); err == nil {
		t.Error("pull through a redirect succeeded")
	}
	if seen := other.seen(); len(seen) != 0 {
		t.Errorf("the redirect target got requests: %q", seen)
	}
}

// Configuration that would rewrite the destination, or print the header, is
// either refused (the project's own config) or not passed to git (the
// server's environment): the token reaches only its origin.
func TestHTTPSIgnoresRoutingConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git over https")
	}
	other := newGitServer(t, "")
	srv := newGitServer(t, "")
	s, id := httpsProject(t, srv.URL)

	if _, err := git(ownerCtx, s.Path(id), "config", "--local", "url."+other.URL+"/.insteadOf", srv.URL+"/"); err != nil {
		t.Fatal(err)
	}
	if err := s.Push(ownerCtx, id); !errors.Is(err, ErrRepoConfig) {
		t.Errorf("push with a url rewrite in the project: %v", err)
	}
	git(ownerCtx, s.Path(id), "config", "--local", "--remove-section", "url."+other.URL+"/")
	if _, err := git(ownerCtx, s.Path(id), "config", "--local", "http.sslVerify", "false"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pull(ownerCtx, id); !errors.Is(err, ErrRepoConfig) {
		t.Errorf("pull with http settings in the project: %v", err)
	}
	git(ownerCtx, s.Path(id), "config", "--local", "--unset", "http.sslVerify")

	t.Setenv("GIT_CONFIG_PARAMETERS", "'url."+other.URL+"/.insteadof'='"+srv.URL+"/'")
	t.Setenv("GIT_TRACE_CURL", "1")
	t.Setenv("GIT_TRACE_REDACT", "0")
	t.Setenv("GIT_CURL_VERBOSE", "1")
	err := s.Push(ownerCtx, id)
	if err == nil {
		t.Fatal("push to a 404 succeeded")
	}
	noSecret(t, err)
	if seen := other.seen(); len(seen) != 0 {
		t.Errorf("the rewritten destination got requests: %q", seen)
	}
	if seen := srv.seen(); len(seen) == 0 || seen[0] != wantAuth {
		t.Errorf("the remote got %q", seen)
	}
}

// Automatic push, from a real commit, sends the token to the remote's origin.
func TestHTTPSAutoPush(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git over https")
	}
	srv := newGitServer(t, "")
	s, id := httpsProject(t, srv.URL)
	done := make(chan error, 1)
	s.OnCommit = func(ctx context.Context, id string) { done <- s.Push(ctx, id) }
	os.WriteFile(filepath.Join(s.Path(id), "main.tex"), []byte("auto\n"), 0o644)
	if _, err := s.Checkpoint(ownerCtx, id, "c"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		noSecret(t, err)
	case <-time.After(30 * time.Second):
		t.Fatal("auto-push didn't run")
	}
	if seen := srv.seen(); len(seen) == 0 || seen[0] != wantAuth {
		t.Errorf("auto-push sent %q", seen)
	}
}
