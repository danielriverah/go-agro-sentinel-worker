# Agro Sentinel Worker — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go-based geospatial worker that syncs agricultural productions from DynamoDB to MySQL, processes Sentinel-2 L2A COG scenes via GDAL, generates vegetation indices/images/statistics with historical chains, and exposes a REST API documented with Scalar.

**Architecture:** Monolith Go with 3 entry points (`cmd/api`, `cmd/worker`, `cmd/sync`) sharing domain and infrastructure packages. GDAL runs as an external process — Go coordinates, GDAL processes raster. A separate IA service (phase 9) analyzes params.json for field recommendations.

**Tech Stack:** Go 1.24+, GDAL 3.x, AWS SDK for Go v2 (S3/DynamoDB/SQS), MySQL 8, go-sql-driver/mysql, gopkg.in/yaml.v3, log/slog (stdlib), net/http (stdlib), Scalar for API docs.

## Global Constraints

- Go 1.24+ with modules
- GDAL invoked as external process — never as a Go library binding
- All GDAL commands must have configurable timeout (default 300s)
- Credentials via environment variables or IAM roles — never in code/config
- Temporary files are per-job in `{temp_dir}/jobs/{job_id}/` and cleaned after completion
- Bands are never loaded fully into RAM — use COG windowed reads via /vsis3/
- Target resolution default: 10m. 20m bands resampled with bilinear interpolation
- SCL band used for cloud cover calculation only, not included in multiband.tif
- Historical chain capped at 20 entries; older data summarized monthly
- API port: 6000
- All structured logging must include job_id, production_id, scene_id where applicable
- Error types: GDAL_ERROR, S3_ERROR, TIMEOUT, VALIDATION_ERROR, STAC_ERROR, IA_ERROR
- Max retries: 3 (configurable)

---

### Task 1: Project scaffold, configuration, and logging

**Files:**
- Create: `go.mod`
- Create: `cmd/api/main.go`
- Create: `cmd/worker/main.go`
- Create: `cmd/sync/main.go`
- Create: `internal/config/config.go`
- Create: `internal/logger/logger.go`
- Create: `internal/http/router.go`
- Create: `internal/http/handlers.go`
- Create: `configs/config.example.yaml`
- Create: `.gitignore`
- Test: `internal/config/config_test.go`
- Test: `internal/logger/logger_test.go`
- Test: `internal/http/handlers_test.go`

**Interfaces:**
- Consumes: nothing (first task)
- Produces:
  - `config.Load(path string) (*Config, error)` — parses YAML + env overrides
  - `config.Config` struct with all fields from spec
  - `logger.New(cfg config.LoggingConfig) *slog.Logger`
  - `http.NewRouter(logger *slog.Logger) *http.ServeMux`
  - `http.HealthHandler(w, r)` returns `{"status":"ok"}`
  - `cmd/api/main.go` starts HTTP server on port 6000
  - `cmd/worker/main.go` prints "worker starting" and exits
  - `cmd/sync/main.go` prints "sync starting" and exits

- [ ] **Step 1: Initialize Go module**

```bash
go mod init agro-sentinel-worker
```

- [ ] **Step 2: Create .gitignore**

```gitignore
# Binaries
/bin/
*.exe
*.dll
*.so
*.dylib

# Test
*.test
*.out
coverage.html

# IDE
.idea/
.vscode/
*.swp

# Config with secrets
configs/config.yaml
!configs/config.example.yaml

# Temp
/tmp/
```

- [ ] **Step 3: Write config test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	yaml := `
app:
  name: agro-sentinel-worker
  environment: test

server:
  host: 0.0.0.0
  port: 6000

sync:
  interval_minutes: 15
  dias_margen_monitoreo: 30

processing:
  temp_dir: /tmp/agro-sentinel
  target_resolution: 10
  resampling_method: bilinear
  workers: 1
  downloads_concurrency: 2

sentinel:
  cloud_cover_scene_max: 70
  cloud_cover_production_max: 23

aws:
  region: us-west-2

s3:
  bucket: test-bucket
  prefix: sentinel/producciones

sqs:
  queue_url: ""

dynamodb:
  table_producciones: monitoring_producciones
  table_escenas: monitoring_escenas

mysql:
  host: localhost
  port: 3306
  database: agro

ia:
  enabled: true
  service_url: http://localhost:8081
  timeout_seconds: 60

gdal:
  timeout_seconds: 300

logging:
  level: info
  format: json
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.App.Name != "agro-sentinel-worker" {
		t.Errorf("app.name = %q, want agro-sentinel-worker", cfg.App.Name)
	}
	if cfg.Server.Port != 6000 {
		t.Errorf("server.port = %d, want 6000", cfg.Server.Port)
	}
	if cfg.Processing.TargetResolution != 10 {
		t.Errorf("processing.target_resolution = %d, want 10", cfg.Processing.TargetResolution)
	}
	if cfg.Sentinel.CloudCoverProductionMax != 23 {
		t.Errorf("sentinel.cloud_cover_production_max = %v, want 23", cfg.Sentinel.CloudCoverProductionMax)
	}
	if cfg.Sync.DiasMargenMonitoreo != 30 {
		t.Errorf("sync.dias_margen_monitoreo = %d, want 30", cfg.Sync.DiasMargenMonitoreo)
	}
}

func TestLoadConfigEnvOverride(t *testing.T) {
	yaml := `
app:
  name: agro-sentinel-worker
  environment: test

server:
  host: 0.0.0.0
  port: 6000

sync:
  interval_minutes: 15
  dias_margen_monitoreo: 30

processing:
  temp_dir: /tmp/agro-sentinel
  target_resolution: 10
  resampling_method: bilinear
  workers: 1
  downloads_concurrency: 2

sentinel:
  cloud_cover_scene_max: 70
  cloud_cover_production_max: 23

aws:
  region: us-west-2

s3:
  bucket: yaml-bucket
  prefix: sentinel/producciones

sqs:
  queue_url: ""

dynamodb:
  table_producciones: monitoring_producciones
  table_escenas: monitoring_escenas

mysql:
  host: localhost
  port: 3306
  database: agro

ia:
  enabled: true
  service_url: http://localhost:8081
  timeout_seconds: 60

gdal:
  timeout_seconds: 300

