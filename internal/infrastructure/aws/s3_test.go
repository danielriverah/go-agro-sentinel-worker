package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"agro-sentinel-worker/internal/config"
)

func requireLocalstack(t *testing.T) config.AWSConfig {
	t.Helper()
	endpoint := os.Getenv("AWS_ENDPOINT_URL")
	if endpoint == "" {
		t.Skip("AWS_ENDPOINT_URL not set, skipping test that requires localstack")
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	return config.AWSConfig{Region: region, Endpoint: endpoint}
}

func TestS3Client_UploadHeadDownloadPresign(t *testing.T) {
	cfg := requireLocalstack(t)

	awsCfg, err := NewSession(cfg)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	bucket := "agro-sentinel-worker-test"
	ctx := context.Background()

	rawClient := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })
	// Ignore the error: the bucket may already exist from a previous test run.
	_, _ = rawClient.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &bucket})

	client := &S3Client{client: rawClient, presign: s3.NewPresignClient(rawClient)}

	dir := t.TempDir()
	srcPath := filepath.Join(dir, "upload.txt")
	content := []byte("hello agro sentinel")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatalf("writing source file: %v", err)
	}

	key := client.BuildKey("sentinel/producciones", 123, "scene-1", "upload.txt")

	if err := client.Upload(ctx, bucket, key, srcPath); err != nil {
		t.Fatalf("Upload: %v", err)
	}

	exists, size, err := client.HeadObject(ctx, bucket, key)
	if err != nil {
		t.Fatalf("HeadObject: %v", err)
	}
	if !exists {
		t.Fatalf("HeadObject: expected object to exist")
	}
	if size != int64(len(content)) {
		t.Errorf("HeadObject size = %d, want %d", size, len(content))
	}

	existsMissing, _, err := client.HeadObject(ctx, bucket, "does/not/exist.txt")
	if err != nil {
		t.Fatalf("HeadObject (missing): %v", err)
	}
	if existsMissing {
		t.Errorf("HeadObject: expected missing object to not exist")
	}

	destPath := filepath.Join(dir, "download.txt")
	if err := client.Download(ctx, bucket, key, destPath); err != nil {
		t.Fatalf("Download: %v", err)
	}

	got, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("downloaded content = %q, want %q", got, content)
	}

	url, err := client.PresignGetObject(ctx, bucket, key, 5*time.Minute)
	if err != nil {
		t.Fatalf("PresignGetObject: %v", err)
	}
	if url == "" {
		t.Errorf("PresignGetObject: expected non-empty URL")
	}
}

func TestBuildKey(t *testing.T) {
	got := BuildKey("sentinel/producciones", 42, "scene-abc", "band.tif")
	want := "sentinel/producciones/42/scene-abc/band.tif"
	if got != want {
		t.Errorf("BuildKey() = %q, want %q", got, want)
	}
}
