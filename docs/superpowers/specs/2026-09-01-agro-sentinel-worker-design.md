# Agro Sentinel Worker — Design Spec

## Overview

Worker geoespacial en Go que sincroniza producciones agrícolas activas desde DynamoDB a MySQL, procesa escenas Sentinel-2 L2A COG mediante GDAL, genera productos derivados (imágenes, índices, estadísticas con histórico), y expone una API REST documentada con Scalar. Un servicio de IA separado analiza los parámetros generados para recomendaciones de campo.

## Architecture

**Enfoque C — Monolito Go + servicio IA separado.**

Un solo proyecto Go con 3 puntos de entrada (`cmd/api`, `cmd/worker`, `cmd/sync`) que comparten dominio e infraestructura. El servicio de IA vive aparte para permitir LLM o ML con stack tecnológico independiente.

```
DynamoDB (otro sistema escribe)
    ├── Producciones activas (fecha_plantacion, dias_produccion)
    └── Escenas disponibles (stac_assets)
           │
           ▼
    Sync Service (Go, loop cada 15min)
           │
           ├── Sincroniza producciones a MySQL
           ├── Verifica polígono → genera BBOX
           ├── Controla fin de ciclo (fecha_plantacion + dias_produccion + margen)
           └── Encola jobs para escenas pendientes
           │
           ▼
    Processing Worker (Go)
           │
           ├── Crea multiband.tif desde COG via /vsis3/
           ├── Evalúa nubosidad
           ├── Si pasa: genera índices, imágenes, params.json con histórico
           ├── Si no pasa: solo natural.png
           └── Llama servicio IA → puede bloquear por cosecha
           │
           ▼
    API (Go, puerto 6000, Scalar docs)
           │
           └── CRUD producciones, escenas, archivos, jobs, desbloqueo

    Servicio IA (separado)
           │
           └── POST params.json → análisis → recomendaciones
```

## Project Structure

```
agro-sentinel-worker/
├── cmd/
│   ├── api/main.go
│   ├── worker/main.go
│   └── sync/main.go
├── internal/
│   ├── config/config.go
│   ├── domain/
│   │   ├── production.go
│   │   ├── scene.go
│   │   ├── band.go
│   │   ├── job.go
│   │   ├── product.go
│   │   └── analysis.go
│   ├── application/
│   │   ├── services/
│   │   └── usecases/
│   ├── infrastructure/
│   │   ├── aws/
│   │   │   ├── s3.go
│   │   │   ├── sqs.go
│   │   │   └── dynamodb.go
│   │   ├── database/
│   │   │   ├── mysql.go
│   │   │   ├── production_repo.go
│   │   │   ├── scene_repo.go
│   │   │   └── file_repo.go
│   │   ├── gdal/
│   │   │   ├── executor.go
│   │   │   ├── translate.go
│   │   │   ├── warp.go
│   │   │   └── vrt.go
│   │   └── stac/client.go
│   ├── sync/sync.go
│   ├── processing/
│   │   ├── bands.go
│   │   ├── multiband.go
│   │   ├── indices.go
│   │   ├── rgb.go
│   │   ├── params.go
│   │   └── statistics.go
│   ├── storage/filesystem.go
│   ├── logger/logger.go
│   └── http/
│       ├── handlers.go
│       └── router.go
├── configs/config.example.yaml
├── scripts/
├── docs/
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── go.sum
```

## Data Model

### MySQL — Tables created by the worker

**s3_monitoring_producciones**

