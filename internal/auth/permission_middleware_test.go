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
