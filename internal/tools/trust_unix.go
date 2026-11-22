//go:build unix

package tools

import (
	"fmt"
	"os"
	"syscall"
)

// unsafe says why p can't be trusted to hold programs Refract runs: it is
// writable by its group or by others, or owned by someone other than the
// current user or root. It returns "" when p is fine or can't be inspected.
func unsafe(p string) string {
	fi, err := os.Stat(p)
	if err != nil {
		return ""
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Sprintf("%s is writable by group or others", p)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		if uid := uint32(os.Geteuid()); st.Uid != uid && st.Uid != 0 {
			return fmt.Sprintf("%s is owned by another user", p)
		}
	}
	return ""
}
