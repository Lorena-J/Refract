package refs

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Record retains source positions so a metadata correction replaces exactly
// one entry, leaving neighbouring entries and comments alone.
type Record struct {
	Key, Kind        string
	Fields           map[string]string
	Start, End, Line int
}

var recordStart = regexp.MustCompile(`(?i)@([a-z]+)\s*([({])`)

// Records reads brace- and parenthesis-delimited BibTeX, including nested
// values and quoted commas. Comments, strings and preambles are not entries.
func Records(src string) []Record {
	var out []Record
	for pos := 0; pos < len(src); {
		if src[pos] == '%' {
			if n := strings.IndexByte(src[pos:], '\n'); n >= 0 {
				pos += n + 1
				continue
			}
			break
		}
		if src[pos] != '@' {
			pos++
			continue
		}
		m := recordStart.FindStringSubmatchIndex(src[pos:])
		if m == nil || m[0] != 0 {
			pos++
			continue
		}
		start, body := pos, pos+m[1]
		kind := strings.ToLower(src[pos+m[2] : pos+m[3]])
		close := byte('}')
		if src[pos+m[4]] == '(' {
			close = ')'
		}
		depth, quoted, end := 0, false, body
		for ; end < len(src); end++ {
			c := src[end]
			if c == '%' && depth == 0 && !quoted {
				if n := strings.IndexByte(src[end:], '\n'); n >= 0 {
					end += n
					continue
				}
				end = len(src)
				break
			}
			if c == '\\' {
				end++
				continue
			}
			if c == '"' && depth == 0 {
				quoted = !quoted
				continue
			}
			if !quoted && depth == 0 && c == close {
				break
			}
			if c == '{' {
				depth++
			} else if c == '}' && depth > 0 {
				depth--
			}
		}
		if end >= len(src) {
			break
		} // never propose replacing a partial entry
		pos = end + 1
		if kind == "comment" || kind == "string" || kind == "preamble" {
			continue
		}
		key, _, ok := strings.Cut(src[body:end], ",")
		key = strings.TrimSpace(key)
		if !ok || !ValidKey(key) {
			continue
		}
		fields := bibFields(blankFieldComments(src[body+strings.IndexByte(src[body:end], ',')+1:end]) + "}")
		out = append(out, Record{key, kind, fields, start, end + 1, 1 + strings.Count(src[:start], "\n")})
	}
	return out
}

// ValidKey excludes characters which could escape a BibTeX entry header.
func ValidKey(key string) bool {
	return key != "" && !strings.ContainsAny(key, "{},()\\\"%# \t\r\n")
}

type Finding struct {
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Kind       string   `json:"kind"`
	Key        string   `json:"key,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	Identifier string   `json:"identifier,omitempty"`
}

type BibliographyEntry struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Key        string `json:"key"`
	DOI        string `json:"doi,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type CitationReport struct {
	Findings []Finding           `json:"findings"`
	Entries  []BibliographyEntry `json:"entries"`
}

// citationToken consumes escapes before looking for another command, so
// \\cite is not a citation. Literal environments and inline verb hide examples.
var citationToken = regexp.MustCompile(`(?s)%[^\n]*|\\begin\{(verbatim\*?|Verbatim\*?|lstlisting|minted|comment)\}|\\verb\*?([^a-zA-Z\s])|\\(?:[a-zA-Z]*[Cc]ite[a-zA-Z]*|nocite)\*?\s*(?:\[[^\]]*\]\s*)*\{([^{}\\#\n]*)\}|\\.`)

func citations(src string) []Finding {
	var out []Finding
	for pos := 0; pos < len(src); {
		m := citationToken.FindStringSubmatchIndex(src[pos:])
		if m == nil {
			break
		}
		start, end := pos+m[0], pos+m[1]
		switch {
		case m[2] >= 0:
			closing := `\end{` + src[pos+m[2]:pos+m[3]] + `}`
			at := strings.Index(src[end:], closing)
			if at < 0 {
				return out
			}
			end += at + len(closing)
		case m[4] >= 0:
			at := strings.Index(src[end:], src[pos+m[4]:pos+m[5]])
			if at < 0 {
				return out
			}
			end += at + 1
		case m[6] >= 0:
			for _, key := range strings.Split(src[pos+m[6]:pos+m[7]], ",") {
				if key = strings.TrimSpace(key); key != "" {
					out = append(out, Finding{Line: 1 + strings.Count(src[:start], "\n"), Key: key})
				}
			}
		}
		pos = end
	}
	return out
}

