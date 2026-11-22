package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

// Embedding contract (refract local --embedded), for the desktop app.
const (
	// embedTokenEnv carries the per-launch token when --token-fd isn't used.
	// It is removed from the process environment as soon as it is read.
	embedTokenEnv = "REFRACT_EMBED_TOKEN"
	// embedCookie is the cookie the app sets, with the token as its value, in
	// its web view before the first load.
	embedCookie = "refract_embed"
	// stateReady is the state of the JSON line printed once the server answers.
	stateReady = "ready"
	// stateError is the state of the JSON line printed when starting fails.
	stateError = "error"
)

const embedTokenMin = 16

// readEmbedToken takes the per-launch token from the inherited file descriptor
// fd (when fd >= 0) or from REFRACT_EMBED_TOKEN, and removes the variable from
// the environment either way, so programs started later (tectonic, git) never
// see it. It is never logged: errors don't contain it.
func readEmbedToken(fd int, getenv func(string) string, unsetenv func(string) error) (string, error) {
	fromEnv := getenv(embedTokenEnv)
	unsetenv(embedTokenEnv)
	tok := fromEnv
	if fd >= 0 {
		f := os.NewFile(uintptr(fd), "token")
		if f == nil {
			return "", fmt.Errorf("--token-fd %d is not an open file descriptor", fd)
		}
		b, err := io.ReadAll(io.LimitReader(f, 1<<10))
		f.Close()
		if err != nil {
			return "", fmt.Errorf("reading the token from --token-fd %d: %w", fd, err)
		}
		tok = string(b)
	}
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "", errors.New("--embedded needs the per-launch token: pass it on the file descriptor given with --token-fd, or in " + embedTokenEnv)
	}
	if len(tok) < embedTokenMin {
		return "", fmt.Errorf("the embedding token is too short: use at least %d characters (for instance 32 random bytes, base64url)", embedTokenMin)
	}
	for _, r := range tok {
		// What a cookie value may hold: printable ASCII except " , ; \.
		if r < 0x21 || r > 0x7e || strings.ContainsRune(`",;\`, r) {
			return "", errors.New("the embedding token has characters a cookie can't carry: use base64url or hex")
		}
	}
	return tok, nil
}

// requireEmbedToken answers 401 to every request that doesn't carry the token
// in the embed cookie, except GET /healthz. It compares in constant time.
func requireEmbedToken(token string) func(http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" {
				next.ServeHTTP(w, r)
				return
			}
			if c, err := r.Cookie(embedCookie); err == nil {
				got := sha256.Sum256([]byte(c.Value))
				if subtle.ConstantTimeCompare(got[:], want[:]) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

// watchStdin cancels ctx when r (the standard input) ends or fails: the
// parent that owns this process is gone and Refract shuts down.
func watchStdin(ctx context.Context, r io.Reader) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		io.Copy(io.Discard, r)
		if ctx.Err() == nil {
			slog.Info("standard input closed; shutting down")
		}
		cancel()
	}()
	return ctx, cancel
}

// jsonOut is the standard output of a program a parent reads: it remembers
// whether a line was written, so exactly one JSON line comes out.
type jsonOut struct {
	w     io.Writer
	wrote atomic.Bool
}

func (j *jsonOut) Write(p []byte) (int, error) {
	j.wrote.Store(true)
	return j.w.Write(p)
}

func printJSON(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	fmt.Fprintf(w, "%s\n", b)
}

type errorStatus struct {
	State string `json:"state"`
	Error string `json:"error"`
}

// wantsJSON reports whether args ask for --embedded or --status, before they
// are parsed, so that even a bad flag is answered with a JSON line.
func wantsJSON(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		a = strings.TrimLeft(a, "-")
		name, val, _ := strings.Cut(a, "=")
		if (name == "embedded" || name == "status") && val != "false" && val != "0" {
			return true
		}
	}
	return false
}