| Column | Type | Notes |
|--------|------|-------|
| id | BIGINT AUTO_INCREMENT PK | |
| produccion_id | BIGINT UNIQUE | FK a tabla ERP |
| cultivo | VARCHAR(100) NULL | Desde DynamoDB |
| ciclo | VARCHAR(50) NULL | Desde DynamoDB (ej: PV2026) |
| bbox_minx | DECIMAL(10,6) NULL | |
| bbox_miny | DECIMAL(10,6) NULL | |
| bbox_maxx | DECIMAL(10,6) NULL | |
| bbox_maxy | DECIMAL(10,6) NULL | |
| monitoring | TINYINT(1) DEFAULT 1 | 0 si no tiene polígono o ciclo terminado |
| monitoring_motivo | VARCHAR(100) NULL | Razón de desactivación: sin_poligono, ciclo_terminado, sin_fecha |
| bloqueado | TINYINT(1) DEFAULT 0 | 1 si IA detectó cosecha |
| bloqueado_motivo | VARCHAR(255) NULL | |
| bloqueado_at | DATETIME NULL | |
| desbloqueado_por | VARCHAR(100) NULL | |
| target_resolution | INT DEFAULT 10 | |
| cloud_cover_max | DECIMAL(5,2) DEFAULT 23.00 | |
| fecha_plantacion | DATE NULL | Desde DynamoDB |
| dias_produccion | INT NULL | Desde DynamoDB |
| fecha_fin_monitoreo | DATE NULL | Calculado: fecha_plantacion + dias_produccion + margen |
| total_escenas | INT DEFAULT 0 | Contador de escenas procesadas |
| total_escenas_validas | INT DEFAULT 0 | Escenas que pasaron calidad |
| last_sync_at | DATETIME NULL | |
| created_at | DATETIME | |
| updated_at | DATETIME | |

**s3_monitoring_escenas**

| Column | Type | Notes |
|--------|------|-------|
| id | BIGINT AUTO_INCREMENT PK | |
| produccion_id | BIGINT | FK s3_monitoring_producciones.produccion_id |
| scene_id | VARCHAR(100) | ej: S2A_MSIL2A_20260901... |
| scene_date | DATE | |
| cloud_cover_scene | DECIMAL(5,2) | % nubosidad escena completa |
| cloud_cover_bbox | DECIMAL(5,2) NULL | % nubosidad en el recorte |
| passes_quality | TINYINT(1) DEFAULT 0 | 1 si cloud_cover_bbox <= max |
| has_multiband | TINYINT(1) DEFAULT 0 | |
| has_params | TINYINT(1) DEFAULT 0 | |
| has_rgb | TINYINT(1) DEFAULT 0 | |
| has_analisis | TINYINT(1) DEFAULT 0 | |
| status | VARCHAR(20) DEFAULT 'PENDING' | PENDING, PROCESSING, COMPLETED, FAILED |
| error_type | VARCHAR(50) NULL | GDAL_ERROR, S3_ERROR, TIMEOUT, etc. |
| error_message | TEXT NULL | |
| retry_count | INT DEFAULT 0 | |
| processed_at | DATETIME NULL | |
| created_at | DATETIME | |
| updated_at | DATETIME | |
| | | UNIQUE KEY (produccion_id, scene_id) |

**s3_monitoring_escena_archivos**

| Column | Type | Notes |
|--------|------|-------|
| id | BIGINT AUTO_INCREMENT PK | |
| escena_id | BIGINT | FK s3_monitoring_escenas.id |
| file_type | VARCHAR(50) | multiband, natural, ndvi, ndre, evi, gndvi, nbr, ndmi, savi, red_edge, false_color, swir, params, analisis |
| file_name | VARCHAR(255) | |
| s3_key | VARCHAR(500) | |
| s3_bucket | VARCHAR(100) | |
| file_size_bytes | BIGINT NULL | |
| resolution_m | INT NULL | |
| width_px | INT NULL | |
| height_px | INT NULL | |
| created_at | DATETIME | |

### DynamoDB — Read-only by worker

**monitoring_producciones**
```json
{
  "produccion_id": 1234,
  "activa": true,
  "cultivo": "maíz",
  "ciclo": "PV2026",
  "fecha_plantacion": "2026-07-15",
  "dias_produccion": 150
}
```

