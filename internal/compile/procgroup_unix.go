//go:build unix

package compile

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup starts cmd in its own process group and makes cancelling
// kill the whole group, so helpers tectonic spawned don't outlive a timeout.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
