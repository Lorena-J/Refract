package projects

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// git runs the git CLI in dir, isolated from any system/global config and
// with hooks disabled: a project's content must never make git execute code.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	base := []string{
		"-c", "core.hooksPath=/dev/null",
		"-c", "user.name=Refract",
		"-c", "user.email=refract@localhost",
		"-c", "commit.gpgsign=false",
		"-c", "init.defaultBranch=main",
	}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w: %s", args, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out.String(), nil
}

// initRepo makes dir a git repo with one commit of its current content.
// Build output (.refract/) is excluded via .git/info/exclude so no file is
// added to the user's project.
func initRepo(ctx context.Context, dir, message string) error {
	if _, err := git(ctx, dir, "init", "-q"); err != nil {
		return err
	}
	info := filepath.Join(dir, ".git", "info")
	if err := os.MkdirAll(info, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(info, "exclude"), []byte(".refract/\n"), 0o644); err != nil {
		return err
	}
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return err
	}
	_, err := git(ctx, dir, "commit", "-q", "--allow-empty", "-m", message)
	return err
}

// Checkpoint commits the working tree if anything changed, and reports whether it did.
func (s *Service) Checkpoint(ctx context.Context, id, message string) (bool, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return false, err
	}
	dir := s.Path(id)
	if _, err := git(ctx, dir, "add", "-A"); err != nil {
		return false, err
	}
	if out, err := git(ctx, dir, "status", "--porcelain"); err != nil || out == "" {
		return false, err
	}
	_, err := git(ctx, dir, "commit", "-q", "-m", message)
	return err == nil, err
}
