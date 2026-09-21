# Quick Start - Agro Sentinel Worker

Guía rápida para comenzar en 5 minutos.

## En 5 Minutos: Setup Completo

### Requisito: Docker instalado
```bash
docker --version  # Verificar instalación
```

### Paso 1: Clonar/Descargar Proyecto
```bash
cd C:\xampp\htdocs\WEB\clientes\DRH\go-agro-sentinel-worker
# O navega al directorio del proyecto
```

### Paso 2: Ejecutar Setup Automático

**Linux/Mac:**
```bash
bash scripts/setup-local-env.sh
# Responde "s" a las preguntas de configuración
```

**Windows (PowerShell):**
```powershell
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
.\scripts\setup-local-env.ps1
```

### Paso 3: Esperar a que Complete
- Construye imágenes: 2-5 minutos
- Inicia servicios: 1-2 minutos
- Setup total: 3-7 minutos

### Paso 4: Verificar
```bash
docker-compose ps
# Todos los servicios deben estar "Up" o "healthy"
```

✓ **¡Listo! Ya puedes hacer testing de sincronización.**

---

## Testing de Sincronización en 3 Pasos

### Paso 1: Insertar Producción en DynamoDB
```bash
aws dynamodb put-item \
  --table-name monitoring_producciones \
  --item '{"produccion_id": {"N": "1"}, "folio": {"S": "TEST"}, "status": {"S": "active"}}' \
  --endpoint-url http://localhost:4566 --region us-east-1
```

### Paso 2: Ejecutar Sincronización
```bash
docker-compose restart sync
sleep 3
```

### Paso 3: Verificar en MySQL
```bash
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
  "SELECT * FROM monitoreo_produccion_temporal LIMIT 5;"
```

✓ **Si ves el dato → La sincronización funciona!**

---

## ¿Qué Documentación Necesito?

### Soy nuevo en el proyecto
→ Comienza con **STARTUP_GUIDE.md**
- Requisitos detallados
- Explicación de cada paso
- Troubleshooting básico

### Quiero probar sincronización a fondo
→ Lee **SYNC_TESTING_GUIDE.md**
- 5 tests diferentes
- Pruebas de escenas
- Pruebas de cambios de estado
- 30-45 minutos de testing completo

### Tengo un problema
→ Consulta **SYNC_TROUBLESHOOTING.md**
- Problemas comunes y soluciones
- Específicos por SO
- Pasos de debug detallados

### Quiero un checklist paso a paso
→ Usa **SYNC_TESTING_CHECKLIST.md**
- Formato interactivo
- Cada paso verificable
- Con comandos listos para copiar

---

## Servicios Disponibles

Después del setup, tienes acceso a:

| Servicio | URL/Host | Puerto | Nota |
|----------|----------|--------|------|
| API REST | http://localhost:8088 | 8088 | Health: /health |
| MySQL | 127.0.0.1 | 3309 | User: root / Pass: root |
| LocalStack (S3) | http://localhost:4566 | 4566 | Mock AWS |
| LocalStack (DynamoDB) | http://localhost:4566 | 4566 | Mock AWS |
| LocalStack (SQS) | http://localhost:4566 | 4566 | Mock AWS |

---

## Comandos Frecuentes

### Ver logs
```bash
docker-compose logs -f sync      # Sync service
docker-compose logs -f mysql     # MySQL
docker-compose logs -f api       # API
docker-compose logs -f worker    # Worker
```

### Conectar a bases de datos
```bash
# MySQL interactivo
mysql -h 127.0.0.1 -P 3309 -u root -proot agro

# O desde container
docker-compose exec mysql bash
```

### Listar datos
```bash
# Producciones en MySQL
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SELECT * FROM monitoreo_produccion_temporal LIMIT 5;"

# Producciones en DynamoDB
aws dynamodb scan --table-name monitoring_producciones \
  --endpoint-url http://localhost:4566 --region us-east-1
```

### Reiniciar servicios
```bash
docker-compose restart sync      # Reiniciar sync
docker-compose restart mysql     # Reiniciar MySQL
docker-compose down              # Parar todo
docker-compose up -d             # Iniciar todo
```

### Limpiar datos
```bash
# MySQL
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
DELETE FROM monitoreo_produccion_temporal;
DELETE FROM monitoreo_escenas;
EOF

# DynamoDB (vaciar tabla)
aws dynamodb scan --table-name monitoring_producciones \
  --endpoint-url http://localhost:4566 --region us-east-1 | jq -r '.Items[].produccion_id.N' | \
  while read id; do
    aws dynamodb delete-item --table-name monitoring_producciones \
      --key "{\"produccion_id\": {\"N\": \"$id\"}}" \
      --endpoint-url http://localhost:4566 --region us-east-1
  done
```

