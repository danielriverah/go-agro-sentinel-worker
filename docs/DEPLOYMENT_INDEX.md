# Índice de Documentación de Deployment - Agro Sentinel Worker

**Versión:** 1.0  
**Estado:** Production Ready  
**Última actualización:** 2026-09-04  
**Mantenedor:** Daniel Rivera Herrada (drh.megafresh@gmail.com)

---

## Documentos Principales

### 1. DEPLOYMENT_PLAN.md - Plan Maestro
**Tiempo de lectura:** 30 minutos  
**Público:** Gerentes, DevOps Leads, Architects  
**Contenido:**
- Checklist completo de pre-deployment
- Pasos detallados de deployment en 6 fases
- Validaciones post-deployment
- Plan de rollback por escenario
- Consideraciones operacionales
- Timeline y estimaciones
- Contactos y escalación

**Cuándo usarlo:**
- Planificar deployment completo
- Preparar equipo
- Revisar requisitos
- Entender timeline

---

### 2. DEPLOYMENT_RUNBOOK.md - Guía Operativa
**Tiempo de lectura:** 15 minutos (referencia rápida)  
**Público:** DevOps Engineers, SREs, On-Call Staff  
**Contenido:**
- Scripts listos para usar
- Procedimientos paso-a-paso
- Troubleshooting
- Validación exhaustiva
- Comandos útiles rápidos

**Cuándo usarlo:**
- Durante deployment real
- Para troubleshooting
- Ejecutar scripts
- Validaciones rápidas

---

### 3. OPERATIONS_GUIDE.md - Manual de Operación
**Tiempo de lectura:** 20 minutos (referencia)  
**Público:** DevOps, DBAs, Site Reliability Engineers  
**Contenido:**
- Checklists diarios
- Monitoreo y alerting
- Performance tuning
- Disaster recovery
- Escalation procedures
- Runbooks de emergencia

**Cuándo usarlo:**
- Operación diaria
- Monitoreo del sistema
- Optimización
- Disaster recovery

---

## Matriz de Decisión Rápida

```
┌─────────────────────────────────────────────────────────────────┐
│ SITUACIÓN                      │ DOCUMENTO           │ ACCIÓN    │
├─────────────────────────────────────────────────────────────────┤
│ Planificar deployment          │ DEPLOYMENT_PLAN     │ Leer 1.0  │
│ Pre-deployment checklist       │ DEPLOYMENT_PLAN     │ §1        │
│ Ejecutar deployment real       │ DEPLOYMENT_RUNBOOK  │ §Proced.  │
│ API no responde                │ OPERATIONS_GUIDE    │ §Runbook  │
│ MySQL lento                    │ OPERATIONS_GUIDE    │ §Tuning   │
│ Problema de espacio            │ OPERATIONS_GUIDE    │ §Runbook  │
│ Necesito hacer rollback        │ DEPLOYMENT_PLAN     │ §4        │
│ Configurar monitoreo           │ OPERATIONS_GUIDE    │ §2        │
│ Backup/Recovery                │ OPERATIONS_GUIDE    │ §3        │
│ Escalación de issue            │ OPERATIONS_GUIDE    │ §5        │
└─────────────────────────────────────────────────────────────────┘
```

---

## Quick Start (5 minutos)

### Para Deployment Rápido
```bash
cd /path/to/go-agro-sentinel-worker

# 1. Ver requisitos
head -100 docs/DEPLOYMENT_PLAN.md | grep -A 5 "Pre-Deployment"

# 2. Ejecutar validaciones
./scripts/pre-deploy-check.sh

# 3. Hacer deployment
./scripts/complete-deployment.sh
```

### Para Operación Diaria
```bash
# Morning
./scripts/morning-startup.sh

# Check issues
docker logs go-agro-sentinel-api | grep -i error

# End of day
./scripts/end-of-day.sh
```

### Para Emergencias
```bash
# API Down
docker logs go-agro-sentinel-api

# MySQL issues
./scripts/health-check.sh

# Need rollback
./scripts/rollback.sh
```

---

## Componentes del Sistema

### Servicios Deployados

| Servicio | Puerto | Dependencia | Crítico |
|----------|--------|-------------|---------|
| **API** | 8088 | MySQL, AWS | Sí |
| **Worker** | N/A | MySQL, SQS | Sí |
| **Sync** | N/A | MySQL, DynamoDB, S3 | No |
| **MySQL** | 3309 | N/A | Sí |
| **LocalStack** | 4566 | N/A | Sí (dev) |

### Databases & Storage

