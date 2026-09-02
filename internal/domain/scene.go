package domain

import "time"

type JobStatus string

const (
	StatusPending    JobStatus = "PENDING"
	StatusProcessing JobStatus = "PROCESSING"
	StatusCompleted  JobStatus = "COMPLETED"
	StatusFailed     JobStatus = "FAILED"
)

type Scene struct {
	ID              int64
	ProduccionID    int64
	SceneID         string
	SceneDate       time.Time
	CloudCoverScene float64
	CloudCoverBBox  *float64
	PassesQuality   bool
	HasMultiband    bool
	HasParams       bool
	HasRGB          bool
	HasAnalisis     bool
	Status          JobStatus
	ErrorType       string
	ErrorMessage    string
	RetryCount      int
	ProcessedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (s *Scene) NeedsProcessing() bool {
	return s.Status == StatusPending
}

func (s *Scene) CanRetry(maxRetries int) bool {
	return s.Status == StatusFailed && s.RetryCount < maxRetries
}

type SceneFile struct {
	ID            int64
	EscenaID      int64
	FileType      FileType
	FileName      string
	S3Key         string
	S3Bucket      string
	FileSizeBytes int64
	ResolutionM   int
	WidthPx       int
	HeightPx      int
	CreatedAt     time.Time
}
