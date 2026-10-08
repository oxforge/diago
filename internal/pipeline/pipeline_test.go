package pipeline

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/schema"
)

func TestRenderFlowValidInput(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"direction": "DOWN",
		"nodes": [
			{"id": "a", "label": "A"},
			{"id": "b", "label": "B"}
		],
		"edges": [{"from": "a", "to": "b"}]
	}`)
	out, err := RenderFlow(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "<?xml"), "output should be XML")
	assert.Contains(t, out, "xmlns=\"http://www.w3.org/2000/svg\"")
}

func TestRenderFlowValidationError(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A", "shape": "octagon"}]
	}`)
	_, err := RenderFlow(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shape", "error should mention the invalid field")
}

func TestRenderFlowInvalidJSON(t *testing.T) {
	_, err := RenderFlow(context.Background(), []byte(`not json`))
	require.Error(t, err)
}

func TestRenderAutoDetectFlow(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}],
		"edges": [{"from": "a", "to": "b"}]
	}`)
	out, err := Render(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "<?xml"), "auto-detect should render flow diagram")
}

func TestRenderAutoDetectSequence(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}],
		"interactions": [{"from": "a", "to": "b", "label": "msg"}]
	}`)
	out, err := Render(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "<?xml"), "auto-detect should render sequence diagram")
}

func TestRenderAutoDetectUnknown(t *testing.T) {
	input := []byte(`{"type": "mindmap", "nodes": []}`)
	_, err := Render(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown diagram type")
}

func TestRenderSequenceValidInput(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "a", "label": "A"},
			{"id": "b", "label": "B"}
		],
		"interactions": [{"from": "a", "to": "b", "label": "hello"}]
	}`)
	out, err := RenderSequence(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "<?xml"), "output should be XML")
	assert.Contains(t, out, `xmlns="http://www.w3.org/2000/svg"`)
}

func TestRenderSequenceValidationError(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [],
		"interactions": []
	}`)
	_, err := RenderSequence(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "actors")
}

func TestThemesProduceDifferentOutput(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}],
		"edges": [{"from": "a", "to": "b", "label": "test"}]
	}`)

	defaultSVG, err := RenderWithTheme(context.Background(), input, "default")
	require.NoError(t, err)
	midnightSVG, err := RenderWithTheme(context.Background(), input, "midnight")
	require.NoError(t, err)
	sketchSVG, err := RenderWithTheme(context.Background(), input, "sketch")
	require.NoError(t, err)

	assert.NotEqual(t, defaultSVG, midnightSVG, "default and midnight should differ")
	assert.NotEqual(t, defaultSVG, sketchSVG, "default and sketch should differ")
	assert.NotEqual(t, midnightSVG, sketchSVG, "midnight and sketch should differ")
}

func TestSketchThemeHasPangolinFont(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}],
		"edges": [{"from": "a", "to": "b"}]
	}`)
	svg, err := RenderWithTheme(context.Background(), input, "sketch")
	require.NoError(t, err)
	assert.Contains(t, svg, "Pangolin")
	assert.Contains(t, svg, "font-family: 'Pangolin'")
}

func TestSketchThemeHasWobblyEdges(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}],
		"edges": [{"from": "a", "to": "b"}]
	}`)
	svg, err := RenderWithTheme(context.Background(), input, "sketch")
	require.NoError(t, err)
	// Sketch edges use subdivided L commands in a <path> instead of <polyline>.
	assert.Contains(t, svg, "<path d=\"M", "sketch edges should use path elements")
}

func TestDefaultThemeHasNoSketchEffects(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}],
		"edges": [{"from": "a", "to": "b"}]
	}`)
	svg, err := RenderWithTheme(context.Background(), input, "default")
	require.NoError(t, err)
	assert.NotContains(t, svg, "Pangolin")
}

