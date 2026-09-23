# Permisos, Roles y Políticas IAM — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the ad-hoc auth mechanisms (`RequireAdmin`, `RequireUserIn`, `AUTH_DELETE_ALLOWED_USER_IDS`) with a unified RBAC system with per-ranch scoping, 18 permissions across 6 modules, 3 seed roles, and graceful degradation.

**Architecture:** Five new MySQL tables define permissions, roles, and user assignments scoped by `centro_costo_id`. A new `PermissionRepo` detects tables at startup and caches effective permissions per user in a `sync.Map`. A `RequirePermission` middleware replaces `RequireAdmin`/`RequireUserIn`. The frontend loads permissions at login and gates navigation/buttons accordingly.

**Tech Stack:** Go 1.22+ (net/http, database/sql), MySQL 8, Vue 3 Composition API + TypeScript + Pinia + vue-i18n, Tailwind CSS v4.

## Global Constraints

- Auth uses MySQL stored procedures — never direct queries to `agro_usuarios`. However, the new `auth_*` tables are owned by this service, not the ERP, so direct queries to them are fine.
- `RunMigrations` is a no-op — the DBA applies all schema changes. Scripts go in `scripts/`.
- The users table is `agro_usuarios` with PK `id`.
- Graceful degradation: if `auth_permisos` doesn't exist, the system runs in permissive mode (current behavior).
- `tableExists()` from `internal/infrastructure/database/schema.go` is the detection pattern.
- All repository constructors probe `information_schema` once at startup.
- Error types follow the `errors.New("CODE")` pattern in `internal/auth/errors.go` with a `codeToErr` map.
- Tests use hand-written mocks (no mocking framework). Pattern: `type mockXxxRepo struct` in `*_test.go`.
- Frontend locales: `web/src/locales/{es,en}.json`. Every user-visible string is i18n'd.

---

## File Structure

### New Files

| File | Responsibility |
|---|---|
| `scripts/phase5-auth-permisos-roles.sql` | DDL for 5 tables + seed data (18 permisos, 3 roles, role-permission mappings) |
| `internal/auth/permission.go` | Domain types: `PermisoEfectivo`, `PermisosUsuario`, helper `TienePermiso(clave, centroCostoID)` |
| `internal/auth/permission_test.go` | Unit tests for `TienePermiso` evaluation logic |
| `internal/infrastructure/database/permission_repo.go` | `PermissionRepo`: table detection, load effective permissions, CRUD for roles/assignments, bootstrap |
| `internal/infrastructure/database/permission_repo_test.go` | Tests for bootstrap logic (uses `tableExists` mock) |
| `internal/auth/permission_middleware.go` | `RequirePermission(permiso)` and `RequireGlobalPermission(permiso)` middlewares |
| `internal/auth/permission_middleware_test.go` | Middleware tests (allow, deny, degraded mode, 401 vs 403) |
| `internal/http/handlers_permisos.go` | HTTP handlers: roles CRUD, user assignments, `GET /auth/permisos`, `POST /auth/refrescar-permisos` |
| `internal/http/handlers_permisos_test.go` | Handler tests |
| `web/src/stores/permissions.ts` | Pinia store for effective permissions + helpers `puede(clave, centroCostoId?)` |
| `web/src/views/RolesPermisosView.vue` | Admin view for managing roles and user permission assignments |

### Modified Files

| File | Changes |
|---|---|
| `internal/auth/errors.go` | Add `ErrPermisosMissing`, `ErrRolEsSistema`, `ErrUltimoSuperadmin`, `ErrNoTeQuitesPermisos` |
| `internal/http/router.go` | Replace `RequireAdmin`/`RequireUserIn` with `RequirePermission`/`RequireGlobalPermission`; add new routes |
| `internal/http/handlers.go` | Add `Permisos PermissionChecker` interface to `Handlers`; filter `ListProducciones` by ranch scope |
| `cmd/api/main.go` | Wire `PermissionRepo`, run bootstrap, inject into `Handlers` |
| `internal/config/config.go` | Add `SuperadminUserIDs` field (kept as temporary fallback alongside the new tables) |
| `web/src/api/client.ts` | Add `permisos` and `roles` API sections |
| `web/src/api/types.ts` | Add `PermisosEfectivos`, `Rol`, `Permiso`, `AsignacionRol`, `AsignacionPermiso` types |
| `web/src/stores/auth.ts` | Load permissions on login, expose `puede()` |
| `web/src/App.vue` | Conditionally show nav items based on permissions |
| `web/src/views/ConfiguracionView.vue` | Add "Roles y Permisos" tab (delegates to `RolesPermisosView`) |
| `web/src/views/ProduccionView.vue` | Gate bloquear/desbloquear/borrar buttons with `puede()` |
| `web/src/locales/es.json` | Add `permisos.*` keys |
| `web/src/locales/en.json` | Add `permisos.*` keys |
| `internal/http/handlers_test.go` | Update mocks to satisfy new `PermissionChecker` interface |
| `internal/http/handlers_delete_test.go` | Update test setup for new permission check (replaces `RequireUserIn`) |

---

### Task 1: SQL Script and Domain Types

**Files:**
- Create: `scripts/phase5-auth-permisos-roles.sql`
- Create: `internal/auth/permission.go`
- Create: `internal/auth/permission_test.go`
- Modify: `internal/auth/errors.go`

**Interfaces:**
- Consumes: nothing
- Produces: `PermisosUsuario` struct with `TienePermiso(clave string, centroCostoID *int64) bool` and `RanchosConPermiso(clave string) []int64`. Error vars `ErrPermisosMissing`, `ErrRolEsSistema`, `ErrUltimoSuperadmin`, `ErrNoTeQuitesPermisos`.

- [ ] **Step 1: Write the SQL script**

Create `scripts/phase5-auth-permisos-roles.sql`:

