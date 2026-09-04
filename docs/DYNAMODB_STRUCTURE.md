# DynamoDB Structure - Agro Sentinel Worker

## Descripción General

DynamoDB es la fuente de datos primaria para el sistema Agro Sentinel. Contiene información sobre producciones activas y escenas de satélite (STAC items). El worker sincroniza periódicamente estos datos a MySQL para procesamiento local y análisis.

**Flujo de datos:** DynamoDB (source) → Sync Service → MySQL (local cache) → Worker Processing → S3

---

## 1. Esquema de Tablas DynamoDB

### 1.1 Tabla: `monitoring_producciones`

Almacena metadatos de producciones agrícolas que están siendo monitoreadas.

#### Claves

| Propiedad | Tipo | Descripción |
|-----------|------|-------------|
| **Partition Key** | `produccion_id` | String (número de producción único) |
| **Sort Key** | — | No hay sort key (tabla simple) |

#### Índices Secundarios

- **GSI por `activa`**: Permite filtrar rápidamente producciones activas (valor booleano)
  - Consultas típicas: `activa = true`
  - Importante para el scan inicial en la sincronización

#### TTL

- **No configurado**: Los datos deben mantenerse indefinidamente (no son datos temporales)

---

### 1.2 Tabla: `monitoring_escenas`

Almacena información de escenas de satélite (STAC items) asociadas a producciones.

#### Claves

| Propiedad | Tipo | Descripción |
|-----------|------|-------------|
| **Partition Key** | `scene_id` | String (identificador único de escena) |
| **Sort Key** | — | No hay sort key |

#### Índices Secundarios

- **GSI por `produccion_id`**: Permite recuperar todas las escenas de una producción
  - Consultas típicas: Filtrar por `produccion_id` después de scan

#### TTL

- **No configurado**: Las escenas se mantienen como histórico del monitoreo

---

## 2. Estructura de Documentos

### 2.1 Documento: Production (monitoring_producciones)

Representa una producción agrícola activa en el sistema de monitoreo.

#### Ejemplo JSON Realista

```json
{
  "produccion_id": 12345,
  "activa": true,
  "cultivo": "maiz",
  "ciclo": "2026-A",
  "fecha_plantacion": "2026-01-15",
  "dias_produccion": 120,
  "articulo_id": 5001,
  "centro_costo_id": 101,
  "nombre_rancho": "Rancho El Remanso"
}
```

#### Esquema de Campos

| Campo | Tipo | Nullable | Descripción | Ejemplo |
|-------|------|----------|-------------|---------|
| `produccion_id` | Number | No | Identificador único de la producción (PK) | `12345` |
| `activa` | Boolean | No | Indica si la producción está activa en monitoreo | `true` |
| `cultivo` | String | No | Nombre del cultivo siendo monitoreado | `"maiz"`, `"soja"`, `"trigo"` |
| `ciclo` | String | No | Ciclo agrícola (ej: año-temporada) | `"2026-A"`, `"2026-B"` |
| `fecha_plantacion` | String | Sí | Fecha de plantación en formato ISO 8601 | `"2026-01-15"` |
| `dias_produccion` | Number | No | Días esperados de ciclo productivo | `90`, `120`, `150` |
| `articulo_id` | Number | No | FK a tabla articulos - Producto agrícola | `5001`, `5002` |
| `centro_costo_id` | Number | No | FK a tabla centros_costos - Centro para contabilidad | `101`, `102` |
| `nombre_rancho` | String | Sí | Nombre denormalizado del rancho (búsquedas rápidas) | `"Rancho El Remanso"` |

#### Convenciones de Datos

- **Booleanos**: Almacenados como `true`/`false` (no como 0/1)
- **Fechas**: Formato `YYYY-MM-DD` (ISO 8601)
- **Nombres**: Sin caracteres especiales, se limpian en origen
- **Valores NULL**: Campos opcionales como `nombre_rancho` pueden no estar presentes

#### Valores Permitidos

