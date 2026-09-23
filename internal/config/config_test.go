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
	t.Setenv("MYSQL_DB", "erp_test")
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
	if cfg.MySQL.Database != "erp_test" {
		t.Errorf("mysql.database = %q, want erp_test (MYSQL_DB env override)", cfg.MySQL.Database)
	}
	if cfg.MySQL.Password != "secret" {
		t.Errorf("mysql.password should come from env")
	}
}
