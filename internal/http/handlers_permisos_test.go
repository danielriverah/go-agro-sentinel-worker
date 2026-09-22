package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"agro-sentinel-worker/internal/auth"
	"agro-sentinel-worker/internal/infrastructure/database"
)

// mockPermissionRepo implements PermissionRepository for handler tests.
type mockPermissionRepo struct {
	disponible         bool
	permisos           *auth.PermisosUsuario
	cargarPermisosErr  error
	roles              []database.RolConPermisos
	listRolesErr       error
	asignaciones       *database.UsuarioAsignaciones
	setUsuarioRolesErr error
	centrosCostos      []database.CentroCostoItem
	catalogoPermisos   []database.PermisoCatalogo
	invalidadoUID      int64
}

var _ PermissionRepository = (*mockPermissionRepo)(nil)

func (m *mockPermissionRepo) Disponible() bool { return m.disponible }
func (m *mockPermissionRepo) ListPermisos(_ context.Context) ([]database.PermisoCatalogo, error) {
	return m.catalogoPermisos, nil
}
func (m *mockPermissionRepo) ListRoles(_ context.Context) ([]database.RolConPermisos, error) {
	return m.roles, m.listRolesErr
}
func (m *mockPermissionRepo) CreateRol(_ context.Context, nombre, descripcion string, permisoIDs []int) (*database.RolConPermisos, error) {
	return &database.RolConPermisos{ID: 99, Nombre: nombre, Descripcion: descripcion, PermisoIDs: permisoIDs}, nil
}
func (m *mockPermissionRepo) UpdateRol(_ context.Context, _ int, _, _ string, _ []int) error {
	return nil
}
func (m *mockPermissionRepo) DeleteRol(_ context.Context, rolID int) error {
	if rolID == 1 {
		return auth.ErrRolEsSistema
	}
	return nil
}
func (m *mockPermissionRepo) GetUsuarioAsignaciones(_ context.Context, _ int64) (*database.UsuarioAsignaciones, error) {
	return m.asignaciones, nil
}
func (m *mockPermissionRepo) SetUsuarioRoles(_ context.Context, _ int64, _ []database.AsignacionRolInput, _ int64) error {
	return m.setUsuarioRolesErr
}
func (m *mockPermissionRepo) SetUsuarioPermisos(_ context.Context, _ int64, _ []database.AsignacionPermisoInput, _ int64) error {
	return nil
}
func (m *mockPermissionRepo) ListCentrosCostos(_ context.Context) ([]database.CentroCostoItem, error) {
	return m.centrosCostos, nil
}
func (m *mockPermissionRepo) CargarPermisos(_ context.Context, uid int64) (*auth.PermisosUsuario, error) {
	if m.cargarPermisosErr != nil {
		return nil, m.cargarPermisosErr
	}
	if m.permisos != nil {
		return m.permisos, nil
	}
	return &auth.PermisosUsuario{UsuarioID: uid}, nil
}
func (m *mockPermissionRepo) InvalidarCache(uid int64) { m.invalidadoUID = uid }

func permisosHandlers(repo *mockPermissionRepo) *Handlers {
	return &Handlers{PermisosRepo: repo}
}

func reqConClaims(method, url string, userID int64) *http.Request {
	req := httptest.NewRequest(method, url, nil)
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{UserID: userID, Username: "testuser"})
	return req.WithContext(ctx)
}

func TestMisPermisosDevuelveDegradedSinTablas(t *testing.T) {
	h := permisosHandlers(&mockPermissionRepo{disponible: false})
	w := httptest.NewRecorder()
	req := reqConClaims("GET", "/api/v1/auth/permisos", 1)

	h.MisPermisos(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env struct{ Data auth.PermisosResponse `json:"data"` }
	json.NewDecoder(w.Body).Decode(&env)
	if !env.Data.Degraded {
		t.Error("expected degraded=true")
	}
}

func TestMisPermisosDevuelvePermisosEfectivos(t *testing.T) {
	cc42 := int64(42)
	h := permisosHandlers(&mockPermissionRepo{
		disponible: true,
		permisos: &auth.PermisosUsuario{
			UsuarioID: 1,
			Permisos: []auth.AsignacionPermiso{
				{Clave: "usuarios.ver", CentroCostoID: nil},
				{Clave: "producciones.ver", CentroCostoID: &cc42},
			},
		},
	})
	w := httptest.NewRecorder()
	req := reqConClaims("GET", "/api/v1/auth/permisos", 1)

	h.MisPermisos(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var env struct{ Data auth.PermisosResponse `json:"data"` }
	json.NewDecoder(w.Body).Decode(&env)
	if env.Data.Degraded {
		t.Error("should not be degraded")
	}
	if len(env.Data.Global) == 0 {
		t.Error("expected global permissions")
	}
}

func TestMisPermisosSinSesionDevuelve401(t *testing.T) {
	h := permisosHandlers(&mockPermissionRepo{disponible: true})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/auth/permisos", nil)

	h.MisPermisos(w, req)

	if w.Code != 401 {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSetUsuarioRolesRechazaAutoModificacion(t *testing.T) {
	h := permisosHandlers(&mockPermissionRepo{disponible: true})
	w := httptest.NewRecorder()
	req := reqConClaims("PUT", "/api/v1/admin/usuarios/1/roles", 1)
	req.SetPathValue("id", "1")

	h.SetUsuarioRoles(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 for self-modification, got %d", w.Code)
	}
}

func TestDeleteRolRechazaRolSistema(t *testing.T) {
	h := permisosHandlers(&mockPermissionRepo{disponible: true})
	w := httptest.NewRecorder()
	req := reqConClaims("DELETE", "/api/v1/admin/roles/1", 1)
	req.SetPathValue("id", "1")

	h.DeleteRol(w, req)

	if w.Code != 403 {
		t.Fatalf("expected 403 for system role, got %d", w.Code)
	}
}

func TestListRolesDevuelve503SinTablas(t *testing.T) {
	h := permisosHandlers(&mockPermissionRepo{disponible: false})
	w := httptest.NewRecorder()
	req := reqConClaims("GET", "/api/v1/admin/roles", 1)

	h.ListRoles(w, req)

	if w.Code != 503 {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}