- **cultivo**: Lista abierta controlada por ERP (ej: maiz, soja, trigo, algodón, etc.)
- **ciclo**: Formato `YYYY-[A|B]` donde A es ciclo primavera, B es ciclo otoño
- **dias_produccion**: Entre 60 y 200 días típicamente
- **centro_costo_id**: Debe existir en tabla `centros_costos` de MySQL

---

### 2.2 Documento: Scene (monitoring_escenas)

Representa una escena de satélite Sentinel-2 asociada a una producción.

#### Ejemplo JSON Realista

```json
{
  "scene_id": "S2A_MSIL2A_20260205T135051_N0510_R024_T19HCC_20260205T135101",
  "produccion_id": 12345,
  "date": "2026-02-05",
  "cloud_cover": 12.5,
  "stac_assets": {
    "B04": {
      "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B04.jp2",
      "resolution": 10
    },
    "B08": {
      "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B08.jp2",
      "resolution": 10
    },
    "B11": {
      "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B11.jp2",
      "resolution": 20
    }
  }
}
```

#### Esquema de Campos

| Campo | Tipo | Nullable | Descripción | Ejemplo |
|-------|------|----------|-------------|---------|
| `scene_id` | String | No | Identificador único STAC de escena (PK) | `"S2A_MSIL2A_20260205T..."` |
| `produccion_id` | Number | No | FK a tabla producciones | `12345` |
| `date` | String | No | Fecha de captura en formato `YYYY-MM-DD` | `"2026-02-05"` |
| `cloud_cover` | Number | No | Porcentaje de cobertura de nubes (0-100) | `12.5`, `45.0`, `0.0` |
| `stac_assets` | Map | Sí | Mapa de bandas espectrales disponibles en STAC | Ver tabla abajo |

#### Subestructura: STAC Assets

Cada asset es una banda espectral disponible en la escena.

```typescript
{
  [bandName: string]: {
    href: string      // URL de descarga del archivo GeoTIFF
    resolution: number // Resolución espacial en metros (10, 20, 60)
  }
}
```

#### Bandas Típicas Sentinel-2

| Banda | Nombre | Resolución | Rango Espectral | Uso |
|-------|--------|------------|-----------------|-----|
| B02 | Blue | 10m | Visible (490nm) | RGB, detección de agua |
| B03 | Green | 10m | Visible (560nm) | RGB |
| B04 | Red | 10m | Visible (665nm) | RGB, NDVI |
| B08 | NIR | 10m | Infrarrojo cercano (842nm) | NDVI, salud de vegetación |
| B11 | SWIR | 20m | Infrarrojo de onda corta (1610nm) | NDVI mejorado, humedad |
| B05-B07 | Narrow Red Edge | 20m | Red-edge (705-783nm) | Detección de estrés |
| B01 | Coastal | 60m | Visible-UV (443nm) | Aerosoles |

#### Convenciones de Datos

- **scene_id**: Formato STAC estándar, incluye timestamp de captura
- **date**: Siempre coincide con la fecha en el scene_id
- **cloud_cover**: Porcentaje de cobertura de nubes en toda la escena (calculado por Sentinel Hub)
- **stac_assets**: Puede contener bandas parciales si hay problemas de disponibilidad
- **resolution**: Siempre 10, 20 o 60 metros (especificación Sentinel-2)

#### Valores Permitidos

- **cloud_cover**: `0.0` a `100.0` (decimal con máximo 1 decimal)
- **resolution**: `[10, 20, 60]` únicamente
- **date**: Formato `YYYY-MM-DD`, sin timestamp

---

## 3. Patrones de Acceso

### 3.1 Búsqueda por `produccion_id`

**Caso de Uso**: Recuperar todas las escenas de una producción específica

**Operación**: Scan con FilterExpression

```go
// DynamoDB Query
{
  "TableName": "monitoring_escenas",
  "FilterExpression": "produccion_id = :pid",
  "ExpressionAttributeValues": {
    ":pid": 12345
  }
}
```