**monitoring_escenas**
```json
{
  "scene_id": "S2A_MSIL2A_20260901...",
  "produccion_id": 1234,
  "date": "2026-09-01",
  "cloud_cover": 15.3,
  "stac_assets": {
    "B02": {"href": "s3://sentinel-cogs/.../B02.tif", "resolution": 10},
    "B03": {"href": "...", "resolution": 10},
    "B04": {"href": "...", "resolution": 10},
    "B05": {"href": "...", "resolution": 20},
    "B06": {"href": "...", "resolution": 20},
    "B07": {"href": "...", "resolution": 20},
    "B08": {"href": "...", "resolution": 10},
    "B8A": {"href": "...", "resolution": 20},
    "B11": {"href": "...", "resolution": 20},
    "B12": {"href": "...", "resolution": 20}
  }
}
```

### MySQL ERP — Read-only by worker

**asignaciones_zonas_producciones** — polígono por produccion_id.

### S3 Output Structure

```
s3://bucket/sentinel/producciones/
    {produccion_id}/
        {scene_id}/
            multiband.tif
            natural.png
            false_color.png
            ndvi.png
            ndre.png
            evi.png
            gndvi.png
            nbr.png
            ndmi.png
            savi.png
            red_edge.png
            swir.png
            params.json
            analisis.json
```

## Sync Service Flow

Runs as a periodic loop (configurable, default 15 minutes).

```
SYNC CYCLE
    │
    ▼
1. Read active productions from DynamoDB
    │
    ▼
2. For each production:
    ├── Exists in s3_monitoring_producciones?
    │     NO → INSERT
    │
    ├── Has fecha_plantacion?
    │     NO → SET monitoring=0, SKIP
    │
    ├── today > fecha_plantacion + dias_produccion + dias_margen?
    │     YES → SET monitoring=0, SKIP (cycle ended)
    │
    ├── bloqueado=1?
    │     YES → SKIP (blocked by IA, only user can unblock)
    │
    ├── Query polygon from asignaciones_zonas_producciones
    │     No polygon? → SET monitoring=0, SKIP
    │     Has polygon, no BBOX? → Calculate BBOX → UPDATE
    │
    ▼
3. Read scenes from DynamoDB for this production
    │
    ▼
4. For each scene:
    ├── Exists in s3_monitoring_escenas?
    │     NO → INSERT with status=PENDING
    │
    ├── status == PENDING and monitoring=1?
    │     YES → Enqueue processing job
    │
    └── Continue
```

### BBOX Calculation

The polygon from `asignaciones_zonas_producciones` is converted to its bounding box (min/max lon/lat). Calculated once and stored. Recalculated if polygon changes.

### Cycle End Control

```
fecha_fin_monitoreo = fecha_plantacion + dias_produccion + dias_margen_monitoreo
```

`dias_margen_monitoreo` defaults to 30, configurable. Prevents premature cutoff since `dias_produccion` is an estimate.

## Processing Worker Flow

