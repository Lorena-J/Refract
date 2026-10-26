package arxiv

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/ponytojas/Refract/internal/projects"
)

var stopwords = map[string]bool{}

func init() {
	for _, w := range strings.Fields("a an the on of for in to and or with is are be was were via from by at as it its this that you your we our all not can do") {
		stopwords[w] = true
	}
}

// ponytail: common Latin accents only; anything else is dropped from keys.
var fold = strings.NewReplacer("á", "a", "à", "a", "ä", "a", "â", "a", "ã", "a", "å", "a", "é", "e", "è", "e", "ë", "e", "ê", "e",
	"í", "i", "ì", "i", "ï", "i", "î", "i", "ó", "o", "ò", "o", "ö", "o", "ô", "o", "õ", "o", "ø", "o", "ú", "u", "ù", "u", "ü", "u", "û", "u",
	"ñ", "n", "ç", "c", "ß", "ss", "ł", "l", "š", "s", "ž", "z", "č", "c", "ć", "c", "ř", "r")

func ascii(s string) string {
	s = fold.Replace(strings.ToLower(s))
	var b strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Key is a citation key in the usual style: first author's surname, year, first significant title word.
func Key(p Paper) string {
	surname := "arxiv"
	if len(p.Authors) > 0 {
		f := strings.Fields(p.Authors[0])
		if s := ascii(f[len(f)-1]); s != "" {
			surname = s
		}
	}
	word := ""
	for _, w := range strings.Fields(p.Title) {
		if w = ascii(w); w != "" && !stopwords[w] {
			word = w
			break
		}
	}
	return fmt.Sprintf("%s%d%s", surname, p.Published.Year(), word)
}

// escape protects BibTeX/LaTeX specials that arXiv titles carry unescaped.
var special = regexp.MustCompile(`(^|[^\\])([&%#])`)

func escape(s string) string {
	for special.MatchString(s) { // twice for adjacent specials ("&&")
		s = special.ReplaceAllString(s, `$1\$2`)
	}
	return s
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
	field("title", escape(p.Title))
	field("author", escape(strings.Join(p.Authors, " and ")))
	field("year", fmt.Sprint(p.Published.Year()))
	field("eprint", p.ID)
	field("archivePrefix", "arXiv")
	field("primaryClass", p.Category)
	field("doi", p.DOI)
	field("url", p.URL())
	b.WriteString("}\n")
	return b.String()
}

var entryKey = regexp.MustCompile(`@\w+\s*\{\s*([^,\s]+)\s*,`)

// AddEntry appends p to a .bib file's content. A paper already present (same
// eprint) is not added again: its existing key is returned. A key taken by
// another entry gets a letter suffix.
func AddEntry(bib string, p Paper) (content, key string, added bool) {
	if m := regexp.MustCompile(`(?i)eprint\s*=\s*[{"]` + regexp.QuoteMeta(p.ID) + `(v\d+)?[}"]`).FindStringIndex(bib); m != nil {
		keys := entryKey.FindAllStringSubmatch(bib[:m[0]], -1)
		if len(keys) > 0 {
			return bib, keys[len(keys)-1][1], false
		}
	}
	taken := map[string]bool{}
	for _, m := range entryKey.FindAllStringSubmatch(bib, -1) {
		taken[m[1]] = true
	}
	base := Key(p)
	key = base
	for c := 'a'; taken[key]; c++ {
		key = base + string(c)
	}
	if bib != "" && !strings.HasSuffix(bib, "\n") {
		bib += "\n"
	}
	if bib != "" {
		bib += "\n"
	}
	return bib + BibTeX(p, key), key, true
}

var bibCommand = regexp.MustCompile(`\\(?:addbibresource|bibliography)\s*(?:\[[^\]]*\])?\s*\{([^}]*)\}`)

// BibTarget picks the .bib file citations go to: the first one a .tex file
// names (\addbibresource or \bibliography), else the first .bib in the
// project, else references.bib (and needsSetup reports that no .tex file
// loads a bibliography yet).
func BibTarget(fsys fs.FS, mainFile string) (target string, needsSetup bool) {
	var texs, bibs []string
	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".refract") {
			return fs.SkipDir
		}
		switch strings.ToLower(path.Ext(p)) {
		case ".tex":
			texs = append(texs, p)
		case ".bib":
			bibs = append(bibs, p)
		}
		return nil
	})
	// The main file first: its bibliography is the one that is printed.
	for i, t := range texs {
		if t == mainFile {
			texs[0], texs[i] = texs[i], texs[0]
		}
	}
	for _, t := range texs {
		b, err := fs.ReadFile(fsys, t)
		if err != nil {
			continue
		}
		for _, m := range bibCommand.FindAllStringSubmatch(string(b), -1) {
			name := strings.TrimSpace(strings.Split(m[1], ",")[0])
			if name == "" {
				continue
			}
			if !strings.HasSuffix(strings.ToLower(name), ".bib") {
				name += ".bib"
			}
			// Paths in \bibliography are relative to the project root (Tectonic runs there).
			return path.Clean(name), false
		}
	}
	if len(bibs) > 0 {
		return bibs[0], true
	}
	return "references.bib", true
}

// Citation is the outcome of Cite.
type Citation struct {
	Key        string `json:"key"`
	Path       string `json:"path"`        // the .bib file, relative to the project
	Added      bool   `json:"added"`       // false: the paper was already there
	NeedsSetup bool   `json:"needs_setup"` // no .tex file loads a bibliography yet
}

// Cite adds p to the bibliography of the project at root (see BibTarget).
func Cite(root *os.Root, mainFile string, p Paper) (Citation, error) {
	target, needsSetup := BibTarget(root.FS(), mainFile)
	target, err := projects.CheckPath(target)
	if err != nil {
		return Citation{}, fmt.Errorf("the bibliography path in the project is not usable: %w", err)
	}
	old, err := root.ReadFile(target)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Citation{}, err
	}
	content, key, added := AddEntry(string(old), p)
	c := Citation{Key: key, Path: target, Added: added, NeedsSetup: needsSetup}
	if !added {
		return c, nil
	}
	if d := path.Dir(target); d != "." {
		if err := root.MkdirAll(d, 0o755); err != nil {
			return c, err
		}
	}
	return c, root.WriteFile(target, []byte(content), 0o644)
}
