// Package model defines the core data types that flow through the rendering pipeline.
// There are two groups of types:
//   - Input model: what schema validation produces and the layout engine consumes
//   - Positioned model: what the layout engine produces and the renderer consumes
package model

// Graph is the input representation of a flow diagram.
type Graph struct {
	Title     string // diagram title, rendered as a band above the diagram; never enters layout
	Direction Direction
	Nodes     []Node
	Edges     []Edge
	Groups    []Group
	Legend    *Legend
}

// Node is a vertex in the flow diagram.
type Node struct {
	ID      string
	Label   string
	Shape   Shape
	Color   string
	Members *Members
}

// Edge is a directed connection between two nodes.
type Edge struct {
	// ID is the stable edge identifier: explicit from the spec, else derived "from->to#n".
	ID        string
	From      string
	To        string
	Label     string
	Style     EdgeStyle
	Direction EdgeDirection
	Color     string

	// Flat marks an edge that carries no order: the layout leaves it out of
	// the layering and routes it on the composed layout (S9, Flat edges),
	// or, when no route is clean, lays it out as an ordinary edge and marks
	// it (PositionedEdge.FlatRanked). Always false on a class relation.
	Flat bool

	// Class relations (Phase 4). Relation is RelationNone on a flow edge.
	Relation Relation
	FromCard string
	ToCard   string
	Directed bool // meaningful only when Relation != RelationNone
}

// Group is a named collection of nodes rendered as a labeled bounding box.
type Group struct {
	ID       string
	Label    string
	Contains []string // node IDs
	Children []string // child group IDs
	Depth    int      // 1 = top-level, 2 = nested, 3 = deeply nested
	Color    string
}
