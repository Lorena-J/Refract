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
func openOutDir(projectDir string) (*os.File, error) { return openBuildDir(projectDir, true) }

// openBuildDir opens .refract/build as openOutDir does; with create false, a
// missing part is an error matching os.ErrNotExist instead of being made. The
// folder opened must be the one checked: a part swapped for a symlink or
// junction between the check and the open is refused.
func openBuildDir(projectDir string, create bool) (*os.File, error) {
	cur, rel := projectDir, ""
	var checked os.FileInfo
	for _, part := range strings.Split(BuildDir, "/") {
		cur, rel = filepath.Join(cur, part), path.Join(rel, part)
		fi, err := os.Lstat(cur)
		switch {
		case create && errors.Is(err, os.ErrNotExist):
			if err := os.Mkdir(cur, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
				return nil, err
			}
			if fi, err = os.Lstat(cur); err != nil {
				return nil, err
			}
		case err != nil:
			return nil, err
		}
		if err := checkDir(fi, rel); err != nil {
			return nil, err
		}
		checked = fi
	}
	dir, err := os.Open(cur)
	if err != nil {
		return nil, err
	}
	if fi, err := dir.Stat(); err != nil || !os.SameFile(fi, checked) {
		dir.Close()
		return nil, fmt.Errorf("%w: %s changed while it was opened", ErrUnsafeBuildDir, BuildDir)
	}
	return dir, nil
}

func checkDir(fi os.FileInfo, rel string) error {
	switch {
	case fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0:
		return fmt.Errorf("%w: %s is a symbolic link; delete it and compile again", ErrUnsafeBuildDir, rel)
	case !fi.IsDir():
		return fmt.Errorf("%w: %s is not a folder", ErrUnsafeBuildDir, rel)
	}
	return nil
}

// openIn opens name, a regular file directly in dir, for reading. A symlink,
// or anything but a regular file, is refused, and so is a file or folder
// replaced between the check and the open.
func openIn(dir *os.File, name string) (*os.File, error) {
	p := filepath.Join(dir.Name(), name)
	before, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, &os.PathError{Op: "open", Path: p, Err: errors.New("not a regular file")}
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	dirNow, derr := os.Lstat(dir.Name())
	dirThen, terr := dir.Stat()
	if err != nil || derr != nil || terr != nil || !os.SameFile(before, opened) || !os.SameFile(dirNow, dirThen) {
		f.Close()
		return nil, &os.PathError{Op: "open", Path: p, Err: errors.New("changed while it was opened")}
	}
	return f, nil
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
