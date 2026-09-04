# Guía de Importación de Datos en DynamoDB

## Descripción General

Esta guía describe cómo importar datos a DynamoDB para el Sistema Agro Sentinel usando las herramientas de importación proporcionadas.

La herramienta permite:
- Importar datos desde archivos JSON o CSV
- Validar automáticamente el esquema de datos
- Importar a AWS DynamoDB o LocalStack (desarrollo local)
- Realizar importaciones en seco (dry-run) para validación
- Manejo automático de errores y reintentos

---

## Inicio Rápido

### 1. Instalación de Requisitos

```bash
# Asegurar Python 3.7+
python3 --version

# Instalar boto3 (cliente AWS para Python)
pip install boto3

# En Linux/Mac, hacer el script ejecutable
chmod +x scripts/import-dynamodb.sh
```

### 2. Importación Básica

```bash
# Importar producciones (con confirmación)
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json

# Importar escenas
./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json
```

### 3. Validar Antes de Importar

```bash
# Usar --dry-run para validar sin modificar datos
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --dry-run
```

---

## Tablas y Esquemas

### Tabla: `monitoring_producciones`

Almacena metadatos de producciones agrícolas.

#### Campos Requeridos

| Campo | Tipo | Descripción | Ejemplo |
|-------|------|-------------|---------|
| `produccion_id` | Number | Identificador único | `12345` |
| `activa` | Boolean | Estado de monitoreo | `true` |
| `cultivo` | String | Nombre del cultivo | `"maiz"` |
| `ciclo` | String | Ciclo agrícola | `"2026-A"` |
| `dias_produccion` | Number | Duración del ciclo | `120` |
| `articulo_id` | Number | ID del artículo | `5001` |
| `centro_costo_id` | Number | ID del centro de costos | `101` |

#### Campos Opcionales

| Campo | Tipo | Descripción | Ejemplo |
|-------|------|-------------|---------|
| `fecha_plantacion` | String | Fecha de plantación | `"2026-01-15"` |
| `nombre_rancho` | String | Nombre del rancho | `"Rancho El Remanso"` |

#### Validaciones

- **produccion_id**: Debe ser > 0
- **ciclo**: Formato `YYYY-[A|B]` (ej: `2026-A`)
- **dias_produccion**: Entre 60 y 200 días
- **fecha_plantacion**: Formato ISO `YYYY-MM-DD`
- **articulo_id**: > 0
- **centro_costo_id**: > 0

#### Ejemplo JSON

```json
[
  {
    "produccion_id": 12345,
    "activa": true,
    "cultivo": "maiz",
    "ciclo": "2026-A",
    "fecha_plantacion": "2026-01-15",
    "dias_produccion": 120,
    "articulo_id": 5001,
    "centro_costo_id": 101,
    "nombre_rancho": "Rancho El Remanso"
  }
]
```

---

### Tabla: `monitoring_escenas`

Almacena información de escenas de satélite Sentinel-2.

#### Campos Requeridos

| Campo | Tipo | Descripción | Ejemplo |
|-------|------|-------------|---------|
| `scene_id` | String | Identificador STAC único | `"S2A_MSIL2A_20260205..."` |
| `produccion_id` | Number | FK a producción | `12345` |
| `date` | String | Fecha de captura | `"2026-02-05"` |
| `cloud_cover` | Number | Cobertura de nubes (%) | `12.5` |

#### Campos Opcionales

| Campo | Tipo | Descripción | Ejemplo |
|-------|------|-------------|---------|
| `stac_assets` | Map | Bandas espectrales | Ver abajo |

#### Validaciones

- **scene_id**: No puede estar vacío
- **produccion_id**: Debe ser > 0 y existir en `monitoring_producciones`
- **date**: Formato ISO `YYYY-MM-DD`
- **cloud_cover**: Entre 0.0 y 100.0
- **stac_assets**: Si existe, debe ser diccionario con `href` y `resolution`

#### Subestructura: STAC Assets

Cada banda espectral es un mapa con:

```json
{
  "band_name": {
    "href": "URL del archivo GeoTIFF",
    "resolution": 10  // o 20, 60
  }
}
```

**Bandas Típicas Sentinel-2:**

| Banda | Descripción | Resolución | Uso |
|-------|-------------|-----------|-----|
| B02 | Blue | 10m | RGB, agua |
| B03 | Green | 10m | RGB |
| B04 | Red | 10m | RGB, NDVI |
| B05-B07 | Red Edge | 20m | Estrés de vegetación |
| B08 | NIR | 10m | NDVI |
| B11 | SWIR | 20m | Humedad, NDVI |

