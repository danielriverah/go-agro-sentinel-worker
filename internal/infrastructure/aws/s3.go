package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"agro-sentinel-worker/internal/domain"
)

// isNotFound reports whether err represents a "not found" response from S3
// (either a typed NotFound error, or an HTTP 404 response error, which is
// what HeadObject returns instead of a typed error).
func isNotFound(err error) bool {
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}

	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		return respErr.HTTPStatusCode() == 404
	}

	return false
}

// S3Client wraps the AWS SDK v2 S3 client with the operations needed by the
// worker: upload/download of local files, existence checks and presigned
// GET URLs.
type S3Client struct {
	client  *s3.Client
	presign *s3.PresignClient
}

// NewS3Client builds an S3Client from an already-resolved AWS SDK config.
func NewS3Client(awsCfg awssdk.Config) *S3Client {
	client := s3.NewFromConfig(awsCfg)
	return &S3Client{
		client:  client,
		presign: s3.NewPresignClient(client),
	}
}

// Upload puts the contents of filePath into bucket/key.
func (c *S3Client) Upload(ctx context.Context, bucket, key, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("opening file %s", filePath), Wrapped: err}
	}
	defer f.Close()

	_, err = c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
		Body:   f,
	})
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("uploading s3://%s/%s", bucket, key), Wrapped: err}
	}

	return nil
}

// Download fetches bucket/key and writes it to destPath.
func (c *S3Client) Download(ctx context.Context, bucket, key, destPath string) error {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
	})
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("downloading s3://%s/%s", bucket, key), Wrapped: err}
	}
	defer out.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("creating file %s", destPath), Wrapped: err}
	}
	defer f.Close()

	if _, err := f.ReadFrom(out.Body); err != nil {
		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("writing file %s", destPath), Wrapped: err}
	}

	return nil
}

// HeadObject reports whether bucket/key exists and, if so, its size.
func (c *S3Client) HeadObject(ctx context.Context, bucket, key string) (bool, int64, error) {
	out, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return false, 0, nil
		}
		return false, 0, &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("head s3://%s/%s", bucket, key), Wrapped: err}
	}

	size := int64(0)
	if out.ContentLength != nil {
		size = *out.ContentLength
	}

	return true, size, nil
}

// PresignGetObject returns a presigned GET URL for bucket/key valid for expiry.
func (c *S3Client) PresignGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	req, err := c.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("presigning s3://%s/%s", bucket, key), Wrapped: err}
	}

	return req.URL, nil
}

// BuildKey builds the canonical S3 key for a scene file: prefix/produccionID/sceneID/fileName.
func BuildKey(prefix string, produccionID int64, sceneID string, fileName string) string {
	return fmt.Sprintf("%s/%d/%s/%s", prefix, produccionID, sceneID, fileName)
}

// BuildKey is also exposed as a method on S3Client for interface convenience.
func (c *S3Client) BuildKey(prefix string, produccionID int64, sceneID string, fileName string) string {
	return BuildKey(prefix, produccionID, sceneID, fileName)
}
