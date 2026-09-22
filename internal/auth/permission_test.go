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
