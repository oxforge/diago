package model

// PositionedGraph is the output of the layout engine, ready for rendering.
type PositionedGraph struct {
	Nodes  []PositionedNode  `json:"nodes"`
	Edges  []PositionedEdge  `json:"edges"`
	Groups []PositionedGroup `json:"groups"`
	Width  float64           `json:"width"`
	Height float64           `json:"height"`
	Title  string            `json:"title,omitempty"` // copied from Graph.Title by the layout stage; render-time only

	// LayoutHints is the anchoring carrier (C18) emitted by the layout
	// engine. Never rendered as geometry; the SVG
	// renderer embeds it as metadata.
	LayoutHints *LayoutHints `json:"layout_hints,omitempty"`

	// AppliedCongruences records the congruences (C17) the layout applied
	// (S12): each set's Rule A copies and, when present, its Rule C
	// fan-out. It is layout metadata the labels stage (S10) and the
	// contract checker read, never rendered directly.
	AppliedCongruences []AppliedCongruence `json:"appliedCongruences,omitempty"`

	// Legend is the resolved legend rows (Phase 4), populated when the
	// spec requested one. Nil when no legend is requested.
	Legend []LegendEntry `json:"legend,omitempty"`
}

// AppliedCongruence is one congruence (C17) whose Rule A copies the layout
// applied (S12). Edge indices are positional into PositionedGraph's
// edge slice (identical to the input g.Edges order).
type AppliedCongruence struct {
	RepEdges []int                     `json:"rep_edges"`
	Members  []AppliedCongruenceMember `json:"members"`
	FanOut   []int                     `json:"fan_out,omitempty"`
}

// AppliedCongruenceMember is one accepted non-representative member: its internal
// edge indices (aligned index-for-index with RepEdges under the congruence node
// bijection) and its rigid translation from the representative's frame.
// Dx and Dy are in the engine's frame, before the direction transform (S11)
// that produces PositionedGraph geometry, and in px in the text profile
// (S12). Geometric consumers must derive the final-frame translation from
// corresponding route points, as the labels stage (S10) and the contract
// checker (C17.1) do, rather than applying Dx/Dy directly.
type AppliedCongruenceMember struct {
	Edges []int   `json:"edges"`
	Dx    float64 `json:"dx"`
	Dy    float64 `json:"dy"`
}

// PositionedNode is a node with computed center coordinates and dimensions.
type PositionedNode struct {
	ID     string   `json:"id"`
	Label  string   `json:"label"`
	Lines  []string `json:"lines,omitempty"` // wrapped label lines (nil = single line)
	Shape  Shape    `json:"shape"`
	X      float64  `json:"x"` // center
	Y      float64  `json:"y"` // center
	Width  float64  `json:"width"`
	Height float64  `json:"height"`
	Color  string   `json:"color,omitempty"`

	// Members is the laid-out record box (Phase 4), populated when this
	// node is a UML class. Nil for a plain flow node.
	Members *PositionedMembers `json:"members,omitempty"`
}

// PositionedEdge is an edge with computed polyline waypoints.
type PositionedEdge struct {
	ID          string        `json:"id"`
	From        string        `json:"from"`
	To          string        `json:"to"`
	Label       string        `json:"label,omitempty"`
	Style       EdgeStyle     `json:"style"`
	Direction   EdgeDirection `json:"direction"`
	Points      []Point       `json:"points"`                // polyline waypoints (first = source port, last = target port)
	LabelPos    *Point        `json:"labelPos,omitempty"`    // optional: layout-computed label position (center of label)
	LabelWidth  float64       `json:"labelWidth,omitempty"`  // rendered label width (0 if no label)
	LabelHeight float64       `json:"labelHeight,omitempty"` // rendered label height (0 if no label)
	// LabelUnresolved marks a label the labels stage (S10) could not place
	// clear of every obstacle (nodes, other edges' wires, other labels,
	// adornment envelopes, group titles): it holds the spot with the least
	// overlap and the engine recorded a `label_unresolved` decision. This is
	// a sanctioned degradation (C0, "never a silent overlap"), so the
	// contract exempts such a label's residual overlap (C14.4).
	LabelUnresolved bool       `json:"labelUnresolved,omitempty"`
	Color           string     `json:"color,omitempty"`
	Crossings       []Crossing `json:"crossings,omitempty"` // points where this edge crosses over another

	// FlatRanked marks a flat edge (Edge.Flat) for which the router of S9's
	// *Flat edges* kept no route: the layout fell back and laid it out as
	// an ordinary edge, ranked and routed by S8, so that the render can
	// warn about it. False on every other edge, a flat edge routed flat
	// included.
	FlatRanked bool `json:"flatRanked,omitempty"`

	// Class relations: the adornment and the cardinalities (C14, S10).
	// Relation is RelationNone on a flow edge.
	Relation Relation  `json:"relation,omitempty"`
	Directed bool      `json:"directed,omitempty"`
	FromCard *EndLabel `json:"from_card,omitempty"`
	ToCard   *EndLabel `json:"to_card,omitempty"`
}

// Crossing marks where this edge passes over another edge, stored
// parametrically along this edge's own polyline so rigid transforms of the
// diagram cannot desynchronize it (C15.1).
type Crossing struct {
	SegmentIndex int     `json:"segment_index"` // index into Points: segment [i, i+1]
	T            float64 `json:"t"`             // position along that segment, 0..1
}

// Point is a 2D coordinate.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// PositionedGroup is a group with computed bounding box.
type PositionedGroup struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	X        float64  `json:"x"` // top-left corner
	Y        float64  `json:"y"` // top-left corner
	Width    float64  `json:"width"`
	Height   float64  `json:"height"`
	Contains []string `json:"contains"` // node IDs in this group
	Children []string `json:"children"` // child group IDs
	Depth    int      `json:"depth"`
	Color    string   `json:"color,omitempty"`
	// LabelWidth/LabelHeight are the measured dimensions of the group title
	// text (theme group label font), with which the labels stage (S10) and
	// the contract (C13, C14.1) treat the title box as an obstacle. Zero when
	// unlabeled.
	LabelWidth  float64 `json:"label_width,omitempty"`
	LabelHeight float64 `json:"label_height,omitempty"`
	// LabelOffset is the title box's left side, from the group's left
	// side, where the layout placed the title in its band (the layered
	// engine's labels stage, S10); zero leaves the title where each
	// renderer puts it by default.
	LabelOffset float64 `json:"label_offset,omitempty"`
	// LabelBlocked marks a title for which the layout found no slot clear
	// of the wires (a sanctioned degradation, C0 and C13): wires may cross
	// it, and every renderer draws the title in front of them, the wire
	// gapped under it.
	LabelBlocked bool `json:"label_blocked,omitempty"`
}
