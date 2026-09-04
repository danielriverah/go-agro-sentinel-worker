# Quick Start - Importación de Datos a DynamoDB

Guía rápida para importar datos a DynamoDB en 5 minutos.

---

## 1. Instalar Dependencias (Primera Vez)

```bash
# En Linux/Mac/PowerShell
pip install -r scripts/requirements.txt

# O solo boto3
pip install boto3
```

---

## 2. Validar Datos (Recomendado)

```bash
# Verificar que los datos están bien antes de importar
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --dry-run
```

Deberías ver:
```
[INFO] Cargados 5 items desde JSON
[INFO] Validando items...
[INFO] Validados: 5, Inválidos: 0
[DRY-RUN] Se escribirían los siguientes items:
  Item 1: {...}
```

---

## 3. Importar Datos

### Opción A: AWS Real

```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json
```

Cuando se te pida confirmación, responde `s`.

### Opción B: LocalStack (Desarrollo Local)

```bash
# Primero, asegurar que LocalStack está corriendo
# docker-compose up -d (si usas docker-compose)
# o localstack start

./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --endpoint-url http://localhost:4566
```

---

## 4. Verificar Importación

```bash
# Ver cantidad de items
aws dynamodb scan \
  --table-name monitoring_producciones \
  --select COUNT \
  --region us-west-2

# Ver detalles de un item específico
aws dynamodb get-item \
  --table-name monitoring_producciones \
  --key '{"produccion_id":{"N":"12345"}}' \
  --region us-west-2
```

---

## Comandos Frecuentes

### Importar Escenas
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_escenas \
  --file data/dynamodb/examples/monitoring_escenas.json
```

### Usar Archivo CSV
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/monitoring_producciones.csv
```

### Con Más Detalles
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --verbose
```

### Con Diferente Región
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/dynamodb/examples/monitoring_producciones.json \
  --region us-east-1
```

---

## Errores Comunes

### "boto3 no está instalado"
```bash
pip install boto3
```

### "Credenciales AWS no encontradas"
```bash
aws configure
# O establecer variables de entorno
export AWS_ACCESS_KEY_ID="..."
export AWS_SECRET_ACCESS_KEY="..."
```

### "JSON malformado"
```bash
# Validar JSON
python3 -m json.tool data/dynamodb/examples/monitoring_producciones.json
```

### "Tabla no existe"
```bash
# Crear tabla
aws dynamodb create-table \
  --table-name monitoring_producciones \
  --attribute-definitions AttributeName=produccion_id,AttributeType=N \
  --key-schema AttributeName=produccion_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST \
  --region us-west-2
```

---

## Paso a Paso: Mi Primer Import

### 1. Copiar ejemplo como base
```bash
cp data/dynamodb/examples/monitoring_producciones.json \
   data/mi_datos.json
```

### 2. Editar con tus datos
```bash
# En Windows: notepad data/mi_datos.json
# En Mac: open -a TextEdit data/mi_datos.json
# En Linux: vim data/mi_datos.json
```

### 3. Validar
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/mi_datos.json \
  --dry-run
```

### 4. Si es válido, importar
```bash
./scripts/import-dynamodb.sh \
  --table monitoring_producciones \
  --file data/mi_datos.json
```

Presionar `s` cuando se pida confirmación.

---

## Documentación Completa

- **Guía Completa**: `docs/DYNAMODB_IMPORT_GUIDE.md`
- **Esquema Detallado**: `docs/DYNAMODB_STRUCTURE.md`
- **Ejemplos**: `data/dynamodb/examples/README.md`
- **Ayuda del Script**: `./scripts/import-dynamodb.sh --help`

---

## Cheat Sheet

```bash
# Validación
--dry-run            # Ver qué se importaría sin hacer cambios

# Ubicación
--table              # Tabla: monitoring_producciones|monitoring_escenas
--file               # Ruta del JSON o CSV

# Conectividad
--endpoint-url       # http://localhost:4566 para LocalStack
--region             # Región AWS (default: us-west-2)

# Debug
--verbose            # Mostrar más detalles
--batch-size 10      # Para imports lentos

# Ayuda
--help               # Ver todas las opciones
```

---

**¿Necesitas ayuda?** Ve a `docs/DYNAMODB_IMPORT_GUIDE.md` para documentación completa.

**Última actualización**: 2026-09-04
