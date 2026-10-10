// Package theme defines the visual styling system for Diago diagrams.
// Themes are self-contained: each theme specifies all visual properties
// without inheritance or overrides from other themes.
package theme

// Theme is the complete visual specification for rendering a diagram. Its
// JSON tags are the theme file's keys.
type Theme struct {
	Name        string            `json:"name"`        // theme name (e.g., "default", "midnight", "sketch")
	Description string            `json:"description"` // human-readable description
	Style       string            `json:"style"`       // "clean" or "sketch" — controls rendering behavior
	Background  string            `json:"background"`
	Node        NodeStyle         `json:"node"`
	Edge        EdgeAppearance    `json:"edge"`
	Group       GroupStyle        `json:"group"`
	Actor       ActorStyle        `json:"actor"`      // sequence diagram actor header boxes and lifelines
	Fragment    FragmentStyle     `json:"fragment"`   // sequence diagram fragment bounding boxes
	Activation  ActivationStyle   `json:"activation"` // sequence diagram activation boxes
	Class       ClassStyle        `json:"class"`      // class diagram record boxes
	Colors      map[string]string `json:"colors"`     // named color palette: "red" → "#e03131", etc.
	// Diff holds the accent colors a diff render uses for added, changed
	// and removed elements. Always populated: an explicit "diff" block in
	// the theme file, else colors.green / colors.blue / colors.red.
	Diff DiffStyle `json:"diff"`
}

// NodeStyle defines how nodes are rendered.
type NodeStyle struct {
	Fill         string    `json:"fill"`
	Stroke       string    `json:"stroke"`
	StrokeWidth  float64   `json:"stroke_width"`
	CornerRadius float64   `json:"corner_radius"`
	Font         FontStyle `json:"font"`
	Padding      Padding   `json:"padding"`
}

// EdgeAppearance defines how edges and their labels are rendered.
// Named EdgeAppearance (not EdgeStyle) to avoid collision with model.EdgeStyle enum.
type EdgeAppearance struct {
	Stroke      string    `json:"stroke"`
	StrokeWidth float64   `json:"stroke_width"`
	ArrowSize   float64   `json:"arrow_size"`
	LabelFont   FontStyle `json:"label_font"`
}

// GroupStyle defines how group bounding boxes are rendered.
type GroupStyle struct {
	Fill        string    `json:"fill"`
	Stroke      string    `json:"stroke"`
	StrokeWidth float64   `json:"stroke_width"`
	StrokeDash  string    `json:"stroke_dash"`
	LabelFont   FontStyle `json:"label_font"`
	Padding     float64   `json:"padding"`
}

// FontStyle defines the typographic properties for a text element.
type FontStyle struct {
	Family string  `json:"family"`
	Size   float64 `json:"size"`
	Weight int     `json:"weight"`
	Color  string  `json:"color"`
}

// Padding defines horizontal and vertical inset values.
type Padding struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ActorStyle defines how sequence diagram actors (header boxes and lifelines) are rendered.
type ActorStyle struct {
	Fill        string    `json:"fill"`
	Stroke      string    `json:"stroke"`
	StrokeWidth float64   `json:"stroke_width"`
	Font        FontStyle `json:"font"`
}

// FragmentStyle defines how sequence diagram fragment bounding boxes are rendered.
type FragmentStyle struct {
	Fill        string    `json:"fill"` // background fill (semi-transparent)
	Stroke      string    `json:"stroke"`
	StrokeWidth float64   `json:"stroke_width"`
	StrokeDash  string    `json:"stroke_dash"`
	LabelFont   FontStyle `json:"label_font"`
}

// ActivationStyle defines how sequence diagram activation boxes are rendered.
type ActivationStyle struct {
	Fill        string  `json:"fill"`
	Stroke      string  `json:"stroke"`
	StrokeWidth float64 `json:"stroke_width"`
	Width       float64 `json:"width"` // width of activation box
}

// DiffStyle is the diff palette: the stroke and text color of added,
// changed and removed elements.
type DiffStyle struct {
	Added   string `json:"added"`
	Changed string `json:"changed"`
	Removed string `json:"removed"`
}

// orPalette is d with each color it leaves empty taken from the palette:
// green for added, blue for changed, red for removed, so a theme without
// a diff block, or with part of one, keeps working.
func (d DiffStyle) orPalette(colors map[string]string) DiffStyle {
	if d.Added == "" {
		d.Added = colors["green"]
	}
	if d.Changed == "" {
		d.Changed = colors["blue"]
	}
	if d.Removed == "" {
		d.Removed = colors["red"]
	}
	return d
}

// ClassStyle defines how class-diagram record boxes are rendered: the
// member-line font and the compartment separator width. The separator
// stroke is the box stroke.
type ClassStyle struct {
	MemberFont     FontStyle `json:"member_font"`
	SeparatorWidth float64   `json:"separator_width"`
}
