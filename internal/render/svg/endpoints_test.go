package svg_test

import (
	"context"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/pipeline"
)

// TestNoEndpointDots checks a renderer convention with no rule id in
// the layout rules spec: a flow edge's polyline must not render
// <circle> elements at its endpoints. The wire itself is sufficient visual
// indication.
func TestNoEndpointDots(t *testing.T) {
	const spec = `{
		"type": "flow",
		"direction": "DOWN",
		"nodes": [
			{"id": "a", "label": "A", "shape": "rect"},
			{"id": "b", "label": "B", "shape": "rect"}
		],
		"edges": [{"from": "a", "to": "b"}]
	}`
	out, err := pipeline.RenderWithOptions(context.Background(), []byte(spec), pipeline.Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	svg := string(out)

	// The only legitimate <circle> elements in a flow SVG are:
	//   * circle-shape nodes (not used in this spec, all rects)
	//   * marker defs (no <circle> inside diago arrow markers)
	// Anything else is an endpoint dot.
	if strings.Contains(svg, "<circle") {
		t.Errorf("SVG contains <circle> element, endpoint dots are forbidden")
	}
}