#### Ejemplo JSON

```json
[
  {
    "scene_id": "S2A_MSIL2A_20260205T135051_N0510_R024_T19HCC_20260205T135101",
    "produccion_id": 12345,
    "date": "2026-02-05",
    "cloud_cover": 12.5,
    "stac_assets": {
      "B02": {
        "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B02.jp2",
        "resolution": 10
      },
      "B03": {
        "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B03.jp2",
        "resolution": 10
      },
      "B04": {
        "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B04.jp2",
        "resolution": 10
      },
      "B08": {
        "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B08.jp2",
        "resolution": 10
      },
      "B11": {
        "href": "https://sentinel-2-l2a.s3.amazonaws.com/tiles/19/H/CC/2026/2/5/0/B11.jp2",
        "resolution": 20
      }
    }
  }
]
```

---

## Formatos de Importación

### Formato JSON

El formato más recomendado. Array de objetos JSON.

**Archivo: `monitoring_producciones.json`**
```json
[
  { "produccion_id": 1, "activa": true, ... },
  { "produccion_id": 2, "activa": true, ... }
]
```

**Ventajas:**
- Soporta tipos de datos complejos (maps, arrays)
- Preserva tipos numéricos y booleanos
- Fácil de validar

**Desventajas:**
- Más verboso que CSV

### Formato CSV

Útil para datos simples, especialmente desde hojas de cálculo.

**Archivo: `monitoring_producciones.csv`**
```csv
produccion_id,activa,cultivo,ciclo,dias_produccion,articulo_id,centro_costo_id,fecha_plantacion,nombre_rancho
12345,true,maiz,2026-A,120,5001,101,2026-01-15,Rancho El Remanso
12346,true,soja,2026-A,140,5002,102,2026-01-20,Finca La Esperanza
```

**Requisitos CSV:**
- Primera línea contiene nombres de columnas
- Booleanos: `true`/`false` (case-insensitive)
- Números: Sin comillas
- Strings: Sin comillas (a menos que contengan comas o comillas)
- Fechas: Formato `YYYY-MM-DD`

**Ventajas:**
- Fácil de crear desde Excel/Sheets
- Compacto
- Compatible con muchas herramientas

**Desventajas:**
- No soporta estructuras anidadas (stac_assets)
- Confusión con tipos de datos

---

## Uso de la Herramienta

### Script Bash (Linux/Mac)

```bash
./scripts/import-dynamodb.sh --table <tabla> --file <archivo> [opciones]
```

#### Opciones

| Opción | Descripción | Ejemplo |
|--------|-------------|---------|
| `--table` | Nombre de tabla (requerido) | `monitoring_producciones` |
| `--file` | Ruta del archivo (requerido) | `data/dynamodb/examples/monitoring_producciones.json` |
| `--endpoint-url` | URL del endpoint DynamoDB | `http://localhost:4566` |
| `--region` | Región AWS | `us-west-2` |
| `--dry-run` | Validar sin modificar | - |
| `--batch-size` | Tamaño de batch | `25` |
| `--verbose` | Mostrar más detalles | - |
| `--help` | Mostrar ayuda | - |

#### Ejemplos

**Ejemplo 1: Importación Simple (AWS Real)**

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json
```

**Ejemplo 2: Con Validación (Dry-Run)**

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json \
  --dry-run
```

**Ejemplo 3: LocalStack (Desarrollo Local)**

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --endpoint-url http://localhost:4566
```

**Ejemplo 4: Con Verbosidad**

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/monitoring_escenas.csv \
  --verbose \
  --region us-east-1
```

### Script Python Directo

Si prefieres usar Python directamente:

```bash
python3 scripts/import-dynamodb.py --table <tabla> --file <archivo> [opciones]
```

---

## Flujo de Trabajo Típico

### 1. Preparar Datos

```bash
# Copiar ejemplos como base
cp data/dynamodb/examples/monitoring_producciones.json \
   data/monitoring_producciones_custom.json

# Editar con tus datos
vim data/monitoring_producciones_custom.json
```

### 2. Validar (Dry-Run)

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones_custom.json \
  --dry-run \
  --verbose
```

**Output esperado:**
```
[INFO] Cargados 5 items desde JSON
[INFO] Validando items...
[INFO] Item 0: Válido
[INFO] Item 1: Válido
...
[INFO] Importados: 5, Inválidos: 0
[DRY-RUN] Se escribirían los siguientes items:
  Item 1: {...}
  Item 2: {...}
  ...