logging:
  level: info
  format: json
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("S3_BUCKET", "env-bucket")
	t.Setenv("MYSQL_HOST", "db.example.com")
	t.Setenv("MYSQL_PASSWORD", "secret")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.S3.Bucket != "env-bucket" {
		t.Errorf("s3.bucket = %q, want env-bucket (env override)", cfg.S3.Bucket)
	}
	if cfg.MySQL.Host != "db.example.com" {
		t.Errorf("mysql.host = %q, want db.example.com (env override)", cfg.MySQL.Host)
	}
	if cfg.MySQL.Password != "secret" {
		t.Errorf("mysql.password should come from env")
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

```bash
go test ./internal/config/ -v
```

Expected: FAIL — package doesn't exist yet.

- [ ] **Step 5: Implement config**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	Server     ServerConfig     `yaml:"server"`
	Sync       SyncConfig       `yaml:"sync"`
	Processing ProcessingConfig `yaml:"processing"`
	Sentinel   SentinelConfig   `yaml:"sentinel"`
	AWS        AWSConfig        `yaml:"aws"`
	S3         S3Config         `yaml:"s3"`
	SQS        SQSConfig        `yaml:"sqs"`
	DynamoDB   DynamoDBConfig   `yaml:"dynamodb"`
	MySQL      MySQLConfig      `yaml:"mysql"`
	IA         IAConfig         `yaml:"ia"`
	GDAL       GDALConfig       `yaml:"gdal"`
	Logging    LoggingConfig    `yaml:"logging"`
}

type AppConfig struct {
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type SyncConfig struct {
	IntervalMinutes     int `yaml:"interval_minutes"`
	DiasMargenMonitoreo int `yaml:"dias_margen_monitoreo"`
}

type ProcessingConfig struct {
	TempDir              string `yaml:"temp_dir"`
	TargetResolution     int    `yaml:"target_resolution"`
	ResamplingMethod     string `yaml:"resampling_method"`
	Workers              int    `yaml:"workers"`
	DownloadsConcurrency int    `yaml:"downloads_concurrency"`
}

type SentinelConfig struct {
	CloudCoverSceneMax      float64 `yaml:"cloud_cover_scene_max"`
	CloudCoverProductionMax float64 `yaml:"cloud_cover_production_max"`
}

type AWSConfig struct {
	Region string `yaml:"region"`
}

type S3Config struct {
	Bucket string `yaml:"bucket"`
	Prefix string `yaml:"prefix"`
}

type SQSConfig struct {
	QueueURL string `yaml:"queue_url"`
}

type DynamoDBConfig struct {
	TableProducciones string `yaml:"table_producciones"`
	TableEscenas      string `yaml:"table_escenas"`
}

type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

type IAConfig struct {
	Enabled        bool   `yaml:"enabled"`
	ServiceURL     string `yaml:"service_url"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
}

type GDALConfig struct {
	TimeoutSeconds int `yaml:"timeout_seconds"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(&cfg)

	return &cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("AWS_REGION"); v != "" {
		cfg.AWS.Region = v
	}
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
		cfg.MySQL.Database = v
	}
	if v := os.Getenv("MYSQL_USER"); v != "" {
		cfg.MySQL.User = v
	}
	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
		cfg.MySQL.Password = v
	}
}
```

- [ ] **Step 6: Add yaml dependency and run tests**

```bash
go get gopkg.in/yaml.v3
go test ./internal/config/ -v
```

Expected: PASS

- [ ] **Step 7: Write logger test**

Create `internal/logger/logger_test.go`:

```go
package logger

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"agro-sentinel-worker/internal/config"
)

func TestNewLogger(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(config.LoggingConfig{Level: "info", Format: "json"}, &buf)

	l.Info("test message", "job_id", "123")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("log output should contain message, got: %s", output)
	}
	if !strings.Contains(output, "123") {
		t.Errorf("log output should contain job_id value, got: %s", output)
	}
}

func TestNewLoggerTextFormat(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(config.LoggingConfig{Level: "debug", Format: "text"}, &buf)

	l.Debug("debug message")

	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("debug log should appear at debug level, got: %s", output)
	}
}
```

- [ ] **Step 8: Run logger test to verify it fails**

```bash
go test ./internal/logger/ -v
```

Expected: FAIL

- [ ] **Step 9: Implement logger**

Create `internal/logger/logger.go`:

```go
package logger

import (
	"io"
	"log/slog"
	"os"

	"agro-sentinel-worker/internal/config"
)

func New(cfg config.LoggingConfig) *slog.Logger {
	return NewWithWriter(cfg, os.Stdout)
}

