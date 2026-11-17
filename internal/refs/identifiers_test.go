package refs

import (
	"testing"
	"testing/fstest"
)

func TestMetadataIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		fields map[string]string
		want   string
	}{
		{map[string]string{"doi": "https://dx.doi.org/10.1000/WORK"}, "doi:10.1000/work"},
		{map[string]string{"doi": "DOI:10.1000/work"}, "doi:10.1000/work"},
		{map[string]string{"doi": "not-a-doi"}, ""},
		{map[string]string{"eprint": "arXiv:2101.00001", "archiveprefix": "arXiv"}, "arxiv:2101.00001"},
		{map[string]string{"eprint": "hep-th/9901001v2", "eprinttype": "arxiv"}, "arxiv:hep-th/9901001v2"},
		{map[string]string{"eprint": "not-an-id", "archiveprefix": "arxiv"}, ""},
		{map[string]string{"eprint": "2101.00001", "archiveprefix": "other"}, ""},
	} {
		if got := MetadataIdentifier(tc.fields); got != tc.want {
			t.Errorf("%v: got %q, want %q", tc.fields, got, tc.want)
		}
	}
}

func TestDuplicateWorksUseNormalizedIdentifiers(t *testing.T) {
	fsys := fstest.MapFS{
		"main.tex": {Data: []byte(`\bibliography{refs}\nocite{*}`)},
		"refs.bib": {Data: []byte(`@misc{a, title={A}, doi={10.1000/Work}}
@misc{b, title={B}, doi={https://dx.doi.org/10.1000/WORK}}
@misc{c, title={C}, doi={doi:10.1000/work}}
@misc{d, title={D}, eprint={2101.00001}, archiveprefix={arxiv}}
@misc{e, title={E}, eprint={arXiv:2101.00001}, eprinttype={arxiv}}
@misc{f, title={Invalid}, doi={invalid}}
@misc{g, title={Invalid}, doi={invalid}}`)},
	}
	report, err := CheckCitations(fsys, "main.tex")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Findings) != 3 {
		t.Fatalf("findings = %+v", report.Findings)
	}
	for i, key := range []string{"b", "c", "e"} {
		if f := report.Findings[i]; f.Kind != "duplicate-work" || f.Key != key {
			t.Errorf("finding = %+v", f)
		}
	}
}
