-- =====================================================
-- FASE 4: ALERTAS DE IA Y TRIGGER DE BLOQUEO
-- Sistema de Monitoreo Agro Sentinel
-- =====================================================
--
-- Ambos objetos YA EXISTEN en la base de producción. Se versionan aquí para
-- poder reproducir el entorno y porque el código depende de su comportamiento.
--
-- POR QUÉ IMPORTAN
-- 1. El trigger deriva `bloqueado` de `posible_cosecha`. Desde que existe, el
--    código NO debe escribir `bloqueado`: sólo escribe `posible_cosecha` y deja
--    que el trigger propague. Escribir ambos los desincronizaría, porque el
--    trigger únicamente reacciona a cambios de posible_cosecha.
-- 2. monitoring_alertas sustituye al bloqueo automático por estado crítico. Un
--    diagnóstico preocupante ahora avisa en vez de detener el monitoreo del
--    lote, que era justo cuando más falta hacía seguirlo.
-- =====================================================

USE agro;

-- =====================================================
-- TRIGGER: bloqueado se deriva de posible_cosecha
-- =====================================================
-- Fragmento del trigger BEFORE UPDATE de s3_monitoring_producciones:
--
--   IF new.posible_cosecha <> old.posible_cosecha THEN
--       SET new.bloqueado = new.posible_cosecha;
--   END IF;
--
-- Consecuencias para la aplicación:
--   - Desbloquear  -> UPDATE ... SET posible_cosecha = 0
--   - Bloquear     -> UPDATE ... SET posible_cosecha = 1
--   - No existe SetBloqueado en el código, a propósito.

-- =====================================================
-- TABLA: monitoring_alertas
-- =====================================================
CREATE TABLE IF NOT EXISTS `monitoring_alertas` (
  `monitoring_alerta_id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `produccion_id` int NOT NULL COMMENT 'FK lógica a producciones.produccion_id (ERP)',
  `scene_name` varchar(255) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `scene_date` date DEFAULT NULL,
  `alert_type` varchar(60) COLLATE utf8mb4_general_ci NOT NULL COMMENT 'ia_analysis',
  `severity` varchar(20) COLLATE utf8mb4_general_ci NOT NULL COMMENT 'baja | media | alta',
  `estado` varchar(20) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'nueva' COMMENT 'nueva | vista | resuelta',
  `title` varchar(200) COLLATE utf8mb4_general_ci NOT NULL,
  `message` text COLLATE utf8mb4_general_ci NOT NULL,
  `action_suggested` varchar(300) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `source` varchar(30) COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'ia',
  `source_json` json DEFAULT NULL COMMENT 'Análisis completo del modelo, para auditoría',
  `notify_email` tinyint NOT NULL DEFAULT '0',
  `email_to` varchar(255) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `emailed_at` datetime DEFAULT NULL,
  `seen_at` datetime DEFAULT NULL,
  `seen_by` varchar(120) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `resolved_at` datetime DEFAULT NULL,
  `resolved_by` varchar(120) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`monitoring_alerta_id`),
  KEY `idx_alert_prod` (`produccion_id`,`estado`),
  KEY `idx_alert_scene` (`scene_name`,`scene_date`),
  KEY `idx_alert_severity` (`severity`,`estado`),
  KEY `idx_alert_created` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- =====================================================
-- VERIFICACIÓN
-- =====================================================
-- Alertas pendientes por rancho y cultivo, como las agrupa la vista:
--   SELECT cc.nombre AS rancho, ar.nombre AS cultivo,
--          SUM(a.estado = 'nueva') AS sin_leer, COUNT(*) AS total
--   FROM monitoring_alertas a
--   JOIN producciones p        ON p.produccion_id    = a.produccion_id
--   LEFT JOIN articulos ar     ON ar.articulo_id     = p.articulo_id
--   LEFT JOIN centros_costos cc ON cc.centro_costo_id = p.centro_costo_id
--   WHERE a.estado <> 'resuelta'
--   GROUP BY cc.nombre, ar.nombre
--   ORDER BY sin_leer DESC;

-- Comprobar que el trigger propaga (sobre una producción de pruebas):
--   UPDATE s3_monitoring_producciones SET posible_cosecha = 1 WHERE produccion_id = <id>;
--   SELECT posible_cosecha, bloqueado FROM s3_monitoring_producciones WHERE produccion_id = <id>;
--   -- ambos deben valer 1