func NewWithWriter(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
	level := parseLevel(cfg.Level)

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

- [ ] **Step 10: Run logger tests**

```bash
go test ./internal/logger/ -v
```

Expected: PASS

- [ ] **Step 11: Write health handler test**

Create `internal/http/handlers_test.go`:

```go
package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
}
```

- [ ] **Step 12: Run handler test to verify it fails**

```bash
go test ./internal/http/ -v
```

Expected: FAIL

- [ ] **Step 13: Implement router and handlers**

Create `internal/http/handlers.go`:

```go
package http

import (
	"encoding/json"
	"net/http"
)

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

Create `internal/http/router.go`:

```go
package http

import (
	"log/slog"
	"net/http"
)

func NewRouter(logger *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", HealthHandler)
	return mux
}
```

- [ ] **Step 14: Run handler tests**

```bash
go test ./internal/http/ -v
```

Expected: PASS

- [ ] **Step 15: Create entry points**

Create `cmd/api/main.go`:

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"agro-sentinel-worker/internal/config"
	apphttp "agro-sentinel-worker/internal/http"
	"agro-sentinel-worker/internal/logger"
)

func main() {
	cfgPath := "configs/config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	l := logger.New(cfg.Logging)
	router := apphttp.NewRouter(l)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	l.Info("API server starting", "addr", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		l.Error("server failed", "error", err)
		os.Exit(1)
	}
}
```

Create `cmd/worker/main.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/logger"
)

func main() {
	cfgPath := "configs/config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	l := logger.New(cfg.Logging)
	l.Info("worker starting", "name", cfg.App.Name)
	fmt.Println("worker: no jobs configured yet")
}
```

Create `cmd/sync/main.go`:

```go
package main

import (
	"fmt"
	"log"
	"os"

	"agro-sentinel-worker/internal/config"
	"agro-sentinel-worker/internal/logger"
)

func main() {
	cfgPath := "configs/config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	l := logger.New(cfg.Logging)
	l.Info("sync starting", "name", cfg.App.Name, "interval_minutes", cfg.Sync.IntervalMinutes)
	fmt.Println("sync: no DynamoDB configured yet")
}
```

- [ ] **Step 16: Create example config**

Copy the YAML from the spec into `configs/config.example.yaml` (the full YAML block from the Configuration section of the design spec).

- [ ] **Step 17: Run all tests**

```bash
go test ./... -v
```

Expected: ALL PASS

- [ ] **Step 18: Verify API starts**

```bash
cp configs/config.example.yaml configs/config.yaml
go run ./cmd/api &
curl http://localhost:6000/health
# Expected: {"status":"ok"}
kill %1
```

- [ ] **Step 19: Commit**

```bash
git init
git add .
git commit -m "feat: project scaffold with config, logging, and health check"
```

---

### Task 2: Domain entities

**Files:**
- Create: `internal/domain/production.go`
- Create: `internal/domain/scene.go`
- Create: `internal/domain/band.go`
- Create: `internal/domain/job.go`
- Create: `internal/domain/product.go`
- Create: `internal/domain/analysis.go`
- Create: `internal/domain/errors.go`
- Test: `internal/domain/production_test.go`
- Test: `internal/domain/scene_test.go`
- Test: `internal/domain/band_test.go`

**Interfaces:**
- Consumes: nothing (pure domain, no dependencies)
- Produces:
  - `domain.Production` struct with all fields from s3_monitoring_producciones
  - `domain.Scene` struct with all fields from s3_monitoring_escenas
  - `domain.SceneFile` struct with all fields from s3_monitoring_escena_archivos
  - `domain.BBox` struct `{MinX, MinY, MaxX, MaxY float64}` with `Validate() error`
  - `domain.Band` type with constants `BandB02` through `BandB12` and `BandSCL`
  - `domain.BandInfo` struct `{Name Band, Resolution int, Href string}`
  - `domain.AllSpectralBands() []Band` — returns B02,B03,B04,B05,B06,B07,B08,B8A,B11,B12
  - `domain.BandsAtResolution(resolution int) []Band`
  - `domain.JobStatus` type with constants `StatusPending`, `StatusProcessing`, `StatusCompleted`, `StatusFailed`
  - `domain.FileType` type with constants for each file type (multiband, natural, ndvi, etc.)
  - `domain.AnalysisResult` struct matching IA service response
  - `domain.ErrType` type with constants `ErrGDAL`, `ErrS3`, `ErrTimeout`, `ErrValidation`, `ErrSTAC`, `ErrIA`
  - `domain.ProcessingError` struct `{Type ErrType, Message string, Wrapped error}` implementing `error`

- [ ] **Step 1: Write BBox validation test**

Create `internal/domain/production_test.go`:

```go
package domain

import "testing"

func TestBBoxValidate(t *testing.T) {
	tests := []struct {
		name    string
		bbox    BBox
		wantErr bool
	}{
		{
			name:    "valid bbox",
			bbox:    BBox{MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
			wantErr: false,
		},
		{
			name:    "minx >= maxx",
			bbox:    BBox{MinX: -102.30, MinY: 21.80, MaxX: -102.35, MaxY: 21.85},
			wantErr: true,
		},
		{
			name:    "miny >= maxy",
			bbox:    BBox{MinX: -102.35, MinY: 21.85, MaxX: -102.30, MaxY: 21.80},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bbox.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProductionShouldMonitor(t *testing.T) {
	p := Production{
		Monitoring: true,
		Bloqueado:  false,
		BBox:       &BBox{MinX: -102.35, MinY: 21.80, MaxX: -102.30, MaxY: 21.85},
	}
	if !p.ShouldProcess() {
		t.Error("production with monitoring=true, not blocked, with bbox should be processable")
	}

	p.Bloqueado = true
	if p.ShouldProcess() {
		t.Error("blocked production should not be processable")
	}
}
```

- [ ] **Step 2: Write band tests**

Create `internal/domain/band_test.go`:

```go
package domain

import "testing"

func TestAllSpectralBands(t *testing.T) {
	bands := AllSpectralBands()
	if len(bands) != 10 {
		t.Errorf("AllSpectralBands() returned %d bands, want 10", len(bands))
	}
}

func TestBandsAtResolution(t *testing.T) {
	bands10m := BandsAtResolution(10)
	if len(bands10m) != 4 {
		t.Errorf("BandsAtResolution(10) = %d bands, want 4 (B02,B03,B04,B08)", len(bands10m))
	}

	bands20m := BandsAtResolution(20)
	if len(bands20m) != 6 {
		t.Errorf("BandsAtResolution(20) = %d bands, want 6 (B05,B06,B07,B8A,B11,B12)", len(bands20m))
	}
}

func TestBandResolution(t *testing.T) {
	if BandB04.Resolution() != 10 {
		t.Errorf("B04 resolution = %d, want 10", BandB04.Resolution())
	}
	if BandB05.Resolution() != 20 {
		t.Errorf("B05 resolution = %d, want 20", BandB05.Resolution())
	}
	if BandSCL.Resolution() != 20 {
		t.Errorf("SCL resolution = %d, want 20", BandSCL.Resolution())
	}
}
```

- [ ] **Step 3: Write scene test**

Create `internal/domain/scene_test.go`:

```go
package domain

import "testing"

func TestSceneNeedsProcessing(t *testing.T) {
	s := Scene{Status: StatusPending}
	if !s.NeedsProcessing() {
		t.Error("PENDING scene should need processing")
	}

	s.Status = StatusCompleted
	if s.NeedsProcessing() {
		t.Error("COMPLETED scene should not need processing")
	}

	s.Status = StatusFailed
	s.RetryCount = 2
	if !s.CanRetry(3) {
		t.Error("FAILED scene with retry_count=2 and max=3 should be retryable")
	}

	s.RetryCount = 3
	if s.CanRetry(3) {
		t.Error("FAILED scene with retry_count=3 and max=3 should not be retryable")
	}
}
```

- [ ] **Step 4: Run tests to verify they fail**

```bash
go test ./internal/domain/ -v
```

Expected: FAIL

- [ ] **Step 5: Implement domain entities**

Create `internal/domain/errors.go`:

```go
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
```

Create `internal/domain/band.go`:

```go
package domain

type Band string

const (
	BandB02 Band = "B02"
	BandB03 Band = "B03"
	BandB04 Band = "B04"
	BandB05 Band = "B05"
	BandB06 Band = "B06"
	BandB07 Band = "B07"
	BandB08 Band = "B08"
	BandB8A Band = "B8A"
	BandB11 Band = "B11"
	BandB12 Band = "B12"
	BandSCL Band = "SCL"
)

var bandResolutions = map[Band]int{
	BandB02: 10,
	BandB03: 10,
	BandB04: 10,
	BandB05: 20,
	BandB06: 20,
	BandB07: 20,
	BandB08: 10,
	BandB8A: 20,
	BandB11: 20,
	BandB12: 20,
	BandSCL: 20,
}

func (b Band) Resolution() int {
	return bandResolutions[b]
}

func AllSpectralBands() []Band {
	return []Band{BandB02, BandB03, BandB04, BandB05, BandB06, BandB07, BandB08, BandB8A, BandB11, BandB12}
}

func BandsAtResolution(resolution int) []Band {
	var result []Band
	for _, b := range AllSpectralBands() {
		if b.Resolution() == resolution {
			result = append(result, b)
		}
	}
	return result
}

type BandInfo struct {
	Name       Band
	Resolution int
	Href       string
}
```

Create `internal/domain/production.go`:

```go
package domain

import (
	"errors"
	"time"
)

type BBox struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

func (b BBox) Validate() error {
	if b.MinX >= b.MaxX {
		return errors.New("bbox: MinX must be less than MaxX")
	}
	if b.MinY >= b.MaxY {
		return errors.New("bbox: MinY must be less than MaxY")
	}
	return nil
}

type Production struct {
	ID                 int64
	ProduccionID       int64
	Cultivo            string
	Ciclo              string
	BBox               *BBox
	Monitoring         bool
	MonitoringMotivo   string
	Bloqueado          bool
	BloqueadoMotivo    string
	BloqueadoAt        *time.Time
	DesbloqueadoPor    string
	TargetResolution   int
	CloudCoverMax      float64
	FechaPlantacion    *time.Time
	DiasProduccion     int
	FechaFinMonitoreo  *time.Time
	TotalEscenas       int
	TotalEscenasValidas int
	LastSyncAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (p *Production) ShouldProcess() bool {
	return p.Monitoring && !p.Bloqueado && p.BBox != nil
}
```

Create `internal/domain/scene.go`:

```go
package domain

import "time"

type JobStatus string

const (
	StatusPending    JobStatus = "PENDING"
	StatusProcessing JobStatus = "PROCESSING"
	StatusCompleted  JobStatus = "COMPLETED"
	StatusFailed     JobStatus = "FAILED"
)

type Scene struct {
	ID              int64
	ProduccionID    int64
	SceneID         string
	SceneDate       time.Time
	CloudCoverScene float64
	CloudCoverBBox  *float64
	PassesQuality   bool
	HasMultiband    bool
	HasParams       bool
	HasRGB          bool
	HasAnalisis     bool
	Status          JobStatus
	ErrorType       string
	ErrorMessage    string
	RetryCount      int
	ProcessedAt     *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (s *Scene) NeedsProcessing() bool {
	return s.Status == StatusPending
}

func (s *Scene) CanRetry(maxRetries int) bool {
	return s.Status == StatusFailed && s.RetryCount < maxRetries
}

type SceneFile struct {
	ID            int64
	EscenaID      int64
	FileType      FileType
	FileName      string
	S3Key         string
	S3Bucket      string
	FileSizeBytes int64
	ResolutionM   int
	WidthPx       int
	HeightPx      int
	CreatedAt     time.Time
}
```

Create `internal/domain/product.go`:

```go
package domain

type FileType string

const (
	FileMultiband  FileType = "multiband"
	FileNatural    FileType = "natural"
	FileFalseColor FileType = "false_color"
	FileNDVI       FileType = "ndvi"
	FileNDRE       FileType = "ndre"
	FileEVI        FileType = "evi"
	FileGNDVI      FileType = "gndvi"
	FileNBR        FileType = "nbr"
	FileNDMI       FileType = "ndmi"
	FileSAVI       FileType = "savi"
	FileRedEdge    FileType = "red_edge"
	FileSWIR       FileType = "swir"
	FileParams     FileType = "params"
	FileAnalisis   FileType = "analisis"
)

func AllImageTypes() []FileType {
	return []FileType{
		FileNatural, FileFalseColor, FileNDVI, FileNDRE,
		FileEVI, FileGNDVI, FileNBR, FileNDMI,
		FileSAVI, FileRedEdge, FileSWIR,
	}
}
```

Create `internal/domain/analysis.go`:

```go
package domain

type AnalysisResult struct {
	EstadoGeneral    string   `json:"estado_general"`
	VigorVegetativo  string   `json:"vigor_vegetativo"`
	EstresDetectado  bool     `json:"estres_detectado"`
	PosibleCosecha   bool     `json:"posible_cosecha"`
	Recomendaciones  []string `json:"recomendaciones"`
	Alertas          []string `json:"alertas"`
	Confianza        float64  `json:"confianza"`
}
```

- [ ] **Step 6: Run all domain tests**

```bash
go test ./internal/domain/ -v
```

Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add .
git commit -m "feat: domain entities — production, scene, band, errors"
```

---

### Task 3: MySQL migrations and repositories

**Files:**
- Create: `internal/infrastructure/database/mysql.go`
- Create: `internal/infrastructure/database/migrations.go`
- Create: `internal/infrastructure/database/production_repo.go`
- Create: `internal/infrastructure/database/scene_repo.go`
- Create: `internal/infrastructure/database/file_repo.go`
- Test: `internal/infrastructure/database/production_repo_test.go`
- Test: `internal/infrastructure/database/scene_repo_test.go`
- Test: `internal/infrastructure/database/file_repo_test.go`

**Interfaces:**
- Consumes: `domain.Production`, `domain.Scene`, `domain.SceneFile`, `domain.BBox`, `config.MySQLConfig`
- Produces:
  - `database.NewConnection(cfg config.MySQLConfig) (*sql.DB, error)`
  - `database.RunMigrations(db *sql.DB) error` — creates the 3 tables if they don't exist
  - `database.ProductionRepo` struct with methods:
    - `Upsert(ctx, production *domain.Production) error`
    - `GetByProduccionID(ctx, produccionID int64) (*domain.Production, error)`
    - `ListActive(ctx) ([]*domain.Production, error)`
    - `UpdateMonitoring(ctx, produccionID int64, monitoring bool, motivo string) error`
    - `UpdateBBox(ctx, produccionID int64, bbox domain.BBox) error`
    - `SetBloqueado(ctx, produccionID int64, motivo string) error`
    - `Desbloquear(ctx, produccionID int64, usuario string) error`
    - `IncrementEscenas(ctx, produccionID int64, valid bool) error`
  - `database.SceneRepo` struct with methods:
    - `Upsert(ctx, scene *domain.Scene) error`
    - `GetByID(ctx, id int64) (*domain.Scene, error)`
    - `GetByProduccionAndSceneID(ctx, produccionID int64, sceneID string) (*domain.Scene, error)`
    - `ListByProduccion(ctx, produccionID int64) ([]*domain.Scene, error)`
    - `UpdateStatus(ctx, id int64, status domain.JobStatus) error`
    - `SetError(ctx, id int64, errType string, errMsg string) error`
    - `SetCompleted(ctx, id int64, passesQuality bool, cloudCoverBBox float64) error`
    - `GetPreviousValidScene(ctx, produccionID int64, beforeDate time.Time) (*domain.Scene, error)`
  - `database.FileRepo` struct with methods:
    - `Create(ctx, file *domain.SceneFile) error`
    - `ListByEscena(ctx, escenaID int64) ([]*domain.SceneFile, error)`
    - `GetByType(ctx, escenaID int64, fileType domain.FileType) (*domain.SceneFile, error)`

- [ ] **Step 1: Write migration SQL constants and test**

Create `internal/infrastructure/database/migrations.go` with the CREATE TABLE statements for all 3 tables matching the spec's column definitions exactly. Include `IF NOT EXISTS` for idempotency.

Create `internal/infrastructure/database/mysql.go` with `NewConnection` that opens a MySQL connection using `go-sql-driver/mysql`.

Write tests in `internal/infrastructure/database/production_repo_test.go` etc. that use `sqltest` or a test MySQL (skip if `MYSQL_TEST_DSN` env is not set). For each repo, test CRUD operations: insert, get, update, list.

The key test patterns:
- Insert a production, get it back, verify all fields
- Upsert same produccion_id, verify no duplicate (UNIQUE constraint)
- Insert scene, update status, verify change
- SetBloqueado then Desbloquear, verify bloqueado=0
- GetPreviousValidScene returns the most recent scene before a given date with passes_quality=1

- [ ] **Step 2: Implement repos**

Each repo takes `*sql.DB` in its constructor. Use prepared statements. All methods take `context.Context` as first parameter.

- [ ] **Step 3: Add go-sql-driver dependency**

```bash
go get github.com/go-sql-driver/mysql
```

- [ ] **Step 4: Run tests (skip if no test DB available)**

```bash
MYSQL_TEST_DSN="user:pass@tcp(localhost:3306)/agro_test" go test ./internal/infrastructure/database/ -v
```

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: MySQL migrations and repositories for producciones, escenas, archivos"
```

---

### Task 4: GDAL executor

**Files:**
- Create: `internal/infrastructure/gdal/executor.go`
- Create: `internal/infrastructure/gdal/translate.go`
- Create: `internal/infrastructure/gdal/warp.go`
- Create: `internal/infrastructure/gdal/info.go`
- Create: `internal/infrastructure/gdal/vrt.go`
- Test: `internal/infrastructure/gdal/executor_test.go`
- Test: `internal/infrastructure/gdal/info_test.go`
- Test: `testdata/tiny.tif` (small GeoTIFF for testing)

**Interfaces:**
- Consumes: `config.GDALConfig`, `domain.BBox`, `domain.Band`
- Produces:
  - `gdal.Executor` struct with:
    - `Run(ctx context.Context, command string, args []string) (stdout string, stderr string, err error)` — runs a GDAL command with timeout, validates exit code
  - `gdal.Info(ctx, executor *Executor, inputPath string) (*GDALInfo, error)` — runs gdalinfo -json, parses output into struct with size, bands, projection, bounds
  - `gdal.Translate(ctx, executor *Executor, opts TranslateOpts) error` — runs gdal_translate with projwin for BBOX crop
  - `gdal.Warp(ctx, executor *Executor, opts WarpOpts) error` — runs gdalwarp with target resolution, resampling method, target SRS
  - `gdal.BuildVRT(ctx, executor *Executor, opts VRTOpts) error` — runs gdalbuildvrt to compose multiple bands into VRT
  - `TranslateOpts{Input, Output string, BBox *domain.BBox, OutputFormat string}`
  - `WarpOpts{Input, Output string, TargetResolution int, ResamplingMethod string, TargetSRS string, BBox *domain.BBox}`
  - `VRTOpts{Inputs []string, Output string, Separate bool}`
  - `GDALInfo{Width, Height int, Bands int, Projection string, BoundsMinX, BoundsMinY, BoundsMaxX, BoundsMaxY float64}`

- [ ] **Step 1: Create a tiny test GeoTIFF**

Use GDAL to create a minimal test file (can be done in test setup):

```bash
gdal_create -of GTiff -outsize 10 10 -bands 1 -burn 128 testdata/tiny.tif
```

Or generate one programmatically in the test setup using gdal_translate from a VRT.

- [ ] **Step 2: Write executor test**

Test that `executor.Run` correctly captures stdout, stderr, and returns error on non-zero exit. Test timeout behavior. Skip tests if `gdalinfo` is not found in PATH.

```go
func TestExecutorRun(t *testing.T) {
	if _, err := exec.LookPath("gdalinfo"); err != nil {
		t.Skip("gdalinfo not found in PATH")
	}
	e := NewExecutor(300)
	stdout, stderr, err := e.Run(context.Background(), "gdalinfo", []string{"--version"})
	if err != nil {
		t.Fatalf("gdalinfo --version failed: %v (stderr: %s)", err, stderr)
	}
	if !strings.Contains(stdout, "GDAL") {
		t.Errorf("expected GDAL in output, got: %s", stdout)
	}
}

func TestExecutorRunTimeout(t *testing.T) {
	e := NewExecutor(1) // 1 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, _, err := e.Run(ctx, "sleep", []string{"10"})
	if err == nil {
		t.Error("expected timeout error")
	}
}
```

- [ ] **Step 3: Implement executor, info, translate, warp, vrt**

The executor runs commands via `exec.CommandContext`, captures stdout/stderr, checks exit code, and wraps errors in `domain.ProcessingError{Type: domain.ErrGDAL}`.

- [ ] **Step 4: Write info parsing test with tiny.tif**

Test that `Info` returns correct width, height, band count from the test file.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/infrastructure/gdal/ -v
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: GDAL executor with translate, warp, vrt, info commands"
```

---

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

### Task 7: Processing core — multiband.tif generation

**Files:**
- Create: `internal/processing/bands.go`
- Create: `internal/processing/multiband.go`
- Create: `internal/storage/filesystem.go`
- Test: `internal/processing/multiband_test.go`
- Test: `internal/storage/filesystem_test.go`

**Interfaces:**
- Consumes:
  - `gdal.Executor.Run(ctx, command, args) (stdout, stderr, err)`
  - `gdal.Warp(ctx, executor, WarpOpts) error`
  - `gdal.BuildVRT(ctx, executor, VRTOpts) error`
  - `gdal.Translate(ctx, executor, TranslateOpts) error`
  - `domain.BBox`, `domain.Band`, `domain.BandInfo`, `domain.AllSpectralBands()`
  - `aws.S3Client.Upload`, `aws.S3Client.HeadObject`
  - `config.ProcessingConfig`
- Produces:
  - `processing.MultibandBuilder` struct with:
    - `New(executor GDALExecutor, s3 S3Uploader, cfg config.ProcessingConfig, logger *slog.Logger) *MultibandBuilder`
    - `Build(ctx, jobDir string, bbox domain.BBox, bands []domain.BandInfo, targetResolution int) (outputPath string, err error)`
  - `processing.GDALExecutor` interface wrapping gdal executor methods
  - `processing.S3Uploader` interface `{Upload(ctx, bucket, key, filePath) error; HeadObject(ctx, bucket, key) (bool, int64, error)}`
  - `storage.JobDir` struct with:
    - `New(baseDir string, jobID string) *JobDir`
    - `Input() string` — returns `{baseDir}/jobs/{jobID}/input/`
    - `Work() string` — returns `{baseDir}/jobs/{jobID}/work/`
    - `Output() string` — returns `{baseDir}/jobs/{jobID}/output/`
    - `Create() error` — creates all subdirectories
    - `Cleanup() error` — removes the entire job directory

- [ ] **Step 1: Write filesystem test**

```go
func TestJobDir(t *testing.T) {
	base := t.TempDir()
	jd := New(base, "test-job-123")

	if err := jd.Create(); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	for _, dir := range []string{jd.Input(), jd.Work(), jd.Output()} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("directory %s should exist", dir)
		}
	}

	if err := jd.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if _, err := os.Stat(jd.Input()); !os.IsNotExist(err) {
		t.Error("job directory should be removed after cleanup")
	}
}
```

- [ ] **Step 2: Implement filesystem.go**

- [ ] **Step 3: Write multiband build test**

Test with mock GDALExecutor that records command calls. Verify:
- Warp is called once per band with correct BBOX and target resolution
- 20m bands get resampling_method=bilinear
- BuildVRT is called with all warped bands, `separate=true`
- Translate is called to convert VRT to GeoTIFF
- Output path is in the job output directory

- [ ] **Step 4: Implement multiband.go**

The flow:
1. For each band: `gdalwarp` with `-te bbox -tr resolution -r bilinear` on the COG href → cropped/resampled band in work dir
2. `gdalbuildvrt -separate` all warped bands → composite.vrt
3. `gdal_translate` composite.vrt → multiband.tif in output dir

- [ ] **Step 5: Run tests**

```bash
go test ./internal/processing/ -v
go test ./internal/storage/ -v
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: multiband.tif generation from COG bands via GDAL"
```

---

### Task 8: SCL cloud cover calculation

**Files:**
- Create: `internal/processing/cloudcover.go`
- Test: `internal/processing/cloudcover_test.go`

**Interfaces:**
- Consumes:
  - `gdal.Executor`, `gdal.Warp`, `gdal.Info`
  - `domain.BBox`, `domain.BandSCL`
- Produces:
  - `processing.CalculateCloudCover(ctx, executor GDALExecutor, sclHref string, bbox domain.BBox, workDir string) (cloudCoverPct float64, coverage CoverageStats, err error)`
  - `processing.CoverageStats` struct `{VegetationPct, SoilPct, WaterPct, CloudPct float64}`

The function:
1. Warp SCL band to BBOX at 20m (native resolution, no resampling — nearest neighbor for categorical data)
2. Run `gdal_translate -of AAIGrid` to get ASCII grid (or use gdalinfo with -stats and histogram)
3. Parse pixel value counts per SCL class
4. Calculate cloud_cover_bbox per spec formula: `(pixels_3 + pixels_8 + pixels_9 + pixels_10) / total_valid * 100`
5. Calculate coverage stats from SCL classes for params.json

- [ ] **Step 1: Write test with known pixel distributions**

Use a mock executor that returns a pre-defined histogram from gdalinfo.

- [ ] **Step 2: Implement cloudcover.go**

- [ ] **Step 3: Run tests and commit**

```bash
go test ./internal/processing/ -v -run CloudCover
git add .
git commit -m "feat: SCL-based cloud cover calculation and coverage stats"
```

---

### Task 9: RGB and band combination images

**Files:**
- Create: `internal/processing/rgb.go`
- Create: `internal/processing/indices.go`
- Test: `internal/processing/rgb_test.go`
- Test: `internal/processing/indices_test.go`

**Interfaces:**
- Consumes:
  - `gdal.Executor`, `gdal.Translate`, `gdal.BuildVRT`
  - `domain.Band`, `domain.FileType`
- Produces:
  - `processing.GenerateRGB(ctx, executor GDALExecutor, multibandPath string, outputPath string, redBand, greenBand, blueBand int) error`
    - Uses `gdal_translate -b R -b G -b B -of PNG -scale -ot Byte` to create 8-bit RGB PNG
  - `processing.GenerateIndex(ctx, executor GDALExecutor, multibandPath string, outputPath string, indexType domain.FileType) error`
    - Uses `gdal_calc.py` or VRT pixel functions to compute index, then `gdal_translate -of PNG` with color ramp
  - `processing.IndexDefinition` struct `{Type domain.FileType, Formula string, Bands []domain.Band, Name string}`
  - `processing.AllIndices() []IndexDefinition` — returns the 7 indices from the spec
  - `processing.AllCompositions() []CompositionDefinition` — returns natural, false_color, red_edge, swir
  - `processing.CompositionDefinition` struct `{Type domain.FileType, RedBand, GreenBand, BlueBand domain.Band, Name string}`

The band order in multiband.tif is: B02(1), B03(2), B04(3), B05(4), B06(5), B07(6), B08(7), B8A(8), B11(9), B12(10).

Compositions use band numbers from multiband.tif:
- natural.png: bands 3,2,1 (B04,B03,B02)
- false_color.png: bands 7,3,2 (B08,B04,B03)
- red_edge.png: bands 5,4,3 (B06,B05,B04)
- swir.png: bands 10,8,3 (B12,B8A,B04)

Index images use a color ramp (green-yellow-red for vegetation indices, blue-white-brown for moisture).

- [ ] **Step 1: Write composition and index definition tests**

Verify `AllIndices()` returns 7 entries, `AllCompositions()` returns 4 entries, band numbers map correctly.

- [ ] **Step 2: Write GenerateRGB test with mock executor**

Verify the gdal_translate command is built with correct `-b` flags and `-of PNG`.

- [ ] **Step 3: Implement rgb.go and indices.go**

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/processing/ -v -run "RGB|Index"
git add .
git commit -m "feat: RGB compositions and vegetation index image generation"
```

