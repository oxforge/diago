// Package contract checks a positioned flow or class layout against the
// output contract, Part 1 of the layout rules spec (rules C2–C17). The
// contract binds every layout engine alike, so the checker reads only the
// JSON layout (model.PositionedGraph) and imports no engine. C1
// (determinism) and C18 (anchoring) are properties of an engine run, not of
// one layout; engine tests check them.
package contract

import (
	"fmt"
	"slices"

	"github.com/oxforge/diago/internal/model"
)

// Violation is one breach of the output contract.
type Violation struct {
	Rule    string // rule or clause id, e.g. "C5" or "C8.2"
	Subject string // edge, node or group id; empty for the whole layout
	Detail  string
}

// RuleIDs lists every id Check can report, in document order.
var RuleIDs = []string{
	"C2.1", "C2.2", "C3", "C4", "C5", "C6.1", "C6.2", "C7",
	"C8.1", "C8.2", "C8.3", "C8.4", "C8.5",
	"C9.1", "C9.2", "C10", "C11", "C12.1", "C12.2", "C12.3", "C13",
	"C14.1", "C14.2", "C14.3", "C14.5", "C15.1", "C15.2", "C16.1", "C16.2",
	"C17.1", "C17.2", "C17.3",
}

// Limits are the contract's distances for one layout profile, in px.
type Limits struct {
	NodeClearance float64 // C5, C6.1, C12.3: wire to foreign node box and to group border; group box to foreign box
	TrackGap      float64 // C6.2: wire to a group side along it; C9.2: distinct parallel wires (checked with 1 px tolerance)
	PortGap       float64 // C9.2: side-by-side terminal segments of a same-face rake
	StubMin       float64 // C7: first and last segment length
	NodeGap       float64 // C11: node box to node box
	LabelGap      float64 // C14.3: label box to its own edge
	Margin        float64 // C16: geometry to canvas edge
	EnvelopeDepth float64 // C14: adornment envelope depth along the first segment
	EnvelopeHalf  float64 // C14: adornment envelope half-width across it
	TitleInsetX   float64 // C13: title box left edge, from the group's left edge
	TitleCenterY  float64 // C13: title box vertical center, from the group's top edge
	TitlePad      float64 // C13: the blank the text renderer writes on each side of a title; zero on screen
	CellW, CellH  float64 // C16: the text profile's cell, in which a point is drawn; zero on screen
}

// ScreenLimits returns the screen-profile values of Part 1.
func ScreenLimits() Limits {
	return Limits{
		NodeClearance: 12,
		TrackGap:      8,
		PortGap:       4,
		StubMin:       20,
		NodeGap:       20,
		LabelGap:      6,
		Margin:        20,
		EnvelopeDepth: 14,
		EnvelopeHalf:  7,
		TitleInsetX:   8,
		TitleCenterY:  16,
	}
}

// TextLimits returns the text-profile values of Part 1 for a layout
// direction, in px at 8 × 16 px per cell: a port gap of one cell across
// the face, a column under DOWN and UP and a row under RIGHT and LEFT, and
// the node gap the text Config (S14) keeps between the neighbors of a
// layer.
func TextLimits(dir model.Direction) Limits {
	l := Limits{
		NodeClearance: 16,
		TrackGap:      16,
		PortGap:       8,
		StubMin:       24,
		NodeGap:       40,
		LabelGap:      8,
		Margin:        16,
		EnvelopeDepth: 8,
		EnvelopeHalf:  8,
		TitleInsetX:   24,
		TitleCenterY:  8,
		TitlePad:      8,
		CellW:         8,
		CellH:         16,
	}
	if dir == model.Right || dir == model.Left {
		l.PortGap, l.NodeGap = 16, 32
	}
	return l
}

// Options configures one check.
type Options struct {
	Limits Limits
	// Direction is the resolved layout direction. It orients C8.2–C8.5 (the
	// in-vertex and edge kinds); Auto skips those four clauses.
	Direction model.Direction
	// EdgeIDs, when non-nil, lists every edge id of the spec; C2.1 reports
	// the ids missing from the layout.
	EdgeIDs []string
	// Text marks a text-profile layout, where every shape is a box: the
	// check reads every node as a rect. With the Limits' cell set
	// (TextLimits), it also reads the cells the text renderer draws in:
	// C14.2 tests a label against the cells another edge's segment is
	// drawn in, C14.5 requires every label box to lie on whole cells, and
	// C17.1 lets a copy's label lie up to half a cell off the translation.
	Text bool
}