func TestRenderWithOptionsThemeOnly(t *testing.T) {
	input := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`)
	data, err := RenderWithOptions(context.Background(), input, Options{Theme: "midnight"})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "<?xml"))
}

// themedSpecs are one spec per diagram type, each with a theme field and
// without one. The flow node's color is what the exporters take from a
// theme.
var themedSpecs = []struct{ kind, plain, themed string }{
	{"flow",
		`{"type":"flow","nodes":[{"id":"a","label":"A","color":"red"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","label":"x"}]}`,
		`{"type":"flow","theme":"midnight","nodes":[{"id":"a","label":"A","color":"red"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b","label":"x"}]}`},
	{"class",
		`{"type":"class","classes":[{"id":"a"},{"id":"b"}],"relations":[{"from":"a","to":"b"}]}`,
		`{"type":"class","theme":"midnight","classes":[{"id":"a"},{"id":"b"}],"relations":[{"from":"a","to":"b"}]}`},
	{"sequence",
		`{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"hi"}]}`,
		`{"type":"sequence","theme":"midnight","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"hi"}]}`},
}

// TestRenderWithOptions_SpecTheme: a render uses the Theme option when set,
// else the spec's theme field, else default, in every format a theme
// reaches.
func TestRenderWithOptions_SpecTheme(t *testing.T) {
	ctx := context.Background()
	for _, s := range themedSpecs {
		formats := []string{"svg"}
		if s.kind == "flow" {
			formats = append(formats, "drawio", "excalidraw")
		}
		for _, format := range formats {
			t.Run(s.kind+"/"+format, func(t *testing.T) {
				render := func(spec, theme string) string {
					t.Helper()
					out, err := RenderWithOptions(ctx, []byte(spec), Options{Format: format, Theme: theme})
					require.NoError(t, err)
					return string(out)
				}
				midnight := render(s.plain, "midnight")
				assert.NotEqual(t, render(s.plain, ""), midnight, "the two themes draw differently")
				assert.Equal(t, midnight, render(s.themed, ""), "the spec's field, without the option")
				assert.Equal(t, render(s.plain, "dark"), render(s.themed, "dark"), "the option over the field")
				assert.Equal(t, render(s.plain, "default"), render(s.plain, ""), "default, without either")
			})
		}
	}
}

// TestRenderWithOptions_UnknownSpecTheme: a theme the spec names that does
// not load is a validation error on the spec's field; text art takes no
// theme and never reads it. A theme the option names stays an error of the
// invocation.
func TestRenderWithOptions_UnknownSpecTheme(t *testing.T) {
	ctx := context.Background()
	for _, s := range themedSpecs {
		t.Run(s.kind, func(t *testing.T) {
			spec := []byte(strings.Replace(s.themed, `"midnight"`, `"nope"`, 1))
			_, err := RenderWithOptions(ctx, spec, Options{})
			var ve schema.ValidationErrors
			require.ErrorAs(t, err, &ve)
			require.Len(t, ve, 1)
			assert.Equal(t, "theme", ve[0].Field)
			assert.Contains(t, ve[0].Message, `"nope"`)

			_, err = RenderWithOptions(ctx, spec, Options{Format: "text"})
			require.NoError(t, err)
			_, err = RenderWithOptions(ctx, spec, Options{Theme: "default"})
			require.NoError(t, err, "the option over a field that does not load")

			_, err = RenderWithOptions(ctx, []byte(s.plain), Options{Theme: "nope"})
			require.Error(t, err)
			assert.False(t, errors.As(err, &ve), "an unknown option is not a spec error: %v", err)
		})
	}
}

func TestSpecThemeName(t *testing.T) {
	for _, tt := range []struct {
		name, spec, override, want string
	}{
		{"override over field", `{"theme":"dark"}`, "sketch", "sketch"},
		{"field", `{"theme":"dark"}`, "", "dark"},
		{"neither", `{"type":"flow"}`, "", "default"},
		{"empty field", `{"theme":""}`, "", "default"},
		{"invalid JSON", `nope`, "", "default"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, specThemeName([]byte(tt.spec), tt.override))
		})
	}
}

