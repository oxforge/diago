// Package metrics measures a positioned flow or class layout against the
// quality objectives (Q1-Q5 of the layout rules spec), an anchored
// layout against its previous one (Q6, MeasureChurn), and counts its
// output-contract defects by kind, on the JSON layout (model.PositionedGraph)
// alone, so every engine is measured alike. A worse value is a regression
// to explain, never a contract violation; internal/layout/contract owns
// those.
package metrics

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/model"
)

// Metrics is one layout's measurements.
type Metrics struct {
	Crossings  int     // Q1: points where two edges' segments cross inside both
	Bends      int     // Q2: turns along every route
	Jogs       int     // Q2: interior segments shorter than JogMin, beyond float noise
	Straight   int     // Q3: routes drawn as one straight segment
	Fans       int     // Q4: nodes with two or more forward neighbors on one side
	Centered   int     // Q4: fans whose node centers on its neighbors' span
	Area       float64 // Q5: the canvas, in px²
	Wire       float64 // Q5: the total length of every route, in px
	Overlaps   int     // pairs of node boxes that overlap
	Intrusions int     // C4 violations: a wire through a node
	Clearance  int     // C5 and C6 violations: a wire near a node, near a group or along its side
	Stubs      int     // C7 violations
	Labels     int     // C14 violations, labels placed as unresolved and titles placed blocked
	Violations int     // every contract violation
}

// Options configures one measurement.
type Options struct {
	Contract contract.Options // the contract check the defect counts come from
	JogMin   float64          // Q2: an interior segment shorter than this, by more than jogTol, is a jog
}

// ScreenOptions measures a screen-profile layout laid out in dir.
func ScreenOptions(dir model.Direction, edgeIDs []string) Options {
	return Options{
		Contract: contract.Options{Limits: contract.ScreenLimits(), Direction: dir, EdgeIDs: edgeIDs},
		JogMin:   12,
	}
}

const (
	tol       = 0.01 // axis alignment, and how far inside both segments a crossing lies
	centerTol = 1.0  // Q4: a fan's node within this of its neighbors' mid-span counts as centered
	jogTol    = 1e-6 // Q2: a segment this much under JogMin is JogMin long, up to float noise
)

// Measure computes pg's metrics.
func Measure(pg *model.PositionedGraph, o Options) Metrics {
	m := Metrics{Area: pg.Width * pg.Height}
	for _, e := range pg.Edges {
		pts := e.Points
		if len(pts) == 2 {
			m.Straight++
		}
		for i := 0; i+1 < len(pts); i++ {
			m.Wire += math.Hypot(pts[i+1].X-pts[i].X, pts[i+1].Y-pts[i].Y)
			if i > 0 && i+2 < len(pts) && length(pts[i], pts[i+1]) < o.JogMin-jogTol {
				m.Jogs++
			}
			if i > 0 && turns(pts[i-1], pts[i], pts[i+1]) {
				m.Bends++
			}
		}
		if e.LabelUnresolved {
			m.Labels++
		}
		for _, c := range []*model.EndLabel{e.FromCard, e.ToCard} {
			if c != nil && c.Unresolved {
				m.Labels++
			}
		}
	}
	for _, g := range pg.Groups {
		if g.LabelBlocked {
			m.Labels++
		}
	}
	m.Crossings = crossings(pg.Edges)
	for i := range pg.Nodes {
		for j := i + 1; j < len(pg.Nodes); j++ {
			if overlap(pg.Nodes[i], pg.Nodes[j]) {
				m.Overlaps++
			}
		}
	}
	m.Fans, m.Centered = fans(pg, o.Contract.Direction)
	vs := contract.Check(pg, o.Contract)
	m.Violations = len(vs)
	for _, v := range vs {
		switch {
		case v.Rule == "C4":
			m.Intrusions++
		case v.Rule == "C5" || strings.HasPrefix(v.Rule, "C6."):
			m.Clearance++
		case v.Rule == "C7":
			m.Stubs++
		case strings.HasPrefix(v.Rule, "C14"):
			m.Labels++
		}
	}
	return m
}

// String renders m as space-separated key=value pairs, the Q values
// first; px values round to whole px.
func (m Metrics) String() string {
	return fmt.Sprintf("cross=%d bends=%d jogs=%d straight=%d fans=%d/%d area=%.0f wire=%.0f overlaps=%d intrusions=%d clearance=%d stubs=%d labels=%d violations=%d",
		m.Crossings, m.Bends, m.Jogs, m.Straight, m.Centered, m.Fans, m.Area, m.Wire,
		m.Overlaps, m.Intrusions, m.Clearance, m.Stubs, m.Labels, m.Violations)
}

// Parse is the inverse of Metrics.String: it reads back one line of a
// metrics baseline (one layout's line, without
// its leading "<spec>/<DIRECTION> " name) into a Metrics, for a tool that
// compares a committed baseline's numbers rather than its text.
func Parse(s string) (Metrics, error) {
	var m Metrics
	var fans int
	n, err := fmt.Sscanf(s, "cross=%d bends=%d jogs=%d straight=%d fans=%d/%d area=%f wire=%f overlaps=%d intrusions=%d clearance=%d stubs=%d labels=%d violations=%d",
		&m.Crossings, &m.Bends, &m.Jogs, &m.Straight, &m.Centered, &fans, &m.Area, &m.Wire,
		&m.Overlaps, &m.Intrusions, &m.Clearance, &m.Stubs, &m.Labels, &m.Violations)
	if err != nil {
		return Metrics{}, fmt.Errorf("metrics.Parse %q: %w", s, err)
	}
	if n != 14 {
		return Metrics{}, fmt.Errorf("metrics.Parse %q: read %d of 14 fields", s, n)
	}
	m.Fans = fans
	return m, nil
}