// Tolerances absorb float error only. They are never loosened to make a
// layout pass. Rule-specific ones live beside their rule.
const (
	orthoTol       = 0.01 // C3: axis alignment
	attachTol      = 2.0  // C2.2, C8.1: endpoint to outline or vertex
	penetrationTol = 0.01 // C4, C5, C6: length inside a box, or along a group side
	slack          = 0.05 // C5, C6, C7, C11, C12, C16: distance floors
	retraceTol     = 0.01 // C10
)

// Check validates pg against rules C2–C17 and returns every violation, in
// rule order, then in edge, node and group order.
func Check(pg *model.PositionedGraph, opts Options) []Violation {
	if pg == nil {
		return nil
	}
	if opts.Text {
		pg = boxed(pg)
	}
	c := newChecker(pg, opts)
	c.checkEdgesPresent()
	c.checkAttachment()
	c.checkOrthogonal()
	c.checkNodeInteriors()
	c.checkNodeClearance()
	c.checkGroupClearance()
	c.checkGroupSides()
	c.checkStubs()
	c.checkShapePorts()
	c.checkSpacing()
	c.checkSelfRetrace()
	c.checkNodeSeparation()
	c.checkGroupContainment()
	c.checkTitles()
	c.checkLabels()
	c.checkCrossings()
	c.checkMargins()
	c.checkCongruence()
	return c.sorted()
}

// boxed returns a copy of pg with every node a rect.
func boxed(pg *model.PositionedGraph) *model.PositionedGraph {
	out := *pg
	out.Nodes = slices.Clone(pg.Nodes)
	for i := range out.Nodes {
		out.Nodes[i].Shape = model.ShapeRect
	}
	return &out
}

type checker struct {
	pg    *model.PositionedGraph
	opts  Options
	lim   Limits
	nodes map[string]model.PositionedNode
	// members[g] holds the ids of every node inside group g, through its
	// child groups too.
	members map[string]map[string]bool
	out     []Violation
}

func newChecker(pg *model.PositionedGraph, opts Options) *checker {
	c := &checker{
		pg:      pg,
		opts:    opts,
		lim:     opts.Limits,
		nodes:   make(map[string]model.PositionedNode, len(pg.Nodes)),
		members: make(map[string]map[string]bool, len(pg.Groups)),
	}
	for _, n := range pg.Nodes {
		c.nodes[n.ID] = n
	}
	byID := make(map[string]model.PositionedGroup, len(pg.Groups))
	for _, g := range pg.Groups {
		byID[g.ID] = g
	}
	var collect func(g model.PositionedGroup, into map[string]bool, depth int)
	collect = func(g model.PositionedGroup, into map[string]bool, depth int) {
		for _, id := range g.Contains {
			into[id] = true
		}
		if depth > len(pg.Groups) {
			return // a malformed child cycle; groups nest at most three deep
		}
		for _, ch := range g.Children {
			if child, ok := byID[ch]; ok {
				collect(child, into, depth+1)
			}
		}
	}
	for _, g := range pg.Groups {
		set := make(map[string]bool)
		collect(g, set, 0)
		c.members[g.ID] = set
	}
	return c
}

func (c *checker) add(rule, subject, format string, args ...any) {
	c.out = append(c.out, Violation{Rule: rule, Subject: subject, Detail: fmt.Sprintf(format, args...)})
}

// sorted returns the violations in RuleIDs order. Within one rule they keep
// the order the checks found them in, whatever order that rule's own check
// iterates (C12 and C13, for instance, iterate groups first).
func (c *checker) sorted() []Violation {
	rank := make(map[string]int, len(RuleIDs))
	for i, id := range RuleIDs {
		rank[id] = i
	}
	slices.SortStableFunc(c.out, func(a, b Violation) int { return rank[a.Rule] - rank[b.Rule] })
	return c.out
}

// routed reports whether e has a polyline. C2.1 reports the others once;
// every other rule skips them.
func routed(e model.PositionedEdge) bool { return len(e.Points) >= 2 }
