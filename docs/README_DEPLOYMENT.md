# Documentación de Deployment - Agro Sentinel Worker

Bienvenido a la documentación integral de deployment del Sistema de Monitoreo Agro Sentinel. Esta carpeta contiene todo lo que necesitas para deployar, operar y mantener el sistema en producción.

## Acceso Rápido

### Por Rol

**👨‍💼 Managers/Leads**
- Leer: [`DEPLOYMENT_PLAN.md`](./DEPLOYMENT_PLAN.md)
- Tiempo: 30 minutos
- Aprenderás: Timeline, requisitos, riesgos, rollback plan

**🔧 DevOps/SREs**
- Leer: [`DEPLOYMENT_RUNBOOK.md`](./DEPLOYMENT_RUNBOOK.md)
- Tiempo: 15 minutos + ejecución
- Aprenderás: Scripts, procedimientos, troubleshooting

**📊 DBAs/Operations**
- Leer: [`OPERATIONS_GUIDE.md`](./OPERATIONS_GUIDE.md)
- Tiempo: 20 minutos + referencia
- Aprenderás: Monitoreo, performance tuning, disaster recovery

**🆘 On-Call/Emergency**
- Leer: [`DEPLOYMENT_INDEX.md`](./DEPLOYMENT_INDEX.md) §Quick Reference
- Tiempo: 2 minutos
- Aprenderás: Runbooks de emergencia, escalation

---

## Documentos Principales

### 📋 DEPLOYMENT_PLAN.md
Plan maestro de deployment. Contiene:
- Checklist pre-deployment (6 secciones)
- Pasos detallados de deployment (6 fases)
- Validaciones post-deployment
- Plan de rollback por escenario
- Consideraciones operacionales
- **Leer PRIMERO antes de cualquier deployment**

### 🚀 DEPLOYMENT_RUNBOOK.md
Guía operativa con scripts listos. Contiene:
- Scripts pre-deployment
- Scripts de database migration
- Scripts de Docker build
- Scripts de health check
- Scripts de rollback
- Procedimientos paso-a-paso
- Troubleshooting detallado

### 📊 OPERATIONS_GUIDE.md
Manual de operación diaria. Contiene:
- Checklists diarios (morning/end-of-day)
- Monitoreo y alerting
- Performance tuning (MySQL, Docker, App)
- Disaster recovery procedures
- Escalation procedures
- Runbooks de emergencia

### 🗂️ DEPLOYMENT_INDEX.md
Índice y referencia rápida. Contiene:
- Matriz de decisión rápida
- Quick start (5 minutos)
- Componentes del sistema
- Checklist de preparación
- Timeframes
- FAQ
- Quick reference card (imprimible)

---

## Quick Start (Elige tu caso)

### Caso 1: Voy a hacer deployment por primera vez
```bash
1. Lee DEPLOYMENT_PLAN.md (30 min)
2. Ejecuta ./scripts/pre-deploy-check.sh (5 min)
3. Ejecuta ./scripts/backup-all.sh (20 min)
4. Sigue DEPLOYMENT_RUNBOOK.md procedimientos (2 horas)
5. Valida con ./scripts/health-check.sh (10 min)
```

### Caso 2: Necesito actualizar solo el código
```bash
1. Lee DEPLOYMENT_RUNBOOK.md §Procedimiento 2 (5 min)
2. git pull origin feat/agro-sentinel-worker
3. docker-compose build
4. docker-compose restart
5. Verifica: curl http://localhost:8088/health
```

### Caso 3: Es una emergencia - API está down
```bash
1. Abre DEPLOYMENT_INDEX.md §Quick Reference
2. Ejecuta ./scripts/health-check.sh
3. Revisa: docker logs go-agro-sentinel-api
4. Intenta: docker-compose restart api
5. Si falla: Escalate a Backend Lead
```

### Caso 4: Sistema lento / Performance issue
```bash
1. Abre OPERATIONS_GUIDE.md §Performance Tuning
2. Ejecuta queries de análisis (MySQL section)
3. Actualiza índices según sea necesario
4. Monitorea con prometheus/grafana
5. Documenta cambios
```

### Caso 5: Necesito hacer rollback
```bash
1. Abre DEPLOYMENT_PLAN.md §Plan de Rollback
2. Identifica escenario
3. Ejecuta ./scripts/rollback.sh
4. Verifica: ./scripts/health-check.sh
5. Documentar root cause
```

