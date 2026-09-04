-- =====================================================
-- FASE 2 ROLLBACK: REVERTIR CAMBIOS A s3_monitoring_producciones
-- Sistema de Monitoreo Agro Sentinel
-- Script para deshacer los cambios realizados en Phase 2
-- =====================================================

USE agro;

-- =====================================================
-- PASO 1: ELIMINAR FOREIGN KEYS CONSTRAINTS
-- =====================================================
-- Eliminar FK hacia articulos
ALTER TABLE s3_monitoring_producciones
DROP FOREIGN KEY fk_s3mon_prod_articulo_id;

-- Eliminar FK hacia centros_costos
ALTER TABLE s3_monitoring_producciones
DROP FOREIGN KEY fk_s3mon_prod_centro_costo_id;

-- =====================================================
-- PASO 2: ELIMINAR ÍNDICES
-- =====================================================
-- Eliminar índice para articulo_id
DROP INDEX idx_s3mon_prod_articulo_id ON s3_monitoring_producciones;

-- Eliminar índice para centro_costo_id
DROP INDEX idx_s3mon_prod_centro_costo_id ON s3_monitoring_producciones;

-- Eliminar índice compuesto
DROP INDEX idx_s3mon_prod_centro_monitoring ON s3_monitoring_producciones;

-- =====================================================
-- PASO 3: ELIMINAR COLUMNAS AGREGADAS
-- =====================================================
-- Eliminar nombre_rancho (varchar(200))
ALTER TABLE s3_monitoring_producciones DROP COLUMN nombre_rancho;

-- Eliminar centro_costo_id (int)
ALTER TABLE s3_monitoring_producciones DROP COLUMN centro_costo_id;

-- Eliminar articulo_id (int)
ALTER TABLE s3_monitoring_producciones DROP COLUMN articulo_id;

-- =====================================================
-- PASO 4: VERIFICACIÓN POST-ROLLBACK
-- =====================================================
-- Mostrar la estructura de la tabla después del rollback
DESCRIBE s3_monitoring_producciones;

-- Verificar que la tabla está en su estado original
SELECT
    COLUMN_NAME,
    COLUMN_TYPE,
    IS_NULLABLE,
    COLUMN_KEY,
    COLUMN_DEFAULT,
    EXTRA,
    COLUMN_COMMENT
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = 'agro'
  AND TABLE_NAME = 's3_monitoring_producciones'
ORDER BY ORDINAL_POSITION;

-- =====================================================
-- FIN FASE 2 ROLLBACK
-- =====================================================
