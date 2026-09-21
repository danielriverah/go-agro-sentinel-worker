package health

import (
	"context"
	"database/sql"
	"fmt"

	"agro-sentinel-worker/internal/domain"
	awsclient "agro-sentinel-worker/internal/infrastructure/aws"
)

// CheckMySQL verifies MySQL connectivity and accessibility.
func CheckMySQL(ctx context.Context, db *sql.DB) error {
	if err := db.PingContext(ctx); err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrMySQL,
			Message: "health check failed",
			Wrapped: err,
		}
	}
	return nil
}

// CheckDynamoDB verifies DynamoDB connectivity and accessibility.
func CheckDynamoDB(ctx context.Context, client *awsclient.DynamoDBClient, tableName string) error {
	if err := client.DescribeTable(ctx, tableName); err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrDynamoDB,
			Message: fmt.Sprintf("health check for table %s failed", tableName),
			Wrapped: err,
		}
	}
	return nil
}

// CheckS3 verifies S3 connectivity and bucket accessibility using the wrapped client.
func CheckS3(ctx context.Context, client *awsclient.S3Client, bucket string) error {
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return &domain.ProcessingError{
			Type:    domain.ErrS3,
			Message: fmt.Sprintf("health check for bucket %s failed", bucket),
			Wrapped: err,
		}
	}
	if !exists {
		return &domain.ProcessingError{
			Type:    domain.ErrS3,
			Message: fmt.Sprintf("bucket %s does not exist or is not accessible", bucket),
		}
	}
	return nil
}
