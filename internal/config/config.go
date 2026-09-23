package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App        AppConfig        `yaml:"app"`
	Server     ServerConfig     `yaml:"server"`
	Auth       AuthConfig       `yaml:"auth"`
	Sync       SyncConfig       `yaml:"sync"`
	Worker     WorkerConfig     `yaml:"worker"`
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

type AuthConfig struct {
	// JWTSecret es la clave de firma HS256. Mínimo 32 caracteres.
	// Override: AUTH_JWT_SECRET env var.
	JWTSecret string `yaml:"jwt_secret"`
	// TokenTTLHours es la duración del token en horas. Default: 8.
	// Override: AUTH_TOKEN_TTL_HOURS env var.
	TokenTTLHours int `yaml:"token_ttl_hours"`
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
	// Schedule is a standard 5-field cron expression (minute hour dom month dow).
	// When set, it takes precedence over IntervalMinutes.
	// Example: "0 6,18 * * *" → runs at 06:00 and 18:00 in Timezone.
	Schedule string `yaml:"schedule"`
	// Timezone is an IANA timezone name used to interpret Schedule.
	// Example: "America/Mexico_City". Defaults to UTC when empty.
	Timezone string `yaml:"timezone"`
}

type WorkerConfig struct {
	// Schedule is a standard 5-field cron expression (minute hour dom month dow).
	// Example: "0 1 * * *" → runs at 01:00 in Timezone.
	Schedule string `yaml:"schedule"`
	// Timezone is an IANA timezone name used to interpret Schedule.
	// Example: "America/Mexico_City". Defaults to UTC when empty.
	Timezone string `yaml:"timezone"`
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
	Region   string `yaml:"region"`
	Endpoint string `yaml:"endpoint"`
}

type S3Config struct {
	Bucket string `yaml:"bucket"`
	Prefix string `yaml:"prefix"`
	// Region es la región AWS para S3 (ej: us-west-2).
	// Override: S3_REGION env var. Si vacía, hereda de aws.region.
	Region string `yaml:"region"`
	// PublicEndpoint rewrites presigned URLs for browser consumption.
	// Set to "http://localhost:4566" when running LocalStack behind Docker
	// so the browser can reach S3 via the host machine instead of the
	// internal Docker hostname.  Leave empty for real AWS (no rewrite).
	PublicEndpoint string `yaml:"public_endpoint"`
}

type SQSConfig struct {
	QueueURL    string `yaml:"queue_url"`
	MaxMessages int    `yaml:"max_messages"`
	WaitSeconds int    `yaml:"wait_seconds"`
	MaxRetries  int    `yaml:"max_retries"`
}