```
JOB RECEIVED (production + scene)
    │
    ▼
1. Validate: monitoring=1, bbox exists, not bloqueado
    │
    ▼
2. Does multiband.tif exist in S3?
    │
    NO ─────────────────────────────────┐
    │                                    │
    ▼                                    │
3. Get band hrefs from scene record       │
    │                                    │
    ▼                                    │
4. For each band (B02-B12):              │
   GDAL reads COG fragment via /vsis3/   │
   Crop by BBOX                          │
   Resample to target_resolution (10m)   │
    │                                    │
    ▼                                    │
5. Compose multiband.tif                  │
   (all bands aligned to 10m)            │
    │                                    │
    ▼                                    │
6. Upload multiband.tif to S3            │
   UPDATE has_multiband=1                │
    │◄───────────────────────────────────┘
    ▼                               (already existed)
7. Evaluate cloud cover in crop
   cloud_cover_bbox = calculate from SCL band
    │
    ├── cloud_cover_bbox > cloud_cover_max (23%)
    │     │
    │     ▼
    │   8a. Generate ONLY natural.png (RGB B04,B03,B02 at 10m)
    │       Upload to S3
    │       UPDATE has_rgb=1, passes_quality=0
    │       → COMPLETED
    │
    └── cloud_cover_bbox <= cloud_cover_max
          │
          ▼
        8b. Generate images:
          ├── natural.png (RGB: B04,B03,B02) — Color natural
          ├── false_color.png (B08,B04,B03) — Falso color infrarrojo
          ├── ndvi.png — Índice de vegetación
          ├── ndre.png — Estrés temprano
          ├── evi.png — Vigor en alta biomasa
          ├── gndvi.png — Contenido clorofila
          ├── nbr.png — Estrés hídrico/quemaduras
          ├── ndmi.png — Humedad en planta
          ├── savi.png — Vigor compensando suelo
          ├── red_edge.png (B06,B05,B04) — Borde rojo
          ├── swir.png (B12,B8A,B04) — Infrarrojo de onda corta
          │
          ▼
        9. Calculate parameters:
          ├── Stats per band (mean, std, min, max, p25, p50, p75)
          ├── Indices: NDVI, NDRE, EVI, GNDVI, NBR, NDMI, SAVI
          │   Each: mean, std, min, max, % coverage by range
          ├── Total area, vegetated area, exposed soil area
          │
          ▼
        10. Build historical chain:
          ├── Find previous scene for this production (passes_quality=1)
          ├── Read its params.json from S3
          ├── Chain: historico = [prev_scene.values + prev_scene.historico]
          │
          ▼
        11. Generate params.json (see Params Schema below)
          │
          ▼
        12. Call IA service with params.json
          ├── posible_cosecha: true → SET bloqueado=1
          ├── Save analisis.json to S3
          │
          ▼
        13. Upload all to S3
            Register files in s3_monitoring_escena_archivos
            UPDATE has_params=1, has_rgb=1, passes_quality=1
            → COMPLETED
```

## Params Schema

```json
{
  "produccion_id": 1234,
  "scene_id": "S2A_...",
  "scene_date": "2026-09-01",
  "cloud_cover_bbox": 12.5,
  "dias_desde_plantacion": 45,
  "indices": {
    "ndvi": {"mean": 0.72, "std": 0.08, "min": 0.15, "max": 0.92, "p25": 0.65, "p50": 0.74, "p75": 0.81},
    "ndre": {"mean": 0.45, "std": 0.05, "...": "..."},
    "evi":  {"...": "..."},
    "gndvi": {"...": "..."},
    "nbr":  {"...": "..."},
    "ndmi": {"...": "..."},
    "savi": {"...": "..."}
  },
  "estadisticas_bandas": {
    "B02": {"mean": 0.08, "std": 0.02, "min": 0.01, "max": 0.25},
    "B03": {"...": "..."},
    "...": "..."
  },
  "cobertura": {
    "vegetacion_pct": 78.5,
    "suelo_pct": 15.2,
    "agua_pct": 0.3,
    "nubes_pct": 6.0
  },
  "historico": [
    {
      "scene_id": "S2A_...",
      "scene_date": "2026-08-22",
      "dias_desde_plantacion": 35,
      "indices": {"ndvi": {"mean": 0.60, "...": "..."}, "...": "..."},
      "cobertura": {"vegetacion_pct": 65.0, "...": "..."},
      "delta": {
        "ndvi_mean_change": 0.12,
        "ndre_mean_change": 0.05,
        "vegetacion_pct_change": 13.5
      }
    }
  ]
}
```

The `historico` array is a chain: each entry includes the previous scene's values plus the previous scene's own `historico`, forming a complete timeline from first scene to current.

### Historical chain size control

