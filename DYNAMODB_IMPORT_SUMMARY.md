# DynamoDB Import Tools - Resumen de Implementación

**Fecha de Creación:** 2026-09-04  
**Sistema:** Agro Sentinel Worker  
**Autor:** Claude Code (Anthropic)

---

## Descripción General

Se ha creado un conjunto completo de herramientas y documentación para importar datos en DynamoDB para el Sistema Agro Sentinel. Las herramientas permiten importar datos desde JSON o CSV, con validación automática de esquema, modo dry-run y soporte para AWS real o LocalStack (desarrollo local).

---

## Archivos Creados

### 1. Scripts de Importación

#### `scripts/import-dynamodb.py` (690 líneas)
**Descripción:** Script Python principal para importación de datos.

**Características:**
- Clase `DynamoDBImporter` con métodos completos
- Carga datos desde JSON o CSV
- Validación automática de esquema por tabla
- Conversión de tipos (string → number, boolean)
- Validaciones específicas:
  - Rango de valores (dias_produccion 60-200, cloud_cover 0-100)
  - Formato de ciclo (YYYY-[A|B])
  - Fechas ISO (YYYY-MM-DD)
  - Estructura de STAC assets
- Importación en batches (configurable)
- Modo dry-run
- Logging detallado
- Manejo robusto de errores

**Uso:**
```bash
python3 scripts/import-dynamodb.py \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --dry-run
```

#### `scripts/import-dynamodb.sh` (360 líneas)
**Descripción:** Script wrapper bash para facilitar el uso.

**Características:**
- Interfaz amigable con colores
- Parseo de argumentos completo
- Validación de requisitos (Python, boto3)
- Confirmación antes de ejecutar
- Help integrado (`--help`)
- Soporte para todas las opciones del script Python
- Mensajes claros y descriptivos

**Uso:**
```bash
./scripts/import-dynamodb.sh --table monitoring_producciones --file data/dynamodb/examples/monitoring_producciones.json
```

#### `scripts/test-import-dynamodb.py` (430 líneas)
**Descripción:** Suite de tests unitarios para validar la herramienta.

**Tests Incluidos (13 tests):**
1. Cargar datos JSON
2. Cargar datos CSV
3. Validar producción válida
4. Rechazar ciclo inválido
5. Rechazar dias_produccion inválido
6. Rechazar campo requerido faltante
7. Validar escena válida
8. Validar rango cloud_cover
9. Convertir tipos de datos
10. Esquemas de tablas
11. Validación de fechas
12. Validación de ciclos
13. Rechazar tabla desconocida

**Uso:**
```bash
python3 scripts/test-import-dynamodb.py
```

#### `scripts/requirements.txt`
**Descripción:** Dependencias Python para la herramienta.

**Contenido:**
- boto3 >= 1.26.0 (AWS SDK para Python)
- pytest >= 7.0.0 (testing, opcional)
- pytest-cov >= 4.0.0 (coverage, opcional)

---

### 2. Datos de Ejemplo

#### `data/dynamodb/examples/monitoring_producciones.json`
**Descripción:** 5 producciones agrícolas de ejemplo.

**Campos Incluidos:**
- 5 producciones variadas (maíz, soja, trigo, algodón)
- Ciclos A y B
- Centros de costo múltiples (101-104)
- Nombres de ranchos denormalizados
- Fechas realistas de plantación
- 1 producción inactiva para ejemplificar ambos estados

**Tamaño:** ~700 bytes
**Items:** 5

#### `data/dynamodb/examples/monitoring_escenas.json`
**Descripción:** 5 escenas de satélite Sentinel-2 de ejemplo.

**Campos Incluidos:**
- Formatos STAC estándar reales
- Fechas distribuidas (Feb-Mar 2026)
- Cloud cover variado (0% a 15.7%)
- Bandas espectrales típicas (B02, B03, B04, B08, B11, etc.)
- URLs realistas de S3
- Resoluciones de banda (10m, 20m, 60m)

**Tamaño:** ~2.5 KB
**Items:** 5

#### `data/dynamodb/examples/monitoring_producciones.csv`
**Descripción:** Mismo contenido que JSON pero en formato CSV.

