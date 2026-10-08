// Package layered is the layered layout engine (Part 2 of the layout rules
// spec), a Sugiyama pipeline in pure Go. Each stage lives in its
// own package; this one holds the Config and assembles the stages.
package layered

import (
	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/layout/layered/size"
	"github.com/oxforge/diago/internal/model"
)

// Config is the engine's unit-agnostic set of distances (S14): px in the
// screen profile, character cells in the text profile.
type Config struct {
	Text        bool            // the text profile: cells, rune measurement, every shape a box
	Dir         model.Direction // the text profile's direction, whose axis sets the cross-axis values; unused on screen
	Size        size.Options    // S1, and S10's label measurement
	NodeGap     float64         // S7: clear space between neighbors in a layer
	DummyWidth  float64         // S5, S7: a long edge's dummy, on the cross axis
	Clearance   float64         // S7, S8: the node clearance
	Span        float64         // S7, S8: the straighten span
	Reach       float64         // S7, S8: how far a side column or self-loop sits outside a node
	LoopMinH    float64         // S8: the flow extent a node with a self-loop is given at least
	BaseGap     float64         // S8: a channel's height without lanes
	LaneGap     float64         // S8: between two lanes of a channel
	InLaneGap   float64         // S8: between two jogs sharing a lane
	TerminalGap float64         // S7, S9: between the centers of two terminals of a layer
	Snap        float64         // S8: stops this close count as one x
	LabelGap    float64         // S10: a label's distance from its own wire
	CardStep    float64         // S10: a cardinality spot's step along its wire
	EnvDepth    float64         // S10: an adornment envelope's depth along the wire
	EnvHalf     float64         // S10: an adornment envelope's half-width across it
	OwnSlack    float64         // S10: how much nearer than a label's own wire another edge's wire may lie to it
	MarginX     float64         // S11: the canvas margin left and right, in the output frame
	MarginY     float64         // S11: the canvas margin above and below, in the output frame
	GroupPadX   float64         // S9: a group's padding left and right of its content, in the output frame
	GroupPadY   float64         // S9: a group's padding below its content, and above it when untitled, in the output frame
	TitleInset  float64         // S9, C13: a group title's start, from its group's left side, in the output frame
	TitleMid    float64         // S9, C13: a group title's center, below its group's top side, in the output frame
	TitleGap    float64         // S9: the clear space between a group title and the content below it, in the output frame
	TitlePad    float64         // S10, C13: the blank beside a text title, at each end of its band
	LabelPad    float64         // S10, C14.5: the blank a text label keeps before and after it along its row
}

// ScreenConfig is the screen profile (S14) for a theme's node, member,
// edge label and group title fonts.
func ScreenConfig(node, member, label, group size.Font) Config {
	return Config{
		Size: size.Options{
			Measure:  font.MeasureText,
			NodeFont: node, MemberFont: member, LabelFont: label, GroupFont: group,
			WrapWidth: 200, PadX: 20, PadY: 12, MinW: 80, MinH: 40,
			CylinderCap: 16, ParallelogramPad: 20,
			ClassPadX: model.ClassPadX, ClassPadY: model.ClassPadY, ClassMinW: model.ClassMinWidth,
		},
		NodeGap: 40, DummyWidth: 8, Clearance: 12, Span: 16, Reach: 20,
		LoopMinH: 40, BaseGap: 40, LaneGap: 8, InLaneGap: 8, TerminalGap: 16, Snap: 0.5,
		LabelGap: 6, CardStep: 10, EnvDepth: 14, EnvHalf: 7, OwnSlack: 2,
		MarginX: 20, MarginY: 20,
		GroupPadX: 12, GroupPadY: 12, TitleInset: 8, TitleMid: 16, TitleGap: 8,
	}
}

// TextConfig is the text profile (S14) for a resolved direction, which it
// records in Dir. The cross axis is columns under DOWN and UP and rows
// under RIGHT and LEFT; a row is twice as tall as a column is wide, so the
// cross-axis distances shrink and the flow-axis ones grow under RIGHT and
// LEFT. The size values and the margins are in the output frame and stay.
// Layout rejects a text Config built for AUTO or for the other axis than
// the direction it resolves: DOWN and UP share their values, as do RIGHT
// and LEFT, so a Config built for DOWN serves UP. Resolve AUTO with
// frame.Resolve before building one.
func TextConfig(dir model.Direction) Config {
	c := Config{
		Text:    true,
		Dir:     dir,
		Size:    size.Options{Cells: true, WrapWidth: 24, PadX: 2, PadY: 1, MinW: 7, MinH: 3, DiamondPad: 4},
		NodeGap: 5, DummyWidth: 1, Clearance: 2, Span: 1, Reach: 3,
		LoopMinH: 5, BaseGap: 3, LaneGap: 1, InLaneGap: 2, TerminalGap: 2, Snap: 0.5,
		LabelGap: 1, CardStep: 1, EnvDepth: 1, EnvHalf: 1,
		MarginX: 2, MarginY: 1,
		GroupPadX: 3, GroupPadY: 1, TitleInset: 3, TitleMid: 0.5, TitleGap: 1, TitlePad: 1,
		LabelPad: 1,
	}
	if sideways(dir) {
		c.NodeGap, c.Clearance, c.Reach = 2, 1, 2
		c.BaseGap, c.LaneGap, c.InLaneGap, c.TerminalGap = 4, 2, 1, 1
	}
	return c
}

// sideways reports whether dir lays out along the horizontal axis (RIGHT
// or LEFT), where the engine's cross axis is vertical.
func sideways(dir model.Direction) bool { return dir == model.Right || dir == model.Left }
