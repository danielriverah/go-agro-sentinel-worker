-- =====================================================
-- FASE 3 (E): TABLA produccion_fases_cultivo
-- Sistema de Monitoreo Agro Sentinel
-- =====================================================
--
-- POR QUÉ
-- Las fases del ciclo (siembra, crecimiento, floración, madurez) no existen en
-- ningún lado del sistema. Sin ellas, la gráfica de tendencias pinta en rojo a
-- todo cultivo joven, porque los umbrales de vigor son fijos.
--
-- La tabla cuelga de producciones.produccion_id (la maestra del ERP), NO de
-- s3_monitoring_produccion_id, para que las fases SOBREVIVAN al borrado del
-- monitoreo y queden como base para predecir ciclos futuros.
--
-- NOTA: max_dias_monitoring NO sirve para esto. Es un plazo administrativo con
-- margen añadido (lechuga: 95 frente a un ciclo real de 60-75 días) y es igual
-- para todos los lotes del mismo cultivo.
-- =====================================================

USE agro;

-- =====================================================
-- PASO 1: CREAR LA TABLA
-- =====================================================
CREATE TABLE produccion_fases_cultivo (
  id             BIGINT AUTO_INCREMENT PRIMARY KEY,
  -- INT, no BIGINT: producciones.produccion_id es int y los tipos deben casar
  -- para que la FK sea aplicable.
  produccion_id  INT          NOT NULL COMMENT 'FK lógica a producciones.produccion_id (ERP)',
  nombre         VARCHAR(60)  CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL COMMENT 'Nombre visible de la fase (ej. Crecimiento)',
  dia_inicio     INT          NOT NULL COMMENT 'Día del cultivo en que empieza la fase, contado desde fecha_plantacion',
  dia_fin        INT          NOT NULL COMMENT 'Día del cultivo en que termina la fase',
  orden          INT          NOT NULL DEFAULT 0 COMMENT 'Orden de presentación de la fase',
  fecha_creacion DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_prod_fases_produccion (produccion_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci
COMMENT='Fases fenológicas por producción. Sobrevive al borrado del monitoreo: sirve de base para predicción de ciclos futuros.';

-- =====================================================
-- PASO 2: VERIFICACIÓN
-- =====================================================
SELECT COUNT(*) AS filas FROM produccion_fases_cultivo;

-- Consulta de referencia para el uso predictivo: fases agrupadas por cultivo.
-- El cultivo se obtiene por JOIN con el ERP (articulos.nombre).
--   SELECT a.nombre AS cultivo, f.nombre AS fase,
--          AVG(f.dia_inicio) AS inicio_medio, AVG(f.dia_fin) AS fin_medio, COUNT(*) AS n
--   FROM produccion_fases_cultivo f
--   JOIN producciones p ON p.produccion_id = f.produccion_id
--   JOIN articulos a    ON a.articulo_id   = p.articulo_id
--   GROUP BY a.nombre, f.nombre
--   ORDER BY a.nombre, inicio_medio;

-- =====================================================
-- ROLLBACK
-- =====================================================
-- DROP TABLE produccion_fases_cultivo;
