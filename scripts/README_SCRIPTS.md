# Scripts de Deployment - Agro Sentinel Worker

Referencia de todos los scripts disponibles para deployment y operación del Sistema Agro Sentinel.

## Índice de Scripts

### Data Import Scripts

#### 0. import-dynamodb.sh & import-dynamodb.py
**Propósito:** Importar datos a DynamoDB desde JSON o CSV

```bash
# Validar datos (recomendado primero)
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --dry-run

# Importar a AWS real
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json

# Importar a LocalStack (desarrollo)
./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json \
  --endpoint-url http://localhost:4566
```

**Componentes:**
- `scripts/import-dynamodb.sh` - Script wrapper (bash)
- `scripts/import-dynamodb.py` - Script principal (Python)
- `scripts/test-import-dynamodb.py` - Tests unitarios
- `scripts/requirements.txt` - Dependencias Python (boto3)

**Tablas Soportadas:**
- `monitoring_producciones` - Producciones agrícolas
- `monitoring_escenas` - Escenas de satélite Sentinel-2

**Características:**
- Importa desde JSON o CSV
- Validación automática de esquema
- Modo dry-run (validar sin cambiar datos)
- Importa a AWS DynamoDB o LocalStack
- Manejo de errores robusto
- Conversión automática de tipos

**Documentación Completa:**
- `docs/DYNAMODB_IMPORT_GUIDE.md` - Guía detallada
- `docs/DYNAMODB_IMPORT_QUICKSTART.md` - Quick start (5 min)
- `data/dynamodb/examples/README.md` - Descripción de ejemplos

**Requisitos:**
- Python 3.7+
- boto3 (`pip install -r scripts/requirements.txt`)
- Credenciales AWS configuradas (si usa AWS real)

**Instalación Rápida:**
```bash
# Instalar dependencias
pip install -r scripts/requirements.txt

# Hacer script ejecutable (Linux/Mac)
chmod +x scripts/import-dynamodb.sh

# Ver ayuda
./scripts/import-dynamodb.sh --help
python3 scripts/import-dynamodb.py --help
```

**Testing:**
```bash
# Ejecutar test suite
python3 scripts/test-import-dynamodb.py

# Output esperado
# Test 1: Cargar datos desde JSON... ✓
# Test 2: Cargar datos desde CSV... ✓
# ...
# Resultados: 13 pasados, 0 fallidos
```

---

### Pre-Deployment Scripts

#### 1. pre-deploy-check.sh
**Propósito:** Valida que todo está listo para deployment

```bash
./scripts/pre-deploy-check.sh
```

**Qué valida:**
- Git working tree limpio
- Docker instalado
- MySQL accessible
- Código compila
- Todos los archivos necesarios existen

**Output esperado:**
```
✓ Git working tree limpio
✓ Docker instalado
✓ Code compila sin errores
...
✓ LISTO PARA DEPLOYMENT
```

---

#### 2. backup-all.sh
**Propósito:** Crea backup completo de todo el sistema

```bash
./scripts/backup-all.sh
```

**Crea backups de:**
- MySQL database (`.sql`)
- Docker volumes (`.tar.gz`)
- Configuración (`.env`, `configs/`)
- S3 data (si está disponible)
- DynamoDB tables (JSON)

**Output:**
```
Ubicación: ./backups/20260904_143022/
- agro_prod.sql (45MB)
- mysql_volume.tar.gz (230MB)
- .env.bak
- configs_backup/
- CHECKSUMS.txt
```

**Nota:** Genera checksums automáticamente para verificación posterior.

---

### Database Scripts

#### 3. migrate-db.sh
**Propósito:** Ejecuta migraciones de base de datos en orden

```bash
export MYSQL_USER=agro_admin
export MYSQL_PASSWORD=your_password
export MYSQL_DB=agro_prod
export MYSQL_HOST=localhost

./scripts/migrate-db.sh
```

**Ejecuta en orden:**
1. `init-mysql.sql` - Crear tablas maestras
2. `phase1-create-tables.sql` - Crear tablas adicionales
3. `phase2-update-s3-monitoring-producciones.sql` - Agregar relaciones

**Validaciones incluidas:**
- Verifica que scripts existen
- Ejecuta migraciones secuencialmente
- Valida tablas después de cada fase
- Muestra conteos de datos

---

### Docker Scripts

