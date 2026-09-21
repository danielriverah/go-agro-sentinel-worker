# Sync Testing Checklist - Agro Sentinel Worker

Checklist interactivo paso a paso para verificar que todo funciona correctamente.

## Sección 1: Preparación

### 1.1 Verificar servicios

- [ ] **Docker Desktop está corriendo**
  ```bash
  docker --version
  # Esperado: Docker version x.x.x
  ```
  **Solución**: Si no está instalado, descargar desde https://www.docker.com

- [ ] **Git disponible**
  ```bash
  git --version
  # Esperado: git version x.x.x
  ```
  **Solución**: Descargar desde https://git-scm.com

- [ ] **Proyecto descargado**
  ```bash
  cd C:\xampp\htdocs\WEB\clientes\DRH\go-agro-sentinel-worker
  pwd  # o cd en PowerShell
  # Debería mostrar la ruta del proyecto
  ```

### 1.2 Limpiar estado previo (Opcional)

- [ ] **Detener contenedores anteriores**
  ```bash
  docker-compose down
  ```
  ✓ Sin errores

- [ ] **Eliminar volúmenes viejos (si es necesario)**
  ```bash
  docker-compose down -v
  ```
  ✓ Datos borrados

---

## Sección 2: Setup Inicial

### 2.1 Construir y levantar servicios

- [ ] **Ejecutar setup automático (Recomendado)**
  ```bash
  # Linux/Mac:
  bash scripts/setup-local-env.sh
  
  # Windows:
  .\scripts\setup-local-env.ps1
  ```
  ✓ Setup completado sin errores

- [ ] **O iniciar manualmente**
  ```bash
  docker-compose build
  docker-compose up -d
  ```
  ✓ Todos los servicios levantados

### 2.2 Verificar servicios activos

```bash
docker-compose ps
```

Debería ver:

| Servicio    | Status     |
|-------------|-----------|
| mysql       | Up (healthy) |
| localstack  | Up        |
| api         | Up        |
| worker      | Up        |
| sync        | Up        |

- [ ] MySQL está healthy
- [ ] LocalStack está up
- [ ] API está up
- [ ] Worker está up
- [ ] Sync está up

### 2.3 Verificar conectividad

- [ ] **MySQL accesible**
  ```bash
  mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SELECT VERSION();"
  ```
  ✓ Devuelve versión de MySQL

- [ ] **API respondiendo**
  ```bash
  curl http://localhost:8088/health
  ```
  ✓ Respuesta: `{"status":"healthy"}`

- [ ] **DynamoDB disponible**
  ```bash
  aws dynamodb list-tables --endpoint-url http://localhost:4566 --region us-east-1
  ```
  ✓ Muestra: `monitoring_producciones`, `monitoring_escenas`

- [ ] **S3 disponible**
  ```bash
  aws s3 ls --endpoint-url http://localhost:4566 --region us-east-1
  ```
  ✓ Muestra: `agro-sentinel-bucket`

---

## Sección 3: Test 1 - Sync Básico

### 3.1 Insertar producción en DynamoDB

```bash
aws dynamodb put-item \
  --table-name monitoring_producciones \
  --item '{
    "produccion_id": {"N": "9001"},
    "folio": {"S": "CHECK-001"},
    "articulo_id": {"N": "1"},
    "centro_costo_id": {"N": "1"},
    "status": {"S": "monitoring"},
    "fecha": {"S": "2026-09-04"},
    "cantidad_escenas": {"N": "0"}
  }' \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

- [ ] ✓ Inserción exitosa (respuesta vacía `{}`)

### 3.2 Verificar que está en DynamoDB

```bash
aws dynamodb get-item \
  --table-name monitoring_producciones \
  --key '{"produccion_id": {"N": "9001"}}' \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

- [ ] ✓ Muestra el item con folio "CHECK-001"

### 3.3 Forzar sincronización

```bash
docker-compose restart sync
sleep 3
```

- [ ] ✓ Sync reiniciado

### 3.4 Verificar en MySQL

```bash
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
  "SELECT produccion_id, folio, status FROM monitoreo_produccion_temporal WHERE folio = 'CHECK-001';"
```

- [ ] ✓ Aparece 1 resultado
- [ ] ✓ folio = "CHECK-001"
- [ ] ✓ status = "monitoring"
- [ ] ✓ fecha_sincronizacion no es NULL

**Si no aparece:**
- [ ] Revisar logs: `docker-compose logs sync | tail -30`
- [ ] Esperar 5 segundos más
- [ ] Verificar que MySQL tiene datos: `SELECT COUNT(*) FROM monitoreo_produccion_temporal;`

---

## Sección 4: Test 2 - Múltiples Producciones

### 4.1 Insertar 5 producciones