**Formato:**
- Encabezados en primera línea
- Valores separados por comas
- Booleanos como `true`/`false`
- Dates como `YYYY-MM-DD`
- Campo nombre_rancho opcional (vacío si no aplica)

**Tamaño:** ~500 bytes
**Registros:** 5

---

### 3. Documentación

#### `docs/DYNAMODB_IMPORT_GUIDE.md` (780 líneas)
**Descripción:** Guía completa y detallada de importación.

**Secciones:**
1. Descripción General y Inicio Rápido
2. Esquemas de Tablas Detallados:
   - monitoring_producciones (campos, tipos, validaciones)
   - monitoring_escenas (campos, bandas, STAC assets)
3. Formatos de Importación (JSON vs CSV)
4. Uso de la Herramienta (bash y Python directo)
5. Flujo de Trabajo Típico (preparar → validar → importar → verificar)
6. Validaciones Automáticas (errores comunes y soluciones)
7. Integración con AWS (configurar credenciales, crear tablas)
8. Desarrollo Local con LocalStack
9. Troubleshooting (problemas comunes)
10. Mejores Prácticas
11. Integración con CI/CD (GitHub Actions example)

**Características:**
- Ejemplos prácticos paso a paso
- Tablas de referencia de campos
- Comandos AWS CLI
- Configuración de credenciales
- Explicación de validaciones
- Soluciones a errores típicos

#### `docs/DYNAMODB_IMPORT_QUICKSTART.md` (200 líneas)
**Descripción:** Guía rápida para empezar en 5 minutos.

**Contenido:**
1. Instalación de requisitos
2. Validación básica
3. Importación simple
4. Comandos frecuentes
5. Errores comunes
6. Paso a paso primera importación
7. Cheat sheet de opciones

**Objetivo:** Permitir a usuarios nuevos importar datos sin leer documentación completa.

#### `data/dynamodb/examples/README.md` (400 líneas)
**Descripción:** Documentación de los archivos de ejemplo.

**Contenido:**
1. Descripción de cada archivo de ejemplo
2. Campos y validaciones específicas
3. Cómo personalizar ejemplos
4. Crear desde CSV/Excel
5. Casos de uso
6. Referencias de valores típicos
7. Preguntas frecuentes

---

### 4. Actualización de Documentación Existente

#### `scripts/README_SCRIPTS.md` (modificado)
**Cambios:** Se agregó sección nueva "Data Import Scripts" al inicio del documento con:
- Descripción de herramientas de importación
- Ejemplos de uso
- Componentes incluidos
- Requisitos
- Instalación rápida
- Testing

---

## Esquemas de Datos Validados

### Tabla: `monitoring_producciones`

```json
{
  "produccion_id": 12345,           // Number, > 0, PK
  "activa": true,                   // Boolean
  "cultivo": "maiz",                // String
  "ciclo": "2026-A",                // String, formato YYYY-[A|B]
  "fecha_plantacion": "2026-01-15", // String, ISO YYYY-MM-DD
  "dias_produccion": 120,           // Number, 60-200
  "articulo_id": 5001,              // Number, > 0
  "centro_costo_id": 101,           // Number, > 0
  "nombre_rancho": "Rancho..."      // String, opcional
}
```

**Validaciones Implementadas:**
- Campos requeridos: 7
- Campos opcionales: 2
- Conversión de tipos automática
- Validación de rango para dias_produccion
- Validación de formato para ciclo
- Validación de formato para fecha_plantacion

### Tabla: `monitoring_escenas`

```json
{
  "scene_id": "S2A_MSIL2A_...",     // String, no vacío, PK
  "produccion_id": 12345,           // Number, > 0
  "date": "2026-02-05",             // String, ISO YYYY-MM-DD
  "cloud_cover": 12.5,              // Number, 0-100
  "stac_assets": {                  // Map, opcional
    "B04": {
      "href": "https://...",        // String
      "resolution": 10              // Number, 10|20|60
    }
  }
}
```

**Validaciones Implementadas:**
- Campos requeridos: 4
- Campos opcionales: 1
- Validación de rango para cloud_cover
- Validación de estructura de STAC assets
- Validación de resoluciones válidas

---

## Funcionalidades Clave

