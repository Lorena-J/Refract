package cite

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/ponytojas/Refract/internal/ai"
	"github.com/ponytojas/Refract/internal/arxiv"
	"github.com/ponytojas/Refract/internal/openalex"
)

// script replies to model calls in order and records the inputs.
type script struct {
	replies []string
	inputs  []string
}

func (s *script) Status(context.Context) ai.Status { return ai.Status{} }
func (s *script) Stream(_ context.Context, req ai.Request) (<-chan ai.Event, error) {
	s.inputs = append(s.inputs, string(req.Input[0]))
	ch := make(chan ai.Event, 2)
	ch <- ai.Event{Type: ai.EventText, Text: s.replies[0]}
	ch <- ai.Event{Type: ai.EventCompleted}
	close(ch)
	s.replies = s.replies[1:]
	return ch, nil
}

func service(t *testing.T, replies ...string) (*Service, *script) {
	t.Helper()
	works, _ := os.ReadFile("../openalex/testdata/search.json")
	atom, _ := os.ReadFile("../arxiv/testdata/search.xml")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/arxiv") {
			w.Write(atom)
			return
		}
		w.Write(works)
	}))
	t.Cleanup(srv.Close)
	p := &script{replies: replies}
	return &Service{
		Provider: p,
		Arxiv:    &arxiv.Client{HTTP: srv.Client(), BaseURL: srv.URL + "/arxiv"},
		OpenAlex: &openalex.Client{HTTP: srv.Client(), BaseURL: srv.URL, DOIURL: srv.URL + "/doi"},
	}, p
}

func TestParseID(t *testing.T) {
	for in, want := range map[string]string{
		"1706.03762v7":                                              "arxiv:1706.03762",
		"https://arxiv.org/abs/1706.03762":                          "arxiv:1706.03762",
		"https://arxiv.org/pdf/1706.03762v2.pdf":                    "arxiv:1706.03762",
		"arXiv:hep-th/9901001":                                      "arxiv:hep-th/9901001",
		"10.1038/nature14539":                                       "doi:10.1038/nature14539",
		"https://doi.org/10.1038/NATURE14539":                       "doi:10.1038/nature14539",
		"https://www.nature.com/articles/10.1038/nature14539?foo=1": "doi:10.1038/nature14539",
		"deep learning review":                                      "",
	} {
		got, _ := ParseID(in)
		if got != want {
			t.Errorf("ParseID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearchMergesSources(t *testing.T) {
	s, _ := service(t)
	cs, err := s.Search(context.Background(), "transformer attention")
	if err != nil || len(cs) != 5 { // 3 OpenAlex works + 2 arXiv papers
		t.Fatalf("search = %d, %v", len(cs), err)
	}
	if cs[0].ID != "doi:10.18653/v1/2021.acl-long.103" || cs[4].ID != "arxiv:1706.03762" || cs[4].URL != "https://arxiv.org/abs/1706.03762" {
		t.Errorf("order/ids = %s … %s", cs[0].ID, cs[4].ID)
	}
}

func TestSuggestOnlyPicksReturnedPapers(t *testing.T) {
	s, p := service(t,
		"```json\n{\"queries\": [\"transformer attention translation\", \"attention calibration\"]}\n```",
		`{"picks": [{"n": 5, "reason": "Introduce el Transformer."}, {"n": 99, "reason": "invented"}, {"n": 5, "reason": "dup"}, {"n": 1, "reason": "Calibra la atención."}]}`)
	cs, err := s.Suggest(context.Background(), "m", "low", `Los Transformers se basan en la atención \cite{x}.`)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].ID != "arxiv:1706.03762" || cs[0].Reason != "Introduce el Transformer." || cs[1].ID != "doi:10.18653/v1/2021.acl-long.103" {
		t.Fatalf("picks = %+v", cs)
	}
	if !strings.Contains(p.inputs[1], "[5] Attention Is All You Need (2017)") {
		t.Errorf("ranking input = %.300s", p.inputs[1])
	}
}

func TestPlace(t *testing.T) {
	c := Candidate{Title: "Attention Is All You Need", Year: 2017}
	sel := " Transformers rely on attention. They scale.\n"
	s, _ := service(t, "Transformers rely on attention~\\cite{vaswani}. They scale.")
	if out, placed := s.Place(context.Background(), "m", "", c, "vaswani", sel, "", ""); !placed || out != " Transformers rely on attention~\\cite{vaswani}. They scale.\n" {
		t.Errorf("placed = %v %q", placed, out)
	}
	// A reply that rewords the text is rejected: the citation goes at the end instead.
	s, _ = service(t, "Transformers mostly rely on attention~\\cite{vaswani}. They scale.")
	if out, placed := s.Place(context.Background(), "m", "", c, "vaswani", sel, "", ""); placed || out != " Transformers rely on attention. They scale~\\cite{vaswani}.\n" {
		t.Errorf("fallback = %v %q", placed, out)
	}
}
