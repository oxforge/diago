// Package theme defines the visual styling system for Diago diagrams.
// Themes are self-contained: each theme specifies all visual properties
// without inheritance or overrides from other themes.
package theme

import "fmt"

// Theme is the complete visual specification for rendering a diagram.
type Theme struct {
	Name        string // theme name (e.g., "default", "midnight", "sketch")
	Description string // human-readable description
	Style       string // "clean" or "sketch" — controls rendering behavior
	Background  string
	Node        NodeStyle
	Edge        EdgeAppearance
	Group       GroupStyle
	Actor       ActorStyle        // sequence diagram actor header boxes and lifelines
	Fragment    FragmentStyle     // sequence diagram fragment bounding boxes
	Activation  ActivationStyle   // sequence diagram activation boxes
	Class       ClassStyle        // class diagram record boxes
	Colors      map[string]string // named color palette: "red" → "#e03131", etc.
	// Diff holds the two accent colors a diff render uses for added and
	// changed elements. Always populated: an explicit
	// "diff" block in the theme file, else colors.green / colors.orange.
	Diff DiffStyle
}

// NodeStyle defines how nodes are rendered.
type NodeStyle struct {
	Fill         string
	Stroke       string
	StrokeWidth  float64
	CornerRadius float64
	Font         FontStyle
	Padding      Padding
}

// EdgeAppearance defines how edges and their labels are rendered.
// Named EdgeAppearance (not EdgeStyle) to avoid collision with model.EdgeStyle enum.
type EdgeAppearance struct {
	Stroke      string
	StrokeWidth float64
	ArrowSize   float64
	LabelFont   FontStyle
}

// GroupStyle defines how group bounding boxes are rendered.
type GroupStyle struct {
	Fill        string
	Stroke      string
	StrokeWidth float64
	StrokeDash  string
	LabelFont   FontStyle
	Padding     float64
}

// FontStyle defines the typographic properties for a text element.
type FontStyle struct {
	Family string
	Size   float64
	Weight int
	Color  string
}

// Padding defines horizontal and vertical inset values.
type Padding struct{ X, Y float64 }

// ActorStyle defines how sequence diagram actors (header boxes and lifelines) are rendered.
type ActorStyle struct {
	Fill        string
	Stroke      string
	StrokeWidth float64
	Font        FontStyle
}

// FragmentStyle defines how sequence diagram fragment bounding boxes are rendered.
type FragmentStyle struct {
	Fill        string // background fill (semi-transparent)
	Stroke      string
	StrokeWidth float64
	StrokeDash  string
	LabelFont   FontStyle
}

// ActivationStyle defines how sequence diagram activation boxes are rendered.
type ActivationStyle struct {
	Fill        string
	Stroke      string
	StrokeWidth float64
	Width       float64 // width of activation box
}

// DiffStyle is the diff palette: added and changed strokes/text.
type DiffStyle struct {
	Added   string
	Changed string
}

// ClassStyle defines how class-diagram record boxes are rendered: the
// member-line font and the compartment separator width. The separator
// stroke is the box stroke.
type ClassStyle struct {
	MemberFont     FontStyle
	SeparatorWidth float64
}

// DefaultTheme returns the default light theme.
// It delegates to Load("default") for the canonical JSON source.
func DefaultTheme() Theme {
	th, err := Load("default")
	if err != nil {
		// Panic is intentional: the default theme is embedded at compile time.
		// A load failure here means the binary itself is corrupt.
		panic(fmt.Sprintf("theme: failed to load embedded default theme: %v", err))
	}
	return th
}
