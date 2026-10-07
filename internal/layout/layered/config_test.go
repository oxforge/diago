package layered

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/model"
)

// The two tests below pin every field of both profiles to S14's table.

func TestScreenConfig_IsTheS14Table(t *testing.T) {
	node, member, label, group := size.Font{Family: "Inter", Size: 14}, size.Font{Family: "Pangolin", Size: 12}, size.Font{Family: "Inter", Size: 12}, size.Font{Family: "Inter", Size: 11}
	got := ScreenConfig(node, member, label, group)
	require.NotNil(t, got.Size.Measure)
	w, h := got.Size.Measure("Start", 14, "Inter")
	fw, fh := font.MeasureText("Start", 14, "Inter")
	assert.Equal(t, [2]float64{fw, fh}, [2]float64{w, h}, "the embedded font metrics")
	got.Size.Measure = nil // a func never compares equal
	assert.Equal(t, Config{
		Size: size.Options{
			NodeFont: node, MemberFont: member, LabelFont: label, GroupFont: group,
			WrapWidth: 200, PadX: 20, PadY: 12, MinW: 80, MinH: 40,
			CylinderCap: 16, ParallelogramPad: 20,
			ClassPadX: 10, ClassPadY: 6, ClassMinW: 60,
		},
		NodeGap: 40, DummyWidth: 8, Clearance: 12, Span: 16, Reach: 20,
		LoopMinH: 40, BaseGap: 40, LaneGap: 8, InLaneGap: 8, TerminalGap: 16, Snap: 0.5,
		LabelGap: 6, CardStep: 10, EnvDepth: 14, EnvHalf: 7, OwnSlack: 2,
		MarginX: 20, MarginY: 20,
		GroupPadX: 12, GroupPadY: 12, TitleInset: 8, TitleMid: 16, TitleGap: 8,
	}, got)
}

func TestTextConfig_IsTheS14Table(t *testing.T) {
	sizes := size.Options{Cells: true, WrapWidth: 24, PadX: 2, PadY: 1, MinW: 7, MinH: 3, DiamondPad: 4}
	columns := func(dir model.Direction) Config {
		return Config{
			Text: true, Dir: dir, Size: sizes, NodeGap: 5, DummyWidth: 1, Clearance: 2, Span: 1, Reach: 3,
			LoopMinH: 5, BaseGap: 3, LaneGap: 1, InLaneGap: 2, TerminalGap: 2, Snap: 0.5,
			LabelGap: 1, CardStep: 1, EnvDepth: 1, EnvHalf: 1, MarginX: 2, MarginY: 1,
			GroupPadX: 3, GroupPadY: 1, TitleInset: 3, TitleMid: 0.5, TitleGap: 1, TitlePad: 1,
			LabelPad: 1,
		}
	}
	rows := func(dir model.Direction) Config {
		return Config{
			Text: true, Dir: dir, Size: sizes, NodeGap: 2, DummyWidth: 1, Clearance: 1, Span: 1, Reach: 2,
			LoopMinH: 5, BaseGap: 4, LaneGap: 2, InLaneGap: 1, TerminalGap: 1, Snap: 0.5,
			LabelGap: 1, CardStep: 1, EnvDepth: 1, EnvHalf: 1, MarginX: 2, MarginY: 1,
			GroupPadX: 3, GroupPadY: 1, TitleInset: 3, TitleMid: 0.5, TitleGap: 1, TitlePad: 1,
			LabelPad: 1,
		}
	}
	for _, tt := range []struct {
		dir  model.Direction
		want Config
	}{
		{model.Down, columns(model.Down)},
		{model.Up, columns(model.Up)},
		{model.Right, rows(model.Right)},
		{model.Left, rows(model.Left)},
	} {
		t.Run(tt.dir.String(), func(t *testing.T) {
			assert.Equal(t, tt.want, TextConfig(tt.dir))
			assert.GreaterOrEqual(t, TextConfig(tt.dir).LabelGap, TextConfig(tt.dir).TitlePad, "a title a label gap from the wires keeps its blanks off them (S10, S14)")
		})
	}
}