// TestRenderTheme: the theme name a render uses, checked as the render
// checks it, and none for text art.
func TestRenderTheme(t *testing.T) {
	name, err := RenderTheme([]byte(`{"theme":"dark"}`), "svg", "")
	require.NoError(t, err)
	assert.Equal(t, "dark", name)
	name, err = RenderTheme([]byte(`{"theme":"nope"}`), "txt", "")
	require.NoError(t, err)
	assert.Empty(t, name, "text art takes no theme")

	_, err = RenderTheme([]byte(`{"theme":"nope"}`), "png", "")
	var ve schema.ValidationErrors
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "theme", ve[0].Field)
	_, err = RenderTheme([]byte(`{}`), "svg", "nope")
	require.Error(t, err)
	assert.False(t, errors.As(err, &ve), "an unknown override is not a spec error: %v", err)
}

func TestRenderWithOptionsSketchTheme(t *testing.T) {
	input := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`)
	data, err := RenderWithOptions(context.Background(), input, Options{Theme: "sketch"})
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "<?xml"))
}

// TestRenderWithOptionsExportFlowOnly: draw.io and Excalidraw export cover
// flow diagrams only; sequence and class specs are refused by name.
func TestRenderWithOptionsExportFlowOnly(t *testing.T) {
	specs := map[string]string{
		"sequence": `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"interactions":[{"from":"a","to":"b","label":"hi"}]}`,
		"class":    `{"type":"class","classes":[{"id":"a","label":"A"}]}`,
	}
	for _, format := range []string{"drawio", "excalidraw"} {
		for kind, spec := range specs {
			t.Run(format+"/"+kind, func(t *testing.T) {
				_, err := RenderWithOptions(context.Background(), []byte(spec), Options{Format: format})
				require.Error(t, err)
				assert.Contains(t, err.Error(), "not supported for "+kind+" diagrams")
			})
		}
	}
}

// TestRenderWithOptionsExportAnchored: an export honours -previous and
// fills the LayoutHints sink like the SVG path.
func TestRenderWithOptionsExportAnchored(t *testing.T) {
	input := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`)
	var fresh, anchored model.LayoutHints
	_, err := RenderWithOptions(context.Background(), input, Options{Format: "drawio", LayoutHints: &fresh})
	require.NoError(t, err)
	require.NotEmpty(t, fresh.Scopes)
	_, err = RenderWithOptions(context.Background(), input, Options{Format: "excalidraw", Previous: &fresh, LayoutHints: &anchored})
	require.NoError(t, err)
	assert.Equal(t, fresh, anchored)
}

