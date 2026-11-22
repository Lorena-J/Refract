//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A pipe where the key should be is refused instead of blocking startup.
func TestLoadOrCreateKeyFIFO(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), localKeyFile)
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip(err)
	}
	done := make(chan error, 1)
	go func() { _, err := loadOrCreateKey(fifo); done <- err }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading a fifo as the key blocked")
	}
}

func TestReadEmbedTokenFromFD(t *testing.T) {
	r, w, _ := os.Pipe()
	w.WriteString(testEmbedToken + "\n")
	w.Close()
	t.Setenv(embedTokenEnv, "ignored-ignored-ignored")
	fd, err := syscall.Dup(int(r.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	tok, err := readEmbedToken(fd, os.Getenv, os.Unsetenv)
	if err != nil || tok != testEmbedToken {
		t.Fatal(tok, err)
	}
}