---

### Task 10: Statistics and params.json generation

**Files:**
- Create: `internal/processing/statistics.go`
- Create: `internal/processing/params.go`
- Test: `internal/processing/statistics_test.go`
- Test: `internal/processing/params_test.go`

**Interfaces:**
- Consumes:
  - `gdal.Executor`, `gdal.Info`
  - `domain.Band`, `domain.AllSpectralBands()`
  - `processing.CoverageStats`
  - `aws.S3Client.Download` (to fetch previous params.json)
  - `database.SceneRepo.GetPreviousValidScene`
- Produces:
  - `processing.CalculateBandStatistics(ctx, executor GDALExecutor, multibandPath string) (map[domain.Band]BandStats, error)`
  - `processing.BandStats` struct `{Mean, Std, Min, Max, P25, P50, P75 float64}`
  - `processing.CalculateIndexStatistics(ctx, executor GDALExecutor, multibandPath string, indices []IndexDefinition) (map[domain.FileType]IndexStats, error)`
  - `processing.IndexStats` struct `{Mean, Std, Min, Max, P25, P50, P75 float64}`
  - `processing.BuildParams(current ParamsInput, previousParams *Params) *Params`
  - `processing.Params` struct matching the params.json schema from the spec
  - `processing.ParamsInput` struct `{ProduccionID int64, SceneID string, SceneDate, FechaPlantacion time.Time, CloudCoverBBox float64, Indices map[FileType]IndexStats, BandStats map[Band]BandStats, Coverage CoverageStats}`
  - `processing.TrimHistorico(historico []HistoricoEntry, maxEntries int) []HistoricoEntry` — keeps last N entries

