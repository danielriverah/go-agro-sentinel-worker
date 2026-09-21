package aws

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
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
	client         *s3.Client
	presign        *s3.PresignClient
	publicEndpoint string // optional: rewrite presigned URLs for browser access (e.g. "http://localhost:4566")
}

// NewS3Client builds an S3Client from an already-resolved AWS SDK config.
// UsePathStyle is always enabled so that LocalStack (and any other endpoint
// override) receives requests as http://host/bucket/key instead of the
// virtual-hosted http://bucket.host/key — the latter requires wildcard DNS
// that is unavailable in Docker networks.
func NewS3Client(awsCfg awssdk.Config) *S3Client {
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	return &S3Client{
		client:  client,
		presign: s3.NewPresignClient(client),
	}
}

// WithPublicEndpoint returns a copy of the client that rewrites presigned URLs
// so browsers can reach them via publicEndpoint instead of the internal Docker
// hostname. Example: "http://localhost:4566".
func (c *S3Client) WithPublicEndpoint(publicEndpoint string) *S3Client {
	cp := *c
	cp.publicEndpoint = publicEndpoint
	return &cp
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
// When publicEndpoint is set the internal hostname in the URL is replaced so
// browsers can reach LocalStack via the host machine (e.g. localhost:4566).
func (c *S3Client) PresignGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	req, err := c.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("presigning s3://%s/%s", bucket, key), Wrapped: err}
	}

	rawURL := req.URL
	if c.publicEndpoint != "" {
		rawURL = rewriteHost(rawURL, c.publicEndpoint)
	}
	return rawURL, nil
}

// rewriteHost replaces the scheme+host of rawURL with the values from publicEndpoint.
// Query string and path are preserved so presign signatures remain valid.
func rewriteHost(rawURL, publicEndpoint string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	pub, err := url.Parse(publicEndpoint)
	if err != nil {
		return rawURL
	}
	parsed.Scheme = pub.Scheme
	parsed.Host = pub.Host
	return parsed.String()
}

// S3ObjectInfo holds the metadata returned by ListObjects for a single key.
type S3ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// BucketExists reports whether the bucket is accessible. Returns false (not an
// error) when the bucket does not exist or is not reachable.
func (c *S3Client) BucketExists(ctx context.Context, bucket string) (bool, error) {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: awssdk.String(bucket),
	})
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ListObjects returns all objects whose key starts with prefix inside bucket.
func (c *S3Client) ListObjects(ctx context.Context, bucket, prefix string) ([]S3ObjectInfo, error) {
	var results []S3ObjectInfo

	paginator := s3.NewListObjectsV2Paginator(c.client, &s3.ListObjectsV2Input{
		Bucket: awssdk.String(bucket),
		Prefix: awssdk.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("listing s3://%s/%s", bucket, prefix), Wrapped: err}
		}
		for _, obj := range page.Contents {
			info := S3ObjectInfo{Key: awssdk.ToString(obj.Key)}
			if obj.Size != nil {
				info.Size = *obj.Size
			}
			if obj.LastModified != nil {
				info.LastModified = *obj.LastModified
			}
			results = append(results, info)
		}
	}
	return results, nil
}

// GetObjectContent downloads bucket/key and returns its bytes.
// Intended for small files (JSON). Returns nil content when the object does
// not exist so callers can treat missing files gracefully.
func (c *S3Client) GetObjectContent(ctx context.Context, bucket, key string) ([]byte, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("reading s3://%s/%s", bucket, key), Wrapped: err}
	}
	defer out.Body.Close()

	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 4096)
	for {
		n, readErr := out.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if readErr != nil {
			break
		}
	}
	return buf, nil
}

// GetObjectStream opens bucket/key for streaming. Returns nil body when the object does not exist.
// The caller must close body when not nil.
func (c *S3Client) GetObjectStream(ctx context.Context, bucket, key string) (body io.ReadCloser, contentType string, contentLength int64, err error) {
	out, s3err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: awssdk.String(bucket),
		Key:    awssdk.String(key),
	})
	if s3err != nil {
		if isNotFound(s3err) {
			return nil, "", 0, nil
		}
		return nil, "", 0, &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("streaming s3://%s/%s", bucket, key), Wrapped: s3err}
	}
	ct := ""
	if out.ContentType != nil {
		ct = *out.ContentType
	}
	cl := int64(0)
	if out.ContentLength != nil {
		cl = *out.ContentLength
	}
	return out.Body, ct, cl, nil
}

// HeadBucket checks if a bucket exists and is accessible.
func (c *S3Client) HeadBucket(ctx context.Context, bucket string) error {
	_, err := c.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: awssdk.String(bucket),
	})
	if err != nil {
		return fmt.Errorf("checking s3 bucket %s: %w", bucket, err)
	}
	return nil
}

// BuildKey builds the canonical S3 key for a scene file: prefix/produccionID/sceneID/fileName.
func BuildKey(prefix string, produccionID int64, sceneID string, fileName string) string {
	return fmt.Sprintf("%s/%d/%s/%s", prefix, produccionID, sceneID, fileName)
}

// BuildKey is also exposed as a method on S3Client for interface convenience.
func (c *S3Client) BuildKey(prefix string, produccionID int64, sceneID string, fileName string) string {
	return BuildKey(prefix, produccionID, sceneID, fileName)
}
