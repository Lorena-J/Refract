package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"strings"

	"refract/internal/projects"
)

type projectsAPI struct {
	svc *projects.Service
	tmp string // where uploads are spooled before unzipping
}

func (a *projectsAPI) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects", a.list)
	mux.HandleFunc("POST /api/projects", a.create)
	mux.HandleFunc("POST /api/projects/import", a.importZip)
	mux.HandleFunc("POST /api/projects/import-folder", a.importFolder)
	mux.HandleFunc("PATCH /api/projects/{id}", a.update)
	mux.HandleFunc("DELETE /api/projects/{id}", a.delete)
	mux.HandleFunc("GET /api/projects/{id}/export", a.export)
	mux.HandleFunc("GET /api/projects/{id}/checks", a.checks)
}

func (a *projectsAPI) list(w http.ResponseWriter, r *http.Request) {
	ps, err := a.svc.List(r.Context())
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, ps)
}

func (a *projectsAPI) create(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name string }
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	p, err := a.svc.Create(r.Context(), in.Name)
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, p)
}

// importZip takes the raw zip as the body; the project name comes from ?name=.
func (a *projectsAPI) importZip(w http.ResponseWriter, r *http.Request) {
	// Spool to disk: zip needs random access.
	if err := os.MkdirAll(a.tmp, 0o700); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	f, err := os.CreateTemp(a.tmp, "upload-*.zip")
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := io.Copy(f, http.MaxBytesReader(w, r.Body, a.svc.Limits.MaxBytes)); err != nil {
		httpError(w, errors.New("upload too large or interrupted"), http.StatusRequestEntityTooLarge)
		return
	}
	res, err := a.svc.ImportZip(r.Context(), strings.TrimSuffix(r.URL.Query().Get("name"), ".zip"), f.Name())
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, res)
}

// importFolder takes a multipart upload whose part filenames are relative paths.
func (a *projectsAPI) importFolder(w http.ResponseWriter, r *http.Request) {
	ct, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || ct != "multipart/form-data" {
		httpError(w, errors.New("multipart/form-data required"), http.StatusBadRequest)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.svc.Limits.MaxBytes+10<<20) // + multipart overhead
	mr := multipart.NewReader(r.Body, params["boundary"])
	res, err := a.svc.Import(r.Context(), r.URL.Query().Get("name"), projects.MultipartSource(mr))
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, res)
}

func (a *projectsAPI) update(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name     *string `json:"name"`
		MainFile *string `json:"main_file"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	if in.Name != nil {
		if err := a.svc.Rename(r.Context(), id, *in.Name); err != nil {
			projectError(w, err)
			return
		}
	}
	if in.MainFile != nil {
		if err := a.svc.SetMainFile(r.Context(), id, *in.MainFile); err != nil {
			projectError(w, err)
			return
		}
	}
	p, err := a.svc.Get(r.Context(), id)
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, p)
}

func (a *projectsAPI) delete(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		projectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *projectsAPI) export(w http.ResponseWriter, r *http.Request) {
	p, err := a.svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": p.Name + ".zip"}))
	if err := a.svc.Export(r.Context(), p.ID, w); err != nil {
		// Headers are gone; the truncated zip fails to open, which is the signal.
		panic(http.ErrAbortHandler)
	}
}

func (a *projectsAPI) checks(w http.ResponseWriter, r *http.Request) {
	ws, err := a.svc.Check(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, ws)
}

func projectError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, projects.ErrNotFound):
		httpError(w, err, http.StatusNotFound)
	case errors.Is(err, projects.ErrUnsafePath), errors.Is(err, projects.ErrTooLarge), errors.Is(err, zip.ErrFormat):
		httpError(w, err, http.StatusBadRequest)
	default:
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			httpError(w, errors.New("upload too large"), http.StatusRequestEntityTooLarge)
			return
		}
		slog.Error("projects", "err", err)
		httpError(w, err, http.StatusInternalServerError)
	}
}
