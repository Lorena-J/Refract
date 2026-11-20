package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ponytojas/Refract/internal/store"
)

// localPort is where refract local listens unless the port is taken.
const localPort = 8080

// localKeyFile holds the generated secret key inside the data folder, until
// the system keychain takes over (#51).
const localKeyFile = "secret.key"

// runLocal is `refract local`: Refract on this computer with nothing to
// configure. Data lives in the per-user data folder, the secret key is made
// on first run, there is no sign-in and the server listens on loopback only.
func runLocal(args []string) error {
	fl := flag.NewFlagSet("refract local", flag.ContinueOnError)
	port := fl.Int("port", 0, "port to listen on, on 127.0.0.1 (default 8080, or a free port when 8080 is taken)")
	noBrowser := fl.Bool("no-browser", false, "don't open the browser")
	fl.Usage = func() {
		fmt.Fprint(fl.Output(), "Usage: refract local [--port N] [--no-browser]\n\n"+
			"Runs Refract on this computer with no configuration: data in your user\n"+
			"folder (REFRACT_DATA_DIR overrides it), no sign-in, 127.0.0.1 only.\n\n")
		fl.PrintDefaults()
	}
	if err := fl.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fl.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (see refract local -h)", fl.Arg(0))
	}
	portSet := false
	fl.Visit(func(f *flag.Flag) { portSet = portSet || f.Name == "port" })
	if portSet && (*port < 1 || *port > 65535) {
		return fmt.Errorf("--port %d: want a port from 1 to 65535", *port)
	}

	slog.SetDefault(slog.New(localLogHandler(os.Stderr)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return startLocal(ctx, localOptions{
		getenv:  os.Getenv,
		port:    *port,
		portSet: portSet,
		out:     os.Stdout,
		open: func(u string) {
			if !*noBrowser {
				openBrowser(u)
			}
		},
	})
}

type localOptions struct {
	getenv  func(string) string
	port    int  // with portSet, the port asked for with --port
	portSet bool // false: localPort, or a free one when it is taken
	out     io.Writer
	open    func(url string) // opens the browser
	// defaultDir replaces localDataDir's result, for tests.
	defaultDir string
}

// startLocal resolves the data folder, key and listener, then runs the server
// until ctx ends.
func startLocal(ctx context.Context, o localOptions) error {
	dataDir := o.getenv("REFRACT_DATA_DIR")
	if dataDir == "" {
		dataDir = o.defaultDir
	}
	if dataDir == "" {
		home, _ := os.UserHomeDir() // an empty home is reported by localDataDir
		var err error
		if dataDir, err = localDataDir(runtime.GOOS, o.getenv, home); err != nil {
			return err
		}
	}
	dataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return err
	}
	if v := o.getenv("REFRACT_AUTH"); v != "" && authMode(v) != "none" {
		slog.Warn("refract local runs without sign-in; REFRACT_AUTH is ignored", "REFRACT_AUTH", v)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("creating the data folder: %w", err)
	}
	var key []byte // nil: run reads REFRACT_SECRET_KEY, which still wins when set
	keyPath := filepath.Join(dataDir, localKeyFile)
	if o.getenv("REFRACT_SECRET_KEY") == "" {
		if key, err = loadOrCreateKey(keyPath); err != nil {
			return err
		}
	}
	return run(ctx, runOptions{
		dataDir:  dataDir,
		key:      key,
		authKind: "none",
		listen:   func() (net.Listener, error) { return listenLocal(o.getenv("REFRACT_ADDR"), o.port, o.portSet) },
		ready: func(ctx context.Context, addr net.Addr) {
			u := "http://" + addr.String()
			if !waitHealthy(ctx, u+"/healthz") {
				return
			}
			fmt.Fprintf(o.out, "\nRefract is running at %s\nData folder: %s\nThere is no sign-in: only this computer can open it. Press Ctrl+C to stop.\n\n", u, dataDir)
			o.open(u)
		},
	})
}

