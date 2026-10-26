package projects

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"path"
	"strings"
)

// Limits caps an import (PLAN.md §5). A field, not config: tests shrink it.
type Limits struct {
	MaxBytes int64 // total uncompressed size
	MaxFiles int
}

var DefaultLimits = Limits{MaxBytes: 200 << 20, MaxFiles: 5000}

// Source yields the files of an import. Names use "/" and are relative to
// the project root; extract validates them, so sources need not.
type Source func(yield func(name string, r io.Reader) error) error

// ZipSource reads a zip archive. A single top-level folder wrapping
// everything (common in exports) is stripped. Symlinks and folders are skipped.
func ZipSource(zr *zip.Reader) Source {
	return func(yield func(string, io.Reader) error) error {
		var files []*zip.File
		for _, f := range zr.File {
			if f.Mode()&os.ModeSymlink != 0 || f.FileInfo().IsDir() || strings.HasSuffix(f.Name, "/") {
				continue
			}
			files = append(files, f)
		}
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = strings.ReplaceAll(f.Name, `\`, "/")
		}
		prefix := commonTopDir(names)
		for i, f := range files {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("%s: %w", f.Name, err)
			}
			err = yield(strings.TrimPrefix(names[i], prefix), rc)
			rc.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}
}

// commonTopDir returns "dir/" if every name lives under that one folder.
func commonTopDir(names []string) string {
	var top string
	for _, n := range names {
		if isJunk(n) {
			continue
		}
		dir, _, ok := strings.Cut(strings.TrimPrefix(n, "/"), "/")
		if !ok || (top != "" && dir != top) {
			return ""
		}
		top = dir
	}
	if top == "" {
		return ""
	}
	return top + "/"
}

// MultipartSource reads a folder upload: one part per file, whose filename is
// the browser's webkitRelativePath ("folder/sub/file.tex"). The selected
// folder's own name is stripped.
func MultipartSource(mr *multipart.Reader) Source {
	return func(yield func(string, io.Reader) error) error {
		for {
			part, err := mr.NextPart()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			// part.FileName() keeps only the base name; we need the relative path.
			_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
			name := params["filename"]
			if err != nil || name == "" {
				part.Close()
				continue
			}
			name = strings.ReplaceAll(name, `\`, "/")
			if _, rest, ok := strings.Cut(name, "/"); ok {
				name = rest
			}
			err = yield(name, part)
			part.Close()
			if err != nil {
				return err
			}
		}
	}
}

// isJunk reports OS metadata that never belongs in a project.
func isJunk(name string) bool {
	return strings.HasPrefix(name, "__MACOSX/") || path.Base(name) == ".DS_Store" || path.Base(name) == "Thumbs.db"
}