**Implementación en Code**:
```go
filt := expression.Name("produccion_id").Equal(expression.Value(produccionID))
expr, _ := expression.NewBuilder().WithFilter(filt).Build()
paginator := dynamodb.NewScanPaginator(client, &dynamodb.ScanInput{
  TableName: &tableName,
  FilterExpression: expr.Filter(),
  // ...
})
```

**Performance**: O(N) - Escanea toda la tabla pero retorna rápidamente si pocas escenas

---

### 3.2 Búsqueda por `activa = true`

**Caso de Uso**: Listar todas las producciones activas (para sincronización)

**Operación**: Scan con FilterExpression

```go
// DynamoDB Query
{
  "TableName": "monitoring_producciones",
  "FilterExpression": "activa = :active",
  "ExpressionAttributeValues": {
    ":active": true
  }
}
```

**Implementación en Code**:
```go
filt := expression.Name("activa").Equal(expression.Value(true))
expr, _ := expression.NewBuilder().WithFilter(filt).Build()
paginator := dynamodb.NewScanPaginator(client, &dynamodb.ScanInput{
  TableName: &tableName,
  FilterExpression: expr.Filter(),
  // ...
})
```

**Performance**: O(N) - Escanea tabla completa, ideal para sincronización periódica

---

### 3.3 Búsqueda por Rango de Fechas

**Caso de Uso**: Encontrar escenas entre dos fechas (actualmente no implementado)

**Operación Potencial**: Scan con FilterExpression de fecha

```go
// Conceptual - no está implementado hoy
filt := expression.Name("date").Between(
  expression.Value("2026-02-01"), 
  expression.Value("2026-02-28"),
)
```

**Recomendación**: Implementar GSI por fecha si esta consulta es frecuente

---

### 3.4 Proyecciones de Campos

**Caso de Uso**: Recuperar solo ciertos campos para optimizar ancho de banda

**Operación**: Scan con ProjectionExpression

```go
// Leer solo produccion_id y cultivo (más rápido)
input := &dynamodb.ScanInput{
  TableName: &tableName,
  ProjectionExpression: aws.String("produccion_id, cultivo"),
  // ...
}
```

**Beneficio**: Reduce consumo de RCUs (Read Capacity Units) si se procesan muchos items

---

## 4. Sincronización MySQL ↔ DynamoDB

### 4.1 Flujo de Sincronización

```
┌─────────────────────────────────────────────────────────────┐
│                   DynamoDB (AWS)                            │
│                                                              │
│  monitoring_producciones    monitoring_escenas              │
│  (STAC source of truth)     (Scene metadata)                │
└────────────────────┬────────────────────┬───────────────────┘
                     │                    │
                     └────────┬───────────┘
                              │
                      Sync Service
                   (every 15 minutes)
                              │
        ┌─────────────────────┴──────────────────────┐
        │                                            │
        ▼                                            ▼
┌──────────────────────────────┐    ┌──────────────────────────────┐
│   MySQL Local Cache          │    │  Processing Decisions        │
│                              │    │                              │
│ s3_monitoring_producciones   │◄───┤ • Monitoring eligibility     │
│ • Full production metadata   │    │ • Cloud cover filters        │
│ • BBox & monitoring state    │    │ • Processing history         │
│                              │    │                              │
│ s3_monitoring_escenas        │    │                              │
│ • Scene metadata             │    │                              │
│ • Job status (PENDING, etc)  │    │                              │
│ • Error tracking             │    │                              │
└──────────────────────────────┘    └──────────────────────────────┘
        │
        │ Worker reads
        │
        ▼
    S3 Downloads
    GDAL Processing
    IA Analysis
```

### 4.2 Campos que se Replican

#### De `monitoring_producciones` (DynamoDB) → `s3_monitoring_producciones` (MySQL)

