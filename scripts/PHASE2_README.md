# FASE 2: Actualización de Tabla s3_monitoring_producciones

## Descripción General

Este conjunto de scripts SQL implementa la Fase 2 del Sistema de Monitoreo Agro Sentinel, agregando nuevas relaciones y campos a la tabla `s3_monitoring_producciones` para conectarla con las tablas de referencias maestras creadas en Fase 1.

## Cambios Implementados

### Nuevos Campos Agregados

1. **articulo_id** (INT, NULL)
   - Posición: Después de `produccion_id`
   - Descripción: Referencia a la tabla `articulos` (cultivos/artículos producidos)
   - Constraint: Foreign Key con DELETE SET NULL, UPDATE CASCADE
   - Índice: `idx_s3mon_prod_articulo_id`

2. **centro_costo_id** (INT, NULL)
   - Posición: Después de `articulo_id`
   - Descripción: Referencia a la tabla `centros_costos` (ranchos/ubicaciones)
   - Constraint: Foreign Key con DELETE SET NULL, UPDATE CASCADE
   - Índice: `idx_s3mon_prod_centro_costo_id`

3. **nombre_rancho** (VARCHAR(200), NULL)
   - Posición: Después de `centro_costo_id`
   - Descripción: Copia desnormalizada del nombre del rancho para facilitar queries sin joins
   - Charset: utf8mb4 (consistente con el resto de la base de datos)

### Índices Creados

- `idx_s3mon_prod_articulo_id`: Optimiza búsquedas por artículo
- `idx_s3mon_prod_centro_costo_id`: Optimiza búsquedas por centro de costo
- `idx_s3mon_prod_centro_monitoring`: Índice compuesto (centro_costo_id, monitoring) para búsquedas comunes

### Constraints Agregadas

- `fk_s3mon_prod_articulo_id`: Foreign Key hacia `articulos(articulo_id)`
  - Acción al eliminar artículo: SET NULL
  - Acción al actualizar: CASCADE

- `fk_s3mon_prod_centro_costo_id`: Foreign Key hacia `centros_costos(centro_costo_id)`
  - Acción al eliminar centro de costo: SET NULL
  - Acción al actualizar: CASCADE

## Estructura del Script

### phase2-update-s3-monitoring-producciones.sql

**Paso 1:** Agregar nuevos campos
- Crea las tres columnas nuevas en la tabla

**Paso 2:** Crear índices
- Agrupa queries frecuentes

**Paso 3:** Crear Foreign Keys
- Establece las relaciones con tablas maestras

**Paso 4:** Actualizar datos existentes
- Popula `articulo_id` desde la tabla `producciones`
- Popula `centro_costo_id` desde la tabla `producciones`
- Popula `nombre_rancho` desde la tabla `centros_costos`

**Paso 5:** Verificación de datos
- Muestra la nueva estructura
- Verifica datos populados
- Genera estadísticas
- Valida integridad de FKs

## Instrucciones de Ejecución

### Opción 1: Ejecución Directa en MySQL

```bash
mysql -u admin -p agro < scripts/phase2-update-s3-monitoring-producciones.sql
```

### Opción 2: Ejecución mediante Workbench

1. Abrir MySQL Workbench
2. Conectar a la base de datos `agro`
3. Abrir el archivo `phase2-update-s3-monitoring-producciones.sql`
4. Ejecutar el script completo (Ctrl+Shift+Enter)
5. Revisar los resultados en la sección de verificación

### Opción 3: Ejecución con Docker

```bash
docker exec -i nombre_contenedor_mysql mysql -u admin -padmin agro < scripts/phase2-update-s3-monitoring-producciones.sql
```

### Opción 4: Ejecución paso a paso (Recomendado para debugging)

Ejecutar cada sección SQL por separado:
1. Sección de ALTER TABLE
2. Sección de índices
3. Sección de Foreign Keys
4. Sección de UPDATE
5. Sección de verificación

## Validaciones Incluidas

El script incluye múltiples validaciones:

