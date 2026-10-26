// Package cite finds real papers for a passage and places citations in it.
// The model only proposes search queries, picks among papers the search
// engines returned, and positions \cite: bibliography entries always come
// from the sources' metadata (see refs), and a placement that changes
// anything but citations is rejected.
package cite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/ponytojas/Refract/internal/ai"
	"github.com/ponytojas/Refract/internal/arxiv"
	"github.com/ponytojas/Refract/internal/openalex"
	"github.com/ponytojas/Refract/internal/quick"
	"github.com/ponytojas/Refract/internal/refs"
)

// Candidate is a paper offered to the user.
type Candidate struct {
	ID       string   `json:"id"` // "doi:10.1038/nature14539" or "arxiv:1706.03762"
	Title    string   `json:"title"`
	Authors  []string `json:"authors"`
	Year     int      `json:"year"`
	Venue    string   `json:"venue"`
	Abstract string   `json:"abstract"`
	CitedBy  int      `json:"cited_by"`
	URL      string   `json:"url"`
	Reason   string   `json:"reason,omitempty"` // why it supports the passage (Suggest only)
}

type Service struct {
	Provider ai.Provider
	Arxiv    *arxiv.Client
	OpenAlex *openalex.Client
}

var (
	arxivURL = regexp.MustCompile(`arxiv\.org/(?:abs|pdf)/([^\s?#]+?)(?:\.pdf)?(?:[?#].*)?$`)
	doiInURL = regexp.MustCompile(`10\.\d{4,9}/[^\s"<>?#]+`)
)

// ParseID recognises what a user pastes for a paper: an arXiv id or URL, a
// DOI, or a URL with a DOI in it. It returns "arxiv:<id>" or "doi:<doi>".
func ParseID(s string) (string, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "arXiv:"), "arxiv:")
	if m := arxivURL.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if arxiv.IsID(s) {
		return "arxiv:" + regexp.MustCompile(`v\d+$`).ReplaceAllString(s, ""), true
	}
	if d := openalex.NormalizeDOI(s); d != "" {
		return "doi:" + d, true
	}
	if m := doiInURL.FindString(s); m != "" && strings.Contains(s, "/") {
		if d := openalex.NormalizeDOI(strings.TrimRight(m, ".,;)")); d != "" {
			return "doi:" + d, true
		}
	}
	return "", false
}

func fromWork(w openalex.Work) Candidate {
	url := "https://openalex.org/" + w.ID
	if w.DOI != "" {
		url = "https://doi.org/" + w.DOI
	}
	return Candidate{ID: "doi:" + w.DOI, Title: w.Title, Authors: w.Authors, Year: w.Year, Venue: w.Venue, Abstract: w.Abstract, CitedBy: w.CitedBy, URL: url}
}

func fromPaper(p arxiv.Paper) Candidate {
	return Candidate{ID: "arxiv:" + p.ID, Title: p.Title, Authors: p.Authors, Year: p.Published.Year(), Venue: "arXiv " + p.Category, Abstract: p.Summary, URL: p.URL()}
}

// Resolve fetches a paper's metadata by id ("doi:…" or "arxiv:…").
func (s *Service) Resolve(ctx context.Context, id string) (refs.Ref, Candidate, error) {
	switch {
	case strings.HasPrefix(id, "arxiv:"):
		p, err := s.Arxiv.Get(ctx, strings.TrimPrefix(id, "arxiv:"))
		if err != nil {
			return refs.Ref{}, Candidate{}, err
		}
		return p.Ref(), fromPaper(*p), nil
	case strings.HasPrefix(id, "doi:"):
		w, err := s.OpenAlex.ByDOI(ctx, strings.TrimPrefix(id, "doi:"))
		if err != nil {
			return refs.Ref{}, Candidate{}, err
		}
		return s.OpenAlex.Ref(ctx, *w), fromWork(*w), nil
	}
	return refs.Ref{}, Candidate{}, fmt.Errorf("not a DOI or arXiv id: %q", id)
}

// Search takes free text, or an id/URL for one paper.
func (s *Service) Search(ctx context.Context, q string) ([]Candidate, error) {
	if id, ok := ParseID(q); ok {
		_, c, err := s.Resolve(ctx, id)
		if err != nil {
			return nil, err
		}
		return []Candidate{c}, nil
	}
	return s.gather(ctx, []string{q}, 10)
}