- [ ] **Step 1: Write BuildParams test**

```go
func TestBuildParams(t *testing.T) {
	current := ParamsInput{
		ProduccionID:   1234,
		SceneID:        "S2A_scene2",
		SceneDate:      time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		FechaPlantacion: time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		CloudCoverBBox: 12.5,
		Indices: map[FileType]IndexStats{
			FileNDVI: {Mean: 0.72, Std: 0.08},
		},
		Coverage: CoverageStats{VegetationPct: 78.5, SoilPct: 15.2},
	}

	previous := &Params{
		ProduccionID:      1234,
		SceneID:           "S2A_scene1",
		SceneDate:         "2026-08-22",
		DiasDesdePlantacion: 38,
		Indices: map[string]IndexStats{
			"ndvi": {Mean: 0.60},
		},
		Historico: nil, // first scene had no history
	}

	params := BuildParams(current, previous)

	if params.DiasDesdePlantacion != 48 {
		t.Errorf("dias = %d, want 48", params.DiasDesdePlantacion)
	}
	if len(params.Historico) != 1 {
		t.Fatalf("historico length = %d, want 1", len(params.Historico))
	}
	if params.Historico[0].SceneID != "S2A_scene1" {
		t.Error("historico[0] should be previous scene")
	}
	if params.Historico[0].Delta["ndvi_mean_change"] != 0.12 {
		t.Errorf("ndvi delta = %v, want 0.12", params.Historico[0].Delta["ndvi_mean_change"])
	}
}

func TestTrimHistorico(t *testing.T) {
	entries := make([]HistoricoEntry, 25)
	for i := range entries {
		entries[i] = HistoricoEntry{SceneID: fmt.Sprintf("scene_%d", i)}
	}

	trimmed := TrimHistorico(entries, 20)
	if len(trimmed) != 20 {
		t.Errorf("trimmed length = %d, want 20", len(trimmed))
	}
}
```

