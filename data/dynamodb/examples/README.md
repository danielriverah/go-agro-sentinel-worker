# DynamoDB Import Examples

Ejemplos de datos JSON listos para importar a DynamoDB usando las herramientas de importación del Sistema Agro Sentinel.

## Archivos

### `monitoring_producciones.json`

Ejemplo de 5 producciones agrícolas activas y una inactiva.

**Contenido:**
- Producciones de maíz, soja, trigo y algodón
- Mezcla de ciclos A y B
- Diferentes centros de costos (101, 102, 103, 104)
- Nombres de ranchos denormalizados
- Datos realistas con fechas de plantación

**Campos:**
```json
{
  "produccion_id": 12345,           // ID único, > 0
  "activa": true,                   // boolean
  "cultivo": "maiz",                // string
  "ciclo": "2026-A",                // formato YYYY-[A|B]
  "fecha_plantacion": "2026-01-15", // ISO YYYY-MM-DD
  "dias_produccion": 120,           // número 60-200
  "articulo_id": 5001,              // FK a artículos
  "centro_costo_id": 101,           // FK a centros
  "nombre_rancho": "Rancho..."      // opcional
}
```

**Validaciones Aplicadas:**
- `produccion_id` debe ser > 0
- `ciclo` debe ser `YYYY-A` o `YYYY-B`
- `dias_produccion` entre 60-200
- `fecha_plantacion` formato ISO o omitido
- Booleanos como `true`/`false`

**Cómo Usar:**

```bash
# Validar primero
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --dry-run

# Luego importar
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json
```

---

### `monitoring_escenas.json`

Ejemplo de 5 escenas de satélite Sentinel-2 asociadas a producciones.

**Contenido:**
- Escenas reales de Sentinel-2 (formatos STAC estándar)
- Fechas distribuidas de febrero a marzo 2026
- Cobertura de nubes variada (0% a 15.7%)
- Bandas espectrales típicas (B02, B03, B04, B08, B11, etc.)
- URLs realistas de S3 (aunque no descargables)

**Campos:**
```json
{
  "scene_id": "S2A_MSIL2A_20260205T135051_N0510_R024_T19HCC_20260205T135101",
  "produccion_id": 12345,           // FK a producción
  "date": "2026-02-05",             // ISO YYYY-MM-DD
  "cloud_cover": 12.5,              // número 0-100
  "stac_assets": {                  // bandas espectrales (opcional)
    "B02": {
      "href": "https://...",        // URL del GeoTIFF
      "resolution": 10              // 10, 20 o 60
    },
    "B03": { ... },
    "B04": { ... },
    "B08": { ... },
    "B11": { ... }
  }
}
```

**Validaciones Aplicadas:**
- `scene_id` no puede estar vacío
- `produccion_id` debe existir en `monitoring_producciones`
- `date` formato ISO o validación
- `cloud_cover` entre 0-100
- `stac_assets` si existe, debe tener `href` y `resolution`
- `resolution` solo 10, 20 o 60

**Bandas Included:**

| Banda | Resolución | Descripción |
|-------|-----------|-------------|
| B02 | 10m | Azul (visible) |
| B03 | 10m | Verde (visible) |
| B04 | 10m | Rojo (visible) |
| B08 | 10m | NIR (infrarrojo cercano) |
| B11 | 20m | SWIR (infrarrojo corto) |

**Cómo Usar:**

```bash
# Validar con verbose
./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json \
  --dry-run \
  --verbose

# Importar
./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json
```

---

## Personalizar Ejemplos

### Crear Tu Propio Archivo de Producciones

1. **Copiar el ejemplo:**
   ```bash
   cp data/dynamodb/examples/monitoring_producciones.json \
      data/monitoring_producciones_custom.json
   ```

2. **Editar con tu editor favorito:**
   ```bash
   vim data/monitoring_producciones_custom.json
   ```

3. **Validar estructura JSON:**
   ```bash
   python3 -m json.tool data/monitoring_producciones_custom.json
   ```

4. **Importar:**
   ```bash
   ./scripts/import-dynamodb.sh \
     --table monitoring_producciones \
     --file data/monitoring_producciones_custom.json \
     --dry-run
   ```

### Crear Desde CSV en Excel/Google Sheets

1. **Crear hoja de cálculo** con estas columnas:
   - produccion_id
   - activa
   - cultivo
   - ciclo
   - fecha_plantacion
   - dias_produccion
   - articulo_id
   - centro_costo_id
   - nombre_rancho (opcional)

2. **Descargar como CSV:**
   - En Excel: Guardar Como → CSV
   - En Sheets: Descargar → CSV

3. **Importar directamente:**
   ```bash
   ./scripts/import-dynamodb.sh \
     --table monitoring_producciones \
     --file data/monitoring_producciones.csv
   ```

---

## Casos de Uso

### Caso 1: Setup Inicial Local

```bash
# Setup LocalStack
localstack start &

# Crear tablas
aws dynamodb create-table \
  --table-name monitoring_producciones \
  --attribute-definitions AttributeName=produccion_id,AttributeType=N \
  --key-schema AttributeName=produccion_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:4566

# Importar ejemplos
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --endpoint-url http://localhost:4566

./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json \
  --endpoint-url http://localhost:4566

# Verificar
aws dynamodb scan \
  --table-name monitoring_producciones \
  --endpoint-url http://localhost:4566
```

### Caso 2: Testing en Development

```bash
# Importar con datos de test
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --dry-run \
  --verbose

# Si todo está bien
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json
```

### Caso 3: Añadir Más Datos

```bash
# Crear nuevo archivo basado en ejemplo
cp data/dynamodb/examples/monitoring_producciones.json \
   data/monitoring_producciones_adicionales.json

# Editar y añadir producción_ids 12350-12359
vim data/monitoring_producciones_adicionales.json

# Importar nuevos datos
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones_adicionales.json
```

---

## Referencia de Valores

### Cultivos Típicos

```
"maiz"
"soja"
"trigo"
"algodon"
"sorgo"
"cebada"
```

### Ciclos Válidos

```
"2026-A"  // Primavera
"2026-B"  // Otoño
```

### Ranchos Ejemplo

```
"Rancho El Remanso"
"Finca La Esperanza"
"Campos del Amanecer"
"Hacienda El Éxito"
"Granja Productiva"
```

### IDs de Artículo (Productos)

```
5001  // Maíz
5002  // Soja
5003  // Trigo
5004  // Algodón
5005  // Sorgo
5006  // Cebada
```

### Centros de Costo

```
101   // Centro Operativo A
102   // Centro Operativo B
103   // Centro Operativo C
104   // Centro Administrativo
```

---

## Documentación Relacionada

- `docs/DYNAMODB_IMPORT_GUIDE.md` - Guía completa de importación
- `docs/DYNAMODB_STRUCTURE.md` - Esquema detallado de tablas
- `scripts/import-dynamodb.sh` - Script wrapper (con --help)
- `scripts/import-dynamodb.py` - Script Python (uso directo)

---

## Preguntas Frecuentes

**P: ¿Puedo importar escenas sin producciones?**
R: Técnicamente sí, pero las escenas deben referenciar `produccion_id` que exista (o la validación lo rechazará).

**P: ¿Qué pasa si uso los mismos produccion_ids?**
R: DynamoDB los sobrescribirá (upsert), reemplazando los datos anteriores.

**P: ¿Puedo importar solo algunos campos?**
R: Sí, pero los campos requeridos deben estar presentes.

**P: ¿Cómo valido sin importar?**
R: Usa `--dry-run` para validar sin modificar datos.

---

**Última actualización**: 2026-09-04  
**Versión**: 1.0
