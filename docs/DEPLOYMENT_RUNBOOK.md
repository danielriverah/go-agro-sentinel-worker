# Deployment Runbook - Agro Sentinel Worker

**Versión:** 1.0  
**Objetivo:** Guía paso a paso para ejecutar deployment en ambiente de producción  
**Tiempo estimado:** 3-4 horas  
**Última actualización:** 2026-09-04

---

## Tabla de Contenidos

1. [Inicio Rápido](#inicio-rápido)
2. [Scripts de Deployment](#scripts-de-deployment)
3. [Procedimientos Operacionales](#procedimientos-operacionales)
4. [Troubleshooting](#troubleshooting)
5. [Validación Exhaustiva](#validación-exhaustiva)

---

## Inicio Rápido

### Para Deployment Rápido (5 minutos)

```bash
#!/bin/bash
# quick-deploy.sh

set -e

echo "=== DEPLOYMENT RÁPIDO AGRO SENTINEL ==="

# 1. Actualizar código
git pull origin feat/agro-sentinel-worker

# 2. Rebuild Docker
docker-compose build

# 3. Reiniciar servicios
docker-compose down
docker-compose up -d

# 4. Verificar
sleep 10
curl http://localhost:8088/health || echo "API NO DISPONIBLE"

echo "=== DEPLOYMENT COMPLETADO ==="
```

### Para Deployment Completo (3-4 horas)

Ver `DEPLOYMENT_PLAN.md` - seguir sección "Pasos de Deployment" completa.

---

## Scripts de Deployment

### 1. Script: Pre-Deployment Validation

**Archivo:** `scripts/pre-deploy-check.sh`

```bash
#!/bin/bash
# pre-deploy-check.sh
# Valida que todo está listo para deployment

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${YELLOW}=== PRE-DEPLOYMENT VALIDATION ===${NC}\n"

PASS=0
FAIL=0

# Función para test
check() {
  if eval "$1" > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} $2"
    ((PASS++))
  else
    echo -e "${RED}✗${NC} $2"
    ((FAIL++))
  fi
}

# Verificaciones
check "git status | grep -q 'working tree clean'" "Git working tree limpio"
check "command -v docker" "Docker instalado"
check "command -v mysql" "MySQL client instalado"
check "command -v go" "Go instalado"
check "go build -o /tmp/test-build ./cmd/api" "Código compila sin errores"
check "test -f .env" "Archivo .env existe"
check "test -f docker-compose.yml" "docker-compose.yml existe"
check "test -f scripts/init-mysql.sql" "Scripts SQL existen"
check "mysql -u root -p -e 'SELECT 1' 2>/dev/null" "MySQL accessible"
check "docker images | grep -q 'agro-sentinel'" "Imagen Docker disponible"

echo ""
echo -e "${GREEN}Pasados: $PASS${NC}"
echo -e "${RED}Fallidos: $FAIL${NC}"

if [ $FAIL -gt 0 ]; then
  echo -e "${RED}ERROR: Hay verificaciones fallidas${NC}"
  exit 1
fi

echo -e "${GREEN}✓ LISTO PARA DEPLOYMENT${NC}"
exit 0
```

**Uso:**
```bash
chmod +x scripts/pre-deploy-check.sh
./scripts/pre-deploy-check.sh
```

---

### 2. Script: Backup Completo

**Archivo:** `scripts/backup-all.sh`

```bash
#!/bin/bash
# backup-all.sh
# Crea backup completo antes de deployment

set -e

BACKUP_DIR="./backups/$(date +%Y%m%d_%H%M%S)"
mkdir -p "$BACKUP_DIR"

echo "=== INICIANDO BACKUP A: $BACKUP_DIR ==="

# 1. Backup MySQL
echo "[1/5] Backup de MySQL..."
mysqldump -u agro_admin -p agro_prod \
  --single-transaction \
  --routines \
  --events \
  > "$BACKUP_DIR/agro_prod.sql"
echo "  ✓ Tamaño: $(du -h "$BACKUP_DIR/agro_prod.sql" | cut -f1)"

# 2. Backup Docker volumes
echo "[2/5] Backup de volúmenes Docker..."
docker run --rm \
  -v go-agro-sentinel-mysql-data:/data \
  -v "$BACKUP_DIR":/backup \
  alpine tar czf /backup/mysql_volume.tar.gz -C /data .
echo "  ✓ Tamaño: $(du -h "$BACKUP_DIR/mysql_volume.tar.gz" | cut -f1)"

# 3. Backup configuración
echo "[3/5] Backup de configuración..."
cp .env "$BACKUP_DIR/.env.bak"
cp -r configs "$BACKUP_DIR/configs_backup"
echo "  ✓ Archivos: .env, configs/*"

# 4. Backup S3 (si está en uso)
echo "[4/5] Backup de S3..."
mkdir -p "$BACKUP_DIR/s3_data"
aws s3 cp s3://agro-sentinel-data "$BACKUP_DIR/s3_data" \
  --recursive \
  --endpoint-url "${AWS_ENDPOINT_URL:-http://localhost:4566}" \
  2>/dev/null || echo "  ! S3 no accesible (local mode)"

# 5. Backup DynamoDB (si está en uso)
echo "[5/5] Backup de DynamoDB..."
aws dynamodb scan --table-name agro-sentinel-scenes \
  --endpoint-url "${AWS_ENDPOINT_URL:-http://localhost:4566}" \
  > "$BACKUP_DIR/dynamodb_scenes.json" 2>/dev/null || echo "  ! DynamoDB no accesible (local mode)"

echo ""
echo "=== BACKUP COMPLETADO ==="
echo "Ubicación: $BACKUP_DIR"
echo "Archivos:"
ls -lh "$BACKUP_DIR/" | tail -n +2

# Generar checksum
cd "$BACKUP_DIR"
sha256sum * > CHECKSUMS.txt
cd - > /dev/null

echo ""
echo "Checksums guardados en: $BACKUP_DIR/CHECKSUMS.txt"
```

**Uso:**
```bash
chmod +x scripts/backup-all.sh
./scripts/backup-all.sh
```

---

### 3. Script: Database Migration

**Archivo:** `scripts/migrate-db.sh`

```bash
#!/bin/bash
# migrate-db.sh
# Ejecuta migraciones de BD en orden

set -e

MYSQL_USER="${MYSQL_USER:-agro_admin}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:-}"
MYSQL_DB="${MYSQL_DB:-agro_prod}"
MYSQL_HOST="${MYSQL_HOST:-localhost}"

echo "=== DATABASE MIGRATION ==="
echo "Host: $MYSQL_HOST"
echo "Database: $MYSQL_DB"
echo ""

# Función para ejecutar script
run_migration() {
  local script=$1
  local name=$2
  
  if [ ! -f "$script" ]; then
    echo "ERROR: Script no encontrado: $script"
    return 1
  fi
  
  echo "Ejecutando: $name..."
  
  if [ -z "$MYSQL_PASSWORD" ]; then
    mysql -u "$MYSQL_USER" -h "$MYSQL_HOST" "$MYSQL_DB" < "$script"
  else
    mysql -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" -h "$MYSQL_HOST" "$MYSQL_DB" < "$script"
  fi
  
  echo "  ✓ $name completado"
}

# Ejecutar migraciones en orden
run_migration "scripts/init-mysql.sql" "Init MySQL - Tablas maestras"
run_migration "scripts/phase1-create-tables.sql" "Phase 1 - Crear tablas"
run_migration "scripts/phase2-update-s3-monitoring-producciones.sql" "Phase 2 - Relaciones"

echo ""
echo "=== MIGRACIONES COMPLETADAS ==="

# Validar
echo "Validando..."
mysql -u "$MYSQL_USER" -h "$MYSQL_HOST" "$MYSQL_DB" << EOF
SELECT 'Tablas creadas:' as info;
SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES 
WHERE TABLE_SCHEMA = DATABASE() 
ORDER BY TABLE_NAME;

SELECT '' as info;
SELECT 'Conteos:' as info;
SELECT 'articulos' as tabla, COUNT(*) as total FROM articulos
UNION ALL
SELECT 'centros_costos', COUNT(*) FROM centros_costos
UNION ALL
SELECT 'producciones', COUNT(*) FROM producciones
UNION ALL
SELECT 'zonas_producciones', COUNT(*) FROM zonas_producciones;
EOF
```

**Uso:**
```bash
export MYSQL_USER=agro_admin
export MYSQL_PASSWORD=secure_password_here
export MYSQL_DB=agro_prod
export MYSQL_HOST=localhost

chmod +x scripts/migrate-db.sh
./scripts/migrate-db.sh
```

---

### 4. Script: Docker Build & Test

**Archivo:** `scripts/build-and-test.sh`

```bash
#!/bin/bash
# build-and-test.sh
# Build imagen Docker y ejecuta tests

set -e

VERSION="${1:-1.0.0}"
REGISTRY="${2:-agro-sentinel}"

echo "=== DOCKER BUILD & TEST ==="
echo "Version: $VERSION"
echo "Registry: $REGISTRY"
echo ""

# 1. Build image
echo "[1/4] Building Docker image..."
docker build \
  --build-arg VERSION="$VERSION" \
  --tag "$REGISTRY/agro-sentinel-worker:$VERSION" \
  --tag "$REGISTRY/agro-sentinel-worker:latest" \
  -f Dockerfile \
  .

echo "  ✓ Imagen built"
echo ""

# 2. Inspect image
echo "[2/4] Verificando imagen..."
docker image inspect "$REGISTRY/agro-sentinel-worker:$VERSION" | \
  jq '.[] | {
    Size: .Size,
    VirtualSize: .VirtualSize,
    Created: .Created,
    Os: .Os,
    Architecture: .Architecture
  }'

echo ""

# 3. Run tests
echo "[3/4] Ejecutando tests..."
docker run --rm "$REGISTRY/agro-sentinel-worker:$VERSION" \
  go test -v ./... || echo "  ! Tests fallidos en contenedor"

echo ""

# 4. Scan vulnerabilities (si tienes trivy)
echo "[4/4] Scanning vulnerabilities..."
if command -v trivy &> /dev/null; then
  trivy image "$REGISTRY/agro-sentinel-worker:$VERSION"
else
  echo "  ! trivy no instalado (opcional)"
fi

echo ""
echo "=== BUILD COMPLETADO ==="
echo "Imagen: $REGISTRY/agro-sentinel-worker:$VERSION"
echo "Ready for deployment"
```

**Uso:**
```bash
chmod +x scripts/build-and-test.sh
./scripts/build-and-test.sh 1.0.0
```

---

### 5. Script: Health Check

**Archivo:** `scripts/health-check.sh`

```bash
#!/bin/bash
# health-check.sh
# Verifica salud de todos los componentes

set -e

API_URL="${API_URL:-http://localhost:8088}"
MYSQL_HOST="${MYSQL_HOST:-localhost}"
MYSQL_USER="${MYSQL_USER:-agro_admin}"
MYSQL_DB="${MYSQL_DB:-agro_prod}"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

check_health() {
  local name=$1
  local cmd=$2
  
  if eval "$cmd" > /dev/null 2>&1; then
    echo -e "${GREEN}✓${NC} $name"
    return 0
  else
    echo -e "${RED}✗${NC} $name"
    return 1
  fi
}

echo "=== HEALTH CHECK ==="
echo ""

PASSED=0
FAILED=0

# Verificaciones
if check_health "API Health" "curl -s $API_URL/health | grep -q 'healthy'"; then
  ((PASSED++))
else
  ((FAILED++))
fi

if check_health "MySQL Connection" "mysql -h $MYSQL_HOST -u $MYSQL_USER -e 'SELECT 1' > /dev/null"; then
  ((PASSED++))
else
  ((FAILED++))
fi

if check_health "Database Exists" "mysql -h $MYSQL_HOST -u $MYSQL_USER -e \"SELECT 1 FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = '$MYSQL_DB'\""; then
  ((PASSED++))
else
  ((FAILED++))
fi

if check_health "Docker Running" "docker ps | grep -q 'agro-sentinel'"; then
  ((PASSED++))
else
  ((FAILED++))
fi

if check_health "MySQL Container Healthy" "docker ps | grep go-agro-sentinel-mysql | grep -q healthy"; then
  ((PASSED++))
else
  ((FAILED++))
fi

# Tables check
TABLE_COUNT=$(mysql -h $MYSQL_HOST -u $MYSQL_USER $MYSQL_DB -se "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = '$MYSQL_DB';" 2>/dev/null || echo "0")

if [ "$TABLE_COUNT" -ge 6 ]; then
  check_health "Database Tables ($TABLE_COUNT found)" "test $TABLE_COUNT -ge 6"
  ((PASSED++))
else
  echo -e "${RED}✗${NC} Database Tables (solo $TABLE_COUNT encontradas, se esperan >=6)"
  ((FAILED++))
fi

echo ""
echo -e "${GREEN}Pasados: $PASSED${NC}"
echo -e "${RED}Fallidos: $FAILED${NC}"

if [ $FAILED -gt 0 ]; then
  echo ""
  echo "Troubleshooting:"
  echo "  - Revisar logs: docker logs go-agro-sentinel-api"
  echo "  - Revisar BD: mysql -h $MYSQL_HOST -u $MYSQL_USER $MYSQL_DB"
  echo "  - Reiniciar: docker-compose restart"
  exit 1
fi

echo ""
echo -e "${GREEN}✓ TODOS LOS SERVICIOS OPERACIONALES${NC}"
exit 0
```

**Uso:**
```bash
chmod +x scripts/health-check.sh
./scripts/health-check.sh
```

---

### 6. Script: Rollback Automático

**Archivo:** `scripts/rollback.sh`

```bash
#!/bin/bash
# rollback.sh
# Revierte deployment a versión anterior

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${RED}=== ROLLBACK INICIADO ===${NC}"
echo "Hora: $(date)"
echo ""

read -p "¿Está seguro de que desea hacer rollback? (s/n): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Ss]$ ]]; then
  echo "Rollback cancelado"
  exit 1
fi

# 1. Detener servicios
echo -e "${YELLOW}[1/4] Deteniendo servicios...${NC}"
docker-compose down
echo -e "${GREEN}✓ Servicios detenidos${NC}"

# 2. Revertir código
echo -e "${YELLOW}[2/4] Revirtiendo código...${NC}"
git reset --hard HEAD~1
echo -e "${GREEN}✓ Código revertido${NC}"

# 3. Restaurar BD
echo -e "${YELLOW}[3/4] Restaurando base de datos...${NC}"
LATEST_BACKUP=$(ls -t backups/*/agro_prod.sql 2>/dev/null | head -1)

if [ -z "$LATEST_BACKUP" ]; then
  echo -e "${RED}ERROR: Backup no encontrado${NC}"
  exit 1
fi

mysql -u agro_admin -p < "$LATEST_BACKUP"
echo -e "${GREEN}✓ Base de datos restaurada desde $LATEST_BACKUP${NC}"

# 4. Reiniciar servicios
echo -e "${YELLOW}[4/4] Reiniciando servicios...${NC}"
docker-compose up -d
sleep 10

# Verificar
if curl -s http://localhost:8088/health | grep -q "healthy"; then
  echo -e "${GREEN}✓ Servicios reiniciados correctamente${NC}"
else
  echo -e "${RED}WARNING: Health check fallido${NC}"
fi

echo ""
echo -e "${GREEN}=== ROLLBACK COMPLETADO ===${NC}"
echo "Verifica estado:"
echo "  docker logs go-agro-sentinel-api"
echo "  curl http://localhost:8088/health"
```

**Uso:**
```bash
chmod +x scripts/rollback.sh
./scripts/rollback.sh
```

---

## Procedimientos Operacionales

### Procedimiento 1: Deployment Completo

#### Tiempo: 3-4 horas

```bash
#!/bin/bash
# complete-deployment.sh

set -e

echo "=== DEPLOYMENT COMPLETO ==="
start_time=$(date +%s)

# FASE 1: Pre-deployment
echo "[FASE 1] PRE-DEPLOYMENT"
./scripts/pre-deploy-check.sh || exit 1
./scripts/backup-all.sh || exit 1

# FASE 2: Database
echo "[FASE 2] MIGRACIÓN DE BD"
./scripts/migrate-db.sh || exit 1

# FASE 3: Docker
echo "[FASE 3] BUILD DOCKER"
./scripts/build-and-test.sh 1.0.0 || exit 1

# FASE 4: Deployment
echo "[FASE 4] DEPLOYMENT"
docker-compose down
docker-compose up -d
sleep 15

# FASE 5: Validation
echo "[FASE 5] VALIDACIÓN"
./scripts/health-check.sh || exit 1

# Timing
end_time=$(date +%s)
duration=$((end_time - start_time))

echo ""
echo "=== DEPLOYMENT EXITOSO ==="
echo "Duración: $((duration / 60)) minutos $((duration % 60)) segundos"
```

---

### Procedimiento 2: Update Solo de Código

#### Tiempo: 5-10 minutos

```bash
#!/bin/bash
# update-code-only.sh

set -e

echo "=== UPDATE CÓDIGO SOLO ==="

# 1. Get latest code
git pull origin feat/agro-sentinel-worker

# 2. Verify compilation
go build -o /tmp/test-api ./cmd/api
go build -o /tmp/test-worker ./cmd/worker

# 3. Rebuild Docker
docker-compose build

# 4. Restart with grace
docker-compose stop api worker sync
sleep 5
docker-compose up -d api worker sync

# 5. Wait and verify
sleep 10
curl http://localhost:8088/health

echo "✓ Code update completado"
```

---

### Procedimiento 3: Actualización de Base de Datos Only

#### Tiempo: 10-15 minutos

```bash
#!/bin/bash
# update-db-only.sh

set -e

echo "=== UPDATE BD SOLO ==="

# 1. Backup
./scripts/backup-all.sh

# 2. Execute migration
./scripts/migrate-db.sh

# 3. Verify
mysql -u agro_admin -p agro_prod -e "
  SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES 
  WHERE TABLE_SCHEMA = 'agro_prod';
"

echo "✓ BD update completado"
```

---

## Troubleshooting

### Problema 1: MySQL Connection Refused

**Síntoma:**
```
ERROR 2003 (HY000): Can't connect to MySQL server on 'localhost' (111)
```

**Solución:**
```bash
# 1. Verificar contenedor
docker ps | grep mysql

# 2. Verificar logs
docker logs go-agro-sentinel-mysql

# 3. Reiniciar
docker-compose restart mysql

# 4. Esperar a que esté healthy
docker exec -it go-agro-sentinel-mysql \
  mysqladmin -u root -proot ping

# 5. Retry
mysql -u agro_admin -p agro_prod -e "SELECT 1;"
```

---

### Problema 2: Docker Build Falla

**Síntoma:**
```
error: golang not found in builder stage
```

**Solución:**
```bash
# 1. Limpiar images
docker system prune -a

# 2. Rebuild
docker build --no-cache -f Dockerfile -t agro-sentinel-worker:latest .

# 3. Si persiste, verificar GO
go version
# Debe ser 1.24+
```

---

### Problema 3: Foreign Key Constraint Failed

**Síntoma:**
```
ERROR 1452 (23000): Cannot add or update a child row
```

**Solución:**
```bash
# 1. Verificar datos
mysql -u agro_admin -p agro_prod << EOF
-- Ver si hay datos huérfanos
SELECT COUNT(*) FROM producciones 
WHERE articulo_id NOT IN (SELECT articulo_id FROM articulos);

-- Limpiar si es necesario
DELETE FROM producciones 
WHERE articulo_id NOT IN (SELECT articulo_id FROM articulos);
EOF

# 2. Retry migration
./scripts/migrate-db.sh
```

---

### Problema 4: Disk Space

**Síntoma:**
```
ERROR: no space left on device
```

**Solución:**
```bash
# 1. Verificar espacio
df -h /

# 2. Limpiar Docker
docker system prune

# 3. Remover volúmenes antiguos
docker volume prune

# 4. Limpiar logs
docker logs --tail 0 go-agro-sentinel-mysql
find /var/lib/docker/containers -name '*.log' -delete

# 5. Expandir storage si es necesario
# (Depende de tu infraestructura)
```

---

### Problema 5: API No Inicia

**Síntoma:**
```
docker logs go-agro-sentinel-api
# Error: database connection failed
```

**Solución:**
```bash
# 1. Verificar variables de entorno
docker inspect go-agro-sentinel-api | grep -A 20 "Env"

# 2. Verificar conectividad de BD
docker exec go-agro-sentinel-api \
  mysql -h go-agro-sentinel-mysql -u agro_admin -p agro_prod -e "SELECT 1;"

# 3. Revisar config
cat configs/config.production.yaml

# 4. Reiniciar con logs
docker-compose logs -f api
```

---

## Validación Exhaustiva

### Checklist de Validación Post-Deployment

```bash
#!/bin/bash
# post-deployment-validation.sh

set -e

CHECKS_PASSED=0
CHECKS_FAILED=0

test_check() {
  if eval "$1"; then
    echo "✓ $2"
    ((CHECKS_PASSED++))
  else
    echo "✗ $2"
    ((CHECKS_FAILED++))
  fi
}

echo "=== POST-DEPLOYMENT VALIDATION ==="
echo ""

# API Tests
echo "API TESTS:"
test_check "curl -s http://localhost:8088/health | grep -q healthy" "API health check"
test_check "curl -s http://localhost:8088/health | grep -q version" "API version available"

# Database Tests
echo ""
echo "DATABASE TESTS:"
test_check "mysql -u agro_admin -p agro_prod -e 'SELECT 1' > /dev/null 2>&1" "MySQL accessible"
test_check "mysql -u agro_admin -p agro_prod -e 'SELECT COUNT(*) FROM articulos' > /dev/null" "articulos table exists"
test_check "mysql -u agro_admin -p agro_prod -e 'SELECT COUNT(*) FROM centros_costos' > /dev/null" "centros_costos table exists"
test_check "mysql -u agro_admin -p agro_prod -e 'SELECT COUNT(*) FROM producciones' > /dev/null" "producciones table exists"

# Docker Tests
echo ""
echo "DOCKER TESTS:"
test_check "docker ps | grep -q go-agro-sentinel-api" "API container running"
test_check "docker ps | grep -q go-agro-sentinel-mysql" "MySQL container running"
test_check "docker ps | grep -q go-agro-sentinel-worker" "Worker container running"

# Data Tests
echo ""
echo "DATA INTEGRITY TESTS:"
test_check "mysql -u agro_admin -p agro_prod -e 'SELECT COUNT(*) FROM articulos' | grep -v 'COUNT' | grep -q '[1-9]'" "Data exists in articulos"
test_check "mysql -u agro_admin -p agro_prod -e 'SELECT COUNT(*) FROM centros_costos' | grep -v 'COUNT' | grep -q '[1-9]'" "Data exists in centros_costos"

# Performance Tests
echo ""
echo "PERFORMANCE TESTS:"
test_check "curl -w '%{time_total}\n' -o /dev/null -s http://localhost:8088/health | awk '{if (\$1 < 1) exit 0; else exit 1}'" "API response < 1 second"

echo ""
echo "SUMMARY:"
echo "  Passed: $CHECKS_PASSED"
echo "  Failed: $CHECKS_FAILED"

if [ $CHECKS_FAILED -eq 0 ]; then
  echo ""
  echo "✓ ALL TESTS PASSED"
  exit 0
else
  echo ""
  echo "✗ SOME TESTS FAILED"
  exit 1
fi
```

---

## Comandos Útiles Rápidos

```bash
# Ver logs en tiempo real
docker-compose logs -f api

# Ejecutar comando en container
docker-compose exec mysql mysql -u agro_admin -p agro_prod

# Acceder a shell de container
docker-compose exec api /bin/bash

# Ver recursos utilizados
docker stats

# Limpiar todo y resetear
docker-compose down -v

# Backup rápido
mysqldump -u agro_admin -p agro_prod > backup_$(date +%s).sql

# Restore rápido
mysql -u agro_admin -p agro_prod < backup_1234567890.sql
```

---

**Este runbook está diseñado para ser impreso y tener a mano durante deployment.**

Para emergencias: contactar drh.megafresh@gmail.com
