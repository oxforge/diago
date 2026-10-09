package pipeline

import (
	"context"
	"fmt"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/layout/contract"
	"github.com/oxforge/diago/internal/layout/layered"
	"github.com/oxforge/diago/internal/layout/layered/frame"
	"github.com/oxforge/diago/internal/layout/layered/size"
	seqlayout "github.com/oxforge/diago/internal/layout/sequence"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// layoutWith lays a flow or class graph out with the layered engine
// (Part 2 of the layout rules spec): under the text Config when text,
// else under the screen Config of th's fonts (S14), anchored on previous
// when it is not nil (C18, S13).
func layoutWith(ctx context.Context, g model.Graph, th theme.Theme, text bool, previous *model.LayoutHints) (*model.PositionedGraph, error) {
	if text {
		tg, cfg := layered.ForText(ctx, g)
		return layered.Layout(ctx, tg, cfg, previous)
	}
	return layered.Layout(ctx, g, screenConfig(th), previous)
}

// screenConfig is the layered engine's screen Config (S14) for th: its
// fonts, its node padding as the label padding, and its group padding on
// both axes.
func screenConfig(th theme.Theme) layered.Config {
	fontOf := func(f theme.FontStyle) size.Font { return size.Font{Family: f.Family, Size: f.Size} }
	cfg := layered.ScreenConfig(fontOf(th.Node.Font), fontOf(th.Class.MemberFont), fontOf(th.Edge.LabelFont), fontOf(th.Group.LabelFont))
	cfg.Size.PadX, cfg.Size.PadY = th.Node.Padding.X, th.Node.Padding.Y
	cfg.GroupPadX, cfg.GroupPadY = th.Group.Padding, th.Group.Padding
	return cfg
}

// graphLayout lays out a flow-shaped graph (flow or class) under the screen
// profile of th.
func graphLayout(ctx context.Context, g model.Graph, th theme.Theme, previous *model.LayoutHints) (*model.PositionedGraph, error) {
	return layoutWith(ctx, g, th, false, previous)
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

// sequenceScreenProfile is the timeline engine's screen profile for th: its
// fonts and its activation bar width.
func sequenceScreenProfile(th theme.Theme) seqlayout.Profile {
	p := seqlayout.ScreenProfile(seqlayout.LayoutFonts{
		Family:        th.Actor.Font.Family,
		HeaderSize:    th.Actor.Font.Size,
		LabelSize:     th.Edge.LabelFont.Size,
		FragLabelSize: th.Fragment.LabelFont.Size,
	})
	p.MeasureText = font.MeasureText
	p.ActivationWidth = th.Activation.Width
	return p
}

// layoutSequenceScreen lays a sequence diagram out under th's screen profile
// and normalizes it.
func layoutSequenceScreen(d model.SequenceDiagram, th theme.Theme) *model.PositionedSequence {
	p := sequenceScreenProfile(th)
	positioned := seqlayout.LayoutWithProfile(d, p)
	seqlayout.NormalizeSequenceWithProfile(positioned, p)
	return positioned
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
