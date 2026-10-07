package text

// drawAt writes text in the cells from at along its row, the box the
// labels stage resolved for it (S10 Text cells), when every one of them is
// on the grid and empty or holds only a group frame's side, which the
// text replaces. Returns false, drawing nothing, otherwise: the caller
// reports the text as dropped, never drawn over other ink nor moved.
func drawAt(g *charGrid, at cellPoint, text string) bool {
	for i := range len([]rune(text)) {
		if !g.isEmpty(at.x+i, at.y) && !g.frameSide(at.x+i, at.y) {
			return false
		}
	}
	g.writeText(at.x, at.y, text)
	return true
}

// placeText writes a text the labels stage left unresolved (C14.4) at the
// first candidate run of cells that is fully empty (one cell of slack each
// side), sweeping outward from seed: beside the wire first, sliding along
// it, then the mirrored side. pts is the edge's polyline, used to decide
// whether the wire runs vertical or horizontal near seed. Returns false
// when no candidate fits (the caller then reports the text as dropped,
// never drawn over ink).
func placeText(g *charGrid, pts []cellPoint, seed cellPoint, text string) bool {
	rs := []rune(text)
	n := len(rs)
	fits := func(x, y int) bool {
		for i := -1; i <= n; i++ {
			cx := x + i
			offGrid := cx < 0 || cx >= g.cols || y < 0 || y >= g.rows
			// The text's own cells (0..n-1) must be on-grid and empty. A
			// slack cell (i == -1 or i == n) that falls off the grid has no
			// ink to collide with, so it trivially satisfies the slack
			// requirement instead of blocking placement outright.
			if i >= 0 && i < n {
				if offGrid || !g.isEmpty(cx, y) {
					return false
				}
				continue
			}
			if offGrid {
				continue
			}
			if !g.isEmpty(cx, y) {
				return false
			}
		}
		return true
	}
	// Is the wire near the seed vertical or horizontal? Pick the segment
	// whose row or column contains the seed; default to vertical.
	vertical := true
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		if a.y == b.y && a.y == seed.y {
			vertical = false
			break
		}
	}
	base := seed
	var dxs, dys []int
	if vertical {
		// at the seed, nudge along the row, then mirror to the wire's other
		// side (the seed is the resolver's left edge, normally already clear)
		dxs = []int{0, 1, -1, 2, -2, 3, -3, -(n + 2), -(n + 3), -(n + 4)}
		dys = []int{0, -1, 1, -2, 2, -3, 3}
	} else {
		// above the wire, slide along it, then mirror below
		dxs = []int{0, 1, -1, 2, -2, 3, -3, 4, -4}
		dys = []int{-1, 1, -2, 2}
	}
	for _, dy := range dys {
		for _, dx := range dxs {
			x, y := base.x+dx, base.y+dy
			if fits(x, y) {
				g.writeText(x, y, string(rs))
				return true
			}
		}
	}
	return false
}

// placeLabel draws the edge-name label: at its box when the labels stage
// resolved it, else on the ladder from its box.
func placeLabel(g *charGrid, ce cellEdge) bool {
	if ce.labelAt == nil || len(ce.pts) < 2 {
		return false
	}
	if !ce.unresolved {
		return drawAt(g, *ce.labelAt, ce.label)
	}
	return placeText(g, ce.pts, *ce.labelAt, ce.label)
}

// placeCard draws a cardinality: at its box when the labels stage resolved
// it, else on the ladder from its box.
func placeCard(g *charGrid, ce cellEdge, c *cellCard) bool {
	if !c.unresolved {
		return drawAt(g, c.at, c.text)
	}
	return placeText(g, ce.pts, c.at, c.text)
}
