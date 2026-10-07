package text

import (
	"github.com/oxforge/diago/internal/model"
)

// drawNode paints the node box: one geometry for every shape, decorated by frame.
func drawNode(g *charGrid, n model.PositionedNode) {
	left, top := px2col(n.X-n.Width/2), px2row(n.Y-n.Height/2)
	right, bottom := px2col(n.X+n.Width/2)-1, px2row(n.Y+n.Height/2)-1
	w, h := right-left+1, bottom-top+1
	if w < 3 || h < 3 {
		return
	}
	if n.Members != nil {
		drawClassBox(g, n, left, top, w, h)
		return
	}
	lines := n.Lines
	if lines == nil {
		lines = []string{n.Label}
	}
	var frame boxFrame
	switch n.Shape {
	case model.ShapeRounded, model.ShapeCircle:
		frame.Rounded = true
	case model.ShapeCylinder:
		frame.Top = '═'
	case model.ShapeDiamond:
		mid := (len(lines) - 1) / 2
		decorated := make([]string, len(lines))
		for i, l := range lines {
			if i == mid {
				decorated[i] = "◇ " + l + " ◇"
			} else {
				decorated[i] = l
			}
		}
		lines = decorated
	}
	g.box(left, top, w, h, lines, frame)
}
