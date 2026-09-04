# Referencia Rápida - Importación CSV

## Inicio Rápido

```bash
# Linux/Mac con Bash
chmod +x scripts/import-csv.sh
./scripts/import-csv.sh import-all

# Windows con PowerShell
.\scripts\import-csv.ps1 -Comando ImportAll

# Windows con MySQL direct
mysql -h localhost -u admin -p admin agro < scripts/import-from-csv.sql
```

---

## Estructura de Tablas (Cheat Sheet)

### ARTICULOS
```csv
articulo_id,nombre,variedad
1,Lechuga,Latina
```

### CENTROS_COSTOS
```csv
centro_costo_id,nombre
1,Rancho Los Andes
```

### PRODUCCIONES
```csv
folio,articulo_id,fecha,hora,fecha_cierre,hora_cierre,usuario,estatus,aplicado,cantidad,centro_costo_id,monitoring
CSJ2601-17-A,1,2026-06-03,08:30:00,,,"admin",N,S,2.50,1,1
```

### ZONAS_PRODUCCIONES
```csv
zona_produccion_id,nombre,nombre_corto,estatus,area,centro_costo_id,poligono
1,Zona Norte,ZNA,P,5.50,1,"[[21.10535, -100.93089], [21.10535, -100.92730]]"
```

### ASIGNACIONES_ZONAS_PRODUCCIONES
```csv
asignacion_zona_prod_id,produccion_id,zona_produccion_id,tipo_asignacion,area,poligono
1,1,1,P,2.50,"[[21.10535, -100.93089], [21.10535, -100.92730]]"
```

### S3_MONITORING_ESCENA_IA_RESUMEN
```csv
s3_monitoring_escena_id,estado_clave,estado_general,riesgo_nivel,riesgo_motivo,fecha_analisis,json_original
1001,normal,OK,bajo,Sin anomalías,2026-06-03 10:30:00,"{""status"": ""normal""}"
```

---

## Comandos Rápidos

### Bash
```bash
# Importar tabla específica
./scripts/import-csv.sh import articulos data/csv/examples/articulos.csv

# Importar todas
./scripts/import-csv.sh import-all

# Validar CSV
./scripts/import-csv.sh validate data/csv/examples/articulos.csv

# Listar tablas
./scripts/import-csv.sh list-tables

# Crear backup
./scripts/import-csv.sh backup

# Con variables de entorno
export DB_HOST=localhost
export DB_USER=admin
export DB_PASS=admin
./scripts/import-csv.sh import-all
```

### PowerShell
```powershell
# Ayuda
.\scripts\import-csv.ps1 -Comando Help

# Importar tabla específica
.\scripts\import-csv.ps1 -Comando Import -Tabla articulos -Archivo "data/csv/examples/articulos.csv"

# Importar todas
.\scripts\import-csv.ps1 -Comando ImportAll

# Validar
.\scripts\import-csv.ps1 -Comando Validate -Archivo "data/csv/examples/articulos.csv"

# Listar
.\scripts\import-csv.ps1 -Comando ListTables

# Backup
.\scripts\import-csv.ps1 -Comando Backup

# Con parámetros custom
.\scripts\import-csv.ps1 -Comando ImportAll -DBHost "192.168.1.100" -DBUser "admin" -DBPass "pass123"
```

### MySQL Direct
```bash
# Habilitar local_infile
mysql -h localhost -u admin -p admin agro -e "SET GLOBAL local_infile = 1;"

# Importar CSV
mysql -h localhost -u admin -p admin agro --local-infile=1 << EOF
LOAD DATA LOCAL INFILE '/ruta/articulos.csv'
INTO TABLE articulos
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS;
EOF

# Ejecutar script SQL completo
mysql -h localhost -u admin -p admin agro < scripts/import-from-csv.sql
```

---

## Valores Válidos

### PRODUCCIONES.estatus
- `N` = Normal (abierta)
- `T` = Terminada/cerrada
- `C` = Cancelada

### PRODUCCIONES.aplicado
- `S` = Sí
- `N` = No

### PRODUCCIONES.monitoring
- `0` = No monitorear
- `1` = Monitorear

### ZONAS_PRODUCCIONES.estatus
- `P` = Producción
- `I` = Inactiva

### ASIGNACIONES_ZONAS_PRODUCCIONES.tipo_asignacion
- `T` = Total
- `P` = Parcial

### S3_MONITORING.estado_clave
- `normal`
- `alerta`
- `crítico`

### S3_MONITORING.riesgo_nivel
- `bajo`
- `medio`
- `alto`

---

## Validaciones Rápidas

```bash
# Verificar archivo existe
ls -la data/csv/examples/articulos.csv

# Contar líneas
wc -l data/csv/examples/articulos.csv

# Ver encabezado
head -1 data/csv/examples/articulos.csv

# Ver datos
head -3 data/csv/examples/articulos.csv

# Verificar codificación
file -b --mime-encoding data/csv/examples/articulos.csv

# Buscar duplicados
awk 'NR>1 {if (seen[$1]++) print "Duplicado: " $0}' data/csv/examples/articulos.csv
```

---

## Errores Comunes y Soluciones

