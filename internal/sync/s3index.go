package sync

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"agro-sentinel-worker/internal/domain"
	"agro-sentinel-worker/internal/processing"
)

// cloudCoverUsableThreshold is the maximum cloud cover (%) under which a
// scene is considered usable. Mirrors the worker's cloudCoverThreshold.
const cloudCoverUsableThreshold = 15.0

// Tipos de archivo reconocidos en S3.
const (
	tipoTruthTif  = "truth_tif"
	tipoRenderTif = "render_tif"
	tipoParams    = "params"
	tipoIA        = "ia"
	tipoIAReq     = "ia_req"
	tipoSceneJSON = "scene_json"
	tipoImage     = "image"
)

// imageNames es el conjunto de PNGs de índices/composiciones que genera el sistema.
var imageNames = map[string]bool{
	"natural": true, "false_color": true, "false_color_veg": true,
	"red_edge": true, "swir": true,
	"ndvi": true, "ndre": true, "evi": true, "gndvi": true,
	"nbr": true, "ndmi": true, "savi": true,
}

// tipoFromKey determina el tipo de archivo a partir del nombre del objeto en S3.
// Devuelve ("", false) cuando el archivo no corresponde a ningún tipo conocido.
func tipoFromKey(key string) (tipo string, isJSON bool, ok bool) {
	base := path.Base(key)
	switch {
	case base == "multiband.tif":
		return tipoTruthTif, false, true
	case base == "render.tif":
		return tipoRenderTif, false, true
	case base == "multiband.params.json":
		return tipoParams, true, true
	case base == "multiband.ia_req.json":
		return tipoIAReq, true, true
	case base == "multiband.ia.json":
		return tipoIA, true, true
	}
	ext := strings.ToLower(path.Ext(base))
	name := strings.TrimSuffix(base, path.Ext(base))
	if ext == ".png" && imageNames[name] {
		return tipoImage, false, true
	}
	if ext == ".json" {
		return tipoSceneJSON, true, true
	}
	return "", false, false
}

// indexSceneFiles lista los objetos S3 bajo {prefix}/{sceneName}/ e indexa en
// s3_monitoring_escena_archivos los que aún no están registrados.
// Para archivos JSON también guarda el contenido en json_content.
// Al finalizar actualiza los flags *_exists de la escena.
func (s *Service) indexSceneFiles(ctx context.Context, prod *domain.Production, scene *domain.Scene) {
	prefix := fmt.Sprintf("%s/%s/", prod.Prefix, scene.SceneName)

	objects, err := s.s3.ListObjects(ctx, s.s3Bucket, prefix)
	if err != nil {
		s.logger.Warn("listing s3 objects failed",
			"produccion_id", prod.ProduccionID,
			"scene_name", scene.SceneName,
			"error", err)
		return
	}
	if len(objects) == 0 {
		return
	}

	var truthTif, renderTif, params, ia bool
	// JSON content for files newly indexed this cycle.
	var paramsJSON, iaJSON string

	for _, obj := range objects {
		tipo, isJSON, ok := tipoFromKey(obj.Key)
		if !ok {
			continue
		}

		hash := fmt.Sprintf("%x", md5.Sum([]byte(obj.Key)))

		exists, err := s.fileRepo.ExistsByKeyHash(ctx, scene.ID, hash)
		if err != nil {
			s.logger.Warn("checking file existence failed",
				"key", obj.Key, "error", err)
			continue
		}
		if exists {
			// Ya indexado — solo actualizar flags.
			switch tipo {
			case tipoTruthTif:
				truthTif = true
			case tipoRenderTif:
				renderTif = true
			case tipoParams:
				params = true
			case tipoIA:
				ia = true
			}
			continue
		}

		// Leer contenido para archivos JSON.
		var jsonContent string
		if isJSON {
			data, err := s.s3.GetObjectContent(ctx, s.s3Bucket, obj.Key)
			if err != nil {
				s.logger.Warn("reading json content failed",
					"key", obj.Key, "error", err)
			} else {
				jsonContent = string(data)
				switch tipo {
				case tipoParams:
					paramsJSON = jsonContent
				case tipoIA:
					iaJSON = jsonContent
				}
			}
		}

		lm := obj.LastModified
		sf := &domain.SceneFile{
			EscenaID:      scene.ID,
			Tipo:          tipo,
			S3Key:         obj.Key,
			S3KeyHash:     hash,
			S3Uri:         fmt.Sprintf("s3://%s/%s", s.s3Bucket, obj.Key),
			Extension:     strings.TrimPrefix(path.Ext(obj.Key), "."),
			SizeBytes:     obj.Size,
			LastModified:  &lm,
			Existe:        true,
			JsonContent:   jsonContent,
			FechaCreacion: time.Now().UTC(),
		}

		if err := s.fileRepo.Create(ctx, sf); err != nil {
			s.logger.Error("indexing scene file failed",
				"key", obj.Key, "tipo", tipo, "error", err)
			continue
		}

		s.logger.Info("scene file indexed",
			"produccion_id", prod.ProduccionID,
			"scene_name", scene.SceneName,
			"tipo", tipo,
			"key", obj.Key)

		switch tipo {
		case tipoTruthTif:
			truthTif = true
		case tipoRenderTif:
			renderTif = true
		case tipoParams:
			params = true
		case tipoIA:
			ia = true
		}
	}

	// Actualizar flags de la escena solo si alguno cambió respecto al estado actual.
	newTruth := scene.TruthTifExists || truthTif
	newRender := scene.RenderTifExists || renderTif
	newParams := scene.ParamsExists || params
	newIA := scene.IaExists || ia

	if newTruth != scene.TruthTifExists || newRender != scene.RenderTifExists ||
		newParams != scene.ParamsExists || newIA != scene.IaExists {
		if err := s.sceneRepo.UpdateExistsFlags(ctx, scene.ID, newTruth, newRender, newParams, newIA); err != nil {
			s.logger.Warn("updating scene exists flags failed",
				"scene_id", scene.ID, "error", err)
		}
	}

	// Si existe el multiband.tif → marcar la escena COMPLETED (el worker terminó).
	if newTruth && scene.Status != domain.StatusCompleted {
		if err := s.sceneRepo.UpdateStatus(ctx, scene.ID, domain.StatusCompleted); err != nil {
			s.logger.Warn("marking scene completed failed",
				"scene_id", scene.ID, "error", err)
		} else {
			s.logger.Info("scene marked COMPLETED (truth_tif found)",
				"produccion_id", prod.ProduccionID,
				"scene_name", scene.SceneName)
		}
	}

	// Si se indexó params.json → actualizar production_cloud, usable y analysis.
	s.updateSceneFromIndexedFiles(ctx, scene, paramsJSON)

	// Si se indexó ia.json → reconstruir s3_monitoring_escena_ia_resumen desde el JSON crudo.
	if iaJSON != "" {
		s.upsertIAResultFromJSON(ctx, scene, iaJSON)
	}
}

