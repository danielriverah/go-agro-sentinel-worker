package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"

	"agro-sentinel-worker/internal/auth"
)

type PermissionRepo struct {
	db                         *sql.DB
	exists                     bool
	hasCentroCostoTipo         bool
	hasCentroCostoTipoCatalogo bool
	cache                      sync.Map // map[int64]*auth.PermisosUsuario
}

func NewPermissionRepo(db *sql.DB) *PermissionRepo {
	return &PermissionRepo{
		db:                         db,
		exists:                     tableExists(db, "auth_permisos"),
		hasCentroCostoTipo:         columnExists(db, "centros_costos", "tipo"),
		hasCentroCostoTipoCatalogo: columnExists(db, "centros_costos", "tipo_centro_costo_id") && tableExists(db, "tipos_centros_costos"),
	}
}

func (r *PermissionRepo) Disponible() bool { return r.exists }

// CargarPermisos queries the DB for all effective permissions of a user
// (from roles + direct assignments) and caches the result.
func (r *PermissionRepo) CargarPermisos(ctx context.Context, usuarioID int64) (*auth.PermisosUsuario, error) {
	if !r.exists {
		return nil, auth.ErrPermisosMissing
	}

	const q = `
SELECT p.clave, ur.centro_costo_id
FROM auth_usuario_roles ur
JOIN auth_rol_permisos rp ON ur.rol_id = rp.rol_id
JOIN auth_permisos p ON rp.permiso_id = p.permiso_id
WHERE ur.usuario_id = ?

UNION

SELECT p.clave, up.centro_costo_id
FROM auth_usuario_permisos up
JOIN auth_permisos p ON up.permiso_id = p.permiso_id
WHERE up.usuario_id = ?`

	rows, err := r.db.QueryContext(ctx, q, usuarioID, usuarioID)
	if err != nil {
		return nil, fmt.Errorf("loading permissions for user %d: %w", usuarioID, err)
	}
	defer rows.Close()

	perms := &auth.PermisosUsuario{UsuarioID: usuarioID}
	for rows.Next() {
		var clave string
		var ccID sql.NullInt64
		if err := rows.Scan(&clave, &ccID); err != nil {
			return nil, fmt.Errorf("scanning permission row: %w", err)
		}
		a := auth.AsignacionPermiso{Clave: clave}
		if ccID.Valid {
			v := ccID.Int64
			a.CentroCostoID = &v
		} else {
			perms.TodosRancho = true
		}
		perms.Permisos = append(perms.Permisos, a)
	}

	r.cache.Store(usuarioID, perms)
	return perms, nil
}

// ObtenerPermisosCacheados returns cached permissions or nil.
func (r *PermissionRepo) ObtenerPermisosCacheados(usuarioID int64) *auth.PermisosUsuario {
	if v, ok := r.cache.Load(usuarioID); ok {
		if p, ok := v.(*auth.PermisosUsuario); ok {
			return p
		}
	}
	return nil
}

// InvalidarCache removes cached permissions for a user (after role/perm change).
func (r *PermissionRepo) InvalidarCache(usuarioID int64) {
	r.cache.Delete(usuarioID)
}

// Bootstrap assigns the Superadmin role to the first active user if no
// assignments exist yet. Called once at API startup.
func (r *PermissionRepo) Bootstrap(ctx context.Context, log *slog.Logger) {
	if !r.exists {
		return
	}

	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM auth_usuario_roles").Scan(&count); err != nil {
		log.Error("bootstrap: could not count auth_usuario_roles", "error", err)
		return
	}
	if count > 0 {
		return
	}

	// Get the first active user via the stored function (auth convention).
	// Fall back to direct query if fn_list_usuarios doesn't exist.
	var firstUserID int64
	err := r.db.QueryRowContext(ctx,
		"SELECT id FROM agro_usuarios WHERE activo = 1 ORDER BY id ASC LIMIT 1",
	).Scan(&firstUserID)
	if err != nil {
		log.Error("bootstrap: no active user found in agro_usuarios", "error", err)
		return
	}

	var superadminRolID int
	err = r.db.QueryRowContext(ctx,
		"SELECT rol_id FROM auth_roles WHERE nombre = 'Superadmin'",
	).Scan(&superadminRolID)
	if err != nil {
		log.Error("bootstrap: Superadmin role not found", "error", err)
		return
	}

	_, err = r.db.ExecContext(ctx,
		"INSERT INTO auth_usuario_roles (usuario_id, rol_id, centro_costo_id, asignado_por) VALUES (?, ?, NULL, ?)",
		firstUserID, superadminRolID, firstUserID,
	)
	if err != nil {
		log.Error("bootstrap: failed to assign Superadmin", "error", err)
		return
	}

	log.Info("bootstrap: usuario asignado como Superadmin", "usuario_id", firstUserID)
}

