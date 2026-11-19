//go:build unix

package compile

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// openOutDir creates .refract/build and opens it, one part at a time and
// without following symlinks, and refuses a project where any part of it is
// one: a pulled repository could otherwise point it at a folder outside the
// project, which the sandbox mounts writable. Everything that touches the
// folder afterwards goes through the returned handle, so replacing the path
// with a symlink later (a git pull during the compile) changes nothing.
func openOutDir(projectDir string) (*dirHandle, error) { return openBuildDir(projectDir, true) }

// dirHandle is an open folder: on Unix, a file descriptor that openat works
// relative to, which the sandbox also mounts.
type dirHandle = os.File

// sandboxDir is the descriptor the sandbox mounts as the build folder.
func sandboxDir(d *dirHandle) *os.File { return d }

// openBuildDir opens .refract/build as openOutDir does; with create false, a
// missing part is an error matching os.ErrNotExist instead of being made.
func openBuildDir(projectDir string, create bool) (*dirHandle, error) {
	const flags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	fd, err := unix.Open(projectDir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: projectDir, Err: err}
	}
	rel := ""
	for _, part := range strings.Split(BuildDir, "/") {
		rel = path.Join(rel, part)
		next, err := unix.Openat(fd, part, flags, 0)
		if create && errors.Is(err, unix.ENOENT) {
			if err = unix.Mkdirat(fd, part, 0o755); err == nil || errors.Is(err, unix.EEXIST) {
				next, err = unix.Openat(fd, part, flags, 0)
			}
		}
		if err != nil {
			defer unix.Close(fd)
			var st unix.Stat_t
			switch {
			case errors.Is(err, unix.ENOENT):
			case unix.Fstatat(fd, part, &st, unix.AT_SYMLINK_NOFOLLOW) != nil:
			case st.Mode&unix.S_IFMT == unix.S_IFLNK:
				return nil, fmt.Errorf("%w: %s is a symbolic link; delete it and compile again", ErrUnsafeBuildDir, rel)
			case st.Mode&unix.S_IFMT != unix.S_IFDIR:
				return nil, fmt.Errorf("%w: %s is not a folder", ErrUnsafeBuildDir, rel)
			}
			return nil, &os.PathError{Op: "open", Path: filepath.Join(projectDir, filepath.FromSlash(rel)), Err: err}
		}
		unix.Close(fd)
		fd = next
	}
	return os.NewFile(uintptr(fd), filepath.Join(projectDir, filepath.FromSlash(BuildDir))), nil
}

// openIn opens name, a regular file directly in dir, for reading. A symlink
// is refused, and so is a FIFO the compile left there, which would block.
func openIn(dir *dirHandle, name string) (*os.File, error) {
	p := filepath.Join(dir.Name(), name)
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: p, Err: err}
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFREG {
		unix.Close(fd)
		return nil, &os.PathError{Op: "open", Path: p, Err: cmp.Or(err, error(unix.EINVAL))}
	}
	return os.NewFile(uintptr(fd), p), nil
}

// createIn creates name in dir; it must not exist yet.
func createIn(dir *dirHandle, name string) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, &os.PathError{Op: "create", Path: filepath.Join(dir.Name(), name), Err: err}
	}
	return os.NewFile(uintptr(fd), filepath.Join(dir.Name(), name)), nil
}

// renameIn renames from to to, both in dir.
func renameIn(dir *dirHandle, from, to string) error {
	if err := unix.Renameat(int(dir.Fd()), from, int(dir.Fd()), to); err != nil {
		return &os.LinkError{Op: "rename", Old: filepath.Join(dir.Name(), from), New: filepath.Join(dir.Name(), to), Err: err}
	}
	return nil
}

// removeIn removes the file name from dir.
func removeIn(dir *dirHandle, name string) error {
	if err := unix.Unlinkat(int(dir.Fd()), name, 0); err != nil {
		return &os.PathError{Op: "remove", Path: filepath.Join(dir.Name(), name), Err: err}
	}
	return nil
}
