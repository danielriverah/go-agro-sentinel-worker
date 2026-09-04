-- =====================================================
-- SCRIPT: Importación de datos desde archivos CSV
-- Sistema Agro Sentinel
-- =====================================================
-- Descripción: Este script importa datos de archivos CSV
-- a las tablas de la base de datos usando LOAD DATA INFILE
--
-- NOTAS IMPORTANTES:
-- 1. Asegúrese de que los archivos CSV existen en el sistema de archivos
-- 2. MySQL necesita permisos para leer archivos del disco
-- 3. Los archivos deben estar en la ruta especificada
-- 4. El usuario MySQL debe tener permisos FILE
-- 5. Ejecute BEFORE de importar: SET GLOBAL local_infile=1;
-- =====================================================

USE agro;

-- =====================================================
-- PASO 1: Habilitar importación desde archivos locales
-- =====================================================
SET GLOBAL local_infile = 1;

-- =====================================================
-- PASO 2: Validación previa - Verificar que las tablas existan
-- =====================================================
SELECT 'Verificando tablas existentes...' as status;

-- Ver cantidad de registros antes de la importación
SELECT 'articulos' as tabla, COUNT(*) as registros_antes FROM articulos
UNION ALL
SELECT 'centros_costos', COUNT(*) FROM centros_costos
UNION ALL
SELECT 'producciones', COUNT(*) FROM producciones
UNION ALL
SELECT 'zonas_producciones', COUNT(*) FROM zonas_producciones
UNION ALL
SELECT 'asignaciones_zonas_producciones', COUNT(*) FROM asignaciones_zonas_producciones;

-- =====================================================
-- PASO 3: Importar tabla ARTICULOS
-- =====================================================
SELECT 'Importando articulos.csv...' as status;

LOAD DATA LOCAL INFILE 'data/csv/examples/articulos.csv'
INTO TABLE articulos
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(articulo_id, nombre, variedad);

-- Validación de importación
SELECT CONCAT('Articulos importados: ', COUNT(*)) as resultado FROM articulos;

-- =====================================================
-- PASO 4: Importar tabla CENTROS_COSTOS
-- =====================================================
SELECT 'Importando centros_costos.csv...' as status;

LOAD DATA LOCAL INFILE 'data/csv/examples/centros_costos.csv'
INTO TABLE centros_costos
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(centro_costo_id, nombre);

-- Validación de importación
SELECT CONCAT('Centros de costo importados: ', COUNT(*)) as resultado FROM centros_costos;

-- =====================================================
-- PASO 5: Importar tabla PRODUCCIONES
-- =====================================================
SELECT 'Importando producciones.csv...' as status;

LOAD DATA LOCAL INFILE 'data/csv/examples/producciones.csv'
INTO TABLE producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(folio, articulo_id, fecha, hora, fecha_cierre, hora_cierre, usuario, estatus, aplicado, cantidad, centro_costo_id, monitoring);

-- Validación de importación
SELECT CONCAT('Producciones importadas: ', COUNT(*)) as resultado FROM producciones;

-- =====================================================
-- PASO 6: Importar tabla ZONAS_PRODUCCIONES
-- =====================================================
SELECT 'Importando zonas_producciones.csv...' as status;

LOAD DATA LOCAL INFILE 'data/csv/examples/zonas_producciones.csv'
INTO TABLE zonas_producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(zona_produccion_id, nombre, nombre_corto, estatus, area, centro_costo_id, poligono);

-- Validación de importación
SELECT CONCAT('Zonas de producción importadas: ', COUNT(*)) as resultado FROM zonas_producciones;

-- =====================================================
-- PASO 7: Importar tabla ASIGNACIONES_ZONAS_PRODUCCIONES
-- =====================================================
SELECT 'Importando asignaciones_zonas_producciones.csv...' as status;

LOAD DATA LOCAL INFILE 'data/csv/examples/asignaciones_zonas_producciones.csv'
INTO TABLE asignaciones_zonas_producciones
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(asignacion_zona_prod_id, produccion_id, zona_produccion_id, tipo_asignacion, area, poligono);

-- Validación de importación
SELECT CONCAT('Asignaciones de zonas importadas: ', COUNT(*)) as resultado FROM asignaciones_zonas_producciones;

-- =====================================================
-- PASO 8: Importar tabla S3_MONITORING_ESCENA_IA_RESUMEN
-- =====================================================
SELECT 'Importando s3_monitoring_escena_ia_resumen.csv...' as status;

LOAD DATA LOCAL INFILE 'data/csv/examples/s3_monitoring_escena_ia_resumen.csv'
INTO TABLE s3_monitoring_escena_ia_resumen
FIELDS TERMINATED BY ','
ENCLOSED BY '"'
LINES TERMINATED BY '\n'
IGNORE 1 ROWS
(s3_monitoring_escena_id, estado_clave, estado_general, riesgo_nivel, riesgo_motivo, fecha_analisis, json_original);

-- Validación de importación
SELECT CONCAT('Registros de monitoreo importados: ', COUNT(*)) as resultado FROM s3_monitoring_escena_ia_resumen;

-- =====================================================
-- PASO 9: Resumen final
-- =====================================================
SELECT '=== RESUMEN DE IMPORTACIÓN ===' as resultado;

SELECT 'articulos' as tabla, COUNT(*) as registros_totales FROM articulos
UNION ALL
SELECT 'centros_costos', COUNT(*) FROM centros_costos
UNION ALL
SELECT 'producciones', COUNT(*) FROM producciones
UNION ALL
SELECT 'zonas_producciones', COUNT(*) FROM zonas_producciones
UNION ALL
SELECT 'asignaciones_zonas_producciones', COUNT(*) FROM asignaciones_zonas_producciones
UNION ALL
SELECT 's3_monitoring_escena_ia_resumen', COUNT(*) FROM s3_monitoring_escena_ia_resumen;

-- =====================================================
-- Verificación de integridad referencial
-- =====================================================
SELECT '=== VERIFICACIÓN DE INTEGRIDAD ===' as resultado;

-- Verificar artículos referenciados en producciones
SELECT 'Artículos sin producción asignada:' as verificacion;
SELECT a.articulo_id, a.nombre
FROM articulos a
LEFT JOIN producciones p ON a.articulo_id = p.articulo_id
WHERE p.produccion_id IS NULL;

-- Verificar centros de costo referenciados
SELECT 'Centros de costo sin producciones:' as verificacion;
SELECT cc.centro_costo_id, cc.nombre
FROM centros_costos cc
LEFT JOIN producciones p ON cc.centro_costo_id = p.centro_costo_id
WHERE p.produccion_id IS NULL;

-- Verificar zonas con asignaciones
SELECT 'Zonas sin asignaciones:' as verificacion;
SELECT z.zona_produccion_id, z.nombre
FROM zonas_producciones z
LEFT JOIN asignaciones_zonas_producciones az ON z.zona_produccion_id = az.zona_produccion_id
WHERE az.asignacion_zona_prod_id IS NULL;

SELECT 'Importación completada exitosamente!' as estado_final;

-- =====================================================
-- FIN DEL SCRIPT DE IMPORTACIÓN
-- =====================================================
