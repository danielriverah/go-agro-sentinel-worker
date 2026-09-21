# Guía de Startup - Agro Sentinel Worker

Guía completa para arrancar el proyecto desde cero con todos los servicios necesarios.

## Tabla de Contenidos

- [Requisitos Previos](#requisitos-previos)
- [Paso 1: Verificar Requisitos](#paso-1-verificar-requisitos)
- [Paso 2: Levantar Docker Compose](#paso-2-levantar-docker-compose)
- [Paso 3: Verificar Health Checks](#paso-3-verificar-health-checks)
- [Paso 4: Crear Schema MySQL](#paso-4-crear-schema-mysql)
- [Paso 5: Crear Tablas DynamoDB](#paso-5-crear-tablas-dynamodb)
- [Paso 6: Crear Buckets S3](#paso-6-crear-buckets-s3)
- [Paso 7: Verificar Conectividad](#paso-7-verificar-conectividad)
- [Paso 8: Cargar Datos de Prueba](#paso-8-cargar-datos-de-prueba)
- [Paso 9: Ejecutar Sync Service](#paso-9-ejecutar-sync-service)
- [Paso 10: Verificar Sincronización](#paso-10-verificar-sincronización)

---

## Requisitos Previos

### Software Obligatorio

1. **Docker Desktop** (v20.10+)
   - Windows: https://www.docker.com/products/docker-desktop
   - Mac: Descargado e instalado
   - Linux: `apt-get install docker.io docker-compose`

2. **Git** (para clonar/controlar versiones)
   - Windows/Mac: https://git-scm.com/
   - Linux: `apt-get install git`

3. **Go** (v1.24+) - Opcional para development
   - https://golang.org/dl/

4. **AWS CLI** - Opcional para testing
   - Windows/Mac: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html
   - Linux: `pip install awscli`

5. **MySQL Client** - Opcional para consultas
   - Windows: Descarga MySQL Installer o usa WSL
   - Mac: `brew install mysql-client`
   - Linux: `apt-get install mysql-client`

### Conocimiento Requerido

- Conceptos básicos de Docker/Docker Compose
- Familiaridad con línea de comandos (bash/powershell)
- Conocimientos de MySQL básico
- Nociones de DynamoDB/S3/SQS

---

## Paso 1: Verificar Requisitos

### Windows (PowerShell)

```powershell
# Verificar Docker
docker --version
docker-compose --version

# Verificar Git
git --version

# Verificar Go (opcional)
go version

# Ejecutar script de verificación
cd C:\xampp\htdocs\WEB\clientes\DRH\go-agro-sentinel-worker
.\scripts\run_tests.ps1 -CheckEnvironment
```

### Linux/Mac (Bash)

```bash
# Verificar Docker
docker --version
docker-compose --version

# Verificar Git
git --version

# Verificar Go (opcional)
go version

# Ejecutar script de verificación
cd /path/to/go-agro-sentinel-worker
./scripts/check-environment.sh
```

**Salida esperada:**
```
✓ Docker instalado
✓ Docker Compose instalado
✓ Git instalado
✓ Go instalado (opcional)
✓ Espacio en disco: OK
```

---

## Paso 2: Levantar Docker Compose

### 2.1 Limpiar Estado Previo (Opcional)

```bash
# Detener contenedores activos
docker-compose down

# Eliminar volúmenes (ADVERTENCIA: borra datos)
docker-compose down -v

# Eliminar imágenes locales
docker rmi go-agro-sentinel:latest
```

### 2.2 Construir Imágenes

```bash
# Descargar y construir todas las imágenes
docker-compose build

# Para reconstruir sin caché (fuerza rebuild)
docker-compose build --no-cache
```

**Tiempo esperado:** 2-5 minutos (depende de conexión)

### 2.3 Iniciar Servicios

```bash
# Iniciar en background
docker-compose up -d

# O en foreground (para ver logs en tiempo real)
docker-compose up

# Mostrar solo un servicio específico
docker-compose up -d mysql
docker-compose up -d localstack
docker-compose up -d api
```

**Servicios que se levantan:**

| Servicio    | Container Name                | Puerto | Rol                              |
|-------------|-------------------------------|--------|----------------------------------|
| MySQL       | go-agro-sentinel-mysql        | 3309   | Base datos principal             |
| LocalStack  | go-agro-sentinel-localstack   | 4566   | S3, SQS, DynamoDB (mock)         |
| API         | go-agro-sentinel-api          | 8088   | API REST endpoints               |
| Worker      | go-agro-sentinel-worker       | -      | Procesa jobs de SQS              |
| Sync        | go-agro-sentinel-sync         | -      | Sincroniza DynamoDB → MySQL      |

---

## Paso 3: Verificar Health Checks

### 3.1 Verificar Servicios en Ejecución

```bash
# Ver estado de todos los contenedores
docker-compose ps

# Salida esperada:
# NAME                              STATUS                PORTS
# go-agro-sentinel-mysql            Up (healthy)          0.0.0.0:3309->3306/tcp
# go-agro-sentinel-localstack       Up                    0.0.0.0:4566->4566/tcp
# go-agro-sentinel-api              Up                    0.0.0.0:8088->6000/tcp
# go-agro-sentinel-worker           Up
# go-agro-sentinel-sync             Up
```

### 3.2 Verificar Logs

```bash
# Ver logs de un servicio específico
docker-compose logs mysql
docker-compose logs localstack
docker-compose logs api

# Ver logs en tiempo real (últimas 50 líneas)
docker-compose logs -f --tail=50 mysql

# Ver logs de un período específico
docker-compose logs --since 5m mysql
```

### 3.3 Test de Conectividad API

```bash
# Verificar que API está respondiendo
curl -s http://localhost:8088/health

# Salida esperada:
# {"status":"healthy"}
```

---

## Paso 4: Crear Schema MySQL

### 4.1 Automático (Recomendado)

```bash
# El script init-mysql.sql se ejecuta automáticamente
# Al iniciar docker-compose (ver volúmenes en docker-compose.yml)

# Verificar que las tablas se crearon
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SHOW TABLES;"

# Salida esperada:
# +-------------------------------------+
# | Tables_in_agro                      |
# +-------------------------------------+
# | articulos                           |
# | centros_costos                      |
# | producciones                        |
# | monitoreo_produccion_temporal       |
# | monitoreo_escenas                   |
# | monitoreo_cambios_estado            |
# +-------------------------------------+
```

### 4.2 Manual (Si es necesario)

```bash
# Ejecutar script SQL manualmente
mysql -h 127.0.0.1 -P 3309 -u root -proot agro < scripts/init-mysql.sql

# O conectarse y ejecutar:
mysql -h 127.0.0.1 -P 3309 -u root -proot

# Dentro de MySQL:
> USE agro;
> SHOW TABLES;
> DESCRIBE producciones;
```

### 4.3 Verificar Conexión desde Go

```bash
# El worker/api conectarán automáticamente
# Revisar logs para errores
docker-compose logs api | grep -i "database\|connected"
docker-compose logs worker | grep -i "database\|connected"
```

---

## Paso 5: Crear Tablas DynamoDB

### 5.1 Automático

```bash
# El script init-localstack.sh crea tablas automáticamente
# Ver en docker-compose.yml: volumes - ./scripts/init-localstack.sh

# Verificar tablas creadas
aws dynamodb list-tables \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Salida esperada:
# {
#     "TableNames": [
#         "monitoring_producciones",
#         "monitoring_escenas"
#     ]
# }
```

### 5.2 Manual (Si es necesario)

```bash
# Crear tabla monitoring_producciones
aws dynamodb create-table \
  --table-name monitoring_producciones \
  --attribute-definitions AttributeName=produccion_id,AttributeType=N \
  --key-schema AttributeName=produccion_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Crear tabla monitoring_escenas
aws dynamodb create-table \
  --table-name monitoring_escenas \
  --attribute-definitions \
    AttributeName=produccion_id,AttributeType=N \
    AttributeName=scene_id,AttributeType=S \
  --key-schema \
    AttributeName=produccion_id,KeyType=HASH \
    AttributeName=scene_id,KeyType=RANGE \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

### 5.3 Verificar Tablas

```bash
# Listar detalles de una tabla
aws dynamodb describe-table \
  --table-name monitoring_producciones \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Contar items en tabla
aws dynamodb scan \
  --table-name monitoring_producciones \
  --select COUNT \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

---

## Paso 6: Crear Buckets S3

### 6.1 Automático

```bash
# El script init-localstack.sh crea bucket automáticamente
# Bucket: agro-sentinel-bucket

# Verificar buckets
aws s3 ls \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Salida esperada:
# 2026-09-04 12:00:00 agro-sentinel-bucket
```

### 6.2 Manual (Si es necesario)

```bash
# Crear bucket
aws s3 mb s3://agro-sentinel-bucket \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Verificar contenido
aws s3 ls s3://agro-sentinel-bucket/ \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

---

## Paso 7: Verificar Conectividad

### 7.1 MySQL

```bash
# Conectar a MySQL
mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SELECT VERSION();"

# Salida esperada:
# +-----------+
# | VERSION() |
# +-----------+
# | 8.0.x     |
# +-----------+
```

### 7.2 DynamoDB (LocalStack)

```bash
# Verificar conexión a DynamoDB
aws dynamodb scan \
  --table-name monitoring_producciones \
  --endpoint-url http://localhost:4566 \
  --region us-east-1 \
  --max-items 1

# Salida esperada:
# {
#     "Items": [],
#     "Count": 0,
#     "ScannedCount": 0
# }
```

### 7.3 S3 (LocalStack)

```bash
# Verificar conexión a S3
aws s3 ls \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Salida esperada:
# 2026-09-04 12:00:00 agro-sentinel-bucket
```

### 7.4 SQS (LocalStack)

```bash
# Verificar cola SQS
aws sqs list-queues \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Salida esperada:
# {
#     "QueueUrls": [
#         "http://localhost:4566/000000000000/agro-sentinel-jobs"
#     ]
# }
```

### 7.5 API HTTP

```bash
# Verificar health endpoint
curl -s http://localhost:8088/health

# Salida esperada:
# {"status":"healthy"}

# Obtener información de dependencias
curl -s http://localhost:8088/health/dependencies
```

---

## Paso 8: Cargar Datos de Prueba

### 8.1 Insertar Datos en MySQL

```bash
# Opción 1: Usar script SQL
mysql -h 127.0.0.1 -P 3309 -u root -proot agro < scripts/import-from-csv.sql

# Opción 2: Insertar manualmente
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
INSERT INTO articulos (nombre, variedad) VALUES ('Tomate', 'Roma');
INSERT INTO articulos (nombre, variedad) VALUES ('Pepino', 'Japonés');
INSERT INTO centros_costos (nombre) VALUES ('Rancho A');
INSERT INTO centros_costos (nombre) VALUES ('Rancho B');
INSERT INTO producciones (folio, articulo_id, centro_costo_id, fecha) 
  VALUES ('PROD-001', 1, 1, CURDATE());
INSERT INTO producciones (folio, articulo_id, centro_costo_id, fecha) 
  VALUES ('PROD-002', 2, 2, CURDATE());
EOF
```

### 8.2 Verificar Datos en MySQL

```bash
# Ver registros insertados
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "
  SELECT COUNT(*) as total_articulos FROM articulos;
  SELECT COUNT(*) as total_producciones FROM producciones;
  SELECT COUNT(*) as total_centros FROM centros_costos;
"

# Salida esperada:
# +-------------------+
# | total_articulos   |
# +-------------------+
# | 2                 |
# +-------------------+
# +----------------------+
# | total_producciones   |
# +----------------------+
# | 2                    |
# +----------------------+
```

### 8.3 Cargar Datos en DynamoDB

```bash
# Opción 1: Usar script Python (recomendado)
cd scripts
python3 import-dynamodb.py

# Opción 2: Insertar manualmente
aws dynamodb put-item \
  --table-name monitoring_producciones \
  --item '{
    "produccion_id": {"N": "1"},
    "status": {"S": "active"},
    "ultimo_monitoreo": {"S": "2026-09-04"}
  }' \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

---

## Paso 9: Ejecutar Sync Service

### 9.1 Iniciar Sync Service

```bash
# Si no está corriendo, iniciarlo
docker-compose up -d sync

# Ver logs del sync service
docker-compose logs -f sync

# Salida esperada:
# [Sync Service] Iniciando sincronización...
# [Sync Service] Conectado a MySQL
# [Sync Service] Conectado a DynamoDB
# [Sync Service] Sincronización completada: X registros
```

### 9.2 Verificar Logs

```bash
# Ver últimas 100 líneas de logs
docker-compose logs --tail=100 sync

# Ver logs con timestamps
docker-compose logs -t sync

# Buscar errores
docker-compose logs sync | grep -i error
```

### 9.3 Parar Sync Service

```bash
# Parar solo sync
docker-compose stop sync

# Reiniciar sync
docker-compose restart sync
```

---

## Paso 10: Verificar Sincronización

### 10.1 Verificar Datos en MySQL

```bash
# Consultar tabla de producciones sincronizadas
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT produccion_id, status, ultimo_monitoreo 
FROM monitoreo_produccion_temporal 
LIMIT 5;
EOF

# Salida esperada:
# +---------------+--------+-------------------+
# | produccion_id | status | ultimo_monitoreo  |
# +---------------+--------+-------------------+
# | 1             | active | 2026-09-04        |
# | 2             | active | 2026-09-04        |
# +---------------+--------+-------------------+
```

### 10.2 Verificar Datos en DynamoDB

```bash
# Escanear tabla
aws dynamodb scan \
  --table-name monitoring_producciones \
  --endpoint-url http://localhost:4566 \
  --region us-east-1 \
  --max-items 10
```

### 10.3 Verificar Archivos en S3

```bash
# Listar archivos
aws s3 ls s3://agro-sentinel-bucket/ \
  --endpoint-url http://localhost:4566 \
  --region us-east-1 --recursive

# Descargar un archivo específico
aws s3 cp s3://agro-sentinel-bucket/scene-001.tif . \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

---

## Checklist de Startup

- [ ] Docker Desktop instalado y funcionando
- [ ] Git instalado
- [ ] Repositorio clonado/actualizado
- [ ] Script de verificación pasó OK
- [ ] Docker Compose build completado
- [ ] Contenedores levantados y sanos
- [ ] MySQL respondiendo a health checks
- [ ] API respondiendo en http://localhost:8088/health
- [ ] LocalStack servicios activos
- [ ] Tablas MySQL creadas
- [ ] Tablas DynamoDB creadas
- [ ] Bucket S3 creado
- [ ] Datos de prueba insertados
- [ ] Sync service iniciado
- [ ] Sincronización verificada

---

## Troubleshooting

### Puerto 3309 (MySQL) ya en uso

```bash
# Encontrar proceso que usa puerto 3309
lsof -i :3309              # Linux/Mac
netstat -ano | findstr :3309  # Windows

# Cambiar puerto en docker-compose.yml
# Cambiar "3309:3306" a "3310:3306"
# Entonces conectar con: mysql -h 127.0.0.1 -P 3310
```

### Puerto 4566 (LocalStack) ya en uso

```bash
# Cambiar puerto en docker-compose.yml
# Cambiar "4566:4566" a "4567:4566"
# Entonces usar: --endpoint-url http://localhost:4567
```

### MySQL no inicia con health check

```bash
# Esperar más tiempo (primera ejecución es lenta)
docker-compose logs mysql

# Si sigue fallando, reconstruir MySQL
docker-compose down -v
docker-compose up -d mysql
```

### Conectar a MySQL desde container

```bash
# Ejecutar bash dentro del container MySQL
docker-compose exec mysql bash

# Dentro del container, conectar
mysql -h localhost -u root -proot agro
```

---

## Próximos Pasos

Después de completar este startup, procede con:

1. **Sync Testing**: Ver `SYNC_TESTING_GUIDE.md`
2. **Testing Completo**: Ejecutar `scripts/test-sync-flow.sh`
3. **Development**: Consultar `docs/` para arquitectura
4. **Deployment**: Ver documentación en `/docs/DEPLOYMENT_SUMMARY.txt`

