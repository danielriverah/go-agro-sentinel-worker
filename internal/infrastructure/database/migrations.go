package database

import (
	"database/sql"
	"fmt"
)

const createProduccionesTable = `
CREATE TABLE IF NOT EXISTS s3_monitoring_producciones (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	produccion_id BIGINT NOT NULL,
	cultivo VARCHAR(100),
	ciclo VARCHAR(50),
	bbox_minx DECIMAL(10,6) NULL,
	bbox_miny DECIMAL(10,6) NULL,
	bbox_maxx DECIMAL(10,6) NULL,
	bbox_maxy DECIMAL(10,6) NULL,
	monitoring TINYINT(1) DEFAULT 1,
	monitoring_motivo VARCHAR(100),
	bloqueado TINYINT(1) DEFAULT 0,
	bloqueado_motivo VARCHAR(255),
	bloqueado_at DATETIME,
	desbloqueado_por VARCHAR(100),
	target_resolution INT DEFAULT 10,
	cloud_cover_max DECIMAL(5,2) DEFAULT 23.00,
	fecha_plantacion DATE,
	dias_produccion INT,
	fecha_fin_monitoreo DATE,
	total_escenas INT DEFAULT 0,
	total_escenas_validas INT DEFAULT 0,
	last_sync_at DATETIME,
	created_at DATETIME,
	updated_at DATETIME,
	UNIQUE KEY uq_produccion_id (produccion_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`

const createEscenasTable = `
CREATE TABLE IF NOT EXISTS s3_monitoring_escenas (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	produccion_id BIGINT NOT NULL,
	scene_id VARCHAR(100) NOT NULL,
	scene_date DATE,
	cloud_cover_scene DECIMAL(5,2),
	cloud_cover_bbox DECIMAL(5,2),
	passes_quality TINYINT(1) DEFAULT 0,
	has_multiband TINYINT(1) DEFAULT 0,
	has_params TINYINT(1) DEFAULT 0,
	has_rgb TINYINT(1) DEFAULT 0,
	has_analisis TINYINT(1) DEFAULT 0,
	status VARCHAR(20) DEFAULT 'PENDING',
	error_type VARCHAR(50),
	error_message TEXT,
	retry_count INT DEFAULT 0,
	processed_at DATETIME,
	created_at DATETIME,
	updated_at DATETIME,
	UNIQUE KEY uq_produccion_scene (produccion_id, scene_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`

const createArchivosTable = `
CREATE TABLE IF NOT EXISTS s3_monitoring_escena_archivos (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	escena_id BIGINT NOT NULL,
	file_type VARCHAR(50),
	file_name VARCHAR(255),
	s3_key VARCHAR(500),
	s3_bucket VARCHAR(100),
	file_size_bytes BIGINT,
	resolution_m INT,
	width_px INT,
	height_px INT,
	created_at DATETIME,
	KEY idx_escena_id (escena_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`

// RunMigrations creates the required tables if they do not already exist.
func RunMigrations(db *sql.DB) error {
	statements := []string{
		createProduccionesTable,
		createEscenasTable,
		createArchivosTable,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("running migration: %w", err)
		}
	}

	return nil
}
