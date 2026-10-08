package pipeline

import (
	"context"

	"github.com/oxforge/diago/internal/layout/layered"
	"github.com/oxforge/diago/internal/layout/layered/size"
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
