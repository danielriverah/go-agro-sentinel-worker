# Sync Troubleshooting - Agro Sentinel Worker

Guía de solución de problemas comunes durante setup y testing de sincronización.

## Tabla de Contenidos

- [Problemas de Startup](#problemas-de-startup)
- [Problemas de Conectividad](#problemas-de-conectividad)
- [Problemas de Sincronización](#problemas-de-sincronización)
- [Problemas de Performance](#problemas-de-performance)
- [Problemas de Datos](#problemas-de-datos)
- [Problemas Específicos por SO](#problemas-específicos-por-so)

---

## Problemas de Startup

### Problema: Docker no inicia

**Síntomas:**
```
docker: command not found
// O
Docker daemon is not running
```

**Solución:**

1. **Verificar instalación**
   ```bash
   docker --version
   docker ps
   ```

2. **Inicia Docker Desktop**
   - Windows/Mac: Abre Docker Desktop desde aplicaciones
   - Linux: `sudo systemctl start docker`

3. **Verificar permisos (Linux)**
   ```bash
   sudo usermod -aG docker $USER
   newgrp docker
   ```

4. **Reinicia el terminal**
   ```bash
   exit  # Cierra terminal
   # Abre nueva terminal
   docker ps
   ```

---

### Problema: docker-compose no funciona

**Síntomas:**
```
docker-compose: command not found
// O
docker-compose version
command not found
```

**Solución:**

1. **Instala Docker Compose**
   ```bash
   # Windows/Mac: Ya incluido en Docker Desktop
   # Linux:
   sudo curl -L "https://github.com/docker/compose/releases/download/v2.20.0/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
   sudo chmod +x /usr/local/bin/docker-compose
   ```

2. **Verifica instalación**
   ```bash
   docker-compose --version
   ```

---

### Problema: Puerto ya en uso

**Síntomas:**
```
ERROR: for mysql Cannot start service mysql: driver failed programming external connectivity on endpoint go-agro-sentinel-mysql: Bind for 0.0.0.0:3309 failed: port is already allocated
```

**Solución:**

1. **Encuentra qué ocupa el puerto**
   ```bash
   # Linux/Mac:
   lsof -i :3309
   
   # Windows (PowerShell):
   netstat -ano | findstr :3309
   ```

2. **Opción A: Detener el proceso**
   ```bash
   # Si es un contenedor viejo:
   docker-compose down
   
   # Si es otra aplicación:
   kill -9 <PID>  # Linux/Mac
   taskkill /PID <PID> /F  # Windows
   ```

3. **Opción B: Cambiar puerto en docker-compose.yml**
   ```yaml
   # Cambiar:
   ports:
     - "3309:3306"
   # Por:
   ports:
     - "3310:3306"
   
   # Luego conectar con:
   mysql -h 127.0.0.1 -P 3310 -u root -proot
   ```

4. **Opción C: Usar socket de Docker**
   ```bash
   # En algunos casos, usa docker exec para conectar
   docker-compose exec mysql mysql -u root -proot agro
   ```

---

### Problema: No hay suficiente espacio en disco

**Síntomas:**
```
ERROR: no space left on device
write error: No space left on device
```

**Solución:**

1. **Verificar espacio disponible**
   ```bash
   # Linux/Mac:
   df -h
   
   # Windows (PowerShell):
   Get-Volume
   ```

2. **Limpiar Docker**
   ```bash
   # Eliminar imágenes no usadas
   docker image prune -a
   
   # Eliminar volúmenes no usados
   docker volume prune
   
   # Limpiar todo (CUIDADO: borra datos)
   docker system prune -a
   ```

3. **Aumentar espacio**
   - Desinstala aplicaciones innecesarias
   - Mueve Docker a otra partición (Docker Desktop: Settings > Resources)
   - Aumenta tamaño del disco virtual

---

### Problema: Permisos denegados al ejecutar scripts

**Síntomas:**
```
Permission denied: ./scripts/setup-local-env.sh
// O
El proceso no tiene acceso a la ruta especificada
```

**Solución:**

1. **Linux/Mac: Dar permisos de ejecución**
   ```bash
   chmod +x scripts/setup-local-env.sh
   chmod +x scripts/test-sync-flow.sh
   ./scripts/setup-local-env.sh
   ```

2. **Windows: Ejecutar como PowerShell**
   ```powershell
   # Si está bloqueado:
   Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
   
   # Luego:
   .\scripts\setup-local-env.ps1
   ```

---

## Problemas de Conectividad

### Problema: MySQL no responde a health check

**Síntomas:**
```
go-agro-sentinel-mysql   Unhealthy
// O
ERROR 2003 (HY000): Can't connect to MySQL server on '127.0.0.1'
```

**Solución:**

1. **Ver logs de MySQL**
   ```bash
   docker-compose logs mysql | tail -50
   ```

2. **Esperar más tiempo (primera ejecución)**
   ```bash
   # MySQL tarda 30+ segundos en iniciarse
   docker-compose ps  # Repetir hasta que sea "healthy"
   ```

3. **Reiniciar MySQL**
   ```bash
   docker-compose down
   docker-compose up -d mysql
   # Esperar 30 segundos
   ```

4. **Verificar memoria de Docker**
   - Docker Desktop: Settings > Resources
   - Aumentar RAM a mínimo 2GB
   - Reiniciar Docker

5. **Conectar directamente sin health check**
   ```bash
   # Mientras esperas health check
   docker-compose exec mysql bash
   mysql -u root -proot agro
   ```

---

### Problema: LocalStack no inicia correctamente

**Síntomas:**
```
curl: (7) Failed to connect to localhost:4566
// O
Connection refused
```

**Solución:**

1. **Ver logs de LocalStack**
   ```bash
   docker-compose logs localstack | tail -100
   ```

2. **Reiniciar LocalStack**
   ```bash
   docker-compose restart localstack
   sleep 10
   ```

3. **Verificar puerto**
   ```bash
   curl -s http://localhost:4566/health
   # Debería retornar algo
   ```

4. **Si sigue fallando, reconstruir**
   ```bash
   docker-compose down
   docker-compose up -d localstack
   sleep 15
   ```

---

### Problema: AWS CLI no puede conectar a LocalStack

**Síntomas:**
```
Unable to locate credentials
// O
Could not connect to the endpoint URL: http://localhost:4566
```

**Solución:**

1. **Instalar/Verificar AWS CLI**
   ```bash
   aws --version
   
   # Si no está:
   pip install awscli
   ```

2. **Configurar credenciales dummy (para LocalStack)**
   ```bash
   aws configure
   # AWS Access Key ID: test
   # AWS Secret Access Key: test
   # Default region: us-east-1
   # Default output format: json
   ```

3. **Verificar que LocalStack está corriendo**
   ```bash
   docker-compose ps | grep localstack
   # Debería estar "Up"
   ```

4. **Probar conexión**
   ```bash
   aws dynamodb list-tables \
     --endpoint-url http://localhost:4566 \
     --region us-east-1
   ```

---

### Problema: MySQL Client no instalado

**Síntomas:**
```
mysql: command not found
```

**Solución:**

1. **Windows**
   ```powershell
   # Con Chocolatey:
   choco install mysql
   
   # O descargar desde:
   # https://dev.mysql.com/downloads/mysql/
   ```

2. **Linux (Ubuntu/Debian)**
   ```bash
   sudo apt-get install mysql-client
   ```

3. **Mac**
   ```bash
   brew install mysql-client
   ```

4. **Alternativa: Usar container**
   ```bash
   docker-compose exec mysql mysql -u root -proot agro
   ```

---

## Problemas de Sincronización

### Problema: Datos no aparecen en MySQL después de insertar en DynamoDB

**Síntomas:**
```
Inserto en DynamoDB pero no veo nada en MySQL
// O
SELECT COUNT(*) FROM monitoreo_produccion_temporal;
# Devuelve 0
```

**Solución paso a paso:**

1. **Verificar que está en DynamoDB**
   ```bash
   aws dynamodb scan \
     --table-name monitoring_producciones \
     --select COUNT \
     --endpoint-url http://localhost:4566 \
     --region us-east-1
   # Debería mostrar Count > 0
   ```

2. **Ver logs del Sync Service**
   ```bash
   docker-compose logs sync | tail -100 | grep -i "error\|sync\|processing"
   ```

3. **Reiniciar Sync Service**
   ```bash
   docker-compose restart sync
   sleep 3
   docker-compose logs -f sync --tail 50
   # Esperar hasta ver "Sync completed"
   ```

4. **Verificar conexión a MySQL desde Sync**
   ```bash
   docker-compose exec sync mysql -h go-agro-sentinel-mysql -u root -proot agro -e "SELECT COUNT(*) FROM monitoreo_produccion_temporal;"
   ```

5. **Verificar tabla existe**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SHOW TABLES;"
   # Debe incluir "monitoreo_produccion_temporal"
   ```

6. **Ver estructura de tabla**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "DESCRIBE monitoreo_produccion_temporal;"
   ```

7. **Fuerza ejecución de Sync**
   ```bash
   docker-compose down sync
   docker-compose up -d sync
   sleep 5
   ```

---

### Problema: Sync service está corriendo pero no sincroniza

**Síntomas:**
```
docker-compose ps muestra sync como "Up"
Pero los datos no se sincronizan
```

**Solución:**

1. **Ver logs detallados**
   ```bash
   docker-compose logs -f sync --tail 200
   # Buscar "error", "fail", "connection"
   ```

2. **Verificar configuración**
   ```bash
   docker-compose config | grep -A 20 "sync:"
   # Verificar variables de entorno
   ```

3. **Verificar conectividad interna**
   ```bash
   docker-compose exec sync bash
   
   # Dentro del container:
   # Probar MySQL
   mysql -h go-agro-sentinel-mysql -u root -proot agro -e "SELECT 1"
   
   # Probar DynamoDB
   aws dynamodb list-tables --endpoint-url http://go-agro-sentinel-localstack:4566 --region us-east-1
   
   exit  # Salir del container
   ```

4. **Buscar errores específicos en logs**
   ```bash
   docker-compose logs sync | grep -E "ERROR|FAIL|panic" | head -20
   ```

---

### Problema: Sincronización es lentísima

**Síntomas:**
```
Inserto 10 registros, pero demora 5+ minutos en sincronizar
```

**Solución:**

1. **Ver recursos de Docker**
   ```bash
   docker stats
   # Buscar si CPU o RAM está al máximo
   ```

2. **Aumentar recursos de Docker**
   - Windows/Mac: Docker Desktop > Settings > Resources
   - Aumentar CPU y RAM
   - Reiniciar Docker

3. **Verificar logs de performance**
   ```bash
   docker-compose logs sync | grep -i "time\|duration\|lag"
   ```

4. **Verificar conexión de red**
   ```bash
   # Medir latencia a MySQL
   docker-compose exec sync bash -c "time mysql -h go-agro-sentinel-mysql -u root -proot agro -e 'SELECT 1'"
   
   # Debería ser <100ms
   ```

5. **Optimizar configuración de Sync**
   - Ver `configs/config.yaml`
   - Aumentar batch size
   - Aumentar worker threads
   - Ajustar poll interval

---

## Problemas de Performance

### Problema: Docker consume demasiada CPU/RAM

**Síntomas:**
```
Computadora muy lenta
Ventilador ruidoso
```

**Solución:**

1. **Verificar uso de recursos**
   ```bash
   docker stats
   ```

2. **Limitar recursos específicos en docker-compose.yml**
   ```yaml
   services:
     mysql:
       deploy:
         resources:
           limits:
             cpus: '1'
             memory: 1G
   ```

3. **Parar contenedores no necesarios**
   ```bash
   # Parar solo API si solo necesitas sync
   docker-compose stop api
   
   # Parar solo worker si no necesitas procesar jobs
   docker-compose stop worker
   ```

4. **Reducir recursos en Docker Desktop**
   - Settings > Resources > CPU/Memory
   - Reducir a lo mínimo (1 CPU, 1-2 GB RAM)

---

### Problema: Conexiones a base de datos timeout

**Síntomas:**
```
ERROR 1040 (HY000): Too many connections
// O
Connection timeout
```

**Solución:**

1. **Ver conexiones activas**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SHOW PROCESSLIST;"
   ```

2. **Matar conexiones huérfanas**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot -e "KILL <PID>;"
   ```

3. **Aumentar max connections en MySQL**
   - Editar `docker-compose.yml`
   ```yaml
   mysql:
     environment:
       MYSQL_INIT_COMMAND: "SET global max_connections=1000"
   ```

4. **Reiniciar MySQL**
   ```bash
   docker-compose restart mysql
   ```

---

## Problemas de Datos

### Problema: Datos duplicados en MySQL

**Síntomas:**
```
Inserto 1 vez pero aparece 2-3 veces en MySQL
```

**Solución:**

1. **Ver registros duplicados**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
   SELECT produccion_id, COUNT(*) as cantidad
   FROM monitoreo_produccion_temporal
   GROUP BY produccion_id
   HAVING cantidad > 1;
   EOF
   ```

2. **Eliminar duplicados**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot agro << EOF
   DELETE FROM monitoreo_produccion_temporal WHERE 
   produccion_id IN (
     SELECT produccion_id FROM (
       SELECT * FROM monitoreo_produccion_temporal
       WHERE produccion_id IN (
         SELECT produccion_id FROM monitoreo_produccion_temporal
         GROUP BY produccion_id HAVING COUNT(*) > 1
       )
     ) AS tmp
   ) AND fecha_sincronizacion NOT IN (
     SELECT MAX(fecha_sincronizacion)
     FROM monitoreo_produccion_temporal
     GROUP BY produccion_id
   );
   EOF
   ```

3. **Prevenir duplicados en el futuro**
   - Usar UNIQUE constraint en SQL
   - Implementar idempotencia en Sync Service
   - Ver código en `internal/sync/`

---

### Problema: Datos no se sincronizan correctamente

**Síntomas:**
```
Sincronización incompleta
Campos faltantes
```

**Solución:**

1. **Verificar estructura de datos en DynamoDB**
   ```bash
   aws dynamodb get-item \
     --table-name monitoring_producciones \
     --key '{"produccion_id": {"N": "1"}}' \
     --endpoint-url http://localhost:4566 \
     --region us-east-1
   ```

2. **Comparar con estructura esperada en MySQL**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "DESCRIBE monitoreo_produccion_temporal;"
   ```

3. **Ver mapeo de campos en código**
   - Archivo: `internal/sync/mapper.go`
   - Verificar que campos DynamoDB están mapeados a columnas MySQL

4. **Ver logs de mapeo**
   ```bash
   docker-compose logs sync | grep -i "map\|field\|attribute"
   ```

---

### Problema: Timestamps están incorrectos

**Síntomas:**
```
fecha_sincronizacion está NULL
// O
Timestamps muy antiguos
```

**Solución:**

1. **Verificar timezone en DynamoDB**
   ```bash
   aws dynamodb get-item \
     --table-name monitoring_producciones \
     --key '{"produccion_id": {"N": "1"}}' \
     --endpoint-url http://localhost:4566 \
     --region us-east-1 | grep -i "fecha\|time"
   ```

2. **Verificar timezone en MySQL**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SELECT @@global.time_zone, @@session.time_zone;"
   ```

3. **Establecer timezone correcto**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot -e "SET GLOBAL time_zone='UTC';"
   ```

4. **Verificar que se actualiza en insert**
   ```bash
   mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e \
     "SELECT produccion_id, fecha_sincronizacion FROM monitoreo_produccion_temporal LIMIT 5;"
   ```

---

## Problemas Específicos por SO

### Windows

#### Problema: WSL no está habilitado

**Síntomas:**
```
Docker requires WSL 2
Hyper-V not enabled
```

**Solución:**

1. **Habilitar Hyper-V**
   ```powershell
   # Run as Administrator
   Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All
   ```

2. **Instalar WSL2**
   ```powershell
   wsl --install
   # Reiniciar
   ```

3. **Configurar Docker para usar WSL2**
   - Docker Desktop > Settings > General
   - Marcar "Use the WSL 2 based engine"
   - Reiniciar Docker

---

#### Problema: Scripts bash no funcionan en PowerShell

**Síntomas:**
```
.\scripts\setup-local-env.sh
command not found: setup-local-env.sh
```

**Solución:**

1. **Usar versión PowerShell del script**
   ```powershell
   .\scripts\setup-local-env.ps1
   ```

2. **O usar WSL bash**
   ```powershell
   wsl bash scripts/setup-local-env.sh
   ```

3. **O instalar Git Bash** (trae bash para Windows)
   ```powershell
   # Usar Git Bash en lugar de PowerShell
   ```

---

### Linux

#### Problema: Permisos de socket de Docker

**Síntomas:**
```
Got permission denied while trying to connect to the Docker daemon socket
```

**Solución:**

```bash
# Opción 1: Usar sudo
sudo docker-compose up -d

# Opción 2: Agregar usuario al grupo docker
sudo usermod -aG docker $USER
newgrp docker
docker-compose up -d  # Sin sudo
```

---

### Mac

#### Problema: "Cannot connect to Docker daemon"

**Síntomas:**
```
Cannot connect to the Docker daemon at unix:///var/run/docker.sock
```

**Solución:**

1. **Abrir Docker Desktop**
   ```bash
   open /Applications/Docker.app
   ```

2. **Esperar a que inicie**
   ```bash
   # Esperar a que Docker Desktop muestre "Docker is running"
   ```

3. **Probar conexión**
   ```bash
   docker ps
   ```

---

#### Problema: Performance lenta en Mac

**Síntomas:**
```
Sync muy lento
MySQL responde lentamente
```

**Solución:**

1. **Aumentar recursos de Docker**
   - Docker Desktop > Preferences > Resources
   - Aumentar CPUs y Memory (mínimo 4 CPUs, 4GB RAM)
   - Reiniciar Docker

2. **Usar native virtualization**
   - Docker > Preferences > Experimental features
   - Habilitar "Use native virtualization framework"

---

## Verificación Final

Después de resolver un problema, ejecuta:

```bash
# Verificar que todo funciona
docker-compose ps
# Todos deben estar "Up"

# Probar sync
docker-compose restart sync
sleep 3

# Ver logs
docker-compose logs sync | tail -20
# No debe haber errores

# Probar con un dato
aws dynamodb put-item \
  --table-name monitoring_producciones \
  --item '{"produccion_id": {"N": "999"}, "folio": {"S": "TEST"}, "status": {"S": "test"}}' \
  --endpoint-url http://localhost:4566 --region us-east-1

docker-compose restart sync
sleep 3

mysql -h 127.0.0.1 -P 3309 -u root -proot agro -e "SELECT * FROM monitoreo_produccion_temporal WHERE folio = 'TEST';"
# Debería aparecer
```

---

## Reportar Problemas

Si el problema persiste después de seguir esta guía:

1. **Recolecta información**
   ```bash
   # Guardar en archivo de logs
   docker-compose logs > docker-logs.txt 2>&1
   docker version > system-info.txt 2>&1
   docker stats > docker-stats.txt 2>&1
   ```

2. **Documenta el problema**
   - Descripción clara
   - Pasos para reproducir
   - Salida de logs
   - SO y versiones (Docker, etc)

3. **Crea issue en Git**
   ```bash
   git issue create --title "Sync troubleshooting" --body @docker-logs.txt
   ```

