package refs

import (
	"strings"
	"testing"
	"testing/fstest"
)

func ref(doi, eprint string) Ref {
	return Ref{Title: "Attention Is All You Need", Authors: []string{"Ashish Vaswani", "Noam Shazeer"}, Year: 2017, DOI: doi, Eprint: eprint,
		BibTeX: func(key string) string {
			return "@misc{" + key + ",\n  eprint = {" + eprint + "},\n  doi = {" + doi + "},\n}\n"
		}}
}

func TestKeyAndEscape(t *testing.T) {
	if k := Key(ref("", "1706.03762")); k != "vaswani2017attention" {
		t.Errorf("key = %q", k)
	}
	if k := Key(Ref{Authors: []string{"José Ñúñez"}, Title: "On the Theory", Year: 2020}); k != "nunez2020theory" {
		t.Errorf("accented key = %q", k)
	}
	if got := Escape(`R&D at 50% & more, \& kept`); got != `R\&D at 50\% \& more, \& kept` {
		t.Errorf("escape = %q", got)
	}
}

func TestAddEntry(t *testing.T) {
	p := ref("", "1706.03762")
	bib, key, added := AddEntry("", p)
	if !added || key != "vaswani2017attention" || !strings.HasPrefix(bib, "@misc{vaswani2017attention,") {
		t.Fatalf("first add = %q %v\n%s", key, added, bib)
	}
	// Same paper again (any version): not duplicated, existing key returned.
	if _, k, added := AddEntry(bib+"@book{other, title={X}}\n", p); added || k != key {
		t.Errorf("re-add = %q %v", k, added)
	}
	// Same DOI written as a URL, under another key.
	byDOI := "@article{lecun15,\n  doi = {https://doi.org/10.1038/Nature14539},\n}\n"
	if _, k, added := AddEntry(byDOI, ref("10.1038/nature14539", "")); added || k != "lecun15" {
		t.Errorf("DOI dedupe = %q %v", k, added)
	}
	// Key taken by another work: suffix.
	taken := "@article{vaswani2017attention,\n  title = {Something else},\n}"
	out, k, added := AddEntry(taken, p)
	if !added || k != "vaswani2017attentiona" || !strings.HasPrefix(out, taken+"\n\n@misc{vaswani2017attentiona,") {
		t.Errorf("collision = %q\n%s", k, out)
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

func TestOnlyAddsCites(t *testing.T) {
	orig := `Transformers rely on attention. They scale well \cite{kaplan2020}.`
	for edited, want := range map[string]bool{
		`Transformers rely on attention~\cite{vaswani}. They scale well \cite{kaplan2020}.`:       true,
		`Transformers rely on attention \citep{vaswani}. They scale well \cite{kaplan2020}.`:      true,
		`Transformers rely on attention. They scale well \cite{kaplan2020,vaswani}.`:              true,
		`Transformers rely on attention. They scale well \cite{vaswani, kaplan2020}.`:             true,
		`Transformers rely on attention~\cite[p.~3]{vaswani}. They scale well \cite{kaplan2020}.`: true,
		orig: false, // nothing added
		`Transformers rely heavily on attention~\cite{vaswani}. They scale well \cite{kaplan2020}.`: false, // reworded
		`Transformers rely on attention~\cite{other}. They scale well \cite{kaplan2020}.`:           false, // another key
		`Transformers rely on attention~\cite{vaswani}. They scale well.`:                           false, // removed a citation
	} {
		if got := OnlyAddsCites(orig, edited, "vaswani"); got != want {
			t.Errorf("OnlyAddsCites(%q) = %v, want %v", edited, got, want)
		}
	}
}

func TestAppendCite(t *testing.T) {
	for sel, want := range map[string]string{
		"They scale well.\n": "They scale well~\\cite{k}.\n",
		"a claim":            "a claim~\\cite{k}",
		"  ends?  ":          "  ends~\\cite{k}?  ",
	} {
		if got := AppendCite(sel, "k"); got != want {
			t.Errorf("AppendCite(%q) = %q, want %q", sel, got, want)
		}
	}
}