```sql
-- Phase 5: Sistema de permisos, roles y políticas IAM
-- Aplicar manualmente por el DBA.

-- 1. Catálogo de acciones
CREATE TABLE IF NOT EXISTS auth_permisos (
  permiso_id     INT AUTO_INCREMENT PRIMARY KEY,
  clave          VARCHAR(50)  NOT NULL UNIQUE,
  modulo         VARCHAR(30)  NOT NULL,
  descripcion    VARCHAR(120) NOT NULL,
  scope          ENUM('global','rancho') NOT NULL DEFAULT 'rancho',
  fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 2. Roles
CREATE TABLE IF NOT EXISTS auth_roles (
  rol_id         INT AUTO_INCREMENT PRIMARY KEY,
  nombre         VARCHAR(50)  NOT NULL UNIQUE,
  descripcion    VARCHAR(200),
  es_sistema     TINYINT(1)   NOT NULL DEFAULT 0,
  fecha_creacion DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 3. Permisos de cada rol
CREATE TABLE IF NOT EXISTS auth_rol_permisos (
  rol_id      INT NOT NULL,
  permiso_id  INT NOT NULL,
  PRIMARY KEY (rol_id, permiso_id),
  KEY idx_permiso (permiso_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 4. Asignación de roles con scope por rancho
CREATE TABLE IF NOT EXISTS auth_usuario_roles (
  usuario_rol_id  BIGINT AUTO_INCREMENT PRIMARY KEY,
  usuario_id      BIGINT       NOT NULL,
  rol_id          INT          NOT NULL,
  centro_costo_id BIGINT       NULL,
  asignado_por    BIGINT       NOT NULL,
  fecha_creacion  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_usr_rol_cc (usuario_id, rol_id, centro_costo_id),
  KEY idx_usuario (usuario_id),
  KEY idx_centro_costo (centro_costo_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 5. Permisos directos (excepciones)
CREATE TABLE IF NOT EXISTS auth_usuario_permisos (
  usuario_permiso_id BIGINT AUTO_INCREMENT PRIMARY KEY,
  usuario_id         BIGINT    NOT NULL,
  permiso_id         INT       NOT NULL,
  centro_costo_id    BIGINT    NULL,
  asignado_por       BIGINT    NOT NULL,
  fecha_creacion     DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_usr_perm_cc (usuario_id, permiso_id, centro_costo_id),
  KEY idx_usuario (usuario_id),
  KEY idx_centro_costo (centro_costo_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 6. Catálogo de permisos (18 acciones)
INSERT INTO auth_permisos (clave, modulo, descripcion, scope) VALUES
  ('producciones.ver',       'producciones', 'Ver lista y detalle de producciones',   'rancho'),
  ('producciones.editar',    'producciones', 'Editar polígono, fases, ia_auto',       'rancho'),
  ('producciones.bloquear',  'producciones', 'Marcar/desmarcar posible cosecha',      'rancho'),
  ('monitoreo.eliminar',     'monitoreo',    'Borrar monitoreo (irreversible)',        'rancho'),
  ('monitoreo.worker',       'monitoreo',    'Disparar worker por producción',         'rancho'),
  ('alertas.ver',            'alertas',      'Ver alertas del rancho',                 'rancho'),
  ('alertas.gestionar',      'alertas',      'Marcar vista/resuelta',                  'rancho'),
  ('fases.ver',              'fases',        'Ver fases de cultivo',                   'rancho'),
  ('fases.editar',           'fases',        'Crear/editar/copiar fases',              'rancho'),
  ('escenas.ver',            'escenas',      'Ver escenas e índices',                  'rancho'),
  ('escenas.analizar',       'escenas',      'Disparar análisis IA',                   'rancho'),
  ('usuarios.ver',           'admin',        'Listar usuarios',                        'global'),
  ('usuarios.administrar',   'admin',        'Activar/desactivar, reset password',     'global'),
  ('roles.administrar',      'admin',        'Crear/editar roles y asignar permisos',  'global'),
  ('sync.ver',               'sistema',      'Ver estado del sync',                    'global'),
  ('sync.ejecutar',          'sistema',      'Disparar sincronización',                'global'),
  ('worker.ver',             'sistema',      'Ver estado global del worker',           'global'),
  ('worker.controlar',       'sistema',      'Cancelar, desbloquear worker',           'global');

-- 7. Roles seed
INSERT INTO auth_roles (nombre, descripcion, es_sistema) VALUES
  ('Superadmin', 'Acceso total al sistema', 1),
  ('Supervisor', 'Ve todo, gestiona alertas/fases, no borra monitoreo', 1),
  ('Operador',   'Solo lectura de producciones, alertas, fases y escenas', 1);

-- 8. Permisos del Superadmin: todos
INSERT INTO auth_rol_permisos (rol_id, permiso_id)
SELECT (SELECT rol_id FROM auth_roles WHERE nombre = 'Superadmin'), permiso_id
FROM auth_permisos;

-- 9. Permisos del Supervisor (12 de 18)
INSERT INTO auth_rol_permisos (rol_id, permiso_id)
SELECT (SELECT rol_id FROM auth_roles WHERE nombre = 'Supervisor'), permiso_id
FROM auth_permisos
WHERE clave IN (
  'producciones.ver', 'producciones.editar', 'producciones.bloquear',
  'monitoreo.worker',
  'alertas.ver', 'alertas.gestionar',
  'fases.ver', 'fases.editar',
  'escenas.ver', 'escenas.analizar',
  'sync.ver', 'worker.ver'
);

-- 10. Permisos del Operador (4 de 18)
INSERT INTO auth_rol_permisos (rol_id, permiso_id)
SELECT (SELECT rol_id FROM auth_roles WHERE nombre = 'Operador'), permiso_id
FROM auth_permisos
WHERE clave IN (
  'producciones.ver', 'alertas.ver', 'fases.ver', 'escenas.ver'
);
```

- [ ] **Step 2: Write the domain types in `internal/auth/permission.go`**

```go
package auth

// PermisoScope distingue entre permisos globales y per-rancho.
type PermisoScope string

const (
	ScopeGlobal PermisoScope = "global"
	ScopeRancho PermisoScope = "rancho"
)

// AsignacionPermiso es un permiso resuelto con su scope de recurso.
// centroCostoID nil significa que aplica a todos los ranchos.
type AsignacionPermiso struct {
	Clave         string
	CentroCostoID *int64
}

// PermisosUsuario contiene todos los permisos efectivos de un usuario,
// calculados a partir de sus roles y permisos directos.
type PermisosUsuario struct {
	UsuarioID   int64
	Permisos    []AsignacionPermiso
	TodosRancho bool // true si algún permiso per-rancho tiene scope NULL (todos los ranchos)
}

// TienePermiso verifica si el usuario puede ejecutar la acción sobre el recurso.
// Para acciones globales, centroCostoID debe ser nil.
// Para acciones per-rancho, centroCostoID es el rancho de la producción.
func (p *PermisosUsuario) TienePermiso(clave string, centroCostoID *int64) bool {
	for _, a := range p.Permisos {
		if a.Clave != clave {
			continue
		}
		// scope NULL en la asignación = aplica a todos los ranchos
		if a.CentroCostoID == nil {
			return true
		}
		// scope NULL en la consulta = acción global, y la asignación es específica → no aplica
		if centroCostoID == nil {
			continue
		}
		if *a.CentroCostoID == *centroCostoID {
			return true
		}
	}
	return false
}

// TienePermisoGlobal es un atajo para acciones que no dependen de un rancho.
func (p *PermisosUsuario) TienePermisoGlobal(clave string) bool {
	return p.TienePermiso(clave, nil)
}

// RanchosConPermiso devuelve los centro_costo_id donde el usuario tiene la
// acción dada. Retorna nil si el usuario tiene el permiso con scope NULL
// (todos los ranchos); el caller debe interpretar nil como "no filtrar".
func (p *PermisosUsuario) RanchosConPermiso(clave string) []int64 {
	var ids []int64
	for _, a := range p.Permisos {
		if a.Clave != clave {
			continue
		}
		if a.CentroCostoID == nil {
			return nil // todos los ranchos
		}
		ids = append(ids, *a.CentroCostoID)
	}
	return ids
}

// PermisosResponse es la forma JSON de GET /api/v1/auth/permisos.
type PermisosResponse struct {
	Global      []string            `json:"global"`
	PorRancho   map[string][]string `json:"por_rancho"`
	RanchosTodos bool               `json:"ranchos_todos"`
	Degraded    bool                `json:"degraded"`
}

// ToResponse convierte los permisos efectivos en la forma JSON para el frontend.
func (p *PermisosUsuario) ToResponse() *PermisosResponse {
	resp := &PermisosResponse{
		Global:    []string{},
		PorRancho: map[string][]string{},
	}

	globalSet := map[string]bool{}
	ranchoSet := map[int64]map[string]bool{}

	for _, a := range p.Permisos {
		if a.CentroCostoID == nil {
			if !globalSet[a.Clave] {
				resp.Global = append(resp.Global, a.Clave)
				globalSet[a.Clave] = true
			}
			resp.RanchosTodos = true
		} else {
			ccID := *a.CentroCostoID
			if ranchoSet[ccID] == nil {
				ranchoSet[ccID] = map[string]bool{}
			}
			if !ranchoSet[ccID][a.Clave] {
				key := fmt.Sprintf("%d", ccID)
				resp.PorRancho[key] = append(resp.PorRancho[key], a.Clave)
				ranchoSet[ccID][a.Clave] = true
			}
		}
	}
	return resp
}
```