- [ ] **Step 2: Implement statistics.go and params.go**

Statistics use `gdalinfo -stats -json` on each band of multiband.tif to extract min, max, mean, std. Percentiles can be calculated via `gdal_translate` with histogram analysis.

params.go builds the JSON structure, calculates `dias_desde_plantacion`, builds the historical chain with deltas.

- [ ] **Step 3: Run tests and commit**

```bash
go test ./internal/processing/ -v -run "Params|Statistics|Trim"
git add .
git commit -m "feat: statistics calculation and params.json with historical chain"
```

---

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

### Task 12: REST API with Scalar

**Files:**
- Modify: `internal/http/router.go` — add all API routes
- Modify: `internal/http/handlers.go` — implement all handlers
- Create: `internal/http/middleware.go` — logging middleware
- Create: `internal/http/responses.go` — JSON response helpers
- Modify: `cmd/api/main.go` — wire dependencies
- Test: `internal/http/handlers_test.go` — expand with all endpoint tests

**Interfaces:**
- Consumes: `database.ProductionRepo`, `database.SceneRepo`, `database.FileRepo`, `aws.S3Client.PresignGetObject`
- Produces:
  - All REST endpoints from the spec
  - Scalar documentation at `/docs`
  - `http.LoggingMiddleware(logger, next) http.Handler`
  - `http.JSON(w, status int, data any)` helper
  - `http.Error(w, status int, message string)` helper

