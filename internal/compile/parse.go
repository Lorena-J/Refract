package compile

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Problem is one entry of the problems panel.
type Problem struct {
	Severity string `json:"severity"` // error | warning
	Kind     string `json:"kind"`     // error | warning | overfull | underfull | absolute-path | undefined-reference | undefined-citation
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

var (
	reLocated = regexp.MustCompile(`^(error|warning): (.+?):(\d+): (.*)$`)
	rePrefix  = regexp.MustCompile(`^(error|warning|note|caused by): ?(.*)$`)
	// "(fontspec)    found." continues a package's message.
	rePackageCont = regexp.MustCompile(`^\(\w+\)\s*`)
	// Wrapper lines Tectonic adds around the real messages.
	noise = []string{
		"something bad happened inside XeTeX",
		"the XeTeX engine had an unrecoverable error",
		"warnings were issued by the TeX engine",
		"halted on potentially-recoverable error",
	}
	reUndefined = regexp.MustCompile("(?:LaTeX|Package \\w+) Warning: (Reference|Citation) `([^']+)' on page \\S+ undefined on input line (\\d+)")
)

// parseStderr reads Tectonic's summary ("error: main.tex:41: ...") into problems.
// Paths are as TeX printed them (relative to the main file, maybe without .tex).
func parseStderr(stderr string) []Problem {
	var out []Problem
	var cur *Problem
	inTranscript := false
	flush := func() {
		if cur != nil {
			cur.Message = strings.TrimSpace(cur.Message)
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(line, "=====") { // engine transcript, already summarized above it
			flush()
			inTranscript = !inTranscript
			continue
		}
		if inTranscript {
			continue
		}
		if m := reLocated.FindStringSubmatch(line); m != nil {
			flush()
			n, _ := strconv.Atoi(m[3])
			cur = &Problem{Severity: m[1], Kind: m[1], File: m[2], Line: n, Message: strings.TrimPrefix(m[4], "! ")}
			continue
		}
		if m := rePrefix.FindStringSubmatch(line); m != nil {
			flush()
			if (m[1] == "error" || m[1] == "warning") && !isNoise(m[2]) {
				cur = &Problem{Severity: m[1], Kind: m[1], Message: m[2]}
			}
			continue
		}
		// Continuation of a multi-line message, e.g. "(fontspec)    found."
		t := strings.TrimSpace(line)
		if cur != nil && t != "" && !strings.HasPrefix(t, "For immediate help") {
			cur.Message += " " + rePackageCont.ReplaceAllString(t, "")
		}
	}
	flush()
	for i := range out {
		switch {
		case strings.HasPrefix(out[i].Message, "Overfull"):
			out[i].Kind = "overfull"
		case strings.HasPrefix(out[i].Message, "Underfull"):
			out[i].Kind = "underfull"
		case strings.HasPrefix(out[i].Message, "accessing absolute path"):
			out[i].Kind = "absolute-path" // e.g. system fonts: the build depends on this machine
		}
	}
	return dedupe(out)
}

func isNoise(msg string) bool {
	for _, n := range noise {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

// parseLog finds undefined references and citations, which Tectonic only
// reports in the TeX log. The log gives the input line but not the file.
func parseLog(log string) []Problem {
	var out []Problem
	for _, m := range reUndefined.FindAllStringSubmatch(unwrapLog(log), -1) {
		n, _ := strconv.Atoi(m[3])
		kind := "undefined-reference"
		if m[1] == "Citation" {
			kind = "undefined-citation"
		}
		out = append(out, Problem{Severity: "warning", Kind: kind, Line: n, Message: fmt.Sprintf("%s `%s' undefined", m[1], m[2])})
	}
	return dedupe(out)
}

// unwrapLog joins lines TeX hard-wrapped at 79 characters.
func unwrapLog(log string) string {
	var b strings.Builder
	for _, l := range strings.Split(log, "\n") {
		b.WriteString(l)
		if len(l) != 79 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func dedupe(ps []Problem) []Problem {
	seen := map[Problem]bool{}
	out := ps[:0]
	for _, p := range ps {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// locate turns TeX's file names into project-relative paths and finds the
// source of undefined references by searching for their key.
func locate(ps []Problem, projectDir, mainFile string) {
	mainDir := path.Dir(mainFile)
	for i := range ps {
		p := &ps[i]
		switch {
		case p.Kind == "undefined-reference" || p.Kind == "undefined-citation":
			key := p.Message[strings.Index(p.Message, "`")+1 : strings.LastIndex(p.Message, "'")]
			p.File, p.Line = findKey(projectDir, key, p.Line, mainFile)
		case p.File != "":
			for _, cand := range []string{p.File, p.File + ".tex"} {
				rel := path.Clean(path.Join(mainDir, cand))
				if fi, err := os.Stat(filepath.Join(projectDir, filepath.FromSlash(rel))); err == nil && !fi.IsDir() {
					p.File = rel
					break
				}
			}
		}
	}
}

// findKey returns where key is used in a \ref- or \cite-like command,
// preferring a use on line, falling back to the main file.
func findKey(projectDir, key string, line int, mainFile string) (string, int) {
	re := regexp.MustCompile(`\\[A-Za-z]*(?:ref|cite)[A-Za-z]*\*?(?:\[[^\]]*\])*\{[^}]*` + regexp.QuoteMeta(key) + `[^}]*\}`)
	var first string
	firstLine := 0
	filepath.WalkDir(projectDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == ".refract") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".tex") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(projectDir, p)
		rel = filepath.ToSlash(rel)
		for i, l := range strings.Split(string(b), "\n") {
			if !re.MatchString(l) {
				continue
			}
			if i+1 == line {
				first, firstLine = rel, i+1
				return filepath.SkipAll
			}
			if first == "" {
				first, firstLine = rel, i+1
			}
		}
		return nil
	})
	if first == "" {
		return mainFile, line
	}
	return first, firstLine
}