// updateSceneFromIndexedFiles parses a newly-indexed params.json and updates
// production_cloud, usable and analysis on the scene record.
func (s *Service) updateSceneFromIndexedFiles(ctx context.Context, scene *domain.Scene, paramsJSON string) {
	if paramsJSON == "" {
		return
	}

	var p processing.Params
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
		s.logger.Warn("parsing params.json for scene update failed",
			"scene_id", scene.ID, "error", err)
		return
	}

	cc := p.CloudCoverBBox
	productionCloud := &cc
	u := cc <= cloudCoverUsableThreshold
	if p.Quality != nil {
		u = p.Quality.Usable && p.Quality.NoDataPct <= processing.MaxNoDataPct
	}
	usable := &u
	analysis := &u

	if err := s.sceneRepo.UpdateFromParams(ctx, scene.ID, productionCloud, usable, analysis); err != nil {
		s.logger.Warn("updating scene from params failed",
			"scene_id", scene.ID, "error", err)
	}
}

// upsertIAResultFromJSON parses the raw Bedrock JSON stored in multiband.ia.json
// and upserts the result into s3_monitoring_escena_ia_resumen.
func (s *Service) upsertIAResultFromJSON(ctx context.Context, scene *domain.Scene, iaJSON string) {
	if s.iaRepo == nil {
		return
	}

	type riesgoObj struct {
		Nivel  string `json:"nivel"`
		Motivo string `json:"motivo"`
	}
	var raw struct {
		EstadoClave   string    `json:"estado_clave"`
		EstadoGeneral string    `json:"estado_general"`
		Riesgo        riesgoObj `json:"riesgo"`
	}
	if err := json.Unmarshal([]byte(iaJSON), &raw); err != nil {
		s.logger.Warn("parsing ia.json failed",
			"scene_id", scene.ID, "error", err)
		return
	}

	now := time.Now().UTC()
	result := &domain.IAResultSummary{
		S3MonitoringEscenaID: scene.ID,
		EstadoClave:          raw.EstadoClave,
		EstadoGeneral:        raw.EstadoGeneral,
		RiesgoNivel:          raw.Riesgo.Nivel,
		RiesgoMotivo:         raw.Riesgo.Motivo,
		FechaAnalisis:        &now,
		JSONOriginal:         iaJSON,
	}

	if err := s.iaRepo.Upsert(ctx, result); err != nil {
		s.logger.Warn("upserting ia result from s3 json failed",
			"scene_id", scene.ID, "error", err)
		return
	}
	s.logger.Info("ia result reconstructed from s3 json",
		"scene_id", scene.ID,
		"estado_clave", raw.EstadoClave)
}