```bash
for i in {1..5}; do
  PROD_ID=$((9010 + i))
  aws dynamodb put-item \
    --table-name monitoring_producciones \
    --item "{
      \"produccion_id\": {\"N\": \"$PROD_ID\"},
      \"folio\": {\"S\": \"CHECK-BATCH-$PROD_ID\"},
      \"articulo_id\": {\"N\": \"1\"},
      \"centro_costo_id\": {\"N\": \"1\"},
      \"status\": {\"S\": \"monitoring\"},
      \"fecha\": {\"S\": \"2026-09-04\"},
      \"cantidad_escenas\": {\"N\": \"0\"}
    }" \
    --endpoint-url http://localhost:4566 \
    --region us-east-1
done
```

- [ ] ✓ 5 producciones insertadas (sin errores)

### 4.2 Ejecutar sincronización

```bash
docker-compose restart sync
sleep 3
```

- [ ] ✓ Sync reiniciado

### 4.3 Verificar todas en MySQL

```bash
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
  "SELECT COUNT(*) as total FROM monitoreo_produccion_temporal WHERE folio LIKE 'CHECK-BATCH-%';"
```

- [ ] ✓ Resultado = 5

### 4.4 Ver detalles

```bash
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
  "SELECT produccion_id, folio, status FROM monitoreo_produccion_temporal WHERE folio LIKE 'CHECK-BATCH-%' ORDER BY produccion_id;"
```

- [ ] ✓ Muestra 5 filas
- [ ] ✓ IDs van de 9011 a 9015
- [ ] ✓ Todos tienen status = "monitoring"

---

## Sección 5: Test 3 - Escenas

### 5.1 Insertar escenas para producción 9001

```bash
for i in {1..2}; do
  SCENE_ID="CHECK-SCENE-9001-$(printf '%03d' $i)"
  aws dynamodb put-item \
    --table-name monitoring_escenas \
    --item "{
      \"produccion_id\": {\"N\": \"9001\"},
      \"scene_id\": {\"S\": \"$SCENE_ID\"},
      \"satellite\": {\"S\": \"Sentinel-2\"},
      \"fecha_captura\": {\"S\": \"2026-09-04T$(printf '%02d' $((10+i))):00:00Z\"},
      \"cloud_coverage\": {\"N\": \"$((i*5))\"},
      \"status\": {\"S\": \"processed\"}
    }" \
    --endpoint-url http://localhost:4566 \
    --region us-east-1
done
```

- [ ] ✓ 2 escenas insertadas

### 5.2 Sincronizar

```bash
docker-compose restart sync
sleep 3
```

- [ ] ✓ Sync reiniciado

### 5.3 Verificar en MySQL

```bash
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
  "SELECT produccion_id, scene_id, satellite, cloud_coverage, status FROM monitoreo_escenas WHERE produccion_id = 9001;"
```

- [ ] ✓ Muestra 2 filas
- [ ] ✓ Todos los campos están presentes
- [ ] ✓ satellite = "Sentinel-2"

---

## Sección 6: Test 4 - Cambios de Estado

### 6.1 Cambiar estado en DynamoDB

```bash
aws dynamodb update-item \
  --table-name monitoring_producciones \
  --key '{"produccion_id": {"N": "9001"}}' \
  --update-expression "SET #s = :status" \
  --expression-attribute-names '{"#s": "status"}' \
  --expression-attribute-values '{":status": {"S": "completed"}}' \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

- [ ] ✓ Update completado (respuesta vacía)

### 6.2 Sincronizar

```bash
docker-compose restart sync
sleep 3
```

- [ ] ✓ Sync reiniciado

### 6.3 Verificar estado en MySQL

```bash
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
  "SELECT produccion_id, status, fecha_actualizacion FROM monitoreo_produccion_temporal WHERE produccion_id = 9001;"
```

- [ ] ✓ status = "completed"
- [ ] ✓ fecha_actualizacion se actualizó (es reciente)

---

## Sección 7: Test 5 - S3

### 7.1 Crear archivo en S3

```bash
echo "Test scene data" > /tmp/test-scene.tif
aws s3 cp /tmp/test-scene.tif s3://agro-sentinel-bucket/scenes/9001/TEST-SCENE.tif \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

- [ ] ✓ Upload completado

### 7.2 Verificar en S3

```bash
aws s3 ls s3://agro-sentinel-bucket/scenes/9001/ \
  --endpoint-url http://localhost:4566 \
  --region us-east-1
```

- [ ] ✓ Muestra TEST-SCENE.tif

### 7.3 Descargar y verificar

```bash
aws s3 cp s3://agro-sentinel-bucket/scenes/9001/TEST-SCENE.tif /tmp/downloaded.tif \
  --endpoint-url http://localhost:4566 \
  --region us-east-1

cat /tmp/downloaded.tif
```

- [ ] ✓ Archivo descargado correctamente
- [ ] ✓ Contenido es "Test scene data"