- [ ] **Step 1: Write handler tests for each endpoint**

Test each endpoint with mock repos:
- `GET /api/v1/producciones` → returns list
- `GET /api/v1/producciones/{id}` → returns production with escenas
- `POST /api/v1/producciones/{id}/desbloquear` → calls Desbloquear, returns updated production
- `GET /api/v1/escenas/{id}/archivos/{tipo}` → returns presigned URL
- `POST /api/v1/sync/trigger` → triggers sync, returns 202

- [ ] **Step 2: Implement all handlers**

Use `http.ServeMux` patterns with Go 1.22+ method routing:
```go
mux.HandleFunc("GET /api/v1/producciones", h.ListProducciones)
mux.HandleFunc("GET /api/v1/producciones/{id}", h.GetProduccion)
```

- [ ] **Step 3: Add Scalar docs**

Serve the Scalar UI at `/docs` using their CDN-hosted JS. The OpenAPI spec can be embedded as a Go constant or served from a YAML file.

```go
mux.HandleFunc("GET /docs", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    fmt.Fprint(w, scalarHTML)
})
mux.HandleFunc("GET /api/v1/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/yaml")
    w.Write(openapiSpec)
})
```

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/http/ -v
git add .
git commit -m "feat: REST API with all endpoints and Scalar documentation"
```

---

### Task 13: IA service client and integration

**Files:**
- Create: `internal/infrastructure/ia/client.go`
- Test: `internal/infrastructure/ia/client_test.go`

**Interfaces:**
- Consumes: `config.IAConfig`, `domain.AnalysisResult`, params.json content
- Produces:
  - `ia.Client` struct with:
    - `New(cfg config.IAConfig) *Client`
    - `Analyze(ctx context.Context, params json.RawMessage) (*domain.AnalysisResult, error)` — POST to IA service, parse response
  - Wraps HTTP errors in `domain.ProcessingError{Type: domain.ErrIA}`
  - Returns `nil, nil` when `cfg.Enabled == false` (IA disabled)

- [ ] **Step 1: Write client test with httptest server**

```go
func TestAnalyze(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "POST" || r.URL.Path != "/analyze" {
            t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
        }
        json.NewEncoder(w).Encode(domain.AnalysisResult{
            EstadoGeneral:   "bueno",
            PosibleCosecha:  false,
            Confianza:       0.85,
        })
    }))
    defer server.Close()

    client := New(config.IAConfig{Enabled: true, ServiceURL: server.URL, TimeoutSeconds: 10})
    result, err := client.Analyze(context.Background(), json.RawMessage(`{"test":true}`))
    if err != nil {
        t.Fatalf("Analyze failed: %v", err)
    }
    if result.EstadoGeneral != "bueno" {
        t.Errorf("estado = %q, want bueno", result.EstadoGeneral)
    }
}

func TestAnalyzeDisabled(t *testing.T) {
    client := New(config.IAConfig{Enabled: false})
    result, err := client.Analyze(context.Background(), nil)
    if err != nil || result != nil {
        t.Error("disabled IA should return nil, nil")
    }
}
```

- [ ] **Step 2: Implement client.go**

- [ ] **Step 3: Update worker.go to use IA client**

After generating params.json, call `ia.Analyze`. If `posible_cosecha == true`, call `productionRepo.SetBloqueado`. If IA fails, log warning but mark scene COMPLETED anyway (spec: IA_ERROR is partial retry).

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/infrastructure/ia/ -v
git add .
git commit -m "feat: IA service client with harvest detection and blocking"
```