| DynamoDB | MySQL | Transformación | Notas |
|----------|-------|----------------|-------|
| `produccion_id` | `produccion_id` | Directo | Clave primaria |
| `activa` | `monitoring` | Directo (bool → tinyint) | Filtro inicial |
| `cultivo` | `cultivo` | Directo | Informativo |
| `ciclo` | `ciclo` | Directo | Informativo |
| `fecha_plantacion` | `fecha_plantacion` | String → DATE | Cálculos de fin monitoreo |
| `dias_produccion` | `dias_produccion` | Directo | Para cálculos |
| `articulo_id` | `articulo_id` | Directo | Desnormalizado desde Phase 3 |
| `centro_costo_id` | `centro_costo_id` | Directo | Desnormalizado desde Phase 3 |
| `nombre_rancho` | `nombre_rancho` | Directo | Desnormalizado desde Phase 3 |
| — | `monitoring_motivo` | Calculado | Ver sección 4.3 |
| — | `bloqueado` | Inicial: false | Administrador puede cambiar |
| — | `target_resolution` | Default: 10 | Configurable por usuario |
| — | `cloud_cover_max` | Default: 23.00 | Configurable por usuario |

#### De `monitoring_escenas` (DynamoDB) → `s3_monitoring_escenas` (MySQL)

| DynamoDB | MySQL | Transformación | Notas |
|----------|-------|----------------|-------|
| `scene_id` | `scene_id` | Directo | Clave única con produccion_id |
| `produccion_id` | `produccion_id` | Directo | FK |
| `date` | `scene_date` | String → DATE | Filtros por rango |
| `cloud_cover` | `cloud_cover_scene` | Directo | Filtro de calidad |
| — | `status` | Inicial: PENDING | Worker actualiza |
| — | `passes_quality` | Calculado | Worker verifica |
| — | `has_multiband` | Calculado | Worker detecta bandas |
| — | `has_rgb` | Calculado | Worker verifica B02,B03,B04 |
| — | `retry_count` | Inicial: 0 | Worker incrementa |

### 4.3 Lógica de Decisión: Monitoring

La sincronización evalúa si una producción debe ser monitoreada y asigna un `monitoring_motivo`:

```go
func evaluateMonitoring(prod *Production) (bool, string) {
  // 1. Validar fecha de plantación
  if prod.FechaPlantacion == nil {
    return false, "SIN_FECHA_PLANTACION"
  }

  // 2. Calcular fin de monitoreo
  fin := FechaPlantacion + DiasProduccion + DiasMargen
  prod.FechaFinMonitoreo = fin

  // 3. Validar si aún está en período de monitoreo
  if now() > fin {
    return false, "FIN_MONITOREO_ALCANZADO"
  }

  // 4. Validar existencia de polígono (BBox)
  bbox := obtenerBBoxDelPoligono(prod.ProduccionID)
  if bbox == nil {
    return false, "SIN_POLIGONO"
  }
  prod.BBox = bbox

  // 5. Aprobado para monitoreo
  return true, "OK"
}
```

#### Motivos de `monitoring_motivo`

| Motivo | Significado | Acción Requerida |
|--------|------------|-----------------|
| `OK` | Producción está siendo monitoreada | — |
| `SIN_FECHA_PLANTACION` | Falta fecha de plantación en DynamoDB | Ingresar en ERP |
| `FIN_MONITOREO_ALCANZADO` | Ciclo productivo completado | Esperado al final del ciclo |
| `SIN_POLIGONO` | No existe polígono en tabla de asignaciones | Ingresar polígono en SIG |

### 4.4 Conflictos Potenciales

#### Problema: Production modificada en DynamoDB después de sincronización

**Escenario**: Cambio de `cultivo` en DynamoDB mientras worker procesa

**Solución**: 
- La siguiente sincronización (en 15 min) actualiza el campo
- Worker usa versión en MySQL, cambio se refleja en siguiente ciclo
- No hay race condition: worker lee de MySQL, DynamoDB es source

#### Problema: Escena duplicada en DynamoDB

**Escenario**: Sentinel Hub envía misma escena con mismos datos

**Solución**:
```sql
UNIQUE KEY uq_produccion_scene (produccion_id, scene_id)
```
- `GetByProduccionAndSceneID` detecta duplicado
- No se inserta dos veces
- Log registra intento

#### Problema: Producción bloqueada (BloqueadoMotivo) pero sigue en DynamoDB con `activa=true`

