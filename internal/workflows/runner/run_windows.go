//go:build windows

package runner

import (
	"os/exec"
	"syscall"
)

// setCmdSysProcAttr configures the command for Windows. We avoid fields not available across versions.
func setCmdSysProcAttr(cmd *exec.Cmd) {
	// Create a new process group on Windows so we can terminate child processes if needed.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killProcessTree attempts to kill the process on Windows. Killing the entire tree requires job objects;
// here we best-effort kill the main process. Future: use golang.org/x/sys/windows and JobObjects if needed.
func killProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
