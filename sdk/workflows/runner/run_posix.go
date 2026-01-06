//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

// setCmdSysProcAttr configures the command to run in its own process group on POSIX systems.
func setCmdSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree attempts to kill the process group on POSIX systems.
func killProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// Negative PID sends the signal to the entire process group.
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		// Fallback to killing just the process
		return cmd.Process.Kill()
	}
	return nil
}
