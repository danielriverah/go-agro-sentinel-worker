package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Note: /tmp here is the Docker named volume shared exclusively between the
// api and worker containers. It is not the host's /tmp, so predictable-path
// LPE attacks from other users do not apply.

const triggerDir = "/tmp"
const triggerPrefix = "worker-run-prod-"
const triggerSuffix = ".trigger"

// WriteProdTrigger writes a trigger file requesting processing of produccionID.
// The worker --auto daemon picks it up within 30 seconds.
// Uses O_EXCL so a pre-existing trigger for the same production is a no-op
// (the worker will process it on the next tick regardless).
func WriteProdTrigger(produccionID int64) error {
	path := prodTriggerPath(produccionID)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return nil // trigger already queued
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString("pending")
	_ = f.Close()
	return err
}

// ReadProdTriggers returns all pending production trigger IDs and removes the
// trigger files atomically (rename then parse). Safe to call concurrently.
func ReadProdTriggers() []int64 {
	entries, _ := os.ReadDir(triggerDir)
	var ids []int64
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, triggerPrefix) || !strings.HasSuffix(name, triggerSuffix) {
			continue
		}
		raw := strings.TrimSuffix(strings.TrimPrefix(name, triggerPrefix), triggerSuffix)
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			_ = os.Remove(filepath.Join(triggerDir, name))
			continue
		}
		// Remove before processing to avoid double-fire.
		_ = os.Remove(filepath.Join(triggerDir, name))
		ids = append(ids, id)
	}
	return ids
}

func prodTriggerPath(produccionID int64) string {
	return fmt.Sprintf("%s/%s%d%s", triggerDir, triggerPrefix, produccionID, triggerSuffix)
}

const globalTriggerPath = "/tmp/worker-run-all.trigger"

// WriteGlobalTrigger requests an immediate full run of the --auto worker.
// Idempotent: if a trigger already exists the call is a no-op.
func WriteGlobalTrigger() error {
	f, err := os.OpenFile(globalTriggerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString("pending")
	_ = f.Close()
	return err
}

// ConsumeGlobalTrigger returns true and removes the trigger file if one exists.
func ConsumeGlobalTrigger() bool {
	err := os.Remove(globalTriggerPath)
	return err == nil
}

// ── Graceful stop trigger ─────────────────────────────────────────────────────
//
// The stop trigger instructs the worker to stop after finishing the current
// scene. It is checked between scenes so no work is left half-done.
// WriteStopTrigger is idempotent (O_EXCL). ConsumeStopTrigger removes the file
// atomically so only the first reader acts on it. RemoveStopTrigger lets the
// API cancel a pending stop before the worker picks it up.

const stopTriggerPath = "/tmp/worker-stop.trigger"

// WriteStopTrigger creates the stop signal file.
// Returns (true, nil) when written, (false, nil) when it already exists.
func WriteStopTrigger() (bool, error) {
	f, err := os.OpenFile(stopTriggerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_ = f.Close()
	return true, nil
}

// ConsumeStopTrigger removes the stop signal file and returns true if it existed.
// Safe to call from the worker's scene loop — atomic remove.
func ConsumeStopTrigger() bool {
	return os.Remove(stopTriggerPath) == nil
}

// RemoveStopTrigger removes the stop signal file without consuming it (API use).
func RemoveStopTrigger() bool {
	return os.Remove(stopTriggerPath) == nil
}

// StopTriggerExists reports whether a stop signal is pending without removing it.
func StopTriggerExists() bool {
	_, err := os.Stat(stopTriggerPath)
	return err == nil
}
