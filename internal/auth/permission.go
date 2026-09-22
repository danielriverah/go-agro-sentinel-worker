package auth

import "fmt"

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
	Global       []string            `json:"global"`
	PorRancho    map[string][]string `json:"por_rancho"`
	RanchosTodos bool                `json:"ranchos_todos"`
	Degraded     bool                `json:"degraded"`
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
