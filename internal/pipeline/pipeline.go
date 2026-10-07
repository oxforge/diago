// Package pipeline orchestrates the full diagram rendering pipeline:
// JSON input → schema validation → layout → SVG rendering.
// Supports flow, sequence, and class diagram types via auto-detection.
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/oxforge/diago/internal/export/drawio"
	"github.com/oxforge/diago/internal/export/excalidraw"
	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/layered/frame"
	seqlayout "github.com/oxforge/diago/internal/layout/sequence"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
	pngrender "github.com/oxforge/diago/internal/render/png"
	svgrender "github.com/oxforge/diago/internal/render/svg"
	textrender "github.com/oxforge/diago/internal/render/text"
	"github.com/oxforge/diago/internal/schema"
	"github.com/oxforge/diago/internal/theme"
)

type specProbe struct {
	Type string `json:"type"`
}

// Options holds optional overrides for rendering.
type Options struct {
	Theme       string             // theme name override (empty = use spec value)
	Format      string             // output format: svg (default), png, text (alias txt), drawio, excalidraw
	PNG         pngrender.Options  // PNG-specific options (ignored when Format != "png")
	Warnings    *[]string          // optional: receives invocation notices (a discarded -previous)
	Previous    *model.LayoutHints // optional anchoring carrier (C18); flow and class only (validated and discarded for sequence, with a warning)
	LayoutHints *model.LayoutHints // optional sink: a flow layout copies its emitted carrier into it
	Advisories  *[]schema.Advisory // optional sink: schema.Check findings plus text-label-dropped, sorted, after the spec's ignore list
}

// PreviousIgnoredSequence is the warning appended to Options.Warnings when a
// Previous anchor is supplied for a sequence render: sequence diagrams have
// no layout freedom to anchor against, so the value is validated then discarded.
const PreviousIgnoredSequence = "-previous ignored: sequence diagrams have no layout freedom"

// graphParser parses a spec into a flow-shaped graph: schema.ParseFlow or
// schema.ParseClass.
type graphParser = func([]byte) (*model.Graph, error)

func resolveTheme(name string) (theme.Theme, error) {
	if name == "" {
		name = "default"
	}
	return theme.Load(name)
}

// specThemeName is the theme a render of spec uses: override (the -theme flag)
// when set, else the spec's top-level theme field, else "default". A spec
// that is not valid JSON names no theme; its parse reports the error.
func specThemeName(spec []byte, override string) string {
	if override != "" {
		return override
	}
	var probe struct {
		Theme string `json:"theme"`
	}
	if json.Unmarshal(spec, &probe) == nil && probe.Theme != "" {
		return probe.Theme
	}
	return "default"
}

// specTheme loads the theme a render of spec uses (specThemeName). A theme the
// spec names that does not load is a validation error on its field "theme";
// one override names stays the error theme.Load returns.
func specTheme(spec []byte, override string) (theme.Theme, error) {
	th, err := theme.Load(specThemeName(spec, override))
	if err != nil && override == "" {
		return theme.Theme{}, schema.ValidationErrors{{Field: "theme", Message: err.Error()}}
	}
	return th, err
}

// RenderTheme names the theme a render of spec in format uses, override
// (the -theme flag) when set, else the spec's theme field, else "default",
// checked to load as the render checks it: "" for text art, which takes no
// theme. ParsePrevious lays a previous spec out under it.
func RenderTheme(spec []byte, format, override string) (string, error) {
	if isTextFormat(format) {
		return "", nil
	}
	if _, err := specTheme(spec, override); err != nil {
		return "", err
	}
	return specThemeName(spec, override), nil
}

// graphLayout lays out a flow-shaped graph (flow or class) under the screen
// profile of th.
func graphLayout(ctx context.Context, g model.Graph, th theme.Theme, previous *model.LayoutHints) (*model.PositionedGraph, error) {
	return layoutWith(ctx, g, th, false, previous)
}

