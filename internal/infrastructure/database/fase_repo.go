package database

import (
	"context"
	"database/sql"
	"fmt"

	"agro-sentinel-worker/internal/domain"
)

// FaseRepo da acceso a produccion_fases_cultivo.
//
// La tabla la crea el DBA por separado, así que puede no existir todavía.
// Su ausencia no es un error: las lecturas devuelven una lista vacía y la
// gráfica de tendencias se dibuja sin franjas de fase.
type FaseRepo struct {
	db     *sql.DB
	exists bool
}

// NewFaseRepo crea el repo y detecta una sola vez si la tabla está disponible.
func NewFaseRepo(db *sql.DB) *FaseRepo {
	return &FaseRepo{db: db, exists: tableExists(db, "produccion_fases_cultivo")}
}

// Disponible indica si la tabla existe. Los manejadores la consultan para
// responder 503 en las escrituras en lugar de fingir que guardaron algo.
func (r *FaseRepo) Disponible() bool { return r.exists }

// ListByProduccion devuelve las fases de una producción del ERP, ordenadas
// para pintarlas de izquierda a derecha.
func (r *FaseRepo) ListByProduccion(ctx context.Context, produccionID int64) ([]*domain.FaseCultivo, error) {
	if !r.exists {
		return nil, nil
	}

	const q = `
SELECT id, produccion_id, nombre, dia_inicio, dia_fin, orden
FROM produccion_fases_cultivo
WHERE produccion_id = ?
ORDER BY orden ASC, dia_inicio ASC`

	rows, err := r.db.QueryContext(ctx, q, produccionID)
	if err != nil {
		return nil, fmt.Errorf("listando fases de cultivo: %w", err)
	}
	defer rows.Close()

	var out []*domain.FaseCultivo
	for rows.Next() {
		var f domain.FaseCultivo
		if err := rows.Scan(&f.ID, &f.ProduccionID, &f.Nombre, &f.DiaInicio, &f.DiaFin, &f.Orden); err != nil {
			return nil, fmt.Errorf("escaneando fase de cultivo: %w", err)
		}
		out = append(out, &f)
	}

	return out, rows.Err()
}

// PlantillaFases es una producción que ya tiene fases configuradas y puede
// servir de modelo para copiar.
type PlantillaFases struct {
	ProduccionID int64                 `json:"produccion_id"`
	Folio        string                `json:"folio"`
	Cultivo      string                `json:"cultivo"`
	Rancho       string                `json:"rancho"`
	Fases        []*domain.FaseCultivo `json:"fases"`
}

// ListPlantillas devuelve sólo las producciones que TIENEN fases, con las
// fases incluidas.
//
// Las trae en una consulta y agrupa en memoria: pedir las fases de cada
// producción por separado sería una consulta por fila. Devolverlas ya
// permite previsualizarlas antes de copiar sin más viajes al servidor.
func (r *FaseRepo) ListPlantillas(ctx context.Context) ([]*PlantillaFases, error) {
	if !r.exists {
		return nil, nil
	}

	const q = `
SELECT f.produccion_id, p.folio,
       COALESCE(ar.nombre, ''), COALESCE(cc.nombre, ''),
       f.id, f.nombre, f.dia_inicio, f.dia_fin, f.orden
FROM produccion_fases_cultivo f
JOIN producciones       p  ON p.produccion_id   = f.produccion_id
LEFT JOIN articulos     ar ON ar.articulo_id    = p.articulo_id
LEFT JOIN centros_costos cc ON cc.centro_costo_id = p.centro_costo_id
ORDER BY ar.nombre, p.folio, f.orden ASC, f.dia_inicio ASC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listando plantillas de fases: %w", err)
	}
	defer rows.Close()

	var out []*PlantillaFases
	porProduccion := make(map[int64]*PlantillaFases)

	for rows.Next() {
		var produccionID int64
		var folio, cultivo, rancho string
		var f domain.FaseCultivo

		if err := rows.Scan(&produccionID, &folio, &cultivo, &rancho,
			&f.ID, &f.Nombre, &f.DiaInicio, &f.DiaFin, &f.Orden); err != nil {
			return nil, fmt.Errorf("escaneando plantilla de fases: %w", err)
		}
		f.ProduccionID = produccionID

		p, ok := porProduccion[produccionID]
		if !ok {
			p = &PlantillaFases{
				ProduccionID: produccionID,
				Folio:        folio,
				Cultivo:      cultivo,
				Rancho:       rancho,
			}
			porProduccion[produccionID] = p
			// El orden del SELECT ya viene por cultivo y folio; conservarlo
			// aquí evita reordenar después.
			out = append(out, p)
		}
		p.Fases = append(p.Fases, &f)
	}

	return out, rows.Err()
}

// ReplaceForProduccion sustituye por completo el conjunto de fases de una
// producción dentro de una transacción.
//
// Reemplazar todo es más simple y más seguro que un CRUD por fila: el editor
// manda el conjunto final y no hay estados intermedios donde las fases se
// solapen o dejen huecos.
func (r *FaseRepo) ReplaceForProduccion(ctx context.Context, produccionID int64, fases []*domain.FaseCultivo) error {
	if !r.exists {
		return fmt.Errorf("la tabla produccion_fases_cultivo no existe todavía")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("abriendo transacción de fases: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM produccion_fases_cultivo WHERE produccion_id = ?`, produccionID); err != nil {
		return fmt.Errorf("borrando fases previas: %w", err)
	}

	const ins = `
INSERT INTO produccion_fases_cultivo (produccion_id, nombre, dia_inicio, dia_fin, orden)
VALUES (?, ?, ?, ?, ?)`

	for i, f := range fases {
		if _, err := tx.ExecContext(ctx, ins, produccionID, f.Nombre, f.DiaInicio, f.DiaFin, i); err != nil {
			return fmt.Errorf("insertando fase %q: %w", f.Nombre, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("confirmando fases: %w", err)
	}
	return nil
}