### 1. Validación de Esquema Automática
- Verifica presencia de campos requeridos
- Valida tipos de datos
- Convierte strings a números/booleanos
- Validaciones específicas por tabla
- Mensajes de error detallados

### 2. Soporte Multi-Formato
- **JSON**: Formato recomendado, soporta estructuras complejas
- **CSV**: Para datos simples, fácil desde Excel

### 3. Modo Dry-Run
- Valida datos sin modificar DynamoDB
- Muestra qué se importaría
- Ideal para testing

### 4. Importación Flexible
- **AWS Real**: Conecta a DynamoDB en AWS
- **LocalStack**: Para desarrollo local sin costos
- Credenciales via env vars o ~/.aws/credentials

### 5. Robustez
- Importación en batches (evita timeouts)
- Manejo de errores por item
- Reintentos automáticos
- Logging detallado

### 6. Testing Completo
- 13 tests unitarios
- Cobertura de validaciones
- Tests de edge cases
- Fácil de ejecutar

---

## Instalación y Uso

### Instalación Rápida

```bash
# 1. Instalar dependencias
pip install -r scripts/requirements.txt

# 2. Hacer script ejecutable (Linux/Mac)
chmod +x scripts/import-dynamodb.sh

# 3. Verificar instalación
python3 scripts/test-import-dynamodb.py
```

### Uso Básico

```bash
# Validar datos primero
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --dry-run

# Si no hay errores, importar
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json
```

### Uso Avanzado

```bash
# LocalStack (desarrollo)
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones.json \
  --endpoint-url http://localhost:4566

# Con más detalles
./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json \
  --verbose

# Personalizar batch size
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --batch-size 10
```

---

## Estructura de Directorios

```
go-agro-sentinel-worker/
├── scripts/
│   ├── import-dynamodb.py           ✓ Script principal (Python)
│   ├── import-dynamodb.sh           ✓ Wrapper (Bash)
│   ├── test-import-dynamodb.py      ✓ Tests unitarios
│   ├── requirements.txt             ✓ Dependencias Python
│   └── README_SCRIPTS.md            ✓ Actualizado
│
├── data/
│   └── dynamodb/
│       └── examples/
│           ├── monitoring_producciones.json    ✓ 5 items
│           ├── monitoring_producciones.csv     ✓ 5 registros
│           ├── monitoring_escenas.json         ✓ 5 items
│           └── README.md                       ✓ Documentación
│
└── docs/
    ├── DYNAMODB_IMPORT_GUIDE.md       ✓ Guía completa (780 líneas)
    ├── DYNAMODB_IMPORT_QUICKSTART.md  ✓ Quick start (200 líneas)
    └── [otros docs existentes]
```

---

## Validaciones Implementadas

### monitoring_producciones
| Validación | Tipo | Detalles |
|------------|------|----------|
| produccion_id | Requerido | > 0, Number |
| activa | Requerido | Boolean |
| cultivo | Requerido | String |
| ciclo | Requerido | Formato YYYY-[A\|B] |
| dias_produccion | Requerido | Number, 60-200 |
| articulo_id | Requerido | > 0, Number |
| centro_costo_id | Requerido | > 0, Number |
| fecha_plantacion | Opcional | ISO YYYY-MM-DD |
| nombre_rancho | Opcional | String |

### monitoring_escenas
| Validación | Tipo | Detalles |
|------------|------|----------|
| scene_id | Requerido | String, no vacío |
| produccion_id | Requerido | > 0, Number |
| date | Requerido | ISO YYYY-MM-DD |
| cloud_cover | Requerido | Number, 0-100 |
| stac_assets | Opcional | Map con href y resolution |

---

## Ejemplos Incluidos

### Producciones (5 items)
1. Maíz ciclo A (Rancho El Remanso) - activa
2. Soja ciclo A (Finca La Esperanza) - activa
3. Trigo ciclo B (Rancho El Remanso) - activa
4. Algodón ciclo B - inactiva (ejemplo de estado)
5. Maíz ciclo A (Campos del Amanecer) - activa

