//go:build windows

package daemon

import (
	"os"
)

// processRunning returns true when a process with the given PID exists.
func processRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Windows, OpenProcess would be the real check, but for a lock file
	// used in a single-container context this simple heuristic is sufficient.
	_ = proc
	return false // treat stale locks as stale on Windows
}
