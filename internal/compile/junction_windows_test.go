package compile

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// On Windows a junction points a folder elsewhere without being a symbolic
// link, and any user can make one: it is refused like a link.
func TestOpenOutputRefusesJunctions(t *testing.T) {
	for _, rel := range []string{".refract", BuildDir} {
		t.Run(rel, func(t *testing.T) {
			proj, outside := outputFixture(t)
			if err := os.MkdirAll(filepath.Join(outside, "build"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(outside, "build", "main.pdf"), []byte(secret), 0o644); err != nil {
				t.Fatal(err)
			}
			at := filepath.Join(proj, filepath.FromSlash(rel))
			if err := os.RemoveAll(at); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command("cmd", "/c", "mklink", "/J", at, outside).CombinedOutput(); err != nil {
				t.Skipf("mklink /J: %v %s", err, out)
			}
			f, err := OpenOutput(proj, "main.pdf")
			if err == nil {
				b, _ := io.ReadAll(f)
				f.Close()
				t.Fatalf("opened through a junction; read %q", b)
			}
		})
	}
}
