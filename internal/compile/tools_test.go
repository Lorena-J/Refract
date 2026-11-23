package compile

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// fakeTool installs an executable script as the tool and empties the PATH, so
// that only the absolute path found through internal/tools can run it.
func fakeTool(t *testing.T, name, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes")
	}
	p := filepath.Join(t.TempDir(), "private-"+name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REFRACT_"+map[string]string{"tectonic": "TECTONIC", "synctex": "SYNCTEX"}[name]+"_BIN", p)
	t.Setenv("PATH", t.TempDir())
}

func TestCompileUsesTheResolvedTectonic(t *testing.T) {
	fakeTool(t, "tectonic", "printf %%PDF > \"$3/main.pdf\"\n") // args: main.tex --outdir DIR …
	dir := t.TempDir()
	write(t, dir, "main.tex", "x")
	res, err := (&Tectonic{Timeout: 10 * time.Second}).Compile(context.Background(), dir, "main.tex")
	if err != nil || !res.OK || res.PDF == "" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestSyncTeXUsesTheResolvedBinary(t *testing.T) {
	fakeTool(t, "synctex", "printf 'Page:2\\nx:72\\ny:100\\nh:72\\nv:100\\nW:50\\nH:10\\n'\n")
	dir := t.TempDir()
	write(t, dir, ".refract/build/main.pdf", "%PDF")
	boxes, err := (&SyncTeX{}).Forward(context.Background(), dir, "main.tex", "main.tex", 1, 0)
	if err != nil || len(boxes) != 1 || boxes[0].Page != 2 {
		t.Fatalf("boxes = %+v, err = %v", boxes, err)
	}
}