---

### Task 14: SQS integration and job queue

**Files:**
- Create: `internal/infrastructure/aws/sqs.go`
- Create: `internal/jobs/processor.go`
- Create: `internal/jobs/queue.go`
- Modify: `cmd/worker/main.go` — switch to SQS-driven loop
- Test: `internal/infrastructure/aws/sqs_test.go`
- Test: `internal/jobs/processor_test.go`

**Interfaces:**
- Consumes: `config.SQSConfig`, `worker.Worker.ProcessScene`
- Produces:
  - `aws.SQSClient` struct with:
    - `ReceiveMessages(ctx, queueURL string, maxMessages int, waitSeconds int) ([]SQSMessage, error)`
    - `DeleteMessage(ctx, queueURL string, receiptHandle string) error`
    - `SendMessage(ctx, queueURL string, body string) error`
  - `SQSMessage` struct `{Body string, ReceiptHandle string}`
  - `jobs.JobMessage` struct `{JobID string, ProduccionID int64, SceneID string}`
  - `jobs.Processor` struct with:
    - `New(sqsClient SQSQueue, worker SceneProcessor, sceneRepo SceneRepository, cfg ProcessorConfig, logger *slog.Logger) *Processor`
    - `Run(ctx context.Context) error` — long-poll SQS loop, process each message, delete on success
  - Idempotency: before processing, check if scene status is already COMPLETED → skip and delete message

- [ ] **Step 1: Write processor test**

Test with mock SQS and mock worker:
- Message received → ProcessScene called → message deleted
- Already COMPLETED → ProcessScene NOT called → message deleted
- ProcessScene fails → message NOT deleted (SQS will redeliver)
- Retry logic: FAILED scene with retry_count < max → reprocess

- [ ] **Step 2: Implement SQS client and processor**

- [ ] **Step 3: Update cmd/worker/main.go for SQS loop**

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/jobs/ -v
git add .
git commit -m "feat: SQS job queue with idempotent processing and retries"
```

---

### Task 15: Docker and docker-compose

**Files:**
- Create: `Dockerfile`
- Create: `docker-compose.yml`
- Create: `.dockerignore`
- Create: `scripts/check-environment.sh`

**Interfaces:**
- Consumes: all 3 binaries from `cmd/`
- Produces: Docker images that run api, worker, sync with GDAL installed

- [ ] **Step 1: Create multi-stage Dockerfile**

```dockerfile
# Build stage
FROM golang:1.24-bookworm AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/api ./cmd/api
RUN CGO_ENABLED=0 go build -o /bin/worker ./cmd/worker
RUN CGO_ENABLED=0 go build -o /bin/sync ./cmd/sync

# Runtime stage
FROM ghcr.io/osgeo/gdal:ubuntu-small-3.9.3
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /bin/api /bin/worker /bin/sync /usr/local/bin/
COPY configs/config.example.yaml /etc/agro-sentinel/config.yaml
WORKDIR /app
```

- [ ] **Step 2: Create docker-compose.yml**

```yaml
services:
  api:
    build: .
    command: api
    ports:
      - "6000:6000"
    environment:
      CONFIG_PATH: /etc/agro-sentinel/config.yaml
      MYSQL_HOST: mysql
      MYSQL_PASSWORD: ${MYSQL_PASSWORD:-devpass}
      AWS_ENDPOINT_URL: http://localstack:4566
      AWS_REGION: us-east-1
      AWS_ACCESS_KEY_ID: test
      AWS_SECRET_ACCESS_KEY: test
    depends_on:
      mysql:
        condition: service_healthy

  worker:
    build: .
    command: worker
    environment:
      CONFIG_PATH: /etc/agro-sentinel/config.yaml
      MYSQL_HOST: mysql
      MYSQL_PASSWORD: ${MYSQL_PASSWORD:-devpass}
      AWS_ENDPOINT_URL: http://localstack:4566
      AWS_REGION: us-east-1
      AWS_ACCESS_KEY_ID: test
      AWS_SECRET_ACCESS_KEY: test
    depends_on:
      mysql:
        condition: service_healthy

  sync:
    build: .
    command: sync
    environment:
      CONFIG_PATH: /etc/agro-sentinel/config.yaml
      MYSQL_HOST: mysql
      MYSQL_PASSWORD: ${MYSQL_PASSWORD:-devpass}
      AWS_ENDPOINT_URL: http://localstack:4566
      AWS_REGION: us-east-1
      AWS_ACCESS_KEY_ID: test
      AWS_SECRET_ACCESS_KEY: test
    depends_on:
      mysql:
        condition: service_healthy

  mysql:
    image: mysql:8
    environment:
      MYSQL_ROOT_PASSWORD: ${MYSQL_PASSWORD:-devpass}
      MYSQL_DATABASE: agro
    ports:
      - "3306:3306"
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 5s
      retries: 10

  localstack:
    image: localstack/localstack:latest
    ports:
      - "4566:4566"
    environment:
      SERVICES: s3,sqs,dynamodb
```

- [ ] **Step 3: Create .dockerignore**

```
.git
bin/
tmp/
*.exe
docs/
```

- [ ] **Step 4: Create check-environment.sh**

Script that verifies GDAL, Go, and other dependencies are available.

- [ ] **Step 5: Build and test locally**

```bash
docker compose build
docker compose up -d mysql localstack
docker compose up api
# Test: curl http://localhost:6000/health
```

- [ ] **Step 6: Commit**

```bash
git add .
git commit -m "feat: Docker multi-stage build and docker-compose for local development"
```

---

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

### Task 17: Integration test — end-to-end processing

**Files:**
- Create: `tests/integration/processing_test.go`
- Create: `tests/testdata/` — small test GeoTIFFs

**Interfaces:**
- Consumes: everything built so far
- Produces: integration test that verifies the full pipeline works

- [ ] **Step 1: Create test GeoTIFFs**

Use GDAL to create small (10x10 pixel) test GeoTIFFs for B02, B03, B04, B08, SCL with known pixel values. These simulate a tiny Sentinel-2 scene.

- [ ] **Step 2: Write integration test**

Skip if GDAL not available. Test:
1. Create multiband.tif from test bands
2. Calculate cloud cover from test SCL
3. Generate natural.png
4. Generate NDVI image
5. Calculate statistics
6. Build params.json with mock previous scene
7. Verify all output files exist and have reasonable sizes

- [ ] **Step 3: Run and commit**

```bash
go test ./tests/integration/ -v -tags integration
git add .
git commit -m "test: end-to-end integration test for processing pipeline"
```