**Escenario**: Admin bloquea para investigación, pero DynamoDB no se actualiza

**Solución**:
- Sincronización respeta flag `bloqueado` en MySQL (no lo sobrescribe)
- Si `bloqueado=true`, saltea sincronización de esa producción
- Admin puede desbloquear manualmente

```go
if existing != nil && existing.Bloqueado {
  logger.Info("skipping blocked produccion", "produccion_id", dp.ProduccionID)
  return nil
}
```

### 4.5 Reconciliación

#### Qué Sucede en Cada Ciclo de Sincronización

1. **Lectura de DynamoDB**: Se listan todas las producciones con `activa=true`
2. **Lectura de MySQL**: Se obtiene estado local existente (si existe)
3. **Evaluación**: Se calcula si debe monitorearse
4. **Upsert**: Se inserta o actualiza en MySQL
5. **Sincronización de Escenas**: Si `monitoring=true`, se sincronizan escenas

#### Campos NO Sobrescritos por DynamoDB

- `bloqueado` - Controlado por admin
- `bloqueado_motivo` - Controlado por admin
- `bloqueado_at` - Controlado por admin
- `desbloqueado_por` - Controlado por admin
- `target_resolution` - Configurable por usuario
- `cloud_cover_max` - Configurable por usuario
- `total_escenas` - Actualizado solo por worker
- `total_escenas_validas` - Actualizado solo por worker

#### Sincronización de Escenas

```go
func syncEscenas(produccionID int64) {
  // Obtener escenas de DynamoDB
  escenas := dynamo.ListEscenas(tableName, produccionID)
  
  for _, de := range escenas {
    // Filtrar por cloud_cover
    if de.CloudCover > config.CloudCoverSceneMax {
      continue  // Rechazar escena muy nublada
    }
    
    // Detectar duplicados
    existing := mysql.GetByProduccionAndSceneID(produccionID, de.SceneID)
    if existing != nil {
      continue  // Escena ya existe
    }
    
    // Insertar nueva
    scene := buildScene(de)
    mysql.Upsert(scene)
  }
}
```

---

## 5. Best Practices

### 5.1 Convención de Nombres

#### Tabla DynamoDB

```
monitoring_<entity>

monitoring_producciones
monitoring_escenas
```

#### Atributos

- **snake_case**: `produccion_id`, `cloud_cover`, `nombre_rancho`
- **Booleanos**: `activa`, `bloqueado`, `passes_quality` (con verbo)
- **Fechas**: `fecha_plantacion`, `date` (con prefijo `fecha_` o campo simple)
- **Identificadores**: `produccion_id`, `scene_id`, `articulo_id`
- **Contadores**: `dias_produccion`, `total_escenas`, `retry_count`

#### Tabla MySQL

```
s3_monitoring_<entity>

s3_monitoring_producciones
s3_monitoring_escenas
s3_monitoring_escena_archivos
```

---

### 5.2 Manejo de NULL/Vacíos

#### En DynamoDB

DynamoDB no almacena atributos con valor NULL. Estrategia:

**Campos Obligatorios** (siempre presentes):
- `produccion_id`, `scene_id`, `activa`, `cultivo`, `cloud_cover`

**Campos Opcionales** (omitir si vacío):
- `nombre_rancho` - Omitir si no disponible
- `fecha_plantacion` - Omitir si desconocida
- `stac_assets` - Puede ser vacío `{}`

```go
// En Go: usando punteros para NULL
type DynamoProduction struct {
  ProduccionID    int64   `dynamodbav:"produccion_id"`     // Obligatorio
  NombreRancho    string  `dynamodbav:"nombre_rancho"`     // Omitido si vacío
  FechaPlantacion string  `dynamodbav:"fecha_plantacion"`  // Omitido si vacío
}
```

#### En MySQL

Usar campos NULL permitidos:

```sql
nombre_rancho VARCHAR(255) NULL,
fecha_plantacion DATE NULL,
cloud_cover_bbox DECIMAL(5,2) NULL
```