[INFO] RESUMEN DE IMPORTACIÓN
```

### 3. Importar (Ejecución Real)

Una vez validado:

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones_custom.json
```

El script solicitará confirmación antes de proceder:
```
Configuración:
  Tabla: monitoring_producciones
  Archivo: data/monitoring_producciones_custom.json
  Región: us-west-2
  Modo: REAL (modificará DynamoDB)

[ADVERTENCIA] Esta operación modificará datos en DynamoDB
¿Continuar? (s/n) s
```

### 4. Verificar Importación

En la consola, verás:
```
[INFO] Iniciando importación de 5 items
[INFO] Validando items...
[INFO] Validados: 5, Inválidos: 0
[INFO] Importando 5 items válidos...
[INFO] RESUMEN DE IMPORTACIÓN
======================================================================
Tabla: monitoring_producciones
Total items: 5
Items exitosos: 5
Items fallidos: 0
Items saltados: 0
...
```

---

## Validaciones Automáticas

La herramienta valida automáticamente:

### Para `monitoring_producciones`

1. **Campos Requeridos**: Presencia de todos los campos obligatorios
2. **Tipos de Datos**: Conversión automática (string → number, boolean)
3. **Rango de Valores**:
   - `produccion_id` > 0
   - `dias_produccion` entre 60-200
   - Ciclo en formato `YYYY-[A|B]`
4. **Fechas**: Formato ISO `YYYY-MM-DD`
5. **Booleanos**: Conversión de string "true"/"false"

### Para `monitoring_escenas`

1. **Campos Requeridos**: `scene_id`, `produccion_id`, `date`, `cloud_cover`
2. **Rango de Valores**:
   - `cloud_cover` entre 0-100
   - `produccion_id` > 0
3. **scene_id**: No puede estar vacío
4. **Fechas**: Formato ISO `YYYY-MM-DD`
5. **STAC Assets**: Si existe, estructura correcta con `href` y `resolution`

### Errores Comunes y Soluciones

| Error | Causa | Solución |
|-------|-------|----------|
| `Partition Key faltante: produccion_id` | Campo requerido no presente | Agregar `produccion_id` en JSON |
| `cloud_cover debe estar entre 0-100` | Valor fuera de rango | Validar valor (0-100) |
| `ciclo debe ser formato YYYY-[A\|B]` | Formato inválido | Usar `2026-A` o `2026-B` |
| `fecha_plantacion debe ser YYYY-MM-DD` | Formato de fecha incorrecto | Usar `2026-01-15` |
| `scene_id no puede estar vacío` | Campo vacío en escena | Completar scene_id |

---

## Integración con AWS

### Configurar Credenciales AWS

**Opción 1: Variables de Entorno**

```bash
export AWS_ACCESS_KEY_ID="tu-access-key"
export AWS_SECRET_ACCESS_KEY="tu-secret-key"
export AWS_REGION="us-west-2"

./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json
```

**Opción 2: Archivo ~/.aws/credentials**

```ini
[default]
aws_access_key_id = tu-access-key
aws_secret_access_key = tu-secret-key

[agro-sentinel]
aws_access_key_id = otro-key
aws_secret_access_key = otro-secret
```

Luego:
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --region us-west-2
```

**Opción 3: AWS IAM Roles (Recomendado en Producción)**

Si ejecutas en EC2 o Lambda con rol IAM:
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json
```

Las credenciales se obtienen automáticamente del rol.

### Crear Tabla en AWS

```bash
# Crear tabla de producciones
aws dynamodb create-table \
  --table-name monitoring_producciones \
  --attribute-definitions AttributeName=produccion_id,AttributeType=N \
  --key-schema AttributeName=produccion_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --region us-west-2

# Crear tabla de escenas
aws dynamodb create-table \
  --table-name monitoring_escenas \
  --attribute-definitions AttributeName=scene_id,AttributeType=S \
  --key-schema AttributeName=scene_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --region us-west-2
```

---

## Desarrollo Local con LocalStack

### Setup LocalStack

**1. Instalar LocalStack**

```bash
pip install localstack localstack-cli
```

**2. Iniciar LocalStack**

```bash
localstack start
```

O con Docker Compose:

```bash
docker-compose up -d localstack
```

**3. Crear Tablas Locales**

```bash
# Producciones
aws dynamodb create-table \
  --table-name monitoring_producciones \
  --attribute-definitions AttributeName=produccion_id,AttributeType=N \
  --key-schema AttributeName=produccion_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:4566 \
  --region us-west-2

# Escenas
aws dynamodb create-table \
  --table-name monitoring_escenas \
  --attribute-definitions AttributeName=scene_id,AttributeType=S \
  --key-schema AttributeName=scene_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --endpoint-url http://localhost:4566 \
  --region us-west-2
```

