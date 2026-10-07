// Package ports holds the port geometry that place (S7), route (S8) and
// the flat edges' router (S9) share: which shapes pin their ports to
// vertices, how far each face reaches, the order of the ends on a face,
// and the vertex each exit of a diamond or circle leaves from. Everything
// is in the engine's top-to-bottom frame: a node's in-face is its upstream
// side, its out-face its downstream side, and offsets run along the cross
// axis from its center, along the flow axis on the side faces.
package ports

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// Shape ratios of the drawn outlines (C2.2).
const (
	Slant    = 0.3  // a parallelogram's slant, as a fraction of its height
	CapRatio = 0.18 // a cylinder's cap, as a fraction of its height
)

// Pinned reports whether a node of shape s connects only at its four
// vertices (C8): diamonds and circles, except in the text profile, where
// every shape is a box.
func Pinned(s model.Shape, text bool) bool {
	return !text && (s == model.ShapeDiamond || s == model.ShapeCircle)
}

// Span is the stretch of a face that ports may use, as offsets from the
// node's center. Cells marks a text-profile face, whose ports sit a whole
// number of cells apart.
type Span struct {
	Lo, Hi float64
	Cells  bool
}

// At is the position of port k (from 0) of n spread evenly over the span:
// the span splits into n + 1 equal parts. On a text-profile face the parts
// round down to whole cells, at least one, the ports center on the span's
// middle, and each moves to the center of the cell it falls in, so ports
// sit on distinct cells. A port on the line between two cells takes the
// one farther from the middle, so the ports stay symmetric about it, and
// one on the middle itself the cell after it. The zero Span puts every
// port on the center.
func (s Span) At(k, n int) float64 {
	if !s.Cells {
		return s.Lo + (s.Hi-s.Lo)*float64(k+1)/float64(n+1)
	}
	step := max(1, math.Floor((s.Hi-s.Lo)/float64(n+1)))
	mid := (s.Hi - s.Lo) / 2
	at := mid + (float64(k)-float64(n-1)/2)*step
	if at < mid && at == math.Floor(at) {
		return s.Lo + at - 0.5
	}
	return s.Lo + math.Floor(at) + 0.5
}

// Faces returns the usable spans of the in-face and the out-face of a
// node of shape s with frame extents w (cross axis) and h (flow axis),
// laid out in direction dir (S8, Shape ports). Pinned shapes have zero
// spans. Otherwise a span is the whole side, except where the drawn
// outline's side does not run across the flow: a hexagon's flat faces
// (the middle half) under DOWN and UP; a parallelogram's top and bottom
// edges, each shortened by the slant at opposite ends, under DOWN and UP;
// and a cylinder's side lines between its caps under RIGHT and LEFT, where
// w is its drawn height. In the text profile every span is the whole side,
// in cells.
func Faces(s model.Shape, w, h float64, dir model.Direction, text bool) (in, out Span) {
	if Pinned(s, text) {
		return Span{}, Span{}
	}
	full := Span{Lo: -w / 2, Hi: w / 2}
	if text {
		full.Cells = true
		return full, full
	}
	vertical := dir == model.Down || dir == model.Up
	switch s {
	case model.ShapeHexagon:
		if vertical {
			flat := Span{Lo: -w / 4, Hi: w / 4}
			return flat, flat
		}
	case model.ShapeParallelogram:
		if vertical {
			top := Span{Lo: -w/2 + Slant*h, Hi: w / 2}
			bottom := Span{Lo: -w / 2, Hi: w/2 - Slant*h}
			if dir == model.Down {
				return top, bottom
			}
			return bottom, top
		}
	case model.ShapeCylinder:
		if !vertical {
			side := Span{Lo: -w/2 + CapRatio*w, Hi: w/2 - CapRatio*w}
			return side, side
		}
	}
	return full, full
}

// Sides returns the usable spans of the left and the right face of a node
// of shape s with frame extents w (cross axis) and h (flow axis), laid out
// in direction dir, as offsets along the flow axis from its center: where
// the drawn outline's side is that face's side (C7), so that a run leaving
// it across the flow is perpendicular to the drawn shape (S8, Self-loops;
// S9, Flat edges). Pinned shapes have zero spans: their side vertices.
// Otherwise a span is the whole side, except a cylinder's side lines
// between its caps under DOWN and UP, and, under RIGHT and LEFT, where the
// side faces become the drawn top and bottom, a hexagon's flat faces (the
// middle half) and a parallelogram's top and bottom edges, each shortened
// by the slant (0.3 times the drawn height, w) at opposite ends. In the
// text profile every span is the whole side, in cells.
func Sides(s model.Shape, w, h float64, dir model.Direction, text bool) (left, right Span) {
	if Pinned(s, text) {
		return Span{}, Span{}
	}
	full := Span{Lo: -h / 2, Hi: h / 2}
	if text {
		full.Cells = true
		return full, full
	}
	vertical := dir == model.Down || dir == model.Up
	switch s {
	case model.ShapeCylinder:
		if vertical {
			side := Span{Lo: -h/2 + CapRatio*h, Hi: h/2 - CapRatio*h}
			return side, side
		}
	case model.ShapeHexagon:
		if !vertical {
			flat := Span{Lo: -h / 4, Hi: h / 4}
			return flat, flat
		}
	case model.ShapeParallelogram:
		if !vertical {
			// The left face is the drawn top, whose edge starts the slant
			// in from the drawn left; the right face the drawn bottom,
			// which stops the slant short of the drawn right. LEFT runs
			// the flow axis against the drawn x.
			top := Span{Lo: -h/2 + Slant*w, Hi: h / 2}
			bottom := Span{Lo: -h / 2, Hi: h/2 - Slant*w}
			if dir == model.Right {
				return top, bottom
			}
			return bottom, top
		}
	}
	return full, full
}

// Room is how wide across the flow a node of shape s, h long on the flow
// axis and laid out on screen in direction dir, must be for the usable
// span of each of its faces to reach span (S8, Shape ports): Faces turned
// round. A pinned shape has no faces and needs no room.
func Room(s model.Shape, span, h float64, dir model.Direction) float64 {
	if Pinned(s, false) {
		return 0
	}
	vertical := dir == model.Down || dir == model.Up
	switch s {
	case model.ShapeHexagon:
		if vertical {
			return 2 * span
		}
	case model.ShapeParallelogram:
		if vertical {
			return span + Slant*h
		}
	case model.ShapeCylinder:
		if !vertical {
			return span / (1 - 2*CapRatio)
		}
	}
	return span
}