// ListPermisos returns the full permission catalog.
func (r *PermissionRepo) ListPermisos(ctx context.Context) ([]PermisoCatalogo, error) {
	if !r.exists {
		return nil, auth.ErrPermisosMissing
	}
	const q = `SELECT permiso_id, clave, modulo, descripcion, scope FROM auth_permisos ORDER BY modulo, clave`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listing permisos: %w", err)
	}
	defer rows.Close()

	var out []PermisoCatalogo
	for rows.Next() {
		var p PermisoCatalogo
		if err := rows.Scan(&p.ID, &p.Clave, &p.Modulo, &p.Descripcion, &p.Scope); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

type PermisoCatalogo struct {
	ID          int    `json:"permiso_id"`
	Clave       string `json:"clave"`
	Modulo      string `json:"modulo"`
	Descripcion string `json:"descripcion"`
	Scope       string `json:"scope"`
}

// RolConPermisos is a role with its associated permission IDs.
type RolConPermisos struct {
	ID          int    `json:"rol_id"`
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion,omitempty"`
	EsSistema   bool   `json:"es_sistema"`
	PermisoIDs  []int  `json:"permiso_ids"`
}

// ListRoles returns all roles with their permission IDs.
func (r *PermissionRepo) ListRoles(ctx context.Context) ([]RolConPermisos, error) {
	if !r.exists {
		return nil, auth.ErrPermisosMissing
	}

	const q = `SELECT rol_id, nombre, COALESCE(descripcion,''), es_sistema FROM auth_roles ORDER BY rol_id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}
	defer rows.Close()

	var roles []RolConPermisos
	for rows.Next() {
		var ro RolConPermisos
		var esSistema int
		if err := rows.Scan(&ro.ID, &ro.Nombre, &ro.Descripcion, &esSistema); err != nil {
			return nil, err
		}
		ro.EsSistema = esSistema == 1
		roles = append(roles, ro)
	}

	// Load permission IDs for each role
	for i := range roles {
		const pq = `SELECT permiso_id FROM auth_rol_permisos WHERE rol_id = ? ORDER BY permiso_id`
		prows, err := r.db.QueryContext(ctx, pq, roles[i].ID)
		if err != nil {
			return nil, err
		}
		for prows.Next() {
			var pid int
			if err := prows.Scan(&pid); err != nil {
				prows.Close()
				return nil, err
			}
			roles[i].PermisoIDs = append(roles[i].PermisoIDs, pid)
		}
		prows.Close()
	}

	return roles, nil
}

// CreateRol creates a new role with the given permissions.
func (r *PermissionRepo) CreateRol(ctx context.Context, nombre, descripcion string, permisoIDs []int) (*RolConPermisos, error) {
	if !r.exists {
		return nil, auth.ErrPermisosMissing
	}

	res, err := r.db.ExecContext(ctx,
		"INSERT INTO auth_roles (nombre, descripcion) VALUES (?, ?)",
		nombre, descripcion,
	)
	if err != nil {
		return nil, fmt.Errorf("creating role: %w", err)
	}

	rolID64, _ := res.LastInsertId()
	rolID := int(rolID64)

	if err := r.setRolPermisos(ctx, rolID, permisoIDs); err != nil {
		return nil, err
	}

	return &RolConPermisos{
		ID:          rolID,
		Nombre:      nombre,
		Descripcion: descripcion,
		PermisoIDs:  permisoIDs,
	}, nil
}

// UpdateRol updates name, description, and permissions of a role.
func (r *PermissionRepo) UpdateRol(ctx context.Context, rolID int, nombre, descripcion string, permisoIDs []int) error {
	if !r.exists {
		return auth.ErrPermisosMissing
	}

	var esSistema int
	err := r.db.QueryRowContext(ctx, "SELECT es_sistema FROM auth_roles WHERE rol_id = ?", rolID).Scan(&esSistema)
	if err != nil {
		if err == sql.ErrNoRows {
			return auth.ErrRolNotFound
		}
		return err
	}

	_, err = r.db.ExecContext(ctx,
		"UPDATE auth_roles SET nombre = ?, descripcion = ? WHERE rol_id = ?",
		nombre, descripcion, rolID,
	)
	if err != nil {
		return fmt.Errorf("updating role: %w", err)
	}

	if err := r.setRolPermisos(ctx, rolID, permisoIDs); err != nil {
		return err
	}

	afectados, err := r.usuariosConRol(ctx, rolID)
	if err != nil {
		return fmt.Errorf("querying users affected by role update: %w", err)
	}
	for _, uid := range afectados {
		r.InvalidarCache(uid)
	}
	return nil
}

// DeleteRol deletes a role if it's not a system role.
func (r *PermissionRepo) DeleteRol(ctx context.Context, rolID int) error {
	if !r.exists {
		return auth.ErrPermisosMissing
	}

	var esSistema int
	err := r.db.QueryRowContext(ctx, "SELECT es_sistema FROM auth_roles WHERE rol_id = ?", rolID).Scan(&esSistema)
	if err != nil {
		if err == sql.ErrNoRows {
			return auth.ErrRolNotFound
		}
		return err
	}
	if esSistema == 1 {
		return auth.ErrRolEsSistema
	}

	// Collect affected users before removing the assignments, so we can
	// invalidate their cached permissions once the role is gone.
	afectados, err := r.usuariosConRol(ctx, rolID)
	if err != nil {
		return fmt.Errorf("querying users affected by role deletion: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx for delete rol: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_rol_permisos WHERE rol_id = ?", rolID); err != nil {
		return fmt.Errorf("deleting rol_permisos: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_usuario_roles WHERE rol_id = ?", rolID); err != nil {
		return fmt.Errorf("deleting usuario_roles: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_roles WHERE rol_id = ?", rolID); err != nil {
		return fmt.Errorf("deleting rol: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing delete rol: %w", err)
	}

	for _, uid := range afectados {
		r.InvalidarCache(uid)
	}
	return nil
}

// usuariosConRol returns the distinct users currently assigned to rolID,
// so callers can invalidate their cached permissions after a role mutation.
func (r *PermissionRepo) usuariosConRol(ctx context.Context, rolID int) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT usuario_id FROM auth_usuario_roles WHERE rol_id = ?", rolID)
	if err != nil {
		return nil, fmt.Errorf("querying affected users: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *PermissionRepo) setRolPermisos(ctx context.Context, rolID int, permisoIDs []int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx for rol_permisos: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_rol_permisos WHERE rol_id = ?", rolID); err != nil {
		return fmt.Errorf("clearing rol_permisos: %w", err)
	}
	for _, pid := range permisoIDs {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO auth_rol_permisos (rol_id, permiso_id) VALUES (?, ?)",
			rolID, pid,
		); err != nil {
			return fmt.Errorf("inserting rol_permiso: %w", err)
		}
	}
	return tx.Commit()
}

// AsignacionRolInput is a single role assignment with scope.
type AsignacionRolInput struct {
	RolID         int    `json:"rol_id"`
	CentroCostoID *int64 `json:"centro_costo_id"`
}

// SetUsuarioRoles replaces all role assignments for a user.
func (r *PermissionRepo) SetUsuarioRoles(ctx context.Context, usuarioID int64, asignaciones []AsignacionRolInput, asignadoPor int64) error {
	if !r.exists {
		return auth.ErrPermisosMissing
	}

	// Protect last Superadmin
	if err := r.protegerUltimoSuperadmin(ctx, usuarioID, asignaciones); err != nil {
		return err
	}

	defer r.InvalidarCache(usuarioID)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx for usuario roles: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_usuario_roles WHERE usuario_id = ?", usuarioID); err != nil {
		return fmt.Errorf("clearing user roles: %w", err)
	}

	for _, a := range asignaciones {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO auth_usuario_roles (usuario_id, rol_id, centro_costo_id, asignado_por) VALUES (?, ?, ?, ?)",
			usuarioID, a.RolID, a.CentroCostoID, asignadoPor,
		); err != nil {
			return fmt.Errorf("inserting user role: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing user roles: %w", err)
	}
	return nil
}

// AsignacionPermisoInput is a single direct permission assignment.
type AsignacionPermisoInput struct {
	PermisoID     int    `json:"permiso_id"`
	CentroCostoID *int64 `json:"centro_costo_id"`
}

// SetUsuarioPermisos replaces all direct permission assignments for a user.
func (r *PermissionRepo) SetUsuarioPermisos(ctx context.Context, usuarioID int64, asignaciones []AsignacionPermisoInput, asignadoPor int64) error {
	if !r.exists {
		return auth.ErrPermisosMissing
	}

	defer r.InvalidarCache(usuarioID)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning tx for usuario permisos: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_usuario_permisos WHERE usuario_id = ?", usuarioID); err != nil {
		return fmt.Errorf("clearing user direct perms: %w", err)
	}

	for _, a := range asignaciones {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO auth_usuario_permisos (usuario_id, permiso_id, centro_costo_id, asignado_por) VALUES (?, ?, ?, ?)",
			usuarioID, a.PermisoID, a.CentroCostoID, asignadoPor,
		); err != nil {
			return fmt.Errorf("inserting user direct perm: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing user direct perms: %w", err)
	}
	return nil
}

// GetUsuarioAsignaciones returns the current role and direct permission
// assignments for a user (for the admin edit UI, not evaluation).
type UsuarioAsignaciones struct {
	Roles    []AsignacionRolDetalle     `json:"roles"`
	Directos []AsignacionPermisoDetalle `json:"directos"`
}

type AsignacionRolDetalle struct {
	UsuarioRolID  int64  `json:"usuario_rol_id"`
	RolID         int    `json:"rol_id"`
	RolNombre     string `json:"rol_nombre"`
	CentroCostoID *int64 `json:"centro_costo_id"`
	RanchoNombre  string `json:"rancho_nombre,omitempty"`
}

type AsignacionPermisoDetalle struct {
	UsuarioPermisoID int64  `json:"usuario_permiso_id"`
	PermisoID        int    `json:"permiso_id"`
	PermisoClave     string `json:"permiso_clave"`
	CentroCostoID    *int64 `json:"centro_costo_id"`
	RanchoNombre     string `json:"rancho_nombre,omitempty"`
}

func (r *PermissionRepo) GetUsuarioAsignaciones(ctx context.Context, usuarioID int64) (*UsuarioAsignaciones, error) {
	if !r.exists {
		return nil, auth.ErrPermisosMissing
	}

	out := &UsuarioAsignaciones{
		Roles:    []AsignacionRolDetalle{},
		Directos: []AsignacionPermisoDetalle{},
	}

	// Roles
	const rq = `
SELECT ur.usuario_rol_id, ur.rol_id, ro.nombre, ur.centro_costo_id,
       COALESCE(cc.nombre, '')
FROM auth_usuario_roles ur
JOIN auth_roles ro ON ur.rol_id = ro.rol_id
LEFT JOIN centros_costos cc ON ur.centro_costo_id = cc.centro_costo_id
WHERE ur.usuario_id = ?
ORDER BY ro.nombre`

	rrows, err := r.db.QueryContext(ctx, rq, usuarioID)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()

	for rrows.Next() {
		var a AsignacionRolDetalle
		var ccID sql.NullInt64
		if err := rrows.Scan(&a.UsuarioRolID, &a.RolID, &a.RolNombre, &ccID, &a.RanchoNombre); err != nil {
			return nil, err
		}
		if ccID.Valid {
			v := ccID.Int64
			a.CentroCostoID = &v
		}
		out.Roles = append(out.Roles, a)
	}

	// Direct permissions
	const dq = `
SELECT up.usuario_permiso_id, up.permiso_id, p.clave, up.centro_costo_id,
       COALESCE(cc.nombre, '')
FROM auth_usuario_permisos up
JOIN auth_permisos p ON up.permiso_id = p.permiso_id
LEFT JOIN centros_costos cc ON up.centro_costo_id = cc.centro_costo_id
WHERE up.usuario_id = ?
ORDER BY p.clave`

	drows, err := r.db.QueryContext(ctx, dq, usuarioID)
	if err != nil {
		return nil, err
	}
	defer drows.Close()

	for drows.Next() {
		var a AsignacionPermisoDetalle
		var ccID sql.NullInt64
		if err := drows.Scan(&a.UsuarioPermisoID, &a.PermisoID, &a.PermisoClave, &ccID, &a.RanchoNombre); err != nil {
			return nil, err
		}
		if ccID.Valid {
			v := ccID.Int64
			a.CentroCostoID = &v
		}
		out.Directos = append(out.Directos, a)
	}

	return out, nil
}

// ListCentrosCostos returns all centros_costos for the ranch selector in the UI.
type CentroCostoItem struct {
	ID     int64  `json:"centro_costo_id"`
	Nombre string `json:"nombre"`
}

func (r *PermissionRepo) ListCentrosCostos(ctx context.Context) ([]CentroCostoItem, error) {
	q := centrosCostosSelectorQuery(r.hasCentroCostoTipo, r.hasCentroCostoTipoCatalogo)
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CentroCostoItem
	for rows.Next() {
		var c CentroCostoItem
		if err := rows.Scan(&c.ID, &c.Nombre); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func centrosCostosSelectorQuery(hasTipo bool, hasTipoCatalogo bool) string {
	if hasTipoCatalogo {
		return `SELECT cc.centro_costo_id, cc.nombre
FROM centros_costos cc
JOIN tipos_centros_costos tcc ON tcc.tipo_centro_costo_id = cc.tipo_centro_costo_id
WHERE UPPER(tcc.nombre) = 'RANCHOS'
ORDER BY cc.nombre`
	}
	if hasTipo {
		return `SELECT centro_costo_id, nombre FROM centros_costos WHERE LOWER(tipo) = 'rancho' ORDER BY nombre`
	}
	return `SELECT centro_costo_id, nombre FROM centros_costos ORDER BY nombre`
}

// protegerUltimoSuperadmin checks that we're not removing the last
// Superadmin from the system.
func (r *PermissionRepo) protegerUltimoSuperadmin(ctx context.Context, usuarioID int64, nuevas []AsignacionRolInput) error {
	var superadminRolID int
	err := r.db.QueryRowContext(ctx,
		"SELECT rol_id FROM auth_roles WHERE nombre = 'Superadmin'",
	).Scan(&superadminRolID)
	if err != nil {
		return nil // no Superadmin role = nothing to protect
	}

	// Check if new assignments still include Superadmin
	for _, a := range nuevas {
		if a.RolID == superadminRolID && a.CentroCostoID == nil {
			return nil // still a superadmin
		}
	}

	// Check if there are other superadmins besides this user
	var otherCount int
	err = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM auth_usuario_roles WHERE rol_id = ? AND centro_costo_id IS NULL AND usuario_id != ?",
		superadminRolID, usuarioID,
	).Scan(&otherCount)
	if err != nil {
		return err
	}

	if otherCount == 0 {
		return auth.ErrUltimoSuperadmin
	}
	return nil
}
