//go:build !unix

package compile

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
)

// dirHandle is an open folder. Without openat, it is an os.Root: every open
// is relative to the folder's own handle (on Windows, NtCreateFile with a root
// directory), and nothing it opens can resolve outside it, links included.
type dirHandle struct{ root *os.Root }

func (d *dirHandle) Close() error { return d.root.Close() }
func (d *dirHandle) Name() string { return d.root.Name() }

// sandboxDir: the sandbox only runs on Linux.
func sandboxDir(*dirHandle) *os.File { return nil }

// testHookBeforeOpen, when set, runs between checking a part of the build
// folder and opening it: tests swap it for a link there.
var testHookBeforeOpen func(projectDir, rel string)

// openOutDir creates .refract/build and opens it, refusing a project where any
// part of it is a symbolic link or junction.
func openOutDir(projectDir string) (*dirHandle, error) { return openBuildDir(projectDir, true) }

// openBuildDir opens .refract/build one part at a time, each relative to the
// handle of the part before, starting from the project folder. A part that is
// a link or junction when checked is refused; one swapped for a link after the
// check can't lead outside the folder it is in, so the result is always inside
// the project. With create false, a missing part is an error matching
// os.ErrNotExist instead of being made.
func openBuildDir(projectDir string, create bool) (*dirHandle, error) {
	cur, err := os.OpenRoot(projectDir)
	if err != nil {
		return nil, err
	}
	rel := ""
	for _, part := range strings.Split(BuildDir, "/") {
		rel = path.Join(rel, part)
		fi, err := cur.Lstat(part)
		if create && errors.Is(err, os.ErrNotExist) {
			if err = cur.Mkdir(part, 0o755); err == nil || errors.Is(err, os.ErrExist) {
				fi, err = cur.Lstat(part)
			}
		}
		if err == nil {
			err = checkDir(fi, rel)
		}
		if err == nil && testHookBeforeOpen != nil {
			testHookBeforeOpen(projectDir, rel)
		}
		var next *os.Root
		if err == nil {
			if next, err = cur.OpenRoot(part); err != nil && !errors.Is(err, os.ErrNotExist) {
				err = fmt.Errorf("%w: %s changed while it was opened: %v", ErrUnsafeBuildDir, rel, err)
			}
		}
		cur.Close()
		if err != nil {
			return nil, err
		}
		cur = next
	}
	return &dirHandle{cur}, nil
}

func checkDir(fi os.FileInfo, rel string) error {
	switch {
	case fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0: // a junction is irregular
		return fmt.Errorf("%w: %s is a symbolic link; delete it and compile again", ErrUnsafeBuildDir, rel)
	case !fi.IsDir():
		return fmt.Errorf("%w: %s is not a folder", ErrUnsafeBuildDir, rel)
	}
	return nil
}

// openIn opens name, a regular file directly in dir, for reading. A link is
// refused; what is opened is checked through its handle, not its path.
func openIn(dir *dirHandle, name string) (*os.File, error) {
	if fi, err := dir.root.Lstat(name); err != nil {
		return nil, err
	} else if !fi.Mode().IsRegular() {
		return nil, &os.PathError{Op: "open", Path: name, Err: errors.New("not a regular file")}
	}
	f, err := dir.root.Open(name)
	if err != nil {
		return nil, err
	}
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return nil, &os.PathError{Op: "open", Path: name, Err: errors.New("not a regular file")}
	}
	return f, nil
}

// createIn creates name in dir; it must not exist yet.
func createIn(dir *dirHandle, name string) (*os.File, error) {
	return dir.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

// renameIn renames from to to, both in dir.
func renameIn(dir *dirHandle, from, to string) error { return dir.root.Rename(from, to) }

// removeIn removes the file name from dir.
func removeIn(dir *dirHandle, name string) error { return dir.root.Remove(name) }
