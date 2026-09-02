package domain

type Band string

const (
	BandB02 Band = "B02"
	BandB03 Band = "B03"
	BandB04 Band = "B04"
	BandB05 Band = "B05"
	BandB06 Band = "B06"
	BandB07 Band = "B07"
	BandB08 Band = "B08"
	BandB8A Band = "B8A"
	BandB11 Band = "B11"
	BandB12 Band = "B12"
	BandSCL Band = "SCL"
)

var bandResolutions = map[Band]int{
	BandB02: 10,
	BandB03: 10,
	BandB04: 10,
	BandB05: 20,
	BandB06: 20,
	BandB07: 20,
	BandB08: 10,
	BandB8A: 20,
	BandB11: 20,
	BandB12: 20,
	BandSCL: 20,
}

func (b Band) Resolution() int {
	return bandResolutions[b]
}

func AllSpectralBands() []Band {
	return []Band{BandB02, BandB03, BandB04, BandB05, BandB06, BandB07, BandB08, BandB8A, BandB11, BandB12}
}

func BandsAtResolution(resolution int) []Band {
	var result []Band
	for _, b := range AllSpectralBands() {
		if b.Resolution() == resolution {
			result = append(result, b)
		}
	}
	return result
}

type BandInfo struct {
	Name       Band
	Resolution int
	Href       string
}