| Nombre | Tipo | Rol |
|--------|------|-----|
| `agro_prod` | MySQL | Datos operacionales |
| `agro-sentinel-scenes` | DynamoDB | Cache distribuido |
| `agro-sentinel-data` | S3 | Almacenamiento de imágenes |
| `agro-sentinel-jobs` | SQS | Queue de trabajos |

### Archivos Clave

```
go-agro-sentinel-worker/
├── docs/
│   ├── DEPLOYMENT_INDEX.md          ← Estás aquí
│   ├── DEPLOYMENT_PLAN.md           ← Plan completo
│   ├── DEPLOYMENT_RUNBOOK.md        ← Guía operativa
│   └── OPERATIONS_GUIDE.md          ← Manual de ops
├── scripts/
│   ├── pre-deploy-check.sh
│   ├── backup-all.sh
│   ├── migrate-db.sh
│   ├── build-and-test.sh
│   ├── health-check.sh
│   └── rollback.sh
├── docker-compose.yml               ← Configuración services
├── Dockerfile                       ← Build image
├── .env.example                     ← Template env vars
└── configs/
    ├── config.yaml                  ← Config producción
    └── config.example.yaml          ← Template config
```

---

## Scripts Disponibles

### Pre-Deployment Scripts

```bash
# Validación completa
./scripts/pre-deploy-check.sh

# Backup de datos
./scripts/backup-all.sh

# Build y test Docker
./scripts/build-and-test.sh 1.0.0
```

### Deployment Scripts

```bash
# Migraciones de BD
./scripts/migrate-db.sh

# Validación post-deployment
./scripts/health-check.sh

# Deployment completo
./scripts/complete-deployment.sh
```

### Operation Scripts

```bash
# Morning startup
./scripts/morning-startup.sh

# End of day report
./scripts/end-of-day.sh

# Rollback completo
./scripts/rollback.sh
```

---

## Checklist de Preparación

### Antes de Leer Documentación
- [ ] Acceso a repositorio Git
- [ ] Acceso a Docker
- [ ] Acceso a MySQL
- [ ] Credenciales AWS
- [ ] Permisos en servidor destino

### Antes de Deployment
- [ ] Leer DEPLOYMENT_PLAN.md completo
- [ ] Ejecutar pre-deployment checklist
- [ ] Crear backups
- [ ] Notificar a stakeholders
- [ ] Equipo en standby

### Después de Deployment
- [ ] Ejecutar validaciones post-deployment
- [ ] Monitorear por 1 hora
- [ ] Documentar issues
- [ ] Actualizar runbooks si es necesario

---

## Timeframes

### Deployment Completo: 3-4 horas

```
[Lectura] 30 min → DEPLOYMENT_PLAN.md
    ↓
[Pre-checks] 20 min → scripts/pre-deploy-check.sh
    ↓
[Backup] 20 min → scripts/backup-all.sh
    ↓
[Database] 15 min → scripts/migrate-db.sh
    ↓
[Docker Build] 15 min → scripts/build-and-test.sh
    ↓
[Deploy] 30 min → docker-compose up -d
    ↓
[Validation] 30 min → scripts/health-check.sh
    ↓
[Monitoring] 60 min → Observar y responder issues
```

### Operación Diaria: 30 minutos

```
[Morning] 10 min → ./scripts/morning-startup.sh
    ↓
[Daily Ops] 15 min → Monitoreo y respuestas
    ↓
[Evening] 5 min → ./scripts/end-of-day.sh
```

### Rollback: 30-50 minutos

```
[Detection] 5 min → Identificar problema
    ↓
[Escalation] 5 min → Contactar lead
    ↓
[Stop Services] 2 min → docker-compose down
    ↓
[Restore] 20-30 min → Restaurar from backup
    ↓
[Verify] 5 min → health-check.sh
    ↓
[Restart] 3 min → docker-compose up -d
```

---

## Variables de Entorno Requeridas

```bash
# MySQL
MYSQL_HOST=localhost
MYSQL_USER=agro_admin
MYSQL_PASSWORD=secure_password_here
MYSQL_DATABASE=agro_prod

# AWS (Producción)
AWS_REGION=us-east-1
AWS_ACCESS_KEY_ID=your_access_key
AWS_SECRET_ACCESS_KEY=your_secret_key
AWS_ENDPOINT_URL=https://s3.amazonaws.com  # O local URL

# Application
LOG_LEVEL=INFO
DEBUG=false
CONFIG_PATH=/etc/agro-sentinel/config.yaml

# Optional (Desarrollo)
SQS_QUEUE_URL=http://localhost:4566/000000000000/agro-sentinel-jobs
DYNAMODB_TABLE=agro-sentinel-scenes
S3_BUCKET=agro-sentinel-data
```

