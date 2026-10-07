package contract

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

const (
	hopRadius     = 5.0 // C15.2: crossings this close to a segment end need no record
	crossMatchTol = 1.0 // C15.2: record to crossing point
)

// checkCrossings verifies C15: every Crossing record names a real segment
// and a parameter in [0,1] (C15.1), and every proper crossing between two
// edges is recorded on one of them (C15.2).
func (c *checker) checkCrossings() {
	for _, e := range c.pg.Edges {
		for k, cr := range e.Crossings {
			if cr.SegmentIndex < 0 || cr.SegmentIndex >= len(e.Points)-1 {
				c.add("C15.1", e.ID, "crossing %d names segment %d of a %d-point route", k, cr.SegmentIndex, len(e.Points))
			}
			if cr.T < 0 || cr.T > 1 {
				c.add("C15.1", e.ID, "crossing %d has t=%.4f outside [0,1]", k, cr.T)
			}
		}
	}
	es := c.pg.Edges
	for a := range es {
		for b := a + 1; b < len(es); b++ {
			for i := 0; i+1 < len(es[a].Points); i++ {
				a1, a2 := es[a].Points[i], es[a].Points[i+1]
				for j := 0; j+1 < len(es[b].Points); j++ {
					b1, b2 := es[b].Points[j], es[b].Points[j+1]
					p, ok := crossing(a1, a2, b1, b2)
					if !ok || nearAny(p, hopRadius, a1, a2, b1, b2) {
						continue
					}
					if !recorded(es[a], p) && !recorded(es[b], p) {
						c.add("C15.2", es[a].ID, "crossing with edge %s at (%.2f,%.2f) has no record", es[b].ID, p.X, p.Y)
					}
				}
			}
		}
	}
}

// crossing returns where segments a1-a2 and b1-b2 cross at a point interior
// to both. Parallel and collinear segments do not cross.
func crossing(a1, a2, b1, b2 model.Point) (model.Point, bool) {
	rx, ry := a2.X-a1.X, a2.Y-a1.Y
	sx, sy := b2.X-b1.X, b2.Y-b1.Y
	den := rx*sy - ry*sx
	if math.Abs(den) < 1e-9 {
		return model.Point{}, false
	}
	qx, qy := b1.X-a1.X, b1.Y-a1.Y
	t := (qx*sy - qy*sx) / den
	u := (qx*ry - qy*rx) / den
	if t <= 0 || t >= 1 || u <= 0 || u >= 1 {
		return model.Point{}, false
	}
	return model.Point{X: a1.X + t*rx, Y: a1.Y + t*ry}, true
}

func nearAny(p model.Point, r float64, pts ...model.Point) bool {
	for _, q := range pts {
		if dist(p, q) <= r {
			return true
		}
	}
	return false
}

// recorded reports whether e carries a Crossing record at p.
func recorded(e model.PositionedEdge, p model.Point) bool {
	for _, cr := range e.Crossings {
		if cr.SegmentIndex < 0 || cr.SegmentIndex >= len(e.Points)-1 {
			continue
		}
		a, b := e.Points[cr.SegmentIndex], e.Points[cr.SegmentIndex+1]
		at := model.Point{X: a.X + cr.T*(b.X-a.X), Y: a.Y + cr.T*(b.Y-a.Y)}
		if dist(at, p) <= crossMatchTol {
			return true
		}
	}
	return false
}
