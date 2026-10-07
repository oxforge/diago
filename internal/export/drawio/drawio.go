// Package drawio exports a PositionedGraph as draw.io (mxGraph) XML.
package drawio

import (
	"encoding/xml"
	"fmt"
	"math"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// --- XML types for mxGraph ---

type mxFile struct {
	XMLName xml.Name  `xml:"mxfile"`
	Diagram mxDiagram `xml:"diagram"`
}

type mxDiagram struct {
	Name       string       `xml:"name,attr"`
	GraphModel mxGraphModel `xml:"mxGraphModel"`
}

type mxGraphModel struct {
	Root mxRoot `xml:"root"`
}

type mxRoot struct {
	Cells []mxCell `xml:"mxCell"`
}

type mxCell struct {
	XMLName  xml.Name    `xml:"mxCell"`
	ID       string      `xml:"id,attr"`
	Value    string      `xml:"value,attr,omitempty"`
	Style    string      `xml:"style,attr,omitempty"`
	Vertex   string      `xml:"vertex,attr,omitempty"`
	Edge     string      `xml:"edge,attr,omitempty"`
	Source   string      `xml:"source,attr,omitempty"`
	Target   string      `xml:"target,attr,omitempty"`
	Parent   string      `xml:"parent,attr,omitempty"`
	Geometry *mxGeometry `xml:"mxGeometry,omitempty"`
}

type mxGeometry struct {
	XMLName  xml.Name `xml:"mxGeometry"`
	X        float64  `xml:"x,attr,omitempty"`
	Y        float64  `xml:"y,attr,omitempty"`
	Width    float64  `xml:"width,attr,omitempty"`
	Height   float64  `xml:"height,attr,omitempty"`
	Relative string   `xml:"relative,attr,omitempty"`
	As       string   `xml:"as,attr"`
	Points   *mxArray `xml:"Array,omitempty"`
}

type mxArray struct {
	XMLName xml.Name  `xml:"Array"`
	As      string    `xml:"as,attr"`
	Points  []mxPoint `xml:"mxPoint"`
}

type mxPoint struct {
	XMLName xml.Name `xml:"mxPoint"`
	X       float64  `xml:"x,attr"`
	Y       float64  `xml:"y,attr"`
}

// resolveAccent resolves a color name or hex value to a hex accent string.
func resolveAccent(color string, th theme.Theme) string {
	if color == "" {
		return ""
	}
	if color[0] == '#' {
		return color
	}
	if hex, ok := th.Colors[color]; ok {
		return hex
	}
	return ""
}

// Export converts a PositionedGraph to draw.io XML bytes.
func Export(pg *model.PositionedGraph, th theme.Theme) []byte {
	cells := []mxCell{
		{ID: "0"},
		{ID: "1", Parent: "0"},
	}

	nodeMembership, groupMembership, groupByID := buildMembershipMaps(pg)

	cells = buildGroupCells(cells, pg, th, groupMembership, groupByID)
	cells = buildNodeCells(cells, pg, th, nodeMembership, groupByID)
	cells = buildEdgeCells(cells, pg, th)
	cells = buildTitleCells(cells, pg, th)

	doc := mxFile{
		Diagram: mxDiagram{
			Name: "Page-1",
			GraphModel: mxGraphModel{
				Root: mxRoot{Cells: cells},
			},
		},
	}

	out, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil
	}
	return append([]byte(xml.Header), out...)
}

func buildMembershipMaps(pg *model.PositionedGraph) (map[string]string, map[string]string, map[string]*model.PositionedGroup) {
	nodeMembership := make(map[string]string)
	groupMembership := make(map[string]string)
	for _, g := range pg.Groups {
		for _, nid := range g.Contains {
			nodeMembership[nid] = g.ID
		}
		for _, cid := range g.Children {
			groupMembership[cid] = g.ID
		}
	}

	groupByID := make(map[string]*model.PositionedGroup, len(pg.Groups))
	for i := range pg.Groups {
		groupByID[pg.Groups[i].ID] = &pg.Groups[i]
	}
	return nodeMembership, groupMembership, groupByID
}

