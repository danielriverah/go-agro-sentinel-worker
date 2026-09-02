### Task 6: Sync service

**Files:**
- Create: `internal/sync/sync.go`
- Create: `internal/sync/bbox.go`
- Modify: `cmd/sync/main.go` — wire up sync service with real dependencies
- Test: `internal/sync/sync_test.go`
- Test: `internal/sync/bbox_test.go`

**Interfaces:**
- Consumes:
  - `aws.DynamoDBClient.ListActiveProducciones(ctx, tableName) ([]DynamoProduction, error)`
  - `aws.DynamoDBClient.ListEscenas(ctx, tableName, produccionID) ([]DynamoScene, error)`
  - `database.ProductionRepo` — all methods
  - `database.SceneRepo.Upsert`, `SceneRepo.GetByProduccionAndSceneID`
  - `config.SyncConfig`, `config.SentinelConfig`
- Produces:
  - `sync.Service` struct with:
    - `New(dynamo DynamoReader, prodRepo ProductionRepository, sceneRepo SceneRepository, polygonRepo PolygonRepository, cfg SyncConfig, sentinel SentinelConfig, logger *slog.Logger) *Service`
    - `RunOnce(ctx context.Context) error` — executes one sync cycle
    - `RunLoop(ctx context.Context) error` — runs sync every N minutes until ctx is cancelled
  - `sync.DynamoReader` interface `{ListActiveProducciones(ctx, table) ([]DynamoProduction, error); ListEscenas(ctx, table, produccionID) ([]DynamoScene, error)}`
  - `sync.ProductionRepository` interface (subset of database.ProductionRepo methods)
  - `sync.SceneRepository` interface (subset of database.SceneRepo methods)
  - `sync.PolygonRepository` interface `{GetPolygonBBox(ctx, produccionID int64) (*domain.BBox, error)}`
  - `sync.CalculateBBoxFromWKT(wkt string) (*domain.BBox, error)` — parses WKT polygon, extracts envelope
  - `sync.CalculateFinMonitoreo(plantacion time.Time, diasProduccion, diasMargen int) time.Time`

- [ ] **Step 1: Write CalculateFinMonitoreo test**

```go
func TestCalculateFinMonitoreo(t *testing.T) {
	plantacion := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	fin := CalculateFinMonitoreo(plantacion, 150, 30)
	expected := time.Date(2027, 1, 11, 0, 0, 0, 0, time.UTC)
	if !fin.Equal(expected) {
		t.Errorf("fin = %v, want %v", fin, expected)
	}
}
```

- [ ] **Step 2: Write CalculateBBoxFromWKT test**

```go
func TestCalculateBBoxFromWKT(t *testing.T) {
	wkt := "POLYGON((-102.35 21.80, -102.30 21.80, -102.30 21.85, -102.35 21.85, -102.35 21.80))"
	bbox, err := CalculateBBoxFromWKT(wkt)
	if err != nil {
		t.Fatalf("CalculateBBoxFromWKT failed: %v", err)
	}
	if bbox.MinX != -102.35 || bbox.MaxX != -102.30 || bbox.MinY != 21.80 || bbox.MaxY != 21.85 {
		t.Errorf("bbox = %+v, unexpected values", bbox)
	}
}
```

- [ ] **Step 3: Write RunOnce test with mock interfaces**

Test the full sync logic with mock DynamoReader, mock repos. Verify:
- New production from Dynamo gets inserted into MySQL
- Production without fecha_plantacion gets monitoring=0
- Production past fin de monitoreo gets monitoring=0
- Production without polygon gets monitoring=0
- Production with polygon gets BBOX calculated
- Scene from Dynamo gets inserted with PENDING status
- Blocked production is skipped

- [ ] **Step 4: Implement sync service**

Use the interface-based design so tests work with mocks. The real implementation in `cmd/sync/main.go` wires the concrete implementations.

- [ ] **Step 5: Implement bbox.go**

Parse WKT polygon — extract coordinate pairs, find min/max X/Y. The WKT format from MySQL spatial columns is standard: `POLYGON((x1 y1, x2 y2, ...))`.

- [ ] **Step 6: Wire cmd/sync/main.go**

Connect to MySQL, create repos, create DynamoDB client, create sync.Service, call RunLoop.

- [ ] **Step 7: Run tests**

```bash
go test ./internal/sync/ -v
```

- [ ] **Step 8: Commit**

```bash
git add .
git commit -m "feat: sync service — DynamoDB to MySQL with BBOX and cycle control"
```

---

