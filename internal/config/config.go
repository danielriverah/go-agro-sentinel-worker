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
