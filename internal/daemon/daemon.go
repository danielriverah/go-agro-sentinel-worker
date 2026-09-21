// Package daemon provides a file-based lock and scheduled-run helpers used by
// long-running worker processes to prevent overlapping executions and to sleep
// until a configured schedule.
//
// Lock hierarchy for the worker process:
//
//	Global lock  (/tmp/worker-global.lock)
//	  Held by --auto. While held, ALL other worker commands are rejected.
//
//	Production lock  (/tmp/worker-prod-{id}.lock)
//	  Held by --production, --scene, and --regen for the duration of that run.
//	  --auto acquires the global lock first, so production locks are never
//	  needed alongside it — but they protect against two manual commands
//	  targeting the same production at the same time.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	globalLockPath     = "/tmp/worker-global.lock"
	productionLockPath = "/tmp/worker-prod-%d.lock"
)

// ErrGlobalLocked is returned when --auto is already running.
var ErrGlobalLocked = errors.New("worker --auto is already running; wait for it to finish before running other commands")

// ErrProductionLocked is returned when the same production is already being processed.
var ErrProductionLocked = errors.New("this production is already being processed by another worker instance")

// CleanStaleProductionLocks removes per-production lock files that are stale:
// either the PID is not running, or the PID is 1 (init — never a real worker),
// or the lock belongs to a different container (same PID, different /proc/cmdline).
// Call at startup of --run-all or --auto to recover from locks left by killed containers.
// CleanStaleGlobalLock removes the global lock file if the PID it contains is
// not a running worker process. Call at startup to recover from a killed --run-all.
func CleanStaleGlobalLock() bool {
	data, err := os.ReadFile(globalLockPath)
	if err != nil {
		return false
	}
	pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if parseErr != nil || pid <= 1 || !isWorkerProcess(pid) {
		_ = os.Remove(globalLockPath)
		_ = os.Remove(strings.TrimSuffix(globalLockPath, ".lock") + ".state")
		return true
	}
	return false
}

func CleanStaleProductionLocks() int {
	entries, _ := os.ReadDir("/tmp")
	removed := 0
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "worker-prod-") || !strings.HasSuffix(e.Name(), ".lock") {
			continue
		}
		path := "/tmp/" + e.Name()
		data, err := os.ReadFile(path)
		if err != nil {
			_ = os.Remove(path)
			removed++
			continue
		}
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		// Stale if: unparseable, PID=1 (init, never a real worker), not running,
		// or running but the process is not a worker binary.
		if parseErr != nil || pid <= 1 || !isWorkerProcess(pid) {
			_ = os.Remove(path)
			_ = os.Remove(strings.TrimSuffix(path, ".lock") + ".state")
			removed++
		}
	}
	return removed
}

// isWorkerProcess returns true only when pid is running AND its cmdline
// contains "worker" — distinguishing real worker processes from init (PID 1)
// or other processes that happen to reuse a PID after a container restart.
func isWorkerProcess(pid int) bool {
	if !processRunning(pid) {
		return false
	}
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		// /proc not available (non-Linux) — fall back to running check only.
		return true
	}
	return strings.Contains(string(cmdline), "worker")
}

// AcquireGlobal acquires the global worker lock used by --auto.
// Returns ErrGlobalLocked if another --auto is running.
// Returns ErrProductionLocked if any --production / --scene / --regen command
// is currently running (--auto refuses to race with manual runs).
func AcquireGlobal() (*Lock, error) {
	// Scan for any live production lock files before acquiring the global lock.
	entries, _ := os.ReadDir("/tmp")
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "worker-prod-") {
			continue
		}
		path := "/tmp/" + e.Name()
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil && pid > 0 && processRunning(pid) {
			return nil, fmt.Errorf("%w: production lock held by PID %d (%s)", ErrProductionLocked, pid, e.Name())
		}
		// Stale lock — remove it.
		_ = os.Remove(path)
	}

	return Acquire(globalLockPath)
}

// AcquireProduction acquires a per-production lock.
// Returns ErrGlobalLocked if --auto is currently running.
// Returns ErrProductionLocked if the same production is already being processed.
func AcquireProduction(produccionID int64) (*Lock, error) {
	// Refuse if --auto holds the global lock.
	if data, err := os.ReadFile(globalLockPath); err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil && pid > 0 && processRunning(pid) {
			return nil, ErrGlobalLocked
		}
		// Stale global lock — clean it up.
		_ = os.Remove(globalLockPath)
	}

	path := fmt.Sprintf(productionLockPath, produccionID)
	lock, err := Acquire(path)
	if err != nil {
		return nil, err
	}
	if lock == nil {
		return nil, ErrProductionLocked
	}
	return lock, nil
}

// Lock represents an acquired process lock backed by a PID file.
type Lock struct {
	path string
}

// Acquire tries to acquire a lock at path. If a previous PID file exists and
// that process is still running the lock is not granted and (nil, nil) is
// returned — the caller should skip this run. Any other error is fatal.
func Acquire(path string) (*Lock, error) {
	// Check whether an existing lock is held by a live process.
	if data, err := os.ReadFile(path); err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil && pid > 0 && processRunning(pid) {
			return nil, nil // already running — skip
		}
		// Stale lock file — remove it.
		_ = os.Remove(path)
	}

	// Write our PID.
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return nil, fmt.Errorf("writing lock file %s: %w", path, err)
	}
	return &Lock{path: path}, nil
}


// NextSchedule returns the duration until the next occurrence of one of the
// given clock hours (0-23) in the given location. The nearest future hour
// wins; if two are equally close the earlier one is preferred.
// Pass time.UTC for UTC, or load a named zone (e.g. "America/Mexico_City").
func NextSchedule(hours []int, loc *time.Location) time.Duration {
	now := time.Now().In(loc)
	var best time.Duration = -1

	for _, h := range hours {
		next := time.Date(now.Year(), now.Month(), now.Day(), h, 0, 0, 0, loc)
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		d := time.Until(next)
		if best < 0 || d < best {
			best = d
		}
	}
	return best
}

// MustLoadLocation loads a time zone by name and panics if it cannot be found.
// Use a tz database name such as "America/Mexico_City" or "UTC".
func MustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(fmt.Sprintf("daemon: unknown time zone %q: %v", name, err))
	}
	return loc
}
