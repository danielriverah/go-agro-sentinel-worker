# Índice de Archivos - Sistema de Importación CSV

## Resumen

Se han creado herramientas y ejemplos completos para importar datos desde CSV en el Sistema Agro Sentinel. La solución incluye:

- ✓ 6 archivos CSV de ejemplo con datos realistas
- ✓ 3 scripts de importación (SQL, Bash, PowerShell)
- ✓ 3 documentos de referencia y guías
- ✓ Directorio de ejemplos organizado
- ✓ Configuración de variables de entorno

---

## Estructura de Archivos Creados

### 1. Archivos CSV de Ejemplo

**Ubicación:** `data/csv/examples/`

| Archivo | Registros | Descripción |
|---------|-----------|-------------|
| `articulos.csv` | 10 | Cultivos/artículos que se producen |
| `centros_costos.csv` | 5 | Ranchos o centros de costo |
| `zonas_producciones.csv` | 5 | Zonas geográficas dentro de ranchos |
| `producciones.csv` | 5 | Producciones realizadas con cultivos |
| `asignaciones_zonas_producciones.csv` | 5 | Asignación de zonas a producciones |
| `s3_monitoring_escena_ia_resumen.csv` | 3 | Análisis IA de escenas satelitales |

**Total:** 33 registros de ejemplo listos para importar

---

### 2. Scripts de Importación

**Ubicación:** `scripts/`

#### `import-from-csv.sql`
**Tipo:** Script SQL
**Propósito:** Importación completa de todas las tablas
**Características:**
- Importa todas las tablas en orden correcto
- Incluye validaciones previas
- Verifica integridad referencial
- Proporciona resumen final
- Compatible con MySQL Workbench y línea de comandos

**Uso:**
```bash
mysql -h localhost -u admin -padmin agro < scripts/import-from-csv.sql
```

---

#### `import-csv.sh`
**Tipo:** Script Bash
**Propósito:** Utilidad completa de importación para Linux/Mac
**Características:**
- Importar tablas individuales
- Importar todas las tablas
- Validar estructura de CSVs
- Listar tablas y registros
- Crear backups automáticos
- Variables de entorno
- Mensajes de error coloreados
- Manejo de errores robusto

**Uso:**
```bash
chmod +x scripts/import-csv.sh
./scripts/import-csv.sh import-all
```

---

#### `import-csv.ps1`
**Tipo:** Script PowerShell
**Propósito:** Utilidad de importación para Windows
**Características:**
- Misma funcionalidad que script Bash
- Interface PowerShell nativa
- Parámetros nombrados
- Validación integrada
- Compatible con Windows 10+

**Uso:**
```powershell
.\scripts\import-csv.ps1 -Comando ImportAll
```

---

### 3. Documentación

**Ubicación:** `docs/`

#### `CSV_IMPORT_GUIDE.md`
**Propósito:** Guía completa de importación
**Contenido:**
- Requisitos previos
- Estructura detallada de cada tabla
- Descripción de campos y validaciones
- 4 métodos diferentes de importación
- Validaciones y errores comunes con soluciones
- Ejemplos paso a paso
- Troubleshooting
- Mejores prácticas
- Referencias rápidas

**Tamaño:** ~8000 líneas
**Uso:** Referencia técnica completa

---

#### `CSV_QUICK_REFERENCE.md`
**Propósito:** Referencia rápida para desarrolladores
**Contenido:**
- Inicio rápido (3 líneas)
- Estructura de tablas en formato cheat sheet
- Comandos rápidos para Bash y PowerShell
- Valores válidos para cada campo
- Validaciones rápidas
- Errores comunes y soluciones (tabla)
- Verificaciones post-importación
- Tips y trucos
- Troubleshooting rápido

**Tamaño:** ~400 líneas
**Uso:** Referencia rápida durante desarrollo

---

#### `CSV_IMPORT_INDEX.md`
**Propósito:** Este archivo - Índice de todos los recursos
**Contenido:**
- Resumen de lo que se creó
- Descripción de cada archivo
- Guía de inicio rápido
- Flujo recomendado
- Matriz de compatibilidad
- Checklist de implementación

---

### 4. Archivos Adicionales

**Ubicación:** `data/csv/examples/`

#### `README.md`
**Propósito:** Documentación del directorio de ejemplos
**Contenido:**
- Descripción de cada CSV
- Estructura y contenido de ejemplo
- Cómo usar los archivos
- Crear archivos personalizados
- Estructura de directorios recomendada
- Validaciones importantes
- Notas técnicas