Note: add `"fmt"` to the imports.

- [ ] **Step 3: Add error variables to `internal/auth/errors.go`**

Append after the existing errors:

```go
// Permisos y roles
ErrPermisosMissing       = errors.New("PERMISOS_TABLES_MISSING")
ErrRolEsSistema          = errors.New("ROL_ES_SISTEMA")
ErrUltimoSuperadmin      = errors.New("ULTIMO_SUPERADMIN")
ErrNoTeQuitesPermisos    = errors.New("NO_TE_QUITES_PERMISOS")
ErrRolNotFound           = errors.New("ROL_NOT_FOUND")
ErrPermisoNotFound       = errors.New("PERMISO_NOT_FOUND")
ErrCentroCostoNotFound   = errors.New("CENTRO_COSTO_NOT_FOUND")
```

And add to `codeToErr`:

```go
"ROL_ES_SISTEMA":        ErrRolEsSistema,
"ULTIMO_SUPERADMIN":     ErrUltimoSuperadmin,
"ROL_NOT_FOUND":         ErrRolNotFound,
```

- [ ] **Step 4: Write tests in `internal/auth/permission_test.go`**

```go
package auth

import (
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }

func TestTienePermisoGlobalConScopeNull(t *testing.T) {
	p := &PermisosUsuario{
		Permisos: []AsignacionPermiso{
			{Clave: "usuarios.ver", CentroCostoID: nil},
		},
	}
	if !p.TienePermisoGlobal("usuarios.ver") {
		t.Error("expected true for global permission with NULL scope")
	}
}

func TestTienePermisoRanchoEspecifico(t *testing.T) {
	p := &PermisosUsuario{
		Permisos: []AsignacionPermiso{
			{Clave: "producciones.ver", CentroCostoID: int64Ptr(42)},
		},
	}
	if !p.TienePermiso("producciones.ver", int64Ptr(42)) {
		t.Error("expected true for matching centro_costo_id")
	}
	if p.TienePermiso("producciones.ver", int64Ptr(99)) {
		t.Error("expected false for non-matching centro_costo_id")
	}
}

func TestTienePermisoScopeNullCubreTodosRanchos(t *testing.T) {
	p := &PermisosUsuario{
		Permisos: []AsignacionPermiso{
			{Clave: "producciones.ver", CentroCostoID: nil},
		},
	}
	if !p.TienePermiso("producciones.ver", int64Ptr(42)) {
		t.Error("NULL scope should grant access to any rancho")
	}
	if !p.TienePermiso("producciones.ver", int64Ptr(99)) {
		t.Error("NULL scope should grant access to any rancho")
	}
}

func TestTienePermisoSinPermisos(t *testing.T) {
	p := &PermisosUsuario{Permisos: nil}
	if p.TienePermisoGlobal("usuarios.ver") {
		t.Error("empty permissions should deny everything")
	}
}

func TestRanchosConPermisoDevuelveNilParaScopeNull(t *testing.T) {
	p := &PermisosUsuario{
		Permisos: []AsignacionPermiso{
			{Clave: "producciones.ver", CentroCostoID: nil},
		},
	}
	ids := p.RanchosConPermiso("producciones.ver")
	if ids != nil {
		t.Errorf("expected nil (all ranchos), got %v", ids)
	}
}

func TestRanchosConPermisoDevuelveListaEspecifica(t *testing.T) {
	p := &PermisosUsuario{
		Permisos: []AsignacionPermiso{
			{Clave: "producciones.ver", CentroCostoID: int64Ptr(10)},
			{Clave: "producciones.ver", CentroCostoID: int64Ptr(20)},
			{Clave: "alertas.ver", CentroCostoID: int64Ptr(10)},
		},
	}
	ids := p.RanchosConPermiso("producciones.ver")
	if len(ids) != 2 {
		t.Errorf("expected 2 ranchos, got %d", len(ids))
	}
}

func TestSupervisorNoPuedeBorrarMonitoreo(t *testing.T) {
	// Supervisor tiene producciones.*, alertas.*, etc. pero NO monitoreo.eliminar
	p := &PermisosUsuario{
		Permisos: []AsignacionPermiso{
			{Clave: "producciones.ver", CentroCostoID: int64Ptr(42)},
			{Clave: "producciones.editar", CentroCostoID: int64Ptr(42)},
			{Clave: "alertas.ver", CentroCostoID: int64Ptr(42)},
		},
	}
	if p.TienePermiso("monitoreo.eliminar", int64Ptr(42)) {
		t.Error("supervisor should NOT have monitoreo.eliminar")
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/auth/ -run TestTienePermiso -v`
Expected: all 7 tests PASS

- [ ] **Step 6: Commit**

```bash
git add scripts/phase5-auth-permisos-roles.sql internal/auth/permission.go internal/auth/permission_test.go internal/auth/errors.go
git commit -m "feat(auth): add permission domain types, SQL schema, and seed data

Defines PermisosUsuario with TienePermiso evaluation logic, 18 permissions
across 6 modules, 3 seed roles (Superadmin, Supervisor, Operador), and
error types for the IAM system."
```

---

### Task 2: Permission Repository with Table Detection and Caching

**Files:**
- Create: `internal/infrastructure/database/permission_repo.go`

**Interfaces:**
- Consumes: `tableExists()` from `schema.go`, `PermisosUsuario`/`AsignacionPermiso` from `permission.go`
- Produces: `PermissionRepo` with methods: `Disponible() bool`, `CargarPermisos(ctx, usuarioID) (*auth.PermisosUsuario, error)`, `ObtenerPermisosCacheados(usuarioID) *auth.PermisosUsuario`, `InvalidarCache(usuarioID)`, `Bootstrap(ctx, log)`, `ListRoles(ctx)`, `GetRol(ctx, rolID)`, `CreateRol(ctx, nombre, descripcion, permisoIDs)`, `UpdateRol(ctx, rolID, nombre, descripcion, permisoIDs)`, `DeleteRol(ctx, rolID)`, `ListPermisos(ctx)`, `GetPermisosEfectivos(ctx, usuarioID)`, `SetUsuarioRoles(ctx, usuarioID, asignaciones, asignadoPor)`, `SetUsuarioPermisos(ctx, usuarioID, asignaciones, asignadoPor)`, `ListCentrosCostos(ctx)`.

