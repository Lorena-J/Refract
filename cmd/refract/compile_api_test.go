package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ponytojas/Refract/internal/access"
	"github.com/ponytojas/Refract/internal/auth"
	"github.com/ponytojas/Refract/internal/compile"
	"github.com/ponytojas/Refract/internal/projects"
	"github.com/ponytojas/Refract/internal/store"
)

type busyCompiler struct{}

func (busyCompiler) Compile(context.Context, string, string) (*compile.Result, error) {
	return nil, compile.ErrBusy
}

func TestCompileBusyIs429(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &projects.Service{DB: db, Dir: filepath.Join(dir, "projects"), Limits: projects.DefaultLimits}
	mux := http.NewServeMux()
	(&compileAPI{svc: svc, runner: &compile.Runner{Compiler: busyCompiler{}}}).routes(&router{mux: mux, acc: &access.Checker{DB: db}})

	ctx := auth.WithUser(context.Background(), auth.User{ID: "ana"})
	p, err := svc.Create(ctx, "Paper")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.WriteFile(ctx, p.ID, "main.tex", strings.NewReader(`\documentclass{article}`)); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetMainFile(ctx, p.ID, "main.tex"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/projects/"+p.ID+"/compile", nil).WithContext(ctx))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var body struct{ Error struct{ Kind string } }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Kind != "compile_busy" {
		t.Fatalf("body %s (%v); want kind compile_busy", w.Body, err)
	}
}

// Downloads of the PDF and the log serve only regular files inside the
// project's build folder, and keep ranges and headers.
func TestOutputDownloadsAreConfined(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &projects.Service{DB: db, Dir: filepath.Join(dir, "projects"), Limits: projects.DefaultLimits}
	mux := http.NewServeMux()
	(&compileAPI{svc: svc}).routes(&router{mux: mux, acc: &access.Checker{DB: db}})
	ctx := auth.WithUser(context.Background(), auth.User{ID: "ana"})

	const secret = "SECRET outside the project"
	outside := t.TempDir()
	for _, n := range []string{"main.pdf", "main.log"} {
		if err := os.WriteFile(filepath.Join(outside, n), []byte(secret), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// project returns a project with main.tex as its main file and its build folder.
	project := func(t *testing.T) (string, string) {
		p, err := svc.Create(ctx, "Paper")
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.WriteFile(ctx, p.ID, "main.tex", strings.NewReader(`\documentclass{article}`)); err != nil {
			t.Fatal(err)
		}
		if err := svc.SetMainFile(ctx, p.ID, "main.tex"); err != nil {
			t.Fatal(err)
		}
		build := filepath.Join(svc.Path(p.ID), filepath.FromSlash(compile.BuildDir))
		if err := os.MkdirAll(build, 0o755); err != nil {
			t.Fatal(err)
		}
		return p.ID, build
	}
	get := func(id, route, rng string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/projects/"+id+"/"+route, nil).WithContext(ctx)
		if rng != "" {
			r.Header.Set("Range", rng)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	link := func(t *testing.T, target, at string) {
		if err := os.Symlink(target, at); err != nil {
			t.Skipf("can't make symbolic links here: %v", err)
		}
	}

	t.Run("normal files, ranges and headers", func(t *testing.T) {
		id, build := project(t)
		os.WriteFile(filepath.Join(build, "main.pdf"), []byte("%PDF-1.7 built"), 0o644)
		os.WriteFile(filepath.Join(build, "main.log"), []byte("This is a log"), 0o644)
		w := get(id, "pdf", "")
		if w.Code != http.StatusOK || w.Body.String() != "%PDF-1.7 built" {
			t.Fatalf("pdf: %d %q", w.Code, w.Body)
		}
		for k, v := range map[string]string{"Content-Type": "application/pdf", "Content-Security-Policy": "sandbox", "Cache-Control": "no-store", "X-Content-Type-Options": "nosniff", "Accept-Ranges": "bytes"} {
			if got := w.Header().Get(k); got != v {
				t.Errorf("pdf %s = %q, want %q", k, got, v)
			}
		}
		w = get(id, "pdf", "bytes=0-3")
		if w.Code != http.StatusPartialContent || w.Body.String() != "%PDF" || w.Header().Get("Content-Range") != "bytes 0-3/14" {
			t.Errorf("range: %d %q %q", w.Code, w.Body, w.Header().Get("Content-Range"))
		}
		w = get(id, "log", "")
		if w.Code != http.StatusOK || w.Body.String() != "This is a log" || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("log: %d %q %q", w.Code, w.Body, w.Header().Get("Content-Type"))
		}
	})

	t.Run("missing output", func(t *testing.T) {
		id, build := project(t)
		os.RemoveAll(filepath.Dir(build))
		for _, route := range []string{"pdf", "log"} {
			if w := get(id, route, ""); w.Code != http.StatusNotFound {
				t.Errorf("%s: %d", route, w.Code)
			}
		}
		if _, err := os.Stat(filepath.Dir(build)); !os.IsNotExist(err) {
			t.Error("a download created the build folder")
		}
	})

	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, build string)
	}{
		{"outputs are links", func(t *testing.T, build string) {
			link(t, filepath.Join(outside, "main.pdf"), filepath.Join(build, "main.pdf"))
			link(t, filepath.Join(outside, "main.log"), filepath.Join(build, "main.log"))
		}},
		{"build folder is a link", func(t *testing.T, build string) {
			os.Remove(build)
			link(t, outside, build)
		}},
		{".refract is a link", func(t *testing.T, build string) {
			os.MkdirAll(filepath.Join(outside, "build"), 0o755)
			os.WriteFile(filepath.Join(outside, "build", "main.pdf"), []byte(secret), 0o644)
			os.WriteFile(filepath.Join(outside, "build", "main.log"), []byte(secret), 0o644)
			os.RemoveAll(filepath.Dir(build))
			link(t, outside, filepath.Dir(build))
		}},
		{"outputs replaced by folders", func(t *testing.T, build string) {
			os.Mkdir(filepath.Join(build, "main.pdf"), 0o755)
			os.Mkdir(filepath.Join(build, "main.log"), 0o755)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, build := project(t)
			tc.setup(t, build)
			for _, route := range []string{"pdf", "log"} {
				w := get(id, route, "")
				if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "SECRET") {
					t.Errorf("%s: %d %q; want 404 without outside data", route, w.Code, w.Body)
				}
			}
		})
	}
}