---

## Scripts Disponibles

### setup-local-env.sh / setup-local-env.ps1
Setup automático completo. Construye imágenes, inicia servicios, carga datos de prueba.

```bash
# Linux/Mac
bash scripts/setup-local-env.sh

# Windows
.\scripts\setup-local-env.ps1 -LoadTestData
```

### test-sync-flow.sh
Suite de 5 tests automatizados para verificar sincronización.

```bash
bash scripts/test-sync-flow.sh
```

Prueba:
1. Sync básico (1 producción)
2. Múltiples producciones (5)
3. Escenas
4. Cambios de estado
5. Verificación de logs

---

## Flujo Típico de Desarrollo

```
1. Setup inicial
   ↓
   bash scripts/setup-local-env.sh
   
2. Verificar servicios
   ↓
   docker-compose ps
   
3. Hacer cambios en código
   ↓
   Editar internal/sync/...
   
4. Reconstruir
   ↓
   docker-compose build
   
5. Reiniciar servicios
   ↓
   docker-compose up -d
   
6. Testing
   ↓
   Insertar datos en DynamoDB
   docker-compose restart sync
   Verificar en MySQL
   
7. Ver logs para debug
   ↓
   docker-compose logs -f sync
```

---

## Pasos Siguientes

Después del setup inicial:

1. **Entender la arquitectura**
   - Ver `docs/` (si existe)
   - Leer comentarios en `internal/`
   - Revisar `docker-compose.yml`

2. **Ejecutar tests completos**
   ```bash
   bash scripts/test-sync-flow.sh
   ```

3. **Hacer cambios al código**
   - Editar archivos en `internal/`
   - Reconstruir: `docker-compose build`
   - Probar: `docker-compose up -d`

4. **Commit de cambios**
   ```bash
   git add .
   git commit -m "feat: describe changes"
   git push
   ```

---

## Troubleshooting Rápido

### Puerto 3309 ya en uso
```bash
# Cambiar puerto en docker-compose.yml
# Línea: ports: - "3310:3306"
# Conectar con: mysql -h 127.0.0.1 -P 3310
```

### MySQL no inicia
```bash
docker-compose logs mysql
# Esperar 30+ segundos (primera ejecución)
# O: docker-compose restart mysql
```

### Datos no sincronizan
```bash
docker-compose logs sync | grep -i error
docker-compose restart sync
```

### No hay espacio en disco
```bash
docker image prune -a
docker volume prune
```

**Más problemas?** → Ver **SYNC_TROUBLESHOOTING.md**

---

## Información de Entorno

El setup configura automáticamente:

**MySQL (Contenedor)**
- Host: go-agro-sentinel-mysql (interno)
- Host: 127.0.0.1 (externo)
- Puerto: 3309
- Usuario: root
- Contraseña: root
- Base de datos: agro

**LocalStack (Mock AWS)**
- Endpoint: http://localhost:4566
- S3: s3://agro-sentinel-bucket
- DynamoDB: monitoring_producciones, monitoring_escenas
- SQS: agro-sentinel-jobs
- Credenciales: test/test

**API**
- URL: http://localhost:8088
- Health: http://localhost:8088/health

---

## URLs y Comandos de Referencia

```bash
# Verificar health del sistema
curl http://localhost:8088/health

# Listar todas las tablas MySQL
mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SHOW DATABASES;"

# Listar buckets S3
aws s3 ls --endpoint-url http://localhost:4566 --region us-east-1

# Listar tablas DynamoDB
aws dynamodb list-tables --endpoint-url http://localhost:4566 --region us-east-1

# Ver proceso sync en tiempo real
docker-compose logs -f sync | grep -E "sync|processing|error"

# Contar registros en MySQL
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SELECT COUNT(*) FROM monitoreo_produccion_temporal;"

# Contar registros en DynamoDB
aws dynamodb scan --table-name monitoring_producciones --select COUNT \
  --endpoint-url http://localhost:4566 --region us-east-1
```

---

## Más Información

- **Setup detallado**: STARTUP_GUIDE.md
- **Testing completo**: SYNC_TESTING_GUIDE.md
- **Problemas y soluciones**: SYNC_TROUBLESHOOTING.md
- **Checklist interactivo**: SYNC_TESTING_CHECKLIST.md

