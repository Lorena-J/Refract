package arxiv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func fixture(t *testing.T) []Paper {
	t.Helper()
	f, err := os.Open("testdata/search.xml") // a real answer from export.arxiv.org
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ps, err := parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return ps
}

func TestParse(t *testing.T) {
	ps := fixture(t)
	if len(ps) != 2 {
		t.Fatalf("papers = %d", len(ps))
	}
	p := ps[1]
	if p.ID != "1706.03762" || p.Version != "v7" || p.Title != "Attention Is All You Need" || p.Category != "cs.CL" || p.Published.Year() != 2017 {
		t.Errorf("paper = %+v", p)
	}
	if len(p.Authors) != 8 || p.Authors[0] != "Ashish Vaswani" || strings.Contains(p.Summary, "\n") {
		t.Errorf("authors/summary = %v / %q", p.Authors, p.Summary[:40])
	}
}

func TestBibTeX(t *testing.T) {
	p := fixture(t)[1]
	if k := Key(p); k != "vaswani2017attention" {
		t.Errorf("key = %q", k)
	}
	b := BibTeX(p, "k")
	for _, want := range []string{"@misc{k,\n", "title = {Attention Is All You Need}", "author = {Ashish Vaswani and Noam Shazeer", "year = {2017}", "eprint = {1706.03762}", "archivePrefix = {arXiv}", "primaryClass = {cs.CL}", "url = {https://arxiv.org/abs/1706.03762}"} {
		if !strings.Contains(b, want) {
			t.Errorf("missing %q in\n%s", want, b)
		}
	}
	if got := escape(`R&D at 50% & more, \& kept`); got != `R\&D at 50\% \& more, \& kept` {
		t.Errorf("escape = %q", got)
	}
	if k := Key(Paper{Authors: []string{"José Ñúñez"}, Title: "On the Theory", Published: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}); k != "nunez2020theory" {
		t.Errorf("accented key = %q", k)
	}
}

func TestAddEntry(t *testing.T) {
	p := fixture(t)[1]
	bib, key, added := AddEntry("", p)
	if !added || key != "vaswani2017attention" || !strings.HasPrefix(bib, "@misc{vaswani2017attention,") {
		t.Fatalf("first add = %q %v\n%s", key, added, bib)
	}
	// Same paper again (any version): not duplicated, existing key returned.
	again, key2, added := AddEntry(bib+"@book{other, title={X}}\n", p)
	if added || key2 != key || strings.Count(again, "1706.03762}") != 2 { // eprint + url
		t.Errorf("re-add = %q %v", key2, added)
	}
	// Key taken by another paper: suffix.
	taken := "@article{vaswani2017attention,\n  title = {Something else},\n}"
	out, key3, added := AddEntry(taken, p)
	if !added || key3 != "vaswani2017attentiona" || !strings.HasPrefix(out, taken+"\n\n@misc{vaswani2017attentiona,") {
		t.Errorf("collision = %q\n%s", key3, out)
	}
}

func TestBibTarget(t *testing.T) {
	for name, c := range map[string]struct {
		files      fstest.MapFS
		want       string
		needsSetup bool
	}{
		"bibliography in main": {fstest.MapFS{"main.tex": {Data: []byte(`\bibliography{refs,extra}`)}, "other.bib": {}}, "refs.bib", false},
		"biblatex resource":    {fstest.MapFS{"main.tex": {Data: []byte(`\addbibresource[label=x]{bib/library.bib}`)}}, "bib/library.bib", false},
		"main file first":      {fstest.MapFS{"a.tex": {Data: []byte(`\bibliography{a}`)}, "main.tex": {Data: []byte(`\bibliography{m}`)}}, "m.bib", false},
		"only a .bib":          {fstest.MapFS{"main.tex": {Data: []byte(`x`)}, "lit.bib": {}}, "lit.bib", true},
		"nothing yet":          {fstest.MapFS{"main.tex": {Data: []byte(`x`)}}, "references.bib", true},
	} {
		got, setup := BibTarget(c.files, "main.tex")
		if got != c.want || setup != c.needsSetup {
			t.Errorf("%s: %q %v, want %q %v", name, got, setup, c.want, c.needsSetup)
		}
	}
}

func TestClient(t *testing.T) {
	body, _ := os.ReadFile("testdata/search.xml")
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		w.Write(body)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseURL: srv.URL}
	ps, err := c.Search(context.Background(), "attention is all you need ti:transformer", 5)
	if err != nil || len(ps) != 2 {
		t.Fatalf("search = %v, %v", ps, err)
	}
	if !strings.Contains(queries[0], "search_query=all%3Aattention+AND+all%3Aneed+AND+ti%3Atransformer") || !strings.Contains(queries[0], "max_results=5") {
		t.Errorf("query = %s", queries[0])
	}
	if _, err := c.Get(context.Background(), "not an id"); err != ErrBadID {
		t.Errorf("bad id: %v", err)
	}
	for _, id := range []string{"1706.03762", "1706.03762v7", "2401.12345", "hep-th/9901001", "math.GT/0309136v2"} {
		if !IsID(id) {
			t.Errorf("IsID(%q) = false", id)
		}
	}
}
