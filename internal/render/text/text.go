// Package text renders positioned flow, class and sequence diagrams as
// Unicode box-drawing text art on an 8x16 px character grid. It expects
// layouts produced under a text profile (C0 and S14 for flow and class,
// the sequence engine's own for sequence): node and group edges on cell
// boundaries, orthogonal routes.
package text

import (
	"fmt"
	"math"
	"sort"

	"github.com/oxforge/diago/internal/model"
)

// Character cell dimensions in pixels, the text profile's 8 × 16 (C0).
const (
	cellW = 8.0
	cellH = 16.0
)

// px2col/px2row map a pixel coordinate to the cell containing it.
func px2col(x float64) int { return int(math.Floor(x / cellW)) }
func px2row(y float64) int { return int(math.Floor(y / cellH)) }

// DroppedLabel is an edge label the renderer could not place without
// overwriting ink (reported, never drawn). ID is the positioned edge's id.
type DroppedLabel struct {
	ID, From, To, Label string
	End                 string // "", "from" or "to": set for a cardinality
}

// Result is the rendered art plus every label that fit nowhere. TopRow is
// the grid row of the art's first line (String drops leading blank rows),
// so a caller holding the layout can map art rows back to cell rows.
type Result struct {
	Art           string
	TopRow        int
	DroppedLabels []DroppedLabel
	Legend        []string // one row per legend entry, appended by the pipeline after the art
}

// Render converts a text-profile PositionedGraph into text art.
func Render(pg *model.PositionedGraph) (Result, error) {
	cols := px2col(pg.Width) + 1
	rows := px2row(pg.Height) + 1
	g := newCharGrid(cols, rows)

	// 1. Groups, shallow to deep, then input order.
	groups := make([]model.PositionedGroup, len(pg.Groups))
	copy(groups, pg.Groups)
	sort.SliceStable(groups, func(i, j int) bool { return groups[i].Depth < groups[j].Depth })
	for _, grp := range groups {
		drawGroup(g, grp)
	}
	g.keepFrames()

	// 2. Edges (side ends one cell outside their face), remembering side arrivals.
	nodeByID := make(map[string]model.PositionedNode, len(pg.Nodes))
	for _, n := range pg.Nodes {
		nodeByID[n.ID] = n
	}
	edges := make([]cellEdge, len(pg.Edges))
	for i, e := range pg.Edges {
		ce, err := toCellEdge(e, nodeByID)
		if err != nil {
			return Result{}, err
		}
		edges[i] = ce
		drawEdgeRuns(g, ce)
	}

	// 3. Crossings: the vertical wire wins the cell.
	for _, ce := range edges {
		stampCrossings(g, ce)
	}

	// 4. Nodes.
	for _, n := range pg.Nodes {
		drawNode(g, n)
	}

	// 5. Arrowheads.
	for _, ce := range edges {
		drawHeads(g, ce)
	}

	// 5b. UML relation adornments (hollow triangle / diamond).
	for _, ce := range edges {
		drawAdornment(g, ce)
	}

	// 6. Cardinalities, then edge labels, each in edge order, a
	// cardinality's from end before its to end (S10's order): first every
	// one the labels stage resolved, at its box, then every one it left
	// unresolved, on the ladder, so that none of those can take a
	// resolved one's cells (S10 Text cells). placeCard and placeLabel draw
	// each text all or nothing: every cell of it, or none, never a partial
	// one left behind.
	var dropped []DroppedLabel
	for _, unresolved := range [2]bool{false, true} {
		for _, ce := range edges {
			for _, c := range []struct {
				card *cellCard
				end  string
			}{{ce.fromCard, "from"}, {ce.toCard, "to"}} {
				if c.card == nil || c.card.unresolved != unresolved {
					continue
				}
				if !placeCard(g, ce, c.card) {
					dropped = append(dropped, DroppedLabel{ID: ce.id, From: ce.from, To: ce.to, Label: c.card.text, End: c.end})
				}
			}
		}
		for _, ce := range edges {
			if ce.label == "" || ce.unresolved != unresolved {
				continue
			}
			if !placeLabel(g, ce) {
				dropped = append(dropped, DroppedLabel{ID: ce.id, From: ce.from, To: ce.to, Label: ce.label})
			}
		}
	}

	art, top := g.render()
	titledArt, titleLines := withTitle(art, pg.Title)
	return Result{Art: titledArt, TopRow: top - titleLines, DroppedLabels: dropped, Legend: legendRows(pg.Legend)}, nil
}

// drawGroup paints a group frame (sides as flags, corners and title as
// glyphs). The title's first rune sits LabelOffset from the frame's left
// side, or three columns in when the layout left the title to the
// renderer.
func drawGroup(g *charGrid, grp model.PositionedGroup) {
	left, top := px2col(grp.X), px2row(grp.Y)
	right, bottom := px2col(grp.X+grp.Width)-1, px2row(grp.Y+grp.Height)-1
	if right-left < 4 || bottom-top < 2 {
		return
	}
	at := left + 3
	if grp.LabelOffset > 0 {
		at = px2col(grp.X + grp.LabelOffset)
	}
	g.frame(left, top, right-left+1, bottom-top+1, grp.Label, at)
}

// Errors.
func errNotAxisAligned(from, to string, seg int) error {
	return fmt.Errorf("text: edge %s->%s segment %d is not axis-aligned after cell mapping", from, to, seg)
}
