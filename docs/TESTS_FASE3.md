# Tests - Fase 3: Sistema Agro Sentinel

Guía completa para ejecutar los tests unitarios e integración para la Fase 3 del Sistema de Monitoreo Agro Sentinel.

## Phase 3 Denormalization Fields

Los siguientes campos fueron agregados en Phase 3 para optimizar consultas y búsquedas sin joins:

### Campos Denormalizados en `s3_monitoring_producciones`

| Campo | Tipo | Fuente | Propósito | Tests |
|-------|------|--------|----------|-------|
| `articulo_id` | Number | DynamoDB | Identificar producto agrícola | `TestProductionRepo_GetByArticuloID` |
| `centro_costo_id` | Number | DynamoDB | Asignar costos al centro responsable | `TestProductionRepo_GetByCentroCostoID` |
| `nombre_rancho` | String | DynamoDB | Búsquedas rápidas sin joins | `TestProductionRepo_UpsertWithNewFields` |

**Sincronización**: Estos campos se sincronizan desde DynamoDB cada 15 minutos (ver `DYNAMODB_STRUCTURE.md` §4.2)

**Validación en Tests**: Se valida que:
- Los campos se inserten correctamente durante upsert
- Se puedan consultar producciones por `articulo_id`
- Se puedan consultar producciones por `centro_costo_id`
- Se actualicen correctamente cuando cambian valores
- No causen duplicados o conflictos

---

## Contenido de Tests

### 1. Tests para ProductionRepo
**Archivo**: `internal/infrastructure/database/production_repo_test.go`

Tests implementados:
- `TestProductionRepo_UpsertAndGet`: Prueba crear y recuperar producción
- `TestProductionRepo_UpsertNoDuplicate`: Verificar comportamiento de actualización (upsert)
- `TestProductionRepo_SetBloqueadoAndDesbloquear`: Test de bloqueo/desbloqueo
- `TestProductionRepo_ListActive`: Listar producciones activas
- `TestProductionRepo_IncrementEscenas`: Incrementar contadores de escenas
- `TestProductionRepo_UpsertWithNewFields`: **Upsert con campos Phase 3 denormalizados**: ArticuloID, CentroCostoID, NombreRancho
- `TestProductionRepo_GetByArticuloID`: Buscar producciones por ArticuloID (nuevo Phase 3)
- `TestProductionRepo_GetByArticuloID_Empty`: Verificar búsqueda vacía por ArticuloID
- `TestProductionRepo_GetByCentroCostoID`: Buscar producciones por CentroCostoID (nuevo Phase 3)
- `TestProductionRepo_GetByCentroCostoID_Empty`: Verificar búsqueda vacía por CentroCostoID
- `TestProductionRepo_UpdateArticuloAndCentro`: **Actualizar campos denormalizados**: ArticuloID, CentroCostoID y NombreRancho

### 2. Tests para IAResultRepository
**Archivo**: `internal/infrastructure/database/ia_result_repo_test.go`

Tests implementados:
- `TestIAResultRepository_UpsertAndGet`: Crear y recuperar resultado IA
- `TestIAResultRepository_UpsertUpdate`: Verificar actualización de resultado IA
- `TestIAResultRepository_GetByEscenaID_NotFound`: Búsqueda de resultado inexistente
- `TestIAResultRepository_ListByProduccion`: Listar resultados IA por producción
- `TestIAResultRepository_ListByProduccion_Empty`: Verificar lista vacía
- `TestIAResultRepository_DeleteByEscenaID`: Eliminar resultado IA
- `TestIAResultRepository_DeleteByEscenaID_NotFound`: Intentar eliminar resultado inexistente

### 3. Tests para Structs de Dominio
**Archivo**: `internal/domain/production_test.go`
- `TestBBoxValidate`: Validación de bounding box
- `TestProductionShouldMonitor`: Lógica de procesamiento de producción
- `TestProductionValidate`: Validación del struct Production con nuevos campos

