package worker

import "time"

// ProgressReporter receives processing events so the caller (main.go) can
// write status updates to the daemon state file without the worker package
// depending on the daemon package.
type ProgressReporter interface {
	SetTotal(total int)
	SetPhase(phase string)
	SceneStarted(scene string, produccionID int64)
	SceneCompleted(scene string, produccionID int64)
	SceneFailed(scene string, produccionID int64)
	SetNextSchedule(t time.Time)
}

// noopReporter is used when no reporter is wired up.
type noopReporter struct{}

func (noopReporter) SetTotal(int)                          {}
func (noopReporter) SetPhase(string)                       {}
func (noopReporter) SceneStarted(string, int64)            {}
func (noopReporter) SceneCompleted(string, int64)          {}
func (noopReporter) SceneFailed(string, int64)             {}
func (noopReporter) SetNextSchedule(time.Time)             {}
