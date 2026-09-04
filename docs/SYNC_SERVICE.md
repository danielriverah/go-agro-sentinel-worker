# Synchronization Service - Agro Sentinel Worker

**Versión:** 1.0  
**Objetivo:** Sincronización periódica de producciones y escenas desde DynamoDB a MySQL  
**Ubicación:** `internal/sync/sync.go`  
**Última actualización:** 2026-09-04

---

## Descripción General

El servicio de sincronización ejecuta un ciclo periódico que sincroniza datos de DynamoDB a MySQL, permitiendo que el worker procese producciones agrícolas de forma local. El flujo es:

```
DynamoDB (Source) → Sync Service → MySQL (Local Cache) → Worker Processing
```

El servicio decide automáticamente qué producciones deben monitorearse basándose en reglas de negocio: validación de fechas de plantación, cálculo de fin de ciclo y disponibilidad de polígonos en SIG.

---

## Responsabilidades Principales

### 1. Lectura desde DynamoDB
- Lista todas las producciones activas de la tabla `monitoring_producciones`
- Filtra por `activa = true` para evitar escaneos innecesarios
- Obtiene escenas satelitales de `monitoring_escenas` por producción

### 2. Sincronización a MySQL
- Inserta o actualiza registros en tabla `s3_monitoring_producciones`
- Preserva campos locales (bloqueado, target_resolution, cloud_cover_max)
- Upsert de escenas en `s3_monitoring_escenas` si producción está monitoreada

### 3. Evaluación de Monitoreo
- Valida presencia de fecha de plantación
- Calcula fecha de fin de monitoreo (`FechaPlantacion + DiasProduccion + DiasMargen`)
- Verifica disponibilidad de polígono (bounding box) en tabla de asignaciones
- Asigna motivo de no monitoreo si falla validación

### 4. Gestión de Estado
- Respeta el flag `bloqueado` (no sobrescribe decisiones administrativas)
- Registra timestamp de última sincronización (`LastSyncAt`)
- Implementa idempotencia: escenas duplicadas son detectadas y descartadas

---

## Interfaces y Métodos Principales

### Tipo: Service

Contiene las dependencias inyectadas y configuración del servicio:

```go
type Service struct {
    dynamo          DynamoReader              // Cliente DynamoDB
    prodRepo        ProductionRepository      // Repositorio MySQL de producciones
    sceneRepo       SceneRepository           // Repositorio MySQL de escenas
    polygonRepo     PolygonRepository         // Consulta bounding boxes
    cfg             config.SyncConfig         // Config de sincronización
    sentinel        config.SentinelConfig     // Config de satélite
    tableProducciones string                  // Nombre tabla DynamoDB producciones
    tableEscenas    string                    // Nombre tabla DynamoDB escenas
    logger          *slog.Logger              // Logger estructurado
    now             func() time.Time          // Testeable
}
```

### Método: New()

Constructor que inyecta dependencias:

```go
func New(
    dynamo DynamoReader,
    prodRepo ProductionRepository,
    sceneRepo SceneRepository,
    polygonRepo PolygonRepository,
    cfg config.SyncConfig,
    sentinel config.SentinelConfig,
    tableProducciones string,
    tableEscenas string,
    logger *slog.Logger,
) *Service
```

**Responsabilidad:** Inicializa el servicio con validaciones básicas. Si logger es nil, usa slog.Default().

### Método: RunOnce()

Ejecuta un ciclo completo de sincronización:

```go
func (s *Service) RunOnce(ctx context.Context) error
```

**Proceso:**
1. Obtiene todas las producciones activas de DynamoDB (`ListActiveProducciones`)
2. Para cada producción, llama a `syncProduccion()` 
3. Captura errores de sincronización individual pero continúa con siguiente
4. Retorna error solo si falla lectura de DynamoDB

**Retorna:** `ProcessingError` tipo `ErrDynamoDB` si falla escaneo inicial

### Método: RunLoop()

Ejecuta RunOnce inmediatamente y luego cada `cfg.IntervalMinutes`:

```go
func (s *Service) RunLoop(ctx context.Context) error
```

**Comportamiento:**
- Primer ciclo ejecuta sin esperar
- Usa `time.Ticker` con intervalo configurable (default: 1 min si <= 0)
- Respeta `ctx.Done()` para shutdown graceful
- Registra errores pero continúa ejecutándose

