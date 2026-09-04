diff --git a/internal/config/config.go b/internal/config/config.go
index 3974bad..4a7ade2 100644
--- a/internal/config/config.go
+++ b/internal/config/config.go
@@ -45,21 +45,22 @@ type ProcessingConfig struct {
 	Workers              int    `yaml:"workers"`
 	DownloadsConcurrency int    `yaml:"downloads_concurrency"`
 }
 
 type SentinelConfig struct {
 	CloudCoverSceneMax      float64 `yaml:"cloud_cover_scene_max"`
 	CloudCoverProductionMax float64 `yaml:"cloud_cover_production_max"`
 }
 
 type AWSConfig struct {
-	Region string `yaml:"region"`
+	Region   string `yaml:"region"`
+	Endpoint string `yaml:"endpoint"`
 }
 
 type S3Config struct {
 	Bucket string `yaml:"bucket"`
 	Prefix string `yaml:"prefix"`
 }
 
 type SQSConfig struct {
 	QueueURL string `yaml:"queue_url"`
 }
@@ -105,20 +106,23 @@ func Load(path string) (*Config, error) {
 
 	applyEnvOverrides(&cfg)
 
 	return &cfg, nil
 }
 
 func applyEnvOverrides(cfg *Config) {
 	if v := os.Getenv("AWS_REGION"); v != "" {
 		cfg.AWS.Region = v
 	}
+	if v := os.Getenv("AWS_ENDPOINT_URL"); v != "" {
+		cfg.AWS.Endpoint = v
+	}
 	if v := os.Getenv("S3_BUCKET"); v != "" {
 		cfg.S3.Bucket = v
 	}
 	if v := os.Getenv("SQS_QUEUE_URL"); v != "" {
 		cfg.SQS.QueueURL = v
 	}
 	if v := os.Getenv("MYSQL_HOST"); v != "" {
 		cfg.MySQL.Host = v
 	}
 	if v := os.Getenv("MYSQL_DATABASE"); v != "" {
diff --git a/internal/infrastructure/aws/dynamodb.go b/internal/infrastructure/aws/dynamodb.go
new file mode 100644
index 0000000..75c0ee7
--- /dev/null
+++ b/internal/infrastructure/aws/dynamodb.go
@@ -0,0 +1,119 @@
+package aws
+
+import (
+	"context"
+	"fmt"
+
+	awssdk "github.com/aws/aws-sdk-go-v2/aws"
+	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
+	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
+	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// DynamoAsset is a single STAC asset entry stored on a DynamoDB scene item.
+type DynamoAsset struct {
+	Href       string `dynamodbav:"href"`
+	Resolution int    `dynamodbav:"resolution"`
+}
+
+// DynamoProduction mirrors a production item stored in DynamoDB.
+type DynamoProduction struct {
+	ProduccionID    int64  `dynamodbav:"produccion_id"`
+	Activa          bool   `dynamodbav:"activa"`
+	Cultivo         string `dynamodbav:"cultivo"`
+	Ciclo           string `dynamodbav:"ciclo"`
+	FechaPlantacion string `dynamodbav:"fecha_plantacion"`
+	DiasProduccion  int    `dynamodbav:"dias_produccion"`
+}
+
+// DynamoScene mirrors a scene item stored in DynamoDB.
+type DynamoScene struct {
+	SceneID      string                 `dynamodbav:"scene_id"`
+	ProduccionID int64                  `dynamodbav:"produccion_id"`
+	Date         string                 `dynamodbav:"date"`
+	CloudCover   float64                `dynamodbav:"cloud_cover"`
+	STACAssets   map[string]DynamoAsset `dynamodbav:"stac_assets"`
+}
+
+// DynamoDBClient wraps the AWS SDK v2 DynamoDB client with the scan
+// operations needed by the worker.
+type DynamoDBClient struct {
+	client *dynamodb.Client
+}
+
+// NewDynamoDBClient builds a DynamoDBClient from an already-resolved AWS SDK config.
+func NewDynamoDBClient(awsCfg awssdk.Config) *DynamoDBClient {
+	return &DynamoDBClient{client: dynamodb.NewFromConfig(awsCfg)}
+}
+
+// ListActiveProducciones scans tableName and returns every item whose
+// "activa" attribute is true.
+func (c *DynamoDBClient) ListActiveProducciones(ctx context.Context, tableName string) ([]DynamoProduction, error) {
+	filt := expression.Name("activa").Equal(expression.Value(true))
+	expr, err := expression.NewBuilder().WithFilter(filt).Build()
+	if err != nil {
+		return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "building filter expression", Wrapped: err}
+	}
+
+	var results []DynamoProduction
+
+	paginator := dynamodb.NewScanPaginator(c.client, &dynamodb.ScanInput{
+		TableName:                 awssdk.String(tableName),
+		FilterExpression:          expr.Filter(),
+		ExpressionAttributeNames:  expr.Names(),
+		ExpressionAttributeValues: expr.Values(),
+	})
+
+	for paginator.HasMorePages() {
+		page, err := paginator.NextPage(ctx)
+		if err != nil {
+			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: fmt.Sprintf("scanning table %s", tableName), Wrapped: err}
+		}
+
+		var pageItems []DynamoProduction
+		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageItems); err != nil {
+			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "unmarshaling producciones", Wrapped: err}
+		}
+
+		results = append(results, pageItems...)
+	}
+
+	return results, nil
+}
+
+// ListEscenas scans tableName and returns every item whose "produccion_id"
+// attribute equals produccionID.
+func (c *DynamoDBClient) ListEscenas(ctx context.Context, tableName string, produccionID int64) ([]DynamoScene, error) {
+	filt := expression.Name("produccion_id").Equal(expression.Value(produccionID))
+	expr, err := expression.NewBuilder().WithFilter(filt).Build()
+	if err != nil {
+		return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "building filter expression", Wrapped: err}
+	}
+
+	var results []DynamoScene
+
+	paginator := dynamodb.NewScanPaginator(c.client, &dynamodb.ScanInput{
+		TableName:                 awssdk.String(tableName),
+		FilterExpression:          expr.Filter(),
+		ExpressionAttributeNames:  expr.Names(),
+		ExpressionAttributeValues: expr.Values(),
+	})
+
+	for paginator.HasMorePages() {
+		page, err := paginator.NextPage(ctx)
+		if err != nil {
+			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: fmt.Sprintf("scanning table %s", tableName), Wrapped: err}
+		}
+
+		var pageItems []DynamoScene
+		if err := attributevalue.UnmarshalListOfMaps(page.Items, &pageItems); err != nil {
+			return nil, &domain.ProcessingError{Type: domain.ErrDynamoDB, Message: "unmarshaling escenas", Wrapped: err}
+		}
+
+		results = append(results, pageItems...)
+	}
+
+	return results, nil
+}
diff --git a/internal/infrastructure/aws/dynamodb_test.go b/internal/infrastructure/aws/dynamodb_test.go
new file mode 100644
index 0000000..e86ecf1
--- /dev/null
+++ b/internal/infrastructure/aws/dynamodb_test.go
@@ -0,0 +1,147 @@
+package aws
+
+import (
+	"context"
+	"testing"
+
+	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
+	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
+	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
+)
+
+func createTable(t *testing.T, client *dynamodb.Client, tableName, hashKey string) {
+	t.Helper()
+	ctx := context.Background()
+
+	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
+		TableName: &tableName,
+		AttributeDefinitions: []types.AttributeDefinition{
+			{AttributeName: &hashKey, AttributeType: types.ScalarAttributeTypeS},
+		},
+		KeySchema: []types.KeySchemaElement{
+			{AttributeName: &hashKey, KeyType: types.KeyTypeHash},
+		},
+		BillingMode: types.BillingModePayPerRequest,
+	})
+	if err != nil {
+		// Table may already exist from a previous run.
+		t.Logf("CreateTable(%s): %v (may already exist)", tableName, err)
+	}
+}
+
+func TestDynamoDBClient_ListActiveProducciones(t *testing.T) {
+	cfg := requireLocalstack(t)
+
+	awsCfg, err := NewSession(cfg)
+	if err != nil {
+		t.Fatalf("NewSession: %v", err)
+	}
+
+	rawClient := dynamodb.NewFromConfig(awsCfg)
+	tableName := "test_monitoring_producciones"
+	createTable(t, rawClient, tableName, "produccion_id")
+
+	ctx := context.Background()
+
+	items := []DynamoProduction{
+		{ProduccionID: 1001, Activa: true, Cultivo: "maiz", Ciclo: "2026-A", FechaPlantacion: "2026-01-01", DiasProduccion: 90},
+		{ProduccionID: 1002, Activa: false, Cultivo: "soja", Ciclo: "2026-A", FechaPlantacion: "2026-01-15", DiasProduccion: 120},
+	}
+
+	for _, item := range items {
+		av, err := attributevalue.MarshalMap(item)
+		if err != nil {
+			t.Fatalf("MarshalMap: %v", err)
+		}
+		if _, err := rawClient.PutItem(ctx, &dynamodb.PutItemInput{TableName: &tableName, Item: av}); err != nil {
+			t.Fatalf("PutItem: %v", err)
+		}
+	}
+
+	client := &DynamoDBClient{client: rawClient}
+
+	got, err := client.ListActiveProducciones(ctx, tableName)
+	if err != nil {
+		t.Fatalf("ListActiveProducciones: %v", err)
+	}
+
+	found := false
+	for _, p := range got {
+		if p.ProduccionID == 1001 {
+			found = true
+			if !p.Activa {
+				t.Errorf("production 1001: Activa = false, want true")
+			}
+		}
+		if p.ProduccionID == 1002 {
+			t.Errorf("production 1002 is inactive and should not be returned")
+		}
+	}
+	if !found {
+		t.Errorf("expected production 1001 in results, got %+v", got)
+	}
+}
+
+func TestDynamoDBClient_ListEscenas(t *testing.T) {
+	cfg := requireLocalstack(t)
+
+	awsCfg, err := NewSession(cfg)
+	if err != nil {
+		t.Fatalf("NewSession: %v", err)
+	}
+
+	rawClient := dynamodb.NewFromConfig(awsCfg)
+	tableName := "test_monitoring_escenas"
+	createTable(t, rawClient, tableName, "scene_id")
+
+	ctx := context.Background()
+
+	scenes := []DynamoScene{
+		{
+			SceneID:      "scene-a",
+			ProduccionID: 2001,
+			Date:         "2026-02-01",
+			CloudCover:   12.5,
+			STACAssets: map[string]DynamoAsset{
+				"B04": {Href: "https://example.com/b04.tif", Resolution: 10},
+			},
+		},
+		{
+			SceneID:      "scene-b",
+			ProduccionID: 2002,
+			Date:         "2026-02-02",
+			CloudCover:   5.0,
+		},
+	}
+
+	for _, scene := range scenes {
+		av, err := attributevalue.MarshalMap(scene)
+		if err != nil {
+			t.Fatalf("MarshalMap: %v", err)
+		}
+		if _, err := rawClient.PutItem(ctx, &dynamodb.PutItemInput{TableName: &tableName, Item: av}); err != nil {
+			t.Fatalf("PutItem: %v", err)
+		}
+	}
+
+	client := &DynamoDBClient{client: rawClient}
+
+	got, err := client.ListEscenas(ctx, tableName, 2001)
+	if err != nil {
+		t.Fatalf("ListEscenas: %v", err)
+	}
+
+	if len(got) != 1 {
+		t.Fatalf("ListEscenas: got %d scenes, want 1 (%+v)", len(got), got)
+	}
+	if got[0].SceneID != "scene-a" {
+		t.Errorf("SceneID = %q, want scene-a", got[0].SceneID)
+	}
+	asset, ok := got[0].STACAssets["B04"]
+	if !ok {
+		t.Fatalf("expected STAC asset B04, got %+v", got[0].STACAssets)
+	}
+	if asset.Resolution != 10 {
+		t.Errorf("asset.Resolution = %d, want 10", asset.Resolution)
+	}
+}
diff --git a/internal/infrastructure/aws/s3.go b/internal/infrastructure/aws/s3.go
new file mode 100644
index 0000000..a9b6fa8
--- /dev/null
+++ b/internal/infrastructure/aws/s3.go
@@ -0,0 +1,138 @@
+package aws
+
+import (
+	"context"
+	"errors"
+	"fmt"
+	"os"
+	"time"
+
+	awssdk "github.com/aws/aws-sdk-go-v2/aws"
+	"github.com/aws/aws-sdk-go-v2/service/s3"
+	"github.com/aws/aws-sdk-go-v2/service/s3/types"
+	smithyhttp "github.com/aws/smithy-go/transport/http"
+
+	"agro-sentinel-worker/internal/domain"
+)
+
+// isNotFound reports whether err represents a "not found" response from S3
+// (either a typed NotFound error, or an HTTP 404 response error, which is
+// what HeadObject returns instead of a typed error).
+func isNotFound(err error) bool {
+	var notFound *types.NotFound
+	if errors.As(err, &notFound) {
+		return true
+	}
+
+	var respErr *smithyhttp.ResponseError
+	if errors.As(err, &respErr) {
+		return respErr.HTTPStatusCode() == 404
+	}
+
+	return false
+}
+
+// S3Client wraps the AWS SDK v2 S3 client with the operations needed by the
+// worker: upload/download of local files, existence checks and presigned
+// GET URLs.
+type S3Client struct {
+	client  *s3.Client
+	presign *s3.PresignClient
+}
+
+// NewS3Client builds an S3Client from an already-resolved AWS SDK config.
+func NewS3Client(awsCfg awssdk.Config) *S3Client {
+	client := s3.NewFromConfig(awsCfg)
+	return &S3Client{
+		client:  client,
+		presign: s3.NewPresignClient(client),
+	}
+}
+
+// Upload puts the contents of filePath into bucket/key.
+func (c *S3Client) Upload(ctx context.Context, bucket, key, filePath string) error {
+	f, err := os.Open(filePath)
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("opening file %s", filePath), Wrapped: err}
+	}
+	defer f.Close()
+
+	_, err = c.client.PutObject(ctx, &s3.PutObjectInput{
+		Bucket: awssdk.String(bucket),
+		Key:    awssdk.String(key),
+		Body:   f,
+	})
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("uploading s3://%s/%s", bucket, key), Wrapped: err}
+	}
+
+	return nil
+}
+
+// Download fetches bucket/key and writes it to destPath.
+func (c *S3Client) Download(ctx context.Context, bucket, key, destPath string) error {
+	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
+		Bucket: awssdk.String(bucket),
+		Key:    awssdk.String(key),
+	})
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("downloading s3://%s/%s", bucket, key), Wrapped: err}
+	}
+	defer out.Body.Close()
+
+	f, err := os.Create(destPath)
+	if err != nil {
+		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("creating file %s", destPath), Wrapped: err}
+	}
+	defer f.Close()
+
+	if _, err := f.ReadFrom(out.Body); err != nil {
+		return &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("writing file %s", destPath), Wrapped: err}
+	}
+
+	return nil
+}
+
+// HeadObject reports whether bucket/key exists and, if so, its size.
+func (c *S3Client) HeadObject(ctx context.Context, bucket, key string) (bool, int64, error) {
+	out, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
+		Bucket: awssdk.String(bucket),
+		Key:    awssdk.String(key),
+	})
+	if err != nil {
+		if isNotFound(err) {
+			return false, 0, nil
+		}
+		return false, 0, &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("head s3://%s/%s", bucket, key), Wrapped: err}
+	}
+
+	size := int64(0)
+	if out.ContentLength != nil {
+		size = *out.ContentLength
+	}
+
+	return true, size, nil
+}
+
+// PresignGetObject returns a presigned GET URL for bucket/key valid for expiry.
+func (c *S3Client) PresignGetObject(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
+	req, err := c.presign.PresignGetObject(ctx, &s3.GetObjectInput{
+		Bucket: awssdk.String(bucket),
+		Key:    awssdk.String(key),
+	}, s3.WithPresignExpires(expiry))
+	if err != nil {
+		return "", &domain.ProcessingError{Type: domain.ErrS3, Message: fmt.Sprintf("presigning s3://%s/%s", bucket, key), Wrapped: err}
+	}
+
+	return req.URL, nil
+}
+
+// BuildKey builds the canonical S3 key for a scene file: prefix/produccionID/sceneID/fileName.
+func BuildKey(prefix string, produccionID int64, sceneID string, fileName string) string {
+	return fmt.Sprintf("%s/%d/%s/%s", prefix, produccionID, sceneID, fileName)
+}
+
+// BuildKey is also exposed as a method on S3Client for interface convenience.
+func (c *S3Client) BuildKey(prefix string, produccionID int64, sceneID string, fileName string) string {
+	return BuildKey(prefix, produccionID, sceneID, fileName)
+}
diff --git a/internal/infrastructure/aws/s3_test.go b/internal/infrastructure/aws/s3_test.go
new file mode 100644
index 0000000..8dc843e
--- /dev/null
+++ b/internal/infrastructure/aws/s3_test.go
@@ -0,0 +1,107 @@
+package aws
+
+import (
+	"context"
+	"os"
+	"path/filepath"
+	"testing"
+	"time"
+
+	"github.com/aws/aws-sdk-go-v2/service/s3"
+
+	"agro-sentinel-worker/internal/config"
+)
+
+func requireLocalstack(t *testing.T) config.AWSConfig {
+	t.Helper()
+	endpoint := os.Getenv("AWS_ENDPOINT_URL")
+	if endpoint == "" {
+		t.Skip("AWS_ENDPOINT_URL not set, skipping test that requires localstack")
+	}
+
+	region := os.Getenv("AWS_REGION")
+	if region == "" {
+		region = "us-east-1"
+	}
+
+	return config.AWSConfig{Region: region, Endpoint: endpoint}
+}
+
+func TestS3Client_UploadHeadDownloadPresign(t *testing.T) {
+	cfg := requireLocalstack(t)
+
+	awsCfg, err := NewSession(cfg)
+	if err != nil {
+		t.Fatalf("NewSession: %v", err)
+	}
+
+	bucket := "agro-sentinel-worker-test"
+	ctx := context.Background()
+
+	rawClient := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.UsePathStyle = true })
+	// Ignore the error: the bucket may already exist from a previous test run.
+	_, _ = rawClient.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: &bucket})
+
+	client := &S3Client{client: rawClient, presign: s3.NewPresignClient(rawClient)}
+
+	dir := t.TempDir()
+	srcPath := filepath.Join(dir, "upload.txt")
+	content := []byte("hello agro sentinel")
+	if err := os.WriteFile(srcPath, content, 0644); err != nil {
+		t.Fatalf("writing source file: %v", err)
+	}
+
+	key := client.BuildKey("sentinel/producciones", 123, "scene-1", "upload.txt")
+
+	if err := client.Upload(ctx, bucket, key, srcPath); err != nil {
+		t.Fatalf("Upload: %v", err)
+	}
+
+	exists, size, err := client.HeadObject(ctx, bucket, key)
+	if err != nil {
+		t.Fatalf("HeadObject: %v", err)
+	}
+	if !exists {
+		t.Fatalf("HeadObject: expected object to exist")
+	}
+	if size != int64(len(content)) {
+		t.Errorf("HeadObject size = %d, want %d", size, len(content))
+	}
+
+	existsMissing, _, err := client.HeadObject(ctx, bucket, "does/not/exist.txt")
+	if err != nil {
+		t.Fatalf("HeadObject (missing): %v", err)
+	}
+	if existsMissing {
+		t.Errorf("HeadObject: expected missing object to not exist")
+	}
+
+	destPath := filepath.Join(dir, "download.txt")
+	if err := client.Download(ctx, bucket, key, destPath); err != nil {
+		t.Fatalf("Download: %v", err)
+	}
+
+	got, err := os.ReadFile(destPath)
+	if err != nil {
+		t.Fatalf("reading downloaded file: %v", err)
+	}
+	if string(got) != string(content) {
+		t.Errorf("downloaded content = %q, want %q", got, content)
+	}
+
+	url, err := client.PresignGetObject(ctx, bucket, key, 5*time.Minute)
+	if err != nil {
+		t.Fatalf("PresignGetObject: %v", err)
+	}
+	if url == "" {
+		t.Errorf("PresignGetObject: expected non-empty URL")
+	}
+}
+
+func TestBuildKey(t *testing.T) {
+	got := BuildKey("sentinel/producciones", 42, "scene-abc", "band.tif")
+	want := "sentinel/producciones/42/scene-abc/band.tif"
+	if got != want {
+		t.Errorf("BuildKey() = %q, want %q", got, want)
+	}
+}
diff --git a/internal/infrastructure/aws/session.go b/internal/infrastructure/aws/session.go
new file mode 100644
index 0000000..c333070
--- /dev/null
+++ b/internal/infrastructure/aws/session.go
@@ -0,0 +1,31 @@
+package aws
+
+import (
+	"context"
+
+	awssdk "github.com/aws/aws-sdk-go-v2/aws"
+	awsconfig "github.com/aws/aws-sdk-go-v2/config"
+
+	"agro-sentinel-worker/internal/config"
+)
+
+// NewSession creates an AWS SDK v2 config from the application's AWS
+// configuration. When cfg.Endpoint is set (e.g. for localstack), all
+// service clients created from the returned config will resolve to that
+// endpoint instead of the real AWS endpoints.
+func NewSession(cfg config.AWSConfig) (awssdk.Config, error) {
+	opts := []func(*awsconfig.LoadOptions) error{
+		awsconfig.WithRegion(cfg.Region),
+	}
+
+	if cfg.Endpoint != "" {
+		opts = append(opts, awsconfig.WithBaseEndpoint(cfg.Endpoint))
+	}
+
+	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
+	if err != nil {
+		return awssdk.Config{}, err
+	}
+
+	return awsCfg, nil
+}