To prevent unbounded growth, the chain keeps the last 20 entries (approximately 1 year of Sentinel-2 revisit at 5-day intervals). Older entries are summarized into an `historico_resumen` field with monthly averages.

## SCL Band (Scene Classification Layer)

The cloud cover within the BBOX is calculated from the SCL (Scene Classification Layer) band included in Sentinel-2 L2A products. SCL classifies each pixel into categories:

| Value | Class | Treatment |
|-------|-------|-----------|
| 0 | No data | Exclude |
| 1 | Saturated/defective | Exclude |
| 2 | Dark area shadows | Include as valid |
| 3 | Cloud shadows | Count as cloud |
| 4 | Vegetation | Valid |
| 5 | Bare soils | Valid |
| 6 | Water | Valid |
| 7 | Unclassified | Valid |
| 8 | Cloud medium probability | Count as cloud |
| 9 | Cloud high probability | Count as cloud |
| 10 | Thin cirrus | Count as cloud |
| 11 | Snow/ice | Valid |

`cloud_cover_bbox = (pixels_3 + pixels_8 + pixels_9 + pixels_10) / total_valid_pixels * 100`

The SCL band is at 20m resolution and must be obtained alongside the spectral bands from the STAC assets in DynamoDB. It is NOT included in multiband.tif — it is used only for the cloud cover calculation and for the `cobertura` classification in params.json.

## Index Formulas

| Index | Formula | Bands | Purpose |
|-------|---------|-------|---------|
| NDVI | (B08-B04)/(B08+B04) | 10m, 10m | General vegetation vigor |
| NDRE | (B08-B05)/(B08+B05) | 10m, 20m→10m | Early stress detection |
| EVI | 2.5*(B08-B04)/(B08+6*B04-7.5*B02+1) | 10m | Dense canopy vigor |
| GNDVI | (B08-B03)/(B08+B03) | 10m, 10m | Chlorophyll content |
| NBR | (B08-B12)/(B08+B12) | 10m, 20m→10m | Severe hydric stress, burns |
| NDMI | (B08-B11)/(B08+B11) | 10m, 20m→10m | Plant moisture content |
| SAVI | 1.5*(B08-B04)/(B08+B04+0.5) | 10m, 10m | Vigor compensating exposed soil |

20m bands are resampled to 10m using bilinear interpolation before index calculation.

## Band Combination Images

| Image | Bands (R,G,B) | Name |
|-------|---------------|------|
| natural.png | B04, B03, B02 | Color Natural |
| false_color.png | B08, B04, B03 | Falso Color Infrarrojo |
| ndvi.png | computed | Índice de Vegetación de Diferencia Normalizada |
| ndre.png | computed | Índice de Borde Rojo de Diferencia Normalizada |
| evi.png | computed | Índice de Vegetación Mejorado |
| gndvi.png | computed | Índice de Vegetación de Diferencia Normalizada Verde |
| nbr.png | computed | Índice de Área Quemada Normalizado |
| ndmi.png | computed | Índice de Humedad de Diferencia Normalizada |
| savi.png | computed | Índice de Vegetación Ajustado al Suelo |
| red_edge.png | B06, B05, B04 | Composición Borde Rojo |
| swir.png | B12, B8A, B04 | Infrarrojo de Onda Corta |

## API Endpoints

Server runs on port **6000**. Documentation via **Scalar** at `/docs`.

```
GET  /health
GET  /health/dependencies

GET  /api/v1/producciones
GET  /api/v1/producciones/{id}
POST /api/v1/producciones/{id}/desbloquear

GET  /api/v1/producciones/{id}/escenas
GET  /api/v1/escenas/{id}
GET  /api/v1/escenas/{id}/params

GET  /api/v1/escenas/{id}/archivos
GET  /api/v1/escenas/{id}/archivos/{tipo}    -- presigned S3 URL

POST /api/v1/jobs
GET  /api/v1/jobs/{id}

POST /api/v1/sync/trigger

POST /api/v1/escenas/{id}/analisis
GET  /api/v1/escenas/{id}/analisis
```

