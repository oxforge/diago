package theme

import (
	"fmt"
	"regexp"
)

// colorRE matches #rrggbb or #rrggbbaa hex colors.
var colorRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)

// validStyles is the set of allowed theme styles.
var validStyles = map[string]bool{
	"clean":  true,
	"sketch": true,
}

// validFontFamilies is the set of supported font families.
var validFontFamilies = map[string]bool{
	"Inter":    true,
	"Pangolin": true,
}

// Validate checks that a Theme has valid values.
// Returns a descriptive error for the first violation found.
func Validate(th Theme) error {
	if err := validateColor("background", th.Background); err != nil {
		return err
	}
	if !validStyles[th.Style] {
		return fmt.Errorf("style %q is not valid; must be one of: clean, sketch", th.Style)
	}

	// Node
	if err := validateColor("node.fill", th.Node.Fill); err != nil {
		return err
	}
	if err := validateColor("node.stroke", th.Node.Stroke); err != nil {
		return err
	}
	if err := validatePositive("node.stroke_width", th.Node.StrokeWidth); err != nil {
		return err
	}
	if err := validateFont("node.font", th.Node.Font); err != nil {
		return err
	}

	// Edge
	if err := validateColor("edge.stroke", th.Edge.Stroke); err != nil {
		return err
	}
	if err := validatePositive("edge.stroke_width", th.Edge.StrokeWidth); err != nil {
		return err
	}
	if err := validatePositive("edge.arrow_size", th.Edge.ArrowSize); err != nil {
		return err
	}
	if err := validateFont("edge.label_font", th.Edge.LabelFont); err != nil {
		return err
	}

	// Group
	if err := validateColor("group.fill", th.Group.Fill); err != nil {
		return err
	}
	if err := validateColor("group.stroke", th.Group.Stroke); err != nil {
		return err
	}
	if err := validatePositive("group.stroke_width", th.Group.StrokeWidth); err != nil {
		return err
	}
	if err := validateFont("group.label_font", th.Group.LabelFont); err != nil {
		return err
	}

	// Actor
	if err := validateColor("actor.fill", th.Actor.Fill); err != nil {
		return err
	}
	if err := validateColor("actor.stroke", th.Actor.Stroke); err != nil {
		return err
	}
	if err := validatePositive("actor.stroke_width", th.Actor.StrokeWidth); err != nil {
		return err
	}
	if err := validateFont("actor.font", th.Actor.Font); err != nil {
		return err
	}

	// Fragment
	if err := validateColor("fragment.fill", th.Fragment.Fill); err != nil {
		return err
	}
	if err := validateColor("fragment.stroke", th.Fragment.Stroke); err != nil {
		return err
	}
	if err := validatePositive("fragment.stroke_width", th.Fragment.StrokeWidth); err != nil {
		return err
	}
	if err := validateFont("fragment.label_font", th.Fragment.LabelFont); err != nil {
		return err
	}

	// Paddings
	if th.Node.Padding.X < 0 {
		return fmt.Errorf("field %q: value %v must be >= 0", "node.padding.x", th.Node.Padding.X)
	}
	if th.Node.Padding.Y < 0 {
		return fmt.Errorf("field %q: value %v must be >= 0", "node.padding.y", th.Node.Padding.Y)
	}
	if th.Group.Padding < minGroupPadding {
		return fmt.Errorf("field %q: value %v must be >= %v, the node clearance a group's own wires keep to its border (C6.2)", "group.padding", th.Group.Padding, minGroupPadding)
	}

	// Activation
	if err := validateColor("activation.fill", th.Activation.Fill); err != nil {
		return err
	}
	if err := validateColor("activation.stroke", th.Activation.Stroke); err != nil {
		return err
	}
	if err := validatePositive("activation.stroke_width", th.Activation.StrokeWidth); err != nil {
		return err
	}
	if err := validatePositive("activation.width", th.Activation.Width); err != nil {
		return err
	}

	// Class
	if err := validateFont("class.member_font", th.Class.MemberFont); err != nil {
		return err
	}
	if err := validatePositive("class.separator_width", th.Class.SeparatorWidth); err != nil {
		return err
	}

	// Palette
	requiredColors := []string{"red", "green", "blue", "yellow", "orange", "purple", "gray"}
	for _, name := range requiredColors {
		hex, ok := th.Colors[name]
		if !ok {
			return fmt.Errorf("colors palette missing required color %q", name)
		}
		if err := validateColor(fmt.Sprintf("colors.%s", name), hex); err != nil {
			return err
		}
	}

	// Diff palette (explicit or fallback; all three must be colors).
	for _, c := range []struct{ field, color string }{
		{"diff.added", th.Diff.Added}, {"diff.changed", th.Diff.Changed}, {"diff.removed", th.Diff.Removed},
	} {
		if err := validateColor(c.field, c.color); err != nil {
			return err
		}
	}

	return nil
}

// minGroupPadding is the least group padding the layout accepts: the node
// clearance of the screen profile (S14).
const minGroupPadding = 12

func validateColor(field, color string) error {
	if color == "" {
		return fmt.Errorf("field %q: color must not be empty", field)
	}
	if !colorRE.MatchString(color) {
		return fmt.Errorf("field %q: color %q is not a valid hex color (#rrggbb or #rrggbbaa)", field, color)
	}
	return nil
}

func validatePositive(field string, val float64) error {
	if val <= 0 {
		return fmt.Errorf("field %q: value %v must be > 0", field, val)
	}
	return nil
}

func validateFont(prefix string, f FontStyle) error {
	if !validFontFamilies[f.Family] {
		return fmt.Errorf("field %q: font family %q is not supported; must be one of: Inter, Pangolin", prefix+".family", f.Family)
	}
	if err := validatePositive(prefix+".size", f.Size); err != nil {
		return err
	}
	if err := validateColor(prefix+".color", f.Color); err != nil {
		return err
	}
	return nil
}
