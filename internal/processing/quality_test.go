package processing

import "testing"

func TestQualityThreshold(t *testing.T) {
	for _, tc := range []struct {
		name          string
		noData, cloud float64
		known, usable bool
		reason        string
	}{
		{"clear", 0, 0, true, true, ""},
		{"exactly15", 15, 0, true, true, ""},
		{"above15", 15.01, 0, true, false, "cobertura_insuficiente"},
		{"allMissing", 100, 0, true, false, "cobertura_insuficiente"},
		{"cloudy", 0, 16, true, false, "nubosidad_alta"},
		{"legacyUnknown", 0, 0, false, false, "calidad_no_verificada"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := &Quality{NoDataPct: tc.noData}
			q.Evaluate(tc.cloud, 15, tc.known)
			if q.Usable != tc.usable || q.Reason != tc.reason {
				t.Fatalf("got %+v", q)
			}
		})
	}
}

func TestParamsExcludeRejectedHistory(t *testing.T) {
	p := BuildParams(ParamsInput{Quality: &Quality{Usable: true}}, &Params{
		SceneID: "rejected", Quality: &Quality{Reason: "cobertura_insuficiente"},
		Historico: []HistoricoEntry{
			{SceneID: "good", Quality: &Quality{Usable: true}},
			{SceneID: "bad", Quality: &Quality{NoDataPct: 30}},
		},
	})
	if len(p.Historico) != 1 || p.Historico[0].SceneID != "good" {
		t.Fatalf("unexpected history: %+v", p.Historico)
	}
}