// gather runs the queries on OpenAlex (and the first on arXiv) and merges
// the results, OpenAlex first, without duplicates.
func (s *Service) gather(ctx context.Context, queries []string, per int) ([]Candidate, error) {
	results := make([][]Candidate, len(queries)+1)
	errs := make([]error, len(queries)+1)
	var wg sync.WaitGroup
	for i, q := range queries {
		wg.Go(func() {
			ws, err := s.OpenAlex.Search(ctx, q, per)
			errs[i] = err
			for _, w := range ws {
				if w.DOI != "" { // only citable works
					results[i] = append(results[i], fromWork(w))
				}
			}
		})
	}
	wg.Go(func() {
		ps, err := s.Arxiv.Search(ctx, queries[0], 5)
		errs[len(queries)] = err
		for _, p := range ps {
			results[len(queries)] = append(results[len(queries)], fromPaper(p))
		}
	})
	wg.Wait()

	seen := map[string]bool{}
	out := []Candidate{}
	for _, rs := range results {
		for _, c := range rs {
			title := strings.ToLower(strings.Join(strings.Fields(c.Title), " "))
			if seen[c.ID] || seen[title] {
				continue
			}
			seen[c.ID], seen[title] = true, true
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		if err := errors.Join(errs...); err != nil {
			return nil, err
		}
	}
	return out, nil
}

const queriesPrompt = `You help find published scientific papers that support a passage of a LaTeX document.
Write up to 3 search queries for a scholarly search engine: short keyword phrases in English (3 to 8 words), each aimed at a different claim in the passage. Ignore LaTeX markup.
Reply with JSON only: {"queries": ["..."]}`

const rankPrompt = `You check which papers support a passage of a LaTeX document. You get the passage and numbered candidate papers with their abstracts.
Pick at most 5 candidates whose own findings directly support a claim made in the passage, best first. Judge each paper by what it found, not by background it mentions; never pick one whose results contradict or do not back the claim. An empty list is a valid answer.
For each pick, write one short sentence saying which claim it supports, in the language the passage is written in.
Reply with JSON only: {"picks": [{"n": 1, "reason": "..."}]}`

// Suggest finds papers that support text, best first, each with a reason.
func (s *Service) Suggest(ctx context.Context, model, effort, text string) ([]Candidate, error) {
	var q struct {
		Queries []string `json:"queries"`
	}
	reply, err := s.complete(ctx, model, effort, queriesPrompt, text)
	if err != nil {
		return nil, err
	}
	if decode(reply, &q); len(q.Queries) == 0 {
		q.Queries = []string{strings.Join(firstWords(text, 10), " ")}
	}
	if len(q.Queries) > 3 {
		q.Queries = q.Queries[:3]
	}
	cands, err := s.gather(ctx, q.Queries, 8)
	if err != nil || len(cands) == 0 {
		return cands, err
	}
	if len(cands) > 24 {
		cands = cands[:24]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Passage:\n%s\n\nCandidates:\n", text)
	for i, c := range cands {
		abstract := c.Abstract
		if len(abstract) > 700 {
			abstract = abstract[:700] + "…"
		}
		if abstract == "" {
			abstract = "(no abstract)"
		}
		fmt.Fprintf(&b, "[%d] %s (%d). %s. Abstract: %s\n", i+1, c.Title, c.Year, c.Venue, abstract)
	}
	var picks struct {
		Picks []struct {
			N      int    `json:"n"`
			Reason string `json:"reason"`
		} `json:"picks"`
	}
	if reply, err = s.complete(ctx, model, effort, rankPrompt, b.String()); err != nil {
		return nil, err
	}
	if err := decode(reply, &picks); err != nil {
		return nil, fmt.Errorf("the model's ranking was unreadable: %w", err)
	}
	out := []Candidate{}
	used := map[int]bool{}
	for _, p := range picks.Picks {
		if p.N < 1 || p.N > len(cands) || used[p.N] || len(out) == 5 {
			continue // only papers the search returned, each once
		}
		used[p.N] = true
		c := cands[p.N-1]
		c.Reason = strings.TrimSpace(p.Reason)
		out = append(out, c)
	}
	return out, nil
}

const placePrompt = `You add one citation to a fragment of a LaTeX document. The fragment is between <selection> tags; the text around it is context. The cited work is described below.
Insert \cite{%[1]s} right after the claim(s) in the fragment that the work supports, usually just before the sentence's final punctuation and written as ~\cite{%[1]s}. If the fragment already cites other works at that spot, add %[1]s inside that \cite{...} instead.
Change nothing else, not a single character. Reply with the whole fragment only: no explanations, no code fences, no tags.

Cited work: %[2]s`

// Place returns selection with \cite{key} where the model put it, if the
// model changed nothing else; otherwise (or on error) at the selection's end.
// placed reports which.
func (s *Service) Place(ctx context.Context, model, effort string, c Candidate, key, selection, before, after string) (text string, placed bool) {
	work := fmt.Sprintf("%s (%d). %s", c.Title, c.Year, c.Abstract)
	if len(work) > 1200 {
		work = work[:1200] + "…"
	}
	input := fmt.Sprintf("Context before the fragment:\n%s<selection>%s</selection>%s\n(context after the fragment ends here)", before, selection, after)
	reply, err := s.complete(ctx, model, effort, fmt.Sprintf(placePrompt, key, work), input)
	if err == nil {
		if out := quick.Clean(selection, reply); refs.OnlyAddsCites(selection, out, key) {
			return out, true
		}
	}
	return refs.AppendCite(selection, key), false
}

func (s *Service) complete(ctx context.Context, model, effort, instructions, input string) (string, error) {
	req := ai.Request{Model: model, Instructions: instructions, Input: []json.RawMessage{ai.UserMessage(input)}}
	if effort != "" {
		req.Reasoning = &ai.Reasoning{Effort: effort}
	}
	return ai.Complete(ctx, s.Provider, req)
}

// decode reads the JSON object in a reply, tolerating code fences or prose around it.
func decode(reply string, v any) error {
	i, j := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if i < 0 || j < i {
		return errors.New("no JSON object")
	}
	return json.Unmarshal([]byte(reply[i:j+1]), v)
}

func firstWords(s string, n int) []string {
	var out []string
	for _, w := range strings.Fields(regexp.MustCompile(`\\[a-zA-Z]+|[{}$~]`).ReplaceAllString(s, " ")) {
		if len(out) == n {
			break
		}
		out = append(out, w)
	}
	return out
}
