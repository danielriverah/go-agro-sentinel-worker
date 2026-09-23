-- =====================================================
-- FASE 3 (A): COLUMNA image_bbox EN s3_monitoring_escenas
-- Sistema de Monitoreo Agro Sentinel
-- =====================================================
--
-- POR QUÉ
-- El tile_bbox vive en s3_monitoring_producciones, no en la escena. Cuando una
-- producción reutiliza el multiband.tif de otra, sus imágenes quedan con la
-- extensión geográfica de la producción ORIGEN, pero eso no se guarda: hoy se
-- resuelve siguiendo la cadena
--     escena -> multiband_ref_escena_id -> escena origen -> producción -> tile_bbox
-- Si se borra la producción origen, la cadena se rompe en el primer eslabón y
-- las imágenes de la otra producción quedan mal georreferenciadas en el mapa.
--
-- Esta columna desnormaliza esa extensión en la propia escena, de modo que cada
-- escena sea autosuficiente. Es requisito previo del borrado de monitoreo.
--
-- SEMÁNTICA
--   NULL       -> la escena usa el tile_bbox de su propia producción
--   con valor  -> tile_bbox de la escena origen del multiband reutilizado
-- =====================================================

USE agro;

-- =====================================================
-- PASO 1: AGREGAR LA COLUMNA
-- =====================================================
ALTER TABLE s3_monitoring_escenas
ADD COLUMN image_bbox JSON NULL
COMMENT 'Extensión real del raster. NULL = usa el tile_bbox de su propia producción; con valor = tile_bbox de la escena origen del multiband reutilizado'
AFTER multiband_ref_escena_id;

-- =====================================================
-- PASO 2: RELLENAR LAS ESCENAS YA PROCESADAS
-- =====================================================
-- Sólo afecta a las que reutilizaron multiband de otra producción: las demás
-- se quedan en NULL, que es justamente lo que significa "uso el mío".
UPDATE s3_monitoring_escenas e
JOIN s3_monitoring_escenas origen
  ON origen.s3_monitoring_escena_id = e.multiband_ref_escena_id
JOIN s3_monitoring_producciones po
  ON po.s3_monitoring_produccion_id = origen.s3_monitoring_produccion_id
SET e.image_bbox = po.tile_bbox
WHERE e.multiband_ref_escena_id IS NOT NULL
  AND e.image_bbox IS NULL
  AND po.tile_bbox IS NOT NULL;

-- =====================================================
-- PASO 3: VERIFICACIÓN
-- =====================================================
-- (a) Cuántas escenas dependen de otra y cuántas quedaron con bbox propio.
SELECT
  COUNT(*)                                             AS escenas_reutilizadas,
  SUM(image_bbox IS NOT NULL)                          AS con_image_bbox,
  SUM(image_bbox IS NULL)                              AS sin_image_bbox_revisar
FROM s3_monitoring_escenas
WHERE multiband_ref_escena_id IS NOT NULL;

-- Lo esperado es sin_image_bbox_revisar = 0. Si no lo es, son escenas cuya
-- producción origen ya no existe o no tiene tile_bbox: su georreferencia no se
-- puede recuperar sin reprocesarlas.

-- (b) Ninguna escena que NO reutiliza debería haber quedado con valor.
SELECT COUNT(*) AS no_reutilizadas_con_bbox_inesperado
FROM s3_monitoring_escenas
WHERE multiband_ref_escena_id IS NULL
  AND image_bbox IS NOT NULL;

-- =====================================================
-- ROLLBACK
-- =====================================================
-- ALTER TABLE s3_monitoring_escenas DROP COLUMN image_bbox;
