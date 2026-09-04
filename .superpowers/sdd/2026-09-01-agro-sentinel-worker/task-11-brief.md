### Task 11: Processing worker orchestration

**Files:**
- Create: `internal/worker/worker.go`
- Modify: `cmd/worker/main.go` — wire up with real dependencies
- Test: `internal/worker/worker_test.go`

**Interfaces:**
- Consumes: all processing functions, all repos, S3 client, GDAL executor
- Produces:
  - `worker.Worker` struct with:
    - `New(deps WorkerDeps) *Worker`
    - `ProcessScene(ctx context.Context, produccionID int64, sceneID string) error` — the full processing flow from spec step 1-13
  - `worker.WorkerDeps` struct containing all dependencies (repos, s3, gdal executor, config, logger, ia client)

- [ ] **Step 1: Write ProcessScene test with all mocks**

Test the full orchestration:
1. Scene exists, production has monitoring=1 and bbox
2. HeadObject returns false for multiband.tif → triggers build
3. MultibandBuilder.Build succeeds
4. Cloud cover calculation returns 12% → below threshold
5. All images generated
6. Params built with historical chain
7. All files uploaded to S3
8. Scene status updated to COMPLETED
9. Files registered in database

Also test:
- Cloud cover > 23% → only natural.png generated
- IA service error → scene still COMPLETED, has_analisis=0
- GDAL error → scene FAILED with error_type=GDAL_ERROR

- [ ] **Step 2: Implement worker.go**

The `ProcessScene` method follows the Processing Worker Flow from the spec exactly. Each step updates the scene status in the database. Errors are caught, classified by type, and stored in the scene record.

Temp files use `storage.JobDir` and are cleaned up in a `defer`.

- [ ] **Step 3: Wire cmd/worker/main.go**

For now, the worker main just processes a single scene from command-line args (before SQS integration in Task 14).

```go
// Usage: go run ./cmd/worker -production 1234 -scene S2A_xxx
```

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/worker/ -v
git add .
git commit -m "feat: processing worker orchestration with full scene pipeline"
```

---