#### 4. build-and-test.sh
**Propósito:** Build imagen Docker y ejecuta tests

```bash
./scripts/build-and-test.sh 1.0.0 agro-sentinel
```

**Parámetros:**
- `$1` = Version (default: 1.0.0)
- `$2` = Registry prefix (default: agro-sentinel)

**Tareas:**
1. Build imagen con tags version y latest
2. Inspeciona imagen (size, created, etc.)
3. Ejecuta tests unitarios
4. Scan vulnerabilidades (si trivy disponible)

**Output:**
```
Version: 1.0.0
Registry: agro-sentinel
Imagen built: agro-sentinel/agro-sentinel-worker:1.0.0
Size: 523MB
Tests: 45 passed, 0 failed
```

---

### Validation Scripts

#### 5. health-check.sh
**Propósito:** Valida salud integral del sistema

```bash
./scripts/health-check.sh
```

**Verifica:**
- API health endpoint
- MySQL connectivity
- Database exists
- Docker containers running
- Container health status
- Database tables exist
- Data integrity

**Output:**
```
✓ API Health
✓ MySQL Connection
✓ Database Exists
✓ Docker Running
✓ MySQL Container Healthy
✓ Database Tables (6 found)

TODOS LOS SERVICIOS OPERACIONALES
```

---

### Rollback Scripts

#### 6. rollback.sh
**Propósito:** Revertir deployment a versión anterior

```bash
./scripts/rollback.sh
```

**Pide confirmación y luego:**
1. Detiene servicios: `docker-compose down`
2. Revierte código: `git reset --hard HEAD~1`
3. Restaura BD: `mysql < backup_latest.sql`
4. Rebuilda Docker: `docker build ...`
5. Reinicia servicios: `docker-compose up -d`

**Requiere:**
- Backup disponible en `backups/*/agro_prod.sql`
- Confirmación del usuario

---

### Operación Scripts

#### 7. morning-startup.sh
**Propósito:** Checklist de startup matutino

```bash
./scripts/morning-startup.sh
```

**Verifica:**
- Estado de containers
- API health
- MySQL status
- Conteos de datos
- Espacio en disco
- Recursos de containers

**Ideal para:** Ejecutar cada mañana antes de abrir servicio

---

#### 8. end-of-day.sh
**Propósito:** Reporte de fin de día y backup

```bash
./scripts/end-of-day.sh
```

**Tareas:**
1. Recolecta logs de todos los servicios
2. Genera estadísticas de BD
3. Analiza errores
4. Crea backup diario
5. Genera resumen en `logs/YYYYMMDD/`

**Output:**
```
Logs: logs/20260904/
Backup: backups/daily_20260904.sql
Summary: logs/20260904/summary.txt
```

---

## Uso Avanzado

### Scriptable Usage (CI/CD Integration)

```bash
#!/bin/bash
# Deployment en CI/CD

set -e

# Pre-deployment
./scripts/pre-deploy-check.sh || exit 1
./scripts/backup-all.sh || exit 1

# Database
./scripts/migrate-db.sh || exit 1

# Docker
./scripts/build-and-test.sh 1.0.0 || exit 1

# Validation
./scripts/health-check.sh || exit 1

echo "✓ Deployment exitoso"
```

### Cron Jobs

```bash
# Daily backup (2 AM)
0 2 * * * /path/to/go-agro-sentinel-worker/scripts/backup-all.sh

# Morning startup check (7 AM)
0 7 * * * /path/to/go-agro-sentinel-worker/scripts/morning-startup.sh

# End of day report (6 PM)
0 18 * * * /path/to/go-agro-sentinel-worker/scripts/end-of-day.sh

# Weekly health check (Mondays 8 AM)
0 8 * * 1 /path/to/go-agro-sentinel-worker/scripts/health-check.sh
```

### Monitoreo Continuo

```bash
# Health check cada 5 minutos
watch -n 300 './scripts/health-check.sh'

# Dashboard de monitoreo
watch -n 5 'docker stats --no-stream agro-sentinel-*'

# Logs en tiempo real
docker-compose logs -f api worker mysql
```

---

## Variables de Entorno Requeridas

Por script:

### migrate-db.sh
```bash
MYSQL_HOST=localhost
MYSQL_USER=agro_admin
MYSQL_PASSWORD=secure_password
MYSQL_DB=agro_prod
```

