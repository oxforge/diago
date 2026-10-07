package ports

import (
	"math"

	"github.com/oxforge/diago/internal/model"
)

// Face is one side of a node's box in the engine's top-to-bottom frame.
type Face int

// The four faces.
const (
	FaceIn    Face = iota // upstream: the top
	FaceOut               // downstream: the bottom
	FaceLeft              // the low end of the cross axis
	FaceRight             // the high end of the cross axis
)

// side is a side of a box in the output frame.
type side int

const (
	sideTop side = iota
	sideBottom
	sideLeft
	sideRight
)

// Inset is how far inside its box the drawn outline (C2.2) of a node of
// shape s lies on face f, at offset off from the middle of the face: along
// the cross axis on the in- and out-faces, along the flow axis on the side
// faces. w and h are the node's frame extents (w on the cross axis) and
// dir the output direction, which decides the side of the drawn shape a
// face becomes. A port or a side attachment at that offset ends Inset
// inside the box, on the outline (S8). In the text profile every shape is
// its box.
func Inset(s model.Shape, w, h float64, dir model.Direction, text bool, f Face, off float64) float64 {
	if text {
		return 0
	}
	sd, o := outputSide(f, off, dir)
	W, H := w, h
	if dir == model.Right || dir == model.Left {
		W, H = h, w
	}
	return depth(s, W, H, sd, o)
}

// outputSide maps face f and an offset along it onto the output frame's
// side and offset (positive toward the right or down).
func outputSide(f Face, off float64, dir model.Direction) (side, float64) {
	switch dir {
	case model.Up:
		return [...]side{sideBottom, sideTop, sideLeft, sideRight}[f], [...]float64{off, off, -off, -off}[f]
	case model.Right:
		return [...]side{sideLeft, sideRight, sideTop, sideBottom}[f], off
	case model.Left:
		return [...]side{sideRight, sideLeft, sideTop, sideBottom}[f], [...]float64{off, off, -off, -off}[f]
	}
	return [...]side{sideTop, sideBottom, sideLeft, sideRight}[f], off
}

// depth is the outline's distance inside the box on side sd at offset o,
// for a drawn W x H box (C2.2).
func depth(s model.Shape, W, H float64, sd side, o float64) float64 {
	if W <= 0 || H <= 0 {
		return 0
	}
	vertical := sd == sideLeft || sd == sideRight
	switch s {
	case model.ShapeCylinder:
		if !vertical {
			u := math.Min(1, math.Abs(o)/(W/2))
			return CapRatio * H * (1 - math.Sqrt(1-u*u))
		}
	case model.ShapeParallelogram:
		slant := Slant * H
		switch sd {
		case sideLeft:
			return slant * (H/2 - o) / H
		case sideRight:
			return slant * (o + H/2) / H
		}
	case model.ShapeHexagon:
		if vertical {
			return (W / 4) * math.Min(1, math.Abs(o)/(H/2))
		}
		if a := math.Abs(o); a > W/4 {
			return (H / 2) * math.Min(1, (a-W/4)/(W/4))
		}
	}
	return 0
}
