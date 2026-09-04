# Review Package — Task 1

## Commits
4095290 feat: project scaffold with config, logging, and health check

## Stat
 .gitignore                                         |  23 +
 .../2026-09-01-agro-sentinel-worker/progress.md    |   1 +
 .../task-1-brief.md                                | 730 +++++++++++++++++++++
 cmd/api/main.go                                    |  35 +
 cmd/sync/main.go                                   |  26 +
 cmd/worker/main.go                                 |  26 +
 configs/config.example.yaml                        |  55 ++
 go.mod                                             |   5 +
 go.sum                                             |   3 +
 internal/config/config.go                          | 133 ++++
 internal/config/config_test.go                     | 173 +++++
 internal/http/handlers.go                          |  11 +
 internal/http/handlers_test.go                     |  29 +
 internal/http/router.go                            |  12 +
 internal/logger/logger.go                          |  41 ++
 internal/logger/logger_test.go                     |  36 +
 16 files changed, 1339 insertions(+)

## Diff
diff --git a/.gitignore b/.gitignore
new file mode 100644
index 0000000..b70dbcb
--- /dev/null
+++ b/.gitignore
@@ -0,0 +1,23 @@
+# Binaries
+/bin/
+*.exe
+*.dll
+*.so
+*.dylib
+
+# Test
+*.test
+*.out
+coverage.html
+
+# IDE
+.idea/
+.vscode/
+*.swp
+
+# Config with secrets
+configs/config.yaml
+!configs/config.example.yaml
+
+# Temp
+/tmp/
diff --git a/.superpowers/sdd/2026-09-01-agro-sentinel-worker/progress.md b/.superpowers/sdd/2026-09-01-agro-sentinel-worker/progress.md
new file mode 100644
index 0000000..49bf668
--- /dev/null
+++ b/.superpowers/sdd/2026-09-01-agro-sentinel-worker/progress.md
@@ -0,0 +1 @@
+# SDD ledger — plan: docs/superpowers/plans/2026-09-01-agro-sentinel-worker.md
diff --git a/.superpowers/sdd/2026-09-01-agro-sentinel-worker/task-1-brief.md b/.superpowers/sdd/2026-09-01-agro-sentinel-worker/task-1-brief.md
new file mode 100644
index 0000000..fee1e59
--- /dev/null
+++ b/.superpowers/sdd/2026-09-01-agro-sentinel-worker/task-1-brief.md
@@ -0,0 +1,730 @@
+### Task 1: Project scaffold, configuration, and logging
+
+**Files:**
+- Create: `go.mod`
+- Create: `cmd/api/main.go`
+- Create: `cmd/worker/main.go`
+- Create: `cmd/sync/main.go`
+- Create: `internal/config/config.go`
+- Create: `internal/logger/logger.go`
+- Create: `internal/http/router.go`
+- Create: `internal/http/handlers.go`
+- Create: `configs/config.example.yaml`
+- Create: `.gitignore`
+- Test: `internal/config/config_test.go`
+- Test: `internal/logger/logger_test.go`
+- Test: `internal/http/handlers_test.go`
+
+**Interfaces:**
+- Consumes: nothing (first task)
+- Produces:
+  - `config.Load(path string) (*Config, error)` — parses YAML + env overrides
+  - `config.Config` struct with all fields from spec
+  - `logger.New(cfg config.LoggingConfig) *slog.Logger`
+  - `http.NewRouter(logger *slog.Logger) *http.ServeMux`
+  - `http.HealthHandler(w, r)` returns `{"status":"ok"}`
+  - `cmd/api/main.go` starts HTTP server on port 6000
+  - `cmd/worker/main.go` prints "worker starting" and exits
+  - `cmd/sync/main.go` prints "sync starting" and exits
+
+- [ ] **Step 1: Initialize Go module**
+
+```bash
+go mod init agro-sentinel-worker
+```
+
+- [ ] **Step 2: Create .gitignore**
+
+```gitignore
+# Binaries
+/bin/
+*.exe
+*.dll
+*.so
+*.dylib
+
+# Test
+*.test
+*.out
+coverage.html
+
+# IDE
+.idea/
+.vscode/
+*.swp
+
+# Config with secrets
+configs/config.yaml
+!configs/config.example.yaml
+
+# Temp
+/tmp/
+```
+
+- [ ] **Step 3: Write config test**
+
+Create `internal/config/config_test.go`:
+
+```go
+package config
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestLoadConfig(t *testing.T) {
+	yaml := `
+app:
+  name: agro-sentinel-worker
+  environment: test
+
+server:
+  host: 0.0.0.0
+  port: 6000
+
+sync:
+  interval_minutes: 15
+  dias_margen_monitoreo: 30
+
+processing:
+  temp_dir: /tmp/agro-sentinel
+  target_resolution: 10
+  resampling_method: bilinear
+  workers: 1
+  downloads_concurrency: 2
+
+sentinel:
+  cloud_cover_scene_max: 70
+  cloud_cover_production_max: 23
+
+aws:
+  region: us-west-2
+
+s3:
+  bucket: test-bucket
+  prefix: sentinel/producciones
+
+sqs:
+  queue_url: ""
+
+dynamodb:
+  table_producciones: monitoring_producciones
+  table_escenas: monitoring_escenas
+
+mysql:
+  host: localhost
+  port: 3306
+  database: agro
+
+ia:
+  enabled: true
+  service_url: http://localhost:8081
+  timeout_seconds: 60
+
+gdal:
+  timeout_seconds: 300
+
+logging:
+  level: info
+  format: json
+`
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.yaml")
+	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
+		t.Fatal(err)
+	}
+
+	cfg, err := Load(path)
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+
+	if cfg.App.Name != "agro-sentinel-worker" {
+		t.Errorf("app.name = %q, want agro-sentinel-worker", cfg.App.Name)
+	}
+	if cfg.Server.Port != 6000 {
+		t.Errorf("server.port = %d, want 6000", cfg.Server.Port)
+	}
+	if cfg.Processing.TargetResolution != 10 {
+		t.Errorf("processing.target_resolution = %d, want 10", cfg.Processing.TargetResolution)
+	}
+	if cfg.Sentinel.CloudCoverProductionMax != 23 {
+		t.Errorf("sentinel.cloud_cover_production_max = %v, want 23", cfg.Sentinel.CloudCoverProductionMax)
+	}
+	if cfg.Sync.DiasMargenMonitoreo != 30 {
+		t.Errorf("sync.dias_margen_monitoreo = %d, want 30", cfg.Sync.DiasMargenMonitoreo)
+	}
+}
+
+func TestLoadConfigEnvOverride(t *testing.T) {
+	yaml := `
+app:
+  name: agro-sentinel-worker
+  environment: test
+
+server:
+  host: 0.0.0.0
+  port: 6000
+
+sync:
+  interval_minutes: 15
+  dias_margen_monitoreo: 30
+
+processing:
+  temp_dir: /tmp/agro-sentinel
+  target_resolution: 10
+  resampling_method: bilinear
+  workers: 1
+  downloads_concurrency: 2
+
+sentinel:
+  cloud_cover_scene_max: 70
+  cloud_cover_production_max: 23
+
+aws:
+  region: us-west-2
+
+s3:
+  bucket: yaml-bucket
+  prefix: sentinel/producciones
+
+sqs:
+  queue_url: ""
+
+dynamodb:
+  table_producciones: monitoring_producciones
+  table_escenas: monitoring_escenas
+
+mysql:
+  host: localhost
+  port: 3306
+  database: agro
+
+ia:
+  enabled: true
+  service_url: http://localhost:8081
+  timeout_seconds: 60
+
+gdal:
+  timeout_seconds: 300
+
+logging:
+  level: info
+  format: json
+`
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.yaml")
+	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
+		t.Fatal(err)
+	}
+
+	t.Setenv("S3_BUCKET", "env-bucket")
+	t.Setenv("MYSQL_HOST", "db.example.com")
+	t.Setenv("MYSQL_PASSWORD", "secret")
+
+	cfg, err := Load(path)
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+
+	if cfg.S3.Bucket != "env-bucket" {
+		t.Errorf("s3.bucket = %q, want env-bucket (env override)", cfg.S3.Bucket)
+	}
+	if cfg.MySQL.Host != "db.example.com" {
+		t.Errorf("mysql.host = %q, want db.example.com (env override)", cfg.MySQL.Host)
+	}
+	if cfg.MySQL.Password != "secret" {
+		t.Errorf("mysql.password should come from env")
+	}
+}
+```
+
+- [ ] **Step 4: Run test to verify it fails**
+
+```bash
+go test ./internal/config/ -v
+```
+
+Expected: FAIL — package doesn't exist yet.
+
+- [ ] **Step 5: Implement config**
+
+Create `internal/config/config.go`:
+
+```go
+package config
+
+import (
+	"fmt"
+	"os"
+
+	"gopkg.in/yaml.v3"
+)
+
+type Config struct {
+	App        AppConfig        `yaml:"app"`
+	Server     ServerConfig     `yaml:"server"`
+	Sync       SyncConfig       `yaml:"sync"`
+	Processing ProcessingConfig `yaml:"processing"`
+	Sentinel   SentinelConfig   `yaml:"sentinel"`
+	AWS        AWSConfig        `yaml:"aws"`
+	S3         S3Config         `yaml:"s3"`
+	SQS        SQSConfig        `yaml:"sqs"`
+	DynamoDB   DynamoDBConfig   `yaml:"dynamodb"`
+	MySQL      MySQLConfig      `yaml:"mysql"`
+	IA         IAConfig         `yaml:"ia"`
+	GDAL       GDALConfig       `yaml:"gdal"`
+	Logging    LoggingConfig    `yaml:"logging"`
+}
+
+type AppConfig struct {
+	Name        string `yaml:"name"`
+	Environment string `yaml:"environment"`
+}
+
+type ServerConfig struct {
+	Host string `yaml:"host"`
+	Port int    `yaml:"port"`
+}
+
+type SyncConfig struct {
+	IntervalMinutes     int `yaml:"interval_minutes"`
+	DiasMargenMonitoreo int `yaml:"dias_margen_monitoreo"`
+}
+
+type ProcessingConfig struct {
+	TempDir              string `yaml:"temp_dir"`
+	TargetResolution     int    `yaml:"target_resolution"`
+	ResamplingMethod     string `yaml:"resampling_method"`
+	Workers              int    `yaml:"workers"`
+	DownloadsConcurrency int    `yaml:"downloads_concurrency"`
+}
+
+type SentinelConfig struct {
+	CloudCoverSceneMax      float64 `yaml:"cloud_cover_scene_max"`
+	CloudCoverProductionMax float64 `yaml:"cloud_cover_production_max"`
+}
+
+type AWSConfig struct {
+	Region string `yaml:"region"`
+}
+
+type S3Config struct {
+	Bucket string `yaml:"bucket"`
+	Prefix string `yaml:"prefix"`
+}
+
+type SQSConfig struct {
+	QueueURL string `yaml:"queue_url"`
+}
+
+type DynamoDBConfig struct {
+	TableProducciones string `yaml:"table_producciones"`
+	TableEscenas      string `yaml:"table_escenas"`
+}
+
+type MySQLConfig struct {
+	Host     string `yaml:"host"`
+	Port     int    `yaml:"port"`
+	Database string `yaml:"database"`
+	User     string `yaml:"user"`
+	Password string `yaml:"password"`
+}
+
+type IAConfig struct {
+	Enabled        bool   `yaml:"enabled"`
+	ServiceURL     string `yaml:"service_url"`
+	TimeoutSeconds int    `yaml:"timeout_seconds"`
+}
+
+type GDALConfig struct {
+	TimeoutSeconds int `yaml:"timeout_seconds"`
+}
+
+type LoggingConfig struct {
+	Level  string `yaml:"level"`
+	Format string `yaml:"format"`
+}
+
+func Load(path string) (*Config, error) {
+	data, err := os.ReadFile(path)
+	if err != nil {
+		return nil, fmt.Errorf("reading config file: %w", err)
+	}
+
+	var cfg Config
+	if err := yaml.Unmarshal(data, &cfg); err != nil {
+		return nil, fmt.Errorf("parsing config file: %w", err)
+	}
+
+	applyEnvOverrides(&cfg)
+
+	return &cfg, nil
+}
+
+func applyEnvOverrides(cfg *Config) {
+	if v := os.Getenv("AWS_REGION"); v != "" {
+		cfg.AWS.Region = v
+	}
+	if v := os.Getenv("S3_BUCKET"); v != "" {
+		cfg.S3.Bucket = v
+	}
+	if v := os.Getenv("SQS_QUEUE_URL"); v != "" {
+		cfg.SQS.QueueURL = v
+	}
+	if v := os.Getenv("MYSQL_HOST"); v != "" {
+		cfg.MySQL.Host = v
+	}
+	if v := os.Getenv("MYSQL_DATABASE"); v != "" {
+		cfg.MySQL.Database = v
+	}
+	if v := os.Getenv("MYSQL_USER"); v != "" {
+		cfg.MySQL.User = v
+	}
+	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
+		cfg.MySQL.Password = v
+	}
+}
+```
+
+- [ ] **Step 6: Add yaml dependency and run tests**
+
+```bash
+go get gopkg.in/yaml.v3
+go test ./internal/config/ -v
+```
+
+Expected: PASS
+
+- [ ] **Step 7: Write logger test**
+
+Create `internal/logger/logger_test.go`:
+
+```go
+package logger
+
+import (
+	"bytes"
+	"log/slog"
+	"strings"
+	"testing"
+
+	"agro-sentinel-worker/internal/config"
+)
+
+func TestNewLogger(t *testing.T) {
+	var buf bytes.Buffer
+	l := NewWithWriter(config.LoggingConfig{Level: "info", Format: "json"}, &buf)
+
+	l.Info("test message", "job_id", "123")
+
+	output := buf.String()
+	if !strings.Contains(output, "test message") {
+		t.Errorf("log output should contain message, got: %s", output)
+	}
+	if !strings.Contains(output, "123") {
+		t.Errorf("log output should contain job_id value, got: %s", output)
+	}
+}
+
+func TestNewLoggerTextFormat(t *testing.T) {
+	var buf bytes.Buffer
+	l := NewWithWriter(config.LoggingConfig{Level: "debug", Format: "text"}, &buf)
+
+	l.Debug("debug message")
+
+	output := buf.String()
+	if !strings.Contains(output, "debug message") {
+		t.Errorf("debug log should appear at debug level, got: %s", output)
+	}
+}
+```
+
+- [ ] **Step 8: Run logger test to verify it fails**
+
+```bash
+go test ./internal/logger/ -v
+```
+
+Expected: FAIL
+
+- [ ] **Step 9: Implement logger**
+
+Create `internal/logger/logger.go`:
+
+```go
+package logger
+
+import (
+	"io"
+	"log/slog"
+	"os"
+
+	"agro-sentinel-worker/internal/config"
+)
+
+func New(cfg config.LoggingConfig) *slog.Logger {
+	return NewWithWriter(cfg, os.Stdout)
+}
+
+func NewWithWriter(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
+	level := parseLevel(cfg.Level)
+
+	var handler slog.Handler
+	opts := &slog.HandlerOptions{Level: level}
+
+	if cfg.Format == "json" {
+		handler = slog.NewJSONHandler(w, opts)
+	} else {
+		handler = slog.NewTextHandler(w, opts)
+	}
+
+	return slog.New(handler)
+}
+
+func parseLevel(s string) slog.Level {
+	switch s {
+	case "debug":
+		return slog.LevelDebug
+	case "warn":
+		return slog.LevelWarn
+	case "error":
+		return slog.LevelError
+	default:
+		return slog.LevelInfo
+	}
+}
+```
+
+- [ ] **Step 10: Run logger tests**
+
+```bash
+go test ./internal/logger/ -v
+```
+
+Expected: PASS
+
+- [ ] **Step 11: Write health handler test**
+
+Create `internal/http/handlers_test.go`:
+
+```go
+package http
+
+import (
+	"encoding/json"
+	"net/http"
+	"net/http/httptest"
+	"testing"
+)
+
+func TestHealthHandler(t *testing.T) {
+	req := httptest.NewRequest("GET", "/health", nil)
+	w := httptest.NewRecorder()
+
+	HealthHandler(w, req)
+
+	resp := w.Result()
+	if resp.StatusCode != http.StatusOK {
+		t.Errorf("status = %d, want 200", resp.StatusCode)
+	}
+
+	var body map[string]string
+	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
+		t.Fatalf("decode response: %v", err)
+	}
+
+	if body["status"] != "ok" {
+		t.Errorf("status = %q, want ok", body["status"])
+	}
+}
+```
+
+- [ ] **Step 12: Run handler test to verify it fails**
+
+```bash
+go test ./internal/http/ -v
+```
+
+Expected: FAIL
+
+- [ ] **Step 13: Implement router and handlers**
+
+Create `internal/http/handlers.go`:
+
+```go
+package http
+
+import (
+	"encoding/json"
+	"net/http"
+)
+
+func HealthHandler(w http.ResponseWriter, r *http.Request) {
+	w.Header().Set("Content-Type", "application/json")
+	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
+}
+```
+
+Create `internal/http/router.go`:
+
+```go
+package http
+
+import (
+	"log/slog"
+	"net/http"
+)
+
+func NewRouter(logger *slog.Logger) *http.ServeMux {
+	mux := http.NewServeMux()
+	mux.HandleFunc("GET /health", HealthHandler)
+	return mux
+}
+```
+
+- [ ] **Step 14: Run handler tests**
+
+```bash
+go test ./internal/http/ -v
+```
+
+Expected: PASS
+
+- [ ] **Step 15: Create entry points**
+
+Create `cmd/api/main.go`:
+
+```go
+package main
+
+import (
+	"fmt"
+	"log"
+	"net/http"
+	"os"
+
+	"agro-sentinel-worker/internal/config"
+	apphttp "agro-sentinel-worker/internal/http"
+	"agro-sentinel-worker/internal/logger"
+)
+
+func main() {
+	cfgPath := "configs/config.yaml"
+	if p := os.Getenv("CONFIG_PATH"); p != "" {
+		cfgPath = p
+	}
+
+	cfg, err := config.Load(cfgPath)
+	if err != nil {
+		log.Fatalf("loading config: %v", err)
+	}
+
+	l := logger.New(cfg.Logging)
+	router := apphttp.NewRouter(l)
+
+	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
+	l.Info("API server starting", "addr", addr)
+
+	if err := http.ListenAndServe(addr, router); err != nil {
+		l.Error("server failed", "error", err)
+		os.Exit(1)
+	}
+}
+```
+
+Create `cmd/worker/main.go`:
+
+```go
+package main
+
+import (
+	"fmt"
+	"log"
+	"os"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/logger"
+)
+
+func main() {
+	cfgPath := "configs/config.yaml"
+	if p := os.Getenv("CONFIG_PATH"); p != "" {
+		cfgPath = p
+	}
+
+	cfg, err := config.Load(cfgPath)
+	if err != nil {
+		log.Fatalf("loading config: %v", err)
+	}
+
+	l := logger.New(cfg.Logging)
+	l.Info("worker starting", "name", cfg.App.Name)
+	fmt.Println("worker: no jobs configured yet")
+}
+```
+
+Create `cmd/sync/main.go`:
+
+```go
+package main
+
+import (
+	"fmt"
+	"log"
+	"os"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/logger"
+)
+
+func main() {
+	cfgPath := "configs/config.yaml"
+	if p := os.Getenv("CONFIG_PATH"); p != "" {
+		cfgPath = p
+	}
+
+	cfg, err := config.Load(cfgPath)
+	if err != nil {
+		log.Fatalf("loading config: %v", err)
+	}
+
+	l := logger.New(cfg.Logging)
+	l.Info("sync starting", "name", cfg.App.Name, "interval_minutes", cfg.Sync.IntervalMinutes)
+	fmt.Println("sync: no DynamoDB configured yet")
+}
+```
+
+- [ ] **Step 16: Create example config**
+
+Copy the YAML from the spec into `configs/config.example.yaml` (the full YAML block from the Configuration section of the design spec).
+
+- [ ] **Step 17: Run all tests**
+
+```bash
+go test ./... -v
+```
+
+Expected: ALL PASS
+
+- [ ] **Step 18: Verify API starts**
+
+```bash
+cp configs/config.example.yaml configs/config.yaml
+go run ./cmd/api &
+curl http://localhost:6000/health
+# Expected: {"status":"ok"}
+kill %1
+```
+
+- [ ] **Step 19: Commit**
+
+```bash
+git init
+git add .
+git commit -m "feat: project scaffold with config, logging, and health check"
+```
+
+---
diff --git a/cmd/api/main.go b/cmd/api/main.go
new file mode 100644
index 0000000..9a1dd0b
--- /dev/null
+++ b/cmd/api/main.go
@@ -0,0 +1,35 @@
+package main
+
+import (
+	"fmt"
+	"log"
+	"net/http"
+	"os"
+
+	"agro-sentinel-worker/internal/config"
+	apphttp "agro-sentinel-worker/internal/http"
+	"agro-sentinel-worker/internal/logger"
+)
+
+func main() {
+	cfgPath := "configs/config.yaml"
+	if p := os.Getenv("CONFIG_PATH"); p != "" {
+		cfgPath = p
+	}
+
+	cfg, err := config.Load(cfgPath)
+	if err != nil {
+		log.Fatalf("loading config: %v", err)
+	}
+
+	l := logger.New(cfg.Logging)
+	router := apphttp.NewRouter(l)
+
+	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
+	l.Info("API server starting", "addr", addr)
+
+	if err := http.ListenAndServe(addr, router); err != nil {
+		l.Error("server failed", "error", err)
+		os.Exit(1)
+	}
+}
diff --git a/cmd/sync/main.go b/cmd/sync/main.go
new file mode 100644
index 0000000..e92c51e
--- /dev/null
+++ b/cmd/sync/main.go
@@ -0,0 +1,26 @@
+package main
+
+import (
+	"fmt"
+	"log"
+	"os"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/logger"
+)
+
+func main() {
+	cfgPath := "configs/config.yaml"
+	if p := os.Getenv("CONFIG_PATH"); p != "" {
+		cfgPath = p
+	}
+
+	cfg, err := config.Load(cfgPath)
+	if err != nil {
+		log.Fatalf("loading config: %v", err)
+	}
+
+	l := logger.New(cfg.Logging)
+	l.Info("sync starting", "name", cfg.App.Name, "interval_minutes", cfg.Sync.IntervalMinutes)
+	fmt.Println("sync: no DynamoDB configured yet")
+}
diff --git a/cmd/worker/main.go b/cmd/worker/main.go
new file mode 100644
index 0000000..700c0b5
--- /dev/null
+++ b/cmd/worker/main.go
@@ -0,0 +1,26 @@
+package main
+
+import (
+	"fmt"
+	"log"
+	"os"
+
+	"agro-sentinel-worker/internal/config"
+	"agro-sentinel-worker/internal/logger"
+)
+
+func main() {
+	cfgPath := "configs/config.yaml"
+	if p := os.Getenv("CONFIG_PATH"); p != "" {
+		cfgPath = p
+	}
+
+	cfg, err := config.Load(cfgPath)
+	if err != nil {
+		log.Fatalf("loading config: %v", err)
+	}
+
+	l := logger.New(cfg.Logging)
+	l.Info("worker starting", "name", cfg.App.Name)
+	fmt.Println("worker: no jobs configured yet")
+}
diff --git a/configs/config.example.yaml b/configs/config.example.yaml
new file mode 100644
index 0000000..7e6853f
--- /dev/null
+++ b/configs/config.example.yaml
@@ -0,0 +1,55 @@
+app:
+  name: agro-sentinel-worker
+  environment: development
+
+server:
+  host: 0.0.0.0
+  port: 6000
+
+sync:
+  interval_minutes: 15
+  dias_margen_monitoreo: 30
+
+processing:
+  temp_dir: /tmp/agro-sentinel
+  target_resolution: 10
+  resampling_method: bilinear
+  workers: 1
+  downloads_concurrency: 2
+
+sentinel:
+  cloud_cover_scene_max: 70
+  cloud_cover_production_max: 23
+
+aws:
+  region: us-west-2
+
+s3:
+  bucket: agro-sentinel-bucket
+  prefix: sentinel/producciones
+
+sqs:
+  queue_url: ""
+
+dynamodb:
+  table_producciones: monitoring_producciones
+  table_escenas: monitoring_escenas
+
+mysql:
+  host: localhost
+  port: 3306
+  database: agro
+  user: ""
+  password: ""
+
+ia:
+  enabled: true
+  service_url: http://localhost:8081
+  timeout_seconds: 60
+
+gdal:
+  timeout_seconds: 300
+
+logging:
+  level: info
+  format: json
diff --git a/go.mod b/go.mod
new file mode 100644
index 0000000..c590812
--- /dev/null
+++ b/go.mod
@@ -0,0 +1,5 @@
+module agro-sentinel-worker
+
+go 1.26.5
+
+require gopkg.in/yaml.v3 v3.0.1 // indirect
diff --git a/go.sum b/go.sum
new file mode 100644
index 0000000..4bc0337
--- /dev/null
+++ b/go.sum
@@ -0,0 +1,3 @@
+gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
+gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
+gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
diff --git a/internal/config/config.go b/internal/config/config.go
new file mode 100644
index 0000000..3974bad
--- /dev/null
+++ b/internal/config/config.go
@@ -0,0 +1,133 @@
+package config
+
+import (
+	"fmt"
+	"os"
+
+	"gopkg.in/yaml.v3"
+)
+
+type Config struct {
+	App        AppConfig        `yaml:"app"`
+	Server     ServerConfig     `yaml:"server"`
+	Sync       SyncConfig       `yaml:"sync"`
+	Processing ProcessingConfig `yaml:"processing"`
+	Sentinel   SentinelConfig   `yaml:"sentinel"`
+	AWS        AWSConfig        `yaml:"aws"`
+	S3         S3Config         `yaml:"s3"`
+	SQS        SQSConfig        `yaml:"sqs"`
+	DynamoDB   DynamoDBConfig   `yaml:"dynamodb"`
+	MySQL      MySQLConfig      `yaml:"mysql"`
+	IA         IAConfig         `yaml:"ia"`
+	GDAL       GDALConfig       `yaml:"gdal"`
+	Logging    LoggingConfig    `yaml:"logging"`
+}
+
+type AppConfig struct {
+	Name        string `yaml:"name"`
+	Environment string `yaml:"environment"`
+}
+
+type ServerConfig struct {
+	Host string `yaml:"host"`
+	Port int    `yaml:"port"`
+}
+
+type SyncConfig struct {
+	IntervalMinutes     int `yaml:"interval_minutes"`
+	DiasMargenMonitoreo int `yaml:"dias_margen_monitoreo"`
+}
+
+type ProcessingConfig struct {
+	TempDir              string `yaml:"temp_dir"`
+	TargetResolution     int    `yaml:"target_resolution"`
+	ResamplingMethod     string `yaml:"resampling_method"`
+	Workers              int    `yaml:"workers"`
+	DownloadsConcurrency int    `yaml:"downloads_concurrency"`
+}
+
+type SentinelConfig struct {
+	CloudCoverSceneMax      float64 `yaml:"cloud_cover_scene_max"`
+	CloudCoverProductionMax float64 `yaml:"cloud_cover_production_max"`
+}
+
+type AWSConfig struct {
+	Region string `yaml:"region"`
+}
+
+type S3Config struct {
+	Bucket string `yaml:"bucket"`
+	Prefix string `yaml:"prefix"`
+}
+
+type SQSConfig struct {
+	QueueURL string `yaml:"queue_url"`
+}
+
+type DynamoDBConfig struct {
+	TableProducciones string `yaml:"table_producciones"`
+	TableEscenas      string `yaml:"table_escenas"`
+}
+
+type MySQLConfig struct {
+	Host     string `yaml:"host"`
+	Port     int    `yaml:"port"`
+	Database string `yaml:"database"`
+	User     string `yaml:"user"`
+	Password string `yaml:"password"`
+}
+
+type IAConfig struct {
+	Enabled        bool   `yaml:"enabled"`
+	ServiceURL     string `yaml:"service_url"`
+	TimeoutSeconds int    `yaml:"timeout_seconds"`
+}
+
+type GDALConfig struct {
+	TimeoutSeconds int `yaml:"timeout_seconds"`
+}
+
+type LoggingConfig struct {
+	Level  string `yaml:"level"`
+	Format string `yaml:"format"`
+}
+
+func Load(path string) (*Config, error) {
+	data, err := os.ReadFile(path)
+	if err != nil {
+		return nil, fmt.Errorf("reading config file: %w", err)
+	}
+
+	var cfg Config
+	if err := yaml.Unmarshal(data, &cfg); err != nil {
+		return nil, fmt.Errorf("parsing config file: %w", err)
+	}
+
+	applyEnvOverrides(&cfg)
+
+	return &cfg, nil
+}
+
+func applyEnvOverrides(cfg *Config) {
+	if v := os.Getenv("AWS_REGION"); v != "" {
+		cfg.AWS.Region = v
+	}
+	if v := os.Getenv("S3_BUCKET"); v != "" {
+		cfg.S3.Bucket = v
+	}
+	if v := os.Getenv("SQS_QUEUE_URL"); v != "" {
+		cfg.SQS.QueueURL = v
+	}
+	if v := os.Getenv("MYSQL_HOST"); v != "" {
+		cfg.MySQL.Host = v
+	}
+	if v := os.Getenv("MYSQL_DATABASE"); v != "" {
+		cfg.MySQL.Database = v
+	}
+	if v := os.Getenv("MYSQL_USER"); v != "" {
+		cfg.MySQL.User = v
+	}
+	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
+		cfg.MySQL.Password = v
+	}
+}
diff --git a/internal/config/config_test.go b/internal/config/config_test.go
new file mode 100644
index 0000000..5e76cb1
--- /dev/null
+++ b/internal/config/config_test.go
@@ -0,0 +1,173 @@
+package config
+
+import (
+	"os"
+	"path/filepath"
+	"testing"
+)
+
+func TestLoadConfig(t *testing.T) {
+	yaml := `
+app:
+  name: agro-sentinel-worker
+  environment: test
+
+server:
+  host: 0.0.0.0
+  port: 6000
+
+sync:
+  interval_minutes: 15
+  dias_margen_monitoreo: 30
+
+processing:
+  temp_dir: /tmp/agro-sentinel
+  target_resolution: 10
+  resampling_method: bilinear
+  workers: 1
+  downloads_concurrency: 2
+
+sentinel:
+  cloud_cover_scene_max: 70
+  cloud_cover_production_max: 23
+
+aws:
+  region: us-west-2
+
+s3:
+  bucket: test-bucket
+  prefix: sentinel/producciones
+
+sqs:
+  queue_url: ""
+
+dynamodb:
+  table_producciones: monitoring_producciones
+  table_escenas: monitoring_escenas
+
+mysql:
+  host: localhost
+  port: 3306
+  database: agro
+
+ia:
+  enabled: true
+  service_url: http://localhost:8081
+  timeout_seconds: 60
+
+gdal:
+  timeout_seconds: 300
+
+logging:
+  level: info
+  format: json
+`
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.yaml")
+	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
+		t.Fatal(err)
+	}
+
+	cfg, err := Load(path)
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+
+	if cfg.App.Name != "agro-sentinel-worker" {
+		t.Errorf("app.name = %q, want agro-sentinel-worker", cfg.App.Name)
+	}
+	if cfg.Server.Port != 6000 {
+		t.Errorf("server.port = %d, want 6000", cfg.Server.Port)
+	}
+	if cfg.Processing.TargetResolution != 10 {
+		t.Errorf("processing.target_resolution = %d, want 10", cfg.Processing.TargetResolution)
+	}
+	if cfg.Sentinel.CloudCoverProductionMax != 23 {
+		t.Errorf("sentinel.cloud_cover_production_max = %v, want 23", cfg.Sentinel.CloudCoverProductionMax)
+	}
+	if cfg.Sync.DiasMargenMonitoreo != 30 {
+		t.Errorf("sync.dias_margen_monitoreo = %d, want 30", cfg.Sync.DiasMargenMonitoreo)
+	}
+}
+
+func TestLoadConfigEnvOverride(t *testing.T) {
+	yaml := `
+app:
+  name: agro-sentinel-worker
+  environment: test
+
+server:
+  host: 0.0.0.0
+  port: 6000
+
+sync:
+  interval_minutes: 15
+  dias_margen_monitoreo: 30
+
+processing:
+  temp_dir: /tmp/agro-sentinel
+  target_resolution: 10
+  resampling_method: bilinear
+  workers: 1
+  downloads_concurrency: 2
+
+sentinel:
+  cloud_cover_scene_max: 70
+  cloud_cover_production_max: 23
+
+aws:
+  region: us-west-2
+
+s3:
+  bucket: yaml-bucket
+  prefix: sentinel/producciones
+
+sqs:
+  queue_url: ""
+
+dynamodb:
+  table_producciones: monitoring_producciones
+  table_escenas: monitoring_escenas
+
+mysql:
+  host: localhost
+  port: 3306
+  database: agro
+
+ia:
+  enabled: true
+  service_url: http://localhost:8081
+  timeout_seconds: 60
+
+gdal:
+  timeout_seconds: 300
+
+logging:
+  level: info
+  format: json
+`
+	dir := t.TempDir()
+	path := filepath.Join(dir, "config.yaml")
+	if err := os.WriteFile(path, []byte(yaml), 0644); err != nil {
+		t.Fatal(err)
+	}
+
+	t.Setenv("S3_BUCKET", "env-bucket")
+	t.Setenv("MYSQL_HOST", "db.example.com")
+	t.Setenv("MYSQL_PASSWORD", "secret")
+
+	cfg, err := Load(path)
+	if err != nil {
+		t.Fatalf("Load failed: %v", err)
+	}
+
+	if cfg.S3.Bucket != "env-bucket" {
+		t.Errorf("s3.bucket = %q, want env-bucket (env override)", cfg.S3.Bucket)
+	}
+	if cfg.MySQL.Host != "db.example.com" {
+		t.Errorf("mysql.host = %q, want db.example.com (env override)", cfg.MySQL.Host)
+	}
+	if cfg.MySQL.Password != "secret" {
+		t.Errorf("mysql.password should come from env")
+	}
+}
diff --git a/internal/http/handlers.go b/internal/http/handlers.go
new file mode 100644
index 0000000..38c9c86
--- /dev/null
+++ b/internal/http/handlers.go
@@ -0,0 +1,11 @@
+package http
+
+import (
+	"encoding/json"
+	"net/http"
+)
+
+func HealthHandler(w http.ResponseWriter, r *http.Request) {
+	w.Header().Set("Content-Type", "application/json")
+	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
+}
diff --git a/internal/http/handlers_test.go b/internal/http/handlers_test.go
new file mode 100644
index 0000000..b0be8e0
--- /dev/null
+++ b/internal/http/handlers_test.go
@@ -0,0 +1,29 @@
+package http
+
+import (
+	"encoding/json"
+	"net/http"
+	"net/http/httptest"
+	"testing"
+)
+
+func TestHealthHandler(t *testing.T) {
+	req := httptest.NewRequest("GET", "/health", nil)
+	w := httptest.NewRecorder()
+
+	HealthHandler(w, req)
+
+	resp := w.Result()
+	if resp.StatusCode != http.StatusOK {
+		t.Errorf("status = %d, want 200", resp.StatusCode)
+	}
+
+	var body map[string]string
+	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
+		t.Fatalf("decode response: %v", err)
+	}
+
+	if body["status"] != "ok" {
+		t.Errorf("status = %q, want ok", body["status"])
+	}
+}
diff --git a/internal/http/router.go b/internal/http/router.go
new file mode 100644
index 0000000..b353f54
--- /dev/null
+++ b/internal/http/router.go
@@ -0,0 +1,12 @@
+package http
+
+import (
+	"log/slog"
+	"net/http"
+)
+
+func NewRouter(logger *slog.Logger) *http.ServeMux {
+	mux := http.NewServeMux()
+	mux.HandleFunc("GET /health", HealthHandler)
+	return mux
+}
diff --git a/internal/logger/logger.go b/internal/logger/logger.go
new file mode 100644
index 0000000..7357ee4
--- /dev/null
+++ b/internal/logger/logger.go
@@ -0,0 +1,41 @@
+package logger
+
+import (
+	"io"
+	"log/slog"
+	"os"
+
+	"agro-sentinel-worker/internal/config"
+)
+
+func New(cfg config.LoggingConfig) *slog.Logger {
+	return NewWithWriter(cfg, os.Stdout)
+}
+
+func NewWithWriter(cfg config.LoggingConfig, w io.Writer) *slog.Logger {
+	level := parseLevel(cfg.Level)
+
+	var handler slog.Handler
+	opts := &slog.HandlerOptions{Level: level}
+
+	if cfg.Format == "json" {
+		handler = slog.NewJSONHandler(w, opts)
+	} else {
+		handler = slog.NewTextHandler(w, opts)
+	}
+
+	return slog.New(handler)
+}
+
+func parseLevel(s string) slog.Level {
+	switch s {
+	case "debug":
+		return slog.LevelDebug
+	case "warn":
+		return slog.LevelWarn
+	case "error":
+		return slog.LevelError
+	default:
+		return slog.LevelInfo
+	}
+}
diff --git a/internal/logger/logger_test.go b/internal/logger/logger_test.go
new file mode 100644
index 0000000..15310f0
--- /dev/null
+++ b/internal/logger/logger_test.go
@@ -0,0 +1,36 @@
+package logger
+
+import (
+	"bytes"
+	"strings"
+	"testing"
+
+	"agro-sentinel-worker/internal/config"
+)
+
+func TestNewLogger(t *testing.T) {
+	var buf bytes.Buffer
+	l := NewWithWriter(config.LoggingConfig{Level: "info", Format: "json"}, &buf)
+
+	l.Info("test message", "job_id", "123")
+
+	output := buf.String()
+	if !strings.Contains(output, "test message") {
+		t.Errorf("log output should contain message, got: %s", output)
+	}
+	if !strings.Contains(output, "123") {
+		t.Errorf("log output should contain job_id value, got: %s", output)
+	}
+}
+
+func TestNewLoggerTextFormat(t *testing.T) {
+	var buf bytes.Buffer
+	l := NewWithWriter(config.LoggingConfig{Level: "debug", Format: "text"}, &buf)
+
+	l.Debug("debug message")
+
+	output := buf.String()
+	if !strings.Contains(output, "debug message") {
+		t.Errorf("debug log should appear at debug level, got: %s", output)
+	}
+}
