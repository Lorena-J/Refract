package projects

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// With only REFRACT_GIT_BIN pointing at git and an empty PATH, a project still
// gets its history: git runs by absolute path.
func TestGitRunsByAbsolutePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script wrapper")
	}
	real, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	wrapper := filepath.Join(t.TempDir(), "private-git")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexec '"+real+"' \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REFRACT_GIT_BIN", wrapper)
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.tex"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := initRepo(t.Context(), dir, "first"); err != nil {
		t.Fatal(err)
	}
	if out, err := git(t.Context(), dir, "log", "--format=%s"); err != nil || out != "first\n" {
		t.Fatalf("log = %q, %v", out, err)
	}
}
