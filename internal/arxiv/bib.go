package arxiv

import (
	"fmt"
	"strings"

	"github.com/ponytojas/Refract/internal/refs"
)

// Ref makes the paper citable, with the BibTeX format arXiv itself exports.
func (p Paper) Ref() refs.Ref {
	return refs.Ref{Title: p.Title, Authors: p.Authors, Year: p.Published.Year(), Eprint: p.ID, DOI: strings.ToLower(p.DOI), BibTeX: func(key string) string { return BibTeX(p, key) }}
}

// BibTeX renders a paper in the format arXiv itself exports.
func BibTeX(p Paper, key string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "@misc{%s,\n", key)
	field := func(name, value string) {
		if value != "" {
			fmt.Fprintf(&b, "  %s = {%s},\n", name, value)
		}
	}
	field("title", refs.Escape(p.Title))
	field("author", refs.Escape(strings.Join(p.Authors, " and ")))
	field("year", fmt.Sprint(p.Published.Year()))
	field("eprint", p.ID)
	field("archivePrefix", "arXiv")
	field("primaryClass", p.Category)
	field("doi", p.DOI)
	field("url", p.URL())
	b.WriteString("}\n")
	return b.String()
}
