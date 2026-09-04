# DynamoDB Import Tools - Índice de Documentación

Bienvenido a las herramientas de importación de datos para DynamoDB en el Sistema Agro Sentinel.

---

## Inicio Rápido (5 minutos)

Si necesitas importar datos en 5 minutos:

👉 **[Guía de Inicio Rápido](DYNAMODB_IMPORT_QUICKSTART.md)**

- Instalación de requisitos
- Comandos básicos
- Errores comunes y soluciones
- Primeras importaciones

---

## Documentación Completa

### Para Usuarios de Importación

**[Guía Completa de Importación](DYNAMODB_IMPORT_GUIDE.md)** (780 líneas)

Cubre:
- Tablas y esquemas en detalle
- Formatos JSON y CSV
- Integración con AWS real
- Desarrollo local con LocalStack
- Mejores prácticas
- Troubleshooting
- CI/CD integration

### Para Desarrolladores

**[Resumen de Implementación](../DYNAMODB_IMPORT_SUMMARY.md)** (400 líneas)

Incluye:
- Descripción técnica de archivos
- Validaciones implementadas
- Arquitectura de la solución
- Ejemplos de datos
- Testing

### Para Entender DynamoDB

**[Estructura de DynamoDB](DYNAMODB_STRUCTURE.md)** (1000+ líneas)

Documentación completa de:
- Esquema de tablas
- Campos y tipos
- Patrones de acceso
- Sincronización MySQL ↔ DynamoDB
- Performance y optimización

---

## Ejemplos de Datos

**[Archivos de Ejemplo](../data/dynamodb/examples/README.md)**

Incluye:
- `monitoring_producciones.json` - 5 producciones ejemplo
- `monitoring_producciones.csv` - Mismo en CSV
- `monitoring_escenas.json` - 5 escenas de satélite
- Cómo personalizar los ejemplos
- Referencia de valores válidos

---

## Scripts y Herramientas

### Script Bash Wrapper
**`scripts/import-dynamodb.sh`** - Interfaz amigable para importar

```bash
./scripts/import-dynamodb.sh --help
```

### Script Python Principal
**`scripts/import-dynamodb.py`** - Lógica de importación y validación

```bash
python3 scripts/import-dynamodb.py --help
```

### Tests Unitarios
**`scripts/test-import-dynamodb.py`** - 13 tests para validar la herramienta

```bash
python3 scripts/test-import-dynamodb.py
```

### Dependencias
**`scripts/requirements.txt`** - Instala boto3

```bash
pip install -r scripts/requirements.txt
```

---

## Mapa de Navegación

```
¿QUÉ QUIERO HACER?

├─ Importar datos YA
│  └─ → Guía de Inicio Rápido (5 min)
│
├─ Entender cómo funciona
│  ├─ Esquema de tablas → DYNAMODB_STRUCTURE.md
│  ├─ Validaciones → DYNAMODB_IMPORT_SUMMARY.md
│  └─ Casos de uso → DYNAMODB_IMPORT_GUIDE.md
│
├─ Crear mis propios datos
│  └─ → data/dynamodb/examples/README.md
│
├─ Usar LocalStack (desarrollo)
│  └─ → Sección en DYNAMODB_IMPORT_GUIDE.md
│
├─ Integrar con CI/CD
│  └─ → Sección en DYNAMODB_IMPORT_GUIDE.md
│
├─ Resolver un problema
│  ├─ Ayuda quick → DYNAMODB_IMPORT_QUICKSTART.md
│  └─ Troubleshooting completo → DYNAMODB_IMPORT_GUIDE.md
│
└─ Ver todos los detalles técnicos
   └─ → DYNAMODB_IMPORT_SUMMARY.md
```

---

## Comandos Comunes

### Instalación
```bash
pip install -r scripts/requirements.txt
```

### Validación (Sin modificar datos)
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --dry-run
```

### Importación Simple
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json
```

