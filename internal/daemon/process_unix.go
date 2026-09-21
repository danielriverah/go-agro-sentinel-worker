//go:build !windows

package daemon

import (
	"os"
	"syscall"
)

// processRunning returns true when a process with the given PID exists.
func processRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
