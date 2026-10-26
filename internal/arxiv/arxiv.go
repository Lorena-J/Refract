// Package arxiv searches the public arXiv API and turns its metadata into
// BibTeX. Entries are built from arXiv's own fields only, never by a model.
package arxiv

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Paper is one arXiv record.
type Paper struct {
	ID         string    `json:"id"`      // 1706.03762 or hep-th/9901001, without version
	Version    string    `json:"version"` // v7
	Title      string    `json:"title"`
	Authors    []string  `json:"authors"`
	Summary    string    `json:"summary"`
	Published  time.Time `json:"published"`
	Category   string    `json:"category"` // primary category, e.g. cs.CL
	DOI        string    `json:"doi,omitempty"`
	JournalRef string    `json:"journal_ref,omitempty"`
}

func (p Paper) URL() string { return "https://arxiv.org/abs/" + p.ID }

// Client calls export.arxiv.org at most once every three seconds, as the
// API's terms of use ask.
type Client struct {
	HTTP    *http.Client
	BaseURL string // https://export.arxiv.org/api/query

	mu   sync.Mutex
	last time.Time
}

const interval = 3 * time.Second

// ErrBadID is returned for anything that is not an arXiv identifier.
var ErrBadID = errors.New("not an arXiv identifier")

var idPattern = regexp.MustCompile(`^(\d{4}\.\d{4,5}|[a-z-]+(\.[A-Z]{2})?/\d{7})(v\d+)?$`)

// IsID reports whether s looks like an arXiv identifier (with or without version).
func IsID(s string) bool { return idPattern.MatchString(strings.TrimSpace(s)) }

// Search runs a free-text search: every word must appear somewhere in the
// record. Field prefixes (ti:, au:, abs:, all:) are passed through as written.
// Stopwords are dropped: arXiv's index has none, so "is" in an AND query
// matches nothing ("attention is all you need" would find no paper).
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Paper, error) {
	var words []string
	for _, w := range strings.Fields(query) {
		if !stopwords[strings.ToLower(w)] {
			words = append(words, w)
		}
	}
	if len(words) == 0 {
		words = strings.Fields(query) // only stopwords: try them anyway
	}
	if len(words) == 0 {
		return nil, errors.New("empty query")
	}
	if len(words) > 12 {
		words = words[:12]
	}
	for i, w := range words {
		if !strings.Contains(w, ":") {
			words[i] = "all:" + w
		}
	}
	q := url.Values{
		"search_query": {strings.Join(words, " AND ")},
		"max_results":  {fmt.Sprint(min(max(limit, 1), 20))},
		"sortBy":       {"relevance"},
	}
	return c.fetch(ctx, q)
}

// Get fetches one paper by identifier.
func (c *Client) Get(ctx context.Context, id string) (*Paper, error) {
	id = strings.TrimSpace(id)
	if !IsID(id) {
		return nil, ErrBadID
	}
	ps, err := c.fetch(ctx, url.Values{"id_list": {id}})
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, fmt.Errorf("arXiv has no paper %s", id)
	}
	return &ps[0], nil
}

func (c *Client) fetch(ctx context.Context, q url.Values) ([]Paper, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Refract (https://github.com/ponytojas/Refract)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("arXiv is unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arXiv answered HTTP %d", resp.StatusCode)
	}
	return parse(io.LimitReader(resp.Body, 8<<20))
}

func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := interval - time.Since(c.last); d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.last = time.Now()
	return nil
}

type feed struct {
	Entries []struct {
		ID         string    `xml:"id"`
		Title      string    `xml:"title"`
		Summary    string    `xml:"summary"`
		Published  time.Time `xml:"published"`
		Authors    []string  `xml:"author>name"`
		DOI        string    `xml:"http://arxiv.org/schemas/atom doi"`
		JournalRef string    `xml:"http://arxiv.org/schemas/atom journal_ref"`
		Primary    struct {
			Term string `xml:"term,attr"`
		} `xml:"http://arxiv.org/schemas/atom primary_category"`
	} `xml:"entry"`
}

var absID = regexp.MustCompile(`/abs/(.+?)(v\d+)?$`)

func parse(r io.Reader) ([]Paper, error) {
	var f feed
	if err := xml.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("unexpected answer from arXiv: %w", err)
	}
	out := []Paper{}
	for _, e := range f.Entries {
		m := absID.FindStringSubmatch(e.ID)
		if m == nil {
			continue // the API reports errors as an entry without an /abs/ id
		}
		out = append(out, Paper{
			ID:         m[1],
			Version:    m[2],
			Title:      oneLine(e.Title),
			Authors:    e.Authors,
			Summary:    oneLine(e.Summary),
			Published:  e.Published,
			Category:   e.Primary.Term,
			DOI:        strings.TrimSpace(e.DOI),
			JournalRef: oneLine(e.JournalRef),
		})
	}
	return out, nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