type DynamoDBConfig struct {
	TableProducciones string `yaml:"table_producciones"`
	TableEscenas      string `yaml:"table_escenas"`
	// AWS connection — when set, DynamoDB uses its own session independent of cfg.AWS.
	// Leave empty to fall back to the global AWS session (LocalStack / env vars).
	Region          string `yaml:"region"`
	Endpoint        string `yaml:"endpoint"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
}

type MySQLConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// IAConfig agrupa la configuración del analizador IA con Bedrock.
// El análisis se dispara manualmente desde la API; no hay cliente HTTP externo.
type IAConfig struct {
	Bedrock BedrockConfig `yaml:"bedrock"`
}

// BedrockConfig holds AWS Bedrock settings for the IA analyzer.
// Auth priority: APIKey (BEDROCK_API_KEY) > AccessKeyID+SecretAccessKey (IA_AWS_*).
type BedrockConfig struct {
	ModelID         string `yaml:"model_id"`
	Region          string `yaml:"region"`
	APIKey          string `yaml:"api_key"`           // overridden by BEDROCK_API_KEY (used as bearer token)
	AccessKeyID     string `yaml:"access_key_id"`     // overridden by IA_AWS_ACCESS_KEY_ID
	SecretAccessKey string `yaml:"secret_access_key"` // overridden by IA_AWS_SECRET_ACCESS_KEY
	TimeoutSeconds  int    `yaml:"timeout_seconds"`
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
	if v := os.Getenv("AWS_ENDPOINT_URL"); v != "" {
		cfg.AWS.Endpoint = v
	}
	if v := os.Getenv("S3_BUCKET"); v != "" {
		cfg.S3.Bucket = v
	}
	if v := os.Getenv("S3_PREFIX"); v != "" {
		cfg.S3.Prefix = v
	}
	if v := os.Getenv("S3_REGION"); v != "" {
		cfg.S3.Region = v
	}
	if v := os.Getenv("S3_PUBLIC_ENDPOINT"); v != "" {
		cfg.S3.PublicEndpoint = v
	}
	if v := os.Getenv("SQS_QUEUE_URL"); v != "" {
		cfg.SQS.QueueURL = v
	}
	if v := os.Getenv("MYSQL_HOST"); v != "" {
		cfg.MySQL.Host = v
	}
	if v := os.Getenv("MYSQL_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.MySQL.Port = p
		}
	}
	if v := os.Getenv("MYSQL_DATABASE"); v != "" {
		cfg.MySQL.Database = v
	} else if v := os.Getenv("MYSQL_DB"); v != "" {
		cfg.MySQL.Database = v
	}
	if v := os.Getenv("MYSQL_USER"); v != "" {
		cfg.MySQL.User = v
	}
	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
		cfg.MySQL.Password = v
	}
	if v := os.Getenv("SYNC_SCHEDULE"); v != "" {
		cfg.Sync.Schedule = v
	}
	if v := os.Getenv("SYNC_TIMEZONE"); v != "" {
		cfg.Sync.Timezone = v
	}
	if v := os.Getenv("AUTH_JWT_SECRET"); v != "" {
		cfg.Auth.JWTSecret = v
	}
	if v := os.Getenv("AUTH_TOKEN_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Auth.TokenTTLHours = n
		}
	}
	if v := os.Getenv("WORKER_SCHEDULE"); v != "" {
		cfg.Worker.Schedule = v
	}
	if v := os.Getenv("WORKER_TIMEZONE"); v != "" {
		cfg.Worker.Timezone = v
	}
	if v := os.Getenv("DYNAMODB_REGION"); v != "" {
		cfg.DynamoDB.Region = v
	}
	if v := os.Getenv("DYNAMODB_TABLE_PRODUCCIONES"); v != "" {
		cfg.DynamoDB.TableProducciones = v
	}
	if v := os.Getenv("DYNAMODB_TABLE_ESCENAS"); v != "" {
		cfg.DynamoDB.TableEscenas = v
	}
	if v := os.Getenv("DYNAMODB_ENDPOINT"); v != "" {
		cfg.DynamoDB.Endpoint = v
	}
	if v := os.Getenv("DYNAMODB_ACCESS_KEY_ID"); v != "" {
		cfg.DynamoDB.AccessKeyID = v
	}
	if v := os.Getenv("DYNAMODB_SECRET_ACCESS_KEY"); v != "" {
		cfg.DynamoDB.SecretAccessKey = v
	}
	if v := os.Getenv("BEDROCK_API_KEY"); v != "" {
		cfg.IA.Bedrock.APIKey = v
	}
	if v := os.Getenv("IA_AWS_ACCESS_KEY_ID"); v != "" {
		cfg.IA.Bedrock.AccessKeyID = v
	}
	if v := os.Getenv("IA_AWS_SECRET_ACCESS_KEY"); v != "" {
		cfg.IA.Bedrock.SecretAccessKey = v
	}
	if v := os.Getenv("IA_BEDROCK_MODEL_ID"); v != "" {
		cfg.IA.Bedrock.ModelID = v
	}
	if v := os.Getenv("IA_BEDROCK_REGION"); v != "" {
		cfg.IA.Bedrock.Region = v
	}
}