---

## Sección 8: Test 6 - Logs y Monitoreo

### 8.1 Ver logs del sync

```bash
docker-compose logs sync | tail -50
```

- [ ] ✓ No hay mensajes de ERROR
- [ ] ✓ Hay mensajes de sincronización
- [ ] ✓ No hay stack traces

### 8.2 Ver logs del API

```bash
docker-compose logs api | tail -20
```

- [ ] ✓ No hay errores de conexión
- [ ] ✓ API está escuchando en puerto correcto

### 8.3 Ver logs del Worker

```bash
docker-compose logs worker | tail -20
```

- [ ] ✓ Worker está activo
- [ ] ✓ No hay errores de conexión

---

## Sección 9: Test 7 - Estrés (Opcional)

### 9.1 Insertar 20 producciones

```bash
for i in {1..20}; do
  PROD_ID=$((9100 + i))
  aws dynamodb put-item \
    --table-name monitoring_producciones \
    --item "{
      \"produccion_id\": {\"N\": \"$PROD_ID\"},
      \"folio\": {\"S\": \"STRESS-$PROD_ID\"},
      \"articulo_id\": {\"N\": \"1\"},
      \"centro_costo_id\": {\"N\": \"1\"},
      \"status\": {\"S\": \"monitoring\"},
      \"fecha\": {\"S\": \"2026-09-04\"},
      \"cantidad_escenas\": {\"N\": \"0\"}
    }" \
    --endpoint-url http://localhost:4566 \
    --region us-east-1 &
done
wait
```

- [ ] ✓ 20 producciones insertadas

### 9.2 Sincronizar

```bash
docker-compose restart sync
sleep 5
```

- [ ] ✓ Sync no se cuelga

### 9.3 Verificar todas en MySQL

```bash
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
  "SELECT COUNT(*) FROM monitoreo_produccion_temporal WHERE folio LIKE 'STRESS-%';"
```

- [ ] ✓ Resultado = 20
- [ ] ✓ No hubo timeout

---

## Sección 10: Limpieza

### 10.1 Limpiar datos de prueba

```bash
# MySQL
mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
DELETE FROM monitoreo_produccion_temporal WHERE produccion_id >= 9000;
DELETE FROM monitoreo_escenas WHERE produccion_id >= 9000;
EOF

# DynamoDB
for i in {1..120}; do
  PROD_ID=$((9000 + i))
  aws dynamodb delete-item \
    --table-name monitoring_producciones \
    --key "{\"produccion_id\": {\"N\": \"$PROD_ID\"}}" \
    --endpoint-url http://localhost:4566 \
    --region us-east-1 2>/dev/null || true
done
```

- [ ] ✓ Datos de prueba eliminados

### 10.2 Generar reporte

```bash
# Consultar estado final
echo "=== Estado Final ==="
echo "Producciones en MySQL:"
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SELECT COUNT(*) FROM monitoreo_produccion_temporal;"

echo "Escenas en MySQL:"
mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SELECT COUNT(*) FROM monitoreo_escenas;"

echo "Producciones en DynamoDB:"
aws dynamodb scan --table-name monitoring_producciones --select COUNT \
  --endpoint-url http://localhost:4566 --region us-east-1 | grep Count
```

- [ ] ✓ Números finales documentados

---

## Resumen Final

### Checklist de Éxito

- [ ] Todos los servicios están activos
- [ ] MySQL recibe datos de DynamoDB correctamente
- [ ] Múltiples producciones se sincronizan
- [ ] Escenas se sincronizan correctamente
- [ ] Cambios de estado se replican
- [ ] S3 funciona correctamente
- [ ] No hay errores en logs
- [ ] Performance es aceptable

### Problemas Encontrados

```
[Documenta aquí cualquier problema encontrado]

1. _________________________________
2. _________________________________
3. _________________________________
```

### Recomendaciones

```
[Documenta aquí cualquier mejora o ajuste necesario]

1. _________________________________
2. _________________________________
3. _________________________________
```

### Fecha de Prueba

- **Fecha**: ___/___/______
- **Probador**: _____________________
- **Entorno**: [ ] Linux  [ ] Mac  [ ] Windows
- **Resultado**: [ ] PASÓ  [ ] FALLÓ

---

## Pasos Siguientes

Si todos los tests pasaron:

1. **Ejecutar suite de tests completa**
   ```bash
   bash scripts/test-sync-flow.sh
   ```

2. **Revisar documentación**
   - STARTUP_GUIDE.md
   - SYNC_TESTING_GUIDE.md
   - SYNC_TROUBLESHOOTING.md

3. **Commit de cambios**
   ```bash
   git add .
   git commit -m "test: sync testing checklist completed"
   ```

4. **Crear PR o push a main**
   ```bash
   git push origin feat/agro-sentinel-worker
   ```

