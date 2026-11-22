package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fake(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// install lays out <root>/bin/refract and returns the bundled folder.
func install(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake shell executables")
	}
	root := t.TempDir()
	exe := fake(t, filepath.Join(root, "bin", "refract"))
	executable = func() (string, error) { return exe, nil }
	t.Cleanup(func() { executable = os.Executable })
	t.Setenv("PATH", t.TempDir()) // empty: nothing found on the PATH
	for _, n := range Names {
		t.Setenv(EnvVar(n), "")
	}
	d, _ := filepath.EvalSymlinks(root)
	return filepath.Join(d, "libexec", "refract")
}

func TestBundledOnly(t *testing.T) {
	dir := install(t)
	if got := Missing(); len(got) != len(Names) {
		t.Fatalf("nothing installed, missing = %v", got)
	}
	fake(t, filepath.Join(dir, "tectonic"))
	fake(t, filepath.Join(dir, "synctex"))
	fake(t, filepath.Join(dir, "rg"))
	fake(t, filepath.Join(dir, "git", "bin", "git"))
	for _, tl := range All() {
		if !tl.Found() || tl.Source != SourceBundled || !filepath.IsAbs(tl.Path) {
			t.Errorf("%+v", tl)
		}
	}
	if got := Find("rg").Path; got != filepath.Join(dir, "rg") {
		t.Errorf("rg = %s", got)
	}
}

func TestPrecedence(t *testing.T) {
	dir := install(t)
	onPath := t.TempDir()
	t.Setenv("PATH", onPath)
	fake(t, filepath.Join(onPath, "rg"))
	if got := Find("rg"); got.Source != SourcePath || got.Path != filepath.Join(onPath, "rg") {
		t.Errorf("path: %+v", got)
	}
	fake(t, filepath.Join(dir, "rg"))
	if got := Find("rg"); got.Source != SourceBundled {
		t.Errorf("bundled beats PATH: %+v", got)
	}
	over := fake(t, filepath.Join(t.TempDir(), "my-rg"))
	t.Setenv("REFRACT_RG_BIN", over)
	if got := Find("rg"); got.Source != SourceEnv || got.Path != over {
		t.Errorf("env beats bundled: %+v", got)
	}
	t.Setenv("REFRACT_RG_BIN", filepath.Join(t.TempDir(), "nope"))
	if got := Find("rg"); got.Found() {
		t.Errorf("a broken override is final: %+v", got)
	}
}

func TestSymlinkedExecutable(t *testing.T) {
	dir := install(t)
	fake(t, filepath.Join(dir, "synctex"))
	real, _ := executable()
	link := filepath.Join(t.TempDir(), "refract")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	executable = func() (string, error) { return link, nil }
	if got := Find("synctex"); got.Source != SourceBundled {
		t.Errorf("%+v", got)
	}
}

func TestGitEnv(t *testing.T) {
	dir := install(t)
	git := fake(t, filepath.Join(dir, "git", "bin", "git"))
	if err := os.MkdirAll(filepath.Join(dir, "git", "share", "git-core", "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := strings.Join(GitEnv(git), "\n")
	for _, want := range []string{
		"GIT_EXEC_PATH=" + filepath.Join(dir, "git", "libexec", "git-core"),
		"GIT_TEMPLATE_DIR=" + filepath.Join(dir, "git", "share", "git-core", "templates"),
		"GIT_CONFIG_NOSYSTEM=1",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("env %q lacks %q", env, want)
		}
	}
	if got := GitEnv("/usr/bin/git"); got != nil {
		t.Errorf("a system git gets no changes: %v", got)
	}
}
