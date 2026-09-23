package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"agro-sentinel-worker/internal/daemon"
	"agro-sentinel-worker/internal/infrastructure/database"
)

// S3PrefixDeleter borra todos los objetos bajo un prefijo.
type S3PrefixDeleter interface {
	DeletePrefix(ctx context.Context, bucket, prefix string) (int, error)
}

// DynamoDeleter borra los ítems de una producción en DynamoDB.
type DynamoDeleter interface {
	DeleteProduccion(ctx context.Context, tableName string, produccionID int64, folio string) error
	DeleteEscenas(ctx context.Context, tableName string, produccionID int64) (int, error)
}

// MonitoreoDeleter borra el monitoreo en MySQL y apaga producciones.monitoring.
type MonitoreoDeleter interface {
	EliminarMonitoreo(ctx context.Context, monitoringID uint, produccionID int64) (database.ResumenBorradoMySQL, error)
	HasImageBBox() bool
	CheckDependents(ctx context.Context, monitoringID uint) error
}

type eliminarMonitoreoRequest struct {
	// Folio que el usuario teclea para confirmar. Un "¿estás seguro?" no basta
	// para algo irreversible en tres sistemas.
	Folio string `json:"folio"`
}

type eliminarMonitoreoResponse struct {
	ProduccionID  int64                        `json:"produccion_id"`
	Folio         string                       `json:"folio"`
	S3Objetos     int                          `json:"s3_objetos_borrados"`
	DynamoEscenas int                          `json:"dynamodb_escenas_borradas"`
	MySQL         database.ResumenBorradoMySQL `json:"mysql"`
	Advertencia   string                       `json:"advertencia,omitempty"`
}

// EliminarMonitoreo maneja DELETE /api/v1/producciones/{id}/monitoreo.
//
// Borra el monitoreo en MySQL, S3 y DynamoDB, conserva la fila de producciones
// y sus fases de cultivo, y apaga producciones.monitoring para que el sync no
// lo recree.
//
// Requiere permiso 'monitoreo.eliminar' asignado a través de roles o permisos
// directos en la tabla de permisos. El permiso se valida a nivel del rancho
// (centro_costo_id) de la producción.
//
// Orden: desvincular + S3 -> DynamoDB -> MySQL. No puede ser atómico entre tres
// sistemas, así que se elige el orden cuyo fallo intermedio es menos dañino:
// registros visibles y reintentables antes que archivos huérfanos invisibles.
func (h *Handlers) EliminarMonitoreo(w http.ResponseWriter, r *http.Request) {
	prod, ok := h.produccionDesdeRuta(w, r)
	if !ok {
		return
	}

	// Verificar permiso monitoreo.eliminar para esta producción (per-rancho)
	if !h.verificarPermisoEliminar(w, r, prod.MonitoringID) {
		return
	}

	var req eliminarMonitoreoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		Error(w, http.StatusBadRequest, "body JSON inválido")
		return
	}
	if strings.TrimSpace(req.Folio) != strings.TrimSpace(prod.Folio) {
		Error(w, http.StatusBadRequest, "el folio de confirmación no coincide con el de la producción")
		return
	}

	// Sólo se borra el monitoreo de un lote ya cerrado. bloqueado lo deriva un
	// trigger de posible_cosecha, así que exigirlo equivale a pedir que la
	// producción esté marcada como cosechada: borrar una en seguimiento activo
	// tiraría datos que todavía se están usando.
	if !prod.Bloqueado {
		Error(w, http.StatusConflict,
			"sólo se puede eliminar el monitoreo de una producción bloqueada; márcala como posible cosecha primero")
		return
	}

	// Sin image_bbox no hay dónde preservar la georreferencia de las escenas
	// que dependen de ésta, y quedarían mal ubicadas en el mapa para siempre.
	if !h.Monitoreo.HasImageBBox() {
		Error(w, http.StatusServiceUnavailable,
			"falta la columna image_bbox; aplica scripts/phase3-add-image-bbox.sql antes de borrar")
		return
	}

	// El lock impide que el worker siga subiendo archivos durante el borrado
	// (volvería a dejar huérfanos) y también rechaza si el --auto global corre.
	lock, err := daemon.AcquireProduction(prod.ProduccionID)
	if err != nil {
		Error(w, http.StatusConflict, "no se puede borrar ahora: "+err.Error())
		return
	}
	if lock == nil {
		Error(w, http.StatusConflict, "la producción se está procesando en este momento; inténtalo al terminar")
		return
	}
	defer lock.Release()

	ctx := r.Context()
	if err := h.Monitoreo.CheckDependents(ctx, prod.ID); err != nil {
		Error(w, http.StatusConflict, err.Error())
		return
	}
	resp := eliminarMonitoreoResponse{ProduccionID: prod.ProduccionID, Folio: prod.Folio}

	// 1. S3 primero: un fallo posterior deja registros visibles y reintentables.
	prefix := strings.TrimRight(prod.Prefix, "/")
	if prefix == "" {
		Error(w, http.StatusInternalServerError, "la producción no tiene prefijo S3; no se puede borrar con seguridad")
		return
	}
	borrados, err := h.S3Delete.DeletePrefix(ctx, h.S3Bucket, prefix+"/")
	if err != nil {
		h.Log.Error("borrando objetos S3", "produccion_id", prod.ProduccionID, "error", err)
		Error(w, http.StatusInternalServerError, "no se pudieron borrar los archivos de S3: "+err.Error())
		return
	}
	resp.S3Objetos = borrados

	// 2. DynamoDB: escenas y luego la producción.
	escenas, err := h.DynamoDelete.DeleteEscenas(ctx, h.DynamoTablaEscenas, prod.ProduccionID)
	if err != nil {
		h.Log.Error("borrando escenas en DynamoDB", "produccion_id", prod.ProduccionID, "error", err)
		Error(w, http.StatusInternalServerError, "archivos borrados, pero falló DynamoDB: "+err.Error())
		return
	}
	resp.DynamoEscenas = escenas

	if err := h.DynamoDelete.DeleteProduccion(ctx, h.DynamoTablaProducciones, prod.ProduccionID, prod.Folio); err != nil {
		h.Log.Error("borrando produccion en DynamoDB", "produccion_id", prod.ProduccionID, "error", err)
		Error(w, http.StatusInternalServerError, "archivos y escenas borrados, pero falló DynamoDB: "+err.Error())
		return
	}

	// 3. MySQL al final, en una transacción.
	resumen, err := h.Monitoreo.EliminarMonitoreo(ctx, prod.ID, prod.ProduccionID)
	if err != nil {
		h.Log.Error("borrando monitoreo en MySQL", "produccion_id", prod.ProduccionID, "error", err)
		Error(w, http.StatusInternalServerError,
			"S3 y DynamoDB quedaron limpios, pero falló MySQL; reintenta el borrado: "+err.Error())
		return
	}
	resp.MySQL = resumen

	if resumen.Dependientes > 0 {
		resp.Advertencia = "se desvincularon escenas de otras producciones que reutilizaban este multiband; conservan su georreferencia"
	}

	h.Log.Info("monitoreo eliminado",
		"produccion_id", prod.ProduccionID, "folio", prod.Folio,
		"s3_objetos", resp.S3Objetos, "dynamo_escenas", resp.DynamoEscenas,
		"mysql_escenas", resumen.Escenas, "dependientes", resumen.Dependientes)

	JSON(w, http.StatusOK, resp)
}