**4. Importar Datos Locales**

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --endpoint-url http://localhost:4566

./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json \
  --endpoint-url http://localhost:4566
```

**5. Verificar Datos**

```bash
# Listar items
aws dynamodb scan \
  --table-name monitoring_producciones \
  --endpoint-url http://localhost:4566 \
  --region us-west-2

# Query específico
aws dynamodb get-item \
  --table-name monitoring_producciones \
  --key '{"produccion_id":{"N":"12345"}}' \
  --endpoint-url http://localhost:4566 \
  --region us-west-2
```

---

## Troubleshooting

### Problema: "boto3 no está instalado"

```bash
pip install boto3
pip install --upgrade boto3
```

### Problema: "Credenciales AWS no encontradas"

```bash
# Verificar credenciales
aws sts get-caller-identity

# O configurar
aws configure
```

### Problema: "Tabla no existe"

```bash
# Listar tablas existentes
aws dynamodb list-tables --region us-west-2

# O para LocalStack
aws dynamodb list-tables \
  --endpoint-url http://localhost:4566 \
  --region us-west-2
```

### Problema: "JSON malformado"

**Verificar JSON:**

```bash
python3 -m json.tool data/dynamodb/examples/monitoring_producciones.json
```

Si hay error, el JSON no es válido.

### Problema: "Item inválido"

```bash
# Usar --verbose y --dry-run para ver detalles
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones.json \
  --dry-run \
  --verbose
```

Esto mostrará exactamente cuáles items fallan y por qué.

### Problema: "Timeout en importación"

Si la importación es muy lenta:

```bash
# Ajustar batch size (más pequeño = más lento pero menos memoria)
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones.json \
  --batch-size 10
```

---

## Mejores Prácticas

### 1. Siempre Usar Dry-Run Primero

```bash
# Primero: validar
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones.json \
  --dry-run

# Si no hay errores, ejecutar
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones.json
```

### 2. Usar Verbose para Debugging

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones.json \
  --verbose
```

### 3. Mantener Archivos de Ejemplo

Los archivos en `data/dynamodb/examples/` son referencias. Crear copias para modificar:

```bash
cp data/dynamodb/examples/monitoring_producciones.json \
   data/monitoring_producciones_batch1.json
```

### 4. Documentar Cambios

Cuando importes datos significativos:

```bash
# Crear log
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones.json \
  --verbose > import_log_$(date +%Y%m%d_%H%M%S).log
```

### 5. Backup Antes de Importación Masiva

```bash
# Exportar datos existentes
aws dynamodb scan \
  --table-name monitoring_producciones \
  --output json > backup_producciones_$(date +%Y%m%d).json
```

---

## Integración con CI/CD

### GitHub Actions Example

```yaml
name: Import DynamoDB Data

on:
  workflow_dispatch:
    inputs:
      table:
        description: 'Tabla a importar'
        required: true
        type: choice
        options:
          - monitoring_producciones
          - monitoring_escenas

jobs:
  import:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: '3.10'
      
      - name: Install dependencies
        run: pip install boto3
      
      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v2
        with:
          aws-access-key-id: ${{ secrets.AWS_ACCESS_KEY_ID }}
          aws-secret-access-key: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
          aws-region: us-west-2
      
      - name: Import to DynamoDB
        run: |
          python3 scripts/import-dynamodb.py \
            --table ${{ github.event.inputs.table }} \
            --file data/dynamodb/examples/${{ github.event.inputs.table }}.json
```

---

## Soporte y Recursos

### Ver Documentación Relacionada

- `docs/DYNAMODB_STRUCTURE.md` - Esquema detallado de DynamoDB
- `docs/DEPLOYMENT_PLAN.md` - Plan de despliegue
- `docs/OPERATIONS_GUIDE.md` - Guía operacional

### Comandos Útiles

**Verificar tabla:**
```bash
aws dynamodb describe-table \
  --table-name monitoring_producciones \
  --region us-west-2
```

**Contar items:**
```bash
aws dynamodb scan \
  --table-name monitoring_producciones \
  --select COUNT \
  --region us-west-2
```

**Eliminar tabla:**
```bash
aws dynamodb delete-table \
  --table-name monitoring_producciones \
  --region us-west-2
```

---

**Última actualización**: 2026-09-04  
**Versión**: 1.0  
**Mantendido por**: Equipo Agro Sentinel