En Go:
```go
type Production struct {
  NombreRancho    string     // Vacío si desconocido
  FechaPlantacion *time.Time // nil si desconocido
  CloudCoverBBox  *float64   // nil si no disponible
}
```

---

### 5.3 Indexación

#### Recomendaciones para GSI en DynamoDB

**1. GSI por `activa` en `monitoring_producciones`**
- **Uso**: Filtrar producciones activas sin escanear inactivas
- **Estado**: Implementado implícitamente (Scan con FilterExpression)
- **Mejora Posible**: Crear GSI formal si scans son muy frecuentes

```yaml
GlobalSecondaryIndex:
  IndexName: activa-index
  KeySchema:
    - AttributeName: activa
      KeyType: HASH
  Projection:
    ProjectionType: ALL
  ProvisionedThroughput:
    ReadCapacityUnits: 5
    WriteCapacityUnits: 1
```

**2. GSI por `fecha` en `monitoring_escenas`** (Potencial)
- **Uso**: Consultas por rango de fechas
- **Estado**: No implementado hoy
- **Cuando Implementar**: Si necesarios reportes por rango de fechas

```yaml
GlobalSecondaryIndex:
  IndexName: produccion-date-index
  KeySchema:
    - AttributeName: produccion_id
      KeyType: HASH
    - AttributeName: date
      KeyType: RANGE
  Projection:
    ProjectionType: ALL
```

#### Índices en MySQL

```sql
-- Ya existen
UNIQUE KEY uq_produccion_id (produccion_id)
UNIQUE KEY uq_produccion_scene (produccion_id, scene_id)

-- Consideraciones de añadir
CREATE INDEX idx_escena_status ON s3_monitoring_escenas(status);
CREATE INDEX idx_escena_date ON s3_monitoring_escenas(scene_date);
```

---

### 5.4 Performance Tips

#### Capacidad Lectora en DynamoDB

**Recomendación**: Usar `BillingMode: PAY_PER_REQUEST`

```go
BillingMode: types.BillingModePayPerRequest
```

**Ventajas**:
- Pagar solo por lecturas reales (no capacidad reservada)
- Scans pueden ser caros pero son ocasionales (cada 15 min)
- Ideal para patrones variables

**Estimación**:
- Scan de 1000 producciones: ~1000 RCUs (read capacity units)
- Cada 15 minutos: 4 scans/hora = 4000 RCUs/hora
- Costo: muy bajo con pay-per-request

#### Optimizaciones de Scan

**1. Usar ProjectionExpression si no necesitas todos los campos**

```go
// Leer solo produccion_id, cultivo, activa
input.ProjectionExpression = aws.String("produccion_id, cultivo, activa")
// Reduce RCUs consumidas
```

**2. Usar FilterExpression en el servidor**

```go
// BUENO: Filtrar en DynamoDB
input.FilterExpression = expr.Filter()

// MALO: Leer todo y filtrar en Go
items, _ := client.Scan(ctx, &dynamodb.ScanInput{...})
for _, item := range items {  // Filtrando en cliente
  if item.Activa { ... }
}
```

**3. Paginación eficiente**

```go
paginator := dynamodb.NewScanPaginator(client, input)
for paginator.HasMorePages() {
  page, _ := paginator.NextPage(ctx)
  // Procesar página
}
// No cargar todo en memoria
```

#### Optimizaciones en MySQL

**Conexión Pooling**:
```go
db, _ := sql.Open("mysql", dsn)
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
```

**Transacciones para Upserts Múltiples**:
```go
tx, _ := db.BeginTx(ctx, nil)
for _, prod := range producciones {
  // Insertar en tx (más rápido)
}
tx.Commit()
```

---

### 5.5 Monitoreo y Observabilidad

#### Métricas DynamoDB Clave

- **ConsumedReadCapacityUnits**: Cuánto se lee realmente
- **ConsumedWriteCapacityUnits**: No debería haber writes en el worker
- **ScanCount**: Items examinados (puede ser mayor que retornados)
- **ItemCount**: Total de items en tabla
- **UserErrors**: Errores de aplicación
- **SystemErrors**: Problemas de AWS

