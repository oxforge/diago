package labels

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/layoutdbg"
)

var (
	titleScreen = TitleOptions{Inset: 8, Gap: 6}
	titleText   = TitleOptions{Inset: 3, Gap: 1, Text: true}
)

// wires is a band 200 wide for a title 40 wide, crossed by vertical wires
// at xs.
func wires(xs ...float64) Band {
	b := Band{ID: "g", Width: 200, Title: 40}
	for _, x := range xs {
		b.Blocked = append(b.Blocked, Span{x, x})
	}
	return b
}

func title(b Band, o TitleOptions) float64 {
	x, _ := Title(context.Background(), b, o)
	return x
}

// TestTitle_BlockedOnlyOnItsFallback pins the flag Title returns beside the
// offset: a title in any of its three slots is not blocked; one left in
// its left slot for want of room is.
func TestTitle_BlockedOnlyOnItsFallback(t *testing.T) {
	for _, tt := range []struct {
		name    string
		b       Band
		blocked bool
	}{
		{"the left slot", wires(), false},
		{"the right slot", wires(30), false},
		{"the middle", wires(30, 170), false},
		{"no room", wires(30, 70, 110, 150), true},
		{"a band narrower than the title", Band{ID: "g", Width: 50, Title: 40}, true},
	} {
		_, blocked := Title(context.Background(), tt.b, titleScreen)
		assert.Equal(t, tt.blocked, blocked, tt.name)
	}
}

func TestTitle_TheLeftSlotWhenItIsFree(t *testing.T) {
	assert.Equal(t, 8.0, title(wires(), titleScreen), "an empty band")
	assert.Equal(t, 8.0, title(wires(60), titleScreen), "the wire keeps 12 px from the title's end at 48")
}

func TestTitle_TheRightSlotWhenTheLeftIsBlocked(t *testing.T) {
	assert.Equal(t, 152.0, title(wires(30), titleScreen))
}

func TestTitle_ASlotKeepsTheGap(t *testing.T) {
	assert.Equal(t, 152.0, title(wires(53), titleScreen), "5 px from the left slot's end")
	assert.Equal(t, 8.0, title(wires(54), titleScreen), "exactly the gap")
}

func TestTitle_TheMiddleOfTheWidestFreeSpan(t *testing.T) {
	assert.Equal(t, 80.0, title(wires(30, 170), titleScreen), "free: [8,24] [36,164] [176,192]")
	assert.Equal(t, 45.0, title(wires(30, 100, 170), titleScreen), "[36,94] and [106,164] tie: the leftmost")
}

func TestTitle_ABoxBlocksItsWholeSpan(t *testing.T) {
	b := Band{ID: "g", Width: 200, Title: 40, Blocked: []Span{{20, 100}}}
	assert.Equal(t, 152.0, title(b, titleScreen))
}

func TestTitle_ABlockedTitleStaysLeftAndIsRecorded(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	b := wires(30, 70, 110, 150)
	x, _ := Title(ctx, b, titleScreen)
	assert.Equal(t, 8.0, x, "free spans of 16, 28, 28, 28 and 36 px hold no 40 px title")
	assert.Contains(t, buf.String(), `"decision":"group_title_blocked"`)
	assert.Contains(t, buf.String(), `"group":"g"`)
}

// TestTitle_ABandNarrowerThanTheTitleStaysLeftAndIsRecorded pins S10's
// fallback for a band too narrow to hold the title in any slot, with
// nothing crossing it at all: Width 50 is under 2 x Inset + Title (56),
// so the trial free window, [Inset, Width-Inset], is itself narrower than
// the title.
func TestTitle_ABandNarrowerThanTheTitleStaysLeftAndIsRecorded(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	b := Band{ID: "g", Width: 50, Title: 40}
	x, _ := Title(ctx, b, titleScreen)
	assert.Equal(t, 8.0, x, "the band's free window [8,42] is 34 px, under the 40 px title")
	assert.Contains(t, buf.String(), `"decision":"group_title_blocked"`)
	assert.Contains(t, buf.String(), `"group":"g"`)
}

// TestTitle_TextCountsWholeCells pins S10 in the text profile: a wire on
// a cell's middle blocks its whole cell, so the gap keeps the blank the
// renderer writes beside a title off it, and a title starts on a cell.
func TestTitle_TextCountsWholeCells(t *testing.T) {
	b := Band{ID: "g", Width: 30, Title: 6, Blocked: []Span{{10.5, 10.5}}}
	assert.Equal(t, 3.0, title(b, titleText), "cells 3-8, the blank in 9, the wire in 10")
	b.Blocked = []Span{{5.5, 5.5}, {20, 20}}
	assert.Equal(t, 10.0, title(b, titleText), "a wire on cell 20's left edge is drawn in cell 20, the right slot's blank")
	b.Blocked = []Span{{5.5, 5.5}, {25.5, 25.5}}
	assert.Equal(t, 12.0, title(b, titleText), "free: cells 7-23, the middle 12.5 rounds down")
}

// TestTitle_ARangedTextSpanBlocksItsWholeCells pins S10's text handling of
// a box span, not a wire's rounded point: a group box or node box already
// spans whole cells, so Title takes it as given, grows it by the one-cell
// Gap on each side, and falls to the right slot when the left one no
// longer fits.
func TestTitle_ARangedTextSpanBlocksItsWholeCells(t *testing.T) {
	b := Band{ID: "g", Width: 30, Title: 6, Blocked: []Span{{4, 9}}}
	assert.Equal(t, 21.0, title(b, titleText),
		"cells 4-8 grown by the gap leave cells 10-26 free; the right slot (21-26) fits, the left one (3-8) does not")
}
