package domain

import "fmt"

type ErrType string

const (
	ErrGDAL       ErrType = "GDAL_ERROR"
	ErrS3         ErrType = "S3_ERROR"
	ErrTimeout    ErrType = "TIMEOUT"
	ErrValidation ErrType = "VALIDATION_ERROR"
	ErrSTAC       ErrType = "STAC_ERROR"
	ErrIA         ErrType = "IA_ERROR"
	ErrMySQL      ErrType = "MYSQL_ERROR"
	ErrDynamoDB   ErrType = "DYNAMODB_ERROR"
	ErrDisk       ErrType = "DISK_ERROR"
	ErrSQS        ErrType = "SQS_ERROR"
)

type ProcessingError struct {
	Type    ErrType
	Message string
	Wrapped error
}

func (e *ProcessingError) Error() string {
	if e.Wrapped != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Wrapped)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

func (e *ProcessingError) Unwrap() error {
	return e.Wrapped
}
