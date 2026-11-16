package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ponytojas/Refract/internal/arxiv"
	"github.com/ponytojas/Refract/internal/openalex"
	"github.com/ponytojas/Refract/internal/projects"
	"github.com/ponytojas/Refract/internal/refs"
)

func (a *papersAPI) citationChecks(w http.ResponseWriter, r *http.Request) {
	p, err := a.projects.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		projectError(w, err)
		return
	}
	root, err := os.OpenRoot(a.projects.Path(p.ID))
	if err != nil {
		projectError(w, err)
		return
	}
	defer root.Close()
	report, err := refs.CheckCitations(root.FS(), p.MainFile)
	if err != nil {
		projectError(w, err)
		return
	}
	writeJSON(w, report)
}

// A single DOI per request lets the client report progress, cancel on close,
// and avoid a large bibliography monopolising the server for minutes.
func (a *papersAPI) checkDOI(w http.ResponseWriter, r *http.Request) {
	var in struct {
		DOI string `json:"doi"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	status := "invalid-doi"
	if doi := openalex.NormalizeDOI(in.DOI); doi != "" {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		_, err := a.papers.OpenAlex.DOIBibTeX(ctx, doi)
		switch {
		case err == nil:
			status = "resolved"
		case errors.Is(err, openalex.ErrDOINotFound):
			status = "unresolved-doi"
		default:
			status = "doi-unavailable"
		}
	}
	writeJSON(w, map[string]string{"status": status})
}

func (a *papersAPI) citationFix(w http.ResponseWriter, r *http.Request) {
	var in struct {
		File string `json:"file"`
		Key  string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		httpError(w, err, http.StatusBadRequest)
		return
	}
	if !strings.HasSuffix(strings.ToLower(in.File), ".bib") || !refs.ValidKey(in.Key) {
		httpError(w, errors.New("invalid bibliography or key"), http.StatusBadRequest)
		return
	}
	before, _, err := a.projects.ReadFile(r.Context(), r.PathValue("id"), in.File)
	if err != nil {
		projectError(w, err)
		return
	}
	if len(before) > 8<<20 {
		httpError(w, projects.ErrTooLarge, http.StatusBadRequest)
		return
	}
	var matches []refs.Record
	for _, entry := range refs.Records(string(before)) {
		if entry.Key == in.Key {
			matches = append(matches, entry)
		}
	}
	if len(matches) != 1 {
		httpError(w, errors.New("entry changed or citation key is ambiguous; run the checks again"), http.StatusConflict)
		return
	}
	fields := matches[0].Fields
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var metadata string
	if doi := fields["doi"]; doi != "" {
		metadata, err = a.papers.OpenAlex.DOIBibTeX(ctx, doi)
	} else if id := fields["eprint"]; arxiv.IsID(id) && (strings.EqualFold(fields["archiveprefix"], "arxiv") || strings.EqualFold(fields["eprinttype"], "arxiv")) {
		var paper *arxiv.Paper
		paper, err = a.papers.Arxiv.Get(ctx, id)
		if err == nil {
			metadata = paper.Ref().BibTeX(in.Key)
		}
	} else {
		httpError(w, errors.New("this entry has no DOI or arXiv identifier"), http.StatusBadRequest)
		return
	}
	if err != nil {
		httpError(w, errors.New("could not retrieve bibliography metadata from the source"), http.StatusBadGateway)
		return
	}
	fix, err := refs.ProposeMetadata(in.File, string(before), in.Key, metadata)
	if err != nil {
		httpError(w, err, http.StatusBadGateway)
		return
	}
	writeJSON(w, fix)
}