**Archivo**: `internal/domain/analysis_test.go`
- `TestIAResultSummaryValidate`: Validación de IAResultSummary
- `TestIAResultSummaryString`: Método String() de IAResultSummary

### 4. Tests de Integración
**Archivo**: `internal/infrastructure/database/integration_test.go`

Tests implementados:

#### TestIntegration_ProductionSyncCycle
Ciclo completo de sincronización desde DynamoDB:
- Crear producción (simulando sync desde DynamoDB)
- **Validar campos denormalizados**: ArticuloID, CentroCostoID, NombreRancho
- Recuperar y verificar todos los campos
- Buscar por ArticuloID y CentroCostoID

#### TestIntegration_SceneProcessingAndIAAnalysis
Ciclo de procesamiento de escenas:
- Crear producción con campos denormalizados
- Crear escena (captura satelital)
- Generar resultado IA
- Recuperar y verificar resultado
- Listar resultados por producción
- Incrementar contadores

#### TestIntegration_CompleteMonitoringWorkflow
Flujo completo con múltiples escenas (valida Phase 3):
- Crear producción con metadata **incluyendo ArticuloID, CentroCostoID, NombreRancho**
- Procesar 5 escenas con IA análisis
- Verificar contadores
- **Actualizar campos denormalizados**: ArticuloID, CentroCostoID, NombreRancho
- **Buscar por criterios actualizados** (ArticuloID, CentroCostoID)
- Eliminar resultado IA
- Verificar cambios en listado

#### TestIntegration_DataValidation
Pruebas de validación de datos:
- Validar que ArticuloID, CentroCostoID, NombreRancho son replicados correctamente
- Verificar constraints de Foreign Key
- Detectar valores NULL donde no deberían existir

## Requisitos

### Base de Datos
Se requiere una instancia MySQL disponible. Los tests usan la variable de entorno `MYSQL_TEST_DSN`.

Ejemplo de DSN:
```
root:password@tcp(localhost:3306)/sentinel_test?parseTime=true
```

### Configuración del Entorno
```bash
# Windows (PowerShell)
$env:MYSQL_TEST_DSN = "root:password@tcp(localhost:3306)/sentinel_test?parseTime=true"

# Linux/Mac (Bash)
export MYSQL_TEST_DSN="root:password@tcp(localhost:3306)/sentinel_test?parseTime=true"
```

### Dependencies
Los siguientes paquetes deben estar instalados:
```bash
go get github.com/go-sql-driver/mysql
```

## Ejecución de Tests

### Ejecutar todos los tests
```bash
go test ./...
```

### Ejecutar tests de un paquete específico
```bash
# Tests del repositorio de producción
go test ./internal/infrastructure/database -run TestProductionRepo

# Tests del repositorio IA result
go test ./internal/infrastructure/database -run TestIAResultRepository

# Tests de structs de dominio
go test ./internal/domain

# Tests de integración
go test ./internal/infrastructure/database -run TestIntegration
```

### Ejecutar un test específico
```bash
go test ./internal/infrastructure/database -run TestProductionRepo_UpsertAndGet
```

### Ejecutar tests con verbose
```bash
go test -v ./...
```

### Ejecutar tests con coverage
```bash
go test -cover ./...

# Generar reporte HTML de coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Ejecutar tests con race detector
```bash
go test -race ./...
```

### Ejecutar tests sin paralelización
```bash
go test -p 1 ./...
```

## Configuración de Base de Datos para Tests

Se pueden usar varias opciones para la base de datos de tests:

### Opción 1: MySQL Local
```bash
# Crear base de datos de tests
mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS sentinel_test;"

# Exportar DSN
export MYSQL_TEST_DSN="root:password@tcp(localhost:3306)/sentinel_test?parseTime=true"
```

### Opción 2: Docker
```bash
# Ejecutar MySQL en Docker
docker run --name mysql-test \
  -e MYSQL_ROOT_PASSWORD=password \
  -e MYSQL_DATABASE=sentinel_test \
  -p 3306:3306 \
  -d mysql:8.0

