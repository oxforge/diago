// Package pipeline orchestrates the full diagram rendering pipeline:
// JSON input → schema validation → layout → SVG rendering.
// Supports flow, sequence, and class diagram types via auto-detection.
package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/oxforge/diago/internal/export/drawio"
	"github.com/oxforge/diago/internal/export/excalidraw"
	seqlayout "github.com/oxforge/diago/internal/layout/sequence"
	"github.com/oxforge/diago/internal/model"
	pngrender "github.com/oxforge/diago/internal/render/png"
	svgrender "github.com/oxforge/diago/internal/render/svg"
	textrender "github.com/oxforge/diago/internal/render/text"
	"github.com/oxforge/diago/internal/schema"
)

// Options holds optional overrides for rendering.
type Options struct {
	Theme       string             // theme name override (empty = use spec value)
	Format      string             // output format: svg (default), png, text (alias txt), drawio, excalidraw
	PNG         pngrender.Options  // PNG-specific options (ignored when Format != "png")
	Warnings    *[]string          // optional: receives invocation notices (a discarded -previous)
	Previous    *model.LayoutHints // optional anchoring carrier (C18); flow and class only (validated and discarded for sequence, with a warning)
	LayoutHints *model.LayoutHints // optional sink: a flow or class layout copies its emitted carrier into it
	Advisories  *[]schema.Advisory // optional sink: schema.Check findings plus text-label-dropped and flat-edge-ranked, sorted, after the spec's ignore list
}

// PreviousIgnoredSequence is the warning appended to Options.Warnings when a
// Previous anchor is supplied for a sequence render: sequence diagrams have
// no layout freedom to anchor against, so the value is validated then discarded.
const PreviousIgnoredSequence = "-previous ignored: sequence diagrams have no layout freedom"

// graphParser parses a spec into a flow-shaped graph: schema.ParseFlow or
// schema.ParseClass.
type graphParser = func([]byte) (*model.Graph, error)

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

// renderWithOptions is RenderWithOptions before its advisories: the
// rendered bytes, the text renderer's dropped labels and the layout's
// ranked flat edges.
func renderWithOptions(ctx context.Context, input []byte, opts Options) ([]byte, []textrender.DroppedLabel, []flatRankedField, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("render: %w", err)
	}
	kind, err := schema.DiagramType(input)
	if err != nil {
		return nil, nil, nil, err
	}
	// Validate a supplied Previous anchor once, up front, for every diagram
	// type: a malformed carrier must be rejected before layout runs, not
	// silently accepted because a specific render path forgot to check it.
	if opts.Previous != nil {
		if err := opts.Previous.Validate(); err != nil {
			return nil, nil, nil, fmt.Errorf("render: previous: %w", err)
		}
	}

	// Normalize and validate the format: an unknown one is an error, never
	// SVG. "txt" is accepted as an alias for "text".
	format := opts.Format
	if format == "" {
		format = "svg"
	}
	if format == "txt" {
		format = "text"
	}
	switch format {
	case "svg", "png":
		data, flatRanked, err := renderImage(ctx, input, kind, format, &opts)
		return data, nil, flatRanked, err
	case "text":
		return renderTextArt(ctx, input, kind, &opts)
	case "drawio", "excalidraw":
		data, flatRanked, err := renderExport(ctx, input, kind, format, opts.Theme, &opts)
		return data, nil, flatRanked, err
	default:
		return nil, nil, nil, fmt.Errorf("render: unsupported format %q: want svg, png, text, drawio, or excalidraw", opts.Format)
	}
}