func buildGroupCells(cells []mxCell, pg *model.PositionedGraph, th theme.Theme, groupMembership map[string]string, groupByID map[string]*model.PositionedGroup) []mxCell {
	for depth := 1; depth <= 3; depth++ {
		for _, g := range pg.Groups {
			if g.Depth != depth {
				continue
			}
			parent := "1"
			if pid, ok := groupMembership[g.ID]; ok {
				parent = pid
			}

			gx, gy := g.X, g.Y
			if pg, ok := groupByID[parent]; ok {
				gx -= pg.X
				gy -= pg.Y
			}

			// The title is its own cell, in front of the edges
			// (buildTitleCells): the swimlane keeps only its box.
			groupStyle := "swimlane;startSize=23;collapsible=0;rounded=1;"
			if accent := resolveAccent(g.Color, th); accent != "" {
				dc := theme.DeriveColors(accent, th.Background, theme.ElementGroup)
				groupStyle += fmt.Sprintf("fillColor=%s;strokeColor=%s;fontColor=%s;", dc.Fill, dc.Stroke, dc.Text)
			}

			cells = append(cells, mxCell{
				ID:     g.ID,
				Style:  groupStyle,
				Vertex: "1",
				Parent: parent,
				Geometry: &mxGeometry{
					X:      round(gx),
					Y:      round(gy),
					Width:  round(g.Width),
					Height: round(g.Height),
					As:     "geometry",
				},
			})
		}
	}
	return cells
}

// buildTitleCells appends each titled group's title as a text cell after
// every edge, so that draw.io draws it in front of them (C13): where the
// layout placed it, LabelOffset from the group's left side or 8 px in when
// the layout left it to the renderer, centered 16 px below the group's top
// side, on a background in the swimlane header's fill that gaps an edge
// under it. The cell sits on the page, not in the swimlane, so dragging
// the group by hand leaves its title behind.
func buildTitleCells(cells []mxCell, pg *model.PositionedGraph, th theme.Theme) []mxCell {
	const titleMid = 16.0
	for i, g := range pg.Groups {
		if g.Label == "" {
			continue
		}
		at := 8.0
		if g.LabelOffset > 0 {
			at = g.LabelOffset
		}
		style := fmt.Sprintf("text;align=left;verticalAlign=middle;spacing=0;fontSize=%g;", th.Group.LabelFont.Size)
		if th.Group.LabelFont.Weight >= 600 {
			style += "fontStyle=1;"
		}
		background := "default" // the swimlane header's own fill
		if accent := resolveAccent(g.Color, th); accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementGroup)
			style += fmt.Sprintf("fontColor=%s;", dc.Text)
			background = dc.Fill
		}
		style += fmt.Sprintf("labelBackgroundColor=%s;", background)
		cells = append(cells, mxCell{
			ID:     fmt.Sprintf("title_%d", i),
			Value:  g.Label,
			Style:  style,
			Vertex: "1",
			Parent: "1",
			Geometry: &mxGeometry{
				X:      round(g.X + at),
				Y:      round(g.Y + titleMid - g.LabelHeight/2),
				Width:  round(g.LabelWidth),
				Height: round(g.LabelHeight),
				As:     "geometry",
			},
		})
	}
	return cells
}

func buildNodeCells(cells []mxCell, pg *model.PositionedGraph, th theme.Theme, nodeMembership map[string]string, groupByID map[string]*model.PositionedGroup) []mxCell {
	for _, n := range pg.Nodes {
		parent := "1"
		if gid, ok := nodeMembership[n.ID]; ok {
			parent = gid
		}

		nx, ny := n.X-n.Width/2, n.Y-n.Height/2
		if g, ok := groupByID[parent]; ok {
			nx -= g.X
			ny -= g.Y
		}

		nStyle := nodeStyle(n.Shape)
		if accent := resolveAccent(n.Color, th); accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementNode)
			nStyle += fmt.Sprintf("fillColor=%s;strokeColor=%s;fontColor=%s;", dc.Fill, dc.Stroke, dc.Text)
		}

		cells = append(cells, mxCell{
			ID:     n.ID,
			Value:  n.Label,
			Style:  nStyle,
			Vertex: "1",
			Parent: parent,
			Geometry: &mxGeometry{
				X:      round(nx),
				Y:      round(ny),
				Width:  round(n.Width),
				Height: round(n.Height),
				As:     "geometry",
			},
		})
	}
	return cells
}

