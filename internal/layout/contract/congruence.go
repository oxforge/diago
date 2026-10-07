package contract

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

const (
	congruenceTol = 0.01 // C17.1–C17.3: route points, slots, spread tracks
	labelMoveTol  = 0.5  // C17.1: translated label centers on screen
)

// edgeAt returns edge i when it exists and is routed.
func (c *checker) edgeAt(i int) (model.PositionedEdge, bool) {
	if i < 0 || i >= len(c.pg.Edges) || !routed(c.pg.Edges[i]) {
		return model.PositionedEdge{}, false
	}
	return c.pg.Edges[i], true
}

// checkCongruence verifies C17 for every congruence the layout reports:
// member routes and labels are translations of the representative's (C17.1),
// fan-out edges enter their members at one slot (C17.2), and paired fan-out
// edges share a spread track without crossing each other (C17.3).
func (c *checker) checkCongruence() {
	for ci, ac := range c.pg.AppliedCongruences {
		c.checkTranslations(ci, ac)
		c.checkSlots(ci, ac)
		c.checkFanOut(ci, ac)
	}
}

func (c *checker) checkTranslations(ci int, ac model.AppliedCongruence) {
	// a text label box lies on whole cells (C14.5), so a label an odd
	// number of cells wider or narrower than its representative's centers
	// half a cell off the translation
	tolX, tolY := labelMoveTol, labelMoveTol
	if c.text() {
		tolX, tolY = c.lim.CellW/2+cellTol, c.lim.CellH/2+cellTol
	}
	for mi, m := range ac.Members {
		var tx, ty float64
		haveT := false
		for k, ri := range ac.RepEdges {
			if k >= len(m.Edges) {
				c.add("C17.1", "", "congruence %d member %d lists %d edges for %d representative edges", ci, mi, len(m.Edges), len(ac.RepEdges))
				break
			}
			rep, okR := c.edgeAt(ri)
			mem, okM := c.edgeAt(m.Edges[k])
			if !okR || !okM {
				continue // C2.1 reports an unrouted edge
			}
			if len(rep.Points) != len(mem.Points) {
				c.add("C17.1", mem.ID, "congruence %d: %d route points, representative %s has %d", ci, len(mem.Points), rep.ID, len(rep.Points))
				continue
			}
			if !haveT {
				tx, ty, haveT = mem.Points[0].X-rep.Points[0].X, mem.Points[0].Y-rep.Points[0].Y, true
			}
			for p := range rep.Points {
				if math.Abs(mem.Points[p].X-rep.Points[p].X-tx) > congruenceTol || math.Abs(mem.Points[p].Y-rep.Points[p].Y-ty) > congruenceTol {
					c.add("C17.1", mem.ID, "congruence %d: point %d is not representative %s's point translated by (%.2f,%.2f)", ci, p, rep.ID, tx, ty)
					break
				}
			}
			switch {
			case rep.LabelPos == nil && mem.LabelPos != nil:
				c.add("C17.1", mem.ID, "congruence %d: has a label, representative %s has none", ci, rep.ID)
			case rep.LabelPos != nil && mem.LabelPos == nil:
				c.add("C17.1", mem.ID, "congruence %d: has no label, representative %s has one", ci, rep.ID)
			case rep.LabelPos != nil &&
				(math.Abs(mem.LabelPos.X-rep.LabelPos.X-tx) > tolX || math.Abs(mem.LabelPos.Y-rep.LabelPos.Y-ty) > tolY):
				c.add("C17.1", mem.ID, "congruence %d: label is not representative %s's label translated by (%.2f,%.2f)", ci, rep.ID, tx, ty)
			}
		}
	}
}

func (c *checker) checkSlots(ci int, ac model.AppliedCongruence) {
	members := len(ac.Members) + 1
	if len(ac.FanOut)%members != 0 {
		c.add("C17.2", "", "congruence %d: %d fan-out edges for %d members", ci, len(ac.FanOut), members)
		return
	}
	per := len(ac.FanOut) / members
	type slot struct {
		at   model.Point
		edge string
	}
	ref := make([]*slot, per)
	for k, fi := range ac.FanOut {
		e, ok := c.edgeAt(fi)
		if !ok {
			continue
		}
		n, ok := c.nodes[e.To]
		if !ok {
			continue
		}
		last := e.Points[len(e.Points)-1]
		at := model.Point{X: last.X - (n.X - n.Width/2), Y: last.Y - (n.Y - n.Height/2)}
		j := k % per
		if ref[j] == nil {
			ref[j] = &slot{at, e.ID}
			continue
		}
		if math.Abs(at.X-ref[j].at.X) > congruenceTol || math.Abs(at.Y-ref[j].at.Y) > congruenceTol {
			c.add("C17.2", e.ID, "congruence %d: enters %s at offset (%.2f,%.2f), edge %s at (%.2f,%.2f)", ci, e.To, at.X, at.Y, ref[j].edge, ref[j].at.X, ref[j].at.Y)
		}
	}
}

func (c *checker) checkFanOut(ci int, ac model.AppliedCongruence) {
	n := len(ac.FanOut)
	type run struct {
		edge  string
		track float64
	}
	byRank := map[int][]run{}
	for i, fi := range ac.FanOut {
		e, ok := c.edgeAt(fi)
		if !ok || c.opts.Direction == model.Auto {
			continue
		}
		if track, ok := c.spreadRun(e); ok {
			rank := min(i, n-1-i)
			byRank[rank] = append(byRank[rank], run{e.ID, track})
		}
	}
	for rank := 0; rank < n; rank++ {
		runs := byRank[rank]
		for _, r := range runs[min(1, len(runs)):] {
			if math.Abs(r.track-runs[0].track) > congruenceTol {
				c.add("C17.3", r.edge, "congruence %d pair %d: spread track %.2f, paired edge %s uses %.2f", ci, rank, r.track, runs[0].edge, runs[0].track)
			}
		}
	}
	for a := range ac.FanOut {
		ea, okA := c.edgeAt(ac.FanOut[a])
		for b := a + 1; b < n && okA; b++ {
			eb, okB := c.edgeAt(ac.FanOut[b])
			if !okB {
				continue
			}
			for i := 0; i+1 < len(ea.Points); i++ {
				for j := 0; j+1 < len(eb.Points); j++ {
					if p, ok := crossing(ea.Points[i], ea.Points[i+1], eb.Points[j], eb.Points[j+1]); ok {
						c.add("C17.3", ea.ID, "congruence %d: fan-out crosses edge %s at (%.2f,%.2f)", ci, eb.ID, p.X, p.Y)
					}
				}
			}
		}
	}
}

// spreadRun is the track of edge e's spread run (C17.3): its first segment
// across the flow axis that lies beyond its source's box on the flow
// axis; false for a route without one.
func (c *checker) spreadRun(e model.PositionedEdge) (float64, bool) {
	src, ok := c.nodes[e.From]
	if !ok {
		return 0, false
	}
	across := c.opts.Direction == model.Right || c.opts.Direction == model.Left
	lo, hi := src.Y-src.Height/2, src.Y+src.Height/2
	if across {
		lo, hi = src.X-src.Width/2, src.X+src.Width/2
	}
	for i := 0; i+1 < len(e.Points); i++ {
		p, q := e.Points[i], e.Points[i+1]
		track, runs := p.Y, horizontal(p, q) && !vertical(p, q)
		if across {
			track, runs = p.X, vertical(p, q) && !horizontal(p, q)
		}
		if runs && (track < lo-congruenceTol || track > hi+congruenceTol) {
			return track, true
		}
	}
	return 0, false
}
