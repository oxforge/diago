package ports

import (
	"cmp"
	"math"
	"slices"

	"github.com/oxforge/diago/internal/model"
)

// Vertex is a vertex a diamond's or circle's exit leaves from. The fourth,
// the top, is the in-vertex (C8).
type Vertex int

// The exit vertices.
const (
	Bottom Vertex = iota
	Left
	Right
)

// Exit is a forward edge leaving a diamond or a circle: Toward is the
// cross-axis position it heads for, the port its first hop ends on (S8,
// Diamond and circle exits).
type Exit struct {
	Edge   int
	Toward float64
}

// Merged reports whether a pinned node of shape s sends its forward
// exits, when it has two or more, all from its bottom vertex, a fan-out
// merged onto one trunk (S8, C8.5): a circle does; a diamond, whose exits
// are the branches of a decision, keeps them apart (Exits).
func Merged(s model.Shape) bool {
	return s == model.ShapeCircle
}

// Exits decides which vertex each exit of a diamond centered at cx, or
// the lone exit of a circle, leaves from (S8, C8.5); a circle's two or
// more merge on the bottom instead (Merged). A lone exit takes the
// bottom. Two or three
// exits each take their own vertex: three go left, bottom and right in
// order of heading; of two, a pair heading beyond band on opposite sides
// takes left and right, and otherwise the one heading farther from cx
// takes the vertex on its side and the other the bottom. Four or more
// share by heading: left or right beyond band, bottom within it. Headings
// tie by edge order. The result is parallel to exits.
func Exits(cx, band float64, exits []Exit) []Vertex {
	out := make([]Vertex, len(exits))
	switch {
	case len(exits) <= 1:
		return out
	case len(exits) >= 4:
		for i, e := range exits {
			out[i] = Heading(cx, band, e.Toward)
		}
		return out
	}
	order := make([]int, len(exits))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Or(cmp.Compare(exits[a].Toward, exits[b].Toward), cmp.Compare(exits[a].Edge, exits[b].Edge))
	})
	if len(exits) == 3 {
		out[order[0]], out[order[1]], out[order[2]] = Left, Bottom, Right
		return out
	}
	a, b := order[0], order[1]
	if Heading(cx, band, exits[a].Toward) == Left && Heading(cx, band, exits[b].Toward) == Right {
		out[a], out[b] = Left, Right
		return out
	}
	// The farther exit heads exactly at cx only when both do, and then
	// takes the right vertex: of two exits heading exactly at the center,
	// the first in edge order takes the right, the other the bottom.
	far := Farther(cx, exits)
	out[far], out[1-far] = Left, Bottom
	if exits[far].Toward >= cx {
		out[far] = Right
	}
	return out
}

// Farther is the index of the farther-heading of two exits of a node
// centered at cx (S8): the one heading farther from cx, and at equal
// distances the first in order of heading, ties by edge order.
func Farther(cx float64, exits []Exit) int {
	a, b := 0, 1
	if cmp.Or(cmp.Compare(exits[1].Toward, exits[0].Toward), cmp.Compare(exits[1].Edge, exits[0].Edge)) < 0 {
		a, b = 1, 0
	}
	if math.Abs(exits[b].Toward-cx) > math.Abs(exits[a].Toward-cx) {
		return b
	}
	return a
}

// Heading is the side an exit of a node centered at cx heads for when it
// heads for x: left or right beyond band, else the bottom.
func Heading(cx, band, x float64) Vertex {
	switch {
	case x < cx-band:
		return Left
	case x > cx+band:
		return Right
	}
	return Bottom
}

// LoopSide is the side vertex the next self-loop on a diamond or circle
// leaves from, given the side vertices other ends already hold (S8): the
// right one, unless it is held and the left one is free.
func LoopSide(leftHeld, rightHeld bool) Vertex {
	if rightHeld && !leftHeld {
		return Left
	}
	return Right
}
