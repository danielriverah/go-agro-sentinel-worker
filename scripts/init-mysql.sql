-- =====================================================
-- USUARIOS Y PERMISOS
-- =====================================================
CREATE USER IF NOT EXISTS 'admin'@'%' IDENTIFIED BY 'admin';
GRANT ALL PRIVILEGES ON *.* TO 'admin'@'%' WITH GRANT OPTION;
FLUSH PRIVILEGES;

-- =====================================================
-- FASE 1: CREAR TABLAS MYSQL
-- Sistema de Monitoreo Agro Sentinel
-- =====================================================

USE agro;

-- =====================================================
-- 1. TABLA: articulos
-- Cultivos/Artículos que se producen
-- =====================================================
CREATE TABLE IF NOT EXISTS `articulos` (
  `articulo_id` int NOT NULL AUTO_INCREMENT,
  `nombre` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `variedad` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `fecha_creacion` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `fecha_actualizacion` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`articulo_id`),
  UNIQUE KEY `articulo_id_UNIQUE` (`articulo_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- =====================================================
-- 2. TABLA: centros_costos
-- Ranchos/Centros de Costo
-- =====================================================
CREATE TABLE IF NOT EXISTS `centros_costos` (
  `centro_costo_id` int NOT NULL AUTO_INCREMENT,
  `nombre` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `fecha_creacion` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `fecha_actualizacion` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`centro_costo_id`),
  UNIQUE KEY `centro_costo_id_UNIQUE` (`centro_costo_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- =====================================================
-- 3. TABLA: producciones
-- Registro de producciones realizadas
-- =====================================================
CREATE TABLE IF NOT EXISTS `producciones` (
  `produccion_id` int NOT NULL AUTO_INCREMENT,
  `folio` varchar(45) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `articulo_id` int NOT NULL,
  `fecha` date NOT NULL,
  `hora` time NOT NULL,
  `fecha_cierre` date DEFAULT NULL,
  `hora_cierre` time DEFAULT NULL,
  `usuario` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT NULL,
  `estatus` char(1) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT 'N' COMMENT 'N=Normal(abierta), T=Terminada/cerrada, C=Cancelada',
  `aplicado` char(1) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT 'S' COMMENT 'S=Sí, N=No',
  `cantidad` decimal(20,6) DEFAULT NULL COMMENT 'Área de la producción en hectáreas',
  `centro_costo_id` int NOT NULL,
  `monitoring` tinyint NOT NULL DEFAULT '0' COMMENT '0=No monitorear, 1=Monitorear',
  `fecha_creacion` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `fecha_actualizacion` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`produccion_id`),
  UNIQUE KEY `produccion_id_UNIQUE` (`produccion_id`),
  UNIQUE KEY `folio_UNIQUE` (`folio`),
  KEY `articulo_producido_idx` (`articulo_id`),
  KEY `produccion_centrocosto_idx` (`centro_costo_id`),
  KEY `estatus_idx` (`estatus`),
  KEY `monitoring_idx` (`monitoring`),
  CONSTRAINT `articulo_producido` FOREIGN KEY (`articulo_id`) REFERENCES `articulos` (`articulo_id`) ON DELETE RESTRICT ON UPDATE CASCADE,
  CONSTRAINT `produccion_centrocosto` FOREIGN KEY (`centro_costo_id`) REFERENCES `centros_costos` (`centro_costo_id`) ON DELETE RESTRICT ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- =====================================================
-- 4. TABLA: zonas_producciones
-- Zonas dentro de ranchos/centros de costo
-- =====================================================
CREATE TABLE IF NOT EXISTS `zonas_producciones` (
  `zona_produccion_id` int NOT NULL AUTO_INCREMENT,
  `nombre` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `nombre_corto` varchar(200) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL,
  `estatus` char(1) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci NOT NULL DEFAULT 'P' COMMENT 'P=Producción, I=Inactiva',
  `area` decimal(20,6) DEFAULT '0.000000' COMMENT 'Área de la zona en hectáreas',
  `centro_costo_id` int DEFAULT NULL,
  `poligono` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci COMMENT 'GeoJSON del polígono de la zona',
  `fecha_creacion` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `fecha_actualizacion` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`zona_produccion_id`),
  UNIQUE KEY `zona_produccion_id_UNIQUE` (`zona_produccion_id`),
  KEY `zonaprodu_centrocosto_idx` (`centro_costo_id`),
  KEY `estatus_idx` (`estatus`),
  CONSTRAINT `zonaprodu_centrocosto` FOREIGN KEY (`centro_costo_id`) REFERENCES `centros_costos` (`centro_costo_id`) ON DELETE SET NULL ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- =====================================================
-- 5. TABLA: asignaciones_zonas_producciones
-- Asignación de zonas a producciones
-- Tabla pivote: producciones (N) ←→ (N) zonas_producciones
-- =====================================================
CREATE TABLE IF NOT EXISTS `asignaciones_zonas_producciones` (
  `asignacion_zona_prod_id` int NOT NULL AUTO_INCREMENT,
  `produccion_id` int DEFAULT NULL,
  `zona_produccion_id` int DEFAULT NULL,
  `tipo_asignacion` char(1) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci DEFAULT 'T' COMMENT 'T=Total, P=Parcial',
  `area` decimal(20,6) DEFAULT NULL COMMENT 'Área asignada de la zona a la producción',
  `poligono` mediumtext CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci COMMENT 'GeoJSON del polígono específico asignado',
  `fecha_creacion` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `fecha_actualizacion` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`asignacion_zona_prod_id`),
  UNIQUE KEY `asginacion_zona_prod_id_UNIQUE` (`asignacion_zona_prod_id`),
  KEY `asigzonaprod_produc_idx` (`produccion_id`),
  KEY `asigzonaprod_zonaprod_idx` (`zona_produccion_id`),
  CONSTRAINT `asigzonaprod_produc` FOREIGN KEY (`produccion_id`) REFERENCES `producciones` (`produccion_id`) ON DELETE CASCADE ON UPDATE CASCADE,
  CONSTRAINT `asigzonaprod_zonaprod` FOREIGN KEY (`zona_produccion_id`) REFERENCES `zonas_producciones` (`zona_produccion_id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- =====================================================
-- 6. TABLA: s3_monitoring_escena_ia_resumen
-- Resumen de análisis IA por escena
-- =====================================================
CREATE TABLE IF NOT EXISTS `s3_monitoring_escena_ia_resumen` (
  `s3_monitoring_escena_ia_resumen_id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `s3_monitoring_escena_id` bigint unsigned NOT NULL,
  `estado_clave` varchar(100) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT 'crítico, alerta, normal, etc.',
  `estado_general` varchar(100) COLLATE utf8mb4_general_ci DEFAULT NULL,
  `riesgo_nivel` varchar(100) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT 'alto, medio, bajo',
  `riesgo_motivo` text COLLATE utf8mb4_general_ci COMMENT 'Razón del riesgo detectado',
  `fecha_analisis` datetime DEFAULT NULL COMMENT 'Cuándo se hizo el análisis IA',
  `json_original` longtext COLLATE utf8mb4_general_ci COMMENT 'JSON completo de la respuesta de IA',
  `fecha_creacion` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `fecha_actualizacion` timestamp NULL DEFAULT NULL ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`s3_monitoring_escena_ia_resumen_id`),
  UNIQUE KEY `s3_monitoring_escena_id` (`s3_monitoring_escena_id`),
  KEY `estado_clave` (`estado_clave`),
  KEY `riesgo_nivel` (`riesgo_nivel`),
  KEY `fecha_analisis` (`fecha_analisis`),
  CONSTRAINT `s3_monitoring_escena_ia_resumen_ibfk_1` FOREIGN KEY (`s3_monitoring_escena_id`) REFERENCES `s3_monitoring_escenas` (`s3_monitoring_escena_id`) ON DELETE CASCADE ON UPDATE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- =====================================================
-- INSERTAR DATOS DE PRUEBA
-- =====================================================

-- Artículos (Cultivos)
INSERT INTO `articulos` (`nombre`, `variedad`) VALUES
('Lechuga', 'Latina'),
('Tomate', 'Cherry'),
('Pepino', 'Tipo Japonés'),
('Cilantro', 'Común'),
('Espinaca', 'Gigante de Holanda');

-- Centros de Costo (Ranchos)
INSERT INTO `centros_costos` (`nombre`) VALUES
('Rancho Los Andes'),
('Rancho San Miguel'),
('Rancho El Peñol'),
('Rancho La Esperanza'),
('Rancho Santa Rosa');

-- Producciones
INSERT INTO `producciones` (`folio`, `articulo_id`, `fecha`, `hora`, `usuario`, `estatus`, `aplicado`, `cantidad`, `centro_costo_id`, `monitoring`)
VALUES
('CSJ2601-17-A', 1, '2026-06-03', '08:30:00', 'admin', 'N', 'S', 2.50, 1, 1),
('CSJ2602-24', 2, '2026-06-10', '09:00:00', 'admin', 'N', 'S', 3.75, 2, 1),
('CSJ2603-31', 3, '2026-06-15', '07:45:00', 'admin', 'N', 'S', 4.20, 3, 1),
('CSJ2604-45', 4, '2026-07-01', '08:15:00', 'admin', 'N', 'S', 1.80, 1, 0),
('CSJ2605-52', 5, '2026-07-05', '09:30:00', 'admin', 'N', 'S', 2.10, 4, 1);

-- Zonas de Producción
INSERT INTO `zonas_producciones` (`nombre`, `nombre_corto`, `estatus`, `area`, `centro_costo_id`, `poligono`)
VALUES
('Zona Norte Los Andes', 'ZNA', 'P', 5.50, 1, '[[21.10535, -100.93089], [21.10535, -100.92730], [21.1068, -100.92730], [21.1068, -100.93089]]'),
('Zona Sur Los Andes', 'ZSA', 'P', 4.20, 1, '[[21.10200, -100.93000], [21.10200, -100.92500], [21.10500, -100.92500], [21.10500, -100.93000]]'),
('Zona Este San Miguel', 'ZES', 'P', 6.00, 2, '[[21.11000, -100.94000], [21.11000, -100.93500], [21.11300, -100.93500], [21.11300, -100.94000]]'),
('Zona Central El Peñol', 'ZCP', 'P', 5.80, 3, '[[21.10800, -100.92000], [21.10800, -100.91500], [21.11100, -100.91500], [21.11100, -100.92000]]');

-- Asignaciones de Zonas a Producciones
INSERT INTO `asignaciones_zonas_producciones` (`produccion_id`, `zona_produccion_id`, `tipo_asignacion`, `area`, `poligono`)
VALUES
(1, 1, 'P', 2.50, '[[21.10535, -100.93089], [21.10535, -100.92730], [21.1068, -100.92730], [21.1068, -100.93089]]'),
(2, 3, 'P', 3.75, '[[21.11000, -100.94000], [21.11000, -100.93500], [21.11300, -100.93500], [21.11300, -100.94000]]'),
(3, 4, 'T', 4.20, '[[21.10800, -100.92000], [21.10800, -100.91500], [21.11100, -100.91500], [21.11100, -100.92000]]'),
(4, 2, 'P', 1.80, '[[21.10200, -100.93000], [21.10200, -100.92500], [21.10500, -100.92500], [21.10500, -100.93000]]'),
(5, 1, 'P', 2.10, '[[21.10580, -100.93050], [21.10580, -100.92800], [21.10650, -100.92800], [21.10650, -100.93050]]');