// localDataDir is the per-user folder where refract local keeps its data:
// ~/Library/Application Support/Refract on macOS, %LocalAppData%\Refract on
// Windows, and $XDG_DATA_HOME/refract or ~/.local/share/refract elsewhere.
func localDataDir(goos string, getenv func(string) string, home string) (string, error) {
	if goos == "windows" {
		if dir := getenv("LocalAppData"); dir != "" {
			return filepath.Join(dir, "Refract"), nil
		}
		return "", errors.New("%LocalAppData% is not set: set REFRACT_DATA_DIR to the folder Refract should use")
	}
	// XDG requires an absolute path and says to ignore a relative one.
	if dir := getenv("XDG_DATA_HOME"); goos != "darwin" && filepath.IsAbs(dir) {
		return filepath.Join(dir, "refract"), nil
	}
	if home == "" {
		return "", errors.New("can't find your home folder: set REFRACT_DATA_DIR to the folder Refract should use")
	}
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Refract"), nil
	}
	return filepath.Join(home, ".local", "share", "refract"), nil
}

// loadOrCreateKey reads the secret key at path, or creates it with a new
// random key on first run. It never replaces an existing file, even an
// unreadable one: credentials stored with that key would be lost.
func loadOrCreateKey(path string) ([]byte, error) {
	for range 2 {
		b, err := os.ReadFile(path)
		if err == nil {
			key, err := store.ParseKey(strings.TrimSpace(string(b)))
			if err != nil {
				return nil, fmt.Errorf("%s is not a valid secret key (%w). Restore the original file; deleting it makes stored passwords and API keys unreadable", path, err)
			}
			return key, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		key := make([]byte, 32)
		rand.Read(key)
		switch err := createExclusive(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n")); {
		case err == nil:
			return key, nil
		case !errors.Is(err, fs.ErrExist):
			return nil, fmt.Errorf("saving the secret key: %w", err)
		}
		// Another refract local created it first: read theirs.
	}
	return nil, fmt.Errorf("can't read or create the secret key at %s", path)
}

// createExclusive writes an owner-only file at path only if nothing is there
// yet. The data is complete before the file appears under its name, so a
// crash never leaves a half-written key.
func createExclusive(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Link(tmp.Name(), path) // fails with fs.ErrExist when path exists
}

// listenLocal opens refract local's listener, on loopback only. REFRACT_ADDR
// and --port choose the address; without either it is 127.0.0.1:8080, or a
// free port when 8080 is taken.
func listenLocal(envAddr string, port int, portSet bool) (net.Listener, error) {
	switch {
	case envAddr != "" && portSet:
		return nil, errors.New("set either REFRACT_ADDR or --port, not both")
	case envAddr != "":
		return listenLoopback(envAddr, "REFRACT_ADDR="+envAddr)
	case portSet:
		return listenLoopback(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "--port")
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort)))
	if err == nil {
		return ln, nil
	}
	slog.Info("port taken, using a free one", "port", localPort, "err", err)
	return net.Listen("tcp", "127.0.0.1:0")
}

// listenLoopback listens on addr and refuses it unless the address bound is
// loopback: without sign-in, other computers must not reach Refract. It
// judges the bound address because a name like "localhost" may resolve to
// anything.
func listenLoopback(addr, what string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if tcp, ok := ln.Addr().(*net.TCPAddr); !ok || !tcp.IP.IsLoopback() {
		ln.Close()
		return nil, fmt.Errorf("%s listens on %s, which other computers can reach; refract local has no sign-in, so it only listens on 127.0.0.1 or ::1", what, ln.Addr())
	}
	return ln, nil
}

// waitHealthy polls url until it answers 200 or ctx ends, and reports which.
func waitHealthy(ctx context.Context, url string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if resp, err := client.Do(req); err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// localLogHandler writes readable lines to a terminal and JSON otherwise
// (for instance when a desktop app or a service manager collects the log).
func localLogHandler(w *os.File) slog.Handler {
	if fi, err := w.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 {
		return slog.NewTextHandler(w, nil)
	}
	return slog.NewJSONHandler(w, nil)
}
