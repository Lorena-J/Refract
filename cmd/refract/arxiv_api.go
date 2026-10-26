package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/ponytojas/Refract/internal/arxiv"
	"github.com/ponytojas/Refract/internal/projects"
)

type arxivAPI struct {
	client   *arxiv.Client
	projects *projects.Service
}

func (a *arxivAPI) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/arxiv/search", a.search)
	mux.HandleFunc("POST /api/projects/{id}/cite", a.cite)
}

// search takes free text or an arXiv id.
func (a *arxivAPI) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" || len(q) > 300 {
		httpError(w, errors.New("query must be 1 to 300 characters"), http.StatusBadRequest)
		return
	}
	if arxiv.IsID(q) {
		p, err := a.client.Get(r.Context(), q)
		if err != nil {
			httpError(w, err, http.StatusBadGateway)
			return
		}
		writeJSON(w, []arxiv.Paper{*p})
		return
	}
	ps, err := a.client.Search(r.Context(), q, 10)
	if err != nil {
		httpError(w, err, http.StatusBadGateway)
		return
	}
	writeJSON(w, ps)
}

// cite adds a paper to the project's bibliography. Only the id comes from the
// client: the entry is built from what arXiv returns for it.
func (a *arxivAPI) cite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	p, err := a.projects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	paper, err := a.client.Get(r.Context(), in.ID)
	if errors.Is(err, arxiv.ErrBadID) {
		httpError(w, err, http.StatusBadRequest)
		return
	} else if err != nil {
		httpError(w, err, http.StatusBadGateway)
		return
	}
	root, err := os.OpenRoot(a.projects.Path(p.ID))
	if err != nil {
		projectError(w, err)
		return
	}
	defer root.Close()
	c, err := arxiv.Cite(root, p.MainFile, *paper)
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, c)
}
