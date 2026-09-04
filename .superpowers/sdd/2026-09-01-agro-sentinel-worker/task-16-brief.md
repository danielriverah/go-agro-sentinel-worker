### Task 16: Health check dependencies endpoint

**Files:**
- Modify: `internal/http/handlers.go` — add HealthDependenciesHandler
- Modify: `internal/http/router.go` — register route
- Test: `internal/http/handlers_test.go` — test dependencies endpoint

**Interfaces:**
- Consumes: `*sql.DB` (ping), `gdal.Executor` (gdalinfo --version), `aws.S3Client` (HeadBucket), `aws.DynamoDBClient` (DescribeTable)
- Produces:
  - `GET /health/dependencies` returning:
  ```json
  {
    "gdal": {"status": "ok", "version": "3.9.3"},
    "mysql": {"status": "ok"},
    "s3": {"status": "ok"},
    "dynamodb": {"status": "ok"}
  }
  ```

- [ ] **Step 1: Write test, implement, run, commit**

```bash
go test ./internal/http/ -v -run HealthDependencies
git add .
git commit -m "feat: health dependencies endpoint checking GDAL, MySQL, S3, DynamoDB"
```

---

