//go:build !unix

package compile

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// openOutDir creates .refract/build and opens it, refusing a project where any
// part of it is a symlink. Without openat there is no race-free version; the
// sandbox, which needs it, only runs on Linux.
func openOutDir(projectDir string) (*os.File, error) {
	cur, rel := projectDir, ""
	for _, part := range strings.Split(BuildDir, "/") {
		cur, rel = filepath.Join(cur, part), path.Join(rel, part)
		fi, err := os.Lstat(cur)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(cur, 0o755); err != nil {
				return nil, err
			}
		case err != nil:
			return nil, err
		case fi.Mode()&os.ModeSymlink != 0:
			return nil, fmt.Errorf("%w: %s is a symbolic link; delete it and compile again", ErrUnsafeBuildDir, rel)
		case !fi.IsDir():
			return nil, fmt.Errorf("%w: %s is not a folder", ErrUnsafeBuildDir, rel)
		}
	}
	return os.Open(cur)
}

func openIn(dir *os.File, name string) (*os.File, error) {
	return os.Open(filepath.Join(dir.Name(), name))
}

func createIn(dir *os.File, name string) (*os.File, error) {
	return os.OpenFile(filepath.Join(dir.Name(), name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

func renameIn(dir *os.File, from, to string) error {
	return os.Rename(filepath.Join(dir.Name(), from), filepath.Join(dir.Name(), to))
}

func removeIn(dir *os.File, name string) error {
	return os.Remove(filepath.Join(dir.Name(), name))
}