// Render auto-detects the diagram type from the JSON "type" field and
// dispatches to the appropriate pipeline.
func Render(ctx context.Context, input []byte) (string, error) {
	var probe specProbe
	if err := json.Unmarshal(input, &probe); err != nil {
		return "", fmt.Errorf("render: invalid JSON: %w", err)
	}
	switch probe.Type {
	case "flow":
		return RenderFlow(ctx, input)
	case "sequence":
		return RenderSequence(ctx, input)
	case "class":
		return RenderClass(ctx, input)
	default:
		return "", fmt.Errorf("render: unknown diagram type: %q", probe.Type)
	}
}

// RenderWithTheme renders a diagram using an explicit theme name,
// overriding any theme specified in the JSON spec ("" uses the spec's).
func RenderWithTheme(ctx context.Context, input []byte, themeName string) (string, error) {
	var probe specProbe
	if err := json.Unmarshal(input, &probe); err != nil {
		return "", fmt.Errorf("render: invalid JSON: %w", err)
	}
	switch probe.Type {
	case "flow":
		return renderFlowWithTheme(ctx, input, themeName)
	case "sequence":
		return renderSequenceWithTheme(ctx, input, themeName)
	case "class":
		return renderClassWithTheme(ctx, input, themeName)
	default:
		return "", fmt.Errorf("render: unknown diagram type: %q", probe.Type)
	}
}

// RenderFlow parses the JSON flow diagram spec in input, lays it out, and
// renders it to an SVG string. Returns a ValidationErrors if the spec is
// invalid, or a wrapped internal error if layout/rendering fails.
func RenderFlow(ctx context.Context, input []byte) (string, error) {
	return renderFlowWithTheme(ctx, input, "")
}

// RenderSequence parses the JSON sequence diagram spec in input, lays it out, and
// renders it to an SVG string. Returns a ValidationErrors if the spec is
// invalid, or a wrapped internal error if layout/rendering fails.
func RenderSequence(ctx context.Context, input []byte) (string, error) {
	return renderSequenceWithTheme(ctx, input, "")
}

// RenderClass parses the JSON class diagram spec in input, lays it out with
// the layered engine, and renders it to an SVG string. Returns a
// ValidationErrors if the spec is invalid, or a wrapped internal error if
// layout/rendering fails.
func RenderClass(ctx context.Context, input []byte) (string, error) {
	return renderClassWithTheme(ctx, input, "")
}

// RenderWithOptions renders with the given options. When opts.Advisories is
// set, a successful render also runs schema.Check on the input (anchored
// when a Previous carrier was given) and appends the text renderer's
// dropped labels as text-label-dropped and the layout's ranked flat edges
// as flat-edge-ranked; a failed render leaves the sink untouched.
func RenderWithOptions(ctx context.Context, input []byte, opts Options) ([]byte, error) {
	data, dropped, flatRanked, err := renderWithOptions(ctx, input, opts)
	if err != nil {
		return nil, err
	}
	if opts.Advisories != nil {
		advs, err := checkAdvisories(input, schema.CheckOptions{Anchored: opts.Previous != nil}, dropped, flatRanked)
		if err != nil {
			return nil, err
		}
		*opts.Advisories = advs
	}
	return data, nil
}

