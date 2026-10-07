// Package excalidraw exports a PositionedGraph as Excalidraw v2 JSON.
package excalidraw

import (
	"encoding/json"
	"fmt"

	"github.com/oxforge/diago/internal/model"
	"github.com/oxforge/diago/internal/theme"
)

// Export converts a PositionedGraph to Excalidraw v2 JSON bytes.
func Export(pg *model.PositionedGraph, th theme.Theme) ([]byte, error) {
	b := builder{
		th:              th,
		elements:        make([]element, 0, len(pg.Nodes)*2+len(pg.Edges)*2),
		nodeGroupIDs:    make(map[string][]string),
		nodeMembership:  make(map[string]string),
		groupMembership: make(map[string]string),
		groupByID:       make(map[string]*model.PositionedGroup),
	}

	// Build membership maps.
	for i := range pg.Groups {
		g := &pg.Groups[i]
		b.groupByID[g.ID] = g
		for _, nid := range g.Contains {
			b.nodeMembership[nid] = g.ID
		}
		for _, cid := range g.Children {
			b.groupMembership[cid] = g.ID
		}
	}

	// Build groupIds chains for each group (inner first, then outer).
	groupChains := make(map[string][]string)
	for _, g := range pg.Groups {
		chain := []string{g.ID}
		cur := g.ID
		for {
			parent, ok := b.groupMembership[cur]
			if !ok {
				break
			}
			chain = append(chain, parent)
			cur = parent
		}
		groupChains[g.ID] = chain
	}

	// Assign groupIds to nodes.
	for _, n := range pg.Nodes {
		if gid, ok := b.nodeMembership[n.ID]; ok {
			b.nodeGroupIDs[n.ID] = groupChains[gid]
		}
	}

	// Collect which shapes each arrow binds to, so we can set boundElements on shapes.
	shapeBoundArrows := make(map[string][]boundRef)

	// Emit nodes (shape + text pairs).
	for _, n := range pg.Nodes {
		gids := b.nodeGroupIDs[n.ID]
		b.addNode(n, gids)
	}

	// Emit edges (arrow + optional label text).
	for i, e := range pg.Edges {
		arrowID := fmt.Sprintf("edge_%d", i)
		labelID := fmt.Sprintf("edgelabel_%d", i)

		// Track bindings on the source/target shapes.
		shapeBoundArrows[e.From] = append(shapeBoundArrows[e.From], boundRef{ID: arrowID, Type: "arrow"})
		shapeBoundArrows[e.To] = append(shapeBoundArrows[e.To], boundRef{ID: arrowID, Type: "arrow"})

		b.addEdge(e, arrowID, labelID)
	}

	// Emit group labels last, in front of the arrows (C13). They follow the
	// edge labels too, which stay right after their arrows as bound text;
	// C14.1 keeps a label off a title box, so at most a backing's pad can
	// touch one.
	for _, g := range pg.Groups {
		if g.Label != "" {
			b.addGroupLabel(g, groupChains[g.ID])
		}
	}

	// Patch shapes with arrow boundElements.
	for idx := range b.elements {
		el := &b.elements[idx]
		if refs, ok := shapeBoundArrows[el.ID]; ok {
			el.BoundElements = append(el.BoundElements, refs...)
		}
	}

	doc := document{
		Type:     "excalidraw",
		Version:  2,
		Elements: b.elements,
	}

	return json.MarshalIndent(doc, "", "  ")
}

// --- Types ---

type document struct {
	Type     string    `json:"type"`
	Version  int       `json:"version"`
	Elements []element `json:"elements"`
}

type element struct {
	Type            string     `json:"type"`
	ID              string     `json:"id"`
	X               float64    `json:"x"`
	Y               float64    `json:"y"`
	Width           float64    `json:"width"`
	Height          float64    `json:"height"`
	StrokeColor     string     `json:"strokeColor"`
	BackgroundColor string     `json:"backgroundColor"`
	FillStyle       string     `json:"fillStyle"`
	StrokeWidth     float64    `json:"strokeWidth"`
	Roughness       int        `json:"roughness"`
	Opacity         int        `json:"opacity"`
	Angle           float64    `json:"angle"`
	Seed            int        `json:"seed"`
	Version         int        `json:"version"`
	IsDeleted       bool       `json:"isDeleted"`
	BoundElements   []boundRef `json:"boundElements"`
	GroupIDs        []string   `json:"groupIds"`
	Roundness       *roundness `json:"roundness,omitempty"`
	ContainerID     *string    `json:"containerId"`

	// Text-specific fields.
	Text          string `json:"text,omitempty"`
	FontSize      int    `json:"fontSize,omitempty"`
	FontFamily    int    `json:"fontFamily,omitempty"`
	TextAlign     string `json:"textAlign,omitempty"`
	VerticalAlign string `json:"verticalAlign,omitempty"`

	// Arrow-specific fields.
	Points         [][]float64 `json:"points,omitempty"`
	StartBinding   *binding    `json:"startBinding"`
	EndBinding     *binding    `json:"endBinding"`
	StartArrowhead *string     `json:"startArrowhead"`
	EndArrowhead   *string     `json:"endArrowhead"`
	StrokeStyle    string      `json:"strokeStyle,omitempty"`
}

type boundRef struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type binding struct {
	ElementID string  `json:"elementId"`
	Focus     float64 `json:"focus"`
	Gap       float64 `json:"gap"`
}

type roundness struct {
	Type int `json:"type"`
}