// CheckCitations scans the selected document and its loaded bibliographies,
// without network requests or modifications. It does not expand TeX macros.
func CheckCitations(fsys fs.FS, mainFile string) (CitationReport, error) {
	out := CitationReport{Findings: []Finding{}, Entries: []BibliographyEntry{}}
	type located struct {
		file   string
		record Record
	}
	var entries []located
	var cites []Finding
	files, findings, err := documentFiles(fsys, mainFile)
	if err != nil {
		return out, err
	}
	out.Findings = append(out.Findings, findings...)
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range paths {
		if strings.EqualFold(path.Ext(p), ".bib") {
			for _, r := range Records(files[p]) {
				entries = append(entries, located{p, r})
			}
		} else {
			for _, c := range citations(files[p]) {
				c.File = p
				cites = append(cites, c)
			}
		}
	}
	byKey := map[string]Record{}
	for _, e := range entries {
		byKey[e.record.Key] = e.record
	}
	used := map[string]bool{}
	for _, c := range cites {
		used[c.Key] = true
		if _, ok := byKey[c.Key]; !ok && c.Key != "*" {
			c.Kind = "missing"
			out.Findings = append(out.Findings, c)
		}
	}
	// A cited child's crossref/xdata is used as well, including chains.
	for range len(entries) {
		changed := false
		for _, e := range entries {
			if used["*"] || used[e.record.Key] {
				for _, field := range []string{"crossref", "xdata"} {
					for _, key := range strings.Split(e.record.Fields[field], ",") {
						key = strings.TrimSpace(key)
						if key != "" && !used[key] {
							used[key] = true
							changed = true
						}
					}
				}
			}
		}
		if !changed {
			break
		}
	}
	seenKey, seenWork := map[string]bool{}, map[string]bool{}
	for _, e := range entries {
		r := e.record
		id := ""
		if r.Fields["doi"] != "" {
			id = "doi:" + r.Fields["doi"]
		} else if r.Fields["eprint"] != "" && (strings.EqualFold(r.Fields["archiveprefix"], "arxiv") || strings.EqualFold(r.Fields["eprinttype"], "arxiv")) {
			id = "arxiv:" + r.Fields["eprint"]
		}
		out.Entries = append(out.Entries, BibliographyEntry{e.file, r.Line, r.Key, r.Fields["doi"], id})
		add := func(kind string, fields []string) {
			out.Findings = append(out.Findings, Finding{File: e.file, Line: r.Line, Kind: kind, Key: r.Key, Fields: fields, Identifier: id})
		}
		if seenKey[r.Key] {
			add("duplicate-key", nil)
		}
		seenKey[r.Key] = true
		work := strings.ToLower(id)
		work = strings.ReplaceAll(work, "https://doi.org/", "")
		work = strings.ReplaceAll(work, "http://doi.org/", "")
		if work != "" && seenWork[work] {
			add("duplicate-work", nil)
		}
		if work != "" {
			seenWork[work] = true
		}
		if !used["*"] && !used[r.Key] && r.Kind != "xdata" {
			add("unused", nil)
		}
		if missing := missingFields(r, byKey); len(missing) > 0 && r.Kind != "xdata" {
			add("incomplete", missing)
		}
	}
	return out, nil
}

func missingFields(r Record, entries map[string]Record) []string {
	fields := map[string]string{}
	visited := map[string]bool{}
	// Inherited metadata counts. Bound recursion to handle cyclic crossrefs.
	var inherit func(Record, int)
	inherit = func(e Record, depth int) {
		if depth > len(entries) || visited[e.Key] {
			return
		}
		visited[e.Key] = true
		for _, name := range []string{"crossref", "xdata"} {
			for _, key := range strings.Split(e.Fields[name], ",") {
				if p, ok := entries[strings.TrimSpace(key)]; ok {
					inherit(p, depth+1)
					if name == "crossref" && fields["booktitle"] == "" && (r.Kind == "inproceedings" || r.Kind == "conference" || r.Kind == "incollection" || r.Kind == "inbook") {
						fields["booktitle"] = p.Fields["title"]
					}
				}
			}
		}
		for k, v := range e.Fields {
			if v != "" {
				fields[k] = v
			}
		}
	}
	inherit(r, 0)
	required := []string{"title"}
	switch r.Kind {
	case "article":
		required = append(required, "author", "journal", "year")
	case "book", "inbook":
		required = append(required, "author/editor", "publisher", "year")
	case "inproceedings", "conference", "incollection":
		required = append(required, "author", "booktitle", "year")
	case "phdthesis", "mastersthesis":
		required = append(required, "author", "school", "year")
	case "techreport":
		required = append(required, "author", "institution", "year")
	}
	var missing []string
	for _, f := range required {
		if f == "journal" && fields["journaltitle"] != "" {
			continue
		}
		if f == "year" && fields["date"] != "" || f == "author/editor" && (fields["author"] != "" || fields["editor"] != "") {
			continue
		}
		if fields[f] == "" {
			missing = append(missing, f)
		}
	}
	return missing
}

