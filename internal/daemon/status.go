package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Phase values for WorkerStatus.
const (
	PhaseProcessing = "processing"
	PhaseSleeping   = "sleeping"
	PhaseIdle       = "idle"
)

// WorkerStatus is the JSON state written alongside each lock file.
// The API reads these files to report progress to the frontend.
type WorkerStatus struct {
	Mode                string     `json:"mode"`
	PID                 int        `json:"pid"`
	StartedAt           time.Time  `json:"started_at"`
	Phase               string     `json:"phase"`
	CurrentScene        string     `json:"current_scene,omitempty"`
	CurrentProduccionID int64      `json:"current_produccion_id,omitempty"`
	ScenesDone          int        `json:"scenes_done"`
	ScenesFailed        int        `json:"scenes_failed"`
	ScenesTotal         int        `json:"scenes_total,omitempty"` // 0 = unknown (--auto)
	NextScheduleAt      *time.Time `json:"next_schedule_at,omitempty"`
	LastCompletedAt     *time.Time `json:"last_completed_at,omitempty"`
}

// statePath returns the state file path for a given lock path.
func statePath(lockPath string) string {
	ext := filepath.Ext(lockPath)
	return strings.TrimSuffix(lockPath, ext) + ".state"
}

// WriteStatus serializes status to the state file associated with l.
func (l *Lock) WriteStatus(s WorkerStatus) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_ = os.WriteFile(statePath(l.path), data, 0o644)
}

// Release removes both the PID file and the state file.
func (l *Lock) Release() {
	_ = os.Remove(l.path)
	_ = os.Remove(statePath(l.path))
}

const schedulerStatePath = "/tmp/worker-scheduler.state"

// WriteSchedulerState persists the scheduler's idle state (phase=sleeping)
// without acquiring the global lock. This allows manual runs to proceed
// while the cron is waiting between fires.
func WriteSchedulerState(next time.Time) {
	s := WorkerStatus{
		Mode:           "auto",
		PID:            os.Getpid(),
		StartedAt:      time.Now(),
		Phase:          PhaseSleeping,
		NextScheduleAt: &next,
	}
	data, _ := json.Marshal(s)
	_ = os.WriteFile(schedulerStatePath, data, 0o644)
}

// UpdateSchedulerStateAfterRun updates last_completed_at and next_schedule_at
// in the scheduler state file after a cron run finishes.
func UpdateSchedulerStateAfterRun(next time.Time) {
	now := time.Now()
	s := WorkerStatus{
		Mode:            "auto",
		PID:             os.Getpid(),
		Phase:           PhaseSleeping,
		NextScheduleAt:  &next,
		LastCompletedAt: &now,
	}
	data, _ := json.Marshal(s)
	_ = os.WriteFile(schedulerStatePath, data, 0o644)
}

// AdvanceSchedulerNextRun updates only next_schedule_at in the scheduler state,
// preserving last_completed_at. Used when a cron tick fires but the run is
// skipped (lock busy, error), so the displayed next time stays accurate.
func AdvanceSchedulerNextRun(next time.Time) {
	data, err := os.ReadFile(schedulerStatePath)
	var s WorkerStatus
	if err == nil {
		_ = json.Unmarshal(data, &s)
	}
	s.NextScheduleAt = &next
	if out, err := json.Marshal(s); err == nil {
		_ = os.WriteFile(schedulerStatePath, out, 0o644)
	}
}

// ClearSchedulerState removes the scheduler state file on shutdown.
func ClearSchedulerState() {
	_ = os.Remove(schedulerStatePath)
}

// ReadGlobalStatus reads the global (--auto) worker state.
// Returns nil if no global process is running or state file is absent.
func ReadGlobalStatus() *WorkerStatus {
	return readStatus(globalLockPath)
}

// ReadProductionStatus reads the state for a specific production lock.
func ReadProductionStatus(produccionID int64) *WorkerStatus {
	return readStatus(fmt.Sprintf(productionLockPath, produccionID))
}

// ReadAllStatuses returns the global status (may be nil) and all active
// per-production statuses, cleaning up stale lock files along the way.
// When the global lock is not held (cron sleeping), falls back to the
// scheduler state file so the API can still report next_schedule_at.
func ReadAllStatuses() (global *WorkerStatus, productions []*WorkerStatus) {
	global = ReadGlobalStatus()
	if global == nil {
		// No active lock — check if the scheduler is sleeping.
		if data, err := os.ReadFile(schedulerStatePath); err == nil {
			var s WorkerStatus
			if json.Unmarshal(data, &s) == nil {
				global = &s
			}
		}
	}

	entries, _ := os.ReadDir("/tmp")
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "worker-prod-") || !strings.HasSuffix(e.Name(), ".lock") {
			continue
		}
		lockPath := "/tmp/" + e.Name()
		s := readStatus(lockPath)
		if s != nil {
			productions = append(productions, s)
		}
	}
	return
}

// ForceUnlock removes the lock and state files for the given lock path
// regardless of whether the process is still running. Use only when the
// process is confirmed dead and the lock is stale.
func ForceUnlock(lockPath string) {
	_ = os.Remove(lockPath)
	_ = os.Remove(statePath(lockPath))
}

// GlobalLockPath returns the path used for the global (--auto) lock.
func GlobalLockPath() string { return globalLockPath }

// ProductionLockPathFor returns the lock path for the given production ID.
func ProductionLockPathFor(produccionID int64) string {
	return fmt.Sprintf(productionLockPath, produccionID)
}

// readStatus reads a WorkerStatus from the state file next to lockPath.
// Returns nil if the lock is stale (process no longer running) or absent.
func readStatus(lockPath string) *WorkerStatus {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil
	}
	pid, parseErr := parseInt(strings.TrimSpace(string(data)))
	if parseErr != nil || !processRunning(pid) {
		// Stale — clean up.
		_ = os.Remove(lockPath)
		_ = os.Remove(statePath(lockPath))
		return nil
	}

	stateData, err := os.ReadFile(statePath(lockPath))
	if err != nil {
		// Lock exists but no state file yet (process just started).
		return &WorkerStatus{PID: pid, Phase: PhaseProcessing}
	}
	var s WorkerStatus
	if err := json.Unmarshal(stateData, &s); err != nil {
		return &WorkerStatus{PID: pid, Phase: PhaseProcessing}
	}
	return &s
}

func parseInt(s string) (int, error) {
	var v int
	_, err := fmt.Sscan(s, &v)
	return v, err
}