---

#### `.env.example`
**Propósito:** Archivo de configuración de ejemplo
**Contenido:**
- Variables de base de datos
- Rutas de importación
- Configuración de charset
- Opciones de logging
- Validaciones
- Opciones avanzadas

**Uso:**
```bash
cp data/csv/examples/.env.example .env
# Editar .env con tus valores
```

---

## Matriz de Compatibilidad

| Función | SQL | Bash | PowerShell | Manual |
|---------|-----|------|-----------|--------|
| Importar una tabla | ✓ | ✓ | ✓ | ✓ |
| Importar todas | ✓ | ✓ | ✓ | ✓ |
| Validar CSV | ✗ | ✓ | ✓ | ✗ |
| Listar tablas | ✗ | ✓ | ✓ | ✗ |
| Crear backup | ✗ | ✓ | ✓ | ✗ |
| Verificaciones | ✓ | ✓ | ✓ | ✗ |
| Variables entorno | ✗ | ✓ | ✓ | ✗ |
| GUI (Workbench) | ✓ | ✗ | ✗ | ✓ |

---

## Guía de Inicio Rápido

### 1. Preparación (5 minutos)

```bash
# Ir al proyecto
cd /xampp/htdocs/WEB/clientes/DRH/go-agro-sentinel-worker

# Hacer scripts ejecutables (Linux/Mac)
chmod +x scripts/import-csv.sh
```

### 2. Verificación (2 minutos)

```bash
# Verificar conexión a MySQL
mysql -h localhost -u admin -padmin agro -e "SELECT 1;"

# Ver archivos de ejemplo
ls -la data/csv/examples/*.csv
```

### 3. Importación (1 minuto)

**Opción A - Bash (Linux/Mac):**
```bash
./scripts/import-csv.sh import-all
```

**Opción B - PowerShell (Windows):**
```powershell
.\scripts\import-csv.ps1 -Comando ImportAll
```

**Opción C - SQL (Cualquier OS):**
```bash
mysql -h localhost -u admin -padmin agro < scripts/import-from-csv.sql
```

### 4. Verificación final (1 minuto)

```bash
# Ver registros importados
mysql -h localhost -u admin -padmin agro -e \
"SELECT COUNT(*) as 'Total Registros' FROM articulos;
SELECT COUNT(*) FROM centros_costos;
SELECT COUNT(*) FROM producciones;
SELECT COUNT(*) FROM zonas_producciones;"
```

---

## Flujo Recomendado de Uso

### Primer uso (Setup inicial)

```
1. Leer: CSV_IMPORT_GUIDE.md (introducción)
   ↓
2. Preparar: Verificar requisitos
   ↓
3. Probar: Importar ejemplos con import-csv.sh
   ↓
4. Validar: Verificar datos en base de datos
   ↓
5. Personalizar: Crear tus propios CSVs
```

### Uso posterior (Importaciones regulares)

```
1. Consultar: CSV_QUICK_REFERENCE.md
   ↓
2. Preparar: Tu archivo CSV personalizado
   ↓
3. Ejecutar: ./scripts/import-csv.sh import <tabla> <archivo>
   ↓
4. Verificar: Ver registros en base de datos
```

### Troubleshooting

```
1. Revisar: Sección "Errores comunes" de CSV_IMPORT_GUIDE.md
   ↓
2. Validar: ./scripts/import-csv.sh validate <archivo>
   ↓
3. Consultar: CSV_QUICK_REFERENCE.md sección Troubleshooting
   ↓
4. Contactar: Soporte técnico si persiste
```

---

## Checklist de Implementación

### Instalación inicial
- [ ] Crear directorio `data/csv/examples/`
- [ ] Copiar archivos CSV de ejemplo
- [ ] Copiar scripts de importación a `scripts/`
- [ ] Copiar documentación a `docs/`
- [ ] Hacer scripts ejecutables: `chmod +x scripts/import-csv.sh`

### Configuración
- [ ] Verificar conexión MySQL
- [ ] Habilitar `local_infile` en MySQL
- [ ] Crear archivo `.env` si es necesario
- [ ] Configurar variables de entorno

### Pruebas
- [ ] Validar archivos CSV: `./scripts/import-csv.sh validate`
- [ ] Crear backup: `./scripts/import-csv.sh backup`
- [ ] Importar datos: `./scripts/import-csv.sh import-all`
- [ ] Verificar integridad: Ver registros en BD