---

## Métricas de Éxito

| Métrica | Target | Cómo Verificar |
|---------|--------|----------------|
| API Health | 100% | `curl http://localhost:8088/health` |
| DB Connectivity | 100% | `mysql ... -e "SELECT 1;"` |
| Response Time p95 | <500ms | Prometheus metrics |
| Error Rate | <1% | Application logs |
| Disk Space Free | >20% | `df -h /` |
| MySQL Connections | <80% max | `SHOW STATUS` |

---

## Escalation Tree

```
Detectar Issue
    ↓
Level 1: DevOps/SRE (5 min)
    ├─ Problema Docker → Restart container
    ├─ Problema BD → Restart MySQL
    └─ Problema disco → Cleanup
    ↓
Level 2: Backend/DBA (30 min)
    ├─ Código issue → Fix + rebuild
    ├─ BD lenta → Optimize queries
    └─ Data corruption → Restore backup
    ↓
Level 3: Management (1 hour)
    ├─ Major incident → Declare SEV
    ├─ Rollback needed → Approve
    └─ PR communication → Handle
```

---

## Common Issues & Solutions

| Issue | Documento | Sección |
|-------|-----------|---------|
| API no inicia | RUNBOOK | Troubleshooting #5 |
| MySQL connection refused | RUNBOOK | Troubleshooting #1 |
| Docker build falla | RUNBOOK | Troubleshooting #2 |
| FK constraint error | RUNBOOK | Troubleshooting #3 |
| Disk space | RUNBOOK | Troubleshooting #4 |
| API lenta | OPERATIONS | Performance Tuning |
| MySQL lento | OPERATIONS | MySQL Optimization |

---

## Recursos Externos

- **Docker Docs:** https://docs.docker.com/
- **MySQL Docs:** https://dev.mysql.com/doc/
- **AWS SDK:** https://aws.amazon.com/sdk-for-go/
- **GDAL:** https://gdal.org/

---

## Historial de Cambios

| Versión | Fecha | Cambios |
|---------|-------|---------|
| 1.0 | 2026-09-04 | Creación inicial |
| | | - DEPLOYMENT_PLAN.md |
| | | - DEPLOYMENT_RUNBOOK.md |
| | | - OPERATIONS_GUIDE.md |

---

## Preguntas Frecuentes

### P: ¿Cuánto tiempo toma deployment?
**R:** 3-4 horas para deployment completo. Puedes hacer update solo de código en 5 minutos.

### P: ¿Cuál es el downtime esperado?
**R:** 5-10 minutos de downtime planeado. Con rollback, 30-50 minutos.

### P: ¿Qué pasa si falla el deployment?
**R:** Ejecuta `./scripts/rollback.sh` para revertir a versión anterior.

### P: ¿Cómo hago backup?
**R:** Ejecuta `./scripts/backup-all.sh` antes de deployment.

### P: ¿Qué hago si MySQL está lento?
**R:** Ver OPERATIONS_GUIDE.md §2 - Performance Tuning.

### P: ¿Cómo configuro alertas?
**R:** Ver OPERATIONS_GUIDE.md §2 - Monitoring & Alerting.

### P: ¿Cuál es el RTO/RPO?
**R:** RTO: 10-30 min, RPO: 1 hora (con backups diarios).

---

## Support & Contact

**Para preguntas sobre deployment:**  
Email: drh.megafresh@gmail.com  
Slack: #agro-sentinel-ops  
Phone: [tu-teléfono]

**En emergencias (24/7):**  
On-Call: [teléfono on-call]  
Slack: @agro-sentinel-team

---

## Quick Reference Card

```
═══════════════════════════════════════════════════════════════
                  AGRO SENTINEL DEPLOYMENT
═══════════════════════════════════════════════════════════════

PRE-DEPLOYMENT:
  ./scripts/pre-deploy-check.sh
  ./scripts/backup-all.sh

DEPLOYMENT:
  ./scripts/migrate-db.sh
  ./scripts/build-and-test.sh
  docker-compose down && docker-compose up -d

VALIDATION:
  ./scripts/health-check.sh
  curl http://localhost:8088/health

ROLLBACK (si es necesario):
  ./scripts/rollback.sh

OPERATIONS:
  ./scripts/morning-startup.sh
  ./scripts/end-of-day.sh

EMERGENCY:
  docker logs go-agro-sentinel-api | tail -50
  ./scripts/health-check.sh
  Escalate to Level 2 if needed

═══════════════════════════════════════════════════════════════
```

---

**Imprime este documento y tenlo a mano durante deployment.**

**Última actualización:** 2026-09-04  
**Próxima revisión:** 2026-10-04
