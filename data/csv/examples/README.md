# Archivos CSV de Ejemplo - Sistema Agro Sentinel

Este directorio contiene archivos CSV de ejemplo listos para importar a la base de datos del Sistema Agro Sentinel.

## Archivos Incluidos

### 1. articulos.csv
**Descripción:** Cultivos/artículos que se producen en los ranchos.

**Estructura:**
```
articulo_id,nombre,variedad
```

**Contenido de ejemplo:**
- Lechuga (variedad Latina)
- Tomate (variedad Cherry)
- Pepino (variedad Tipo Japonés)
- Cilantro (variedad Común)
- Espinaca (variedad Gigante de Holanda)
- Brócoli (variedad Calabrese)
- Zanahoria (variedad Nantes)
- Cebolla (variedad Amarilla)
- Ajo (variedad Blanco)
- Papa (variedad Spunta)

**Notas:**
- El campo `articulo_id` debe ser único
- El campo `nombre` es obligatorio
- El campo `variedad` es opcional

---

### 2. centros_costos.csv
**Descripción:** Ranchos o centros de costo donde se producen los cultivos.

**Estructura:**
```
centro_costo_id,nombre
```

**Contenido de ejemplo:**
- Rancho Los Andes
- Rancho San Miguel
- Rancho El Peñol
- Rancho La Esperanza
- Rancho Santa Rosa

**Notas:**
- El campo `centro_costo_id` debe ser único
- El campo `nombre` es obligatorio y debe ser descriptivo

---

### 3. zonas_producciones.csv
**Descripción:** Zonas geográficas dentro de los ranchos para delimitar áreas de cultivo.

**Estructura:**
```
zona_produccion_id,nombre,nombre_corto,estatus,area,centro_costo_id,poligono
```

**Contenido de ejemplo:**
- Zona Norte Los Andes (ZNA) - Área: 5.50 ha - Rancho Los Andes
- Zona Sur Los Andes (ZSA) - Área: 4.20 ha - Rancho Los Andes
- Zona Este San Miguel (ZES) - Área: 6.00 ha - Rancho San Miguel
- Zona Central El Peñol (ZCP) - Área: 5.80 ha - Rancho El Peñol
- Zona Oeste Esperanza (ZOE) - Área: 7.25 ha - Rancho La Esperanza

**Notas:**
- El campo `poligono` contiene coordenadas GeoJSON
- El campo `estatus` debe ser `P` (Producción) o `I` (Inactiva)
- El campo `area` está en hectáreas
- Las zonas deben estar asociadas a un centro de costo existente

---

### 4. producciones.csv
**Descripción:** Registro de producciones realizadas con detalles de cultivos y ranchos.

**Estructura:**
```
folio,articulo_id,fecha,hora,fecha_cierre,hora_cierre,usuario,estatus,aplicado,cantidad,centro_costo_id,monitoring
```

**Contenido de ejemplo:**
- CSJ2601-17-A: Lechuga en Rancho Los Andes (2.50 ha) - Monitoreo ACTIVO
- CSJ2602-24: Tomate en Rancho San Miguel (3.75 ha) - Monitoreo ACTIVO
- CSJ2603-31: Pepino en Rancho El Peñol (4.20 ha) - Monitoreo ACTIVO
- CSJ2604-45: Cilantro en Rancho Los Andes (1.80 ha) - Sin monitoreo
- CSJ2605-52: Espinaca en Rancho La Esperanza (2.10 ha) - Monitoreo ACTIVO

**Notas:**
- El campo `folio` es el ID único de cada producción
- El campo `fecha` y `hora` son obligatorios
- El campo `estatus` debe ser: N (Normal), T (Terminada), C (Cancelada)
- El campo `aplicado` debe ser: S (Sí), N (No)
- El campo `monitoring` debe ser: 0 (No), 1 (Sí)
- La `cantidad` es el área en hectáreas

---

### 5. asignaciones_zonas_producciones.csv
**Descripción:** Tabla relacional que asigna zonas a producciones específicas.

**Estructura:**
```
asignacion_zona_prod_id,produccion_id,zona_produccion_id,tipo_asignacion,area,poligono
```

**Contenido de ejemplo:**
- Asignación 1: Producción 1 (Lechuga) → Zona 1 (Zona Norte Los Andes)
- Asignación 2: Producción 2 (Tomate) → Zona 3 (Zona Este San Miguel)
- Asignación 3: Producción 3 (Pepino) → Zona 4 (Zona Central El Peñol) - TOTAL
- Asignación 4: Producción 4 (Cilantro) → Zona 2 (Zona Sur Los Andes)
- Asignación 5: Producción 5 (Espinaca) → Zona 1 (Zona Norte Los Andes)

**Notas:**
- El campo `tipo_asignacion` debe ser: T (Total), P (Parcial)
- El área asignada no debe superar el área disponible de la zona
- Una producción puede estar asignada a múltiples zonas
- El campo `poligono` especifica el área exacta si es asignación parcial

---

### 6. s3_monitoring_escena_ia_resumen.csv
**Descripción:** Resumen de análisis de IA sobre escenas satelitales.

**Estructura:**
```
s3_monitoring_escena_id,estado_clave,estado_general,riesgo_nivel,riesgo_motivo,fecha_analisis,json_original
```

