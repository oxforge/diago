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
	// StatusPaint makes status the only color on strokes, wires and text:
	// an element's own color sets its fill and nothing else, and an edge,
	// relation or interaction ignores its color. A diff render sets it.
	StatusPaint bool
	// Changes is the change list drawn under the diagram, one row each;
	// none for a plain render or a diff with nothing changed.
	Changes []ChangeLine
}

func (o *RenderOptions) class(kind, id string) string {
	if o == nil || o.Class == nil {
		return ""
	}
	return o.Class(kind, id)
}

// ownStrokes reports whether an element's own color reaches its strokes,
// wires and text: always, except in a status-paint render.
func (o *RenderOptions) ownStrokes() bool { return o == nil || !o.StatusPaint }

// changes returns the change list to draw; nil for a plain render.
func (o *RenderOptions) changes() []ChangeLine {
	if o == nil {
		return nil
	}
	return o.Changes
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
// because diago's shapes are groups of primitives; a label's backing is no
// shape and takes no status stroke. A removed element is dashed, faded and
// struck through in the removed color.
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
.removed { opacity: 0.6; }
%s { stroke: %s; stroke-dasharray: 6 4; }
.removed text { fill: %s; text-decoration: line-through; }
.added rect.backing, .changed rect.backing, .removed rect.backing { stroke: none; }
`, shapes("added"), th.Diff.Added, th.Diff.Added,
		shapes("changed"), th.Diff.Changed, th.Diff.Changed,
		shapes("removed"), th.Diff.Removed, th.Diff.Removed)
}

// ClassDiffStyles is DiffStyles plus the per-member rules of a class diff;
// a removed member line takes its fading from .removed.
func ClassDiffStyles(th theme.Theme) string {
	return DiffStyles(th) + fmt.Sprintf(`text.member.added { fill: %s; font-weight: 600; }
text.member.changed { fill: %s; font-weight: 600; }
text.member.removed { fill: %s; text-decoration: line-through; }
`, th.Diff.Added, th.Diff.Changed, th.Diff.Removed)
}

// openStatusMarkerID names the open (async) marker an interaction of the
// given class references.
func openStatusMarkerID(class string) string { return "diago-arrow-open-" + class }

// SequenceDiffStyles is DiffStyles plus the selectors sequence renders
// need: lifelines, dividers and self-loops are <line> elements. A
// section's group sits inside its fragment's, where .changed and .added
// (or .removed) tie on specificity; the nested rules let an added or
// removed section of a changed fragment keep its own color.
func SequenceDiffStyles(th theme.Theme) string {
	return DiffStyles(th) + fmt.Sprintf(`.added line { stroke: %[1]s; stroke-width: 2.5; }
.changed line { stroke: %[2]s; stroke-width: 2.5; }
.removed line { stroke: %[3]s; stroke-dasharray: 6 4; }
.changed .added line { stroke: %[1]s; }
.changed .added text { fill: %[1]s; }
.changed .removed line { stroke: %[3]s; }
.changed .removed text { fill: %[3]s; }
`, th.Diff.Added, th.Diff.Changed, th.Diff.Removed)
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