### build-and-test.sh
```bash
# Opcional - para push a registry
DOCKER_REGISTRY=your.registry.com
DOCKER_USERNAME=your_username
DOCKER_PASSWORD=your_password
```

### health-check.sh
```bash
API_URL=http://localhost:8088
MYSQL_HOST=localhost
MYSQL_USER=agro_admin
MYSQL_PASSWORD=secure_password
MYSQL_DB=agro_prod
```

---

## Error Codes

| Código | Significado | Acción |
|--------|-----------|--------|
| 0 | Éxito | Continuar |
| 1 | Error genérico | Revisar logs |
| 2 | Parámetros inválidos | Verificar uso |
| 127 | Comando no encontrado | Instalar dependencia |
| 255 | Cancelado por usuario | Reintentar |

---

## Troubleshooting Scripts

Si un script falla:

```bash
# 1. Ver qué falla
bash -x ./scripts/script-name.sh

# 2. Ver output completo
./scripts/script-name.sh 2>&1 | tee output.log

# 3. Ejecutar en modo seguro
set -e
source ./scripts/script-name.sh

# 4. Validar dependencias
which docker
which mysql
which go
```

---

## Personalización

### Modificar Scripts

Puedes personalizar variables al inicio de cada script:

```bash
# En pre-deploy-check.sh
RED='\033[0;31m'       # Color para errores
GREEN='\033[0;32m'     # Color para éxito
TIMEOUT_SECONDS=300    # Timeout máximo
```

### Agregar Validaciones Propias

```bash
# Agregar check al health-check.sh
check_health "Mi Check" "mi_comando_aqui"
```

---

## Performance

Tiempo de ejecución esperado por script:

| Script | Tiempo |
|--------|--------|
| pre-deploy-check.sh | 2-5 min |
| backup-all.sh | 5-30 min |
| migrate-db.sh | 2-5 min |
| build-and-test.sh | 10-15 min |
| health-check.sh | 1-2 min |
| rollback.sh | 20-30 min |
| morning-startup.sh | 2-5 min |
| end-of-day.sh | 5-10 min |

---

## Logging

Todos los scripts generan logs:

```bash
# Logs se guardan en
logs/YYYYMMDD/
├── api.log
├── worker.log
├── mysql.log
├── db_stats.txt
└── summary.txt
```

Consultar logs:
```bash
tail -f logs/*/api.log
grep -i error logs/*/*.log
cat logs/*/summary.txt
```

---

## Dependencias de Scripts

### Required
- bash 4.0+
- docker
- docker-compose
- mysql (client)

### Optional
- git (para rollback)
- curl (para health checks)
- jq (para JSON parsing)
- trivy (para security scanning)
- aws-cli (para S3/DynamoDB backups)

---

## Cheat Sheet

```bash
# Verificación rápida
./scripts/health-check.sh

# Backup antes de cualquier cosa
./scripts/backup-all.sh

# Deployment completo
./scripts/pre-deploy-check.sh && \
./scripts/migrate-db.sh && \
./scripts/build-and-test.sh && \
./scripts/health-check.sh

# Emergencia - rollback
./scripts/rollback.sh

# Monitoreo diario
./scripts/morning-startup.sh
./scripts/end-of-day.sh

# Debugging
bash -x ./scripts/script-name.sh 2>&1 | less
```

---

## FAQ sobre Scripts

**P: ¿Puedo ejecutar scripts en paralelo?**  
R: No, algunos dependen de otros. Ejecuta secuencialmente.

**P: ¿Se pueden personalizar los scripts?**  
R: Sí, modifica las variables al inicio de cada script.

**P: ¿Qué pasa si un script falla?**  
R: Se detiene (set -e). Revisa logs y corrige el problema.

**P: ¿Se pueden usar en CI/CD?**  
R: Sí, son idempotentes y retornan exit codes correctos.

**P: ¿Dónde se guardan los logs?**  
R: En `logs/YYYYMMDD/` dentro del proyecto.

---

## Support

Para problemas con scripts:
1. Ejecuta con `bash -x` para debug
2. Revisa variable de entorno requeridas
3. Contacta: drh.megafresh@gmail.com

---

**Versión:** 1.0  
**Última actualización:** 2026-09-04  
**Mantenedor:** Daniel Rivera Herrada