- [ ] **Step 1: Write `internal/infrastructure/database/permission_repo.go`**

```go
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
	db     *sql.DB
	exists bool
	cache  sync.Map // map[int64]*auth.PermisosUsuario
}

func NewPermissionRepo(db *sql.DB) *PermissionRepo {
	return &PermissionRepo{
		db:     db,
		exists: tableExists(db, "auth_permisos"),
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
		return v.(*auth.PermisosUsuario)
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

	return r.setRolPermisos(ctx, rolID, permisoIDs)
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

	r.db.ExecContext(ctx, "DELETE FROM auth_rol_permisos WHERE rol_id = ?", rolID)
	r.db.ExecContext(ctx, "DELETE FROM auth_usuario_roles WHERE rol_id = ?", rolID)
	_, err = r.db.ExecContext(ctx, "DELETE FROM auth_roles WHERE rol_id = ?", rolID)
	return err
}

func (r *PermissionRepo) setRolPermisos(ctx context.Context, rolID int, permisoIDs []int) error {
	r.db.ExecContext(ctx, "DELETE FROM auth_rol_permisos WHERE rol_id = ?", rolID)
	for _, pid := range permisoIDs {
		if _, err := r.db.ExecContext(ctx,
			"INSERT INTO auth_rol_permisos (rol_id, permiso_id) VALUES (?, ?)",
			rolID, pid,
		); err != nil {
			return fmt.Errorf("inserting rol_permiso: %w", err)
		}
	}
	return nil
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

	_, err := r.db.ExecContext(ctx, "DELETE FROM auth_usuario_roles WHERE usuario_id = ?", usuarioID)
	if err != nil {
		return fmt.Errorf("clearing user roles: %w", err)
	}

	for _, a := range asignaciones {
		_, err := r.db.ExecContext(ctx,
			"INSERT INTO auth_usuario_roles (usuario_id, rol_id, centro_costo_id, asignado_por) VALUES (?, ?, ?, ?)",
			usuarioID, a.RolID, a.CentroCostoID, asignadoPor,
		)
		if err != nil {
			return fmt.Errorf("inserting user role: %w", err)
		}
	}

	r.InvalidarCache(usuarioID)
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

	_, err := r.db.ExecContext(ctx, "DELETE FROM auth_usuario_permisos WHERE usuario_id = ?", usuarioID)
	if err != nil {
		return fmt.Errorf("clearing user direct perms: %w", err)
	}

	for _, a := range asignaciones {
		_, err := r.db.ExecContext(ctx,
			"INSERT INTO auth_usuario_permisos (usuario_id, permiso_id, centro_costo_id, asignado_por) VALUES (?, ?, ?, ?)",
			usuarioID, a.PermisoID, a.CentroCostoID, asignadoPor,
		)
		if err != nil {
			return fmt.Errorf("inserting user direct perm: %w", err)
		}
	}

	r.InvalidarCache(usuarioID)
	return nil
}

// GetUsuarioAsignaciones returns the current role and direct permission
// assignments for a user (for the admin edit UI, not evaluation).
type UsuarioAsignaciones struct {
	Roles    []AsignacionRolDetalle    `json:"roles"`
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

	out := &UsuarioAsignaciones{}

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
	const q = `SELECT centro_costo_id, nombre FROM centros_costos ORDER BY nombre`
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
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/infrastructure/database/`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add internal/infrastructure/database/permission_repo.go
git commit -m "feat(auth): add PermissionRepo with caching, bootstrap, and CRUD

Table detection at startup, sync.Map cache per user, bootstrap first user
as Superadmin, roles/permissions CRUD, last-Superadmin protection."
```

---

### Task 3: Permission Middleware

**Files:**
- Create: `internal/auth/permission_middleware.go`
- Create: `internal/auth/permission_middleware_test.go`

**Interfaces:**
- Consumes: `PermisosUsuario`, `ClaimsFromContext()`, `writeAuthError()`
- Produces: `RequirePermission(permiso string, resolver CentroCostoResolver) func(http.Handler) http.Handler`, `RequireGlobalPermission(permiso string, loader PermissionLoader) func(http.Handler) http.Handler`

- [ ] **Step 1: Write `internal/auth/permission_middleware.go`**

```go
package auth

import (
	"context"
	"net/http"
	"strconv"
)

// PermissionLoader loads or returns cached effective permissions for a user.
type PermissionLoader interface {
	Disponible() bool
	CargarPermisos(ctx context.Context, usuarioID int64) (*PermisosUsuario, error)
	ObtenerPermisosCacheados(usuarioID int64) *PermisosUsuario
}

// CentroCostoResolver resolves a produccion monitoring ID to its centro_costo_id.
type CentroCostoResolver interface {
	GetCentroCostoByMonitoringID(ctx context.Context, monitoringID uint) (*int64, error)
}

func getOrLoadPermisos(ctx context.Context, loader PermissionLoader, userID int64) (*PermisosUsuario, error) {
	if p := loader.ObtenerPermisosCacheados(userID); p != nil {
		return p, nil
	}
	return loader.CargarPermisos(ctx, userID)
}

