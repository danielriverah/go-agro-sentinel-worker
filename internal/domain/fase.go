package domain

// FaseCultivo es un tramo del ciclo de una producción, medido en días desde
// fecha_plantacion.
//
// Cuelga de producciones.produccion_id (la maestra del ERP), no del monitoreo:
// así sobrevive al borrado del monitoreo y queda como base para predecir
// ciclos futuros del mismo cultivo.
type FaseCultivo struct {
	ID           int64  `json:"id"`
	ProduccionID int64  `json:"produccion_id"`
	Nombre       string `json:"nombre"`
	DiaInicio    int    `json:"dia_inicio"`
	DiaFin       int    `json:"dia_fin"`
	Orden        int    `json:"orden"`
}

// Validate comprueba que la fase sea utilizable para pintar un tramo del ciclo.
func (f FaseCultivo) Validate() error {
	if f.Nombre == "" {
		return &ProcessingError{Type: ErrValidation, Message: "la fase necesita un nombre"}
	}
	if f.DiaInicio < 0 {
		return &ProcessingError{Type: ErrValidation, Message: "dia_inicio no puede ser negativo"}
	}
	if f.DiaFin <= f.DiaInicio {
		return &ProcessingError{Type: ErrValidation, Message: "dia_fin debe ser mayor que dia_inicio"}
	}
	return nil
}
