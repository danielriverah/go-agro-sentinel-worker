# Guía de Sync Testing - Agro Sentinel Worker

Guía paso a paso para hacer pruebas controladas de sincronización entre DynamoDB → S3 → MySQL.

## Tabla de Contenidos

- [Flujo de Prueba](#flujo-de-prueba)
- [Test 1: Sincronización Básica (1 Producción)](#test-1-sincronización-básica)
- [Test 2: Múltiples Producciones](#test-2-múltiples-producciones)
- [Test 3: Procesamiento de Escenas](#test-3-procesamiento-de-escenas)
- [Test 4: Validación de Estados](#test-4-validación-de-estados)
- [Test 5: Verificación de S3](#test-5-verificación-de-s3)
- [Troubleshooting](#troubleshooting)
- [Casos de Prueba Adicionales](#casos-de-prueba-adicionales)

---

## Flujo de Prueba

```
DynamoDB (monitoring_producciones)
    ↓
Sync Service (Lee DynamoDB)
    ↓
MySQL (Inserta en monitoreo_produccion_temporal)
    ↓
Worker Service (Lee MySQL)
    ↓
S3 (Guarda imágenes/escenas)
    ↓
Verificación final
```

---

## Test 1: Sincronización Básica

### Objetivo
Verificar que 1 producción se sincroniza correctamente de DynamoDB → MySQL.

### Paso 1.1: Verificar Estado Inicial

```bash
# Contar producciones actuales en DynamoDB
aws dynamodb scan \
  --table-name monitoring_producciones \
  --select COUNT \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Salida esperada: Count: 0 (o número actual)
# Guarda este número para comparar luego
```

### Paso 1.2: Insertar 1 Producción en DynamoDB

```bash
# Opción A: Insertar manualmente
aws dynamodb put-item \
  --table-name monitoring_producciones \
  --item '{
    "produccion_id": {"N": "101"},
    "folio": {"S": "TEST-SYNC-001"},
    "articulo_id": {"N": "1"},
    "centro_costo_id": {"N": "1"},
    "status": {"S": "monitoring"},
    "fecha": {"S": "2026-09-04"},
    "fecha_creacion": {"S": "2026-09-04T12:00:00Z"},
    "fecha_actualizacion": {"S": "2026-09-04T12:00:00Z"},
    "ultima_escena": {"S": ""},
    "cantidad_escenas": {"N": "0"}
  }' \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Salida esperada: {} (vacío significa éxito)

# Opción B: Usar script Python
python3 scripts/test-import-dynamodb.py --produccion-id 101

# Opción C: Insertar desde MySQL primero
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
INSERT INTO producciones (produccion_id, folio, articulo_id, centro_costo_id, fecha)
VALUES (101, 'TEST-SYNC-001', 1, 1, '2026-09-04');
EOF
```

### Paso 1.3: Verificar Inserción en DynamoDB

```bash
# Consultar el registro que se acaba de insertar
aws dynamodb get-item \
  --table-name monitoring_producciones \
  --key '{"produccion_id": {"N": "101"}}' \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Salida esperada:
# {
#   "Item": {
#     "produccion_id": {"N": "101"},
#     "folio": {"S": "TEST-SYNC-001"},
#     "status": {"S": "monitoring"},
#     ...
#   }
# }
```

### Paso 1.4: Forzar Sincronización

```bash
# Opción A: Reiniciar sync service (fuerza lectura)
docker-compose restart sync

# Ver logs mientras sincroniza
docker-compose logs -f sync --tail 50

# Opción B: Esperar a que sync lea automáticamente (cada X segundos)
# Ver logs del sync service
docker-compose logs sync | grep -i "sync\|sync\|processing"

# Opción C: Ejecutar sync manualmente (si está disponible CLI)
# docker-compose exec sync /app/api sync --force
```

### Paso 1.5: Verificar Sincronización en MySQL

```bash
# Esperar 5-10 segundos (tiempo de sincronización)

# Buscar el registro sincronizado
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT produccion_id, folio, status, fecha_sincronizacion 
FROM monitoreo_produccion_temporal 
WHERE folio = 'TEST-SYNC-001'
LIMIT 1;
EOF

# Salida esperada:
# +---------------+-------------------+----------+---------------------+
# | produccion_id | folio             | status   | fecha_sincronizacion|
# +---------------+-------------------+----------+---------------------+
# | 101           | TEST-SYNC-001     | active   | 2026-09-04 12:05:00 |
# +---------------+-------------------+----------+---------------------+

# Si no aparece, ver los logs
docker-compose logs sync | grep -i "error\|fail"
```

### Paso 1.6: Resumen Paso 1

Si llegaste aquí correctamente:
- ✓ Dato insertado en DynamoDB
- ✓ Sincronización se ejecutó
- ✓ Dato llegó a MySQL
- ✓ Status es "active"

---

## Test 2: Múltiples Producciones

### Objetivo
Verificar que múltiples producciones se sincronizan sin perder datos.

### Paso 2.1: Insertar 5 Producciones

```bash
# Script Python (recomendado)
cat > /tmp/insert_test_data.py << 'EOF'
import boto3
from datetime import datetime

dynamodb = boto3.resource(
    'dynamodb',
    endpoint_url='http://localhost:4566',
    region_name='us-east-1',
    aws_access_key_id='test',
    aws_secret_access_key='test'
)

table = dynamodb.Table('monitoring_producciones')

for i in range(201, 206):
    item = {
        'produccion_id': i,
        'folio': f'TEST-BATCH-{i}',
        'articulo_id': (i % 3) + 1,
        'centro_costo_id': (i % 2) + 1,
        'status': 'monitoring',
        'fecha': '2026-09-04',
        'fecha_creacion': datetime.now().isoformat(),
        'fecha_actualizacion': datetime.now().isoformat(),
        'ultima_escena': '',
        'cantidad_escenas': 0
    }
    table.put_item(Item=item)
    print(f"✓ Inserción {i} completada")

print("\n✓ Todas 5 producciones insertadas")
EOF

python3 /tmp/insert_test_data.py
```

### Paso 2.2: Verificar Inserciones en DynamoDB

```bash
# Contar total de registros
aws dynamodb scan \
  --table-name monitoring_producciones \
  --select COUNT \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Deberías ver al menos 6 (la del test 1 + 5 nuevas)
```

### Paso 2.3: Ejecutar Sincronización

```bash
# Reiniciar sync
docker-compose restart sync
sleep 2

# Ver progreso
docker-compose logs -f sync --tail 50
```

### Paso 2.4: Verificar Sincronización

```bash
# Contar sincronizaciones en MySQL
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT COUNT(*) as total_sincronizados,
       COUNT(DISTINCT status) as estados_distintos
FROM monitoreo_produccion_temporal 
WHERE folio LIKE 'TEST-BATCH-%';
EOF

# Salida esperada:
# +---------------------+-------------------+
# | total_sincronizados | estados_distintos |
# +---------------------+-------------------+
# | 5                   | 1                 |
# +---------------------+-------------------+

# Ver detalles de cada uno
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT produccion_id, folio, status, fecha_sincronizacion 
FROM monitoreo_produccion_temporal 
WHERE folio LIKE 'TEST-BATCH-%'
ORDER BY produccion_id;
EOF
```

### Paso 2.5: Resumen Paso 2

- ✓ 5 producciones en DynamoDB
- ✓ Todas se sincronizaron
- ✓ Todos los estados correctos
- ✓ Timestamps válidos

---

## Test 3: Procesamiento de Escenas

### Objetivo
Verificar que las escenas se procesan correctamente en la tabla monitoring_escenas.

### Paso 3.1: Insertar Escenas

```bash
# Insertar escenas para la producción 101
cat > /tmp/insert_scenes.py << 'EOF'
import boto3
from datetime import datetime

dynamodb = boto3.resource(
    'dynamodb',
    endpoint_url='http://localhost:4566',
    region_name='us-east-1'
)

table = dynamodb.Table('monitoring_escenas')

scenes = [
    {
        'produccion_id': 101,
        'scene_id': 'SCENE-101-001',
        'satellite': 'Sentinel-2',
        'fecha_captura': '2026-09-04T10:30:00Z',
        'fecha_procesamiento': datetime.now().isoformat(),
        'cloud_coverage': 5.2,
        's3_key': 'scenes/101/SCENE-101-001.tif',
        'status': 'processed'
    },
    {
        'produccion_id': 101,
        'scene_id': 'SCENE-101-002',
        'satellite': 'Sentinel-2',
        'fecha_captura': '2026-09-04T14:30:00Z',
        'fecha_procesamiento': datetime.now().isoformat(),
        'cloud_coverage': 8.1,
        's3_key': 'scenes/101/SCENE-101-002.tif',
        'status': 'processed'
    },
    {
        'produccion_id': 101,
        'scene_id': 'SCENE-101-003',
        'satellite': 'Sentinel-2',
        'fecha_captura': '2026-09-04T18:30:00Z',
        'fecha_procesamiento': datetime.now().isoformat(),
        'cloud_coverage': 12.5,
        's3_key': 'scenes/101/SCENE-101-003.tif',
        'status': 'processing'
    }
]

for scene in scenes:
    table.put_item(Item=scene)
    print(f"✓ Escena {scene['scene_id']} insertada")

print("\n✓ Todas las escenas insertadas")
EOF

python3 /tmp/insert_scenes.py
```

### Paso 3.2: Verificar Escenas en DynamoDB

```bash
# Contar escenas
aws dynamodb scan \
  --table-name monitoring_escenas \
  --select COUNT \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# Deberías ver Count: 3

# Ver detalles
aws dynamodb query \
  --table-name monitoring_escenas \
  --key-condition-expression "produccion_id = :pid" \
  --expression-attribute-values '{":pid": {"N": "101"}}' \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

### Paso 3.3: Ejecutar Sincronización

```bash
# Reiniciar sync para que procese escenas
docker-compose restart sync
sleep 3

# Ver logs
docker-compose logs -f sync --tail 100
```

### Paso 3.4: Verificar Escenas en MySQL

```bash
# Consultar escenas sincronizadas
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT produccion_id, scene_id, satellite, cloud_coverage, status
FROM monitoreo_escenas 
WHERE produccion_id = 101
ORDER BY fecha_captura;
EOF

# Salida esperada:
# +---------------+----------------+-----------+----------------+--------+
# | produccion_id | scene_id       | satellite | cloud_coverage | status |
# +---------------+----------------+-----------+----------------+--------+
# | 101           | SCENE-101-001  | Sentinel-2| 5.2            | processed |
# | 101           | SCENE-101-002  | Sentinel-2| 8.1            | processed |
# | 101           | SCENE-101-003  | Sentinel-2| 12.5           | processing|
# +---------------+----------------+-----------+----------------+--------+

# Contar por status
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT status, COUNT(*) as cantidad
FROM monitoreo_escenas 
WHERE produccion_id = 101
GROUP BY status;
EOF
```

### Paso 3.5: Resumen Paso 3

- ✓ 3 escenas insertadas en DynamoDB
- ✓ Escenas sincronizadas a MySQL
- ✓ Datos completos (satellite, cloud_coverage, etc.)
- ✓ Estados correctos (processed, processing)

---

## Test 4: Validación de Estados

### Objetivo
Verificar que los cambios de estado se sincronizan correctamente.

### Paso 4.1: Cambiar Estado en DynamoDB

```bash
# Actualizar estado de una producción
aws dynamodb update-item \
  --table-name monitoring_producciones \
  --key '{"produccion_id": {"N": "101"}}' \
  --update-expression "SET #s = :status" \
  --expression-attribute-names '{"#s": "status"}' \
  --expression-attribute-values '{":status": {"S": "completed"}}' \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

echo "✓ Estado actualizado a 'completed'"
```

### Paso 4.2: Ejecutar Sincronización

```bash
# Reiniciar sync
docker-compose restart sync
sleep 2

# Ver logs
docker-compose logs sync | grep -i "update\|sync"
```

### Paso 4.3: Verificar Cambio en MySQL

```bash
# Ver estado actualizado
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT produccion_id, status, fecha_sincronizacion 
FROM monitoreo_produccion_temporal 
WHERE produccion_id = 101;
EOF

# Deberías ver status = 'completed' con nueva fecha_sincronizacion
```

### Paso 4.4: Resumen Paso 4

- ✓ Estado cambió en DynamoDB
- ✓ Cambio se sincronizó a MySQL
- ✓ Timestamp se actualizó

---

## Test 5: Verificación de S3

### Objetivo
Verificar que las escenas se guardan correctamente en S3.

### Paso 5.1: Crear Archivos de Escena en S3

```bash
# Crear directorio de escenas
aws s3 cp /dev/null s3://agro-sentinel-bucket/scenes/101/test.txt \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

# O crear archivos simulados
for i in 1 2 3; do
  echo "Simulación escena $i" > /tmp/scene_$i.tif
  aws s3 cp /tmp/scene_$i.tif \
    s3://agro-sentinel-bucket/scenes/101/SCENE-101-00$i.tif \
    --endpoint-url http://localhost:4566 \
    --region us-east-1
  echo "✓ SCENE-101-00$i.tif subida"
done
```

### Paso 5.2: Verificar Archivos en S3

```bash
# Listar archivos
aws s3 ls s3://agro-sentinel-bucket/scenes/101/ \
  --endpoint-url http://localhost:4566 \
  --region us-east-1 --recursive

# Salida esperada:
# 2026-09-04 12:15:00      32 scenes/101/SCENE-101-001.tif
# 2026-09-04 12:16:00      32 scenes/101/SCENE-101-002.tif
# 2026-09-04 12:17:00      32 scenes/101/SCENE-101-003.tif

# Contar archivos
aws s3 ls s3://agro-sentinel-bucket/scenes/ \
  --endpoint-url http://localhost:4566 \
  --region us-east-1 --recursive --summarize

# Salida esperada: Total Objects: 3
```

### Paso 5.3: Resumen Paso 5

- ✓ Archivos creados en S3
- ✓ Estructura de directorio correcta
- ✓ Nombres de archivo válidos

---

## Test Completo Automático

```bash
# Ejecutar todos los tests de una vez
bash scripts/test-sync-flow.sh

# Ver salida:
# ═════════════════════════════════════════════════════════════════
# Iniciando Sync Testing
# ═════════════════════════════════════════════════════════════════
# 
# [TEST 1/5] Sync Básico
# ✓ DynamoDB: 1 producción insertada
# ✓ MySQL: 1 producción sincronizada
# 
# [TEST 2/5] Múltiples Producciones
# ✓ DynamoDB: 5 producciones insertadas
# ✓ MySQL: 5 producciones sincronizadas
# 
# ... (más tests)
#
# ═════════════════════════════════════════════════════════════════
# RESULTADO: 5/5 tests PASARON
# ═════════════════════════════════════════════════════════════════
```

---

## Troubleshooting

### Datos no aparecen en MySQL

```bash
# Paso 1: Verificar que están en DynamoDB
aws dynamodb scan --table-name monitoring_producciones \
  --select COUNT --endpoint-url http://localhost:4566

# Paso 2: Ver logs del sync service
docker-compose logs sync | tail -50

# Paso 3: Buscar errores específicos
docker-compose logs sync | grep -i "error\|fail\|connection"

# Paso 4: Verificar conexión a MySQL desde sync
docker-compose exec sync bash
mysql -h go-agro-sentinel-mysql -u root -proot agro -e "SELECT COUNT(*) FROM monitoreo_produccion_temporal;"

# Paso 5: Reiniciar servicios
docker-compose down
docker-compose up -d
```

### Sincronización muy lenta

```bash
# Ajustar frecuencia de sync (depende de configuración)
# Ver CONFIG_PATH en docker-compose.yml

# Verificar recursos de Docker
docker stats

# Si CPU/RAM altos, aumentar en Docker Desktop
# Settings > Resources > CPU/Memory
```

### Datos duplicados en MySQL

```bash
# Ver registros duplicados
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT produccion_id, folio, COUNT(*) as cantidad
FROM monitoreo_produccion_temporal 
GROUP BY produccion_id, folio
HAVING cantidad > 1;
EOF

# Solución: Limpiar tabla
TRUNCATE TABLE monitoreo_produccion_temporal;

# Y reiniciar sincronización
docker-compose restart sync
```

### Error de conexión a DynamoDB

```bash
# Verificar que LocalStack está corriendo
docker-compose ps | grep localstack

# Ver logs de LocalStack
docker-compose logs localstack

# Si no está, reiniciar
docker-compose up -d localstack

# Verificar que puerto 4566 es accesible
curl -s http://localhost:4566/health
```

### Error de conexión a MySQL

```bash
# Verificar que MySQL está corriendo
docker-compose ps | grep mysql

# Ver logs de MySQL
docker-compose logs mysql | tail -50

# Si está fallando health check
docker-compose restart mysql

# Conectar manualmente
mysql -h 127.0.0.1 -P 3309 -u root -proot
```

---

## Casos de Prueba Adicionales

### Test: Escenas Incompletas

```bash
# Insertar escena sin procesar
aws dynamodb put-item \
  --table-name monitoring_escenas \
  --item '{
    "produccion_id": {"N": "102"},
    "scene_id": {"S": "SCENE-102-INCOMPLETE"},
    "status": {"S": "pending"},
    "fecha_captura": {"S": "2026-09-04T20:00:00Z"}
  }' \
  --endpoint-url http://localhost:4566

# Verificar que se sincroniza correctamente
docker-compose restart sync
sleep 2

# Buscar en MySQL
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT scene_id, status FROM monitoreo_escenas 
WHERE scene_id = 'SCENE-102-INCOMPLETE';
EOF
```

### Test: Actualización Parcial

```bash
# Actualizar solo algunos campos en DynamoDB
aws dynamodb update-item \
  --table-name monitoring_producciones \
  --key '{"produccion_id": {"N": "101"}}' \
  --update-expression "SET cantidad_escenas = :qty, ultima_escena = :scene" \
  --expression-attribute-values '{
    ":qty": {"N": "3"},
    ":scene": {"S": "SCENE-101-003"}
  }' \
  --endpoint-url http://localhost:4566

# Sincronizar y verificar
docker-compose restart sync
sleep 2

mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT produccion_id, cantidad_escenas, ultima_escena 
FROM monitoreo_produccion_temporal 
WHERE produccion_id = 101;
EOF
```

### Test: Timestamp Consistency

```bash
# Verificar que timestamps se mantienen consistentes
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
SELECT 
  produccion_id,
  fecha_creacion,
  fecha_actualizacion,
  fecha_sincronizacion,
  TIMESTAMPDIFF(SECOND, fecha_actualizacion, fecha_sincronizacion) as lag_segundos
FROM monitoreo_produccion_temporal 
WHERE produccion_id IN (101, 201, 202, 203, 204, 205)
ORDER BY produccion_id;
EOF

# El lag_segundos debe estar entre 2-10 segundos
```

---

## Resumen de Estados Esperados

| Etapa              | Tabla DynamoDB              | Tabla MySQL                  | S3                          |
|-------------------|------------------------------|------------------------------|----------------------------|
| Inicialización    | Vacía                        | Vacía                        | Vacía                      |
| Inserción datos   | 6 registros                  | 0 registros                  | 0 archivos                 |
| Sync 1            | 6 registros                  | 6 registros                  | 0 archivos                 |
| Escenas insertadas| 6 + 3 escenas                | 6 + 3 escenas                | 0 archivos                 |
| Sync 2            | Sin cambios                  | 3 escenas sincronizadas      | 0 archivos                 |
| Escenas en S3     | Sin cambios                  | Sin cambios                  | 3 archivos                 |
| Estado actualizado| 1 "completed"                | 1 "completed"                | Sin cambios                |
| Sync 3            | Sin cambios                  | Estado actualizado           | Sin cambios                |