---

## Scripts Principales

```bash
# Pre-deployment
./scripts/pre-deploy-check.sh       # ✓ Valida todo está listo
./scripts/backup-all.sh             # ✓ Crea backup completo

# Deployment
./scripts/migrate-db.sh             # ✓ Ejecuta migraciones BD
./scripts/build-and-test.sh 1.0.0   # ✓ Build y test Docker
./scripts/health-check.sh           # ✓ Valida salud del sistema

# Operación
./scripts/morning-startup.sh        # ✓ Morning checklist
./scripts/end-of-day.sh             # ✓ End of day report

# Emergencia
./scripts/rollback.sh               # ✓ Revertir deployment
```

---

## Timeframes

| Tarea | Tiempo | Documento |
|-------|--------|-----------|
| Leer plan completo | 30 min | DEPLOYMENT_PLAN |
| Preparación (backups, checks) | 45 min | DEPLOYMENT_PLAN §Pre-Deployment |
| Database migration | 15 min | DEPLOYMENT_RUNBOOK §Fase 2 |
| Docker build | 15 min | DEPLOYMENT_RUNBOOK §Fase 3 |
| Deployment + validation | 60 min | DEPLOYMENT_RUNBOOK §Fase 4-5 |
| Post-deployment monitoring | 60 min | OPERATIONS_GUIDE §Daily Ops |
| **TOTAL DEPLOYMENT** | **3-4 horas** | |
| Code-only update | 5-10 min | DEPLOYMENT_RUNBOOK §Proced 2 |
| Emergency rollback | 30-50 min | DEPLOYMENT_PLAN §Rollback |

---

## Componentes del Sistema

```
┌──────────────────────────────────────────────────────┐
│         AGRO SENTINEL WORKER ARCHITECTURE            │
├──────────────────────────────────────────────────────┤
│                                                      │
│  ┌─────────┐  ┌──────────┐  ┌────────┐             │
│  │   API   │  │  Worker  │  │  Sync  │             │
│  │ :8088   │  │  (queue) │  │(s3/ddb)│             │
│  └────┬────┘  └────┬─────┘  └───┬────┘             │
│       │            │            │                  │
│       └────────────┼────────────┘                  │
│                    │                               │
│         ┌──────────▼──────────┐                    │
│         │    MySQL (agro)     │                    │
│         │ :3309               │                    │
│         └──────────┬──────────┘                    │
│                    │                               │
│       ┌────────────┼────────────┐                  │
│       │            │            │                  │
│  ┌────▼───┐  ┌────▼──┐  ┌─────▼─┐               │
│  │   S3   │  │  SQS  │  │DynamoDB│               │
│  │ (Images)  │(Jobs) │  │(Cache) │               │
│  └────────┘  └───────┘  └────────┘               │
│                                                      │
└──────────────────────────────────────────────────────┘
```

---

## Requisitos Mínimos

### Hardware
- CPU: 4 cores mínimo, 8 recomendado
- RAM: 8GB mínimo, 16GB recomendado
- Disk: 50GB mínimo, SSD recomendado
- Network: 1Gbps, sin reglas de firewall bloqueando puertos

### Software
- Docker 20.10+
- Docker Compose 2.0+
- Go 1.24+ (para build local)
- MySQL 8.0+ (cliente)
- AWS CLI v2 (para AWS integración)

### Acceso
- Git repository access
- AWS credentials (keys/OIDC)
- MySQL admin credentials
- Docker registry access
- SSH key para servidor

---

## Variables de Entorno

Todas las variables están documentadas en:
- DEPLOYMENT_PLAN.md §Fase 1 - Configurar Variables
- `.env.example` (copiar y modificar)
- `configs/config.example.yaml` (copiar y modificar)

Mínimamente necesitas:
```bash
MYSQL_HOST=localhost
MYSQL_USER=agro_admin
MYSQL_PASSWORD=your_secure_password
MYSQL_DATABASE=agro_prod
AWS_REGION=us-east-1
```

---

## Checklist de Deployment

### Pre-Deployment (Hoy)
- [ ] Leer DEPLOYMENT_PLAN.md completo
- [ ] Ejecutar `./scripts/pre-deploy-check.sh`
- [ ] Crear backups con `./scripts/backup-all.sh`
- [ ] Notificar stakeholders
- [ ] Preparar equipo de escalation

