package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	crypto_rand "crypto/rand"
	"time"

	"agro-sentinel-worker/internal/auth"
	"agro-sentinel-worker/internal/config"
	apphttp "agro-sentinel-worker/internal/http"
	"agro-sentinel-worker/internal/health"
	"agro-sentinel-worker/internal/ia"
	"agro-sentinel-worker/internal/infrastructure/aws"
	"agro-sentinel-worker/internal/infrastructure/database"
	"agro-sentinel-worker/internal/logger"
	"agro-sentinel-worker/internal/retry"
	"agro-sentinel-worker/internal/sync"
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Connect to MySQL with retry.
	db, err := connectMySQLWithRetry(ctx, cfg.MySQL, l)
	if err != nil {
		l.Error("could not connect to mysql — api will not start", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	awsCfg, err := aws.NewSession(cfg.AWS)
	if err != nil {
		l.Error("creating aws session failed", "error", err)
		os.Exit(1)
	}

	dynamoCfg, err := aws.NewDynamoDBSession(cfg.DynamoDB)
	if err != nil {
		l.Error("creating dynamodb session failed", "error", err)
		os.Exit(1)
	}

	// Validate all dependencies before continuing.
	dynamoClient := aws.NewDynamoDBClient(dynamoCfg)
	s3Client := aws.NewS3Client(awsCfg)

	if err := health.CheckDynamoDB(ctx, dynamoClient, cfg.DynamoDB.TableProducciones); err != nil {
		l.Error("dynamodb health check failed", "error", err)
		os.Exit(1)
	}
	l.Info("dynamodb health check passed")

	if err := health.CheckS3(ctx, s3Client, cfg.S3.Bucket); err != nil {
		l.Error("s3 health check failed", "error", err)
		os.Exit(1)
	}
	l.Info("s3 health check passed")

	if cfg.S3.PublicEndpoint != "" {
		s3Client = s3Client.WithPublicEndpoint(cfg.S3.PublicEndpoint)
	}

	prodRepo := database.NewProductionRepo(db)
	sceneRepo := database.NewSceneRepo(db)
	fileRepo := database.NewFileRepo(db)
	iaResultRepo := database.NewIAResultRepository(db)
	alertaRepo := database.NewAlertaRepo(db)
	if !alertaRepo.Disponible() {
		l.Warn("monitoring_alertas no existe: los análisis IA no generarán avisos")
	}

	permRepo := database.NewPermissionRepo(db)
	if permRepo.Disponible() {
		permRepo.Bootstrap(ctx, l)
		l.Info("permission system active")
	} else {
		l.Warn("auth_permisos table not found, running in permissive mode")
	}

	var syncer apphttp.Syncer
	polygonRepo := database.NewPolygonRepo(db, sync.CalculateBBoxFromWKT)
	syncer = sync.New(
		dynamoClient,
		prodRepo,
		sceneRepo,
		polygonRepo,
		s3Client,
		fileRepo,
		iaResultRepo,
		cfg.Sync,
		cfg.Sentinel,
		cfg.DynamoDB.TableProducciones,
		cfg.DynamoDB.TableEscenas,
		cfg.S3.Bucket,
		l,
	)

	// IA Analyzer — optional; only wired when Bedrock credentials are present.
	var iaTriggerer apphttp.IATriggerer
	hasSecret := cfg.IA.Bedrock.SecretAccessKey != "" || cfg.IA.Bedrock.APIKey != ""
	if cfg.IA.Bedrock.AccessKeyID != "" && hasSecret && cfg.IA.Bedrock.ModelID != "" {
		bedrockClient, bedrockErr := aws.NewBedrockClient(cfg.IA.Bedrock)
		if bedrockErr != nil {
			l.Warn("Bedrock client not configured — IA endpoints will return 503", "error", bedrockErr)
		} else {
			iaTriggerer = ia.New(ia.Deps{
				S3:          s3Client,
				S3Up:        s3Client,
				Files:       fileRepo,
				Bedrock:     bedrockClient,
				Results:     iaResultRepo,
				Scenes:      sceneRepo,
				SceneInfo:   sceneRepo,
				Productions: prodRepo,
				Alertas:     alertaRepo,
				Bucket:      cfg.S3.Bucket,
				TempDir:     cfg.Processing.TempDir,
				Logger:      l,
			})
			l.Info("Bedrock IA analyzer configured", "model", cfg.IA.Bedrock.ModelID)
		}
	}

	h := &apphttp.Handlers{
		Productions: prodRepo,
		Scenes:      sceneRepo,
		Timeline:    sceneRepo,
		Fases:       database.NewFaseRepo(db),
		Alertas:     alertaRepo,
		Files:       fileRepo,
		IAResults:   iaResultRepo,
		IA:          iaTriggerer,
		S3:          s3Client,
		S3Stream:    s3Client,
		S3Bucket:    cfg.S3.Bucket,
		S3Prefix:    cfg.S3.Prefix,
		Sync:        syncer,

		S3Delete:                s3Client,
		DynamoDelete:            dynamoClient,
		Monitoreo:               database.NewMonitoreoRepo(db),
		DynamoTablaProducciones: cfg.DynamoDB.TableProducciones,
		DynamoTablaEscenas:      cfg.DynamoDB.TableEscenas,
		DeleteAllowedUserIDs:    cfg.Auth.DeleteAllowedUserIDs,

		Log: l,
		DB:          db,
		S3Health:    s3Client,
		DynamoDB:    dynamoClient,
		GDAL:        apphttp.NewGDALCommand(),

		Permisos:     permRepo,
		PermisosRepo: permRepo,
	}

	// Auth — JWT secret must be ≥32 bytes.
	// Production: fail-closed if missing (predictable secret = forged tokens).
	// Dev: generate a random ephemeral key — tokens don't survive restarts, which is fine.
	jwtSecret := []byte(cfg.Auth.JWTSecret)
	if len(jwtSecret) < 32 {
		isProd := cfg.App.Environment == "production" || cfg.App.Environment == "prod"
		if isProd {
			l.Error("AUTH_JWT_SECRET is missing or shorter than 32 bytes — refusing to start in production")
			os.Exit(1)
		}
		ephemeral := make([]byte, 32)
		if _, err := crypto_rand.Read(ephemeral); err != nil {
			l.Error("failed to generate ephemeral JWT secret", "error", err)
			os.Exit(1)
		}
		jwtSecret = ephemeral
		l.Warn("AUTH_JWT_SECRET not set — using ephemeral random key (dev only, tokens expire on restart)")
	}
	tokenTTL := time.Duration(cfg.Auth.TokenTTLHours) * time.Hour
	if tokenTTL <= 0 {
		tokenTTL = 8 * time.Hour
	}

	authH := &apphttp.AuthHandlers{
		Repo:      auth.NewRepo(db),
		SecretKey: jwtSecret,
		TokenTTL:  tokenTTL,
		Log:       l,
	}

	// El borrado de monitoreo es irreversible, así que por omisión queda
	// cerrado. Decirlo al arrancar evita que alguien lo dé por roto al ver un 403.
	if len(cfg.Auth.DeleteAllowedUserIDs) == 0 {
		l.Warn("eliminar monitoreo está deshabilitado: ningún usuario autorizado — configura AUTH_DELETE_ALLOWED_USER_IDS")
	} else {
		l.Info("eliminar monitoreo habilitado", "usuarios_autorizados", len(cfg.Auth.DeleteAllowedUserIDs))
	}

	router := apphttp.NewRouter(l, h, authH, jwtSecret)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	l.Info("API server starting", "addr", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		l.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// connectMySQLWithRetry connects to MySQL with exponential backoff retry logic.
func connectMySQLWithRetry(ctx context.Context, cfg config.MySQLConfig, l interface {
	Info(string, ...any)
	Error(string, ...any)
}) (*sql.DB, error) {
	cfg2 := retry.APIConfig
	attempt := 0
	var db *sql.DB

	err := retry.WithBackoff(ctx, cfg2, func() error {
		attempt++
		var connErr error
		db, connErr = database.NewConnection(cfg)
		if connErr == nil {
			l.Info("mysql connected", "attempt", attempt)
			return nil
		}
		if attempt < cfg2.MaxAttempts {
			l.Error("mysql connection failed, retrying",
				"attempt", attempt,
				"max", cfg2.MaxAttempts,
				"error", connErr)
		}
		return connErr
	})

	if err != nil {
		return nil, err
	}

	return db, nil
}