// TestRenderWithOptionsFormat covers the accepted format names and, more
// importantly, that an unrecognised one is rejected. Unknown values used to
// fall through to the SVG return, so typos produced SVG and exited 0.
func TestRenderWithOptionsFormat(t *testing.T) {
	input := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`)

	tests := []struct {
		format     string
		wantErr    string
		wantPrefix string
	}{
		{format: "", wantPrefix: "<svg"},
		{format: "svg", wantPrefix: "<svg"},
		{format: "text", wantPrefix: ""}, // text art, asserted below
		{format: "txt", wantPrefix: ""},  // alias for text
		{format: "drawio", wantPrefix: "<mxfile"},
		{format: "excalidraw", wantPrefix: `"type": "excalidraw"`},
		{format: "bogus", wantErr: "unsupported format"},
		{format: "SVG", wantErr: "unsupported format"},
	}

	for _, tt := range tests {
		t.Run("format="+tt.format, func(t *testing.T) {
			data, err := RenderWithOptions(context.Background(), input, Options{Format: tt.format})
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Contains(t, err.Error(), "svg, png, text, drawio, or excalidraw",
					"the error should name the valid formats")
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, data)
			if tt.wantPrefix != "" {
				assert.Contains(t, string(data[:min(len(data), 200)]), tt.wantPrefix)
			} else {
				assert.NotContains(t, string(data), "<svg", "text output must not be SVG")
			}
		})
	}
}

func TestRenderWithOptionsPNGFormat(t *testing.T) {
	if _, err := exec.LookPath("resvg"); err != nil {
		t.Skip("resvg not installed, skipping")
	}

	input := []byte(`{
		"type": "flow",
		"nodes": [{"id": "a", "label": "A"}, {"id": "b", "label": "B"}],
		"edges": [{"from": "a", "to": "b"}]
	}`)
	data, err := RenderWithOptions(context.Background(), input, Options{Format: "png"})
	require.NoError(t, err)
	require.True(t, len(data) > 8, "PNG output should not be empty")
	// PNG magic bytes
	assert.Equal(t, byte(0x89), data[0])
	assert.Equal(t, byte('P'), data[1])
}

// --- Integration tests (Task 11) ---

func TestThemeDoesNotChangeGeometry(t *testing.T) {
	input := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`)

	dataDefault, err := RenderWithOptions(context.Background(), input, Options{Theme: "default"})
	require.NoError(t, err)
	dataMidnight, err := RenderWithOptions(context.Background(), input, Options{Theme: "midnight"})
	require.NoError(t, err)
	svgDefault := string(dataDefault)
	svgMidnight := string(dataMidnight)

	assert.NotEqual(t, svgDefault, svgMidnight, "different themes should produce different SVGs")

	extractViewBox := func(svg string) string {
		start := strings.Index(svg, `viewBox="`)
		if start == -1 {
			return ""
		}
		start += len(`viewBox="`)
		end := strings.Index(svg[start:], `"`)
		if end == -1 {
			return ""
		}
		return svg[start : start+end]
	}
	assert.Equal(t, extractViewBox(svgDefault), extractViewBox(svgMidnight),
		"different themes should produce the same geometry (viewBox)")
}

func TestLayoutFlow(t *testing.T) {
	input := []byte(`{
		"type": "flow",
		"direction": "DOWN",
		"nodes": [
			{"id": "a", "label": "Node A"},
			{"id": "b", "label": "Node B"},
			{"id": "c", "label": "Node C"}
		],
		"edges": [
			{"from": "a", "to": "b", "label": "edge1"},
			{"from": "b", "to": "c", "label": "edge2"}
		]
	}`)

	pg, err := LayoutFlow(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, pg)

	assert.Len(t, pg.Nodes, 3, "should have 3 positioned nodes")
	assert.Len(t, pg.Edges, 2, "should have 2 positioned edges")
	assert.Greater(t, pg.Width, 0.0, "width should be positive")
	assert.Greater(t, pg.Height, 0.0, "height should be positive")

	// Verify node IDs are present.
	ids := make(map[string]bool)
	for _, n := range pg.Nodes {
		ids[n.ID] = true
		assert.Greater(t, n.Width, 0.0, "node %s width should be positive", n.ID)
		assert.Greater(t, n.Height, 0.0, "node %s height should be positive", n.ID)
	}
	assert.True(t, ids["a"], "node a should be present")
	assert.True(t, ids["b"], "node b should be present")
	assert.True(t, ids["c"], "node c should be present")
}