// RequireGlobalPermission checks a global-scope permission (no ranch needed).
// If the permission tables don't exist, it passes through (permissive mode).
func RequireGlobalPermission(permiso string, loader PermissionLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !loader.Disponible() {
				next.ServeHTTP(w, r)
				return
			}

			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeAuthError(w, http.StatusUnauthorized, "autenticación requerida")
				return
			}

			perms, err := getOrLoadPermisos(r.Context(), loader, claims.UserID)
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "error cargando permisos")
				return
			}

			if !perms.TienePermisoGlobal(permiso) {
				writeAuthError(w, http.StatusForbidden, "no tienes permiso: "+permiso)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequirePermission checks a ranch-scoped permission by resolving the {id}
// path parameter to a centro_costo_id.
// If the permission tables don't exist, it passes through (permissive mode).
func RequirePermission(permiso string, loader PermissionLoader, resolver CentroCostoResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !loader.Disponible() {
				next.ServeHTTP(w, r)
				return
			}

			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				writeAuthError(w, http.StatusUnauthorized, "autenticación requerida")
				return
			}

			perms, err := getOrLoadPermisos(r.Context(), loader, claims.UserID)
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "error cargando permisos")
				return
			}

			idStr := r.PathValue("id")
			id, _ := strconv.ParseUint(idStr, 10, 64)
			if id == 0 {
				writeAuthError(w, http.StatusBadRequest, "id inválido")
				return
			}

			ccID, err := resolver.GetCentroCostoByMonitoringID(r.Context(), uint(id))
			if err != nil {
				writeAuthError(w, http.StatusInternalServerError, "error resolviendo rancho")
				return
			}

			if !perms.TienePermiso(permiso, ccID) {
				writeAuthError(w, http.StatusForbidden, "no tienes permiso: "+permiso)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 2: Write tests in `internal/auth/permission_middleware_test.go`**

```go
package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockLoader struct {
	disponible bool
	permisos   map[int64]*PermisosUsuario
}

func (m *mockLoader) Disponible() bool { return m.disponible }
func (m *mockLoader) CargarPermisos(_ context.Context, uid int64) (*PermisosUsuario, error) {
	if p, ok := m.permisos[uid]; ok {
		return p, nil
	}
	return &PermisosUsuario{UsuarioID: uid}, nil
}
func (m *mockLoader) ObtenerPermisosCacheados(uid int64) *PermisosUsuario {
	if p, ok := m.permisos[uid]; ok {
		return p
	}
	return nil
}

type mockResolver struct {
	mapping map[uint]*int64
}

func (m *mockResolver) GetCentroCostoByMonitoringID(_ context.Context, id uint) (*int64, error) {
	return m.mapping[id], nil
}

func runGlobalMiddleware(t *testing.T, loader *mockLoader, permiso string, userID int64, hasClaims bool) (int, bool) {
	t.Helper()
	called := false
	h := RequireGlobalPermission(permiso, loader)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	if hasClaims {
		req = req.WithContext(context.WithValue(req.Context(), claimsKey, &Claims{UserID: userID, Username: "u"}))
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w.Code, called
}

func TestGlobalPermissionDegradedModePassesThrough(t *testing.T) {
	loader := &mockLoader{disponible: false}
	code, called := runGlobalMiddleware(t, loader, "usuarios.ver", 1, false)
	if code != 200 || !called {
		t.Errorf("degraded mode should pass through; code=%d called=%v", code, called)
	}
}

func TestGlobalPermissionDeniesWithoutSession(t *testing.T) {
	loader := &mockLoader{disponible: true, permisos: map[int64]*PermisosUsuario{}}
	code, called := runGlobalMiddleware(t, loader, "usuarios.ver", 1, false)
	if code != 401 {
		t.Errorf("expected 401, got %d", code)
	}
	if called {
		t.Error("handler should not have been called")
	}
}

func TestGlobalPermissionDeniesMissingPermission(t *testing.T) {
	loader := &mockLoader{
		disponible: true,
		permisos: map[int64]*PermisosUsuario{
			1: {UsuarioID: 1, Permisos: []AsignacionPermiso{
				{Clave: "sync.ver", CentroCostoID: nil},
			}},
		},
	}
	code, called := runGlobalMiddleware(t, loader, "usuarios.ver", 1, true)
	if code != 403 {
		t.Errorf("expected 403, got %d", code)
	}
	if called {
		t.Error("handler should not have been called")
	}
}

func TestGlobalPermissionAllowsWithPermission(t *testing.T) {
	loader := &mockLoader{
		disponible: true,
		permisos: map[int64]*PermisosUsuario{
			1: {UsuarioID: 1, Permisos: []AsignacionPermiso{
				{Clave: "usuarios.ver", CentroCostoID: nil},
			}},
		},
	}
	code, called := runGlobalMiddleware(t, loader, "usuarios.ver", 1, true)
	if code != 200 {
		t.Errorf("expected 200, got %d", code)
	}
	if !called {
		t.Error("handler should have been called")
	}
}

func TestRanchPermissionDeniesWrongRancho(t *testing.T) {
	cc42 := int64(42)
	loader := &mockLoader{
		disponible: true,
		permisos: map[int64]*PermisosUsuario{
			1: {UsuarioID: 1, Permisos: []AsignacionPermiso{
				{Clave: "monitoreo.eliminar", CentroCostoID: &cc42},
			}},
		},
	}
	cc99 := int64(99)
	resolver := &mockResolver{mapping: map[uint]*int64{5: &cc99}}

	called := false
	h := RequirePermission("monitoreo.eliminar", loader, resolver)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("DELETE", "/api/v1/producciones/5/monitoreo", nil)
	req.SetPathValue("id", "5")
	req = req.WithContext(context.WithValue(req.Context(), claimsKey, &Claims{UserID: 1, Username: "u"}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 403 {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if called {
		t.Error("handler should not be called for wrong rancho")
	}
}

func TestRanchPermissionAllowsCorrectRancho(t *testing.T) {
	cc42 := int64(42)
	loader := &mockLoader{
		disponible: true,
		permisos: map[int64]*PermisosUsuario{
			1: {UsuarioID: 1, Permisos: []AsignacionPermiso{
				{Clave: "monitoreo.eliminar", CentroCostoID: &cc42},
			}},
		},
	}
	resolver := &mockResolver{mapping: map[uint]*int64{5: &cc42}}

	called := false
	h := RequirePermission("monitoreo.eliminar", loader, resolver)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("DELETE", "/api/v1/producciones/5/monitoreo", nil)
	req.SetPathValue("id", "5")
	req = req.WithContext(context.WithValue(req.Context(), claimsKey, &Claims{UserID: 1, Username: "u"}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !called {
		t.Error("handler should have been called")
	}
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/auth/ -run TestGlobal -v && go test ./internal/auth/ -run TestRanch -v`
Expected: all 6 tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/auth/permission_middleware.go internal/auth/permission_middleware_test.go
git commit -m "feat(auth): add RequirePermission and RequireGlobalPermission middlewares

Graceful degradation when permission tables don't exist (passes through).
Distinguishes 401 (no session) from 403 (no permission). Ranch-scoped
permissions resolve centro_costo_id from the production's monitoring ID."
```

---

### Task 4: CentroCostoResolver and Production Filtering

**Files:**
- Modify: `internal/infrastructure/database/production_repo.go` — add `GetCentroCostoByMonitoringID`
- Modify: `internal/http/handlers.go` — add `PermissionChecker` interface, filter `ListProducciones`

**Interfaces:**
- Consumes: `CentroCostoResolver` from `permission_middleware.go`, `PermisosUsuario` from `permission.go`
- Produces: `ProductionRepo.GetCentroCostoByMonitoringID(ctx, monitoringID) (*int64, error)`, `PermissionChecker` interface on `Handlers`

- [ ] **Step 1: Add `GetCentroCostoByMonitoringID` to `production_repo.go`**

Add after `GetERPDetails`:

```go
// GetCentroCostoByMonitoringID resolves a monitoring production ID to its
// centro_costo_id via the ERP tables. Returns nil when not found.
func (r *ProductionRepo) GetCentroCostoByMonitoringID(ctx context.Context, monitoringID uint) (*int64, error) {
	const q = `
SELECT p.centro_costo_id
FROM s3_monitoring_producciones mp
JOIN producciones p ON mp.produccion_id = p.produccion_id
WHERE mp.s3_monitoring_produccion_id = ?`

	var ccID int64
	err := r.db.QueryRowContext(ctx, q, monitoringID).Scan(&ccID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("resolving centro_costo for monitoring %d: %w", monitoringID, err)
	}
	return &ccID, nil
}
```

- [ ] **Step 2: Add `PermissionChecker` interface and filter `ListProducciones`**

Add to `internal/http/handlers.go` after the existing interfaces:

```go
// PermissionChecker provides permission evaluation for handlers.
// Nil or unavailable = permissive mode (no filtering).
type PermissionChecker interface {
	Disponible() bool
	CargarPermisos(ctx context.Context, usuarioID int64) (*auth.PermisosUsuario, error)
	ObtenerPermisosCacheados(usuarioID int64) *auth.PermisosUsuario
	InvalidarCache(usuarioID int64)
}
```

Add `Permisos PermissionChecker` field to `Handlers` struct.

Modify `ListProducciones` to filter by ranch permissions: after fetching all productions, if `h.Permisos` is non-nil and `Disponible()`, load the user's permissions and filter productions to only those where `centro_costo_id` is in the allowed set.

- [ ] **Step 3: Update mock in `handlers_test.go`**

Add a `mockPermissionChecker` that returns `Disponible() = false` (permissive mode) for existing tests to pass unchanged.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/http/ -v`
Expected: all existing tests still PASS

- [ ] **Step 5: Commit**

```bash
git add internal/infrastructure/database/production_repo.go internal/http/handlers.go internal/http/handlers_test.go
git commit -m "feat(auth): add CentroCostoResolver and production filtering by ranch

GetCentroCostoByMonitoringID resolves monitoring ID to centro_costo_id.
ListProducciones now filters by the user's producciones.ver permission scope."
```

---

### Task 5: Wire Router, Replace Old Middlewares, Add Permission Endpoints

**Files:**
- Modify: `internal/http/router.go` — replace `RequireAdmin`/`RequireUserIn` with `RequirePermission`/`RequireGlobalPermission`; add new routes
- Create: `internal/http/handlers_permisos.go` — HTTP handlers for roles CRUD, user assignments, `GET /auth/permisos`, `POST /auth/refrescar-permisos`
- Modify: `cmd/api/main.go` — wire `PermissionRepo`, run bootstrap
- Modify: `internal/config/config.go` — keep `DeleteAllowedUserIDs` as fallback

**Interfaces:**
- Consumes: All from Tasks 1-4
- Produces: Complete backend wiring; all endpoints functional

- [ ] **Step 1: Create `internal/http/handlers_permisos.go`**

This file contains:
- `PermissionRepository` interface (CRUD for roles, permisos, assignments)
- Handlers: `ListRoles`, `CreateRol`, `UpdateRol`, `DeleteRol`, `ListPermisosCatalogo`, `GetUsuarioPermisos`, `SetUsuarioRoles`, `SetUsuarioPermisos`, `MisPermisos`, `RefrescarPermisos`, `ListCentrosCostos`
- Follow the existing handler pattern (JSON envelope `{"data": ...}`, `Error()` helper for errors)

- [ ] **Step 2: Update `internal/http/router.go`**

Replace all `RequireAdmin` calls with `RequireGlobalPermission("usuarios.ver", h.Permisos)` or `RequireGlobalPermission("usuarios.administrar", h.Permisos)`.

Replace `RequireUserIn(h.DeleteAllowedUserIDs)` with `RequirePermission("monitoreo.eliminar", h.Permisos, h.Productions)` (where `h.Productions` implements `CentroCostoResolver`).

Add ranch-scoped permission middleware for write operations: bloquear, desbloquear, fases PUT, poligono PUT, escena analizar, worker run-production.

Add global permission middleware for: sync trigger, worker cancel/unlock/run.

Add new routes:
- `GET /api/v1/admin/roles`
- `POST /api/v1/admin/roles`
- `PUT /api/v1/admin/roles/{id}`
- `DELETE /api/v1/admin/roles/{id}`
- `GET /api/v1/admin/permisos`
- `GET /api/v1/admin/usuarios/{id}/permisos`
- `PUT /api/v1/admin/usuarios/{id}/roles`
- `PUT /api/v1/admin/usuarios/{id}/permisos-directos`
- `GET /api/v1/admin/centros-costos`
- `GET /api/v1/auth/permisos`
- `POST /api/v1/auth/refrescar-permisos`

All admin routes use `RequireGlobalPermission("roles.administrar", h.Permisos)`.

- [ ] **Step 3: Update `cmd/api/main.go`**

After creating repos, add:
```go
permRepo := database.NewPermissionRepo(db)
if permRepo.Disponible() {
    permRepo.Bootstrap(ctx, l)
    l.Info("permission system active")
} else {
    l.Warn("auth_permisos table not found, running in permissive mode")
}
```

Set `h.Permisos = permRepo`.

Pass `permRepo` to the router for middleware construction.

Update `NewRouter` signature to accept the permission loader and resolver.

Keep `DeleteAllowedUserIDs` wiring as fallback when tables don't exist (the new `RequirePermission` middleware handles this via `Disponible()`).

- [ ] **Step 4: Verify compilation**

Run: `go build ./...`
Expected: success

- [ ] **Step 5: Run all tests**

Run: `go test ./internal/... -v`
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add internal/http/router.go internal/http/handlers_permisos.go cmd/api/main.go internal/config/config.go
git commit -m "feat(auth): wire permission system, replace RequireAdmin/RequireUserIn

All write routes now guarded by RequirePermission or RequireGlobalPermission.
Admin routes for roles/permissions CRUD. Bootstrap assigns first user as
Superadmin. Graceful degradation when tables don't exist."
```

---

### Task 6: Handler Tests for Permission Endpoints

**Files:**
- Create: `internal/http/handlers_permisos_test.go`
- Modify: `internal/http/handlers_delete_test.go` — update for new middleware

**Interfaces:**
- Consumes: handlers from Task 5
- Produces: test coverage for permission endpoints and delete middleware migration

- [ ] **Step 1: Write `internal/http/handlers_permisos_test.go`**

Test cases:
- `TestMisPermisosDevuelveDegradedSinTablas` — when `Disponible()` is false, returns `{"degraded": true}`
- `TestMisPermisosDevuelvePermisosEfectivos` — returns global + por_rancho
- `TestListRolesRequiereRolesAdministrar` — returns 403 without `roles.administrar`
- `TestSetUsuarioRolesProtegeSuperadmin` — returns error when removing last Superadmin
- `TestSetUsuarioRolesRechazaAutoQuitarse` — returns error when user removes own permissions

- [ ] **Step 2: Update `handlers_delete_test.go`**

Replace `RequireUserIn` test setup with the new `RequirePermission` middleware. The `deleteHandlersCon` helper needs updating to pass a mock `PermissionChecker` and `CentroCostoResolver`.

- [ ] **Step 3: Run all tests**

Run: `go test ./internal/http/ -v`
Expected: all tests PASS

- [ ] **Step 4: Commit**

```bash
git add internal/http/handlers_permisos_test.go internal/http/handlers_delete_test.go
git commit -m "test(auth): add permission handler tests, update delete test middleware

Tests MisPermisos degraded mode, effective permissions response, role CRUD
authorization, last-Superadmin protection, and self-demotion prevention."
```

---

### Task 7: Frontend — Permissions Store and API Client

**Files:**
- Modify: `web/src/api/types.ts` — add permission types
- Modify: `web/src/api/client.ts` — add `permisos` and `roles` API sections
- Create: `web/src/stores/permissions.ts` — Pinia store with `puede()` helper
- Modify: `web/src/stores/auth.ts` — load permissions on login

**Interfaces:**
- Consumes: backend endpoints from Task 5
- Produces: `usePermissionsStore()` with `puede(clave, centroCostoId?)`, `ranchosCon(clave)`, `tieneAlgunPermiso(modulo)`

- [ ] **Step 1: Add types to `web/src/api/types.ts`**

```typescript
// ── Permisos y Roles ──────────────────────────────────────────────────────────

export interface PermisosEfectivos {
  global: string[]
  por_rancho: Record<string, string[]>
  ranchos_todos: boolean
  degraded: boolean
}

export interface PermisoCatalogo {
  permiso_id: number
  clave: string
  modulo: string
  descripcion: string
  scope: 'global' | 'rancho'
}

export interface Rol {
  rol_id: number
  nombre: string
  descripcion: string
  es_sistema: boolean
  permiso_ids: number[]
}

export interface AsignacionRol {
  rol_id: number
  centro_costo_id: number | null
}

export interface AsignacionRolDetalle {
  usuario_rol_id: number
  rol_id: number
  rol_nombre: string
  centro_costo_id: number | null
  rancho_nombre: string
}

export interface AsignacionPermisoDetalle {
  usuario_permiso_id: number
  permiso_id: number
  permiso_clave: string
  centro_costo_id: number | null
  rancho_nombre: string
}

export interface UsuarioAsignaciones {
  roles: AsignacionRolDetalle[]
  directos: AsignacionPermisoDetalle[]
}

export interface CentroCostoItem {
  centro_costo_id: number
  nombre: string
}
```

- [ ] **Step 2: Add API methods to `web/src/api/client.ts`**

```typescript
export const permisos = {
  mis: () =>
    http.get<{ data: PermisosEfectivos }>('/auth/permisos').then(unwrap),

  refrescar: () =>
    http.post('/auth/refrescar-permisos'),
}

export const roles = {
  list: () =>
    http.get<{ data: Rol[] }>('/admin/roles').then(unwrap),

  create: (nombre: string, descripcion: string, permiso_ids: number[]) =>
    http.post<{ data: Rol }>('/admin/roles', { nombre, descripcion, permiso_ids }).then(unwrap),

  update: (id: number, nombre: string, descripcion: string, permiso_ids: number[]) =>
    http.put<{ data: Rol }>(`/admin/roles/${id}`, { nombre, descripcion, permiso_ids }).then(unwrap),

  delete: (id: number) =>
    http.delete(`/admin/roles/${id}`),

  permisosCatalogo: () =>
    http.get<{ data: PermisoCatalogo[] }>('/admin/permisos').then(unwrap),

  getUsuarioPermisos: (userId: number) =>
    http.get<{ data: UsuarioAsignaciones }>(`/admin/usuarios/${userId}/permisos`).then(unwrap),

  setUsuarioRoles: (userId: number, asignaciones: AsignacionRol[]) =>
    http.put(`/admin/usuarios/${userId}/roles`, { asignaciones }),

  setUsuarioPermisos: (userId: number, asignaciones: { permiso_id: number; centro_costo_id: number | null }[]) =>
    http.put(`/admin/usuarios/${userId}/permisos-directos`, { asignaciones }),

  centrosCostos: () =>
    http.get<{ data: CentroCostoItem[] }>('/admin/centros-costos').then(unwrap),
}
```

- [ ] **Step 3: Create `web/src/stores/permissions.ts`**

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { permisos as permisosApi } from '@/api/client'
import type { PermisosEfectivos } from '@/api/types'

export const usePermissionsStore = defineStore('permissions', () => {
  const data = ref<PermisosEfectivos | null>(null)
  const loaded = ref(false)

  async function load() {
    try {
      data.value = await permisosApi.mis()
    } catch {
      data.value = { global: [], por_rancho: {}, ranchos_todos: false, degraded: true }
    }
    loaded.value = true
  }

  async function refrescar() {
    await permisosApi.refrescar()
    await load()
  }

  function reset() {
    data.value = null
    loaded.value = false
  }

  const degraded = computed(() => data.value?.degraded ?? true)

  function puede(clave: string, centroCostoId?: number): boolean {
    if (!data.value || data.value.degraded) return true
    if (data.value.global.includes(clave)) return true
    if (data.value.ranchos_todos && data.value.global.includes(clave)) return true
    if (centroCostoId != null) {
      const permsRancho = data.value.por_rancho[String(centroCostoId)]
      if (permsRancho?.includes(clave)) return true
    }
    // Check ranchos_todos: if user has the permission with NULL scope
    // it appears in global for ranch-scoped perms too
    if (data.value.ranchos_todos) {
      // permissions with NULL scope are in global regardless of scope type
      if (data.value.global.includes(clave)) return true
    }
    return false
  }

  function ranchosCon(clave: string): number[] | null {
    if (!data.value || data.value.degraded) return null
    if (data.value.global.includes(clave)) return null // all ranchos
    const ids: number[] = []
    for (const [ccId, perms] of Object.entries(data.value.por_rancho)) {
      if (perms.includes(clave)) ids.push(Number(ccId))
    }
    return ids
  }

  function tieneAlgunPermiso(modulo: string): boolean {
    if (!data.value || data.value.degraded) return true
    const prefix = modulo + '.'
    if (data.value.global.some((p) => p.startsWith(prefix))) return true
    for (const perms of Object.values(data.value.por_rancho)) {
      if (perms.some((p) => p.startsWith(prefix))) return true
    }
    return false
  }

  return { data, loaded, degraded, load, refrescar, reset, puede, ranchosCon, tieneAlgunPermiso }
})
```

- [ ] **Step 4: Modify `web/src/stores/auth.ts`**

In the `login` function, after setting token/username, call `usePermissionsStore().load()`.

In `logout`, call `usePermissionsStore().reset()`.

In the watch in `App.vue` that triggers on `auth.isAuthenticated`, add `permStore.load()` alongside `prodStore.loadAll()`.

- [ ] **Step 5: Commit**

```bash
git add web/src/api/types.ts web/src/api/client.ts web/src/stores/permissions.ts web/src/stores/auth.ts
git commit -m "feat(frontend): add permissions store and API client

Permissions loaded at login, cached in Pinia store. puede() helper checks
ranch-scoped and global permissions. Degraded mode returns true for all
checks (backwards compatible)."
```

---

### Task 8: Frontend — Conditional Navigation and Action Gating

**Files:**
- Modify: `web/src/App.vue` — conditionally show nav items
- Modify: `web/src/views/ProduccionView.vue` — gate buttons
- Modify: `web/src/locales/es.json` — add `permisos.*` keys
- Modify: `web/src/locales/en.json` — add `permisos.*` keys

**Interfaces:**
- Consumes: `usePermissionsStore().puede()`, `tieneAlgunPermiso()`
- Produces: UI that respects permissions

- [ ] **Step 1: Update `App.vue` navigation**

Import `usePermissionsStore`. Conditionally show:
- "Alertas" menu item: `permStore.tieneAlgunPermiso('alertas')`
- "Configuración" menu item: `permStore.tieneAlgunPermiso('admin')`

In the auth watch, add `permStore.load()` when authenticated, `permStore.reset()` on logout.

- [ ] **Step 2: Update `ProduccionView.vue` action buttons**

Import `usePermissionsStore`. For each production's `CentroCostoID`:
- "Marcar posible cosecha" button: `:disabled="!permStore.puede('producciones.bloquear', prod.CentroCostoID)"`
- "Eliminar monitoreo" button: `:disabled="!permStore.puede('monitoreo.eliminar', prod.CentroCostoID)"`
- "Analizar IA" button: `:disabled="!permStore.puede('escenas.analizar', prod.CentroCostoID)"`

Show a tooltip on disabled buttons: `"No tienes permiso para esta acción"`.

Note: `CentroCostoID` is not currently in the `Production` type sent to the frontend. Add it to the Go API response and to the TypeScript `Production` interface.

- [ ] **Step 3: Add locale keys**

Add to both `es.json` and `en.json`:

```json
"permisos": {
  "sinPermiso": "No tienes permiso para esta acción",
  "titulo": "Roles y Permisos",
  "roles": "Roles",
  "permisos": "Permisos",
  "usuarios": "Asignaciones de usuario",
  "agregarRol": "Agregar rol",
  "crearRol": "Crear rol",
  "editarRol": "Editar rol",
  "eliminarRol": "Eliminar rol",
  "nombre": "Nombre",
  "descripcion": "Descripción",
  "todosRanchos": "Todos los ranchos",
  "seleccionarRancho": "Seleccionar rancho",
  "rolSistema": "Rol de sistema (no se puede eliminar)",
  "confirmarEliminar": "¿Estás seguro de eliminar el rol \"{nombre}\"?",
  "permisoDirecto": "Permiso directo",
  "efectivos": "Permisos efectivos",
  "origen": "Origen",
  "guardar": "Guardar",
  "modulos": {
    "producciones": "Producciones",
    "monitoreo": "Monitoreo",
    "alertas": "Alertas",
    "fases": "Fases",
    "escenas": "Escenas",
    "admin": "Administración",
    "sistema": "Sistema"
  }
}
```

English version with translations accordingly.

- [ ] **Step 4: Test in browser**

Start the dev server and verify:
- Nav items show/hide based on permissions
- Buttons are disabled without permission
- Degraded mode (no tables) shows everything (backwards compatible)

- [ ] **Step 5: Commit**

```bash
git add web/src/App.vue web/src/views/ProduccionView.vue web/src/locales/es.json web/src/locales/en.json
git commit -m "feat(frontend): gate navigation and action buttons by permissions

Menu items and action buttons respect the user's effective permissions.
Degraded mode (no permission tables) shows everything for backwards
compatibility."
```

---

### Task 9: Frontend — Roles and Permissions Admin View

**Files:**
- Create: `web/src/views/RolesPermisosView.vue`
- Modify: `web/src/views/ConfiguracionView.vue` — add tab for roles/permisos
- Modify: `web/src/router/index.ts` — no new route needed (it's a tab within Configuración)

**Interfaces:**
- Consumes: `roles.*` and `admin.*` API methods, `usePermissionsStore()`
- Produces: Complete admin UI for managing roles and user permission assignments

- [ ] **Step 1: Create `web/src/views/RolesPermisosView.vue`**

Two-panel layout:

**Left panel — Roles:**
- List all roles with badge for permission count
- Click to expand: checkboxes grouped by module (producciones, monitoreo, alertas, etc.)
- "Crear rol" button opens inline form (name + description + checkboxes)
- System roles (`es_sistema`) show a lock icon, can't be deleted but can have perms viewed
- "Guardar" saves the role's permissions

**Right panel — User Permissions (appears when a user is selected from the user list):**
- Section 1: "Roles asignados" — chips showing `Rol @ Rancho`. Add button opens a dropdown (rol selector + rancho selector with "Todos" option).
- Section 2: "Permisos directos" — chips showing `Permiso @ Rancho`. Add button with permission + rancho selectors.
- Section 3: "Permisos efectivos" (read-only) — collapsible table by module showing final resolved permissions.

Use `roles.centrosCostos()` to populate the rancho selector.

- [ ] **Step 2: Add tab to `ConfiguracionView.vue`**

Add a tab bar at the top: "Usuarios" | "Roles y Permisos".
- "Roles y Permisos" tab is visible only when `permStore.puede('roles.administrar')`.
- When selected, renders `<RolesPermisosView />`.
- User list in the right panel reuses the already-loaded `usuarios` data.

- [ ] **Step 3: Test in browser**

- Create a custom role with specific permissions
- Assign the role to a user with a specific ranch scope
- Verify effective permissions show correctly
- Verify the user list tab still works
- Test that system roles can't be deleted
- Test that the last Superadmin can't be removed

- [ ] **Step 4: Commit**

```bash
git add web/src/views/RolesPermisosView.vue web/src/views/ConfiguracionView.vue
git commit -m "feat(frontend): add roles and permissions admin view

Two-panel layout: role editor with per-module checkboxes, user assignment
with ranch scope selector. System roles protected from deletion. Effective
permissions preview."
```

---

### Task 10: Integration Testing and Cleanup

**Files:**
- Modify: various test files for final verification
- Remove: `DeleteAllowedUserIDs` from `Handlers` struct (after confirming tables exist in test env)

**Interfaces:**
- Consumes: everything from Tasks 1-9
- Produces: fully integrated, tested permission system

- [ ] **Step 1: Run full backend test suite**

Run: `go test ./internal/... -v -count=1`
Expected: all tests PASS

- [ ] **Step 2: Run full frontend build**

Run: `cd web && npm run build`
Expected: no TypeScript errors, build succeeds

- [ ] **Step 3: Manual integration test checklist**

With dev server running and permission tables created:
- [ ] First startup: verify bootstrap log "usuario asignado como Superadmin"
- [ ] `GET /api/v1/auth/permisos` returns full permissions for Superadmin
- [ ] Create a new "Operador" user, assign Operador role for ranch X only
- [ ] Login as Operador: only see productions from ranch X
- [ ] Operador cannot see Configuración in nav
- [ ] Operador cannot delete monitoring (button disabled)
- [ ] Add `monitoreo.eliminar` as direct permission for ranch X to Operador
- [ ] Operador can now delete monitoring in ranch X only
- [ ] Remove Superadmin from self: should be rejected (last Superadmin)
- [ ] Without permission tables: all features work as before (permissive mode)

- [ ] **Step 4: Verify the `AUTH_DELETE_ALLOWED_USER_IDS` transition**

- With tables NOT created + env var set: `RequireUserIn` still works
- With tables created + env var set: log warns, permissions table takes precedence
- With tables created + env var empty: permissions table is the only gate

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat(auth): complete IAM permission system with roles, ranch scoping, and UI

Replaces RequireAdmin and RequireUserIn with RequirePermission.
18 permissions across 6 modules, 3 seed roles, per-ranch scoping,
direct permission overrides, graceful degradation, and admin UI."
```
