package refs

import (
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/ponytojas/Refract/internal/projects"
)

var literalToken = regexp.MustCompile(`(?s)%[^\n]*|\\begin\{(verbatim\*?|Verbatim\*?|lstlisting|minted|comment)\}|\\verb\*?([^a-zA-Z\s])|\\.`)

// Blank comments, literal examples and escaped backslashes, keeping offsets.
func documentText(src string) string {
	b := []byte(src)
	for pos := 0; pos < len(src); {
		m := literalToken.FindStringSubmatchIndex(src[pos:])
		if m == nil {
			break
		}
		start, end := pos+m[0], pos+m[1]
		mask := src[start] == '%'
		if m[2] >= 0 {
			closing := `\end{` + src[pos+m[2]:pos+m[3]] + `}`
			at := strings.Index(src[end:], closing)
			if at < 0 {
				end = len(src)
			} else {
				end += at + len(closing)
			}
			mask = true
		} else if m[4] >= 0 {
			at := strings.Index(src[end:], src[pos+m[4]:pos+m[5]])
			if at < 0 {
				end = len(src)
			} else {
				end += at + 1
			}
			mask = true
		} else if !mask && end-start == 2 && src[start+1] == '\\' {
			mask = true
		}
		if mask {
			for i := start; i < end; i++ {
				if b[i] != '\n' {
					b[i] = ' '
				}
			}
		}
		pos = end
	}
	return string(b)
}

var documentCommand = regexp.MustCompile(`\\(input|include)\s*\{([^{}]*)\}|\\(input)\s+([^\s{}%\\]+)|\\(addbibresource|bibliography)\s*(?:\[[^\]]*\])?\s*\{([^{}]*)\}|\\(end)\s*\{document\}`)

// Follow the selected manuscript only. Paths resolve from the main file's
// directory, where Tectonic runs, including names in nested source files.
func documentFiles(fsys fs.FS, mainFile string) (map[string]string, []Finding, error) {
	files := map[string]string{}
	var findings []Finding
	if mainFile == "" {
		return files, []Finding{{Kind: "main-not-set"}}, nil
	}
	visited := map[string]bool{}
	var load func(string) error
	load = func(file string) error {
		if visited[file] {
			return nil
		}
		visited[file] = true
		if _, err := projects.CheckPath(file); err != nil {
			findings = append(findings, Finding{File: file, Line: 1, Kind: "unreadable-source"})
			return nil
		}
		info, err := fs.Stat(fsys, file)
		if err != nil || !info.Mode().IsRegular() {
			findings = append(findings, Finding{File: file, Line: 1, Kind: "unreadable-source"})
			return nil
		}
		if info.Size() > maxSymbolFile {
			findings = append(findings, Finding{File: file, Line: 1, Kind: "skipped"})
			return nil
		}
		b, err := fs.ReadFile(fsys, file)
		if err != nil {
			return err
		}
		if strings.EqualFold(path.Ext(file), ".bib") {
			files[file] = string(b)
			return nil
		}
		text := documentText(string(b))
		for _, m := range documentCommand.FindAllStringSubmatchIndex(text, -1) {
			if m[14] >= 0 {
				text = text[:m[0]]
				break
			}
		}
		files[file] = text
		for _, m := range documentCommand.FindAllStringSubmatch(text, -1) {
			switch {
			case m[1] != "" || m[3] != "":
				name := strings.TrimSpace(m[2] + m[4])
				if name == "" {
					continue
				}
				if path.Ext(name) == "" {
					name += ".tex"
				}
				if err := load(documentPath(mainFile, name)); err != nil {
					return err
				}
			case m[5] != "":
				arg := m[6]
				if m[5] == "bibliography" {
					arg = strings.Join(strings.Fields(arg), "")
				}
				for _, name := range strings.Split(arg, ",") {
					name = strings.TrimSpace(name)
					if name == "" {
						continue
					}
					if !strings.EqualFold(path.Ext(name), ".bib") {
						name += ".bib"
					}
					if err := load(documentPath(mainFile, name)); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	err := load(mainFile)
	return files, findings, err
}

func documentPath(mainFile, name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "/") {
		return name
	}
	return path.Join(mainDir(mainFile), name)
}
