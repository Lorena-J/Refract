package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"refract/internal/compile"
	"refract/internal/projects"
)

type compileAPI struct {
	svc    *projects.Service
	runner *compile.Runner
}

func (a *compileAPI) routes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/projects/{id}/compile", a.compile)
	mux.HandleFunc("GET /api/projects/{id}/pdf", a.output(".pdf", "application/pdf"))
	mux.HandleFunc("GET /api/projects/{id}/log", a.output(".log", "text/plain; charset=utf-8"))
}

// compile builds the main file. A successful build triggered by the user is
// a checkpoint commit (the AI's compiles in M6 call the Compiler directly).
func (a *compileAPI) compile(w http.ResponseWriter, r *http.Request) {
	p, err := a.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	if p.MainFile == "" {
		httpError(w, errors.New("choose the main file first"), http.StatusBadRequest)
		return
	}
	res, err := a.runner.Compile(r.Context(), p.ID, a.svc.Path(p.ID), p.MainFile)
	if err != nil {
		slog.Error("compile", "project", p.ID, "err", err)
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	committed := false
	if res.OK {
		if committed, err = a.svc.Checkpoint(r.Context(), p.ID, "checkpoint: compile"); err != nil {
			slog.Error("checkpoint", "project", p.ID, "err", err)
		}
	}
	writeJSON(w, struct {
		*compile.Result
		Checkpoint bool `json:"checkpoint"`
	}{res, committed})
}

// output serves the last build's PDF or log (Range requests work: PDF.js uses them).
func (a *compileAPI) output(ext, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := a.svc.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			projectError(w, err)
			return
		}
		base := path.Base(p.MainFile)
		name := strings.TrimSuffix(base, path.Ext(base)) + ext
		f, err := os.Open(filepath.Join(a.svc.Path(p.ID), filepath.FromSlash(compile.BuildDir), name))
		if err != nil {
			httpError(w, errors.New("not built yet"), http.StatusNotFound)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			httpError(w, err, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "sandbox")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, name, info.ModTime(), f)
	}
}