---

## Ciclo de Ejecución Detallado

### Fase 1: Obtención de Producciones

```
DynamoDB.ListActiveProducciones()
    ↓
FilterExpression: activa = true
    ↓
Retorna: []DynamoProduction
    ↓
Para cada producción: syncProduccion(ctx, dynaProduction)
```

### Fase 2: Sincronización por Producción (syncProduccion)

```
1. GetByProduccionID(ctx, produccionID)
   └─ Obtiene estado existente en MySQL (si existe)

2. buildProduction(dp, existing)
   └─ Merge de datos DynamoDB con estado local
   └─ Preserva: bloqueado, bloqueado_motivo, target_resolution, cloud_cover_max
   └─ Mapea campos Phase 3: ArticuloID, CentroCostoID, NombreRancho
   └─ Parsea fecha de plantación (formato YYYY-MM-DD)

3. Si existing.Bloqueado:
   └─ Saltea sincronización (respeta bloqueo administrativo)
   └─ Log: "skipping blocked produccion"

4. evaluateMonitoring(ctx, &prod)
   ├─ ¿FechaPlantacion != nil? 
   │  └─ Si no: motivo = "SIN_FECHA_PLANTACION" → monitoring = false
   │
   ├─ Calcula: FechaFinMonitoreo = FechaPlantacion + DiasProduccion + DiasMargen
   │  └─ Guarda en prod.FechaFinMonitoreo
   │
   ├─ ¿now() > FechaFinMonitoreo?
   │  └─ Si: motivo = "FIN_MONITOREO_ALCANZADO" → monitoring = false
   │
   ├─ GetPolygonBBox(ctx, produccionID)
   │  └─ Consulta tabla asignaciones_zonas_producciones
   │  └─ Extrae BBOX del WKT polygon
   │  └─ Si error o nil: motivo = "SIN_POLIGONO" → monitoring = false
   │
   └─ Si todo OK: motivo = "OK" → monitoring = true

5. prod.Monitoring = monitoring
   prod.MonitoringMotivo = motivo
   prod.LastSyncAt = &now()

6. prodRepo.Upsert(ctx, &prod)
   └─ Inserta o actualiza en MySQL

7. Si monitoring == true:
   └─ syncEscenas(ctx, produccionID)
      └─ (Ver Fase 3)
```

### Fase 3: Sincronización de Escenas (syncEscenas)

```
1. dynamo.ListEscenas(ctx, tableEscenas, produccionID)
   └─ Obtiene todas las escenas de la producción desde DynamoDB

2. Para cada escena DynamoDB:
   
   a) ¿CloudCover > CloudCoverSceneMax?
      └─ Si: continue (rechazar escena nublada, defecto 70%)
   
   b) GetByProduccionAndSceneID(ctx, produccionID, sceneID)
      └─ ¿Escena ya existe en MySQL?
      └─ Si: continue (detecta duplicado, idempotente)
   
   c) parseSceneDate(date)
      └─ Intenta formato YYYY-MM-DD
      └─ Fallback: RFC3339
   
   d) Crea domain.Scene
      ├─ ProduccionID: produccionID
      ├─ SceneID: de.SceneID
      ├─ SceneDate: sceneDate
      ├─ CloudCoverScene: de.CloudCover
      └─ Status: domain.StatusPending (siempre, para worker)
   
   e) sceneRepo.Upsert(ctx, scene)
      └─ Inserta en MySQL (con UNIQUE constraint)
```

---

## Manejo de Errores y Reintentos

### Estrategia de Errores

**Nivel de Producción (syncProduccion):**
- Si obtiene producción: error capturado, loguea y continúa
- Si falla evaluación de monitoreo: error capturado, loguea y continúa
- Si falla upsert: error capturado, loguea y continúa
- Si falla sincronización de escenas: error capturado, loguea y continúa

**Nivel de Ciclo (RunOnce):**
- Si falla ListActiveProducciones: retorna ProcessingError tipo ErrDynamoDB
- Ciclos fallidos se reintentan automáticamente en siguiente intervalo

**Nivel de Loop (RunLoop):**
- Si RunOnce() retorna error: loguea "sync cycle failed" pero continúa
- Próximo ciclo se ejecuta al siguiente intervalo
- No hay retry exponencial automático (reintentos vía intervalo regular)

### Códigos de Motivo de Monitoreo

