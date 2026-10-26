package compile

import (
	"context"
	"os/exec"
	"testing"
)

func TestParseView(t *testing.T) {
	// Real `synctex view` output (v1.5) for a Tectonic build.
	out := "This is SyncTeX command line utility, version 1.5\nSyncTeX result begin\nOutput:.refract/build/main.pdf\nPage:1\nx:182.475708\ny:146.719803\nh:133.768372\nv:148.772095\nW:343.711060\nH:9.075951\nbefore:\noffset:-1\nmiddle:\nafter:\nSyncTeX result end\n"
	got := parseView(out)
	if len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
	b := got[0]
	if b.Page != 1 || b.X != 133.768372 || b.W != 343.711060 || b.H != 9.075951 || b.Y < 139.69 || b.Y > 139.70 {
		t.Fatalf("box = %+v", b)
	}
}

func TestParseEdit(t *testing.T) {
	out := "This is SyncTeX command line utility, version 1.5\nSyncTeX result begin\nOutput:/p/.refract/build/main.pdf\nInput:/p/chapters/two.tex\nLine:1\nColumn:-1\nOffset:0\nContext:\nSyncTeX result end\n"
	if in, line := parseEdit(out); in != "/p/chapters/two.tex" || line != 1 {
		t.Fatalf("got %q %d", in, line)
	}
}

// Integration: forward and inverse sync across \input and \include.
func TestSyncTeXRoundTrip(t *testing.T) {
	tc := tectonic(t)
	if _, err := exec.LookPath("synctex"); err != nil {
		t.Skip("synctex not installed")
	}
	dir := t.TempDir()
	write(t, dir, "main.tex", "\\documentclass{article}\n\\begin{document}\nHello first paragraph.\n\n\\input{chapters/one}\n\\include{chapters/two}\n\\end{document}\n")
	write(t, dir, "chapters/one.tex", "Line one of chapter one.\n\nSecond paragraph in chapter one here.\n")
	write(t, dir, "chapters/two.tex", "Chapter two text on its own page.\n")
	if res, err := tc.Compile(context.Background(), dir, "main.tex"); err != nil || !res.OK {
		t.Fatalf("compile: %+v %v", res, err)
	}
	s := &SyncTeX{}
	ctx := context.Background()

	cases := []struct {
		file string
		line int
		page int
	}{
		{"main.tex", 3, 1},
		{"chapters/one.tex", 3, 1},
		{"chapters/two.tex", 1, 2}, // \include starts a new page
	}
	for _, c := range cases {
		boxes, err := s.Forward(ctx, dir, "main.tex", c.file, c.line, 0)
		if err != nil {
			t.Fatalf("forward %s:%d: %v", c.file, c.line, err)
		}
		b := boxes[0]
		if b.Page != c.page || b.W <= 0 || b.H <= 0 {
			t.Fatalf("forward %s:%d = %+v, want page %d", c.file, c.line, b, c.page)
		}
		// Clicking the middle of that box must lead back to the same place.
		file, line, err := s.Inverse(ctx, dir, "main.tex", b.Page, b.X+b.W/2, b.Y+b.H/2)
		if err != nil || file != c.file || line != c.line {
			t.Fatalf("inverse of %s:%d = %s:%d %v", c.file, c.line, file, line, err)
		}
	}
}
