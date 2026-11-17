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
	"github.com/ponytojas/Refract/internal/arxiv"
	"github.com/ponytojas/Refract/internal/auth"
	"github.com/ponytojas/Refract/internal/cite"
	"github.com/ponytojas/Refract/internal/openalex"
	"github.com/ponytojas/Refract/internal/projects"
	"github.com/ponytojas/Refract/internal/refs"
	"github.com/ponytojas/Refract/internal/store"
)

func TestCitationChecksAPI(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &projects.Service{DB: db, Dir: filepath.Join(dir, "projects"), Limits: projects.DefaultLimits}
	ctx := auth.WithUser(context.Background(), auth.User{ID: "owner"})
	p, err := svc.Create(ctx, "Checks")
	if err != nil {
		t.Fatal(err)
	}
	bib := "@article{local, title={Old}, doi={10.1000/ok}, note={Keep this}}\n"
	for file, text := range map[string]string{"main.tex": `\bibliography{refs}\cite{local,missing}`, "refs.bib": bib} {
		if err := svc.WriteFile(ctx, p.ID, file, strings.NewReader(text)); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.SetMainFile(ctx, p.ID, "main.tex"); err != nil {
		t.Fatal(err)
	}
	var calls int
	atom, err := os.ReadFile("../../internal/arxiv/testdata/search.xml")
	if err != nil {
		t.Fatal(err)
	}
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Path {
		case "/arxiv":
			w.Write(atom)
		case "/10.1000/ok":
			w.Write([]byte(`@article{remote, title={Real title}, author={A. Author}, year={2024}, journal={Journal}, doi={10.1000/ok}}`))
		case "/10.1000/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/10.1000/rate":
			w.WriteHeader(http.StatusTooManyRequests)
		case "/10.1000/html":
			w.Write([]byte(`<html>No BibTeX</html>`))
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer source.Close()
	a := &papersAPI{projects: svc, papers: &cite.Service{OpenAlex: &openalex.Client{HTTP: source.Client(), DOIURL: source.URL}, Arxiv: &arxiv.Client{HTTP: source.Client(), BaseURL: source.URL + "/arxiv"}}}
	mux := http.NewServeMux()
	a.routes(&router{mux: mux, acc: &access.Checker{DB: db}})
	call := func(user, method, endpoint, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/projects/"+p.ID+"/citation-checks"+endpoint, strings.NewReader(body)).WithContext(auth.WithUser(context.Background(), auth.User{ID: user}))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, endpoint := range []struct{ method, path, body string }{{"GET", "", ""}, {"POST", "/doi", `{"doi":"10.1000/ok"}`}, {"POST", "/fix", `{"file":"refs.bib","key":"local"}`}} {
		if w := call("stranger", endpoint.method, endpoint.path, endpoint.body); w.Code != 404 {
			t.Fatalf("non-owner: %d %s", w.Code, w.Body)
		}
	}
	if calls != 0 {
		t.Fatal("unauthorised request contacted source")
	}
	w := call("owner", "GET", "", "")
	var report refs.CitationReport
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &report) != nil || len(report.Findings) != 2 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if calls != 0 {
		t.Fatal("local checks contacted source")
	}
	for doi, status := range map[string]string{"10.1000/ok": "resolved", "10.1000/missing": "unresolved-doi", "10.1000/rate": "doi-unavailable", "10.1000/html": "doi-unavailable", "bad": "invalid-doi"} {
		w := call("owner", "POST", "/doi", `{"doi":"`+doi+`"}`)
		var got map[string]string
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &got) != nil || got["status"] != status {
			t.Fatalf("%s: %d %s", doi, w.Code, w.Body)
		}
	}
	w = call("owner", "POST", "/fix", `{"file":"refs.bib","key":"local"}`)
	var fix refs.MetadataFix
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &fix) != nil || fix.Before != bib || !strings.Contains(fix.After, "@article{local,") || !strings.Contains(fix.After, "note = {Keep this}") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	current, _, err := svc.ReadFile(ctx, p.ID, "refs.bib")
	if err != nil || string(current) != bib {
		t.Fatalf("proposal changed live bibliography: %s %v", current, err)
	}
	if w := call("owner", "POST", "/fix", `{"file":"../refs.bib","key":"local"}`); w.Code != 400 {
		t.Fatalf("unsafe path: %d", w.Code)
	}
	preprint := `@misc{preprint, title={Old title}, eprint={arXiv:1706.03762}, archivePrefix={arXiv}}`
	if err := svc.WriteFile(ctx, p.ID, "preprints.bib", strings.NewReader(preprint)); err != nil {
		t.Fatal(err)
	}
	w = call("owner", "POST", "/fix", `{"file":"preprints.bib","key":"preprint"}`)
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &fix) != nil || fix.Before != preprint || !strings.Contains(fix.After, "@misc{preprint,") || !strings.Contains(fix.After, "Attention Is All You Need") {
		t.Fatalf("arxiv: %d %s", w.Code, w.Body)
	}
	// An ambiguous key is refused before contacting the metadata source.
	if err := svc.WriteFile(ctx, p.ID, "refs.bib", strings.NewReader(bib+bib)); err != nil {
		t.Fatal(err)
	}
	beforeCalls := calls
	w = call("owner", "POST", "/fix", `{"file":"refs.bib","key":"local"}`)
	if w.Code != http.StatusConflict || calls != beforeCalls {
		t.Fatalf("ambiguous key: %d, source calls %d -> %d", w.Code, beforeCalls, calls)
	}
}