| Motivo | Causa | Acción Requerida |
|--------|-------|------------------|
| `OK` | Producción está siendo monitoreada | — |
| `SIN_FECHA_PLANTACION` | FechaPlantacion es NULL en DynamoDB | Ingresar en ERP |
| `FIN_MONITOREO_ALCANZADO` | Ciclo productivo completado | Esperado fin ciclo |
| `SIN_POLIGONO` | No existe BBox en tabla asignaciones | Ingresar polígono en SIG |

### Tipos de Errores (domain.ProcessingError)

- `ErrDynamoDB`: Falla en lectura de DynamoDB (tabla no existe, credenciales inválidas)
- `ErrMySQL`: Falla en operación MySQL (conexión perdida, constraint violation)
- `ErrProcessing`: Error de lógica de procesamiento

---

## Campos que se Sincronizan

### De DynamoDB a MySQL

#### Tabla: s3_monitoring_producciones

| Campo DynamoDB | Campo MySQL | Transformación | Notas |
|---|---|---|---|
| `produccion_id` | `produccion_id` | Directo | PK |
| `cultivo` | `cultivo` | Directo | Descriptivo |
| `ciclo` | `ciclo` | Directo | Formato: YYYY-[A\|B] |
| `fecha_plantacion` | `fecha_plantacion` | String → DATE | Para cálculos |
| `dias_produccion` | `dias_produccion` | Directo | Ciclo productivo |
| `articulo_id` | `articulo_id` | Directo | Phase 3: desnormalizado |
| `centro_costo_id` | `centro_costo_id` | Directo | Phase 3: desnormalizado |
| `nombre_rancho` | `nombre_rancho` | Directo | Phase 3: desnormalizado |
| — | `monitoring` | Calculado | evaluateMonitoring() |
| — | `monitoring_motivo` | Calculado | OK \| SIN_FECHA_PLANTACION \| FIN_MONITOREO_ALCANZADO \| SIN_POLIGONO |
| — | `bbox` | Calculado | De asignaciones_zonas_producciones (WKT) |
| — | `fecha_fin_monitoreo` | Calculado | FechaPlantacion + DiasProduccion + DiasMargen |
| — | `last_sync_at` | Timestamp | NOW() al upsert |

#### Tabla: s3_monitoring_escenas

| Campo DynamoDB | Campo MySQL | Transformación | Notas |
|---|---|---|---|
| `scene_id` | `scene_id` | Directo | STAC ID |
| `produccion_id` | `produccion_id` | Directo | FK |
| `date` | `scene_date` | String → DATE | Ordenamiento |
| `cloud_cover` | `cloud_cover_scene` | Directo | Filtrado por ConfigSceneMax |
| — | `status` | Inicial: PENDING | Worker actualiza |
| — | `created_at` | Timestamp | NOW() en insert |

### Campos NO Sobrescritos por DynamoDB

Estos campos son controlados localmente y no se actualizan desde DynamoDB:

- `bloqueado` - Decisión administrativa
- `bloqueado_motivo` - Por qué fue bloqueado
- `bloqueado_at` - Cuándo fue bloqueado
- `desbloqueado_por` - Quién desbloqueó
- `target_resolution` - Configurable por usuario (10, 20, 60m)
- `cloud_cover_max` - Configurable por usuario (defecto 23%)
- `total_escenas` - Actualizado solo por worker
- `total_escenas_validas` - Actualizado solo por worker

---

## Conflictos Potenciales y Resolución

### Conflicto 1: Producción Bloqueada en MySQL pero Activa en DynamoDB

**Escenario:** Admin marca producción como `bloqueado=true` para investigación. DynamoDB sigue con `activa=true`.

**Resolución:**
```go
if existing != nil && existing.Bloqueado {
    s.logger.Info("skipping blocked produccion", "produccion_id", dp.ProduccionID)
    return nil  // Saltea sincronización, respeta bloqueo
}
```

**Comportamiento:** La producción NO se actualiza desde DynamoDB hasta que sea desbloqueada.

### Conflicto 2: Escena Duplicada en DynamoDB

**Escenario:** Sentinel Hub envía misma escena dos veces con mismo scene_id.

**Resolución:**
```sql
UNIQUE KEY uq_produccion_scene (produccion_id, scene_id)
```

```go
existing, _ := s.sceneRepo.GetByProduccionAndSceneID(ctx, produccionID, de.SceneID)
if existing != nil {
    continue  // Detecta duplicado, no inserta
}
```

