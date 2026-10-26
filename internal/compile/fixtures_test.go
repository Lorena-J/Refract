package compile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"refract/internal/projects"
	"refract/internal/store"
)

// TestFixtures compiles every private Prism export in fixtures/ (gitignored).
// fixtures/<name>.pdf, when present, is Prism's PDF: page counts must match.
// fixtures/<name>-unfixed.zip is an original with system-font lookups: it is
// not compiled, only checked for the portability warning.
func TestFixtures(t *testing.T) {
	zips, _ := filepath.Glob("../../fixtures/*.zip")
	if len(zips) == 0 {
		t.Skip("no fixtures/*.zip")
	}
	tc := tectonic(t)
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &projects.Service{DB: db, Dir: t.TempDir(), Limits: projects.DefaultLimits}

	for _, z := range zips {
		name := strings.TrimSuffix(filepath.Base(z), ".zip")
		t.Run(name, func(t *testing.T) {
			res, err := svc.ImportZip(context.Background(), name, z)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasSuffix(name, "-unfixed") {
				for _, w := range res.Warnings {
					if w.Kind == "font-exists" {
						return
					}
				}
				t.Fatalf("no \\IfFontExistsTF warning; got %+v", res.Warnings)
			}
			if res.Project.MainFile == "" {
				t.Fatalf("main file not detected; candidates %v", res.Candidates)
			}
			out, err := tc.Compile(context.Background(), svc.Path(res.Project.ID), res.Project.MainFile)
			if err != nil {
				t.Fatal(err)
			}
			if !out.OK {
				t.Fatalf("compile failed: %+v", out.Problems)
			}
			if want := filepath.Join("../../fixtures", name+".pdf"); fileExists(want) {
				if got, exp := pdfPages(t, filepath.Join(svc.Path(res.Project.ID), out.PDF)), pdfPages(t, want); got != exp {
					t.Errorf("pages = %d, Prism's PDF has %d", got, exp)
				}
			}
		})
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