func buildEdgeCells(cells []mxCell, pg *model.PositionedGraph, th theme.Theme) []mxCell {
	nodeByID := make(map[string]model.PositionedNode, len(pg.Nodes))
	for _, n := range pg.Nodes {
		nodeByID[n.ID] = n
	}

	for i, e := range pg.Edges {
		edgeID := fmt.Sprintf("edge_%d", i)
		style := edgeStyle(e.Style, e.Direction)

		if accent := resolveAccent(e.Color, th); accent != "" {
			dc := theme.DeriveColors(accent, th.Background, theme.ElementEdge)
			style += fmt.Sprintf("strokeColor=%s;fontColor=%s;", dc.Stroke, dc.Text)
		}

		if len(e.Points) >= 2 {
			src, ok1 := nodeByID[e.From]
			tgt, ok2 := nodeByID[e.To]
			if ok1 && ok2 {
				exitX, exitY := portRelative(e.Points[0], src)
				entryX, entryY := portRelative(e.Points[len(e.Points)-1], tgt)
				style += fmt.Sprintf("exitX=%.2f;exitY=%.2f;exitDx=0;exitDy=0;entryX=%.2f;entryY=%.2f;entryDx=0;entryDy=0;", exitX, exitY, entryX, entryY)
			}
		}

		cell := mxCell{
			ID:     edgeID,
			Value:  e.Label,
			Style:  style,
			Edge:   "1",
			Source: e.From,
			Target: e.To,
			Parent: "1",
			Geometry: &mxGeometry{
				Relative: "1",
				As:       "geometry",
			},
		}

		if len(e.Points) > 2 {
			pts := make([]mxPoint, 0, len(e.Points)-2)
			for _, p := range e.Points[1 : len(e.Points)-1] {
				pts = append(pts, mxPoint{X: round(p.X), Y: round(p.Y)})
			}
			cell.Geometry.Points = &mxArray{
				As:     "points",
				Points: pts,
			}
		}

		cells = append(cells, cell)
	}
	return cells
}

// nodeStyle returns the draw.io style string for a node shape.
func nodeStyle(s model.Shape) string {
	switch s {
	case model.ShapeRect:
		return "rounded=0;whiteSpace=wrap;html=1;"
	case model.ShapeRounded:
		return "rounded=1;whiteSpace=wrap;html=1;"
	case model.ShapeCircle:
		return "ellipse;whiteSpace=wrap;html=1;"
	case model.ShapeDiamond:
		return "rhombus;whiteSpace=wrap;html=1;"
	case model.ShapeCylinder:
		return "shape=cylinder3;whiteSpace=wrap;html=1;"
	case model.ShapeHexagon:
		return "shape=hexagon;perimeter=hexagonPerimeter2;whiteSpace=wrap;html=1;"
	case model.ShapeParallelogram:
		return "shape=parallelogram;perimeter=parallelogramPerimeter;whiteSpace=wrap;html=1;"
	default:
		return "rounded=0;whiteSpace=wrap;html=1;"
	}
}

// edgeStyle returns the draw.io style string for an edge. diago routes
// every edge orthogonally, so every edge is an orthogonal draw.io edge.
func edgeStyle(s model.EdgeStyle, d model.EdgeDirection) string {
	style := "edgeStyle=orthogonalEdgeStyle;"

	switch s {
	case model.EdgeDashed:
		style += "dashed=1;"
	case model.EdgeDotted:
		style += "dashed=1;dashPattern=1 3;"
	case model.EdgeThick:
		style += "strokeWidth=3;"
	}

	switch d {
	case model.EdgeForward:
		style += "endArrow=block;endFill=1;startArrow=none;"
	case model.EdgeBackward:
		style += "startArrow=block;startFill=1;endArrow=none;"
	case model.EdgeBoth:
		style += "startArrow=block;startFill=1;endArrow=block;endFill=1;"
	case model.EdgeNone:
		style += "startArrow=none;endArrow=none;"
	}

	return style
}

// portRelative computes the relative (0–1) position of a port point on a node.
// Node coordinates are center-based; the result is relative to the node's top-left.
func portRelative(pt model.Point, n model.PositionedNode) (float64, float64) {
	left := n.X - n.Width/2
	top := n.Y - n.Height/2
	rx := (pt.X - left) / n.Width
	ry := (pt.Y - top) / n.Height
	// Clamp to [0,1].
	rx = math.Max(0, math.Min(1, rx))
	ry = math.Max(0, math.Min(1, ry))
	return math.Round(rx*100) / 100, math.Round(ry*100) / 100
}

// round rounds to 1 decimal place to keep XML clean.
func round(v float64) float64 {
	return math.Round(v*10) / 10
}