**Comportamiento:** Segunda inserción es silenciosa, idempotente.

### Conflicto 3: Campo Modificado en DynamoDB Después de Sincronización

**Escenario:** Cultivo cambió en DynamoDB mientras worker procesa.

**Resolución:** DynamoDB es source of truth. Siguiente ciclo de sincronización (cada 15 min) actualiza el campo.

**Comportamiento:** Worker usa versión en MySQL del ciclo actual. Cambio se refleja en próximo ciclo.

### Conflicto 4: Producción sin Polígono Registra SIN_POLIGONO

**Escenario:** Producción no tiene polígono en tabla asignaciones_zonas_producciones.

**Resolución:**
```go
bbox, err := s.polygonRepo.GetPolygonBBox(ctx, prod.ProduccionID)
if err != nil {
    s.logger.Error("getting polygon bbox failed", ...)
    return false, MotivoSinPoligono
}
if bbox == nil {
    return false, MotivoSinPoligono
}
```

**Comportamiento:** `monitoring=false`, `monitoring_motivo="SIN_POLIGONO"`. Admin debe ingresar polígono en SIG.

---

## Ejemplos de Uso

### Inicializar Servicio

```go
logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

svc := sync.New(
    dynamoClient,
    prodRepo,
    sceneRepo,
    polygonRepo,
    syncConfig,
    sentinelConfig,
    "monitoring_producciones",
    "monitoring_escenas",
    logger,
)
```

### Ejecutar Ciclo Único

```go
ctx := context.Background()
if err := svc.RunOnce(ctx); err != nil {
    log.Fatal("Sync failed:", err)
}
```

### Ejecutar Loop Continuo

```go
ctx := context.Background()
err := svc.RunLoop(ctx)  // Bloqueante hasta que ctx.Done()
if err != nil && err != context.Canceled {
    log.Fatal("Loop error:", err)
}
```

### En main.go del Worker

```go
syncService := sync.New(
    awsClient,
    prodRepo,
    sceneRepo,
    polygonRepo,
    cfg.Sync,
    cfg.Sentinel,
    cfg.DynamoDB.TableProducciones,
    cfg.DynamoDB.TableEscenas,
    logger,
)

go func() {
    if err := syncService.RunLoop(ctx); err != nil && err != context.Canceled {
        logger.Error("sync service failed", "error", err)
    }
}()
```

---

## Troubleshooting

### Problema: Sync no ejecuta

**Síntomas:** Producciones en DynamoDB pero no aparecen en MySQL.

**Diagnóstico:**
1. ¿Logs muestran "sync cycle failed"?
2. ¿Config `interval_minutes` > 0?
3. ¿Credenciales AWS válidas?

```bash
docker logs agro-sentinel-worker | grep sync
```

**Solución:**
- Verificar credenciales AWS en env vars
- Verificar tabla DynamoDB existe: `aws dynamodb describe-table --table-name monitoring_producciones`
- Verificar MySQL accesible desde container

### Problema: Escenas no se sincronizan

**Síntomas:** Producciones aparecen en MySQL pero sin escenas.

**Causa Común:** Cloud cover demasiado alto.

```go
if de.CloudCover > s.sentinel.CloudCoverSceneMax {
    continue  // Rechaza escena
}
```

**Solución:**
- Verificar config `sentinel.cloud_cover_scene_max` (defecto: 70%)
- Bajarlo temporalmente para testing: `cloud_cover_scene_max: 100`
- Verificar logs: `Getting scene failed` con `cloud_cover` value

### Problema: Motivo SIN_POLIGONO en Todas Producciones

**Síntomas:** Todas las producciones tienen `monitoring=false, monitoring_motivo="SIN_POLIGONO"`.

**Causa:** Tabla `asignaciones_zonas_producciones` vacía o PolygonRepository falla.

**Solución:**
1. Verificar tabla existe: `SELECT COUNT(*) FROM asignaciones_zonas_producciones`
2. Verificar hay datos: `SELECT produccion_id FROM asignaciones_zonas_producciones LIMIT 5`
3. Verificar campo polygon tiene datos WKT válidos
4. Revisar logs de `GetPolygonBBox` para error específico

---

**Última actualización:** 2026-09-04  
**Mantenido por:** Equipo Agro Sentinel
