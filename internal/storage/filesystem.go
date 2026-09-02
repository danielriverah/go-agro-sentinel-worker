// Package storage provides filesystem layout helpers for job processing.
package storage

import (
	"os"
	"path/filepath"
)

// JobDir represents the on-disk directory layout for a single job:
//
//	{baseDir}/jobs/{jobID}/input/
//	{baseDir}/jobs/{jobID}/work/
//	{baseDir}/jobs/{jobID}/output/
type JobDir struct {
	baseDir string
	jobID   string
}

// New creates a JobDir rooted at baseDir for the given jobID.
func New(baseDir string, jobID string) *JobDir {
	return &JobDir{baseDir: baseDir, jobID: jobID}
}

// root returns {baseDir}/jobs/{jobID}.
func (j *JobDir) root() string {
	return filepath.Join(j.baseDir, "jobs", j.jobID)
}

// Root returns {baseDir}/jobs/{jobID}, the job's top-level directory. It is
// the path processing functions that lay out their own work/output
// subdirectories (e.g. processing.MultibandBuilder.Build) expect.
func (j *JobDir) Root() string {
	return j.root()
}

// Input returns {baseDir}/jobs/{jobID}/input/.
func (j *JobDir) Input() string {
	return filepath.Join(j.root(), "input")
}

// Work returns {baseDir}/jobs/{jobID}/work/.
func (j *JobDir) Work() string {
	return filepath.Join(j.root(), "work")
}

// Output returns {baseDir}/jobs/{jobID}/output/.
func (j *JobDir) Output() string {
	return filepath.Join(j.root(), "output")
}

// Create creates the input, work, and output subdirectories.
func (j *JobDir) Create() error {
	for _, dir := range []string{j.Input(), j.Work(), j.Output()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Cleanup removes the entire job directory tree.
func (j *JobDir) Cleanup() error {
	return os.RemoveAll(j.root())
}