#### Logs Recomendados

```go
logger.Info("sync cycle started",
  "table", tableName,
  "items_scanned", scanCount,
  "items_filtered", len(results),
)

logger.Error("dynamodb scan failed",
  "table", tableName,
  "error", err,
  "retry_count", retries,
)
```

#### CloudWatch Alarms

```yaml
DynamoDBScanLatencyAlarm:
  MetricName: ConsumedReadCapacityUnits
  Statistic: Sum
  Period: 300
  Threshold: 10000  # RCUs en 5 minutos
  ComparisonOperator: GreaterThanThreshold
  AlarmActions:
    - SNS Topic para notificación

SyncCycleFailureAlarm:
  MetricName: FailedInvocations
  Statistic: Sum
  Period: 900
  Threshold: 1
  ComparisonOperator: GreaterThanOrEqualToThreshold
```

---

### 5.6 Mitigación de Riesgos

#### Riesgo: Sincronización Fuera de Orden

**Problema**: Si sync service falla, escenas pueden no sincronizarse

**Mitigación**:
- Retry logic con exponential backoff
- Logging exhaustivo de syncronization state
- CloudWatch alarmas si sync no se ejecuta en 30 min

#### Riesgo: Datos Inconsistentes DynamoDB ↔ MySQL

**Problema**: Producción en MySQL pero no en DynamoDB

**Mitigación**:
- DynamoDB es source of truth
- Si producción no está en DynamoDB con `activa=true`, no se monitorea
- Reconciliación semanal checando orphaned records en MySQL

#### Riesgo: Cambios de Datos Perdidos

**Problema**: User cambia algo en MySQL directamente

**Mitigación**:
- Campos clave no deben ser editables en UI (marcados como read-only)
- Cambios de `cultivo`, `dias_produccion` solo desde ERP (DynamoDB source)
- Fields editables en UI: `bloqueado`, `target_resolution`, `cloud_cover_max`

#### Riesgo: Capacidad Insuficiente en DynamoDB

**Problema**: Scans se ralentizan con tabla en crecimiento

**Mitigación**:
- Con PAY_PER_REQUEST: No hay límite de capacidad
- Monitorear item count de tabla
- Si >100k items, considerar particionamiento por fecha

---

## 6. Configuración

### 6.1 Archivo config.yaml

```yaml
dynamodb:
  table_producciones: monitoring_producciones
  table_escenas: monitoring_escenas

aws:
  region: us-west-2
  endpoint: ""  # Dejar vacío para AWS, o usar http://localhost:4566 para LocalStack

sync:
  interval_minutes: 15
  dias_margen_monitoreo: 30
```

### 6.2 Variables de Entorno

```bash
# Sobreescriben config.yaml
AWS_REGION=us-west-2
AWS_ENDPOINT_URL=

# LocalStack para desarrollo
AWS_ENDPOINT_URL=http://localhost:4566
```

### 6.3 Creación de Tablas (LocalStack)

```bash
# Para desarrollo local con LocalStack

# Crear tabla de producciones
aws dynamodb create-table \
  --table-name monitoring_producciones \
  --attribute-definitions \
    AttributeName=produccion_id,AttributeType=N \
  --key-schema \
    AttributeName=produccion_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:4566

# Crear tabla de escenas
aws dynamodb create-table \
  --table-name monitoring_escenas \
  --attribute-definitions \
    AttributeName=scene_id,AttributeType=S \
  --key-schema \
    AttributeName=scene_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:4566
```

---

## 7. Ejemplos de Código

### 7.1 Leer Producciones Activas

```go
client := aws.NewDynamoDBClient(awsCfg)

producciones, err := client.ListActiveProducciones(ctx, "monitoring_producciones")
if err != nil {
  log.Fatal(err)
}

for _, prod := range producciones {
  fmt.Printf("Producción %d: %s (%s)\n", 
    prod.ProduccionID, prod.Cultivo, prod.Ciclo)
}
```

### 7.2 Leer Escenas de una Producción

