package refs

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCitationChecksFixture(t *testing.T) {
	report, err := CheckCitations(os.DirFS("testdata/checks"), "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range report.Findings {
		got = append(got, f.File+":"+f.Kind+":"+f.Key)
	}
	want := []string{"main.tex:missing:missing", "library.bib:unused:unused", "library.bib:incomplete:unfinished", "other.bib:duplicate-key:complete", "other.bib:duplicate-work:duplicate", "other.bib:unused:duplicate"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, f := range report.Findings {
		if f.Kind == "missing" && f.Line != 3 {
			t.Fatalf("missing citation line: %+v", f)
		}
		if f.Kind == "incomplete" && !reflect.DeepEqual(f.Fields, []string{"author", "journal", "year"}) {
			t.Fatalf("missing fields: %+v", f)
		}
	}
}

func TestCitationsSkipLiteralAndComments(t *testing.T) {
	source := "% \\cite{comment}\n" + `\verb|\cite{inline}| \\cite{escape}
\begin{verbatim}\cite{literal}\end{verbatim}
\begin{minted}{tex}\cite{code}\end{minted}
\citep[see][p. 4]{a, b} \textcite{c} \nocite{*}`
	var keys []string
	for _, f := range citations(source) {
		keys = append(keys, f.Key)
	}
	if !reflect.DeepEqual(keys, []string{"a", "b", "c", "*"}) {
		t.Fatal(keys)
	}
}

func TestCitationsInheritanceAndNoCite(t *testing.T) {
	fsys := fstest.MapFS{
		"main.tex": {Data: []byte(`\bibliography{refs} \cite{child}`)},
		"refs.bib": {Data: []byte(`@inproceedings{child, title={Child}, author={Someone}, crossref={parent}}
@proceedings{parent, title={Proceedings}, year=2024}
@misc{unused, title={Unused}}`)},
		".git/hidden.tex":              {Data: []byte(`\cite{hidden}`)},
		".refract/build/generated.tex": {Data: []byte(`\cite{generated}`)},
	}
	r, err := CheckCitations(fsys, "main.tex")
	if err != nil || len(r.Findings) != 1 || r.Findings[0].Key != "unused" {
		t.Fatalf("%+v %v", r, err)
	}
	fsys["main.tex"].Data = []byte(`\bibliography{refs} \nocite{*}`)
	r, err = CheckCitations(fsys, "main.tex")
	if err != nil || len(r.Findings) != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	// Cyclic and repeated inheritance terminates and still uses local fields.
	entries := map[string]Record{"a": {Key: "a", Fields: map[string]string{"crossref": "b", "title": "A"}}, "b": {Key: "b", Fields: map[string]string{"crossref": "a", "year": "2024"}}}
	if got := missingFields(entries["a"], entries); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestRecordsAndMetadataProposal(t *testing.T) {
	bib := "% @article{ignored, title={ignore}}\n@comment{note, @book{also-ignored, title={x}}}\n" + `@article(local,
 title = "A {nested, title}", % } ignored closing brace
 doi = {10.1000/work},
 note = {Keep {\LaTeX}, commas and \%},
 keywords = "my, tags"
)
@misc{neighbour, title={Untouched}}
`
	records := Records(bib)
	if len(records) != 2 || records[0].Key != "local" || records[0].Line != 3 {
		t.Fatalf("%+v", records)
	}
	fix, err := ProposeMetadata("refs.bib", bib, "local", `@article{remote, title={Authoritative title}, author={A. Writer}, year={2024}, journal={Journal}, doi={10.1000/work}}`)
	if err != nil {
		t.Fatal(err)
	}
	if fix.Before != bib || fix.Path != "refs.bib" {
		t.Fatal(fix)
	}
	for _, want := range []string{"@article{local,", `title={Authoritative title}`, `note = {Keep {\LaTeX}, commas and \%}`, `keywords = "my, tags"`, "@misc{neighbour, title={Untouched}}"} {
		if !strings.Contains(fix.After, want) {
			t.Errorf("missing %s in %s", want, fix.After)
		}
	}
	if !strings.HasPrefix(fix.After, bib[:records[0].Start]) || !strings.HasSuffix(fix.After, bib[records[0].End:]) {
		t.Fatal("neighbouring content changed")
	}
	if len(Records(fix.After)) != 2 {
		t.Fatal(fix.After)
	}
	for _, data := range []string{bib + bib, "@article{unclosed, title={x}"} {
		if _, err := ProposeMetadata("refs.bib", data, "local", "@misc{x,title={X}}"); err == nil {
			t.Fatal("ambiguous/missing entry accepted")
		}
	}
	if _, err := ProposeMetadata("refs.bib", bib, "local", "<html>Unavailable</html>"); err == nil {
		t.Fatal("invalid metadata accepted")
	}
}
