package refs

import (
	"fmt"
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

func TestCitationsMultilineAndMulticite(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []Finding
	}{
		{"multiline", "\\cite{knuth84,\n lamport94,\n missing}", []Finding{{Key: "knuth84", Line: 1}, {Key: "lamport94", Line: 1}, {Key: "missing", Line: 1}}},
		{"multicite", `\cites{a}{b}`, []Finding{{Key: "a", Line: 1}, {Key: "b", Line: 1}}},
		{"notes and lines", "\\parencites[see][12]{a}\n [also][34]{b, c}\n\\cite{d}", []Finding{{Key: "a", Line: 1}, {Key: "b", Line: 2}, {Key: "c", Line: 2}, {Key: "d", Line: 3}}},
		{"global notes", `\Autocites(see)(and references therein)[12]{a}[34]{b}`, []Finding{{Key: "a", Line: 1}, {Key: "b", Line: 1}}},
		{"footnote text", `\footcitetexts{a}{b}`, []Finding{{Key: "a", Line: 1}, {Key: "b", Line: 1}}},
		{"single cite followed by prose", `\cite{a}{ordinary text} \citet{b}{more prose}`, []Finding{{Key: "a", Line: 1}, {Key: "b", Line: 1}}},
		{"literal multicites", `\\cites{escaped}{also-escaped} \verb|\cites{inline}{hidden}| \begin{verbatim}\cites{example}{hidden}\end{verbatim} \cites{a}{b}`, []Finding{{Key: "a", Line: 1}, {Key: "b", Line: 1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := citations(tc.source); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCitationChecksMultilineAndMulticite(t *testing.T) {
	fsys := fstest.MapFS{
		"main.tex": {Data: []byte("\\bibliography{refs}\n\\cite{a, % another key on the next line\n b, missing}\n\\cites{c}\n [see][42]{missing-too}\n")},
		"refs.bib": {Data: []byte("@misc{a, title={A}}\n@misc{b, title={B}}\n@misc{c, title={C}}")},
	}
	report, err := CheckCitations(fsys, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	want := []Finding{
		{File: "main.tex", Line: 2, Kind: "missing", Key: "missing"},
		{File: "main.tex", Line: 5, Kind: "missing", Key: "missing-too"},
	}
	if !reflect.DeepEqual(report.Findings, want) {
		t.Fatalf("got %+v, want %+v", report.Findings, want)
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

func TestRecordsRecoverAfterUnclosedEntry(t *testing.T) {
	for _, broken := range []string{
		"@article{broken, title={Missing entry brace}\n",
		"@article{broken, title={Missing field brace\n",
		"@article{broken, title=\"Missing quote\n",
		"@article{broken, title={Missing entry brace}\n@misc{also-broken, title={Oops}\n",
	} {
		bib := broken + "  @misc{later, title={Valid}}\n@misc{last, title={Also valid}}"
		records := Records(bib)
		if len(records) != 2 || records[0].Key != "later" || records[1].Key != "last" {
			t.Fatalf("recovery from %q: %+v", broken, records)
		}
		if records[0].Line != strings.Count(broken, "\n")+1 || bib[records[0].Start:records[0].End] != "@misc{later, title={Valid}}" {
			t.Fatalf("wrong recovered position: %+v", records[0])
		}
		fix, err := ProposeMetadata("refs.bib", bib, "later", "@misc{remote, title={Updated}}")
		if err != nil || !strings.HasPrefix(fix.After, broken) || !strings.Contains(fix.After, "@misc{last, title={Also valid}}") {
			t.Fatalf("proposal after recovery: %+v %v", fix, err)
		}
	}
	// An @ inside a complete field is content, not a recovery boundary.
	if records := Records("@misc{valid, title={Notes:\n@someone\nmore text}}\n@misc{next, title={Next}}"); len(records) != 2 || records[0].Key != "valid" {
		t.Fatalf("split a valid entry: %+v", records)
	}
	fsys := fstest.MapFS{
		"main.tex": {Data: []byte(`\bibliography{refs} \cite{later}`)},
		"refs.bib": {Data: []byte("@misc{broken, title={Unclosed}\n@misc{later, title={Valid}}")},
	}
	if report, err := CheckCitations(fsys, "main.tex"); err != nil || len(report.Findings) != 0 {
		t.Fatalf("a later citation was reported missing: %+v %v", report, err)
	}
}

func BenchmarkCitationChecksLargeBibliography(b *testing.B) {
	var bib, tex strings.Builder
	tex.WriteString("\\bibliography{refs}\n")
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&bib, "@misc{key%d, title={A bibliography entry with enough text to resemble an export}}\n", i)
		fmt.Fprintf(&tex, "\\cite{key%d}\n", i)
	}
	fsys := fstest.MapFS{"main.tex": {Data: []byte(tex.String())}, "refs.bib": {Data: []byte(bib.String())}}
	b.ResetTimer()
	for b.Loop() {
		if report, err := CheckCitations(fsys, "main.tex"); err != nil || len(report.Findings) != 0 {
			b.Fatalf("%+v %v", report, err)
		}
	}
}
