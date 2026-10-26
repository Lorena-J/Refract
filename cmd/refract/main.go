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
	"syscall"
	"time"

	"refract/internal/ai"
	"refract/internal/auth"
	"refract/internal/projects"
	"refract/internal/siwc"
	"refract/internal/store"
	"refract/web"
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

	dataDir := cmp.Or(os.Getenv("REFRACT_DATA_DIR"), "/data")
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

	app := http.NewServeMux()
	a.routes(app, os.Getenv("REFRACT_DEV") == "1")
	pa.routes(app)
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