```go
escenas, err := client.ListEscenas(ctx, "monitoring_escenas", 12345)
if err != nil {
  log.Fatal(err)
}

for _, escena := range escenas {
  fmt.Printf("Escena %s del %s (Cloud: %.1f%%)\n",
    escena.SceneID, escena.Date, escena.CloudCover)
  
  for bandName, asset := range escena.STACAssets {
    fmt.Printf("  %s: %dm - %s\n", 
      bandName, asset.Resolution, asset.Href)
  }
}
```

### 7.3 Insertar Datos Manualmente (Testing)

```go
item := aws.DynamoProduction{
  ProduccionID:    12345,
  Activa:          true,
  Cultivo:         "maiz",
  Ciclo:           "2026-A",
  FechaPlantacion: "2026-01-15",
  DiasProduccion:  120,
  ArticuloID:      5001,
  CentroCostoID:   101,
  NombreRancho:    "Rancho El Remanso",
}

av, _ := attributevalue.MarshalMap(item)
_, _ := client.PutItem(ctx, &dynamodb.PutItemInput{
  TableName: aws.String("monitoring_producciones"),
  Item:      av,
})
```

---

## 8. Troubleshooting

### Problema: Synchronization no ejecuta

**Síntoma**: Producciones en DynamoDB pero no aparecen en MySQL

**Diagnóstico**:
1. Verificar logs del sync service: `sync stopped` vs `sync cycle failed`
2. Verificar config: ¿`interval_minutes` es positivo?
3. Verificar credenciales AWS: `aws dynamodb scan --table-name monitoring_producciones`

**Solución**:
```bash
# Revisar logs (si es Docker)
docker logs agro-sentinel-sync

# Verificar conectividad a DynamoDB
aws dynamodb describe-table --table-name monitoring_producciones \
  --region us-west-2

# Ejecutar sync manualmente (testing)
cd cmd/sync && go run main.go
```

---

### Problema: Escenas no se sincronizan

**Síntoma**: Producciones aparecen pero escenas no

**Causa Común**: `cloud_cover` demasiado alto

**Diagnóstico**:
```go
// En sync.go, línea ~255
if de.CloudCover > s.sentinel.CloudCoverSceneMax {
  continue  // ← Escena rechazada por nubes
}
```

**Solución**:
1. Revisar config: `sentinel.cloud_cover_scene_max` (default: 70%)
2. Bajarlo temporalmente para testing: `cloud_cover_scene_max: 100`

---

### Problema: Duplicados de escenas

**Síntoma**: Escena aparece 2+ veces con mismo `scene_id`

**Diagnóstico**: Constraint `UNIQUE KEY uq_produccion_scene` debería prevenir esto

```sql
SELECT COUNT(*), produccion_id, scene_id
FROM s3_monitoring_escenas
GROUP BY produccion_id, scene_id
HAVING COUNT(*) > 1;
```

**Solución**: Limpiar manualmente si ocurre (raro)
```sql
DELETE FROM s3_monitoring_escenas
WHERE id NOT IN (
  SELECT MIN(id) FROM s3_monitoring_escenas
  GROUP BY produccion_id, scene_id
);
```

---

## 9. Referencias y Recursos

### AWS SDK Documentation

- [AWS SDK for Go v2 - DynamoDB](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/dynamodb)
- [DynamoDB Expression Builder](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression)

### Sentinel-2 Resources

- [Sentinel-2 L2A Product Specification](https://sentinel.esa.int/documents/247904/349490/S2_MSI_L2A_BOI_REPORT.pdf)
- [STAC Specification](https://stacspec.org/)
- [Sentinel Hub STAC API](https://www.sentinel-hub.com/develop/api/stac/)

### DynamoDB Best Practices

- [AWS DynamoDB Best Practices](https://docs.aws.amazon.com/amazondynamodb/latest/developerguide/best-practices.html)
- [DynamoDB Pricing](https://aws.amazon.com/dynamodb/pricing/)

---

**Última actualización**: 2026-09-04  
**Versión**: 1.0  
**Mantendido por**: Equipo Agro Sentinel
