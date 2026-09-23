package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"agro-sentinel-worker/internal/domain"
)

// AlertaRepo da acceso a monitoring_alertas.
type AlertaRepo struct {
	db     *sql.DB
	exists bool
}

// NewAlertaRepo crea el repo y detecta una sola vez si la tabla existe.
func NewAlertaRepo(db *sql.DB) *AlertaRepo {
	return &AlertaRepo{db: db, exists: tableExists(db, "monitoring_alertas")}
}

// Disponible indica si la tabla está creada.
func (r *AlertaRepo) Disponible() bool { return r.exists }

// Crear inserta una alerta y devuelve su ID.
//
// Que falle no debe tumbar el análisis que la originó: quien llama registra
// el error y sigue. Perder un aviso es malo, perder el análisis es peor.
func (r *AlertaRepo) Crear(ctx context.Context, a *domain.Alerta) (uint64, error) {
	if !r.exists {
		return 0, fmt.Errorf("la tabla monitoring_alertas no existe")
	}

	const q = `
INSERT INTO monitoring_alertas (
	produccion_id, scene_name, scene_date, alert_type, severity, estado,
	title, message, action_suggested, source, source_json, notify_email
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	out, err := r.db.ExecContext(ctx, q,
		a.ProduccionID, nullString(a.SceneName), nullTimeVal(a.SceneDate),
		a.AlertType, a.Severity, a.Estado,
		a.Title, a.Message, nullString(a.ActionSuggested),
		a.Source, nullJSON(a.SourceJSON), boolToTinyint(a.NotifyEmail),
	)
	if err != nil {
		return 0, fmt.Errorf("insertando alerta: %w", err)
	}

	id, err := out.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("obteniendo id de alerta: %w", err)
	}
	return uint64(id), nil
}

// FiltroAlertas acota el listado.
type FiltroAlertas struct {
	// Estados a incluir. Vacío = todos.
	Estados []string
	// Severidades a incluir. Vacío = todas.
	Severidades []string
	// ProduccionID > 0 limita a una producción.
	ProduccionID int64
	Limite       int
}

const alertaSelectCols = `
SELECT a.monitoring_alerta_id, a.produccion_id, a.scene_name, a.scene_date,
       a.alert_type, a.severity, a.estado, a.title, a.message,
       a.action_suggested, a.source, a.notify_email,
       a.seen_at, a.seen_by, a.resolved_at, a.resolved_by,
       a.created_at, a.updated_at,
       p.folio, ar.nombre AS cultivo, cc.nombre AS rancho
FROM monitoring_alertas a
LEFT JOIN producciones   p  ON p.produccion_id   = a.produccion_id
LEFT JOIN articulos      ar ON ar.articulo_id    = p.articulo_id
LEFT JOIN centros_costos cc ON cc.centro_costo_id = p.centro_costo_id`

// List devuelve las alertas que cumplen el filtro, más recientes primero.
//
// No trae source_json: en un listado multiplicaría el tamaño de la respuesta
// por un dato que sólo se mira al abrir una alerta concreta.
func (r *AlertaRepo) List(ctx context.Context, f FiltroAlertas) ([]*domain.Alerta, error) {
	if !r.exists {
		return nil, nil
	}

	var where []string
	var args []any

	if len(f.Estados) > 0 {
		where = append(where, "a.estado IN ("+placeholders(len(f.Estados))+")")
		for _, e := range f.Estados {
			args = append(args, e)
		}
	}
	if len(f.Severidades) > 0 {
		where = append(where, "a.severity IN ("+placeholders(len(f.Severidades))+")")
		for _, s := range f.Severidades {
			args = append(args, s)
		}
	}
	if f.ProduccionID > 0 {
		where = append(where, "a.produccion_id = ?")
		args = append(args, f.ProduccionID)
	}

	q := alertaSelectCols
	if len(where) > 0 {
		q += "\nWHERE " + strings.Join(where, " AND ")
	}
	q += "\nORDER BY a.created_at DESC"

	limite := f.Limite
	if limite <= 0 || limite > 1000 {
		limite = 500
	}
	q += "\nLIMIT ?"
	args = append(args, limite)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("listando alertas: %w", err)
	}
	defer rows.Close()

	var out []*domain.Alerta
	for rows.Next() {
		a, err := scanAlerta(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// MarcarVista pasa la alerta a "vista" y registra quién y cuándo.
// Idempotente: volver a marcarla no cambia el primer registro.
func (r *AlertaRepo) MarcarVista(ctx context.Context, id uint64, usuario string) error {
	if !r.exists {
		return fmt.Errorf("la tabla monitoring_alertas no existe")
	}

	const q = `
UPDATE monitoring_alertas
SET estado  = IF(estado = 'nueva', 'vista', estado),
    seen_at = COALESCE(seen_at, ?),
    seen_by = COALESCE(seen_by, ?)
WHERE monitoring_alerta_id = ?`

	if _, err := r.db.ExecContext(ctx, q, time.Now().UTC(), nullString(usuario), id); err != nil {
		return fmt.Errorf("marcando alerta vista: %w", err)
	}
	return nil
}

// MarcarResuelta cierra la alerta. Resolver implica haberla visto, así que
// completa también seen_at si estaba vacío.
func (r *AlertaRepo) MarcarResuelta(ctx context.Context, id uint64, usuario string) error {
	if !r.exists {
		return fmt.Errorf("la tabla monitoring_alertas no existe")
	}

	const q = `
UPDATE monitoring_alertas
SET estado      = 'resuelta',
    resolved_at = ?,
    resolved_by = ?,
    seen_at     = COALESCE(seen_at, ?),
    seen_by     = COALESCE(seen_by, ?)
WHERE monitoring_alerta_id = ?`

	now := time.Now().UTC()
	if _, err := r.db.ExecContext(ctx, q, now, nullString(usuario), now, nullString(usuario), id); err != nil {
		return fmt.Errorf("resolviendo alerta: %w", err)
	}
	return nil
}

func scanAlerta(rs rowScanner) (*domain.Alerta, error) {
	var a domain.Alerta
	var sceneName, accion, seenBy, resolvedBy, folio, cultivo, rancho sql.NullString
	var sceneDate, seenAt, resolvedAt, updatedAt sql.NullTime
	var notifyEmail int

	err := rs.Scan(
		&a.ID, &a.ProduccionID, &sceneName, &sceneDate,
		&a.AlertType, &a.Severity, &a.Estado, &a.Title, &a.Message,
		&accion, &a.Source, &notifyEmail,
		&seenAt, &seenBy, &resolvedAt, &resolvedBy,
		&a.CreatedAt, &updatedAt,
		&folio, &cultivo, &rancho,
	)
	if err != nil {
		return nil, fmt.Errorf("escaneando alerta: %w", err)
	}

	a.SceneName = sceneName.String
	a.ActionSuggested = accion.String
	a.SeenBy = seenBy.String
	a.ResolvedBy = resolvedBy.String
	a.Folio = folio.String
	a.Cultivo = cultivo.String
	a.Rancho = rancho.String
	a.NotifyEmail = notifyEmail != 0

	if sceneDate.Valid {
		a.SceneDate = &sceneDate.Time
	}
	if seenAt.Valid {
		a.SeenAt = &seenAt.Time
	}
	if resolvedAt.Valid {
		a.ResolvedAt = &resolvedAt.Time
	}
	if updatedAt.Valid {
		a.UpdatedAt = &updatedAt.Time
	}

	return &a, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