func length(p, q model.Point) float64 { return math.Hypot(q.X-p.X, q.Y-p.Y) }

// turns reports whether a route changes direction at b.
func turns(a, b, c model.Point) bool {
	cross := (b.X-a.X)*(c.Y-b.Y) - (b.Y-a.Y)*(c.X-b.X)
	return math.Abs(cross) > tol*(length(a, b)+length(b, c))
}

// crossings counts the pairs of segments of two different edges, one
// horizontal and one vertical, that cross strictly inside both.
func crossings(edges []model.PositionedEdge) int {
	n := 0
	for a := range edges {
		for b := a + 1; b < len(edges); b++ {
			pa, pb := edges[a].Points, edges[b].Points
			for i := 0; i+1 < len(pa); i++ {
				for j := 0; j+1 < len(pb); j++ {
					if cross(pa[i], pa[i+1], pb[j], pb[j+1]) {
						n++
					}
				}
			}
		}
	}
	return n
}

func cross(p1, p2, q1, q2 model.Point) bool {
	h, v := [2]model.Point{p1, p2}, [2]model.Point{q1, q2}
	if math.Abs(p1.Y-p2.Y) > tol {
		h, v = v, h
	}
	if math.Abs(h[0].Y-h[1].Y) > tol || math.Abs(v[0].X-v[1].X) > tol {
		return false // not one horizontal and one vertical segment
	}
	x, y := v[0].X, h[0].Y
	return inside(x, h[0].X, h[1].X) && inside(y, v[0].Y, v[1].Y)
}

// inside reports whether x lies strictly between a and b, by more than tol.
func inside(x, a, b float64) bool {
	return x > math.Min(a, b)+tol && x < math.Max(a, b)-tol
}

// overlap reports whether two node boxes share an area.
func overlap(a, b model.PositionedNode) bool {
	return math.Abs(a.X-b.X) < (a.Width+b.Width)/2-tol && math.Abs(a.Y-b.Y) < (a.Height+b.Height)/2-tol
}

// fans counts the nodes with two or more distinct forward neighbors
// downstream (a fan-out) or upstream (a fan-in), each side counted apart,
// and those whose cross-axis center lies within centerTol of the midpoint
// of their neighbors' extreme cross-axis centers. A diamond's fan-out of
// two whose main line runs straight counts as centered too (Q4, mainLine).
// A forward edge's target box lies wholly downstream of its source's (C0).
// Under Auto nothing counts.
func fans(pg *model.PositionedGraph, dir model.Direction) (fans, centered int) {
	if dir == model.Auto {
		return 0, 0
	}
	byID := make(map[string]int, len(pg.Nodes))
	for i, n := range pg.Nodes {
		byID[n.ID] = i
	}
	sideways := dir == model.Right || dir == model.Left
	flow := func(n model.PositionedNode) (lo, hi float64) {
		c, ext := n.Y, n.Height
		if sideways {
			c, ext = n.X, n.Width
		}
		lo, hi = c-ext/2, c+ext/2
		if dir == model.Up || dir == model.Left {
			lo, hi = -hi, -lo
		}
		return lo, hi
	}
	crossC := func(n model.PositionedNode) float64 {
		if sideways {
			return n.Y
		}
		return n.X
	}
	down := make([][]int, len(pg.Nodes)) // per node: forward neighbors downstream
	up := make([][]int, len(pg.Nodes))   // and upstream
	for _, e := range pg.Edges {
		f, okf := byID[e.From]
		t, okt := byID[e.To]
		if !okf || !okt || f == t {
			continue
		}
		_, fHi := flow(pg.Nodes[f])
		tLo, _ := flow(pg.Nodes[t])
		if tLo < fHi-tol {
			continue // not forward
		}
		if !slices.Contains(down[f], t) {
			down[f] = append(down[f], t)
			up[t] = append(up[t], f)
		}
	}
	for i, n := range pg.Nodes {
		for side, nbrs := range [2][]int{down[i], up[i]} {
			if len(nbrs) < 2 {
				continue
			}
			fans++
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, k := range nbrs {
				lo, hi = math.Min(lo, crossC(pg.Nodes[k])), math.Max(hi, crossC(pg.Nodes[k]))
			}
			if math.Abs(crossC(n)-(lo+hi)/2) <= centerTol || (side == 0 && mainLine(pg, i, down)) {
				centered++
			}
		}
	}
	return fans, centered
}

// mainLine reports whether node i is a diamond whose fan-out of two has a
// main line that runs straight (Q4, S7's primary exit): exactly one of its
// two downstream neighbors has a downstream neighbor of its own, and an
// edge from i to that one is a single segment.
func mainLine(pg *model.PositionedGraph, i int, down [][]int) bool {
	if pg.Nodes[i].Shape != model.ShapeDiamond || len(down[i]) != 2 {
		return false
	}
	a, b := down[i][0], down[i][1]
	if (len(down[a]) > 0) == (len(down[b]) > 0) {
		return false
	}
	main := a
	if len(down[b]) > 0 {
		main = b
	}
	for _, e := range pg.Edges {
		if e.From == pg.Nodes[i].ID && e.To == pg.Nodes[main].ID && len(e.Points) == 2 {
			return true
		}
	}
	return false
}
