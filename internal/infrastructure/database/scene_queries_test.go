package database

import (
	"strings"
	"testing"
)

// Las consultas se arman con marcadores según exista la columna image_bbox.
// El fallo clásico de ese enfoque es que la lista de columnas y la de
// placeholders dejen de cuadrar, y MySQL sólo lo diría en tiempo de ejecución.
func TestSceneUpsertPlaceholdersMatchColumns(t *testing.T) {
	for _, withBBox := range []bool{false, true} {
		name := "sin image_bbox"
		if withBBox {
			name = "con image_bbox"
		}

		t.Run(name, func(t *testing.T) {
			q := buildSceneUpsert(withBBox)

			if strings.Contains(q, "IMG_") {
				t.Fatalf("quedaron marcadores sin sustituir:\n%s", q)
			}

			cols := countColumns(t, q)
			placeholders := countPlaceholders(t, q)

			if cols != placeholders {
				t.Errorf("columnas = %d, placeholders = %d (deben coincidir)\n%s", cols, placeholders, q)
			}

			// 21 campos base + fecha_creacion + fecha_actualizacion, más
			// image_bbox cuando la columna existe.
			want := 23
			if withBBox {
				want = 24
			}
			if cols != want {
				t.Errorf("columnas = %d, want %d", cols, want)
			}

			if got := strings.Contains(q, "image_bbox"); got != withBBox {
				t.Errorf("image_bbox presente = %v, want %v", got, withBBox)
			}
		})
	}
}

func TestSceneSelectColsTracksImageBBox(t *testing.T) {
	for _, withBBox := range []bool{false, true} {
		q := buildSceneSelectCols(withBBox)

		if strings.Contains(q, "IMG_") {
			t.Errorf("quedaron marcadores sin sustituir:\n%s", q)
		}
		if got := strings.Contains(q, "e.image_bbox"); got != withBBox {
			t.Errorf("withBBox=%v: image_bbox presente = %v", withBBox, got)
		}
	}
}

// countColumns cuenta los nombres listados entre "INSERT INTO tabla (" y ")".
func countColumns(t *testing.T, q string) int {
	t.Helper()
	open := strings.Index(q, "(")
	close := strings.Index(q, ")")
	if open < 0 || close < open {
		t.Fatalf("no se encontró la lista de columnas:\n%s", q)
	}
	return len(splitNonEmpty(q[open+1 : close]))
}

// countPlaceholders cuenta los "?" de la cláusula VALUES.
func countPlaceholders(t *testing.T, q string) int {
	t.Helper()
	start := strings.Index(q, "VALUES (")
	if start < 0 {
		t.Fatalf("no se encontró VALUES:\n%s", q)
	}
	rest := q[start+len("VALUES ("):]
	end := strings.Index(rest, ")")
	if end < 0 {
		t.Fatalf("VALUES sin cerrar:\n%s", q)
	}
	return strings.Count(rest[:end], "?")
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return out
}
