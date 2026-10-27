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
	"github.com/ponytojas/Refract/internal/arxiv"
	"github.com/ponytojas/Refract/internal/assistant"
	"github.com/ponytojas/Refract/internal/auth"
	"github.com/ponytojas/Refract/internal/cite"
	"github.com/ponytojas/Refract/internal/compile"
	"github.com/ponytojas/Refract/internal/github"
	"github.com/ponytojas/Refract/internal/openalex"
	"github.com/ponytojas/Refract/internal/projects"
	"github.com/ponytojas/Refract/internal/siwc"
	"github.com/ponytojas/Refract/internal/store"
	"github.com/ponytojas/Refract/internal/users"
	"github.com/ponytojas/Refract/web"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

const usage = `Refract, a private AI-assisted LaTeX workspace.

Usage:
  refract            run the server (configured with REFRACT_* environment variables)
  refract login      sign in with ChatGPT on this computer and write a credential file
                     to upload in Settings → Assistant (run "refract login -h" for options)
  refract version    print the version
`

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println("refract", version)
			return
		case "help", "--help", "-h":
			fmt.Print(usage)
			return
		}
	}
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
		base:  session,
		model: os.Getenv("REFRACT_MODEL_AGENT"),
		quick: os.Getenv("REFRACT_MODEL_QUICK"),
	}

	pa := &projectsAPI{
		svc: &projects.Service{DB: db, Dir: filepath.Join(dataDir, "projects"), Limits: projects.DefaultLimits, Key: key},
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

	outbound := &http.Client{Timeout: 30 * time.Second}
	papers := &cite.Service{
		Provider: nil, // set per request: each user's own (see papersAPI)
		Arxiv:    &arxiv.Client{HTTP: outbound, BaseURL: "https://export.arxiv.org/api/query"},
		OpenAlex: &openalex.Client{HTTP: outbound, BaseURL: "https://api.openalex.org", DOIURL: "https://doi.org", APIKey: os.Getenv("REFRACT_OPENALEX_API_KEY")},
	}
	asa := &assistantAPI{svc: &assistant.Service{
		DB:       db,
		Projects: pa.svc,
		Agent: agent.Agent{
			Compiler:      ca.runner.Compiler,
			MaxIterations: 25,
			Papers:        papers,
			Timeout:       15 * time.Minute,
		},
		Provider: a.providerFor,
		ModelFor: func(ctx context.Context, want assistant.Choice) (assistant.Choice, error) {
			return a.choose(ctx, want, a.model)
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
	ra := &remoteAPI{svc: pa.svc, db: db, key: key}
	if id := os.Getenv("REFRACT_GITHUB_CLIENT_ID"); id != "" {
		ra.github = &github.Client{HTTP: outbound, ClientID: id, Web: "https://github.com", API: "https://api.github.com"}
	}
	pa.svc.OnCommit = ra.AutoPush
	pa.svc.GitHubToken = ra.Token
	ra.routes(app)
	(&papersAPI{papers: papers, projects: pa.svc, provider: a.providerFor, choose: func(ctx context.Context, want assistant.Choice) (assistant.Choice, error) {
		return a.choose(ctx, want, a.quick)
	}}).routes(app)
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
	people := &users.Service{DB: db, Key: key}
	mux.Handle("/", guard(people.Middleware(auth.RequireCSRFHeader(app))))

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
		return auth.Unauthenticated, nil // one local user
	default:
		return nil, fmt.Errorf("REFRACT_AUTH=%q: want cloudflare or none", mode)
	}
}