### Importación a LocalStack
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --endpoint-url http://localhost:4566
```

### Testing
```bash
python3 scripts/test-import-dynamodb.py
```

---

## Tablas Soportadas

### `monitoring_producciones`
Producciones agrícolas siendo monitoreadas

**Campos Clave:**
- `produccion_id` (PK) - ID único
- `activa` - Boolean, estado de monitoreo
- `cultivo` - Tipo de cultivo
- `ciclo` - Ciclo agrícola (YYYY-[A|B])
- `dias_produccion` - Duración (60-200)

[Ver esquema completo →](DYNAMODB_STRUCTURE.md#tabla-monitoring_producciones)

### `monitoring_escenas`
Escenas de satélite Sentinel-2

**Campos Clave:**
- `scene_id` (PK) - ID STAC único
- `produccion_id` (FK) - Producción asociada
- `date` - Fecha de captura
- `cloud_cover` - Cobertura de nubes (0-100)
- `stac_assets` - Bandas espectrales

[Ver esquema completo →](DYNAMODB_STRUCTURE.md#tabla-monitoring_escenas)

---

## Validaciones Automáticas

La herramienta valida automáticamente:

### monitoring_producciones
- ✓ Campos requeridos presentes
- ✓ Tipos de datos correctos
- ✓ `produccion_id` > 0
- ✓ `ciclo` formato YYYY-[A|B]
- ✓ `dias_produccion` entre 60-200
- ✓ `fecha_plantacion` ISO YYYY-MM-DD (si existe)

### monitoring_escenas
- ✓ Campos requeridos presentes
- ✓ Tipos de datos correctos
- ✓ `produccion_id` > 0
- ✓ `cloud_cover` entre 0-100
- ✓ `date` ISO YYYY-MM-DD
- ✓ STAC assets estructura correcta

---

## Características

### Formatos Soportados
- ✓ JSON (recomendado, soporta estructuras complejas)
- ✓ CSV (simple, fácil desde Excel)

### Destinos Soportados
- ✓ AWS DynamoDB real
- ✓ LocalStack (desarrollo local)

### Modos de Operación
- ✓ Validación (dry-run, sin modificar)
- ✓ Importación real
- ✓ Testing automático

### Robustez
- ✓ Importación en batches
- ✓ Manejo de errores por item
- ✓ Logging detallado
- ✓ Conversión automática de tipos

---

## Ejemplos Incluidos

### Producciones (5 items)
1. Maíz ciclo A - Rancho El Remanso (activa)
2. Soja ciclo A - Finca La Esperanza (activa)
3. Trigo ciclo B - Rancho El Remanso (activa)
4. Algodón ciclo B - Sin rancho (inactiva)
5. Maíz ciclo A - Campos del Amanecer (activa)

### Escenas (5 items)
1. Sentinel-2A - Feb 5, 2026 - Cloud 12.5%
2. Sentinel-2B - Feb 10, 2026 - Cloud 8.3%
3. Sentinel-2A - Feb 15, 2026 - Cloud 5.2%
4. Sentinel-2B - Feb 20, 2026 - Cloud 0.0%
5. Sentinel-2A - Feb 25, 2026 - Cloud 15.7%

---

## Requisitos

### Obligatorios
- Python 3.7+
- boto3 (`pip install boto3`)

### Opcionales
- Credenciales AWS (para AWS real)
- LocalStack (para desarrollo sin costos)

---

## Flujo Típico de Trabajo

1. **Preparar datos**
   - Copiar ejemplo
   - Editar con tus datos
   - Validar JSON/CSV

2. **Validar** (sin modificar)
   ```bash
   ./scripts/import-dynamodb.sh --table ... --file ... --dry-run
   ```

3. **Revisar salida**
   - ¿Todos los items son válidos?
   - ¿Errores? Revisar y corregir

4. **Importar**
   ```bash
   ./scripts/import-dynamodb.sh --table ... --file ...
   ```

5. **Verificar**
   - Contar items importados
   - Samplear datos

---

## Documentación por Rol

### Para Analistas / Usuarios
1. [Guía de Inicio Rápido](DYNAMODB_IMPORT_QUICKSTART.md) - Start here
2. [Ejemplos](../data/dynamodb/examples/README.md) - Cómo personalizar

### Para Administradores
1. [Guía Completa](DYNAMODB_IMPORT_GUIDE.md) - Configuración completa
2. [LocalStack Setup](DYNAMODB_IMPORT_GUIDE.md#desarrollo-local-con-localstack) - Dev environment
3. [AWS Integration](DYNAMODB_IMPORT_GUIDE.md#integración-con-aws) - Production

### Para Desarrolladores
1. [Resumen Técnico](../DYNAMODB_IMPORT_SUMMARY.md) - Arquitectura
2. [DYNAMODB_STRUCTURE.md](DYNAMODB_STRUCTURE.md) - Esquema detallado
3. Tests: `python3 scripts/test-import-dynamodb.py`

---

## Preguntas Frecuentes

**P: ¿Por dónde empiezo?**  
R: Ve a [Guía de Inicio Rápido](DYNAMODB_IMPORT_QUICKSTART.md)

**P: ¿Puedo usar datos de Excel?**  
R: Sí, exporta como CSV y usa el script

**P: ¿Necesito AWS configurado?**  
R: Para AWS real sí. Para desarrollo local usa LocalStack

**P: ¿Qué significa dry-run?**  
R: Valida datos sin modificar DynamoDB. Usa siempre primero

**P: ¿Cómo personalizo los ejemplos?**  
R: Copia el archivo JSON/CSV y edita. Ver en `data/dynamodb/examples/README.md`

---

## Soporte

### Problemas Comunes
1. Revisar [DYNAMODB_IMPORT_QUICKSTART.md](DYNAMODB_IMPORT_QUICKSTART.md#errores-comunes)
2. Revisar [Troubleshooting](DYNAMODB_IMPORT_GUIDE.md#troubleshooting) en guía completa

### Ver Ayuda
```bash
./scripts/import-dynamodb.sh --help
python3 scripts/import-dynamodb.py --help
```

### Ejecutar Tests
```bash
python3 scripts/test-import-dynamodb.py
```

---

## Archivos del Proyecto

```
go-agro-sentinel-worker/
├── docs/
│   ├── DYNAMODB_IMPORT_INDEX.md          ← Estás aquí
│   ├── DYNAMODB_IMPORT_QUICKSTART.md     ← Empieza aquí
│   ├── DYNAMODB_IMPORT_GUIDE.md          ← Completo
│   └── DYNAMODB_STRUCTURE.md             ← Esquemas
│
├── scripts/
│   ├── import-dynamodb.sh                ← Usar esto
│   ├── import-dynamodb.py                ← O esto
│   ├── test-import-dynamodb.py           ← Tests
│   ├── requirements.txt                  ← Dependencias
│   └── README_SCRIPTS.md                 ← Ref scripts
│
├── data/dynamodb/examples/
│   ├── monitoring_producciones.json      ← Ejemplo
│   ├── monitoring_producciones.csv       ← Ejemplo
│   ├── monitoring_escenas.json           ← Ejemplo
│   └── README.md                         ← Personalizar
│
└── DYNAMODB_IMPORT_SUMMARY.md            ← Implementación técnica
```

---

## Siguiente Paso

**[👉 Ir a Guía de Inicio Rápido](DYNAMODB_IMPORT_QUICKSTART.md)**

O selecciona por tu necesidad:
- [Importar datos ahora](DYNAMODB_IMPORT_QUICKSTART.md)
- [Entender el sistema](DYNAMODB_STRUCTURE.md)
- [Guía completa](DYNAMODB_IMPORT_GUIDE.md)
- [Ver detalles técnicos](../DYNAMODB_IMPORT_SUMMARY.md)

---

**Última actualización:** 2026-09-04  
**Versión:** 1.0  
**Sistema:** Agro Sentinel Worker