func renderWithOptions(ctx context.Context, input []byte, opts Options) ([]byte, []textrender.DroppedLabel, []flatRankedField, error) {
	var probe specProbe
	if err := json.Unmarshal(input, &probe); err != nil {
		return nil, nil, nil, fmt.Errorf("render: invalid JSON: %w", err)
	}
	// Validate a supplied Previous anchor once, up front, for every diagram
	// type: a malformed carrier must be rejected before layout runs, not
	// silently accepted because a specific render path forgot to check it.
	if opts.Previous != nil {
		if err := opts.Previous.Validate(); err != nil {
			return nil, nil, nil, fmt.Errorf("render: previous: %w", err)
		}
	}
	themeName := opts.Theme

	// Normalize and validate the format. An unrecognised value used to fall
	// through to the SVG return below, so a typo produced SVG and exited 0.
	// "txt" is accepted as an alias for "text".
	format := opts.Format
	if format == "" {
		format = "svg"
	}
	if format == "txt" {
		format = "text"
	}
	switch format {
	case "svg", "png", "text":
	case "drawio", "excalidraw":
		data, flatRanked, err := renderExport(ctx, input, probe.Type, format, themeName, &opts)
		return data, nil, flatRanked, err
	default:
		return nil, nil, nil, fmt.Errorf("render: unsupported format %q: want svg, png, text, drawio, or excalidraw", opts.Format)
	}

	if format == "text" {
		switch probe.Type {
		case "flow":
			res, hints, flatRanked, err := renderFlowText(ctx, input, themeName, opts.Previous)
			if err != nil {
				return nil, nil, nil, err
			}
			if opts.LayoutHints != nil && hints != nil {
				*opts.LayoutHints = *hints
			}
			return []byte(res.Art), res.DroppedLabels, flatRanked, nil
		case "sequence":
			if err := discardPreviousForSequence(&opts); err != nil {
				return nil, nil, nil, err
			}
			res, err := renderSequenceText(ctx, input)
			if err != nil {
				return nil, nil, nil, err
			}
			return []byte(res.Art), res.DroppedLabels, nil, nil
		case "class":
			res, hints, flatRanked, err := renderGraphText(ctx, input, "class", schema.ParseClass, opts.Previous)
			if err != nil {
				return nil, nil, nil, err
			}
			if opts.LayoutHints != nil && hints != nil {
				*opts.LayoutHints = *hints
			}
			// A legend is one row per relation kind, under the art.
			art := res.Art
			if len(res.Legend) > 0 {
				art = strings.TrimRight(art, "\n") + "\n\n" + strings.Join(res.Legend, "\n") + "\n"
			}
			return []byte(art), res.DroppedLabels, flatRanked, nil
		default:
			return nil, nil, nil, fmt.Errorf("render: unknown diagram type: %q", probe.Type)
		}
	}

	var svg string
	var flatRanked []flatRankedField
	var err error
	switch probe.Type {
	case "flow":
		var hints *model.LayoutHints
		svg, hints, flatRanked, err = renderFlowFull(ctx, input, themeName, opts.Previous)
		if err == nil && opts.LayoutHints != nil && hints != nil {
			*opts.LayoutHints = *hints
		}
	case "sequence":
		if err = discardPreviousForSequence(&opts); err != nil {
			return nil, nil, nil, err
		}
		svg, err = renderSequenceFull(ctx, input, themeName)
	case "class":
		var hints *model.LayoutHints
		svg, hints, flatRanked, err = renderGraphFull(ctx, input, "class", schema.ParseClass, themeName, opts.Previous)
		if err == nil && opts.LayoutHints != nil && hints != nil {
			*opts.LayoutHints = *hints
		}
	default:
		return nil, nil, nil, fmt.Errorf("render: unknown diagram type: %q", probe.Type)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if format == "png" {
		png, err := pngrender.Render(ctx, []byte(svg), opts.PNG)
		return png, nil, flatRanked, err
	}
	return []byte(svg), nil, flatRanked, nil
}

// discardPreviousForSequence discards opts.Previous for a sequence render:
// sequence diagrams have no layout freedom to anchor against. Validation of
// opts.Previous already happened once, up front, in renderWithOptions, so
// this only appends the discard warning to opts.Warnings when one was
// supplied.
func discardPreviousForSequence(opts *Options) error {
	if opts.Previous == nil {
		return nil
	}
	if opts.Warnings != nil {
		*opts.Warnings = append(*opts.Warnings, PreviousIgnoredSequence)
	}
	return nil
}

// renderExport lays a flow spec out under the theme's screen profile and
// converts the positioned graph to a draw.io or Excalidraw document. The
// exporters work from the laid-out model, never from the SVG, and cover flow
// diagrams only.
func renderExport(ctx context.Context, input []byte, diagType, format, themeName string, opts *Options) ([]byte, []flatRankedField, error) {
	if diagType != "flow" {
		return nil, nil, fmt.Errorf("render: %s format is not supported for %s diagrams", format, diagType)
	}
	pg, th, err := layoutGraph(ctx, input, "flow", schema.ParseFlow, themeName, opts.Previous)
	if err != nil {
		return nil, nil, err
	}
	if opts.LayoutHints != nil && pg.LayoutHints != nil {
		*opts.LayoutHints = *pg.LayoutHints
	}
	flatRanked := flatRankedFields(pg, opts.Previous)
	if format == "drawio" {
		return drawio.Export(pg, th), flatRanked, nil
	}
	data, err := excalidraw.Export(pg, th)
	return data, flatRanked, err
}

// LayoutFlow parses and lays out a flow diagram, returning the positioned graph.
// The theme is always "default" (font metrics are theme-independent for layout).
// The ctx is forwarded to the layered engine so that any debug logger
// attached via layoutdbg.NewContext receives its decision records.
func LayoutFlow(ctx context.Context, input []byte) (*model.PositionedGraph, error) {
	return LayoutFlowForFormat(ctx, input, "svg", nil)
}

// isTextFormat reports whether format names text art ("text", or its alias
// "txt").
func isTextFormat(format string) bool {
	return format == "txt" || format == "text"
}

// LayoutFlowForFormat is LayoutFlow with the output format known: text art
// lays out under the text profile (S14); every other format uses the screen
// profile of the default theme.
func LayoutFlowForFormat(ctx context.Context, input []byte, format string, previous *model.LayoutHints) (*model.PositionedGraph, error) {
	return LayoutGraphForFormat(ctx, input, schema.ParseFlow, format, previous)
}

// LayoutClassForFormat is LayoutGraphForFormat for a class spec.
func LayoutClassForFormat(ctx context.Context, input []byte, format string, previous *model.LayoutHints) (*model.PositionedGraph, error) {
	return LayoutGraphForFormat(ctx, input, schema.ParseClass, format, previous)
}

// LayoutGraphForFormat parses a flow-shaped spec with parse and lays it out
// with the output format known: text art lays out under the text profile
// (S14); every other format uses the screen profile of the default theme.
// Every edge routes orthogonally regardless of format.
func LayoutGraphForFormat(ctx context.Context, input []byte, parse graphParser, format string, previous *model.LayoutHints) (*model.PositionedGraph, error) {
	graph, err := parse(input)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	text := isTextFormat(format)
	var pg *model.PositionedGraph
	if text {
		pg, err = layoutWith(ctx, *graph, theme.Theme{}, true, previous)
	} else {
		th, tErr := resolveTheme("default")
		if tErr != nil {
			return nil, fmt.Errorf("theme: %w", tErr)
		}
		pg, err = graphLayout(ctx, *graph, th, previous)
	}
	if err != nil {
		return nil, fmt.Errorf("layout: %w", err)
	}
	reportContract(ctx, pg, *graph, text)
	return pg, nil
}

// reportContract runs the output-contract checker (internal/layout/contract,
// rules C2-C17) on the final positioned graph, laid out from g, and emits
// one debug decision per violation. It only runs when a debug logger is
// armed on ctx (--debug), so a render without --debug emits no decision
// records and costs one Armed check.
// A text layout is checked against the text profile's limits
// (contract.TextLimits, C0) rather than the screen ones.
func reportContract(ctx context.Context, pg *model.PositionedGraph, g model.Graph, text bool) {
	if !layoutdbg.Armed(ctx) {
		return
	}
	// AUTO resolves the same way the layered engine resolved it for the
	// layout itself (frame.Resolve, S11), so C8.2-C8.5 orient correctly.
	// ResolveQuiet is frame.Resolve without its own "direction_resolved"
	// record: the layout run that produced pg already emitted that record,
	// so this call must not add a second.
	dir := frame.ResolveQuiet(g)
	limits := contract.ScreenLimits()
	if text {
		limits = contract.TextLimits(dir)
	}
	opts := contract.Options{
		Limits:    limits,
		Direction: dir,
		EdgeIDs:   edgeIDs(g),
		Text:      text,
	}
	for _, v := range contract.Check(pg, opts) {
		layoutdbg.Decision(ctx, "contract_violation",
			"phase", "contract",
			"module", "diago",
			"spec_ref", v.Rule,
			"subject", v.Subject,
			"detail", v.Detail,
		)
	}
}

// edgeIDs lists g's edge ids, in edge order: the ids a contract check
// expects in the layout (C2.1). Mirrors corpus.EdgeIDs, which pipeline
// cannot import (corpus imports pipeline).
func edgeIDs(g model.Graph) []string {
	ids := make([]string, len(g.Edges))
	for i, e := range g.Edges {
		ids[i] = e.ID
	}
	return ids
}

// LayoutSequence parses and lays out a sequence diagram, returning the positioned sequence.
// The theme is always "default" (font metrics are theme-independent for layout).
// The ctx is currently unused by the sequence layout engine but is accepted for
// symmetry with LayoutFlow so callers can plumb the per-request debug logger.
func LayoutSequence(ctx context.Context, input []byte) (*model.PositionedSequence, error) {
	return LayoutSequenceForFormat(ctx, input, "svg")
}

// LayoutSequenceForFormat is LayoutSequence with the output format known:
// text art lays out under the sequence engine's text profile, on the cell
// grid.
func LayoutSequenceForFormat(ctx context.Context, input []byte, format string) (*model.PositionedSequence, error) {
	_ = ctx
	diagram, err := schema.ParseSequence(input)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if isTextFormat(format) {
		p := seqlayout.TextProfile()
		positioned := seqlayout.LayoutWithProfile(*diagram, p)
		seqlayout.NormalizeSequenceWithProfile(positioned, p)
		return positioned, nil
	}
	th, err := resolveTheme("default")
	if err != nil {
		return nil, fmt.Errorf("theme: %w", err)
	}
	fonts := seqlayout.LayoutFonts{
		Family:        th.Actor.Font.Family,
		HeaderSize:    th.Actor.Font.Size,
		LabelSize:     th.Edge.LabelFont.Size,
		FragLabelSize: th.Fragment.LabelFont.Size,
	}
	positioned := seqlayout.Layout(*diagram, font.MeasureText, fonts)
	seqlayout.NormalizeSequence(positioned, font.MeasureText, fonts)
	return positioned, nil
}

func renderFlowWithTheme(ctx context.Context, input []byte, themeName string) (string, error) {
	svg, _, _, err := renderFlowFull(ctx, input, themeName, nil)
	return svg, err
}

func renderSequenceWithTheme(ctx context.Context, input []byte, themeName string) (string, error) {
	return renderSequenceFull(ctx, input, themeName)
}

func renderClassWithTheme(ctx context.Context, input []byte, themeName string) (string, error) {
	svg, _, _, err := renderGraphFull(ctx, input, "class", schema.ParseClass, themeName, nil)
	return svg, err
}

// layoutGraph parses a flow-shaped spec with parse, validates it, and lays it
// out under the theme the render uses (themeName, else the spec's field),
// returning the positioned graph and that theme. kind ("flow" or "class")
// names the diagram in error messages. Shared by the SVG, PNG and export
// renderers.
func layoutGraph(ctx context.Context, input []byte, kind string, parse graphParser, themeName string, previous *model.LayoutHints) (*model.PositionedGraph, theme.Theme, error) {
	graph, err := parse(input)
	if err != nil {
		return nil, theme.Theme{}, fmt.Errorf("render %s: %w", kind, err)
	}

	th, err := specTheme(input, themeName)
	if err != nil {
		return nil, theme.Theme{}, fmt.Errorf("render %s: %w", kind, err)
	}

	positioned, err := graphLayout(ctx, *graph, th, previous)
	if err != nil {
		return nil, theme.Theme{}, fmt.Errorf("render %s: %w", kind, err)
	}

	reportContract(ctx, positioned, *graph, false)
	return positioned, th, nil
}

func renderFlowFull(ctx context.Context, input []byte, themeName string, previous *model.LayoutHints) (string, *model.LayoutHints, []flatRankedField, error) {
	return renderGraphFull(ctx, input, "flow", schema.ParseFlow, themeName, previous)
}

func renderGraphFull(ctx context.Context, input []byte, kind string, parse graphParser, themeName string, previous *model.LayoutHints) (string, *model.LayoutHints, []flatRankedField, error) {
	positioned, th, err := layoutGraph(ctx, input, kind, parse, themeName, previous)
	if err != nil {
		return "", nil, nil, err
	}
	return svgrender.Render(positioned, th), positioned.LayoutHints, flatRankedFields(positioned, previous), nil
}

func renderFlowText(ctx context.Context, input []byte, _ string, previous *model.LayoutHints) (textrender.Result, *model.LayoutHints, []flatRankedField, error) {
	return renderGraphText(ctx, input, "flow", schema.ParseFlow, previous)
}

func renderGraphText(ctx context.Context, input []byte, kind string, parse graphParser, previous *model.LayoutHints) (textrender.Result, *model.LayoutHints, []flatRankedField, error) {
	// Text art is drawn with box-drawing characters on a character grid: every
	// edge already routes orthogonally, so LayoutGraphForFormat just lays out
	// under the text profile (S14). The theme is irrelevant to text.
	positioned, err := LayoutGraphForFormat(ctx, input, parse, "txt", previous)
	if err != nil {
		return textrender.Result{}, nil, nil, fmt.Errorf("render %s: %w", kind, err)
	}
	res, err := textrender.Render(positioned)
	if err != nil {
		return textrender.Result{}, nil, nil, err
	}
	return res, positioned.LayoutHints, flatRankedFields(positioned, previous), nil
}

func renderSequenceText(_ context.Context, input []byte) (textrender.Result, error) {
	diagram, err := schema.ParseSequence(input)
	if err != nil {
		return textrender.Result{}, fmt.Errorf("render sequence: %w", err)
	}
	p := seqlayout.TextProfile()
	positioned := seqlayout.LayoutWithProfile(*diagram, p)
	seqlayout.NormalizeSequenceWithProfile(positioned, p)
	return textrender.RenderSequence(positioned, diagram.Activations)
}

func renderSequenceFull(_ context.Context, input []byte, themeName string) (string, error) {
	// Step 1: Parse and validate the JSON spec.
	diagram, err := schema.ParseSequence(input)
	if err != nil {
		return "", fmt.Errorf("render sequence: %w", err)
	}

	th, err := specTheme(input, themeName)
	if err != nil {
		return "", fmt.Errorf("render sequence: %w", err)
	}

	// Step 2: Lay out the sequence diagram using the timeline engine.
	positioned := seqlayout.Layout(*diagram, font.MeasureText, seqlayout.LayoutFonts{
		Family:        th.Actor.Font.Family,
		HeaderSize:    th.Actor.Font.Size,
		LabelSize:     th.Edge.LabelFont.Size,
		FragLabelSize: th.Fragment.LabelFont.Size,
	})

	// Step 2b: Normalize coordinates for consistent margins.
	seqlayout.NormalizeSequence(positioned, font.MeasureText, seqlayout.LayoutFonts{
		Family:        th.Actor.Font.Family,
		HeaderSize:    th.Actor.Font.Size,
		LabelSize:     th.Edge.LabelFont.Size,
		FragLabelSize: th.Fragment.LabelFont.Size,
	})

	// Step 3: Render to SVG using the resolved theme.
	svg := svgrender.RenderSequence(positioned, th)

	return svg, nil
}