**Contenido de ejemplo:**
- Escena 1001: Estado NORMAL - Riesgo BAJO - Sin anomalías detectadas
- Escena 1002: Estado ALERTA - Riesgo MEDIO - Posible enfermedad fúngica (powdery mildew)
- Escena 1003: Estado CRÍTICO - Riesgo ALTO - Alto estrés hídrico detectado

**Notas:**
- El campo `estado_clave` debe ser: normal, alerta, crítico
- El campo `riesgo_nivel` debe ser: bajo, medio, alto
- El campo `json_original` contiene la respuesta completa del análisis IA
- Las fechas están en formato ISO 8601 (YYYY-MM-DD HH:MM:SS)

---

## Cómo Usar Estos Archivos

### Opción 1: Importación automática completa
```bash
./scripts/import-csv.sh import-all
```

### Opción 2: Importación selectiva
```bash
./scripts/import-csv.sh import articulos data/csv/examples/articulos.csv
./scripts/import-csv.sh import centros_costos data/csv/examples/centros_costos.csv
```

### Opción 3: Importación con SQL directo
```bash
mysql -h localhost -u admin -padmin agro < scripts/import-from-csv.sql
```

### Opción 4: PowerShell (Windows)
```powershell
.\scripts\import-csv.ps1 -Comando ImportAll
```

---

## Crear Archivos CSV Personalizados

### Paso 1: Crear archivo nuevo
```bash
cp articulos.csv articulos_customizado.csv
```

### Paso 2: Editar con tu editor favorito
Puedes usar:
- Excel / Google Sheets (guardar como CSV)
- VS Code
- Notepad++
- LibreOffice Calc

### Paso 3: Validar estructura
```bash
./scripts/import-csv.sh validate articulos_customizado.csv
```

### Paso 4: Importar
```bash
./scripts/import-csv.sh import articulos data/csv/custom/articulos_customizado.csv
```

---

## Estructura de Directorios Recomendada

```
data/csv/
├── examples/              # Archivos de ejemplo (plantillas)
│   ├── articulos.csv
│   ├── centros_costos.csv
│   ├── producciones.csv
│   ├── zonas_producciones.csv
│   ├── asignaciones_zonas_producciones.csv
│   └── s3_monitoring_escena_ia_resumen.csv
├── custom/                # Tus archivos personalizados
│   └── (tus archivos aquí)
├── backups/               # Backups de importaciones
│   └── (archivos de backup)
└── archive/               # Archivos históricos
    └── (archivos antiguos)
```

---

## Validaciones Importantes

### Antes de importar:
1. ✓ Verificar que los encabezados coincidan exactamente
2. ✓ Verificar que los tipos de datos sean correctos
3. ✓ Verificar que no haya espacios en blanco al final de las líneas
4. ✓ Verificar que los valores obligatorios no estén vacíos
5. ✓ Verificar que los valores enumerados sean válidos

### Después de importar:
1. ✓ Verificar que la cantidad de registros sea la esperada
2. ✓ Verificar que las referencias foráneas sean válidas
3. ✓ Verificar que no haya duplicados
4. ✓ Verificar que los datos se vean correctos en la base de datos

---

## Requisitos del Sistema

- MySQL 5.7 o superior
- Base de datos "agro" creada
- Permisos FILE en MySQL
- Acceso a línea de comandos (Bash o PowerShell)
- Archivos CSV en formato UTF-8

---

## Notas Técnicas

### Formato CSV
- **Separador de campos:** Coma (`,`)
- **Comillas:** Doble comilla (`"`)
- **Salto de línea:** LF (`\n`)
- **Codificación:** UTF-8
- **Encabezado:** Sí (primera línea)

### Campos especiales

#### GeoJSON (poligono)
```json
[[latitude1, longitude1], [latitude2, longitude2], ...]
```
Ejemplo: `[[21.10535, -100.93089], [21.10535, -100.92730], [21.1068, -100.92730]]`

#### JSON (json_original)
```json
{"status": "normal", "score": 0.95}
```
Las comillas internas deben escaparse: `{""status"": ""normal""}`

#### Fechas
Formato: `YYYY-MM-DD` (ejemplo: `2026-06-03`)

#### Horas
Formato: `HH:MM:SS` (ejemplo: `08:30:00`)

#### DateTime
Formato: `YYYY-MM-DD HH:MM:SS` (ejemplo: `2026-06-03 10:30:00`)

#### Decimales
Usar punto como separador decimal (ejemplo: `2.50` no `2,50`)

---

## Troubleshooting

### Problema: "Archivo no encontrado"
**Solución:** Verificar que el archivo existe y usar ruta correcta
```bash
ls -la data/csv/examples/articulos.csv
```

### Problema: "Encoding incorrecto"
**Solución:** Guardar el archivo en UTF-8
```bash
# Convertir a UTF-8
iconv -f ISO-8859-1 -t UTF-8 articulos.csv > articulos_utf8.csv
```

### Problema: "Comillas mal escapadas"
**Solución:** Verificar que las comillas en JSON estén bien escapadas
```csv
-- Incorrecto
json_original,{"status": "normal"}

-- Correcto
json_original,"{""status"": ""normal""}"
```

---

## Más Información

- Documentación completa: `docs/CSV_IMPORT_GUIDE.md`
- Referencia rápida: `docs/CSV_QUICK_REFERENCE.md`
- Scripts de importación: `scripts/import-csv.sh`, `scripts/import-csv.ps1`

---

**Última actualización:** 2026-09-04
**Versión:** 1.0