// verificarPermisoEliminar valida que el usuario tiene el permiso 'monitoreo.eliminar'
// para el rancho (centro_costo_id) de la producción. Retorna false si:
// - La tabla de permisos no existe (modo permisivo)
// - El usuario no tiene el permiso asignado
func (h *Handlers) verificarPermisoEliminar(w http.ResponseWriter, r *http.Request, monitoringID uint) bool {
	// Si los permisos no están disponibles, modo permisivo (backward compat)
	if h.Permisos == nil || !h.Permisos.Disponible() {
		return true
	}

	// Obtener claims del usuario
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		Error(w, http.StatusUnauthorized, "autenticación requerida")
		return false
	}

	// Resolver el centro_costo_id de la producción
	ccID, err := h.Productions.GetCentroCostoByMonitoringID(r.Context(), monitoringID)
	if err != nil {
		h.Log.Error("resolving centro_costo for monitoring", "monitoring_id", monitoringID, "error", err)
		Error(w, http.StatusInternalServerError, "error verificando permisos")
		return false
	}

	// Cargar permisos del usuario
	perms, err := h.Permisos.CargarPermisos(r.Context(), claims.UserID)
	if err != nil {
		h.Log.Error("loading permissions", "usuario_id", claims.UserID, "error", err)
		Error(w, http.StatusInternalServerError, "error verificando permisos")
		return false
	}

	// Verificar que tiene el permiso monitoreo.eliminar para este rancho
	if !perms.TienePermiso("monitoreo.eliminar", ccID) {
		Error(w, http.StatusForbidden,
			"no tienes el permiso 'monitoreo.eliminar' para este rancho; pídele a un administrador que lo asigne")
		return false
	}

	return true
}
