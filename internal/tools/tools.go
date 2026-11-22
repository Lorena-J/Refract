// Package tools finds the external programs Refract runs: tectonic, synctex,
// rg and git.
//
// Each is looked up, in this order, in:
//
//  1. its REFRACT_<NAME>_BIN override (REFRACT_TECTONIC_BIN, REFRACT_SYNCTEX_BIN,
//     REFRACT_RG_BIN, REFRACT_GIT_BIN). A set override is final: if it names
//     nothing runnable the tool is reported missing, never silently replaced.
//  2. the bundled folder next to the executable: <exe dir>/../libexec/refract/
//     on Unix-likes, the executable's own folder on Windows (symlinks of the
//     executable are resolved first).
//  3. the PATH.
//
// Callers always run the absolute path Find returns. A bundled Git lives in its
// own tree (see below) and is told, through GitEnv, to use only that tree's
// helpers and templates, never the system's.
//
// The bundled folder (Dir) holds:
//
//	tectonic  synctex  rg                 the three single-file tools (".exe" on Windows)
//	git/bin/git                           the git executable (git/bin/git.exe on Windows)
//	git/libexec/git-core/                 Git's helpers (GIT_EXEC_PATH)
//	git/share/git-core/templates/         optional; Git's init templates (GIT_TEMPLATE_DIR)
package tools

import (
	"cmp"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Names of the tools, in the order they are reported.
var Names = []string{"git", "tectonic", "synctex", "rg"}

// Where a tool was found.
const (
	SourceEnv     = "env"
	SourceBundled = "bundled"
	SourcePath    = "path"
)

// Tool is the result of looking one tool up. Path is empty when it is missing.
type Tool struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source,omitempty"`
}

// Found reports whether the tool can be run.
func (t Tool) Found() bool { return t.Path != "" }

// executable and goos are replaced in tests.
var (
	executable = os.Executable
	goos       = runtime.GOOS
)

// EnvVar is the override variable of a tool: REFRACT_TECTONIC_BIN for "tectonic".
func EnvVar(name string) string { return "REFRACT_" + strings.ToUpper(name) + "_BIN" }

// Dir is the bundled folder, or "" when the executable's location is unknown.
func Dir() string {
	exe, err := executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	d := filepath.Dir(exe)
	if goos == "windows" {
		return d
	}
	return filepath.Join(d, "..", "libexec", "refract")
}

func exeName(name string) string {
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

// bundled is where name lives inside Dir().
func bundled(dir, name string) string {
	if name == "git" {
		return filepath.Join(dir, "git", "bin", exeName(name))
	}
	return filepath.Join(dir, exeName(name))
}

func runnable(p string) bool {
	fi, err := os.Stat(p)
	if err != nil || fi.IsDir() {
		return false
	}
	return goos == "windows" || fi.Mode()&0o111 != 0
}

// Find looks name up as described in the package documentation.
func Find(name string) Tool {
	t := Tool{Name: name}
	if v := os.Getenv(EnvVar(name)); v != "" {
		if p, err := exec.LookPath(v); err == nil {
			if abs, err := filepath.Abs(p); err == nil {
				t.Path, t.Source = abs, SourceEnv
			}
		}
		return t
	}
	if dir := Dir(); dir != "" {
		if p := bundled(dir, name); runnable(p) {
			t.Path, t.Source = filepath.Clean(p), SourceBundled
			return t
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			t.Path, t.Source = abs, SourcePath
		}
	}
	return t
}

// Path is Find(name).Path, or name itself when missing so that running it
// fails with the usual "executable file not found" error.
func Path(name string) string { return cmp.Or(Find(name).Path, name) }

// All looks every tool up.
func All() []Tool {
	out := make([]Tool, len(Names))
	for i, n := range Names {
		out[i] = Find(n)
	}
	return out
}

// Missing lists the names of the tools that weren't found.
func Missing() []string {
	var out []string
	for _, t := range All() {
		if !t.Found() {
			out = append(out, t.Name)
		}
	}
	return out
}

// GitEnv is the environment to add when running the git at gitPath. For a
// bundled Git it points Git at its own helpers and templates and ignores the
// system configuration; for any other Git it adds nothing.
func GitEnv(gitPath string) []string {
	dir := Dir()
	if dir == "" || gitPath == "" {
		return nil
	}
	root := filepath.Join(dir, "git")
	if filepath.Clean(gitPath) != filepath.Clean(bundled(dir, "git")) {
		return nil
	}
	env := []string{
		"GIT_EXEC_PATH=" + filepath.Join(root, "libexec", "git-core"),
		"GIT_CONFIG_NOSYSTEM=1",
	}
	if t := filepath.Join(root, "share", "git-core", "templates"); isDir(t) {
		env = append(env, "GIT_TEMPLATE_DIR="+t)
	}
	return env
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// Features says what stops working without each tool.
var Features = map[string]string{
	"git":      "project history",
	"tectonic": "compiling",
	"synctex":  "source and PDF jumps",
	"rg":       "the assistant's search",
}

// Hint tells what to do about a missing tool, for the log.
func Hint(name string) string {
	switch {
	case name == "git" && goos == "darwin":
		return "the Refract installation is incomplete: reinstall Refract, or install git (xcode-select --install) and restart; REFRACT_GIT_BIN names another git"
	case name == "git" && goos == "windows":
		return "the Refract installation is incomplete: reinstall Refract, or install Git for Windows and restart; REFRACT_GIT_BIN names another git"
	case name == "git":
		return "install git with the system's package manager (for example apt install git), or set REFRACT_GIT_BIN"
	default:
		return "install it, or set " + EnvVar(name)
	}
}
