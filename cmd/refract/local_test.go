package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ponytojas/Refract/internal/providers"
	"github.com/ponytojas/Refract/internal/store"
)

func TestLocalDataDir(t *testing.T) {
	env := func(kv ...string) func(string) string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return func(k string) string { return m[k] }
	}
	home := filepath.FromSlash("/home/ana")
	for _, tc := range []struct {
		goos    string
		env     func(string) string
		home    string
		want    string // slash-separated; empty means an error
		wantErr string
	}{
		{"darwin", env(), home, "/home/ana/Library/Application Support/Refract", ""},
		{"darwin", env("XDG_DATA_HOME", "/xdg"), home, "/home/ana/Library/Application Support/Refract", ""},
		{"linux", env(), home, "/home/ana/.local/share/refract", ""},
		{"linux", env("XDG_DATA_HOME", "/xdg"), home, "/xdg/refract", ""},
		{"freebsd", env("XDG_DATA_HOME", "/xdg"), home, "/xdg/refract", ""},
		// A relative XDG_DATA_HOME is invalid and ignored.
		{"linux", env("XDG_DATA_HOME", "rel/xdg"), home, "/home/ana/.local/share/refract", ""},
		{"linux", env(), "", "", "home folder"},
		{"darwin", env(), "", "", "home folder"},
		{"windows", env("LocalAppData", "/Users/ana/AppData/Local"), "", "/Users/ana/AppData/Local/Refract", ""},
		{"windows", env(), home, "", "LocalAppData"},
	} {
		got, err := localDataDir(tc.goos, tc.env, tc.home)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "REFRACT_DATA_DIR") {
				t.Errorf("%s: err = %v, want one mentioning %q and REFRACT_DATA_DIR", tc.goos, err, tc.wantErr)
			}
			continue
		}
		if err != nil || got != filepath.FromSlash(tc.want) {
			t.Errorf("%s: got %q, %v; want %q", tc.goos, got, err, filepath.FromSlash(tc.want))
		}
	}
}

func TestLoadOrCreateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), localKeyFile)
	first, err := loadOrCreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 {
		t.Fatalf("key is %d bytes", len(first))
	}
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("key file mode = %v, %v; want 0600", fi.Mode().Perm(), err)
		}
	}
	again, err := loadOrCreateKey(path)
	if err != nil || !bytes.Equal(first, again) {
		t.Fatalf("second load: %v, same key = %v", err, bytes.Equal(first, again))
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".tmp-*")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}

	// An invalid file is reported, never replaced.
	bad := filepath.Join(t.TempDir(), localKeyFile)
	os.WriteFile(bad, []byte("not a key\n"), 0o600)
	if _, err := loadOrCreateKey(bad); err == nil || !strings.Contains(err.Error(), bad) {
		t.Fatalf("invalid key: err = %v, want one naming the file", err)
	}
	if b, _ := os.ReadFile(bad); string(b) != "not a key\n" {
		t.Fatalf("invalid key file was changed: %q", b)
	}
}

func TestLoadOrCreateKeyConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), localKeyFile)
	keys := make([][]byte, 8)
	var wg sync.WaitGroup
	for i := range keys {
		wg.Go(func() {
			k, err := loadOrCreateKey(path)
			if err != nil {
				t.Error(err)
			}
			keys[i] = k
		})
	}
	wg.Wait()
	for _, k := range keys[1:] {
		if !bytes.Equal(k, keys[0]) {
			t.Fatal("concurrent first runs ended with different keys")
		}
	}
}

