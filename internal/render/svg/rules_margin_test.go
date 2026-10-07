package svg_test

import (
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/pipeline"
)

// Rendering-layer coverage for C16 (Margins) and a renderer convention with
// no rule id in the layout rules spec (no endpoint dots).
//
// These can't be tested at the JSON-layout layer: C16 only manifests in the
// final SVG viewBox and element positions, and the endpoint-dot check is a
// pure rendering concern (no <circle> elements at edge endpoints).

// renderFlowFromJSON runs the flow pipeline end-to-end: parse JSON, lay out
// with the layered engine, render to SVG, returning the serialized document.
// Helper shared by the tests in this file.
func renderFlowFromJSON(t *testing.T, jsonSpec string) string {
	t.Helper()

	svg, err := pipeline.RenderFlow(context.Background(), []byte(jsonSpec))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return svg
}

// extractViewBox returns the (x, y, w, h) of the SVG viewBox attribute.
func extractViewBox(t *testing.T, svg string) (x, y, w, h float64) {
	t.Helper()
	re := regexp.MustCompile(`viewBox="([^"]+)"`)
	m := re.FindStringSubmatch(svg)
	if m == nil {
		t.Fatalf("no viewBox attribute found in SVG")
	}
	parts := strings.Fields(m[1])
	if len(parts) != 4 {
		t.Fatalf("viewBox has %d parts, want 4: %q", len(parts), m[1])
	}
	var err error
	if x, err = strconv.ParseFloat(parts[0], 64); err != nil {
		t.Fatalf("parse viewBox x: %v", err)
	}
	if y, err = strconv.ParseFloat(parts[1], 64); err != nil {
		t.Fatalf("parse viewBox y: %v", err)
	}
	if w, err = strconv.ParseFloat(parts[2], 64); err != nil {
		t.Fatalf("parse viewBox w: %v", err)
	}
	if h, err = strconv.ParseFloat(parts[3], 64); err != nil {
		t.Fatalf("parse viewBox h: %v", err)
	}
	return
}

// collectNodeRects returns every non-background <rect ...> found in the SVG
// as (x, y, w, h) quads. The first rect in the document is the background
// (full viewBox fill) and is skipped.
func collectNodeRects(svg string) [][4]float64 {
	// Match self-closing <rect ... /> with at least x/y/width/height attrs.
	re := regexp.MustCompile(`<rect\s+x="([0-9.-]+)"\s+y="([0-9.-]+)"\s+width="([0-9.-]+)"\s+height="([0-9.-]+)"`)
	matches := re.FindAllStringSubmatch(svg, -1)
	out := make([][4]float64, 0, len(matches))
	for _, m := range matches {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		w, _ := strconv.ParseFloat(m[3], 64)
		h, _ := strconv.ParseFloat(m[4], 64)
		out = append(out, [4]float64{x, y, w, h})
	}
	return out
}

// TestC16_1_DiagramMargin_MinimumGutter checks C16.1: every node box lies
// at least 20px inside the canvas, on all four sides.
func TestC16_1_DiagramMargin_MinimumGutter(t *testing.T) {
	const spec = `{
		"type": "flow",
		"direction": "DOWN",
		"nodes": [
			{"id": "a", "label": "A", "shape": "rect"},
			{"id": "b", "label": "B", "shape": "rect"}
		],
		"edges": [{"from": "a", "to": "b"}]
	}`
	svg := renderFlowFromJSON(t, spec)

	vbX, vbY, vbW, vbH := extractViewBox(t, svg)
	rects := collectNodeRects(svg)
	if len(rects) < 2 {
		// The first rect is the background; we need at least 1 content rect.
		t.Fatalf("expected >= 2 rects (background + content), got %d", len(rects))
	}

	// Skip index 0 (background rect spans the full viewBox).
	const minMargin = 20.0
	const eps = 0.01
	vbRight := vbX + vbW
	vbBottom := vbY + vbH
	for i, r := range rects[1:] {
		x, y, w, h := r[0], r[1], r[2], r[3]
		if x-vbX < minMargin-eps {
			t.Errorf("rect[%d] left gutter = %g, want >= %g", i+1, x-vbX, minMargin)
		}
		if y-vbY < minMargin-eps {
			t.Errorf("rect[%d] top gutter = %g, want >= %g", i+1, y-vbY, minMargin)
		}
		if vbRight-(x+w) < minMargin-eps {
			t.Errorf("rect[%d] right gutter = %g, want >= %g", i+1, vbRight-(x+w), minMargin)
		}
		if vbBottom-(y+h) < minMargin-eps {
			t.Errorf("rect[%d] bottom gutter = %g, want >= %g", i+1, vbBottom-(y+h), minMargin)
		}
	}
}

// TestC16_2_DiagramMargin_Exact checks C16.2: the canvas is tight, so on
// each of the four sides some geometry lies exactly 20px from the edge.
func TestC16_2_DiagramMargin_Exact(t *testing.T) {
	const spec = `{
		"type": "flow",
		"direction": "DOWN",
		"nodes": [
			{"id": "a", "label": "A", "shape": "rect"},
			{"id": "b", "label": "B", "shape": "rect"}
		],
		"edges": [{"from": "a", "to": "b"}]
	}`
	svg := renderFlowFromJSON(t, spec)

	vbX, vbY, vbW, vbH := extractViewBox(t, svg)
	rects := collectNodeRects(svg)
	if len(rects) < 2 {
		t.Fatalf("expected >= 2 rects (background + content), got %d", len(rects))
	}

	// Find the outermost content extents (skip index 0 = background).
	minL, minT := vbX+vbW, vbY+vbH
	maxR, maxB := vbX, vbY
	for _, r := range rects[1:] {
		x, y, w, h := r[0], r[1], r[2], r[3]
		if x < minL {
			minL = x
		}
		if y < minT {
			minT = y
		}
		if x+w > maxR {
			maxR = x + w
		}
		if y+h > maxB {
			maxB = y + h
		}
	}

	const want = 20.0
	const eps = 0.01
	leftGutter := minL - vbX
	topGutter := minT - vbY
	rightGutter := (vbX + vbW) - maxR
	bottomGutter := (vbY + vbH) - maxB

	if math.Abs(leftGutter-want) > eps {
		t.Errorf("left gutter = %g, want %g", leftGutter, want)
	}
	if math.Abs(topGutter-want) > eps {
		t.Errorf("top gutter = %g, want %g", topGutter, want)
	}
	if math.Abs(rightGutter-want) > eps {
		t.Errorf("right gutter = %g, want %g", rightGutter, want)
	}
	if math.Abs(bottomGutter-want) > eps {
		t.Errorf("bottom gutter = %g, want %g", bottomGutter, want)
	}
}

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
	svg := renderFlowFromJSON(t, spec)

	// The only legitimate <circle> elements in a flow SVG are:
	//   * circle-shape nodes (not used in this spec, all rects)
	//   * marker defs (no <circle> inside diago arrow markers)
	// Anything else is an endpoint dot.
	if strings.Contains(svg, "<circle") {
		t.Errorf("SVG contains <circle> element, endpoint dots are forbidden")
	}
}