func TestLayoutFlowInvalidSpec(t *testing.T) {
	input := []byte(`{"type":"flow","nodes":[{"id":"a","label":"A","shape":"octagon"}]}`)
	_, err := LayoutFlow(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shape")
}

func TestLayoutFlowInvalidJSON(t *testing.T) {
	_, err := LayoutFlow(context.Background(), []byte(`not json`))
	require.Error(t, err)
}

func TestLayoutSequence(t *testing.T) {
	input := []byte(`{
		"type": "sequence",
		"actors": [
			{"id": "client", "label": "Client"},
			{"id": "server", "label": "Server"},
			{"id": "db", "label": "Database"}
		],
		"interactions": [
			{"from": "client", "to": "server", "label": "request", "style": "solid"},
			{"from": "server", "to": "db", "label": "query", "style": "solid"},
			{"from": "db", "to": "server", "label": "result", "style": "dashed"},
			{"from": "server", "to": "client", "label": "response", "style": "dashed"}
		]
	}`)

	ps, err := LayoutSequence(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, ps)

	assert.Len(t, ps.Actors, 3, "should have 3 positioned actors")
	assert.Len(t, ps.Interactions, 4, "should have 4 positioned interactions")
	assert.Greater(t, ps.Width, 0.0, "width should be positive")
	assert.Greater(t, ps.Height, 0.0, "height should be positive")

	// Verify actor IDs are present and lifelines are set.
	ids := make(map[string]bool)
	for _, a := range ps.Actors {
		ids[a.ID] = true
		assert.Greater(t, a.BoxW, 0.0, "actor %s box width should be positive", a.ID)
		assert.Greater(t, a.BoxH, 0.0, "actor %s box height should be positive", a.ID)
		assert.Greater(t, a.LineBottom, a.LineTop, "actor %s lifeline should span downward", a.ID)
	}
	assert.True(t, ids["client"], "client actor should be present")
	assert.True(t, ids["server"], "server actor should be present")
	assert.True(t, ids["db"], "db actor should be present")
}

func TestLayoutSequenceInvalidSpec(t *testing.T) {
	input := []byte(`{"type":"sequence","actors":[],"interactions":[]}`)
	_, err := LayoutSequence(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "actors")
}

func TestLayoutSequenceInvalidJSON(t *testing.T) {
	_, err := LayoutSequence(context.Background(), []byte(`not json`))
	require.Error(t, err)
}

// TestReportContract_DebugWiring verifies that when a DEBUG-armed logger is
// attached via layoutdbg.NewContext, reportContract runs the output-contract
// checker (internal/layout/contract) and emits one "contract_violation"
// decision per violation with phase/module/spec_ref/subject/detail keys —
// and that nothing contract-related is emitted (or computed) when debug is
// not armed.
func TestReportContract_DebugWiring(t *testing.T) {
	// Synthetic positioned graph with a guaranteed violation (an unrouted
	// edge, C2.1) so the armed path deterministically has output regardless
	// of layout engine behavior.
	pg := &model.PositionedGraph{
		Width:  200,
		Height: 200,
		Nodes: []model.PositionedNode{
			{ID: "a", Shape: model.ShapeRect, X: 100, Y: 100, Width: 40, Height: 40},
		},
		Edges: []model.PositionedEdge{
			// Unrouted edge: guaranteed C2.1 violation.
			{ID: "a->a#0", From: "a", To: "a"},
		},
	}
	g := model.Graph{
		Direction: model.Down,
		Nodes:     []model.Node{{ID: "a"}},
		Edges:     []model.Edge{{ID: "a->a#0", From: "a", To: "a"}},
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := layoutdbg.NewContext(context.Background(), logger)

	reportContract(ctx, pg, g, false)

	out := buf.String()
	require.Contains(t, out, `"decision":"contract_violation"`, "armed context")
	for _, key := range []string{`"phase":"contract"`, `"module":"diago"`, `"spec_ref":"C2.1"`, `"subject":"a->a#0"`, `"detail":`} {
		assert.Contains(t, out, key, "decision record")
	}

	// Unarmed: no logger on ctx — nothing may be emitted.
	buf.Reset()
	reportContract(context.Background(), pg, g, false)
	assert.Empty(t, buf.String(), "unarmed context")

	// Armed end-to-end through LayoutFlow on a clean spec: no violations,
	// so no contract_violation records — but the call must not fail.
	buf.Reset()
	spec := []byte(`{"type":"flow","direction":"DOWN","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`)
	_, err := LayoutFlow(ctx, spec)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), `"decision":"contract_violation"`, "a clean two-node spec")
}