| Error | Causa | Solución |
|-------|-------|----------|
| "Can't find file" | Ruta incorrecta | Usar ruta absoluta |
| "Access denied; FILE privilege" | Sin permisos | `GRANT FILE ON *.* TO 'admin'@'localhost'` |
| "Loading local data is disabled" | local_infile deshabilitado | `SET GLOBAL local_infile = 1;` |
| "Duplicate entry" | IDs duplicados | Limpiar tabla: `DELETE FROM tabla; ALTER TABLE tabla AUTO_INCREMENT = 1;` |
| "Foreign key constraint" | Referencias inválidas | Importar en orden correcto |
| "Incorrect date value" | Formato fecha incorrecto | Usar `YYYY-MM-DD` |
| "Data too long" | Campo muy grande | Reducir tamaño del dato |
| "Syntax error" | CSV malformado | Validar con `./import-csv.sh validate` |

---

## Verificaciones Post-Importación

```sql
-- Contar registros por tabla
SELECT 'articulos' as tabla, COUNT(*) FROM articulos
UNION ALL SELECT 'centros_costos', COUNT(*) FROM centros_costos
UNION ALL SELECT 'producciones', COUNT(*) FROM producciones
UNION ALL SELECT 'zonas_producciones', COUNT(*) FROM zonas_producciones
UNION ALL SELECT 'asignaciones_zonas_producciones', COUNT(*) FROM asignaciones_zonas_producciones;

-- Verificar integridad referencial
SELECT p.folio, a.nombre, c.nombre
FROM producciones p
JOIN articulos a ON p.articulo_id = a.articulo_id
JOIN centros_costos c ON p.centro_costo_id = c.centro_costo_id
LIMIT 5;

-- Detectar huérfanos
SELECT * FROM producciones
WHERE articulo_id NOT IN (SELECT articulo_id FROM articulos)
   OR centro_costo_id NOT IN (SELECT centro_costo_id FROM centros_costos);

-- Ver registros más recientes
SELECT * FROM articulos ORDER BY fecha_creacion DESC LIMIT 5;
```

---

## Limpieza e Re-Importación

```sql
-- Desconectar restricciones
SET FOREIGN_KEY_CHECKS = 0;

-- Limpiar todas las tablas
TRUNCATE TABLE asignaciones_zonas_producciones;
TRUNCATE TABLE s3_monitoring_escena_ia_resumen;
TRUNCATE TABLE producciones;
TRUNCATE TABLE zonas_producciones;
TRUNCATE TABLE articulos;
TRUNCATE TABLE centros_costos;

-- Reconectar restricciones
SET FOREIGN_KEY_CHECKS = 1;
```

---

## Variables de Entorno

```bash
# Configurar para host remoto
export DB_HOST=192.168.1.100
export DB_USER=agro_admin
export DB_PASS=secure_password
export DB_NAME=agro
export DB_PORT=3307

# Luego ejecutar scripts
./scripts/import-csv.sh import-all
```

---

## Ejemplos Prácticos

### Importar solo artículos y centros
```bash
./scripts/import-csv.sh import articulos data/csv/examples/articulos.csv
./scripts/import-csv.sh import centros_costos data/csv/examples/centros_costos.csv
```

### Importar y verificar
```bash
./scripts/import-csv.sh import articulos data/csv/examples/articulos.csv && \
mysql -h localhost -u admin -padmin agro -e "SELECT COUNT(*) FROM articulos;"
```

### Crear y restaurar backup
```bash
# Backup
./scripts/import-csv.sh backup

# Listar backups
ls -la backups/

# Restaurar
mysql -h localhost -u admin -padmin agro < backups/agro_backup_20260904_143022.sql
```

### Validar antes de importar
```bash
for file in data/csv/examples/*.csv; do
    ./scripts/import-csv.sh validate "$file"
done
```

---

## Archivos de Referencia

| Archivo | Propósito |
|---------|-----------|
| `scripts/import-csv.sh` | Script Bash principal |
| `scripts/import-csv.ps1` | Script PowerShell para Windows |
| `scripts/import-from-csv.sql` | Script SQL completo de importación |
| `data/csv/examples/` | Archivos CSV de ejemplo |
| `docs/CSV_IMPORT_GUIDE.md` | Documentación completa |
| `docs/CSV_QUICK_REFERENCE.md` | Esta referencia rápida |

---

## Tips y Trucos

```bash
# Importar con logging
./scripts/import-csv.sh import-all 2>&1 | tee import.log

# Importar en background
nohup ./scripts/import-csv.sh import-all > import.log 2>&1 &

# Validar todos los CSVs
for csv in data/csv/examples/*.csv; do
    echo "Validando: $csv"
    ./scripts/import-csv.sh validate "$csv" || break
done

# Contar registros en todas las tablas
mysql -h localhost -u admin -padmin agro -N -e \
"SELECT TABLE_NAME, TABLE_ROWS FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA='agro'"

# Exportar tabla a CSV
mysql -h localhost -u admin -padmin agro \
-e "SELECT * FROM articulos" > export_articulos.csv
```

---

## Troubleshooting Rápido

```bash
# 1. Verificar MySQL está corriendo
mysql -h localhost -u admin -padmin -e "SELECT 1;"

# 2. Verificar archivo CSV existe
test -f data/csv/examples/articulos.csv && echo "OK" || echo "No existe"

# 3. Verificar permisos script
ls -la scripts/import-csv.sh

# 4. Hacer ejecutable
chmod +x scripts/import-csv.sh

# 5. Ver error completo
./scripts/import-csv.sh import articulos data/csv/examples/articulos.csv 2>&1

# 6. Verificar codificación UTF-8
file -b --mime-encoding data/csv/examples/articulos.csv
```

---

**Última actualización:** 2026-09-04
**Versión:** 1.0
