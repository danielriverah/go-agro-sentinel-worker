package domain

import (
	"encoding/json"
	"errors"
	"time"
)

// BBox is an axis-aligned bounding box (lon/lat).
type BBox struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

// MarshalJSONColumn serializa el bbox al formato que usan las columnas
// tile_bbox e image_bbox, de modo que ParseTileBBox pueda releerlo.
func (b BBox) MarshalJSONColumn() json.RawMessage {
	raw, err := json.Marshal(struct {
		MinLon float64 `json:"min_lon"`
		MinLat float64 `json:"min_lat"`
		MaxLon float64 `json:"max_lon"`
		MaxLat float64 `json:"max_lat"`
	}{b.MinX, b.MinY, b.MaxX, b.MaxY})
	if err != nil {
		return nil
	}
	return raw
}

func (b BBox) Validate() error {
	if b.MinX >= b.MaxX {
		return errors.New("bbox: MinX must be less than MaxX")
	}
	if b.MinY >= b.MaxY {
		return errors.New("bbox: MinY must be less than MaxY")
	}
	return nil
}

type pboxShape struct {
	MinLon float64   `json:"min_lon"`
	MinLat float64   `json:"min_lat"`
	MaxLon float64   `json:"max_lon"`
	MaxLat float64   `json:"max_lat"`
	PBox   []float64 `json:"pbox"`
}

// Production maps to s3_monitoring_producciones.
// ArticuloID and CentroCostoID are enriched from ERP tables in memory only —
// they are NOT columns of s3_monitoring_producciones.
type Production struct {
	ID                   uint            // s3_monitoring_produccion_id
	ProduccionID         int64           // produccion_id (FK → producciones, UNIQUE)
	Cosecha              string          // cosecha
	Vaiedades            string          // vaiedades (typo preserved from real schema)
	Prefix               string          // prefix
	Monitoring           bool            // monitoring
	MaxDiasMonitoring    int             // max_dias_monitoring
	FechaFin             *time.Time      // fecha_fin
	FechaPlantacion      *time.Time      // fecha_plantacion
	PBoxJSON             json.RawMessage // pbox (JSON)
	PolygonBBoxJSON      json.RawMessage // polygon_bbox (JSON)
	TileBBoxJSON         json.RawMessage // tile_bbox (JSON)
	TileCenterLat        *float64        // tile_center_lat
	TileCenterLon        *float64        // tile_center_lon
	TileEdgeMeters       uint            // tile_edge_meters
	Fase2CompletaAt      *time.Time      // fase2_completa_at
	PoligonoJSON         json.RawMessage // poligono (JSON)
	TifCompleteAt        *time.Time      // tif_complete_at
	IaCompleteAt         *time.Time      // ia_complete_at
	IAuto                bool            // ia_auto
	PosibleCosecha       bool            // posible_cosecha
	Bloqueado            bool            // bloqueado (column to be added to table)
	UltimaSincronizacion *time.Time      // ultima_sincronizacion
	FechaCreacion        time.Time       // fecha_creacion
	FechaActualizacion   *time.Time      // fecha_actualizacion

	// In-memory enrichment from ERP — not persisted in s3_monitoring_producciones.
	ArticuloID    int64
	CentroCostoID int64
	Rancho        string // centros_costos.nombre
	Folio         string // producciones.folio
}

// ProductionStats holds scene-level aggregates computed from s3_monitoring_escenas.
type ProductionStats struct {
	MonitoringProduccionID uint       `json:"monitoring_produccion_id"`
	ScenasTotal            int        `json:"scenes_total"`
	ScenasUsables          int        `json:"scenes_usables"`
	ScenasCompletadas      int        `json:"scenes_completadas"`
	ScenasPendientes       int        `json:"scenes_pendientes"`
	ScenasTifOk            int        `json:"scenes_tif_ok"`
	ScenasIaOk             int        `json:"scenes_ia_ok"`
	LastTifFecha           *time.Time `json:"last_tif_fecha"`
	LastIaFecha            *time.Time `json:"last_ia_fecha"`
}

// ParsePBox parses the pbox JSON column and returns a BBox.
// Returns nil when pbox is empty or cannot be parsed.
func (p *Production) ParsePBox() *BBox {
	if len(p.PBoxJSON) == 0 {
		return nil
	}
	var pb pboxShape
	if err := json.Unmarshal(p.PBoxJSON, &pb); err != nil {
		return nil
	}
	if len(pb.PBox) == 4 {
		return &BBox{MinX: pb.PBox[0], MinY: pb.PBox[1], MaxX: pb.PBox[2], MaxY: pb.PBox[3]}
	}
	if pb.MaxLon != 0 {
		return &BBox{MinX: pb.MinLon, MinY: pb.MinLat, MaxX: pb.MaxLon, MaxY: pb.MaxLat}
	}
	return nil
}

// ParseTileBBox parses the tile_bbox JSON column and returns a BBox.
// tile_bbox is the fixed-size square used as the download window for COG bands.
// Returns nil when the field is empty or cannot be parsed.
func (p *Production) ParseTileBBox() *BBox {
	if len(p.TileBBoxJSON) == 0 {
		return nil
	}
	var pb pboxShape
	if err := json.Unmarshal(p.TileBBoxJSON, &pb); err != nil {
		return nil
	}
	if len(pb.PBox) == 4 {
		return &BBox{MinX: pb.PBox[0], MinY: pb.PBox[1], MaxX: pb.PBox[2], MaxY: pb.PBox[3]}
	}
	if pb.MaxLon != 0 {
		return &BBox{MinX: pb.MinLon, MinY: pb.MinLat, MaxX: pb.MaxLon, MaxY: pb.MaxLat}
	}
	return nil
}

// ParsePolygonBBox parses the polygon_bbox JSON column and returns a BBox.
// polygon_bbox is the tight bounding box around the production's field polygon.
// Returns nil when the field is empty or cannot be parsed.
func (p *Production) ParsePolygonBBox() *BBox {
	if len(p.PolygonBBoxJSON) == 0 {
		return nil
	}
	var pb pboxShape
	if err := json.Unmarshal(p.PolygonBBoxJSON, &pb); err != nil {
		return nil
	}
	if len(pb.PBox) == 4 {
		return &BBox{MinX: pb.PBox[0], MinY: pb.PBox[1], MaxX: pb.PBox[2], MaxY: pb.PBox[3]}
	}
	if pb.MaxLon != 0 {
		return &BBox{MinX: pb.MinLon, MinY: pb.MinLat, MaxX: pb.MaxLon, MaxY: pb.MaxLat}
	}
	return nil
}

// ShouldProcess returns true when the production is actively monitored and not blocked.
func (p *Production) ShouldProcess() bool {
	return p.Monitoring && !p.Bloqueado
}

// HasRequiredFields returns true when the production has all spatial fields
// required to generate scene files (pbox, tile_bbox, poligono, fecha_plantacion).
// Without these fields the processing worker cannot operate on the production's scenes.
func (p *Production) HasRequiredFields() bool {
	return len(p.PBoxJSON) > 0 &&
		len(p.TileBBoxJSON) > 0 &&
		len(p.PoligonoJSON) > 0 &&
		p.FechaPlantacion != nil
}

// Validate verifies that the critical fields of Production are valid.
func (p *Production) Validate() error {
	if p.ProduccionID <= 0 {
		return errors.New("Production: ProduccionID must be greater than 0")
	}
	return nil
}