## IA Service

Separate service (Go or Python). Interface:

```
POST /analyze
Body: params.json content
Response:
{
  "estado_general": "bueno|regular|malo",
  "vigor_vegetativo": "alto|medio|bajo",
  "estres_detectado": true|false,
  "posible_cosecha": true|false,
  "recomendaciones": ["..."],
  "alertas": ["..."],
  "confianza": 0.85
}
```

When `posible_cosecha: true`, the worker blocks the production (`bloqueado=1`). Only a user can unblock via `POST /api/v1/producciones/{id}/desbloquear`.

The service starts with LLM (Claude/GPT). Architecture allows replacing with a trained ML model without changing the worker.

## Configuration

```yaml
app:
  name: agro-sentinel-worker
  environment: development

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
  bucket: agro-sentinel-bucket
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
```

Credentials via environment variables or IAM roles — never in config files.

## Development Phases

| Phase | Scope |
|-------|-------|
| 1 | Go project, config, logging, health check, MySQL migrations |
| 2 | GDAL executor (gdalinfo, gdal_translate, gdalwarp) |
| 3 | AWS clients: S3 (upload/download/presigned), DynamoDB (read) |
| 4 | Sync service: DynamoDB → MySQL, polygon → BBOX, cycle control |
| 5 | Processing core: COG via /vsis3/, BBOX crop, resampling, multiband.tif |
| 6 | Products: natural.png, index/combination images |
| 7 | Params + historical: index calculation, statistics, params.json chain |
| 8 | API + Scalar: REST endpoints, documentation |
| 9 | IA service: separate service, worker integration, harvest blocking |
| 10 | SQS + Jobs: async processing, idempotency, retries |
| 11 | Frontend: web application consuming the API |

## Docker

```yaml
# docker-compose.yml services:
api:        Go binary, port 6000
sync:       Go binary
worker:     Go binary
mysql:      MySQL 8
localstack: S3/SQS/DynamoDB for development
ia-service: IA service (when developed)
```

Multi-stage Dockerfile: compile Go, copy binaries to GDAL-enabled image.

## Design Principles

1. Go coordinates. GDAL processes raster.
2. DynamoDB is read-only for the worker. MySQL is the worker's own state.
3. Jobs are idempotent. Same scene+production processed once.
4. Bands are never loaded fully into RAM. COG + GDAL windowed reads.
5. Concurrency is configurable and conservative.
6. 10/20/60m resolutions handled explicitly. Default target: 10m.
7. IA is decoupled — separate service, replaceable backend.
8. Harvest detection blocks monitoring; only users unblock.
9. Historical chain in params.json enables trend analysis.
10. Temp files are per-job and cleaned up after completion.
11. Every stage has a timeout.
12. Credentials never in code or config files.
13. SCL band used for cloud cover calculation, not included in multiband.tif.
14. Historical chain capped at 20 entries; older data summarized monthly.
15. Error classification (GDAL_ERROR, S3_ERROR, TIMEOUT, etc.) enables targeted retry logic.

## Error Handling and Retries

Failed scenes are retried up to 3 times (configurable). The `error_type` field enables targeted handling:

| Error Type | Retry | Strategy |
|------------|-------|----------|
| GDAL_ERROR | Yes | Retry with exponential backoff |
| S3_ERROR | Yes | Retry after 60s |
| TIMEOUT | Yes | Retry with increased timeout |
| VALIDATION_ERROR | No | Requires manual intervention |
| STAC_ERROR | Yes | Retry after 120s |
| IA_ERROR | Partial | Scene marked COMPLETED without analisis.json; IA retried separately |

When IA service is unavailable, the scene still completes (params.json and images are generated). The IA analysis can be triggered later via `POST /api/v1/escenas/{id}/analisis`.
