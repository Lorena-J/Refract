package compile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

const secret = "outside the project"

// outputFixture makes a project with a built main.pdf and a folder outside
// it holding a file the project must never serve.
func outputFixture(t *testing.T) (proj, outside string) {
	t.Helper()
	proj, outside = t.TempDir(), t.TempDir()
	build := filepath.Join(proj, filepath.FromSlash(BuildDir))
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, "main.pdf"), []byte("%PDF built"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "main.pdf"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
	return proj, outside
}

// symlink makes link point at target, or skips the test where the platform
// doesn't let this user make links (Windows without the privilege).
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("can't make symbolic links here: %v", err)
	}
}

func TestOpenOutputServesBuiltFile(t *testing.T) {
	proj, _ := outputFixture(t)
	f, err := OpenOutput(proj, "main.pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if b, _ := io.ReadAll(f); string(b) != "%PDF built" {
		t.Errorf("read %q", b)
	}
}

func TestOpenOutputMissing(t *testing.T) {
	proj, _ := outputFixture(t)
	if _, err := OpenOutput(proj, "other.pdf"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: err = %v, want not exist", err)
	}
	empty := t.TempDir()
	if _, err := OpenOutput(empty, "main.pdf"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("never built: err = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(empty, ".refract")); !errors.Is(err, os.ErrNotExist) {
		t.Error("opening an output created the build folder")
	}
}

func TestOpenOutputRefusesBadNames(t *testing.T) {
	proj, _ := outputFixture(t)
	for _, name := range []string{"", ".", "..", "../main.pdf", `..\main.pdf`, "sub/main.pdf"} {
		if f, err := OpenOutput(proj, name); err == nil {
			f.Close()
			t.Errorf("%q: opened", name)
		}
	}
}

// Every way a project can point the output elsewhere is refused, and no data
// from outside is returned.
func TestOpenOutputRefusesLinksAndReplacements(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, proj, outside string)
	}{
		{"build folder is a link", func(t *testing.T, proj, outside string) {
			build := filepath.Join(proj, filepath.FromSlash(BuildDir))
			if err := os.RemoveAll(build); err != nil {
				t.Fatal(err)
			}
			symlink(t, outside, build)
		}},
		{".refract is a link", func(t *testing.T, proj, outside string) {
			if err := os.MkdirAll(filepath.Join(outside, "build"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "build", "main.pdf"), []byte(secret), 0o644); err != nil {
				t.Fatal(err)
			}
			dot := filepath.Join(proj, ".refract")
			if err := os.RemoveAll(dot); err != nil {
				t.Fatal(err)
			}
			symlink(t, outside, dot)
		}},
		{"output is a link", func(t *testing.T, proj, outside string) {
			pdf := filepath.Join(proj, filepath.FromSlash(BuildDir), "main.pdf")
			if err := os.Remove(pdf); err != nil {
				t.Fatal(err)
			}
			symlink(t, filepath.Join(outside, "main.pdf"), pdf)
		}},
		{"output is a folder", func(t *testing.T, proj, outside string) {
			pdf := filepath.Join(proj, filepath.FromSlash(BuildDir), "main.pdf")
			if err := os.Remove(pdf); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(pdf, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"build folder is a file", func(t *testing.T, proj, outside string) {
			build := filepath.Join(proj, filepath.FromSlash(BuildDir))
			if err := os.RemoveAll(build); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(build, []byte(secret), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proj, outside := outputFixture(t)
			tc.setup(t, proj, outside)
			f, err := OpenOutput(proj, "main.pdf")
			if err == nil {
				b, _ := io.ReadAll(f)
				f.Close()
				t.Fatalf("opened; read %q", b)
			}
		})
	}
}

// A folder swapped in after the build folder was opened is not where the
// output is read from: the handle is.
func TestOpenOutputReadsThroughCheckedFolder(t *testing.T) {
	proj, outside := outputFixture(t)
	dir, err := openBuildDir(proj, false)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	build := filepath.Join(proj, filepath.FromSlash(BuildDir))
	if err := os.Rename(build, build+".checked"); err != nil {
		t.Skipf("can't move an open folder here: %v", err) // Windows
	}
	symlink(t, outside, build)
	f, err := openIn(dir, "main.pdf")
	if err != nil {
		return // refused: fine on platforms without openat
	}
	defer f.Close()
	if b, _ := io.ReadAll(f); string(b) == secret {
		t.Fatal("read the file outside the project")
	}
}
