### Task 5: AWS clients (S3 + DynamoDB)

**Files:**
- Create: `internal/infrastructure/aws/session.go`
- Create: `internal/infrastructure/aws/s3.go`
- Create: `internal/infrastructure/aws/dynamodb.go`
- Test: `internal/infrastructure/aws/s3_test.go`
- Test: `internal/infrastructure/aws/dynamodb_test.go`

**Interfaces:**
- Consumes: `config.AWSConfig`, `config.S3Config`, `config.DynamoDBConfig`, `domain.Production`, `domain.Scene`
- Produces:
  - `aws.NewSession(cfg config.AWSConfig) (aws.Config, error)` — creates AWS config
  - `aws.S3Client` struct with:
    - `Upload(ctx, bucket, key string, filePath string) error`
    - `Download(ctx, bucket, key string, destPath string) error`
    - `HeadObject(ctx, bucket, key string) (exists bool, size int64, err error)`
    - `PresignGetObject(ctx, bucket, key string, expiry time.Duration) (url string, err error)`
    - `BuildKey(prefix string, produccionID int64, sceneID string, fileName string) string`
  - `aws.DynamoDBClient` struct with:
    - `ListActiveProducciones(ctx, tableName string) ([]DynamoProduction, error)`
    - `ListEscenas(ctx, tableName string, produccionID int64) ([]DynamoScene, error)`
  - `DynamoProduction` struct `{ProduccionID int64, Activa bool, Cultivo, Ciclo string, FechaPlantacion string, DiasProduccion int}`
  - `DynamoScene` struct `{SceneID string, ProduccionID int64, Date string, CloudCover float64, STACAssets map[string]DynamoAsset}`
  - `DynamoAsset` struct `{Href string, Resolution int}`

- [ ] **Step 1: Write S3 test with localstack skip**

Tests should skip if `AWS_ENDPOINT_URL` env is not set (no localstack running). Test upload, head, download, presign.

- [ ] **Step 2: Write DynamoDB test with localstack skip**

Test ListActiveProducciones returns items inserted via test setup.

- [ ] **Step 3: Implement AWS clients**

Use AWS SDK v2. `s3.NewFromConfig(awsCfg)`, `dynamodb.NewFromConfig(awsCfg)`. Support custom endpoint for localstack via env.

- [ ] **Step 4: Add AWS SDK dependency**

```bash
go get github.com/aws/aws-sdk-go-v2
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/service/s3
go get github.com/aws/aws-sdk-go-v2/service/dynamodb
go get github.com/aws/aws-sdk-go-v2/feature/s3/manager
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/infrastructure/aws/ -v
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: AWS clients for S3 and DynamoDB"
```

---

