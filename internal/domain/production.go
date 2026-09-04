package domain

import (
	"errors"
	"time"
)

type BBox struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
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

type Production struct {
	ID                  int64
	ProduccionID        int64
	ArticuloID          int64 // FK a articulos - Identificador del artículo/producto agrícola
	CentroCostoID       int64 // FK a centros_costos - Centro de costo para contabilidad
	Cultivo             string
	Ciclo               string
	NombreRancho        string // Nombre desnormalizado del rancho para búsquedas rápidas
	BBox                *BBox
	Monitoring          bool
	MonitoringMotivo    string
	Bloqueado           bool
	BloqueadoMotivo     string
	BloqueadoAt         *time.Time
	DesbloqueadoPor     string
	TargetResolution    int
	CloudCoverMax       float64
	FechaPlantacion     *time.Time
	DiasProduccion      int
	FechaFinMonitoreo   *time.Time
	TotalEscenas        int
	TotalEscenasValidas int
	LastSyncAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (p *Production) ShouldProcess() bool {
	return p.Monitoring && !p.Bloqueado && p.BBox != nil
}

// Validate verifies that the critical fields of Production are valid.
func (p *Production) Validate() error {
	if p.ProduccionID <= 0 {
		return errors.New("Production: ProduccionID must be greater than 0")
	}
	if p.Cultivo == "" {
		return errors.New("Production: Cultivo is required")
	}
	if p.Ciclo == "" {
		return errors.New("Production: Ciclo is required")
	}
	if p.BBox != nil {
		if err := p.BBox.Validate(); err != nil {
			return err
		}
	}
	return nil
}