// TestReportContract_TextLimits verifies that reportContract checks a text
// layout against contract.TextLimits rather than contract.ScreenLimits.
// PositionedNode.X is the node's center, so a node 40 px wide at X=38 has
// its left edge at 18 px from the canvas's left edge: inside the screen
// profile's 20 px margin (C16.1) but outside the text profile's 16 px one.
func TestReportContract_TextLimits(t *testing.T) {
	pg := &model.PositionedGraph{
		Width:  200,
		Height: 200,
		Nodes: []model.PositionedNode{
			{ID: "a", Shape: model.ShapeRect, X: 38, Y: 50, Width: 40, Height: 40},
		},
	}
	g := model.Graph{Direction: model.Down, Nodes: []model.Node{{ID: "a"}}}

	run := func(text bool) string {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		ctx := layoutdbg.NewContext(context.Background(), logger)
		reportContract(ctx, pg, g, text)
		return buf.String()
	}

	assert.Contains(t, run(false), `"spec_ref":"C16.1"`, "screen limits (margin 20): a node 18 px from the edge")
	assert.NotContains(t, run(true), `"spec_ref":"C16.1"`, "text limits (margin 16): a node 18 px from the edge")
}

// TestRender_DebugResolvesAutoOnce pins end-to-end the guarantee
// ResolveQuiet exists for: a --debug-armed render of an AUTO spec emits
// exactly one "direction_resolved" record, never two. The layout run
// itself resolves AUTO with frame.Resolve, which logs; reportContract
// resolves the same graph again, with ResolveQuiet, to orient the rules it
// checks, and must not log a second time. Checked in both the screen and
// text profiles, for a flow spec and, since AUTO is a valid class
// direction too (schemas/class.json), a class one.
func TestRender_DebugResolvesAutoOnce(t *testing.T) {
	flowSpec := []byte(`{"type":"flow","direction":"AUTO","nodes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"edges":[{"from":"a","to":"b"}]}`)
	classSpec := []byte(`{"type":"class","direction":"AUTO","classes":[{"id":"a","label":"A"},{"id":"b","label":"B"}],"relations":[{"from":"a","to":"b"}]}`)

	for _, tt := range []struct {
		name   string
		spec   []byte
		format string
	}{
		{"flow svg", flowSpec, "svg"},
		{"flow text", flowSpec, "text"},
		{"class svg", classSpec, "svg"},
		{"class text", classSpec, "text"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			ctx := layoutdbg.NewContext(context.Background(), logger)

			_, err := RenderWithOptions(ctx, tt.spec, Options{Format: tt.format})
			require.NoError(t, err)

			got := strings.Count(buf.String(), `"decision":"direction_resolved"`)
			assert.Equal(t, 1, got, "expected exactly one direction_resolved record, got: %s", buf.String())
		})
	}
}

// TestRenderWithOptions_SketchCarriesNoFilter: a sketch render of every
// diagram type carries no SVG filter, so resvg rasterizes it as fast as
// the clean themes.
func TestRenderWithOptions_SketchCarriesNoFilter(t *testing.T) {
	for typ, spec := range map[string]string{
		"flow": `{"type":"flow","nodes":[{"id":"a","label":"A","shape":"cylinder"},{"id":"b","label":"B","shape":"diamond"},{"id":"c","label":"C","shape":"hexagon"}],` +
			`"edges":[{"from":"a","to":"b","label":"x"},{"from":"b","to":"c"}],"groups":[{"id":"g","label":"G","contains":["b","c"]}]}`,
		"sequence": `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],` +
			`"interactions":[{"from":"a","to":"b","label":"hi"},{"from":"b","to":"b","label":"self"}]}`,
		"class": `{"type":"class","classes":[{"id":"a","label":"A","attributes":[{"text":"x: int"}]},{"id":"b","label":"B"}],` +
			`"relations":[{"from":"a","to":"b","kind":"inheritance"}],"legend":true}`,
	} {
		t.Run(typ, func(t *testing.T) {
			out, err := RenderWithOptions(context.Background(), []byte(spec), Options{Theme: "sketch"})
			require.NoError(t, err)
			assert.NotContains(t, string(out), "<filter")
			assert.NotContains(t, string(out), "filter=")
		})
	}
}
