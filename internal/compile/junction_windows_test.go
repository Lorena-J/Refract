package compile

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// junction makes link a junction to target: on Windows any user can make one,
// and it points a folder elsewhere without being a symbolic link.
func junction(t *testing.T, target, link string) {
	t.Helper()
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink /J: %v %s", err, out)
	}
}

// outsideBuild puts a build folder with a main.pdf in outside, as a link to
// it at .refract would expose.
func outsideBuild(t *testing.T, outside string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(outside, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "build", "main.pdf"), []byte(secret), 0o644); err != nil {
		t.Fatal(err)
	}
}

func expectRefused(t *testing.T, proj string) {
	t.Helper()
	f, err := OpenOutput(proj, "main.pdf")
	if err == nil {
		b, _ := io.ReadAll(f)
		f.Close()
		t.Fatalf("opened; read %q", b)
	}
}

func TestOpenOutputRefusesJunctions(t *testing.T) {
	for _, rel := range []string{".refract", BuildDir} {
		t.Run(rel, func(t *testing.T) {
			proj, outside := outputFixture(t)
			outsideBuild(t, outside)
			at := filepath.Join(proj, filepath.FromSlash(rel))
			if err := os.RemoveAll(at); err != nil {
				t.Fatal(err)
			}
			junction(t, outside, at)
			expectRefused(t, proj)
		})
	}
}

// A part of the build folder swapped for a junction or a symbolic link
// between its check and its open (a concurrent git pull) leads nowhere
// outside the project: each part is opened relative to the one before.
func TestOpenOutputRefusesSwapBetweenCheckAndOpen(t *testing.T) {
	for _, kind := range []string{"junction", "symlink"} {
		for _, rel := range []string{".refract", BuildDir} {
			t.Run(kind+" "+rel, func(t *testing.T) {
				proj, outside := outputFixture(t)
				outsideBuild(t, outside)
				swapped := false
				testHookBeforeOpen = func(projectDir, at string) {
					if at != rel || swapped {
						return
					}
					swapped = true
					p := filepath.Join(projectDir, filepath.FromSlash(rel))
					if err := os.Rename(p, p+".checked"); err != nil {
						t.Fatalf("swap: %v", err)
					}
					if kind == "junction" {
						junction(t, outside, p)
					} else if err := os.Symlink(outside, p); err != nil {
						t.Fatalf("symlink: %v", err)
					}
				}
				t.Cleanup(func() { testHookBeforeOpen = nil })
				expectRefused(t, proj)
				if !swapped {
					t.Fatal("the hook never ran")
				}
			})
		}
	}
}