// renderTextArt renders a spec of the given kind as text art. Text art
// takes no theme, but a theme the spec or -theme names must load. A flow
// or class layout's carrier goes to opts.LayoutHints, and a class
// diagram's legend, one row per relation kind, under the art.
func renderTextArt(ctx context.Context, input []byte, kind string, opts *Options) ([]byte, []textrender.DroppedLabel, []flatRankedField, error) {
	if _, err := specTheme(input, opts.Theme); err != nil {
		return nil, nil, nil, err
	}
	switch kind {
	case "flow":
		res, hints, flatRanked, err := renderFlowText(ctx, input, opts.Previous)
		if err != nil {
			return nil, nil, nil, err
		}
		opts.keepHints(hints)
		return []byte(res.Art), res.DroppedLabels, flatRanked, nil
	case "sequence":
		discardPreviousForSequence(opts)
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
		opts.keepHints(hints)
		art := res.Art
		if len(res.Legend) > 0 {
			art = strings.TrimRight(art, "\n") + "\n\n" + strings.Join(res.Legend, "\n") + "\n"
		}
		return []byte(art), res.DroppedLabels, flatRanked, nil
	default:
		return nil, nil, nil, fmt.Errorf("render: unknown diagram type: %q", kind)
	}
}

// renderImage renders a spec of the given kind as SVG, or as PNG
// rasterized from the SVG when format is "png". A flow or class layout's
// carrier goes to opts.LayoutHints.
func renderImage(ctx context.Context, input []byte, kind, format string, opts *Options) ([]byte, []flatRankedField, error) {
	var svg string
	var hints *model.LayoutHints
	var flatRanked []flatRankedField
	var err error
	switch kind {
	case "flow":
		svg, hints, flatRanked, err = renderFlowFull(ctx, input, opts.Theme, opts.Previous)
	case "sequence":
		discardPreviousForSequence(opts)
		svg, err = renderSequenceFull(ctx, input, opts.Theme)
	case "class":
		svg, hints, flatRanked, err = renderGraphFull(ctx, input, "class", schema.ParseClass, opts.Theme, opts.Previous)
	default:
		return nil, nil, fmt.Errorf("render: unknown diagram type: %q", kind)
	}
	if err != nil {
		return nil, nil, err
	}
	opts.keepHints(hints)
	if format == "png" {
		png, err := pngrender.Render(ctx, []byte(svg), opts.PNG)
		return png, flatRanked, err
	}
	return []byte(svg), flatRanked, nil
}

// keepHints copies the carrier a flow or class layout emitted into the
// LayoutHints sink, when there are both.
func (o *Options) keepHints(hints *model.LayoutHints) {
	if o.LayoutHints != nil && hints != nil {
		*o.LayoutHints = *hints
	}
}

// discardPreviousForSequence discards opts.Previous for a sequence render:
// sequence diagrams have no layout freedom to anchor against. Validation of
// opts.Previous already happened once, up front, in renderWithOptions, so
// this only appends the discard warning to opts.Warnings when one was
// supplied.
func discardPreviousForSequence(opts *Options) {
	if opts.Previous == nil {
		return
	}
	if opts.Warnings != nil {
		*opts.Warnings = append(*opts.Warnings, PreviousIgnoredSequence)
	}
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
	opts.keepHints(pg.LayoutHints)
	flatRanked := flatRankedFields(pg, opts.Previous)
	if format == "drawio" {
		return drawio.Export(pg, th), flatRanked, nil
	}
	data, err := excalidraw.Export(pg, th)
	return data, flatRanked, err
}

// isTextFormat reports whether format names text art ("text", or its alias
// "txt").
func isTextFormat(format string) bool {
	return format == "txt" || format == "text"
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

func renderFlowText(ctx context.Context, input []byte, previous *model.LayoutHints) (textrender.Result, *model.LayoutHints, []flatRankedField, error) {
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
	return textrender.RenderSequence(positioned, diagram.Activations), nil
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

	// Steps 2 and 2b: lay out with the timeline engine and normalize.
	positioned := layoutSequenceScreen(*diagram, th)

	// Step 3: Render to SVG using the resolved theme.
	svg := svgrender.RenderSequence(positioned, th)

	return svg, nil
}
