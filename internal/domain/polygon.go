package domain

import (
	"errors"
	"fmt"
	"math"
)

// MinVertexSeparationMeters is the smallest allowed distance between two
// consecutive vertices. Anything closer is a hand-drawn closing artifact that
// GEOS reports as a self-intersection, and is well below the 10 m resolution
// of a Sentinel-2 pixel anyway.
const MinVertexSeparationMeters = 5.0

// metersPerDegreeLat is accurate enough for the separation check at any
// latitude the monitored productions occupy.
const metersPerDegreeLat = 110574.0

// Ring is a polygon exterior ring as [[lon,lat],...], the order used in
// Production.PoligonoJSON. It is not required to be closed — the last point
// is implicitly joined back to the first.
type Ring [][2]float64

// SegmentCrossing reports two ring sides that intersect each other.
type SegmentCrossing struct {
	SideA int     `json:"side_a"` // index of the first vertex of side A
	SideB int     `json:"side_b"`
	Lon   float64 `json:"lon"`
	Lat   float64 `json:"lat"`
}

func (c SegmentCrossing) Error() string {
	return fmt.Sprintf("los lados %d y %d se cruzan en %.8f, %.8f",
		c.SideA+1, c.SideB+1, c.Lon, c.Lat)
}

// Validate checks that the ring is a simple polygon: at least 3 vertices, no
// near-duplicate consecutive vertices, and no self-intersections. GDAL refuses
// invalid polygons as a -cutline, which fails the whole scene.
func (r Ring) Validate() error {
	pts := r.open()
	if len(pts) < 3 {
		return errors.New("el polígono necesita al menos 3 vértices")
	}

	for i := range pts {
		j := (i + 1) % len(pts)
		if d := distanceMeters(pts[i], pts[j]); d < MinVertexSeparationMeters {
			return fmt.Errorf("los vértices %d y %d están a %.1f m — deben estar separados al menos %.0f m",
				i+1, j+1, d, MinVertexSeparationMeters)
		}
	}

	if c := pts.crossing(); c != nil {
		return *c
	}
	return nil
}

// Crossing returns the first pair of intersecting sides, or nil when the ring
// is simple. Exposed so the API can report the exact point to the editor.
func (r Ring) Crossing() *SegmentCrossing {
	return r.open().crossing()
}

// open drops a closing point equal to the first, so sides can be walked with
// a plain modulo without producing a zero-length segment.
func (r Ring) open() Ring {
	if n := len(r); n > 1 && r[0] == r[n-1] {
		return r[:n-1]
	}
	return r
}

func (r Ring) crossing() *SegmentCrossing {
	n := len(r)
	if n < 4 {
		return nil // a triangle cannot self-intersect
	}
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			// Adjacent sides share a vertex by construction; the pair
			// (0, n-1) is adjacent through the closing side.
			if j == i+1 || (i == 0 && j == n-1) {
				continue
			}
			p1, p2 := r[i], r[(i+1)%n]
			q1, q2 := r[j], r[(j+1)%n]
			if lon, lat, ok := segmentIntersection(p1, p2, q1, q2); ok {
				return &SegmentCrossing{SideA: i, SideB: j, Lon: lon, Lat: lat}
			}
		}
	}
	return nil
}

// SignedArea returns twice the signed area of the ring (the shoelace sum).
// Positive means counter-clockwise, which is what the GeoJSON spec requires
// for an exterior ring.
func (r Ring) SignedArea() float64 {
	pts := r.open()
	var sum float64
	for i := range pts {
		j := (i + 1) % len(pts)
		sum += pts[i][0]*pts[j][1] - pts[j][0]*pts[i][1]
	}
	return sum
}

// IsCounterClockwise reports whether the ring winds the way GeoJSON expects.
func (r Ring) IsCounterClockwise() bool { return r.SignedArea() > 0 }

