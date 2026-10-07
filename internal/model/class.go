package model

// Members is a class node's record-box content. A non-nil Members on a
// Node marks it as a UML class; its Shape stays ShapeRect so every
// rectangle path (the layered engine's ports, the exporters, text boxes)
// applies.
type Members struct {
	Stereotype string
	TypeParams []string
	Attributes []Member
	Methods    []Member
}

// Member is one attribute or method line of a class.
type Member struct {
	Visibility string // "+", "-", "#", "~" or ""
	Text       string
	Static     bool
	Abstract   bool
	// Mark is the text-diff status column ("+ ", "- ", "~ ", "  "),
	// composed before the visibility glyph. Set only by diff.StampLabels
	// on the text path; empty everywhere else.
	Mark string
}

// Legend is the spec's legend request; nil on the Graph means no legend.
// Labels overrides the row label per relation kind; an empty value hides
// that row.
type Legend struct{ Labels map[string]string }

// Record-box metrics shared by sizing (layer) and rendering (svg).
const (
	ClassPadX     = 10.0
	ClassPadY     = 6.0
	ClassMinWidth = 60.0
)

// Adorned reports whether the relation is drawn with a UML adornment at
// its source end (hollow triangle or diamond) instead of an arrowhead.
func (r Relation) Adorned() bool {
	switch r {
	case RelationInheritance, RelationRealization, RelationAggregation, RelationComposition:
		return true
	}
	return false
}

// Dashed reports whether the relation's wire is dashed.
func (r Relation) Dashed() bool {
	return r == RelationRealization || r == RelationDependency
}

// Swapped reports whether the parser swaps the relation's endpoints so the
// supertype or interface becomes the layout source.
func (r Relation) Swapped() bool {
	return r == RelationInheritance || r == RelationRealization
}

// PositionedMembers is the laid-out record box: composed lines with their
// styling flags and the two line heights the renderer stacks them by.
type PositionedMembers struct {
	Header           []PositionedLine `json:"header"` // «stereotype» line (if any), then the name line
	Attributes       []PositionedLine `json:"attributes"`
	Methods          []PositionedLine `json:"methods"`
	HeaderLineHeight float64          `json:"header_line_height"`
	MemberLineHeight float64          `json:"member_line_height"`
}

// PositionedLine is one text line of a record box.
type PositionedLine struct {
	Text       string `json:"text"`
	Stereotype bool   `json:"stereotype,omitempty"` // the «…» header line
	Italic     bool   `json:"italic,omitempty"`     // abstract name or member
	Underline  bool   `json:"underline,omitempty"`  // static member
}

// EndLabel is a cardinality at one end of a relation, placed by the labels
// stage (S10).
type EndLabel struct {
	Text       string  `json:"text"`
	Pos        *Point  `json:"pos,omitempty"` // center
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Unresolved bool    `json:"unresolved,omitempty"`
}

// LegendEntry is one resolved legend row.
type LegendEntry struct {
	Kind  string `json:"kind"` // relation kind
	Label string `json:"label"`
}