### Deployment Day
- [ ] Confirmar ventana de mantenimiento
- [ ] Conectar con team en Slack
- [ ] Ejecutar deployment según DEPLOYMENT_RUNBOOK
- [ ] Monitorear durante 1 hora
- [ ] Documentar cualquier issue

### Post-Deployment
- [ ] Validación de funcionalidad completa
- [ ] Monitoreo por 24 horas
- [ ] Feedback de usuarios
- [ ] Actualizar documentación si es necesario
- [ ] Post-mortem si hay issues

---

## Troubleshooting Rápido

| Problema | Solución |
|----------|----------|
| API no responde | DEPLOYMENT_RUNBOOK §Troubleshooting #5 |
| MySQL connection error | DEPLOYMENT_RUNBOOK §Troubleshooting #1 |
| Docker build falla | DEPLOYMENT_RUNBOOK §Troubleshooting #2 |
| FK constraint error | DEPLOYMENT_RUNBOOK §Troubleshooting #3 |
| Disk full | DEPLOYMENT_RUNBOOK §Troubleshooting #4 |
| Performance degradation | OPERATIONS_GUIDE §Performance Tuning |
| Data corruption | OPERATIONS_GUIDE §Disaster Recovery |

---

## Métricas de Éxito

Después de deployment, verifica:

```bash
✓ API Health:        curl http://localhost:8088/health
✓ DB Connection:     mysql -u agro_admin -p agro_prod -e "SELECT 1;"
✓ All Tables:        mysql ... -e "SHOW TABLES;"
✓ Data Integrity:    mysql ... -e "CHECK TABLE ...;"
✓ Disk Space:        df -h / (>20% libre)
✓ Container Health:  docker ps (all healthy)
✓ No Recent Errors:  docker logs | grep -i error | wc -l
```

---

## Support & Escalation

**Para preguntas:**
- Email: drh.megafresh@gmail.com
- Slack: #agro-sentinel-ops

**Para emergencias (24/7):**
- On-Call: [teléfono on-call]
- Escalate a Backend Lead si > 30 min

**Documentación relacionada:**
- README principal: `../README.md`
- Plan de implementación: `../docs/`
- Especificación: `../docs/DESIGN_SPECIFICATION.md`

---

## Versiones de Documentos

| Documento | Versión | Fecha | Estado |
|-----------|---------|-------|--------|
| DEPLOYMENT_PLAN | 1.0 | 2026-09-04 | Production Ready |
| DEPLOYMENT_RUNBOOK | 1.0 | 2026-09-04 | Production Ready |
| OPERATIONS_GUIDE | 1.0 | 2026-09-04 | Production Ready |
| DEPLOYMENT_INDEX | 1.0 | 2026-09-04 | Production Ready |

---

## ¿Necesitas Ayuda?

### Si tienes dudas...
1. Busca en la sección FAQ: DEPLOYMENT_INDEX.md
2. Revisa troubleshooting: DEPLOYMENT_RUNBOOK.md
3. Contáctame: drh.megafresh@gmail.com

### Si hay un problema...
1. Ejecuta `./scripts/health-check.sh`
2. Revisa los logs: `docker logs [container]`
3. Escalate según matriz: OPERATIONS_GUIDE.md §5

### Si necesitas rollback...
1. Abre DEPLOYMENT_PLAN.md §Plan de Rollback
2. Ejecuta `./scripts/rollback.sh`
3. Valida con `./scripts/health-check.sh`

---

## Contribución

Si encuentras errores o mejoras en esta documentación:
1. Haz un issue en el repositorio
2. O contacta a: drh.megafresh@gmail.com
3. Documentaremos y actualizaremos

---

**Última actualización:** 2026-09-04  
**Próxima revisión:** 2026-10-04  
**Creado por:** Daniel Rivera Herrada  

---

## Índice de Archivos en Docs

```
docs/
├── README_DEPLOYMENT.md          ← Estás aquí (inicio)
├── DEPLOYMENT_PLAN.md            ← Plan completo (30 min read)
├── DEPLOYMENT_RUNBOOK.md         ← Guía operativa (scripts)
├── OPERATIONS_GUIDE.md           ← Manual de operación
├── DEPLOYMENT_INDEX.md           ← Referencia rápida
└── [otros documentos...]
```

**Comienza aquí → [`DEPLOYMENT_PLAN.md`](./DEPLOYMENT_PLAN.md)**

---

*Imprime este README y tenlo a mano durante deployment.*