// MetadataFix is a proposal only. Accepting remains an ordinary editor edit.
type MetadataFix struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// ProposeMetadata preserves the citation key, neighbouring entries and local
// fields (notes, keywords, attachments); only source-supplied fields replace
// existing values. A duplicate key is ambiguous and requires manual repair.
func ProposeMetadata(file, bib, key, metadata string) (MetadataFix, error) {
	if !ValidKey(key) {
		return MetadataFix{}, errors.New("invalid citation key")
	}
	source := Records(metadata)
	if len(source) != 1 || source[0].Fields["title"] == "" {
		return MetadataFix{}, errors.New("source did not return a complete bibliography entry")
	}
	var found []Record
	for _, r := range Records(bib) {
		if r.Key == key {
			found = append(found, r)
		}
	}
	if len(found) != 1 {
		return MetadataFix{}, errors.New("entry changed or citation key is ambiguous; run the checks again")
	}
	r := found[0]
	// Keep the original raw values for fields absent from the authoritative
	// entry, rather than converting LaTeX markup to completion's plain text.
	replacement := metadata[source[0].Start:source[0].End]
	comma := strings.IndexByte(replacement, ',')
	open := strings.IndexAny(replacement, "{(")
	replacement = replacement[:open+1] + key + replacement[comma:]
	var extras []string
	for name := range r.Fields {
		if _, ok := source[0].Fields[name]; !ok {
			extras = append(extras, name)
		}
	}
	slices.Sort(extras)
	// Source replacement is intentionally shown in full for review. Local
	// values are copied verbatim by scanning their top-level field boundaries.
	raw := rawFields(bib[r.Start:r.End])
	closing := replacement[len(replacement)-1:]
	replacement = strings.TrimRight(replacement[:len(replacement)-1], " \t\r\n,") + ",\n"
	for _, name := range extras {
		if value := raw[name]; value != "" {
			replacement += "  " + name + " = " + value + ",\n"
		}
	}
	replacement += closing
	return MetadataFix{file, bib, bib[:r.Start] + replacement + bib[r.End:]}, nil
}

func rawFields(entry string) map[string]string {
	start := strings.IndexByte(entry, ',') + 1
	entry = entry[:start] + blankFieldComments(entry[start:])
	fields := map[string]string{}
	depth, quoted := 0, false
	for i := start; i < len(entry); i++ {
		c := entry[i]
		if c == '\\' {
			i++
			continue
		}
		if c == '"' && depth == 0 {
			quoted = !quoted
		}
		if !quoted && depth == 0 && (c == ',' || c == '}' || c == ')') {
			if k, v, ok := strings.Cut(entry[start:i], "="); ok {
				fields[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
			}
			start = i + 1
			if c != ',' {
				break
			}
		}
		if c == '{' {
			depth++
		} else if c == '}' && depth > 0 {
			depth--
		}
	}
	return fields
}

// Comments between fields must not swallow the next field's name, or let a
// commented closing brace terminate an entry. Values keep literal percents.
func blankFieldComments(body string) string {
	b := []byte(body)
	depth, quoted := 0, false
	for i := 0; i < len(b); i++ {
		switch c := b[i]; {
		case c == '\\':
			i++
		case c == '"' && depth == 0:
			quoted = !quoted
		case c == '{':
			depth++
		case c == '}' && depth > 0:
			depth--
		case c == '%' && depth == 0 && !quoted:
			for i < len(b) && b[i] != '\n' {
				b[i] = ' '
				i++
			}
		}
	}
	return string(b)
}
