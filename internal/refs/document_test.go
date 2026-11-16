package refs

import (
	"reflect"
	"testing"
	"testing/fstest"
)

func TestCitationChecksOnlySelectedDocument(t *testing.T) {
	fsys := fstest.MapFS{
		"paper/main.tex": {Data: []byte(`\bibliography{refs,../shared/extra}
\input{chap/intro} \include{results}
% \input{commented}
\begin{verbatim}\input{literal}\bibliography{fake}\end{verbatim}
\verb|\input{inline}|
\\input{escaped}
\end{document}
\cite{after-end}\input{ignored}`)},
		"paper/chap/intro.tex":       {Data: []byte(`\cite{shared}\input{chap/method}\addbibresource{refs.bib}`)},
		"paper/chap/method.tex":      {Data: []byte(`\cite{extra}\input{chap/intro}`)},
		"paper/results.tex":          {Data: []byte(`\cite{shared}`)},
		"paper/refs.bib":             {Data: []byte(`@misc{shared,title={Shared work}}`)},
		"shared/extra.bib":           {Data: []byte(`@misc{extra,title={Another work}}`)},
		"paper/main_2.tex":           {Data: []byte(`\cite{freund2007}\bibliography{unused}`)},
		"paper/unused.bib":           {Data: []byte(`@misc{shared,title={Unloaded duplicate}}`)},
		"experiment1/references.bib": {Data: []byte(`@misc{shared,title={Independent duplicate}}`)},
	}
	r, err := CheckCitations(fsys, "paper/main.tex")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Findings) != 0 {
		t.Fatalf("unrelated draft, bibliography or literal polluted findings: %+v", r.Findings)
	}
	if len(r.Entries) != 2 {
		t.Fatalf("loaded entries: %+v", r.Entries)
	}
	// An actually loaded duplicate is still an error, even if that .bib was
	// already mentioned in another source (which must not load it twice).
	fsys["paper/refs.bib"].Data = []byte(`@misc{shared,title={Shared work}} @misc{shared,title={Duplicate}}`)
	r, err = CheckCitations(fsys, "paper/main.tex")
	if err != nil || len(r.Findings) != 1 || r.Findings[0].Kind != "duplicate-key" {
		t.Fatalf("%+v %v", r.Findings, err)
	}
	// The chosen main file controls the report; switching drafts is explicit.
	r, err = CheckCitations(fsys, "paper/main_2.tex")
	if err != nil || len(r.Findings) != 2 || r.Findings[0].Key != "freund2007" || r.Findings[0].Kind != "missing" {
		t.Fatalf("%+v %v", r.Findings, err)
	}
}

func TestCitationChecksReportIncompleteScope(t *testing.T) {
	fsys := fstest.MapFS{"main.tex": {Data: []byte(`\input{absent}\bibliography{missing}\cite{x}`)}}
	r, err := CheckCitations(fsys, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, f := range r.Findings {
		kinds = append(kinds, f.Kind)
	}
	if !reflect.DeepEqual(kinds, []string{"unreadable-source", "unreadable-source", "missing"}) {
		t.Fatal(r.Findings)
	}
	r, err = CheckCitations(fsys, "")
	if err != nil || len(r.Findings) != 1 || r.Findings[0].Kind != "main-not-set" {
		t.Fatalf("%+v %v", r, err)
	}
}
