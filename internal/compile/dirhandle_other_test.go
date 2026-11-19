//go:build !unix

package compile

import (
	"os"
	"testing"
)

// openTestDir opens dir as the helpers' folder handle.
func openTestDir(t *testing.T, dir string) *dirHandle {
	t.Helper()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return &dirHandle{r}
}
