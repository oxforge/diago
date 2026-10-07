package theme

// jsonFontStyle mirrors FontStyle for JSON decoding.
type jsonFontStyle struct {
	Family string  `json:"family"`
	Size   float64 `json:"size"`
	Weight int     `json:"weight"`
	Color  string  `json:"color"`
}

// jsonPadding mirrors Padding for JSON decoding.
type jsonPadding struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// jsonNodeStyle mirrors NodeStyle for JSON decoding.
type jsonNodeStyle struct {
	Fill         string        `json:"fill"`
	Stroke       string        `json:"stroke"`
	StrokeWidth  float64       `json:"stroke_width"`
	CornerRadius float64       `json:"corner_radius"`
	Font         jsonFontStyle `json:"font"`
	Padding      jsonPadding   `json:"padding"`
}

// jsonEdgeAppearance mirrors EdgeAppearance for JSON decoding.
type jsonEdgeAppearance struct {
	Stroke      string        `json:"stroke"`
	StrokeWidth float64       `json:"stroke_width"`
	ArrowSize   float64       `json:"arrow_size"`
	LabelFont   jsonFontStyle `json:"label_font"`
}

// jsonGroupStyle mirrors GroupStyle for JSON decoding.
type jsonGroupStyle struct {
	Fill        string        `json:"fill"`
	Stroke      string        `json:"stroke"`
	StrokeWidth float64       `json:"stroke_width"`
	StrokeDash  string        `json:"stroke_dash"`
	LabelFont   jsonFontStyle `json:"label_font"`
	Padding     float64       `json:"padding"`
}

// jsonActorStyle mirrors ActorStyle for JSON decoding.
type jsonActorStyle struct {
	Fill        string        `json:"fill"`
	Stroke      string        `json:"stroke"`
	StrokeWidth float64       `json:"stroke_width"`
	Font        jsonFontStyle `json:"font"`
}

// jsonFragmentStyle mirrors FragmentStyle for JSON decoding.
type jsonFragmentStyle struct {
	Fill        string        `json:"fill"`
	Stroke      string        `json:"stroke"`
	StrokeWidth float64       `json:"stroke_width"`
	StrokeDash  string        `json:"stroke_dash"`
	LabelFont   jsonFontStyle `json:"label_font"`
}

// jsonActivationStyle mirrors ActivationStyle for JSON decoding.
type jsonActivationStyle struct {
	Fill        string  `json:"fill"`
	Stroke      string  `json:"stroke"`
	StrokeWidth float64 `json:"stroke_width"`
	Width       float64 `json:"width"`
}

// jsonDiffStyle mirrors DiffStyle for JSON decoding.
type jsonDiffStyle struct {
	Added   string `json:"added"`
	Changed string `json:"changed"`
	Removed string `json:"removed"`
}

// jsonClassStyle mirrors ClassStyle for JSON decoding.
type jsonClassStyle struct {
	MemberFont     jsonFontStyle `json:"member_font"`
	SeparatorWidth float64       `json:"separator_width"`
}

// jsonTheme is the top-level JSON representation of a Theme.
type jsonTheme struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Style       string              `json:"style"`
	Background  string              `json:"background"`
	Node        jsonNodeStyle       `json:"node"`
	Edge        jsonEdgeAppearance  `json:"edge"`
	Group       jsonGroupStyle      `json:"group"`
	Actor       jsonActorStyle      `json:"actor"`
	Fragment    jsonFragmentStyle   `json:"fragment"`
	Activation  jsonActivationStyle `json:"activation"`
	Class       jsonClassStyle      `json:"class"`
	Colors      map[string]string   `json:"colors"`
	Diff        *jsonDiffStyle      `json:"diff"`
}

// toTheme converts the JSON representation to the Go Theme type.
func toTheme(jt jsonTheme) Theme {
	return Theme{
		Name:        jt.Name,
		Description: jt.Description,
		Style:       jt.Style,
		Background:  jt.Background,
		Node: NodeStyle{
			Fill:         jt.Node.Fill,
			Stroke:       jt.Node.Stroke,
			StrokeWidth:  jt.Node.StrokeWidth,
			CornerRadius: jt.Node.CornerRadius,
			Font:         toFontStyle(jt.Node.Font),
			Padding:      Padding{X: jt.Node.Padding.X, Y: jt.Node.Padding.Y},
		},
		Edge: EdgeAppearance{
			Stroke:      jt.Edge.Stroke,
			StrokeWidth: jt.Edge.StrokeWidth,
			ArrowSize:   jt.Edge.ArrowSize,
			LabelFont:   toFontStyle(jt.Edge.LabelFont),
		},
		Group: GroupStyle{
			Fill:        jt.Group.Fill,
			Stroke:      jt.Group.Stroke,
			StrokeWidth: jt.Group.StrokeWidth,
			StrokeDash:  jt.Group.StrokeDash,
			LabelFont:   toFontStyle(jt.Group.LabelFont),
			Padding:     jt.Group.Padding,
		},
		Actor: ActorStyle{
			Fill:        jt.Actor.Fill,
			Stroke:      jt.Actor.Stroke,
			StrokeWidth: jt.Actor.StrokeWidth,
			Font:        toFontStyle(jt.Actor.Font),
		},
		Fragment: FragmentStyle{
			Fill:        jt.Fragment.Fill,
			Stroke:      jt.Fragment.Stroke,
			StrokeWidth: jt.Fragment.StrokeWidth,
			StrokeDash:  jt.Fragment.StrokeDash,
			LabelFont:   toFontStyle(jt.Fragment.LabelFont),
		},
		Activation: ActivationStyle{
			Fill:        jt.Activation.Fill,
			Stroke:      jt.Activation.Stroke,
			StrokeWidth: jt.Activation.StrokeWidth,
			Width:       jt.Activation.Width,
		},
		Class: ClassStyle{
			MemberFont:     toFontStyle(jt.Class.MemberFont),
			SeparatorWidth: jt.Class.SeparatorWidth,
		},
		Colors: jt.Colors,
		Diff:   toDiffStyle(jt.Diff, jt.Colors),
	}
}

func toFontStyle(jf jsonFontStyle) FontStyle {
	return FontStyle(jf)
}

// toDiffStyle takes the explicit diff block, falling back per field to the
// palette's green (added), blue (changed) and red (removed) so custom
// themes without the block keep working.
func toDiffStyle(d *jsonDiffStyle, colors map[string]string) DiffStyle {
	out := DiffStyle{Added: colors["green"], Changed: colors["blue"], Removed: colors["red"]}
	if d != nil {
		if d.Added != "" {
			out.Added = d.Added
		}
		if d.Changed != "" {
			out.Changed = d.Changed
		}
		if d.Removed != "" {
			out.Removed = d.Removed
		}
	}
	return out
}
