-- =====================================================
-- FASE 2: ACTUALIZAR TABLA s3_monitoring_producciones
-- Sistema de Monitoreo Agro Sentinel
-- Agregar relaciones con articulos y centros_costos
-- =====================================================

USE agro;

-- =====================================================
-- PASO 1: AGREGAR NUEVOS CAMPOS A LA TABLA
-- =====================================================
-- Agregar articulo_id (int, FK a articulos) - después de produccion_id
-- Agregar centro_costo_id (int, FK a centros_costos) - después de articulo_id
-- Agregar nombre_rancho (varchar(200)) - después de centro_costo_id

ALTER TABLE s3_monitoring_producciones
ADD COLUMN articulo_id INT NULL COMMENT 'FK a articulos (cultivo/artículo producido)'
AFTER produccion_id;

ALTER TABLE s3_monitoring_producciones
ADD COLUMN centro_costo_id INT NULL COMMENT 'FK a centros_costos (rancho/ubicación de la producción)'
AFTER articulo_id;

ALTER TABLE s3_monitoring_producciones
ADD COLUMN nombre_rancho VARCHAR(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NULL COMMENT 'Nombre del rancho/centro de costo (copia desnormalizada para facilitar queries)'
AFTER centro_costo_id;

-- =====================================================
-- PASO 2: CREAR ÍNDICES PARA LOS CAMPOS NUEVOS
-- =====================================================
-- Índice para articulo_id para optimizar búsquedas y joins
CREATE INDEX idx_s3mon_prod_articulo_id ON s3_monitoring_producciones(articulo_id);

-- Índice para centro_costo_id para optimizar búsquedas y joins
CREATE INDEX idx_s3mon_prod_centro_costo_id ON s3_monitoring_producciones(centro_costo_id);

-- Índice compuesto para búsquedas comunes (centro_costo + monitoreo status)
CREATE INDEX idx_s3mon_prod_centro_monitoring ON s3_monitoring_producciones(centro_costo_id, monitoring);

-- =====================================================
-- PASO 3: CREAR FOREIGN KEYS CONSTRAINTS
-- =====================================================
-- FK hacia articulos table
ALTER TABLE s3_monitoring_producciones
ADD CONSTRAINT fk_s3mon_prod_articulo_id
FOREIGN KEY (articulo_id)
REFERENCES articulos(articulo_id)
ON DELETE SET NULL
ON UPDATE CASCADE
COMMENT 'Referencia a artículos (cultivos) disponibles';

-- FK hacia centros_costos table
ALTER TABLE s3_monitoring_producciones
ADD CONSTRAINT fk_s3mon_prod_centro_costo_id
FOREIGN KEY (centro_costo_id)
REFERENCES centros_costos(centro_costo_id)
ON DELETE SET NULL
ON UPDATE CASCADE
COMMENT 'Referencia a centros de costo (ranchos/ubicaciones)';

-- =====================================================
-- PASO 4: ACTUALIZAR DATOS EXISTENTES
-- =====================================================
-- Rellenar articulo_id desde la tabla producciones
-- Si la produccion_id existe en producciones, tomar su articulo_id
UPDATE s3_monitoring_producciones smp
JOIN producciones p ON smp.produccion_id = p.produccion_id
SET smp.articulo_id = p.articulo_id
WHERE smp.articulo_id IS NULL;

-- Rellenar centro_costo_id desde la tabla producciones
-- Si la produccion_id existe en producciones, tomar su centro_costo_id
UPDATE s3_monitoring_producciones smp
JOIN producciones p ON smp.produccion_id = p.produccion_id
SET smp.centro_costo_id = p.centro_costo_id
WHERE smp.centro_costo_id IS NULL;

-- Rellenar nombre_rancho desde la tabla centros_costos
-- Si centro_costo_id está poblado, tomar el nombre del rancho desde centros_costos
UPDATE s3_monitoring_producciones smp
JOIN centros_costos cc ON smp.centro_costo_id = cc.centro_costo_id
SET smp.nombre_rancho = cc.nombre
WHERE smp.nombre_rancho IS NULL AND smp.centro_costo_id IS NOT NULL;

-- Alternativa: Si no existe en producciones pero sí hay datos parciales
-- Actualizar nombre_rancho desde centros_costos sin requerir articulo_id
UPDATE s3_monitoring_producciones smp
JOIN centros_costos cc ON smp.centro_costo_id = cc.centro_costo_id
SET smp.nombre_rancho = cc.nombre
WHERE smp.nombre_rancho IS NULL;

-- =====================================================
-- PASO 5: VERIFICACIÓN DE DATOS
-- =====================================================
-- Mostrar la estructura actualizada de la tabla
DESCRIBE s3_monitoring_producciones;

-- Verificar registros con los nuevos campos populados
SELECT
    smp.id,
    smp.produccion_id,
    smp.articulo_id,
    a.nombre as articulo_nombre,
    smp.centro_costo_id,
    smp.nombre_rancho,
    smp.cultivo,
    smp.ciclo,
    smp.monitoring,
    smp.created_at
FROM s3_monitoring_producciones smp
LEFT JOIN articulos a ON smp.articulo_id = a.articulo_id
ORDER BY smp.id DESC
LIMIT 10;

-- Estadísticas de datos
SELECT
    COUNT(*) as total_registros,
    COUNT(DISTINCT produccion_id) as producciones_unicas,
    COUNT(DISTINCT articulo_id) as articulos_diferentes,
    COUNT(DISTINCT centro_costo_id) as centros_costo_diferentes,
    SUM(CASE WHEN articulo_id IS NOT NULL THEN 1 ELSE 0 END) as registros_con_articulo,
    SUM(CASE WHEN centro_costo_id IS NOT NULL THEN 1 ELSE 0 END) as registros_con_centro_costo,
    SUM(CASE WHEN nombre_rancho IS NOT NULL THEN 1 ELSE 0 END) as registros_con_nombre_rancho
FROM s3_monitoring_producciones;

-- Verificar integridad de FKs: registros que NO pueden ser joinados
SELECT
    smp.id,
    smp.articulo_id,
    smp.centro_costo_id,
    smp.nombre_rancho
FROM s3_monitoring_producciones smp
LEFT JOIN articulos a ON smp.articulo_id = a.articulo_id
LEFT JOIN centros_costos cc ON smp.centro_costo_id = cc.centro_costo_id
WHERE (smp.articulo_id IS NOT NULL AND a.articulo_id IS NULL)
   OR (smp.centro_costo_id IS NOT NULL AND cc.centro_costo_id IS NULL);

-- =====================================================
-- FIN FASE 2
-- =====================================================