### Documentación
- [ ] Leer guía principal: `CSV_IMPORT_GUIDE.md`
- [ ] Bookmarkear referencia rápida: `CSV_QUICK_REFERENCE.md`
- [ ] Revisar ejemplos: `data/csv/examples/README.md`
- [ ] Compartir con equipo

---

## Archivos Principales por Rol

### Para Desarrolladores
- Documento: `CSV_QUICK_REFERENCE.md`
- Script: `import-csv.sh` (Linux/Mac) o `import-csv.ps1` (Windows)
- Ejemplos: `data/csv/examples/*.csv`

### Para Administradores
- Documento: `CSV_IMPORT_GUIDE.md` (sección Troubleshooting)
- Script: `import-from-csv.sql` para automatización
- Configuración: `.env` para variables de entorno

### Para Data Analysts
- Documento: `data/csv/examples/README.md`
- Script: `import-csv.sh validate` para validar datos
- Ejemplos: Ver estructura de CSVs

### Para Nuevos Miembros del Equipo
1. Leer: `CSV_IMPORT_INDEX.md` (este archivo)
2. Leer: `CSV_QUICK_REFERENCE.md`
3. Practicar: Ejecutar `import-csv.sh import-all`
4. Profundizar: `CSV_IMPORT_GUIDE.md` según necesidad

---

## Requisitos Previos

### Sistema Operativo
- Windows 10+, Linux, o macOS
- Bash o PowerShell disponibles
- MySQL Client instalado

### Base de Datos
- MySQL 5.7 o superior
- Base de datos "agro" creada
- Tablas creadas con scripts en `scripts/phase1-create-tables.sql`

### Permisos
- Acceso a archivos del proyecto
- Acceso a línea de comandos
- Permisos MySQL: FILE, SELECT, INSERT, UPDATE

---

## Localización de Archivos

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
│           ├── s3_monitoring_escena_ia_resumen.csv
│           ├── README.md
│           └── .env.example
├── scripts/
│   ├── import-csv.sh
│   ├── import-csv.ps1
│   └── import-from-csv.sql
├── docs/
│   ├── CSV_IMPORT_GUIDE.md
│   ├── CSV_QUICK_REFERENCE.md
│   └── CSV_IMPORT_INDEX.md (este archivo)
└── backups/
    └── (archivos de backup generados)
```

---

## Próximos Pasos

### Inmediatamente
1. Leer este índice
2. Ejecutar primera importación
3. Verificar datos en base de datos

### En los próximos días
1. Revisar `CSV_IMPORT_GUIDE.md`
2. Crear primeros CSVs personalizados
3. Configurar automatización si es necesario

### Para el futuro
1. Integrar con pipelines de CI/CD
2. Crear validaciones adicionales
3. Desarrollar dashboard de importación
4. Automatizar importaciones periódicas

---

## Soporte y Recursos

### Documentación
- Guía completa: `docs/CSV_IMPORT_GUIDE.md`
- Referencia rápida: `docs/CSV_QUICK_REFERENCE.md`
- Ejemplos: `data/csv/examples/README.md`

### Ejemplos
- Archivos CSV listos: `data/csv/examples/*.csv`
- Plantilla SQL: `scripts/import-from-csv.sql`
- Plantilla Bash: `scripts/import-csv.sh`
- Plantilla PowerShell: `scripts/import-csv.ps1`

### Troubleshooting
1. Revisar "Errores comunes" en CSV_IMPORT_GUIDE.md
2. Ejecutar validaciones: `import-csv.sh validate`
3. Revisar logs: `grep -i error import.log`
4. Consultar CSV_QUICK_REFERENCE.md

---

## Changelog

### Versión 1.0 (2026-09-04)
- ✓ Creados 6 archivos CSV de ejemplo
- ✓ Script SQL de importación completo
- ✓ Script Bash con funcionalidad avanzada
- ✓ Script PowerShell para Windows
- ✓ Guía completa de importación
- ✓ Referencia rápida
- ✓ Documentación de ejemplos
- ✓ Archivo de configuración

---

## Métricas

| Métrica | Valor |
|---------|-------|
| Archivos CSV creados | 6 |
| Registros de ejemplo | 33 |
| Scripts de importación | 3 |
| Documentos | 3 + 2 |
| Líneas de documentación | ~8500 |
| Líneas de código (scripts) | ~1500 |
| Tablas soportadas | 6 |
| Comandos disponibles | 6 |

---

**Versión:** 1.0
**Fecha:** 2026-09-04
**Estado:** Listo para producción
