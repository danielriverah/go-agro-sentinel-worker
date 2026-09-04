# Operations Guide - Agro Sentinel Worker

**Versión:** 1.0  
**Objetivo:** Guía de operación y monitoreo post-deployment  
**Público:** DevOps, DBAs, Site Reliability Engineers  
**Última actualización:** 2026-09-04

---

## Tabla de Contenidos

1. [Daily Operations](#daily-operations)
2. [Monitoring & Alerting](#monitoring--alerting)
3. [Performance Tuning](#performance-tuning)
4. [Disaster Recovery](#disaster-recovery)
5. [Escalation Procedures](#escalation-procedures)

---

## Daily Operations

### Startup Checklist (Mañana)

```bash
#!/bin/bash
# morning-startup.sh

echo "=== AGRO SENTINEL - MORNING STARTUP CHECK ==="
echo "Hora: $(date)"
echo ""

# 1. Verificar Docker
echo "[1] Docker Status..."
docker ps -a | grep agro-sentinel || echo "WARNING: Containers no encontrados"

# 2. Health Check API
echo ""
echo "[2] API Health..."
if curl -s http://localhost:8088/health | jq . > /dev/null 2>&1; then
  echo "  ✓ API healthy"
else
  echo "  ✗ API unhealthy"
fi

# 3. MySQL Status
echo ""
echo "[3] MySQL Status..."
if mysql -u agro_admin -p agro_prod -e "SELECT 1" > /dev/null 2>&1; then
  mysql -u agro_admin -p agro_prod << EOF
  SELECT 'Tablas:' as info;
  SELECT COUNT(*) as total_tables FROM information_schema.TABLES 
  WHERE TABLE_SCHEMA = 'agro_prod';
  
  SELECT '' as info;
  SELECT 'Datos:' as info;
  SELECT 'articulos' as tabla, COUNT(*) as total FROM articulos
  UNION ALL
  SELECT 'centros_costos', COUNT(*) FROM centros_costos
  UNION ALL
  SELECT 'producciones', COUNT(*) FROM producciones;
EOF
else
  echo "  ✗ MySQL no accesible"
fi

# 4. Disk Space
echo ""
echo "[4] Disk Space..."
df -h / | awk 'NR==2 {print "  " $5 " usado, " $4 " disponible"}'

# 5. Container Resources
echo ""
echo "[5] Container Resources..."
docker stats --no-stream agro-sentinel-api agro-sentinel-mysql agro-sentinel-worker 2>/dev/null || true

echo ""
echo "=== READY FOR OPERATIONS ==="
```

### End-of-Day Checklist

```bash
#!/bin/bash
# end-of-day.sh

echo "=== AGRO SENTINEL - END OF DAY REPORT ==="
echo "Hora: $(date)"
echo ""

# 1. Recolectar logs
echo "[1] Recolectando logs..."
mkdir -p logs/$(date +%Y%m%d)
docker logs go-agro-sentinel-api > logs/$(date +%Y%m%d)/api.log 2>&1 || true
docker logs go-agro-sentinel-worker > logs/$(date +%Y%m%d)/worker.log 2>&1 || true
docker logs go-agro-sentinel-mysql > logs/$(date +%Y%m%d)/mysql.log 2>&1 || true

# 2. Estadísticas de BD
echo "[2] Estadísticas BD..."
mysql -u agro_admin -p agro_prod << EOF > logs/$(date +%Y%m%d)/db_stats.txt
SELECT DATE(NOW()) as fecha, TIME(NOW()) as hora;
SELECT 'Database Size' as metric;
SELECT
  SUM(ROUND(((data_length + index_length) / 1024 / 1024), 2)) as total_mb
FROM information_schema.TABLES
WHERE TABLE_SCHEMA = 'agro_prod';

SELECT 'Table Counts' as metric;
SELECT 'articulos' as tabla, COUNT(*) as total FROM articulos
UNION ALL
SELECT 'centros_costos', COUNT(*) FROM centros_costos
UNION ALL
SELECT 'producciones', COUNT(*) FROM producciones
UNION ALL
SELECT 'zonas_producciones', COUNT(*) FROM zonas_producciones;
EOF

# 3. Errores en logs
echo "[3] Analizando errores..."
echo "Errores en API:" >> logs/$(date +%Y%m%d)/summary.txt
docker logs go-agro-sentinel-api | grep -i error | wc -l >> logs/$(date +%Y%m%d)/summary.txt

echo "Errores en Worker:" >> logs/$(date +%Y%m%d)/summary.txt
docker logs go-agro-sentinel-worker | grep -i error | wc -l >> logs/$(date +%Y%m%d)/summary.txt

# 4. Backup
echo "[4] Backup diario..."
mysqldump -u agro_admin -p agro_prod \
  > backups/daily_$(date +%Y%m%d).sql

echo ""
echo "=== REPORTE GUARDADO ==="
echo "Logs: logs/$(date +%Y%m%d)/"
echo "Backup: backups/daily_$(date +%Y%m%d).sql"
```

---

## Monitoring & Alerting

### 1. Sistema de Monitoreo

#### Métricas a Monitorear

```bash
#!/bin/bash
# monitoring-dashboard.sh
# Muestra dashboard de monitoreo en tiempo real

watch -n 5 '
clear
echo "=== AGRO SENTINEL MONITORING DASHBOARD ==="
echo "Hora: $(date)"
echo ""

echo "CONTAINERS:"
docker ps --format "table {{.Names}}\t{{.Status}}" | grep agro-sentinel
echo ""

echo "CPU & MEMORY:"
docker stats --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}" agro-sentinel-api agro-sentinel-mysql agro-sentinel-worker
echo ""

echo "DISK SPACE:"
df -h / | tail -1
echo ""

echo "MYSQL CONNECTIONS:"
mysql -u agro_admin -p agro_prod -e "SHOW STATUS WHERE variable_name = \"Threads_connected\";" 2>/dev/null || echo "N/A"
echo ""

echo "API STATUS:"
curl -s http://localhost:8088/health | jq ".status" 2>/dev/null || echo "DOWN"
'
```

#### Alertas Sugeridas

**Nivel 1 - Critical (Actuar inmediatamente)**
- API no responde
- MySQL down
- Disk full (>95%)
- CPU > 90% por >10 min
- Memory > 90% por >10 min

**Nivel 2 - Warning (Investigar en 1 hora)**
- CPU > 80% por >5 min
- Memory > 85% por >5 min
- Response time > 1s
- Error rate > 1%
- Restarting containers

**Nivel 3 - Info (Log para análisis)**
- Database size aumentando >10% al mes
- Slow queries detectadas
- Índices no utilizados
- Oportunidades de optimización

### 2. Prometheus Metrics

#### Setup Prometheus (Opcional)

```yaml
# prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

alerting:
  alertmanagers:
    - static_configs:
        - targets:
            - localhost:9093

rule_files:
  - "alert_rules.yml"

scrape_configs:
  - job_name: 'agro-sentinel-api'
    metrics_path: '/metrics'
    static_configs:
      - targets: ['localhost:8088']

  - job_name: 'mysql'
    static_configs:
      - targets: ['localhost:9104']  # MySQL exporter
```

#### Alert Rules

```yaml
# alert_rules.yml
groups:
  - name: agro-sentinel
    rules:
      - alert: APIDown
        expr: up{job="agro-sentinel-api"} == 0
        for: 1m
        annotations:
          summary: "API Agro Sentinel está down"

      - alert: HighErrorRate
        expr: rate(errors_total[5m]) > 0.01
        for: 5m
        annotations:
          summary: "Error rate > 1%"

      - alert: HighLatency
        expr: histogram_quantile(0.95, rate(request_duration_seconds_bucket[5m])) > 1
        annotations:
          summary: "p95 latency > 1 segundo"

      - alert: DiskSpaceLow
        expr: node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"} < 0.1
        annotations:
          summary: "Espacio en disco < 10%"
```

### 3. Grafana Dashboard

#### Dashboard JSON (Configuración Básica)

```json
{
  "dashboard": {
    "title": "Agro Sentinel Worker",
    "panels": [
      {
        "title": "API Request Rate",
        "targets": [
          {
            "expr": "rate(http_requests_total[5m])"
          }
        ]
      },
      {
        "title": "API Response Time",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, http_request_duration_seconds_bucket)"
          }
        ]
      },
      {
        "title": "MySQL Connections",
        "targets": [
          {
            "expr": "mysql_global_status_threads_connected"
          }
        ]
      },
      {
        "title": "Database Size",
        "targets": [
          {
            "expr": "mysql_info_schema_tables_size_bytes"
          }
        ]
      }
    ]
  }
}
```

---

## Performance Tuning

### 1. MySQL Optimization

#### Verificar Slow Queries

```bash
# Habilitar slow query log
mysql -u root -p << EOF
SET GLOBAL slow_query_log = 'ON';
SET GLOBAL long_query_time = 2;
SET GLOBAL log_queries_not_using_indexes = 'ON';
EOF

# Ver queries lentas
tail -f /var/lib/mysql/slow.log
```

#### Analizar Queries

```sql
-- Top 10 queries lentas
SELECT
  SUM(Count_star) as calls,
  AVG(Avg_timer_wait)/1000000000000 as avg_time_sec,
  SUM(Sum_timer_wait)/1000000000000 as total_time_sec,
  DIGEST_TEXT as query
FROM performance_schema.events_statements_summary_by_digest
ORDER BY total_time_sec DESC
LIMIT 10;
```

#### Indices Optimization

```sql
-- Encontrar índices no utilizados
SELECT
  object_schema,
  object_name,
  index_name
FROM performance_schema.table_io_waits_summary_by_index_usage
WHERE index_name != 'PRIMARY' AND count_star = 0
ORDER BY object_schema, object_name;

-- Índices redundantes
SELECT
  a.table_schema,
  a.table_name,
  a.index_name,
  GROUP_CONCAT(a.column_name ORDER BY a.seq_in_index) as index_columns,
  b.index_name as redundant_index,
  GROUP_CONCAT(b.column_name ORDER BY b.seq_in_index) as redundant_columns
FROM information_schema.statistics a
JOIN information_schema.statistics b ON a.table_schema = b.table_schema
  AND a.table_name = b.table_name
  AND a.column_name = b.column_name
  AND a.seq_in_index = b.seq_in_index
WHERE a.index_name != b.index_name
  AND a.table_schema != 'mysql'
GROUP BY a.table_schema, a.table_name, a.index_name, b.index_name;
```

#### Actualizar Table Statistics

```sql
-- Forzar recálculo de estadísticas
ANALYZE TABLE articulos;
ANALYZE TABLE centros_costos;
ANALYZE TABLE producciones;
ANALYZE TABLE zonas_producciones;
ANALYZE TABLE asignaciones_zonas_producciones;
ANALYZE TABLE s3_monitoring_escena_ia_resumen;
```

### 2. Docker Optimization

#### Límites de Recursos

```yaml
# docker-compose.yml (actualizado)
services:
  api:
    mem_limit: 512m
    memswap_limit: 512m
    cpus: 1.0

  mysql:
    mem_limit: 1024m
    memswap_limit: 1024m
    cpus: 2.0

  worker:
    mem_limit: 256m
    memswap_limit: 256m
    cpus: 0.5
```

#### Monitoreo de Recursos

```bash
# Ver límites y uso actual
docker stats --no-stream --format "table {{.Container}}\t{{.MemPerc}}\t{{.MemLimit}}\t{{.CPUPerc}}"

# Obtener detalles
docker inspect go-agro-sentinel-api | jq '.[] | .HostConfig | {Memory, MemorySwap, CpuQuota}'
```

### 3. Application Tuning

#### Connection Pool Tuning

```yaml
# En config.yaml
database:
  host: localhost
  max_connections: 20
  max_idle_connections: 5
  connection_max_lifetime: 5m
  connection_max_idle_time: 2m

aws:
  max_retries: 3
  timeout: 30s
  client_pool_size: 10
```

#### Cache Configuration

```yaml
# En config.yaml
cache:
  enabled: true
  ttl: 5m
  max_size: 1000
  cleanup_interval: 1m

dynamodb:
  batch_write_capacity: 25
  batch_read_capacity: 100
```

---

## Disaster Recovery

### 1. Recovery Procedures

#### Procedimiento: Restaurar desde Backup

```bash
#!/bin/bash
# recover-from-backup.sh

BACKUP_FILE=${1:-backup_latest.sql}

if [ ! -f "$BACKUP_FILE" ]; then
  echo "Error: Backup no encontrado: $BACKUP_FILE"
  exit 1
fi

echo "Restaurando desde: $BACKUP_FILE"

# 1. Detener servicios
docker-compose stop

# 2. Restaurar
mysql -u root -p < "$BACKUP_FILE"

# 3. Verificar
mysql -u agro_admin -p agro_prod -e "SELECT COUNT(*) FROM producciones;"

# 4. Reiniciar
docker-compose start

echo "Restore completado"
```

#### Procedimiento: Punto de Recuperación

```bash
#!/bin/bash
# point-in-time-recovery.sh

# MySQL Point-in-Time Recovery (PITR)
# Requiere binary logs habilitados

TARGET_TIME="2026-09-04 14:00:00"

echo "Recuperando estado en: $TARGET_TIME"

# 1. Restaurar backup anterior a TARGET_TIME
mysql -u root -p < backup_before_target.sql

# 2. Aplicar binary logs hasta TARGET_TIME
mysqlbinlog /var/lib/mysql/bin-log.000001 \
  --stop-datetime="$TARGET_TIME" | \
  mysql -u root -p

echo "Point-in-time recovery completado"
```

### 2. Backup Strategies

#### Backup Diario Automático

```bash
#!/bin/bash
# backup-scheduler.sh
# Cron: 0 2 * * * /path/to/backup-scheduler.sh

BACKUP_DIR="backups/$(date +%Y/%m)"
mkdir -p "$BACKUP_DIR"

BACKUP_FILE="$BACKUP_DIR/agro_prod_$(date +%Y%m%d_%H%M%S).sql"

mysqldump -u agro_admin -p agro_prod \
  --single-transaction \
  --routines \
  --events \
  --quick \
  > "$BACKUP_FILE"

# Comprimir
gzip "$BACKUP_FILE"

# Limpiar backups antiguos (>30 días)
find backups -name "*.sql.gz" -mtime +30 -delete

echo "Backup completado: $BACKUP_FILE.gz"
```

#### Verificar Integridad de Backups

```bash
#!/bin/bash
# verify-backups.sh

BACKUP_FILE=${1:-$(ls -t backups/*.sql.gz | head -1)}

echo "Verificando: $BACKUP_FILE"

# 1. Descomprimir
gunzip -c "$BACKUP_FILE" | head -100 | grep -q "CREATE TABLE" || {
  echo "ERROR: Backup parece corrupto"
  exit 1
}

# 2. Verificar tamaño
SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
echo "Tamaño: $SIZE"

# 3. Contar tablas
TABLES=$(gunzip -c "$BACKUP_FILE" | grep -c "CREATE TABLE")
echo "Tablas: $TABLES"

if [ "$TABLES" -ge 6 ]; then
  echo "✓ Backup válido"
else
  echo "✗ Backup inválido (muy pocas tablas)"
  exit 1
fi
```

### 3. Disaster Recovery Plan

| Escenario | RTO | RPO | Procedure |
|-----------|-----|-----|-----------|
| MySQL down | 10 min | 1 hour | Restart container, restore from daily backup |
| Data corruption | 30 min | 1 hour | Stop writes, restore from backup, verify |
| Complete loss | 2 hours | Daily | Restore from off-site backup, resync AWS |
| Code bug | 15 min | Real-time | Rollback git, rebuild Docker |
| Network issue | 30 min | N/A | Failover to DR site |

---

## Escalation Procedures

### 1. Escalation Matrix

```
┌─────────────────────────────────────────────────────────────┐
│                    ESCALATION MATRIX                         │
├─────────────────────────────────────────────────────────────┤
│ ISSUE TYPE        │ LEVEL 1  │ LEVEL 2     │ LEVEL 3         │
├─────────────────────────────────────────────────────────────┤
│ API Down (5m)     │ DevOps   │ Backend     │ CTO (if >1h)    │
│ MySQL Down        │ DBA      │ DevOps      │ DBA Manager     │
│ Disk Full         │ DevOps   │ Infra Mgr   │ CTO             │
│ Data Corruption   │ DBA      │ Backup Mgr  │ Data Manager    │
│ Slow Performance  │ Backend  │ DBA         │ Performance Eng │
│ Security Issue    │ Security │ CTO         │ CISO            │
└─────────────────────────────────────────────────────────────┘
```

### 2. On-Call Procedures

#### Incoming Alert

1. **Acknowledge** (dentro de 5 minutos)
   - Responder a alert
   - Confirmar que se está investigando

2. **Investigate** (dentro de 15 minutos)
   - Revisar logs
   - Ejecutar health checks
   - Determinar severity

3. **Escalate if needed** (dentro de 30 minutos)
   - Contactar Level 2 si no se puede resolver
   - Proporcionar contexto y logs

4. **Remediate** (según severity)
   - Critical: Actuar inmediatamente
   - Warning: Investigar y planificar fix
   - Info: Log para análisis posterior

5. **Communicate** (durante y después)
   - Mantener stakeholders informados
   - Post-mortem si es incident

---

## Runbooks Rápidos

### Runbook: API No Responde

```bash
# 1. Verificar si está running
docker ps | grep api

# 2. Revisar logs
docker logs -f go-agro-sentinel-api | tail -50

# 3. Revisar errores
docker logs go-agro-sentinel-api | grep -i error | tail -10

# 4. Intentar reiniciar
docker-compose restart api
sleep 10

# 5. Verificar health
curl -v http://localhost:8088/health

# 6. Si no funciona, escalar a Backend
```

### Runbook: MySQL Lento

```bash
# 1. Ver conexiones
mysql -u agro_admin -p agro_prod -e "SHOW PROCESSLIST;" | head -20

# 2. Ver queries lentas
mysql -u agro_admin -p agro_prod -e "
  SELECT
    Digest_text,
    Count_star,
    Avg_timer_wait/1000000000000 as avg_seconds
  FROM performance_schema.events_statements_summary_by_digest
  ORDER BY Avg_timer_wait DESC
  LIMIT 5;
"

# 3. Optimizar índices
mysql -u agro_admin -p agro_prod -e "ANALYZE TABLE producciones; OPTIMIZE TABLE producciones;"

# 4. Monitorear mejoría
watch -n 5 'mysql -u agro_admin -p agro_prod -e "SHOW STATUS LIKE \"Threads_connected\";"'

# 5. Si no funciona, escalar a DBA
```

### Runbook: Disk Full

```bash
# 1. Ver uso
df -h /
du -sh /var/lib/docker/volumes/*

# 2. Limpiar logs antiguos
find /var/lib/docker/containers -name '*.log' -delete

# 3. Limpiar Docker
docker system prune -a

# 4. Limpiar backups antiguos
find backups -type f -mtime +30 -delete

# 5. Si sigue lleno, escalar a Infra
```

---

## Contacts & Support

**24/7 Support Contact:** [team-slack-channel]  
**Email:** ops-team@company.com  
**Escalation:** [escalation-phone-number]

---

**Última actualización:** 2026-09-04  
**Próxima revisión:** 2026-10-04