# Exportar DSN
export MYSQL_TEST_DSN="root:password@tcp(localhost:3306)/sentinel_test?parseTime=true"
```

### Opción 3: Docker Compose
El proyecto incluye `docker-compose.yml` que configura todo automáticamente:
```bash
docker-compose up -d mysql

# Los tests usarán la configuración del compose
export MYSQL_TEST_DSN="root:root@tcp(localhost:3306)/sentinel_test?parseTime=true"
```

## Resultados Esperados

Todos los tests deberían pasar:
- ✅ 11 tests de ProductionRepo
- ✅ 7 tests de IAResultRepository
- ✅ 7 tests de structs de dominio
- ✅ 4 tests de integración

Total: **29 tests**

## Troubleshooting

### Error: "MYSQL_TEST_DSN not set"
Los tests se saltan si no está configurada la variable de entorno. Asegúrate de que:
```bash
# Verificar que la variable está configurada
echo $MYSQL_TEST_DSN  # Linux/Mac
echo %MYSQL_TEST_DSN%  # Windows (CMD)
$env:MYSQL_TEST_DSN   # Windows (PowerShell)
```

### Error de conexión a MySQL
Verifica que:
1. MySQL está corriendo: `mysql -u root -p -e "SELECT 1"`
2. La base de datos existe: `mysql -u root -p -e "SHOW DATABASES LIKE 'sentinel_test'"`
3. El DSN es correcto (usuario, contraseña, host, puerto, BD)

### Error: "table doesn't exist"
Los tests ejecutan migraciones automáticamente. Si fallan:
1. Verifica que la función `RunMigrations()` está disponible
2. Asegúrate que el usuario MySQL tiene permisos CREATE TABLE
3. Limpia la base de datos: `mysql -u root -p -e "DROP DATABASE sentinel_test; CREATE DATABASE sentinel_test;"`

## Cobertura de Tests

Los tests cubren:

**ProductionRepo**:
- ✅ Upsert con 3 campos nuevos (ArticuloID, CentroCostoID, NombreRancho)
- ✅ GetByProduccionID
- ✅ GetByArticuloID (nuevo)
- ✅ GetByCentroCostoID (nuevo)
- ✅ UpdateArticuloAndCentro (nuevo)
- ✅ Búsquedas vacías
- ✅ Actualizaciones de estado (bloqueo, monitoreo)
- ✅ Contadores de escenas

**IAResultRepository**:
- ✅ Upsert (insert y update)
- ✅ GetByEscenaID
- ✅ ListByProduccion
- ✅ DeleteByEscenaID
- ✅ Manejo de casos no encontrados
- ✅ Manejo de listas vacías

**Structs**:
- ✅ Production.Validate() con nuevos campos
- ✅ IAResultSummary.Validate()
- ✅ IAResultSummary.String()
- ✅ BBox.Validate()

**Integración**:
- ✅ Ciclo de sincronización DynamoDB
- ✅ Procesamiento de escenas y análisis IA
- ✅ Flujo completo con múltiples escenas
- ✅ Validación de datos en workflow

## Notas

1. Los tests usan `randomID()` para generar IDs únicos, evitando conflictos
2. Los tests usan `testDB(t)` que automatiza:
   - Lectura de DSN desde env var
   - Conexión a MySQL
   - Ejecución de migraciones
   - Cleanup (cierre de conexión)
3. Los tests de integración demuestran flujos reales del sistema
4. Se usa `context.Background()` para todos los contextos (sincronía)

## Próximos Pasos

Después de validar todos los tests, considera:
1. Agregar tests para concurrencia (race detector)
2. Agregar tests de performance
3. Crear fixtures de datos de prueba
4. Documentar casos de error esperados