1. **DESCRIBE table**: Confirma la estructura actualizada
2. **SELECT con JOINs**: Verifica datos populados correctamente
3. **Estadísticas**: Cuenta registros con cada campo
4. **Integridad de FKs**: Detecta referencias huérfanas (none expected)

## Rollback (Revertir Cambios)

Si es necesario deshacer los cambios, ejecutar:

```bash
mysql -u admin -p agro < scripts/phase2-rollback-s3-monitoring-producciones.sql
```

**ADVERTENCIA:** El rollback eliminará permanentemente los datos de los tres campos nuevos.

## Consideraciones Importantes

### Compatibilidad con el Código Existente

Los cambios son **backward compatible**:
- Los campos nuevos son NULL por defecto
- El código existente seguirá funcionando sin modificaciones
- Las Foreign Keys usan DELETE SET NULL para máxima compatibilidad

### Rendimiento

Los índices agregados mejoran el rendimiento en:
- Búsquedas por artículo: `SELECT * WHERE articulo_id = ?`
- Búsquedas por centro de costo: `SELECT * WHERE centro_costo_id = ?`
- Búsquedas combinadas: `SELECT * WHERE centro_costo_id = ? AND monitoring = ?`

### Datos Históricos

Si existen registros sin `produccion_id` válido:
- Los campos `articulo_id` y `centro_costo_id` quedarán NULL
- El campo `nombre_rancho` también quedará NULL
- Esto es correcto y no causa inconsistencias

## Próximos Pasos (Fase 3)

Después de ejecutar este script, considerar:

1. **Actualizar el código Go**: Modificar `migrations.go` para incluir estos campos en la definición de tabla
2. **Actualizar structs de dominio**: Agregar campos a `domain.Production`
3. **Actualizar queries**: Aprovechar los nuevos índices para optimizar búsquedas
4. **Tests**: Agregar tests para verificar las nuevas relaciones

## Soporte y Troubleshooting

### Error: "Foreign key constraint fails"

**Causa:** Existen valores en `articulo_id` o `centro_costo_id` que no corresponden a registros válidos

**Solución:** Revisar datos antes de ejecutar el script o permitir NULL y ejecutar el rollback

### Error: "Duplicate key name"

**Causa:** Los índices ya existen (ejecución duplicada)

**Solución:** Cambiar `CREATE INDEX` por `CREATE INDEX IF NOT EXISTS` (MySQL 5.7.4+)

### Rendimiento lento durante UPDATE

**Causa:** Tablas grandes sin índices temporales

**Solución:** Ejecutar en horarios de bajo tráfico o usar transacciones con checkpoints

## Archivos de Referencia

- **Fase 1**: `scripts/phase1-create-tables.sql` - Crear tablas maestras (articulos, centros_costos, producciones)
- **Fase 2**: `scripts/phase2-update-s3-monitoring-producciones.sql` - Script principal (ESTE ARCHIVO)
- **Rollback**: `scripts/phase2-rollback-s3-monitoring-producciones.sql` - Revertir cambios
- **Documentación**: `docs/superpowers/specs/2026-09-01-agro-sentinel-worker-design.md` - Especificación completa
- **Plan**: `docs/superpowers/plans/2026-09-01-agro-sentinel-worker.md` - Plan de implementación

## Auditoría y Logs

Para auditar cambios después de la ejecución:

```sql
-- Ver cambios recientes en s3_monitoring_producciones
SELECT id, articulo_id, centro_costo_id, nombre_rancho, updated_at
FROM s3_monitoring_producciones
WHERE updated_at > DATE_SUB(NOW(), INTERVAL 1 HOUR)
ORDER BY updated_at DESC;

-- Verificar integridad de datos
SELECT COUNT(*) as total, 
       COUNT(articulo_id) as con_articulo,
       COUNT(centro_costo_id) as con_centro_costo,
       COUNT(nombre_rancho) as con_nombre_rancho
FROM s3_monitoring_producciones;
```

## Contribuidor

**Fase 2 - Script SQL**: Daniel Rivera Herrada (drh.megafresh@gmail.com)
**Fecha**: 2026-09-04
**Sistema**: Agro Sentinel - Worker

---

**Última actualización**: 2026-09-04
