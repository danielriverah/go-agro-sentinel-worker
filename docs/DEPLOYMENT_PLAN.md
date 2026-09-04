# Plan de Deployment - Agro Sentinel Worker

**Versión:** 1.0  
**Fecha:** 2026-09-04  
**Autor:** Daniel Rivera Herrada  
**Estado:** Production Ready

## Tabla de Contenidos

1. [Pre-Deployment Checklist](#pre-deployment-checklist)
2. [Pasos de Deployment](#pasos-de-deployment)
3. [Validación Post-Deployment](#validación-post-deployment)
4. [Plan de Rollback](#plan-de-rollback)
5. [Consideraciones Operacionales](#consideraciones-operacionales)
6. [Timeline y Estimaciones](#timeline-y-estimaciones)
7. [Contactos y Escalación](#contactos-y-escalación)

---

## Pre-Deployment Checklist

### 1. Verificaciones de Código

- [ ] **Git Branch**: Verificar que se está en la rama correcta (`feat/agro-sentinel-worker`)
  ```bash
  git branch -v
  git log --oneline -5
  ```

- [ ] **Cambios Locales**: Asegurar que no haya cambios no comprometidos
  ```bash
  git status
  git diff --stat
  ```

- [ ] **Commits**: Revisar que todos los commits están en la rama
  ```bash
  git log main..HEAD --oneline
  ```

- [ ] **Código Go**: Verificar que el código compila sin errores
  ```bash
  go build -o /tmp/api ./cmd/api
  go build -o /tmp/worker ./cmd/worker
  go build -o /tmp/sync ./cmd/sync
  ```

- [ ] **Formato de Código**: Ejecutar `gofmt` en todos los archivos
  ```bash
  go fmt ./...
  ```

### 2. Tests Requeridos

- [ ] **Unit Tests**: Todos los tests deben pasar
  ```bash
  go test -v ./...
  ```

- [ ] **Coverage**: Mínimo 70% de cobertura
  ```bash
  go test -cover ./...
  ```

- [ ] **Tests de Integración**: Ejecutar tests con dependencias (MySQL, S3, SQS, DynamoDB)
  ```bash
  docker-compose up -d
  go test -v -tags=integration ./...
  ```

- [ ] **Health Checks**: Verificar endpoints de salud
  ```bash
  curl http://localhost:8088/health
  curl http://localhost:6000/health
  ```

### 3. Validaciones de Base de Datos

- [ ] **Backup de Datos**: Crear backup de la BD antes de deployment
  ```bash
  mysqldump -u admin -padmin --all-databases > backup_$(date +%Y%m%d_%H%M%S).sql
  ```

- [ ] **Verificar Integridad**: Comprobar integridad de bases de datos
  ```sql
  CHECK TABLE articulos, centros_costos, producciones, zonas_producciones;
  ```

- [ ] **Schema Validation**: Verificar que el esquema es correcto
  ```sql
  DESCRIBE articulos;
  DESCRIBE centros_costos;
  DESCRIBE producciones;
  DESCRIBE zonas_producciones;
  DESCRIBE asignaciones_zonas_producciones;
  DESCRIBE s3_monitoring_escena_ia_resumen;
  ```

- [ ] **Datos de Prueba**: Validar que los datos de prueba existen
  ```sql
  SELECT COUNT(*) as total_articulos FROM articulos;
  SELECT COUNT(*) as total_centros FROM centros_costos;
  SELECT COUNT(*) as total_producciones FROM producciones;
  ```

### 4. Backups Requeridos

- [ ] **MySQL Full Backup**: Backup completo de la base de datos
  ```bash
  mysqldump -u admin -padmin agro > backup_agro_$(date +%Y%m%d_%H%M%S).sql
  ```

- [ ] **Docker Volumes**: Backup de volúmenes persistentes
  ```bash
  docker run --rm \
    -v go-agro-sentinel-mysql-data:/data \
    -v $(pwd)/backups:/backup \
    alpine tar czf /backup/mysql_$(date +%Y%m%d_%H%M%S).tar.gz -C /data .
  ```

- [ ] **LocalStack/S3 Data**: Backup de datos en S3 local
  ```bash
  aws s3 cp s3://agro-sentinel-data . --recursive \
    --endpoint-url http://localhost:4566
  ```

- [ ] **DynamoDB Tables**: Exportar tablas DynamoDB
  ```bash
  aws dynamodb scan --table-name agro-sentinel-scenes \
    --endpoint-url http://localhost:4566 > dynamodb_backup.json
  ```

### 5. Variables de Entorno

- [ ] **Producción**: Verificar archivo `.env.production`
  ```bash
  ls -la .env.production
  cat .env.production | head -10
  ```

- [ ] **Secrets**: Verificar que todos los secrets están configurados
  ```bash
  # AWS credentials
  echo $AWS_ACCESS_KEY_ID
  echo $AWS_REGION
  
  # MySQL credentials
  echo $MYSQL_HOST
  echo $MYSQL_USER
  ```

- [ ] **Configuración YAML**: Verificar archivo de configuración
  ```bash
  cat configs/config.production.yaml | head -20
  ```

### 6. Infraestructura

- [ ] **Docker**: Verificar que Docker está instalado y en ejecución
  ```bash
  docker --version
  docker ps
  docker-compose --version
  ```

- [ ] **Puertos Disponibles**: Verificar que los puertos requeridos están disponibles
  ```bash
  # Puerto 8088 para API
  netstat -an | grep 8088
  # Puerto 3309 para MySQL
  netstat -an | grep 3309
  # Puerto 4566 para LocalStack
  netstat -an | grep 4566
  ```

- [ ] **Espacio en Disco**: Verificar espacio disponible
  ```bash
  df -h /
  du -sh /var/lib/docker/volumes/
  ```

---

## Pasos de Deployment

### Fase 1: Preparación

#### Paso 1.1: Clonar o Actualizar Repositorio
```bash
cd /ruta/a/deployment
git clone https://github.com/tu-org/go-agro-sentinel-worker.git
cd go-agro-sentinel-worker
git checkout feat/agro-sentinel-worker
git pull origin feat/agro-sentinel-worker
```

#### Paso 1.2: Configurar Variables de Entorno
```bash
# Copiar archivo .env de ejemplo
cp .env.example .env.production

# Editar con valores de producción
nano .env.production
```

Contenido ejemplo de `.env.production`:
```
MYSQL_HOST=prod-mysql.internal
MYSQL_USER=agro_admin
MYSQL_PASSWORD=secure_password_here
MYSQL_DATABASE=agro_prod

AWS_REGION=us-east-1
AWS_ENDPOINT_URL=https://s3.amazonaws.com

GDAL_CONFIG_PATH=/usr/local/etc/gdal

LOG_LEVEL=INFO
DEBUG=false
```

#### Paso 1.3: Crear Directorio de Logs
```bash
mkdir -p /var/log/agro-sentinel
chmod 755 /var/log/agro-sentinel
```

---

### Fase 2: Base de Datos

#### Paso 2.1: Crear Base de Datos
```bash
mysql -u root -p << EOF
CREATE DATABASE IF NOT EXISTS agro_prod CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
CREATE USER IF NOT EXISTS 'agro_admin'@'%' IDENTIFIED BY 'secure_password_here';
GRANT ALL PRIVILEGES ON agro_prod.* TO 'agro_admin'@'%' WITH GRANT OPTION;
FLUSH PRIVILEGES;
EOF
```

#### Paso 2.2: Ejecutar Scripts SQL en Orden

**IMPORTANTE**: Ejecutar en este orden exacto:

**1. Init MySQL (Crear tablas maestras)**
```bash
mysql -u agro_admin -p agro_prod < scripts/init-mysql.sql
```

Verificar resultado:
```sql
SELECT COUNT(*) as articulos FROM articulos;
SELECT COUNT(*) as centros FROM centros_costos;
SELECT COUNT(*) as producciones FROM producciones;
```

**2. Phase 1 - Create Tables (Tablas adicionales)**
```bash
mysql -u agro_admin -p agro_prod < scripts/phase1-create-tables.sql
```

Verificar:
```sql
DESCRIBE zonas_producciones;
DESCRIBE asignaciones_zonas_producciones;
DESCRIBE s3_monitoring_escena_ia_resumen;
```

**3. Phase 2 - Update S3 Monitoring (Relaciones)**
```bash
mysql -u agro_admin -p agro_prod < scripts/phase2-update-s3-monitoring-producciones.sql
```

Verificar:
```sql
DESCRIBE s3_monitoring_producciones;
SELECT COUNT(articulo_id) FROM s3_monitoring_producciones WHERE articulo_id IS NOT NULL;
```

#### Paso 2.3: Validar Integridad de Datos

```sql
-- Verificar Foreign Keys
SELECT TABLE_NAME, CONSTRAINT_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
WHERE REFERENCED_TABLE_SCHEMA = 'agro_prod' AND REFERENCED_TABLE_NAME IS NOT NULL
ORDER BY TABLE_NAME;

-- Verificar índices
SHOW INDEX FROM producciones;
SHOW INDEX FROM s3_monitoring_producciones;

-- Verificar que no hay datos huérfanos
SELECT COUNT(*) as producciones_sin_articulo
FROM producciones WHERE articulo_id NOT IN (SELECT articulo_id FROM articulos);

SELECT COUNT(*) as producciones_sin_centro
FROM producciones WHERE centro_costo_id NOT IN (SELECT centro_costo_id FROM centros_costos);
```

---

### Fase 3: Build de Docker

#### Paso 3.1: Construir Imagen Docker
```bash
docker build \
  --tag agro-sentinel-worker:1.0.0 \
  --tag agro-sentinel-worker:latest \
  -f Dockerfile \
  .
```

Verificar:
```bash
docker images | grep agro-sentinel-worker
docker image inspect agro-sentinel-worker:1.0.0
```

#### Paso 3.2: Probar Imagen Localmente
```bash
# Iniciar contenedores
docker-compose up -d

# Esperar a que MySQL esté listo (10-15 segundos)
sleep 15

# Verificar logs
docker logs go-agro-sentinel-api
docker logs go-agro-sentinel-worker
docker logs go-agro-sentinel-sync

# Probar conectividad
curl http://localhost:8088/health
```

---

### Fase 4: Migración de Datos

#### Paso 4.1: Sincronización Inicial DynamoDB
```bash
# Ejecutar comando de sincronización
docker exec go-agro-sentinel-sync /usr/local/bin/sync \
  --config /etc/agro-sentinel/config.yaml \
  --mode full-sync

# Verificar resultado
aws dynamodb scan --table-name agro-sentinel-scenes \
  --endpoint-url http://localhost:4566 \
  --select COUNT
```

#### Paso 4.2: Verificar Sincronización en S3
```bash
# Listar objetos en bucket S3
aws s3 ls s3://agro-sentinel-data \
  --recursive \
  --endpoint-url http://localhost:4566

# Contar objetos
aws s3 ls s3://agro-sentinel-data \
  --recursive \
  --endpoint-url http://localhost:4566 | wc -l
```

---

### Fase 5: Configuración de Servicios

#### Paso 5.1: Configurar systemd (Linux)
```bash
sudo tee /etc/systemd/system/agro-sentinel-api.service > /dev/null <<EOF
[Unit]
Description=Agro Sentinel API Service
After=network.target docker.service
Requires=docker.service

[Service]
Type=exec
ExecStart=docker run --name agro-sentinel-api \\
  --network host \\
  -e CONFIG_PATH=/etc/agro-sentinel/config.yaml \\
  -e MYSQL_HOST=localhost \\
  -e MYSQL_USER=agro_admin \\
  -e MYSQL_PASSWORD=secure_password_here \\
  -v /etc/agro-sentinel:/etc/agro-sentinel \\
  agro-sentinel-worker:1.0.0 api

Restart=always
RestartSec=10s

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable agro-sentinel-api.service
sudo systemctl start agro-sentinel-api.service
```

#### Paso 5.2: Verificar Estado de Servicios
```bash
sudo systemctl status agro-sentinel-api.service
sudo journalctl -u agro-sentinel-api.service -f
```

---

### Fase 6: Verificación de Deployment

#### Paso 6.1: Health Checks
```bash
# Verificar API
curl -v http://localhost:8088/health

# Verificar base de datos
mysql -u agro_admin -p agro_prod -e "SELECT 1;"

# Verificar S3
aws s3 ls --endpoint-url http://localhost:4566

# Verificar SQS
aws sqs list-queues --endpoint-url http://localhost:4566

# Verificar DynamoDB
aws dynamodb list-tables --endpoint-url http://localhost:4566
```

#### Paso 6.2: Logs
```bash
# Revisar logs de API
docker logs go-agro-sentinel-api | tail -50

# Revisar logs de Worker
docker logs go-agro-sentinel-worker | tail -50

# Buscar errores
docker logs go-agro-sentinel-api | grep -i error
docker logs go-agro-sentinel-worker | grep -i error
```

---

## Validación Post-Deployment

### 1. Queries de Verificación

#### Validar Tablas Creadas
```sql
-- Contar registros por tabla
SELECT 'articulos' as tabla, COUNT(*) as total FROM articulos
UNION ALL
SELECT 'centros_costos', COUNT(*) FROM centros_costos
UNION ALL
SELECT 'producciones', COUNT(*) FROM producciones
UNION ALL
SELECT 'zonas_producciones', COUNT(*) FROM zonas_producciones
UNION ALL
SELECT 'asignaciones_zonas_producciones', COUNT(*) FROM asignaciones_zonas_producciones
UNION ALL
SELECT 's3_monitoring_escena_ia_resumen', COUNT(*) FROM s3_monitoring_escena_ia_resumen;
```

#### Verificar Integridad de Relaciones
```sql
-- Producciones con artículos válidos
SELECT COUNT(*) as producciones_ok
FROM producciones p
WHERE p.articulo_id IN (SELECT articulo_id FROM articulos);

-- Producciones con centros válidos
SELECT COUNT(*) as centros_ok
FROM producciones p
WHERE p.centro_costo_id IN (SELECT centro_costo_id FROM centros_costos);

-- Zonas con centros válidos
SELECT COUNT(*) as zonas_ok
FROM zonas_producciones z
WHERE z.centro_costo_id IN (SELECT centro_costo_id FROM centros_costos)
OR z.centro_costo_id IS NULL;
```

#### Verificar Índices
```sql
-- Listar índices por tabla
SELECT TABLE_NAME, INDEX_NAME, COLUMN_NAME, SEQ_IN_INDEX
FROM INFORMATION_SCHEMA.STATISTICS
WHERE TABLE_SCHEMA = 'agro_prod'
ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX;
```

### 2. Tests de Funcionalidad

#### Test: Crear Nueva Producción
```bash
cat << 'EOF' | mysql -u agro_admin -p agro_prod
-- Insertar nueva producción de prueba
INSERT INTO producciones (
  folio, articulo_id, fecha, hora, usuario, 
  estatus, aplicado, cantidad, centro_costo_id, monitoring
) VALUES (
  'TEST-' || DATE_FORMAT(NOW(), '%Y%m%d%H%i%s'),
  1,
  CURDATE(),
  CURTIME(),
  'test-user',
  'N',
  'S',
  2.50,
  1,
  1
);

-- Verificar inserción
SELECT * FROM producciones WHERE folio LIKE 'TEST-%' ORDER BY fecha DESC LIMIT 1;
EOF
```

#### Test: Verificar Relaciones
```bash
mysql -u agro_admin -p agro_prod << 'EOF'
-- Producción con sus datos relacionados
SELECT 
  p.folio,
  a.nombre as cultivo,
  c.nombre as rancho,
  z.nombre as zona,
  p.cantidad,
  p.monitoring
FROM producciones p
LEFT JOIN articulos a ON p.articulo_id = a.articulo_id
LEFT JOIN centros_costos c ON p.centro_costo_id = c.centro_costo_id
LEFT JOIN asignaciones_zonas_producciones azp ON p.produccion_id = azp.produccion_id
LEFT JOIN zonas_producciones z ON azp.zona_produccion_id = z.zona_produccion_id
WHERE p.monitoring = 1
LIMIT 5;
EOF
```

#### Test: API Health Check
```bash
# Obtener estado de salud
curl -s http://localhost:8088/health | jq '.'

# Verificar respuesta esperada
curl -s http://localhost:8088/health | jq '.status' | grep -q "healthy" && echo "OK" || echo "FAIL"
```

### 3. Performance Checks

#### Medir Tiempo de Queries
```sql
-- Mostrar tiempo de ejecución
SET @start = NOW(6);

SELECT COUNT(*) FROM producciones WHERE monitoring = 1;

SELECT TIMESTAMPDIFF(MICROSECOND, @start, NOW(6)) as microseconds;

-- Explicar plan de query
EXPLAIN SELECT * FROM producciones WHERE monitoring = 1;

EXPLAIN SELECT * FROM s3_monitoring_producciones 
  WHERE centro_costo_id = 1 AND monitoring = 1;
```

#### Verificar Uso de Índices
```sql
-- Verificar que se usan los índices correctos
EXPLAIN SELECT * FROM producciones 
WHERE centro_costo_id = 1 AND monitoring = 1;

-- Debe usar índice de centros_costos
EXPLAIN SELECT * FROM s3_monitoring_producciones 
WHERE centro_costo_id = 1;

-- Debe usar índice compuesto
EXPLAIN SELECT * FROM s3_monitoring_producciones 
WHERE centro_costo_id = 1 AND monitoring = 1;
```

### 4. Monitoreo Recomendado

#### Metricas a Monitorear
- [ ] **CPU Usage**: Debe ser < 80%
- [ ] **Memory Usage**: Debe ser < 85%
- [ ] **Disk Space**: Debe ser > 10% libre
- [ ] **MySQL Connections**: Debe ser < 80% del máximo
- [ ] **Response Time**: Debe ser < 500ms para 95th percentile
- [ ] **Error Rate**: Debe ser < 1%
- [ ] **DynamoDB Throughput**: Verificar WCU y RCU usage
- [ ] **S3 Requests**: Verificar tasa de lecturas/escrituras

#### Comandos de Monitoreo
```bash
# Monitorear CPU y memoria
top -u mysql

# Monitorear conexiones MySQL
mysql -u agro_admin -p agro_prod -e "SHOW PROCESSLIST;"

# Monitorear tamaño de BD
mysql -u agro_admin -p agro_prod -e "
SELECT 
  TABLE_NAME,
  ROUND(((data_length + index_length) / 1024 / 1024), 2) as size_mb
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'agro_prod'
ORDER BY (data_length + index_length) DESC;
"

# Monitorear tamaño de volúmenes Docker
docker exec go-agro-sentinel-mysql du -sh /var/lib/mysql

# Monitorear conexiones a AWS
docker logs go-agro-sentinel-worker | grep -i "connected\|error" | tail -20
```

---

## Plan de Rollback

### Escenarios de Rollback

#### Escenario 1: Error en MySQL
**Síntomas:**
- Fallos de conexión a base de datos
- Errores de integridad de datos
- Tablas no existen

**Pasos de Rollback:**

```bash
# 1. Detener servicios
docker-compose down

# 2. Restaurar backup de MySQL
mysql -u root -p < backup_agro_$(date +%Y%m%d)_before_deployment.sql

# 3. Verificar integridad
mysql -u agro_admin -p agro_prod -e "SHOW TABLES;"
mysql -u agro_admin -p agro_prod -e "SELECT COUNT(*) FROM producciones;"

# 4. Reiniciar servicios
docker-compose up -d

# 5. Verificar estado
curl http://localhost:8088/health
```

#### Escenario 2: Error en Código/Docker

**Síntomas:**
- API no inicia
- Worker no procesa jobs
- Errores en logs

**Pasos de Rollback:**

```bash
# 1. Detener contenedores fallidos
docker-compose stop

# 2. Cambiar a versión anterior
git checkout HEAD~1
docker build --tag agro-sentinel-worker:rollback .

# 3. Actualizar docker-compose.yml (si es necesario)
# Cambiar imagen a agro-sentinel-worker:rollback

# 4. Reiniciar con versión anterior
docker-compose up -d

# 5. Verificar
docker logs go-agro-sentinel-api
curl http://localhost:8088/health
```

#### Escenario 3: Error de Datos/Sincronización

**Síntomas:**
- Datos inconsistentes en DynamoDB
- Errores en sincronización
- Pérdida de datos en S3

**Pasos de Rollback:**

```bash
# 1. Detener worker y sync
docker-compose stop worker sync

# 2. Restaurar datos en AWS (desde backup S3)
aws s3 cp s3://agro-sentinel-backups/latest/ . \
  --recursive \
  --endpoint-url http://localhost:4566

# 3. Sincronizar DynamoDB desde backup
aws dynamodb batch-write-item \
  --request-items file://dynamodb_backup.json \
  --endpoint-url http://localhost:4566

# 4. Reiniciar servicios
docker-compose up -d worker sync

# 5. Monitorear
docker logs -f go-agro-sentinel-sync
```

### Scripts de Rollback Automático

#### Script: Rollback Completo
```bash
#!/bin/bash
# rollback.sh - Revierte deployment completo

set -e

echo "=== AGRO SENTINEL ROLLBACK INICIADO ==="
echo "Hora: $(date)"

# 1. Detener servicios
echo "[1/5] Deteniendo servicios..."
docker-compose down

# 2. Cambiar código
echo "[2/5] Revirtiendo código..."
git reset --hard HEAD~1

# 3. Restaurar BD
echo "[3/5] Restaurando base de datos..."
LATEST_BACKUP=$(ls -t backup_agro_*.sql | head -1)
mysql -u root -p < $LATEST_BACKUP

# 4. Rebuil imagen
echo "[4/5] Rebuildeando imagen Docker..."
docker build --tag agro-sentinel-worker:rollback .

# 5. Reiniciar
echo "[5/5] Reiniciando servicios..."
docker-compose up -d

echo "=== ROLLBACK COMPLETADO ==="
echo "Verifica el estado:"
echo "  docker logs go-agro-sentinel-api"
echo "  curl http://localhost:8088/health"
```

#### Script: Rollback de BD Solo
```bash
#!/bin/bash
# rollback-db.sh - Revierte solo cambios de base de datos

set -e

BACKUP_FILE=${1:-$(ls -t backup_agro_*.sql | head -1)}

if [ ! -f "$BACKUP_FILE" ]; then
  echo "Error: Archivo de backup no encontrado: $BACKUP_FILE"
  exit 1
fi

echo "Restaurando desde: $BACKUP_FILE"

# Detener servicios
docker-compose stop api worker sync

# Restaurar
mysql -u root -p < "$BACKUP_FILE"

# Reiniciar
docker-compose start api worker sync

echo "Rollback de BD completado"
```

### Timeline de Rollback

| Fase | Acción | Tiempo |
|------|--------|--------|
| 1 | Detectar problema | Inmediato |
| 2 | Escalar a DBA/DevOps | 5 minutos |
| 3 | Detener servicios | 2 minutos |
| 4 | Restaurar backup | 10-30 minutos |
| 5 | Verificar integridad | 5 minutos |
| 6 | Reiniciar servicios | 3 minutos |
| 7 | Health checks | 5 minutos |
| **Total** | | **30-50 minutos** |

---

## Consideraciones Operacionales

### 1. Monitoreo de Performance

#### Métricas Críticas

```sql
-- Query para monitorear performance
SELECT
  TABLE_NAME,
  ROUND(DATA_LENGTH / 1024 / 1024, 2) as data_mb,
  ROUND(INDEX_LENGTH / 1024 / 1024, 2) as index_mb,
  TABLE_ROWS as rows,
  ROUND(DATA_LENGTH / TABLE_ROWS, 2) as avg_row_bytes
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'agro_prod'
ORDER BY DATA_LENGTH DESC;
```

#### Alertas Sugeridas

- [ ] **CPU > 80%** - Escalar a DevOps
- [ ] **Memoria > 85%** - Revisar queries lentas
- [ ] **Disk > 90%** - Aumentar espacio de almacenamiento
- [ ] **Conexiones MySQL > 80%** - Revisar pool de conexiones
- [ ] **Error Rate > 1%** - Revisar logs de aplicación
- [ ] **Response Time p95 > 1s** - Optimizar queries
- [ ] **DynamoDB Throttling** - Aumentar capacidad de escritura
- [ ] **S3 Errors > 5** - Verificar conectividad a AWS

### 2. Mantenimiento Recomendado

#### Mantenimiento Diario
```bash
# Monitorear logs
docker logs go-agro-sentinel-api | grep -i error | wc -l

# Verificar espacio en disco
df -h / | tail -1

# Verificar memoria
free -h

# Backup incremental
mysqldump -u agro_admin -p agro_prod \
  --single-transaction \
  > backup_daily_$(date +%Y%m%d).sql
```

#### Mantenimiento Semanal
```bash
# Optimizar tablas
mysql -u agro_admin -p agro_prod << EOF
OPTIMIZE TABLE articulos;
OPTIMIZE TABLE centros_costos;
OPTIMIZE TABLE producciones;
OPTIMIZE TABLE zonas_producciones;
OPTIMIZE TABLE asignaciones_zonas_producciones;
OPTIMIZE TABLE s3_monitoring_escena_ia_resumen;
EOF

# Limpiar logs antiguos
find /var/log/agro-sentinel -name "*.log" -mtime +30 -delete

# Revisión de indices
mysql -u agro_admin -p agro_prod << EOF
ANALYZE TABLE producciones;
ANALYZE TABLE s3_monitoring_producciones;
EOF
```

#### Mantenimiento Mensual
```bash
# Actualizar estadísticas
mysql -u agro_admin -p agro_prod << EOF
ANALYZE TABLE articulos;
ANALYZE TABLE centros_costos;
ANALYZE TABLE producciones;
ANALYZE TABLE zonas_producciones;
ANALYZE TABLE asignaciones_zonas_producciones;
EOF

# Revisar crecimiento de datos
mysql -u agro_admin -p agro_prod << EOF
SELECT 
  TABLE_NAME,
  ROUND(((data_length + index_length) / 1024 / 1024), 2) as size_mb
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'agro_prod'
ORDER BY (data_length + index_length) DESC;
EOF

# Backup completo
mysqldump -u agro_admin -p agro_prod \
  > backup_monthly_$(date +%Y%m%d).sql

# Verificar integridad
mysql -u agro_admin -p agro_prod << EOF
CHECK TABLE articulos;
CHECK TABLE centros_costos;
CHECK TABLE producciones;
EOF
```

### 3. Gestión de Versiones

#### Versionado Docker
```bash
# Build con tags
docker build \
  --tag agro-sentinel-worker:1.0.0 \
  --tag agro-sentinel-worker:1.0 \
  --tag agro-sentinel-worker:latest \
  .

# Empujar a registry
docker push your-registry/agro-sentinel-worker:1.0.0
docker push your-registry/agro-sentinel-worker:1.0
docker push your-registry/agro-sentinel-worker:latest
```

#### Versionado de Base de Datos
```bash
# Crear archivo de versión
echo "1.0.0" > /etc/agro-sentinel/db-version.txt

# Mantener histórico de scripts
mkdir -p /var/lib/agro-sentinel/db-migrations
cp scripts/phase1-create-tables.sql /var/lib/agro-sentinel/db-migrations/001-phase1.sql
cp scripts/phase2-update-s3-monitoring-producciones.sql /var/lib/agro-sentinel/db-migrations/002-phase2.sql
```

### 4. Seguridad

#### Credenciales
- [ ] Usar `.env` con variables seguras (no en git)
- [ ] Rotar contraseñas cada 90 días
- [ ] Usar AWS Secrets Manager para credentials
- [ ] Habilitar SSL/TLS para conexiones MySQL

#### Backups
- [ ] Encriptar backups en tránsito
- [ ] Almacenar backups en S3 con encriptación
- [ ] Probar restauración de backups mensualmente
- [ ] Mantener backups por mínimo 30 días

#### Acceso
- [ ] Limitar acceso a BD solo a servicios autorizados
- [ ] Usar IP whitelisting para acceso remoto
- [ ] Auditar accesos a datos sensibles
- [ ] Cambiar contraseñas administrativas regularmente

---

## Timeline y Estimaciones

### Timeline de Deployment (4-5 horas)

| Fase | Tarea | Tiempo | Crítica |
|------|-------|--------|---------|
| **Pre-Deployment** | | |
| | Verificación de código | 15 min | Sí |
| | Tests unitarios | 10 min | Sí |
| | Backup de BD | 20 min | Sí |
| **Base de Datos** | | |
| | Script init-mysql.sql | 5 min | Sí |
| | Script phase1 | 5 min | Sí |
| | Script phase2 | 10 min | Sí |
| | Validación de datos | 10 min | Sí |
| **Docker Build** | | |
| | Build de imagen | 10 min | Sí |
| | Test local | 5 min | Sí |
| **Migración** | | |
| | Sincronización DynamoDB | 15 min | No |
| | Verificación S3 | 5 min | No |
| **Post-Deployment** | | |
| | Health checks | 10 min | Sí |
| | Tests de funcionalidad | 20 min | Sí |
| | Monitoreo inicial | 15 min | No |
| **Total** | | **150-200 min** | |

### Estimación de Downtime

- **Downtime planeado**: 5-10 minutos (cambio de versión)
- **Downtime inesperado**: 30-50 minutos (si hay rollback)
- **Ventana de mantenimiento recomendada**: 2-3 horas (buffer incluido)

### Horarios Recomendados

- **No hacer deployment**: Viernes después de las 2 PM
- **Mejor horario**: Martes-Jueves, 10 AM - 12 PM
- **Horario para rollback**: 24/7 disponible

---

## Contactos y Escalación

### Equipo de Deployment

| Rol | Nombre | Email | Teléfono |
|-----|--------|-------|----------|
| **DevOps Lead** | [Tu Nombre] | [tu.email] | [tu.teléfono] |
| **DBA** | [Nombre DBA] | [email.dba] | [teléfono.dba] |
| **Backend Lead** | Daniel Rivera | drh.megafresh@gmail.com | [teléfono] |
| **Ops Manager** | [Nombre] | [email] | [teléfono] |

### Escalación de Problemas

#### Nivel 1: Equipo de DevOps
- Problemas con Docker
- Problemas de infraestructura
- Problemas de conectividad
- **Response time**: 15 minutos

#### Nivel 2: DBA
- Errores de base de datos
- Problemas de sincronización
- Corrupción de datos
- **Response time**: 30 minutos

#### Nivel 3: Backend Lead
- Errores de código
- Lógica de negocio incorrecta
- Problemas de performance
- **Response time**: 30 minutos

#### Nivel 4: Gerencia IT
- Decisiones de rollback
- Escalamiento de recursos
- Comunicación con usuarios finales
- **Response time**: 1 hora

### Comunicación Durante Deployment

#### Antes del Deployment
```
SUBJECT: [INFO] Mantenimiento Programado - Agro Sentinel Worker
FECHA: [DATE]
HORA: [TIME] - [TIME + 3 HORAS]
IMPACTO: Posible indisponibilidad de 5-10 minutos

Se realizará actualización de:
- Base de datos MySQL
- Imagen Docker
- Sincronización de datos en AWS

Cualquier pregunta contactar a [email]
```

#### Durante el Deployment
```
SUBJECT: [IN-PROGRESS] Deployment Agro Sentinel Worker
HORA INICIO: [TIME]
FASE ACTUAL: [PHASE]
ETA FINALIZACIÓN: [ETA]

No interrumpir servicios
Cualquier incidente contactar a [email]
```

#### Después del Deployment
```
SUBJECT: [COMPLETED] Deployment Agro Sentinel Worker
HORA FINALIZACIÓN: [TIME]
DURACIÓN: [DURATION]
ESTADO: [SUCCESS/FALLBACK/ISSUES]

Sistema operacional y probado
Gracias por su paciencia
```

---

## Checklist Final de Deployment

### Antes de Iniciar
- [ ] Todos los pre-requeritos completados
- [ ] Backups realizados y verificados
- [ ] Team de escalación disponible
- [ ] Ventana de mantenimiento comunicada
- [ ] Rollback plan revisado y aprobado

### Durante Deployment
- [ ] Monitoreo activo de logs
- [ ] Health checks pasando
- [ ] Métricas normales
- [ ] Documentar cualquier desviación

### Después de Deployment
- [ ] Tests de funcionalidad completados
- [ ] Monitoreo por 1 hora mínimo
- [ ] Feedback de usuarios
- [ ] Documentar lecciones aprendidas
- [ ] Actualizar runbook para próximos deployments

---

## Documentación de Referencia

- **Plan de Implementación**: `docs/IMPLEMENTATION_PLAN.md`
- **Especificación de Diseño**: `docs/DESIGN_SPECIFICATION.md`
- **README Principal**: `README.md`
- **Configuración Docker**: `docker-compose.yml`, `Dockerfile`
- **Scripts SQL**: `scripts/`
- **Código Fuente**: `cmd/`, `internal/`

---

**Documento versión 1.0**  
**Última actualización: 2026-09-04**  
**Próxima revisión: 2026-10-04**

Para preguntas o sugerencias sobre este plan, contactar a: drh.megafresh@gmail.com
