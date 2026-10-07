package svg

import (
	"fmt"
	"sort"

	"github.com/oxforge/diago/internal/theme"
)

// ClassDimmed is the class of an unchanged element in a diff render.
const ClassDimmed = "dimmed"

// RenderOptions parameterizes a status-aware render (Phase 2b). A nil
// *RenderOptions is exactly today's output.
type RenderOptions struct {
	// Class returns the CSS class for an element: kind is "node", "edge",
	// "group" or "member" for a flow or class render, or "actor", "lifeline",
	// "interaction", "fragment" or "section" for a sequence render; id is the
	// node id, the logical edge id, the group id, "<node id>/<attributes|
	// methods>/<index>" for a member line, the actor id, the interaction
	// index, the fragment index, or "<fragment>/<section>". "" means no class
	// attribute.
	Class func(kind, id string) string
	// Styles is an extra <style> block emitted after the font faces.
	Styles string
	// MarkerFills maps a class to the arrowhead fill it gets. A marker pair
	// diago-arrow-<class> / diago-arrow-thick-<class> is emitted for a flow
	// render, or diago-arrow-<class> / diago-arrow-open-<class> for a
	// sequence render, per class an edge or interaction actually uses.
	MarkerFills map[string]string
}

func (o *RenderOptions) class(kind, id string) string {
	if o == nil || o.Class == nil {
		return ""
	}
	return o.Class(kind, id)
}

// statusMarkerID names the marker an edge of the given class references.
func statusMarkerID(class string, thick bool) string {
	if thick {
		return "diago-arrow-thick-" + class
	}
	return "diago-arrow-" + class
}

// DiffStyles is the style block of a diff render, parameterized by the
// theme's diff colors. Selectors go by element type under a classed group
// because diago's shapes are groups of primitives; a group title's backing
// is no shape and takes no status stroke.
func DiffStyles(th theme.Theme) string {
	shapes := func(class string) string {
		return fmt.Sprintf(".%[1]s rect, .%[1]s polygon, .%[1]s circle, .%[1]s ellipse, .%[1]s path, .%[1]s polyline", class)
	}
	return fmt.Sprintf(`
.dimmed { opacity: 0.5; }
%s { stroke: %s; stroke-width: 2.5; }
.added text { fill: %s; font-weight: 600; }
%s { stroke: %s; stroke-width: 2.5; }
.changed text { fill: %s; font-weight: 600; }
.removed { opacity: 0.35; }
%s { stroke-dasharray: 6 4; }
.added rect.backing, .changed rect.backing { stroke: none; }
`, shapes("added"), th.Diff.Added, th.Diff.Added,
		shapes("changed"), th.Diff.Changed, th.Diff.Changed,
		shapes("removed"))
}

// ClassDiffStyles is DiffStyles plus the per-member rules of a class diff.
func ClassDiffStyles(th theme.Theme) string {
	return DiffStyles(th) + fmt.Sprintf(`text.member.added { fill: %s; font-weight: 600; }
text.member.changed { fill: %s; font-weight: 600; }
text.member.removed { opacity: 0.35; text-decoration: line-through; }
`, th.Diff.Added, th.Diff.Changed)
}

// openStatusMarkerID names the open (async) marker an interaction of the
// given class references.
func openStatusMarkerID(class string) string { return "diago-arrow-open-" + class }

// SequenceDiffStyles is DiffStyles plus the selectors sequence renders
// need: lifelines, dividers and self-loops are <line> elements.
func SequenceDiffStyles(th theme.Theme) string {
	return DiffStyles(th) + fmt.Sprintf(`.added line { stroke: %s; stroke-width: 2.5; }
.changed line { stroke: %s; stroke-width: 2.5; }
.removed line { stroke-dasharray: 6 4; }
`, th.Diff.Added, th.Diff.Changed)
}

// sortedKeys keeps marker emission deterministic.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
