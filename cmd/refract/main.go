// Command refract serves the Refract API and the embedded web UI.
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/ponytojas/Refract/internal/agent"
	"github.com/ponytojas/Refract/internal/ai"
	"github.com/ponytojas/Refract/internal/assistant"
	"github.com/ponytojas/Refract/internal/auth"
	"github.com/ponytojas/Refract/internal/compile"
	"github.com/ponytojas/Refract/internal/projects"
	"github.com/ponytojas/Refract/internal/siwc"
	"github.com/ponytojas/Refract/internal/store"
	"github.com/ponytojas/Refract/web"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "login" {
		if err := runLogin(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "login failed:", err)
			os.Exit(1)
		}
		return
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Absolute: tectonic and synctex run in other working directories.
	dataDir, err := filepath.Abs(cmp.Or(os.Getenv("REFRACT_DATA_DIR"), "/data"))
	if err != nil {
		return err
	}
	addr := cmp.Or(os.Getenv("REFRACT_ADDR"), ":8080")

	guard, err := authMiddleware(ctx)
	if err != nil {
		return err
	}
	key, err := store.ParseKey(os.Getenv("REFRACT_SECRET_KEY"))
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	db, err := store.Open(filepath.Join(dataDir, "refract.db"))
	if err != nil {
		return err
	}
	defer db.Close()

	session := &siwc.Session{DB: db, Key: key, OAuth: siwc.NewOAuth(ctx), AllowedSubject: os.Getenv("REFRACT_ALLOWED_SUBJECT")}
	if _, err := session.HostID(ctx); err != nil {
		return err
	}
	a := &api{
		session:  session,
		provider: &ai.SIWC{Tokens: session, BaseURL: siwc.Resource, HTTP: &http.Client{}},
		model:    os.Getenv("REFRACT_MODEL_AGENT"),
	}

	pa := &projectsAPI{
		svc: &projects.Service{DB: db, Dir: filepath.Join(dataDir, "projects"), Limits: projects.DefaultLimits},
		tmp: filepath.Join(dataDir, "tmp"),
	}

	timeout := 90 * time.Second
	if v := os.Getenv("REFRACT_COMPILE_TIMEOUT"); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			return fmt.Errorf("REFRACT_COMPILE_TIMEOUT=%q: want a positive number of seconds", v)
		}
		timeout = time.Duration(secs) * time.Second
	}
	ca := &compileAPI{svc: pa.svc, synctex: &compile.SyncTeX{}, runner: &compile.Runner{Compiler: &compile.Tectonic{
		Bundle:   os.Getenv("REFRACT_TECTONIC_BUNDLE"),
		CacheDir: os.Getenv("TECTONIC_CACHE_DIR"),
		Timeout:  timeout,
	}}}

	asa := &assistantAPI{svc: &assistant.Service{
		DB:       db,
		Projects: pa.svc,
		Agent: agent.Agent{
			Provider:      a.provider,
			Compiler:      ca.runner.Compiler,
			MaxIterations: 25,
			Timeout:       15 * time.Minute,
		},
		ModelFor: func(ctx context.Context) (string, error) {
			if a.model != "" {
				return a.model, nil
			}
			// No model configured: use the first one the plan offers.
			ms, err := a.provider.Models(ctx)
			if err != nil {
				return "", err
			}
			if len(ms) == 0 {
				return "", errors.New("no models available on this ChatGPT plan; set REFRACT_MODEL_AGENT")
			}
			return ms[0].Slug, nil
		},
	}}
	if err := asa.svc.Recover(ctx); err != nil {
		return err
	}

	app := http.NewServeMux()
	a.routes(app, os.Getenv("REFRACT_DEV") == "1")
	pa.routes(app)
	ca.routes(app)
	asa.routes(app)
	app.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	app.Handle("/", web.Handler())

	mux := http.NewServeMux()
	// Outside auth: the container platform's health check hits the origin directly.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			http.Error(w, "db unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	})
	mux.Handle("/", guard(auth.RequireCSRFHeader(app)))

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("listening", "addr", addr, "data_dir", dataDir)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// authMiddleware picks the web UI guard from REFRACT_AUTH. It fails closed:
// running without auth must be requested explicitly.
func authMiddleware(ctx context.Context) (func(http.Handler) http.Handler, error) {
	switch mode := cmp.Or(os.Getenv("REFRACT_AUTH"), "cloudflare"); mode {
	case "cloudflare":
		team, aud := os.Getenv("REFRACT_CF_TEAM_DOMAIN"), os.Getenv("REFRACT_CF_AUD")
		if team == "" || aud == "" {
			return nil, errors.New("REFRACT_AUTH=cloudflare needs REFRACT_CF_TEAM_DOMAIN and REFRACT_CF_AUD (or set REFRACT_AUTH=none to run unprotected)")
		}
		return auth.CloudflareAccess(ctx, team, aud), nil
	case "none":
		slog.Warn("REFRACT_AUTH=none: the web UI is unprotected; only use this locally or behind your own auth")
		return func(h http.Handler) http.Handler { return h }, nil
	default:
		return nil, fmt.Errorf("REFRACT_AUTH=%q: want cloudflare or none", mode)
	}
}