// Normalized returns the ring wound counter-clockwise, reversing it when
// needed so stored polygons always follow the GeoJSON right-hand rule.
func (r Ring) Normalized() Ring {
	pts := r.open()
	if pts.IsCounterClockwise() {
		out := make(Ring, len(pts))
		copy(out, pts)
		return out
	}
	out := make(Ring, len(pts))
	for i := range pts {
		out[i] = pts[len(pts)-1-i]
	}
	return out
}

// BBox returns the tight bounding box of the ring.
func (r Ring) BBox() *BBox {
	if len(r) == 0 {
		return nil
	}
	b := &BBox{MinX: r[0][0], MinY: r[0][1], MaxX: r[0][0], MaxY: r[0][1]}
	for _, p := range r[1:] {
		b.MinX = math.Min(b.MinX, p[0])
		b.MaxX = math.Max(b.MaxX, p[0])
		b.MinY = math.Min(b.MinY, p[1])
		b.MaxY = math.Max(b.MaxY, p[1])
	}
	return b
}

// WithinBBox reports whether every vertex falls inside b. Used to keep an
// edited polygon inside the already-downloaded tile, so existing multiband
// rasters stay usable.
func (r Ring) WithinBBox(b *BBox) bool {
	if b == nil {
		return true
	}
	for _, p := range r {
		if p[0] < b.MinX || p[0] > b.MaxX || p[1] < b.MinY || p[1] > b.MaxY {
			return false
		}
	}
	return true
}

// AreaHectares approximates the ring area, projecting degrees to meters with
// the cosine of the mean latitude. Accurate to well under a percent at the
// size of a production plot.
func (r Ring) AreaHectares() float64 {
	pts := r.open()
	if len(pts) < 3 {
		return 0
	}
	var latSum float64
	for _, p := range pts {
		latSum += p[1]
	}
	meanLat := latSum / float64(len(pts))
	mx := metersPerDegreeLat * math.Cos(meanLat*math.Pi/180)
	my := metersPerDegreeLat

	var sum float64
	for i := range pts {
		j := (i + 1) % len(pts)
		sum += (pts[i][0]*mx)*(pts[j][1]*my) - (pts[j][0]*mx)*(pts[i][1]*my)
	}
	return math.Abs(sum) / 2 / 10000
}

func distanceMeters(a, b [2]float64) float64 {
	meanLat := (a[1] + b[1]) / 2
	dx := (b[0] - a[0]) * metersPerDegreeLat * math.Cos(meanLat*math.Pi/180)
	dy := (b[1] - a[1]) * metersPerDegreeLat
	return math.Hypot(dx, dy)
}

// segmentIntersection returns the crossing point of segments p1p2 and q1q2.
// Collinear overlap counts as a crossing: GEOS rejects it too.
func segmentIntersection(p1, p2, q1, q2 [2]float64) (lon, lat float64, ok bool) {
	rx, ry := p2[0]-p1[0], p2[1]-p1[1]
	sx, sy := q2[0]-q1[0], q2[1]-q1[1]

	denom := rx*sy - ry*sx
	qpx, qpy := q1[0]-p1[0], q1[1]-p1[1]

	if denom == 0 {
		// Parallel; only an overlap along the same line is a problem.
		if qpx*ry-qpy*rx != 0 {
			return 0, 0, false
		}
		rr := rx*rx + ry*ry
		if rr == 0 {
			return 0, 0, false
		}
		t0 := (qpx*rx + qpy*ry) / rr
		t1 := t0 + (sx*rx+sy*ry)/rr
		if math.Min(t0, t1) > 1 || math.Max(t0, t1) < 0 {
			return 0, 0, false
		}
		return q1[0], q1[1], true
	}

	t := (qpx*sy - qpy*sx) / denom
	u := (qpx*ry - qpy*rx) / denom
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return 0, 0, false
	}
	return p1[0] + t*rx, p1[1] + t*ry, true
}
