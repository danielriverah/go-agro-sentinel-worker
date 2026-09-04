# Guía de Importación de CSV - Sistema Agro Sentinel

## Descripción General

Esta guía proporciona instrucciones detalladas para importar datos desde archivos CSV a la base de datos del Sistema Agro Sentinel. Incluye ejemplos completos, validaciones y mejores prácticas.

---

## Tabla de Contenidos

1. [Requisitos previos](#requisitos-previos)
2. [Estructura de los CSV](#estructura-de-los-csv)
3. [Directorios y archivos](#directorios-y-archivos)
4. [Métodos de importación](#métodos-de-importación)
5. [Validaciones y errores comunes](#validaciones-y-errores-comunes)
6. [Ejemplos paso a paso](#ejemplos-paso-a-paso)
7. [Troubleshooting](#troubleshooting)
8. [Mejores prácticas](#mejores-prácticas)

---

## Requisitos previos

### Base de datos
- MySQL Server 5.7 o superior
- Base de datos "agro" creada
- Tablas creadas usando scripts en `scripts/phase1-create-tables.sql`

### Permisos MySQL
```sql
-- El usuario MySQL debe tener permisos FILE
GRANT FILE ON *.* TO 'admin'@'localhost';
FLUSH PRIVILEGES;

-- Habilitar importación local
SET GLOBAL local_infile = 1;
```

### Sistema operativo
- Bash (para scripts .sh)
- MySQL client instalado y en PATH
- Acceso a archivos CSV

---

## Estructura de los CSV

### 1. ARTICULOS (articulos.csv)

Cultivos o artículos que se producen en los ranchos.

**Encabezado:**
```
articulo_id,nombre,variedad
```

**Campos:**
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| articulo_id | INT | Sí | ID único del artículo (autoincremental) |
| nombre | VARCHAR(200) | Sí | Nombre del cultivo (ej: Lechuga, Tomate) |
| variedad | VARCHAR(200) | No | Variedad específica (ej: Latina, Cherry) |

**Ejemplo:**
```csv
articulo_id,nombre,variedad
1,Lechuga,Latina
2,Tomate,Cherry
3,Pepino,Tipo Japonés
```

**Validaciones:**
- articulo_id debe ser único
- nombre no puede estar vacío
- variedad es opcional

---

### 2. CENTROS_COSTOS (centros_costos.csv)

Ranchos o centros de costo donde se producen los cultivos.

**Encabezado:**
```
centro_costo_id,nombre
```

**Campos:**
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| centro_costo_id | INT | Sí | ID único del centro (autoincremental) |
| nombre | VARCHAR(200) | Sí | Nombre del rancho o centro de costo |

**Ejemplo:**
```csv
centro_costo_id,nombre
1,Rancho Los Andes
2,Rancho San Miguel
3,Rancho El Peñol
```

**Validaciones:**
- centro_costo_id debe ser único
- nombre no puede estar vacío

---

### 3. PRODUCCIONES (producciones.csv)

Registro de producciones realizadas con detalles de cultivos y ranchos.

**Encabezado:**
```
folio,articulo_id,fecha,hora,fecha_cierre,hora_cierre,usuario,estatus,aplicado,cantidad,centro_costo_id,monitoring
```

**Campos:**
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| folio | VARCHAR(45) | Sí | Identificador único de la producción (ej: CSJ2601-17-A) |
| articulo_id | INT | Sí | ID del artículo (debe existir en articulos) |
| fecha | DATE | Sí | Fecha de inicio (formato: YYYY-MM-DD) |
| hora | TIME | Sí | Hora de inicio (formato: HH:MM:SS) |
| fecha_cierre | DATE | No | Fecha de cierre (opcional) |
| hora_cierre | TIME | No | Hora de cierre (opcional) |
| usuario | VARCHAR(200) | No | Usuario que registró la producción |
| estatus | CHAR(1) | Sí | N=Normal, T=Terminada, C=Cancelada |
| aplicado | CHAR(1) | Sí | S=Sí, N=No |
| cantidad | DECIMAL(20,6) | No | Área en hectáreas |
| centro_costo_id | INT | Sí | ID del rancho (debe existir en centros_costos) |
| monitoring | TINYINT | Sí | 0=No monitorear, 1=Monitorear |

**Ejemplo:**
```csv
folio,articulo_id,fecha,hora,fecha_cierre,hora_cierre,usuario,estatus,aplicado,cantidad,centro_costo_id,monitoring
CSJ2601-17-A,1,2026-06-03,08:30:00,,,"admin",N,S,2.50,1,1
CSJ2602-24,2,2026-06-10,09:00:00,,,"admin",N,S,3.75,2,1
```

**Validaciones:**
- folio debe ser único
- articulo_id y centro_costo_id deben existir
- fecha y hora son obligatorias
- estatus debe ser una de: N, T, C
- aplicado debe ser: S, N
- monitoring debe ser: 0, 1

---

### 4. ZONAS_PRODUCCIONES (zonas_producciones.csv)

Zonas geográficas dentro de los ranchos para delimitar áreas de cultivo.

**Encabezado:**
```
zona_produccion_id,nombre,nombre_corto,estatus,area,centro_costo_id,poligono
```

**Campos:**
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| zona_produccion_id | INT | Sí | ID único de la zona |
| nombre | VARCHAR(200) | Sí | Nombre completo de la zona |
| nombre_corto | VARCHAR(200) | Sí | Abreviatura (ej: ZNA, ZSA) |
| estatus | CHAR(1) | Sí | P=Producción, I=Inactiva |
| area | DECIMAL(20,6) | No | Área en hectáreas |
| centro_costo_id | INT | No | ID del rancho relacionado |
| poligono | MEDIUMTEXT | No | Coordenadas GeoJSON del polígono |

**Ejemplo:**
```csv
zona_produccion_id,nombre,nombre_corto,estatus,area,centro_costo_id,poligono
1,Zona Norte Los Andes,ZNA,P,5.50,1,"[[21.10535, -100.93089], [21.10535, -100.92730], [21.1068, -100.92730], [21.1068, -100.93089]]"
2,Zona Sur Los Andes,ZSA,P,4.20,1,"[[21.10200, -100.93000], [21.10200, -100.92500], [21.10500, -100.92500], [21.10500, -100.93000]]"
```

**Notas sobre GeoJSON:**
- Formato: Array de coordenadas [latitud, longitud]
- Debe estar entrecomillado en el CSV
- Representa un polígono cerrado
- Última coordenada puede ser igual a la primera

**Validaciones:**
- zona_produccion_id debe ser único
- nombre no puede estar vacío
- nombre_corto no puede estar vacío
- estatus debe ser: P, I
- poligono debe ser válido JSON (si se proporciona)

---

### 5. ASIGNACIONES_ZONAS_PRODUCCIONES (asignaciones_zonas_producciones.csv)

Tabla relacional que asigna zonas a producciones específicas.

**Encabezado:**
```
asignacion_zona_prod_id,produccion_id,zona_produccion_id,tipo_asignacion,area,poligono
```

**Campos:**
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| asignacion_zona_prod_id | INT | Sí | ID único de la asignación |
| produccion_id | INT | Sí | ID de la producción |
| zona_produccion_id | INT | Sí | ID de la zona |
| tipo_asignacion | CHAR(1) | Sí | T=Total, P=Parcial |
| area | DECIMAL(20,6) | No | Área asignada en hectáreas |
| poligono | MEDIUMTEXT | No | Coordenadas GeoJSON específicas |

**Ejemplo:**
```csv
asignacion_zona_prod_id,produccion_id,zona_produccion_id,tipo_asignacion,area,poligono
1,1,1,P,2.50,"[[21.10535, -100.93089], [21.10535, -100.92730], [21.1068, -100.92730], [21.1068, -100.93089]]"
2,2,3,P,3.75,"[[21.11000, -100.94000], [21.11000, -100.93500], [21.11300, -100.93500], [21.11300, -100.94000]]"
```

**Validaciones:**
- asignacion_zona_prod_id debe ser único
- produccion_id y zona_produccion_id deben existir
- tipo_asignacion debe ser: T, P
- El área asignada no debe superar el área de la zona

---

### 6. S3_MONITORING_ESCENA_IA_RESUMEN (s3_monitoring_escena_ia_resumen.csv)

Resumen de análisis de IA sobre escenas satelitales.

**Encabezado:**
```
s3_monitoring_escena_id,estado_clave,estado_general,riesgo_nivel,riesgo_motivo,fecha_analisis,json_original
```

**Campos:**
| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| s3_monitoring_escena_id | BIGINT | Sí | ID único de la escena |
| estado_clave | VARCHAR(100) | No | crítico, alerta, normal, etc. |
| estado_general | VARCHAR(100) | No | Estado general de la escena |
| riesgo_nivel | VARCHAR(100) | No | alto, medio, bajo |
| riesgo_motivo | TEXT | No | Descripción del riesgo detectado |
| fecha_analisis | DATETIME | No | Cuándo se realizó el análisis |
| json_original | LONGTEXT | No | JSON completo de la respuesta IA |

**Ejemplo:**
```csv
s3_monitoring_escena_id,estado_clave,estado_general,riesgo_nivel,riesgo_motivo,fecha_analisis,json_original
1001,normal,OK,bajo,Sin anomalías detectadas,2026-06-03 10:30:00,"{""status"": ""normal"", ""score"": 0.95}"
1002,alerta,ALERTA,medio,Posible enfermedad fúngica,2026-06-10 14:15:00,"{""status"": ""alerta"", ""disease"": ""powdery_mildew""}"
1003,crítico,CRÍTICO,alto,Alto estrés hídrico,2026-06-15 09:45:00,"{""status"": ""critico"", ""issue"": ""water_stress""}"
```

**Notas sobre JSON:**
- Las comillas internas deben escaparse (doble comilla)
- Debe estar entrecomillado en el CSV
- Puede contener cualquier estructura válida JSON

**Validaciones:**
- s3_monitoring_escena_id debe ser único
- estado_clave debe ser uno de: crítico, alerta, normal
- riesgo_nivel debe ser uno de: alto, medio, bajo
- json_original debe ser JSON válido si se proporciona

---

## Directorios y archivos

### Estructura de directorios

```
go-agro-sentinel-worker/
├── data/
│   └── csv/
│       └── examples/
│           ├── articulos.csv
│           ├── centros_costos.csv
│           ├── producciones.csv
│           ├── zonas_producciones.csv
│           ├── asignaciones_zonas_producciones.csv
│           └── s3_monitoring_escena_ia_resumen.csv
├── scripts/
│   ├── import-from-csv.sql
│   └── import-csv.sh
└── docs/
    └── CSV_IMPORT_GUIDE.md
```

### Archivos de ejemplo

Todos los archivos de ejemplo están en `data/csv/examples/` y están listos para usar como plantilla o importarse directamente.

---

## Métodos de importación

### Método 1: Script SQL completo (Recomendado)

El script `scripts/import-from-csv.sql` importa todos los CSV de una vez con validaciones.

**Prerequisitos:**
```bash
# En MySQL console
SET GLOBAL local_infile = 1;
```

**Ejecución:**
```bash
# Desde Windows
mysql -h localhost -u admin -p admin agro < scripts/import-from-csv.sql

# Desde MySQL Workbench
File → Open SQL Script → scripts/import-from-csv.sql → Execute
```

**Ventajas:**
- Importa todas las tablas en orden correcto
- Incluye validaciones
- Verifica integridad referencial
- Control transaccional

---

### Método 2: Script Bash (Recomendado para automatización)

El script `scripts/import-csv.sh` ofrece funcionalidad avanzada para importar, validar y respaldar datos.

**Permisos:**
```bash
# Hacer el script ejecutable
chmod +x scripts/import-csv.sh
```

**Uso básico:**
```bash
# Importar una tabla específica
./scripts/import-csv.sh import articulos data/csv/examples/articulos.csv

# Importar todos los CSV
./scripts/import-csv.sh import-all

# Validar un CSV
./scripts/import-csv.sh validate data/csv/examples/articulos.csv

# Listar tablas y registros
./scripts/import-csv.sh list-tables

# Crear backup
./scripts/import-csv.sh backup
```

**Variables de entorno:**
```bash
# Default: localhost, admin, admin, agro, 3306
export DB_HOST=localhost
export DB_USER=admin
export DB_PASS=admin
export DB_NAME=agro
export DB_PORT=3306

./scripts/import-csv.sh import-all
```

---

### Método 3: MySQL LOAD DATA INFILE (Manual)

Para importar un CSV específico manualmente:

```sql
SET GLOBAL local_infile = 1;

LOAD DATA LOCAL INFILE '/ruta/completa/articulos.csv'
INTO TABLE articulos
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(articulo_id, nombre, variedad);
```

---

### Método 4: MySQL Workbench (GUI)

1. Abre MySQL Workbench
2. Conecta a la base de datos "agro"
3. Click derecho en la tabla → "Table Data Import Wizard"
4. Selecciona el archivo CSV
5. Configura mapeo de columnas
6. Ejecuta la importación

---

## Validaciones y errores comunes

### Validaciones previas

Antes de importar, verificar:

```bash
# 1. Archivo existe
ls -la data/csv/examples/articulos.csv

# 2. Archivo tiene contenido
wc -l data/csv/examples/articulos.csv

# 3. Primer línea es el encabezado correcto
head -1 data/csv/examples/articulos.csv

# 4. Datos están bien formados
head -3 data/csv/examples/articulos.csv
```

---

### Errores comunes y soluciones

#### Error: "Can't find file"
```
ERROR: Can't find file './articulos.csv'
```
**Solución:** Usar ruta absoluta en LOAD DATA INFILE
```sql
-- Incorrecto
LOAD DATA LOCAL INFILE './articulos.csv' ...

-- Correcto
LOAD DATA LOCAL INFILE '/ruta/completa/articulos.csv' ...
```

---

#### Error: "User does not have FILE privilege"
```
ERROR 1045: Access denied; you need FILE privilege
```
**Solución:** Otorgar permisos a usuario MySQL
```sql
GRANT FILE ON *.* TO 'admin'@'localhost';
FLUSH PRIVILEGES;
```

---

#### Error: "local_infile is disabled"
```
ERROR 3948: Loading local data is disabled
```
**Solución:** Habilitar local_infile
```sql
SET GLOBAL local_infile = 1;
```

O en my.cnf:
```ini
[mysqld]
local-infile=1
```

---

#### Error: "Duplicate entry"
```
ERROR 1062: Duplicate entry '1' for key 'articulo_id'
```
**Solución:** Limpiar tabla antes de importar
```sql
-- Borrar datos existentes
DELETE FROM articulos;

-- Resetear auto increment
ALTER TABLE articulos AUTO_INCREMENT = 1;

-- Luego importar
LOAD DATA LOCAL INFILE ...
```

---

#### Error: "Foreign key constraint fails"
```
ERROR 1452: Cannot add or update a child row
```
**Solución:** Importar en orden correcto
1. articulos
2. centros_costos
3. zonas_producciones
4. producciones
5. asignaciones_zonas_producciones
6. s3_monitoring_escena_ia_resumen

---

#### Error: "Invalid date format"
```
ERROR 1366: Incorrect date value
```
**Solución:** Usar formato DATE correcto (YYYY-MM-DD)
```csv
-- Correcto
2026-06-03

-- Incorrecto
03/06/2026
06-03-2026
```

---

### Validaciones post-importación

```sql
-- Verificar cantidad de registros
SELECT 'articulos' as tabla, COUNT(*) as registros FROM articulos
UNION ALL
SELECT 'centros_costos', COUNT(*) FROM centros_costos
UNION ALL
SELECT 'producciones', COUNT(*) FROM producciones;

-- Verificar integridad referencial
SELECT * FROM producciones p
WHERE p.articulo_id NOT IN (SELECT articulo_id FROM articulos)
   OR p.centro_costo_id NOT IN (SELECT centro_costo_id FROM centros_costos);

-- Verificar datos duplicados
SELECT articulo_id, COUNT(*) as repeticiones
FROM articulos
GROUP BY articulo_id
HAVING COUNT(*) > 1;
```

---

## Ejemplos paso a paso

### Ejemplo 1: Importación inicial completa

```bash
# 1. Navegar al proyecto
cd /xampp/htdocs/WEB/clientes/DRH/go-agro-sentinel-worker

# 2. Hacer script ejecutable
chmod +x scripts/import-csv.sh

# 3. Validar conexión
scripts/import-csv.sh list-tables

# 4. Crear backup (por si acaso)
scripts/import-csv.sh backup

# 5. Importar todos los datos
scripts/import-csv.sh import-all

# 6. Verificar resultados
scripts/import-csv.sh list-tables
```

---

### Ejemplo 2: Importación selectiva

```bash
# Importar solo artículos
./scripts/import-csv.sh import articulos data/csv/examples/articulos.csv

# Importar solo centros de costo
./scripts/import-csv.sh import centros_costos data/csv/examples/centros_costos.csv

# Importar solo producciones
./scripts/import-csv.sh import producciones data/csv/examples/producciones.csv
```

---

### Ejemplo 3: Validar e importar con variables de entorno

```bash
# Configurar variables
export DB_HOST=192.168.1.100
export DB_USER=agro_user
export DB_PASS=secure_password
export DB_NAME=agro
export DB_PORT=3306

# Validar archivos
./scripts/import-csv.sh validate data/csv/examples/articulos.csv
./scripts/import-csv.sh validate data/csv/examples/producciones.csv

# Importar
./scripts/import-csv.sh import-all
```

---

### Ejemplo 4: Importar CSV personalizado

Crear archivo `data/csv/custom/mis_articulos.csv`:
```csv
articulo_id,nombre,variedad
11,Lechuga Oreja de Conejo,Roja
12,Tomate Roma,Roma
13,Lechuga Mantequilla,Mantequilla
```

Importar:
```bash
./scripts/import-csv.sh import articulos data/csv/custom/mis_articulos.csv
```

---

### Ejemplo 5: Limpiar e reimportar

```bash
# Conectar a MySQL
mysql -h localhost -u admin -p admin agro

# En MySQL:
USE agro;

-- Deshabilitar restricciones foráneas temporalmente
SET FOREIGN_KEY_CHECKS = 0;

-- Limpiar tablas
DELETE FROM asignaciones_zonas_producciones;
DELETE FROM producciones;
DELETE FROM zonas_producciones;
DELETE FROM articulos;
DELETE FROM centros_costos;

-- Resetear auto increment
ALTER TABLE articulos AUTO_INCREMENT = 1;
ALTER TABLE centros_costos AUTO_INCREMENT = 1;
ALTER TABLE producciones AUTO_INCREMENT = 1;
ALTER TABLE zonas_producciones AUTO_INCREMENT = 1;
ALTER TABLE asignaciones_zonas_producciones AUTO_INCREMENT = 1;

-- Reabilitar restricciones
SET FOREIGN_KEY_CHECKS = 1;

EXIT;

# Luego importar
./scripts/import-csv.sh import-all
```

---

## Troubleshooting

### Problema: El script Bash no funciona en Windows

**Solución:** Usar Windows Subsystem for Linux (WSL)
```bash
# En WSL
wsl
cd /mnt/c/xampp/htdocs/WEB/clientes/DRH/go-agro-sentinel-worker
./scripts/import-csv.sh import-all
```

**Alternativa:** Usar Git Bash
```bash
# Git Bash incluye bash shell
bash scripts/import-csv.sh import-all
```

---

### Problema: Permisos de archivo denegados

**Solución:**
```bash
# Dar permisos de lectura
chmod 644 data/csv/examples/*.csv

# Dar permisos de ejecución al script
chmod 755 scripts/import-csv.sh
```

---

### Problema: Conexión MySQL rechazada

**Solución:**
```bash
# Verificar que MySQL está corriendo
mysql -h localhost -u admin -p admin -e "SELECT 1;"

# Si falla, iniciar MySQL
# Windows: services.msc → MySQL80 → Start
# Linux: sudo systemctl start mysql

# Verificar credenciales
echo $DB_USER
echo $DB_PASS
echo $DB_HOST
```

---

### Problema: Caracteres especiales dañados (UTF-8)

**Solución:** Asegurar UTF-8 en la importación
```sql
-- En MySQL antes de importar
SET NAMES utf8mb4;
SET CHARACTER SET utf8mb4;

LOAD DATA LOCAL INFILE ...
```

---

### Problema: Líneas duplicadas después de importar

**Solución:** Verificar si el CSV tiene duplicados
```bash
# Mostrar líneas duplicadas
sort data/csv/examples/articulos.csv | uniq -d

# Mostrar estadísticas del CSV
wc -l data/csv/examples/articulos.csv
sort -u data/csv/examples/articulos.csv | wc -l
```

---

## Mejores prácticas

### 1. Siempre crear backup antes de importar

```bash
./scripts/import-csv.sh backup
```

### 2. Validar CSVs antes de importar

```bash
./scripts/import-csv.sh validate data/csv/examples/articulos.csv
```

### 3. Importar en el orden correcto

1. articulos
2. centros_costos
3. zonas_producciones
4. producciones
5. asignaciones_zonas_producciones
6. s3_monitoring_escena_ia_resumen

### 4. Verificar datos después de importar

```sql
SELECT COUNT(*) FROM articulos;
SELECT * FROM articulos LIMIT 5;
```

### 5. Mantener archivos de ejemplo limpios

Los archivos en `data/csv/examples/` deben ser plantillas limpias y reutilizables.

### 6. Usar rutas absolutas en scripts

```bash
# No usar rutas relativas
# Correcto
LOAD DATA LOCAL INFILE '/home/user/agro/data/csv/examples/articulos.csv'

# Evitar
LOAD DATA LOCAL INFILE './articulos.csv'
```

### 7. Documentar cambios en CSVs

Si modifica un CSV de ejemplo, incluya comentario:
```csv
-- Modificado 2026-09-04: Agregados nuevos cultivos
articulo_id,nombre,variedad
...
```

### 8. Usar transacciones para importaciones críticas

```sql
START TRANSACTION;

LOAD DATA LOCAL INFILE ...
-- Verificar datos
SELECT COUNT(*) FROM tabla;

-- Si todo está bien
COMMIT;

-- Si hay problema
ROLLBACK;
```

### 9. Logs de importación

```bash
# Guardar salida en archivo
./scripts/import-csv.sh import-all > import.log 2>&1

# Ver el log
cat import.log
```

### 10. Automatizar importaciones regulares

```bash
# En crontab para Linux
0 2 * * * /home/user/agro/scripts/import-csv.sh import-all >> /var/log/agro-import.log 2>&1

# En Windows Task Scheduler
# Crear tarea que ejecute: bash.exe scripts/import-csv.sh import-all
```

---

## Referencias rápidas

### Comandos útiles

```bash
# Listar archivos CSV
ls -la data/csv/examples/

# Contar líneas en CSV
wc -l data/csv/examples/articulos.csv

# Ver primeras 10 líneas
head -10 data/csv/examples/articulos.csv

# Ver últimas 5 líneas
tail -5 data/csv/examples/articulos.csv

# Validar formato CSV
file data/csv/examples/articulos.csv

# Verificar codificación
file --mime-encoding data/csv/examples/articulos.csv
```

### Queries MySQL útiles

```sql
-- Ver todas las tablas
SHOW TABLES;

-- Ver estructura de tabla
DESCRIBE articulos;

-- Ver registros
SELECT * FROM articulos;

-- Ver con LIMIT
SELECT * FROM articulos LIMIT 5;

-- Contar registros
SELECT COUNT(*) FROM articulos;

-- Ver registros recientes
SELECT * FROM articulos ORDER BY fecha_creacion DESC LIMIT 10;

-- Ver duplicados
SELECT nombre, COUNT(*) FROM articulos GROUP BY nombre HAVING COUNT(*) > 1;
```

---

## Contacto y soporte

Para problemas o preguntas sobre la importación de CSV, consulte:
- Documentación: `/docs/CSV_IMPORT_GUIDE.md`
- Scripts: `/scripts/import-csv.sh` y `/scripts/import-from-csv.sql`
- Ejemplos: `/data/csv/examples/`

---

**Última actualización:** 2026-09-04
**Versión:** 1.0
