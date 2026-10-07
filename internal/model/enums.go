//go:generate go run ../../cmd/generate-enums

package model

// --- Direction ---

// Direction specifies the overall flow direction of the graph layout.
type Direction int

const (
	Auto Direction = iota
	Down
	Up
	Right
	Left
)

// --- Shape ---

// Shape enumerates the visual shapes a node can take.
type Shape int

const (
	ShapeRect Shape = iota
	ShapeRounded
	ShapeCircle
	ShapeDiamond
	ShapeCylinder
	ShapeHexagon
	ShapeParallelogram
)

// --- EdgeStyle ---

// EdgeStyle enumerates the visual styles for edges.
type EdgeStyle int

const (
	EdgeSolid EdgeStyle = iota
	EdgeDashed
	EdgeDotted
	EdgeThick
)

// --- EdgeDirection ---

// EdgeDirection controls which end(s) of an edge have arrowheads.
type EdgeDirection int

const (
	EdgeForward  EdgeDirection = iota // arrow at target (default)
	EdgeBackward                      // arrow at source
	EdgeBoth                          // arrows at both ends
	EdgeNone                          // no arrowhead at either end
)

// --- InteractionStyle ---

// InteractionStyle enumerates the visual styles for sequence diagram interactions.
type InteractionStyle int

const (
	InteractionSolid  InteractionStyle = iota // solid line, filled arrowhead
	InteractionDashed                         // dashed line, filled arrowhead
	InteractionAsync                          // solid line, open arrowhead
)

// --- FragmentType ---

// FragmentType enumerates the types of sequence diagram fragments.
type FragmentType int

const (
	FragmentAlt FragmentType = iota
	FragmentOpt
	FragmentLoop
	FragmentPar
	FragmentBreak
)

// --- Relation ---

// Relation is a class-diagram relation kind. RelationNone marks a plain
// flow edge; every class relation has a concrete kind (the parser maps an
// absent kind to RelationAssociation).
type Relation int

const (
	RelationNone Relation = iota
	RelationAssociation
	RelationInheritance
	RelationRealization
	RelationDependency
	RelationAggregation
	RelationComposition
)