### Escenas (5 items)
1. S2A escena Feb 5, 2026 - Prod 12345 - Cloud 12.5%
2. S2B escena Feb 10, 2026 - Prod 12345 - Cloud 8.3%
3. S2A escena Feb 15, 2026 - Prod 12346 - Cloud 5.2%
4. S2B escena Feb 20, 2026 - Prod 12347 - Cloud 0.0%
5. S2A escena Feb 25, 2026 - Prod 12349 - Cloud 15.7%

Cada escena incluye bandas B02, B03, B04, B08, B11 con URLs realistas.

---

## Testing

### Ejecutar Tests
```bash
python3 scripts/test-import-dynamodb.py
```

### Salida Esperada
```
============================================================
DynamoDB Import Tool - Test Suite
============================================================

Test 1: Cargar datos desde JSON... ✓
Test 2: Cargar datos desde CSV... ✓
Test 3: Validar producción válida... ✓
Test 4: Rechazar ciclo inválido... ✓
Test 5: Rechazar dias_produccion inválido... ✓
Test 6: Rechazar campo requerido faltante... ✓
Test 7: Validar escena válida... ✓
Test 8: Validar rango de cloud_cover... ✓
Test 9: Convertir tipos de datos... ✓
Test 10: Esquemas de tablas... ✓
Test 11: Validación de fechas... ✓
Test 12: Validación de ciclos... ✓
Test 13: Rechazar tabla desconocida... ✓

============================================================
Resultados: 13 pasados, 0 fallidos
============================================================
```

---

## Requisitos

### Obligatorios
- Python 3.7 o superior
- boto3 (AWS SDK para Python)
- Bash 4.0+ (para script wrapper)

### Opcionales
- AWS credenciales configuradas (si usa AWS real)
- LocalStack (para desarrollo local)
- jq (para manipulación de JSON)

---

## Próximos Pasos

Los usuarios pueden:

1. **Usar ejemplos tal cual:**
   ```bash
   ./scripts/import-dynamodb.sh \
     --table monitoring_producciones \
     --file data/dynamodb/examples/monitoring_producciones.json
   ```

2. **Crear archivos personalizados:**
   ```bash
   cp data/dynamodb/examples/monitoring_producciones.json data/mis_datos.json
   # Editar data/mis_datos.json con sus datos
   ./scripts/import-dynamodb.sh --table monitoring_producciones --file data/mis_datos.json
   ```

3. **Importar desde CSV de Excel:**
   - Crear hoja con columnas del esquema
   - Exportar como CSV
   - Importar con el script

4. **Automatizar en CI/CD:**
   - Usar GitHub Actions (ej. en DYNAMODB_IMPORT_GUIDE.md)
   - Programar con cron jobs
   - Integrar en pipelines

---

## Documentación Relacionada

- `docs/DYNAMODB_STRUCTURE.md` - Esquema detallado completo
- `docs/DEPLOYMENT_PLAN.md` - Plan de deployment general
- `scripts/README_SCRIPTS.md` - Referencia de todos los scripts

---

## Notas Técnicas

### Validaciones por Tabla
La herramienta mantiene esquemas por tabla en `TABLE_SCHEMAS` dictionary:
- Campos requeridos vs opcionales
- Tipos de datos esperados
- Partition key y sort key
- Validaciones específicas

### Importación en Batches
- Default: 25 items por batch
- Configurable con `--batch-size`
- Optimiza consumo de RCUs en DynamoDB
- Evita timeouts en importaciones grandes

### Conversión de Tipos
- String → Number (int/float)
- String → Boolean ("true"/"false" case-insensitive)
- Preserva tipos ya correctos
- Omite campos vacíos (DynamoDB no almacena NULL)

### Manejo de Errores
- Validación pre-importación
- Errores por item con línea específica
- Resumen de importación al final
- Exit codes correctos (0=éxito, 1=fallos, 2=error fatal)

---

## Soporte

Para preguntas o problemas:
1. Revisar `docs/DYNAMODB_IMPORT_GUIDE.md`
2. Ejecutar `./scripts/import-dynamodb.sh --help`
3. Revisar sección Troubleshooting en guía
4. Ejecutar tests: `python3 scripts/test-import-dynamodb.py`

---

**Creado:** 2026-09-04  
**Por:** Claude Code Agent (Anthropic)  
**Proyecto:** Agro Sentinel Worker  
**Estado:** Completo y Listo para Usar

