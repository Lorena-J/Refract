//go:build unix

package compile

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// A FIFO left in the build folder (the document is untrusted) is not read: it would block.
func TestReadInRefusesFIFO(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "main.log"), 0o644); err != nil {
		t.Skip(err)
	}
	d, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := readIn(d, "main.log"); err == nil {
		t.Error("read a FIFO")
	}
}