func TestListenLocal(t *testing.T) {
	port := func(ln net.Listener) int { return ln.Addr().(*net.TCPAddr).Port }
	loopback := func(t *testing.T, ln net.Listener) {
		t.Helper()
		if !ln.Addr().(*net.TCPAddr).IP.IsLoopback() {
			t.Fatalf("listens on %s", ln.Addr())
		}
	}

	t.Run("default port, or a free one when taken", func(t *testing.T) {
		ln, err := listenLocal("", 0, false)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		loopback(t, ln)
		if port(ln) != localPort {
			// Something else on this machine has 8080: that already is the fallback.
			t.Logf("port %d was taken; got %d", localPort, port(ln))
			return
		}
		// 8080 is now ours, so a second start falls back to another port.
		other, err := listenLocal("", 0, false)
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close()
		loopback(t, other)
		if port(other) == localPort || port(other) == 0 {
			t.Fatalf("fallback port = %d", port(other))
		}
	})

	t.Run("--port is used as given and not replaced when taken", func(t *testing.T) {
		busy, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer busy.Close()
		if _, err := listenLocal("", port(busy), true); err == nil {
			t.Fatal("a taken --port was replaced by another")
		}
		busy.Close()
		ln, err := listenLocal("", port(busy), true)
		if err != nil {
			t.Fatal(err)
		}
		defer ln.Close()
		loopback(t, ln)
		if port(ln) != port(busy) {
			t.Fatalf("port = %d, want %d", port(ln), port(busy))
		}
	})

	t.Run("REFRACT_ADDR must be loopback", func(t *testing.T) {
		for _, addr := range []string{"0.0.0.0:0", ":0", "[::]:0"} {
			ln, err := listenLocal(addr, 0, false)
			if err == nil {
				ln.Close()
				t.Errorf("REFRACT_ADDR=%s accepted", addr)
				continue
			}
			if !strings.Contains(err.Error(), "other computers") {
				t.Errorf("REFRACT_ADDR=%s: %v", addr, err)
			}
		}
		ln, err := listenLocal("127.0.0.1:0", 0, false)
		if err != nil {
			t.Fatal(err)
		}
		ln.Close()
		if _, err := listenLocal("127.0.0.1:0", 9000, true); err == nil {
			t.Error("REFRACT_ADDR and --port together were accepted")
		}
	})
}

// TestLocalStartsAndReopens runs refract local twice on the same data folder
// with no REFRACT_* variables: the second run uses the key the first made, so
// a credential stored in between is still readable.
func TestLocalStartsAndReopens(t *testing.T) {
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "REFRACT_") {
			t.Setenv(name, "")
		}
	}
	dataDir := filepath.Join(t.TempDir(), "Refract")

	start := func() (stop func()) {
		t.Helper()
		ctx, cancel := context.WithCancel(context.Background())
		opened := make(chan string, 1)
		var out bytes.Buffer
		done := make(chan error, 1)
		go func() {
			done <- startLocal(ctx, localOptions{
				getenv:     os.Getenv,
				port:       0,
				portSet:    false,
				out:        &out,
				open:       func(u string) { opened <- u },
				defaultDir: dataDir,
			})
		}()
		var u string
		select {
		case u = <-opened:
		case err := <-done:
			cancel()
			t.Fatalf("refract local stopped: %v", err)
		case <-time.After(20 * time.Second):
			cancel()
			t.Fatal("the browser was never opened")
		}
		host, p, _ := net.SplitHostPort(strings.TrimPrefix(u, "http://"))
		if n, _ := strconv.Atoi(p); host != "127.0.0.1" || n == 0 {
			t.Errorf("opened %s, want a 127.0.0.1 address", u)
		}
		if !strings.Contains(out.String(), u) || !strings.Contains(out.String(), dataDir) {
			t.Errorf("printed %q, want the address and the data folder", out.String())
		}
		// No sign-in: the API answers as the local user.
		req, _ := http.NewRequest(http.MethodGet, u+"/api/projects", nil)
		req.Header.Set("X-Refract", "1")
		if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusOK {
			t.Errorf("GET /api/projects: %v %v", resp, err)
		} else {
			resp.Body.Close()
		}
		return func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("stopping: %v", err)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("refract local didn't stop")
			}
		}
	}

	start()()
	keyPath := filepath.Join(dataDir, localKeyFile)
	key, err := loadOrCreateKey(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "refract.db")); err != nil {
		t.Fatal(err)
	}
	cred := providers.Config{ID: "p1", Kind: "openai", Name: "OpenAI", APIKey: "sk-test"}
	withDB(t, dataDir, func(db *providers.Store) {
		db.Key = key
		if err := db.Save(context.Background(), "local", cred); err != nil {
			t.Fatal(err)
		}
	})

	start()()
	again, err := loadOrCreateKey(keyPath)
	if err != nil || !bytes.Equal(again, key) {
		t.Fatalf("the key changed between runs: %v", err)
	}
	withDB(t, dataDir, func(db *providers.Store) {
		db.Key = again
		cs, err := db.List(context.Background(), "local")
		if err != nil || len(cs) != 1 || cs[0].APIKey != cred.APIKey {
			t.Fatalf("stored credential after restart: %+v, %v", cs, err)
		}
	})
}

func withDB(t *testing.T, dataDir string, f func(*providers.Store)) {
	t.Helper()
	db, err := store.Open(filepath.Join(dataDir, "refract.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f(&providers.Store{DB: db})
}
