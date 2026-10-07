package model

// --- Sequence diagram input model ---

// SequenceDiagram is the input representation of a sequence diagram.
type SequenceDiagram struct {
	Title        string
	Actors       []Actor
	Interactions []Interaction
	Fragments    []Fragment
	Activations  bool // true by default
}

// Actor is a participant in a sequence diagram.
type Actor struct {
	ID    string
	Label string
	Color string
}

// Interaction is a message between two actors.
type Interaction struct {
	From  string
	To    string
	Label string
	Style InteractionStyle
	Color string
}

// FragmentSection is one section of a sequence diagram fragment (e.g., one branch of alt).
type FragmentSection struct {
	Label string
	Start int // interaction index (0-based, inclusive)
	End   int // interaction index (0-based, inclusive)
}

// Fragment is a labeled region spanning a range of interactions and a set of actors.
type Fragment struct {
	Type     FragmentType
	Over     []string          // actor IDs
	Sections []FragmentSection // at least one; alt may have multiple
}

// --- Sequence diagram positioned model ---

// PositionedSequence is the output of the sequence diagram layout engine.
type PositionedSequence struct {
	Title        string                  `json:"title,omitempty"`
	Actors       []PositionedActor       `json:"actors"`
	Interactions []PositionedInteraction `json:"interactions"`
	Fragments    []PositionedFragment    `json:"fragments"`
	Activations  []PositionedActivation  `json:"activations"`
	Width        float64                 `json:"width"`
	Height       float64                 `json:"height"`
}

// PositionedActor is an actor with computed header box and lifeline coordinates.
type PositionedActor struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	BoxX       float64 `json:"boxX"` // top-left of header box
	BoxY       float64 `json:"boxY"` // top-left of header box
	BoxW       float64 `json:"boxW"`
	BoxH       float64 `json:"boxH"`
	LineX      float64 `json:"lineX"`      // lifeline X (center of box)
	LineTop    float64 `json:"lineTop"`    // where lifeline starts (bottom of box)
	LineBottom float64 `json:"lineBottom"` // where lifeline ends
	Color      string  `json:"color,omitempty"`
}

// PositionedInteraction is an interaction with computed arrow coordinates.
type PositionedInteraction struct {
	From       string           `json:"from"`
	To         string           `json:"to"`
	Label      string           `json:"label,omitempty"`
	Style      InteractionStyle `json:"style"`
	Y          float64          `json:"y"`     // vertical position of the arrow
	FromX      float64          `json:"fromX"` // start X
	ToX        float64          `json:"toX"`   // end X
	IsSelf     bool             `json:"isSelf,omitempty"`
	SelfPoints []Point          `json:"selfPoints,omitempty"` // only for self-messages: the U-shaped loop waypoints
	Color      string           `json:"color,omitempty"`
}

// PositionedFragment is a fragment with computed bounding box.
type PositionedFragment struct {
	Type     FragmentType                `json:"type"`
	X        float64                     `json:"x"`
	Y        float64                     `json:"y"`
	Width    float64                     `json:"width"`
	Height   float64                     `json:"height"`
	Sections []PositionedFragmentSection `json:"sections"`
}

// PositionedFragmentSection is a section within a fragment with its divider position.
type PositionedFragmentSection struct {
	Label string  `json:"label"`
	Y     float64 `json:"y"` // Y of the divider line (or top of first section = fragment Y)
}

// PositionedActivation is an activation box on an actor's lifeline.
type PositionedActivation struct {
	ActorID string  `json:"actorId"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"` // top
	Width   float64 `json:"width"`
	Height  float64 `json:"height"` // bottom - top
}
