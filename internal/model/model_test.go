package model_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirectionString(t *testing.T) {
	tests := []struct {
		d    model.Direction
		want string
	}{
		{model.Auto, "AUTO"},
		{model.Down, "DOWN"},
		{model.Up, "UP"},
		{model.Right, "RIGHT"},
		{model.Left, "LEFT"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.d.String())
		})
	}
}

func TestShapeString(t *testing.T) {
	tests := []struct {
		s    model.Shape
		want string
	}{
		{model.ShapeRect, "rect"},
		{model.ShapeRounded, "rounded"},
		{model.ShapeCircle, "circle"},
		{model.ShapeDiamond, "diamond"},
		{model.ShapeCylinder, "cylinder"},
		{model.ShapeHexagon, "hexagon"},
		{model.ShapeParallelogram, "parallelogram"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.String())
		})
	}
}

func TestEdgeStyleString(t *testing.T) {
	tests := []struct {
		e    model.EdgeStyle
		want string
	}{
		{model.EdgeSolid, "solid"},
		{model.EdgeDashed, "dashed"},
		{model.EdgeDotted, "dotted"},
		{model.EdgeThick, "thick"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.e.String())
		})
	}
}

func TestParseDirection(t *testing.T) {
	tests := []struct {
		input   string
		want    model.Direction
		wantErr bool
	}{
		{"DOWN", model.Down, false},
		{"", model.Auto, false},
		{"AUTO", model.Auto, false},
		{"UP", model.Up, false},
		{"RIGHT", model.Right, false},
		{"LEFT", model.Left, false},
		{"UNDEFINED", model.Auto, false},
		{"invalid", model.Auto, true},
		{"top-bottom", model.Auto, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := model.ParseDirection(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseShape(t *testing.T) {
	tests := []struct {
		input   string
		want    model.Shape
		wantErr bool
	}{
		{"rect", model.ShapeRect, false},
		{"", model.ShapeRect, false},
		{"rounded", model.ShapeRounded, false},
		{"circle", model.ShapeCircle, false},
		{"diamond", model.ShapeDiamond, false},
		{"cylinder", model.ShapeCylinder, false},
		{"hexagon", model.ShapeHexagon, false},
		{"parallelogram", model.ShapeParallelogram, false},
		{"octagon", model.ShapeRect, true},
		{"square", model.ShapeRect, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := model.ParseShape(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseEdgeStyle(t *testing.T) {
	tests := []struct {
		input   string
		want    model.EdgeStyle
		wantErr bool
	}{
		{"solid", model.EdgeSolid, false},
		{"", model.EdgeSolid, false},
		{"dashed", model.EdgeDashed, false},
		{"dotted", model.EdgeDotted, false},
		{"thick", model.EdgeThick, false},
		{"bold", model.EdgeSolid, true},
		{"double", model.EdgeSolid, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := model.ParseEdgeStyle(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestInteractionStyleString(t *testing.T) {
	tests := []struct {
		s    model.InteractionStyle
		want string
	}{
		{model.InteractionSolid, "solid"},
		{model.InteractionDashed, "dashed"},
		{model.InteractionAsync, "async"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.s.String())
		})
	}
}

func TestFragmentTypeString(t *testing.T) {
	tests := []struct {
		f    model.FragmentType
		want string
	}{
		{model.FragmentAlt, "alt"},
		{model.FragmentOpt, "opt"},
		{model.FragmentLoop, "loop"},
		{model.FragmentPar, "par"},
		{model.FragmentBreak, "break"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.f.String())
		})
	}
}

func TestParseInteractionStyle(t *testing.T) {
	tests := []struct {
		input   string
		want    model.InteractionStyle
		wantErr bool
	}{
		{"solid", model.InteractionSolid, false},
		{"", model.InteractionSolid, false},
		{"dashed", model.InteractionDashed, false},
		{"async", model.InteractionAsync, false},
		{"dotted", model.InteractionSolid, true},
		{"arrow", model.InteractionSolid, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := model.ParseInteractionStyle(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseFragmentType(t *testing.T) {
	tests := []struct {
		input   string
		want    model.FragmentType
		wantErr bool
	}{
		{"alt", model.FragmentAlt, false},
		{"opt", model.FragmentOpt, false},
		{"loop", model.FragmentLoop, false},
		{"par", model.FragmentPar, false},
		{"break", model.FragmentBreak, false},
		{"if", model.FragmentAlt, true},
		{"group", model.FragmentAlt, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := model.ParseFragmentType(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseDirection_Auto(t *testing.T) {
	d, err := model.ParseDirection("AUTO")
	require.NoError(t, err)
	assert.Equal(t, model.Auto, d)
}

func TestDirection_AutoString(t *testing.T) {
	assert.Equal(t, "AUTO", model.Auto.String())
}

func TestParseDirection_UndefinedBackwardCompat(t *testing.T) {
	d, err := model.ParseDirection("UNDEFINED")
	require.NoError(t, err)
	assert.Equal(t, model.Auto, d)
}

func TestEdgeDirectionString(t *testing.T) {
	tests := []struct {
		dir  model.EdgeDirection
		want string
	}{
		{model.EdgeForward, "forward"},
		{model.EdgeBackward, "backward"},
		{model.EdgeBoth, "both"},
		{model.EdgeDirection(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.dir.String())
		})
	}
}

func TestParseEdgeDirection(t *testing.T) {
	tests := []struct {
		input string
		want  model.EdgeDirection
		err   bool
	}{
		{"forward", model.EdgeForward, false},
		{"backward", model.EdgeBackward, false},
		{"both", model.EdgeBoth, false},
		{"", model.EdgeForward, false},
		{"invalid", model.EdgeForward, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := model.ParseEdgeDirection(tt.input)
			if tt.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestPositionedNode_ColorRoundTrip(t *testing.T) {
	node := model.PositionedNode{
		ID: "a", Label: "A", Shape: model.ShapeRect,
		X: 50, Y: 50, Width: 80, Height: 40, Color: "red",
	}
	data, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	var got model.PositionedNode
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Color != "red" {
		t.Errorf("color not preserved: got %q", got.Color)
	}
}

func TestPositionedGraph_JSONRoundTrip(t *testing.T) {
	lp := &model.Point{X: 250, Y: 100}
	orig := model.PositionedGraph{
		Nodes: []model.PositionedNode{
			{ID: "n1", Label: "Client", Shape: model.ShapeRect, X: 100, Y: 50, Width: 120, Height: 40},
			{ID: "n2", Label: "API", Lines: []string{"API", "Gateway"}, Shape: model.ShapeRounded, X: 300, Y: 50, Width: 140, Height: 40},
			{ID: "n3", Label: "DB", Shape: model.ShapeCylinder, X: 300, Y: 150, Width: 120, Height: 40},
		},
		Edges: []model.PositionedEdge{
			{
				From:        "n1",
				To:          "n2",
				Label:       "HTTPS",
				Style:       model.EdgeDashed,
				Direction:   model.EdgeForward,
				Points:      []model.Point{{X: 160, Y: 50}, {X: 230, Y: 50}},
				LabelPos:    lp,
				LabelWidth:  50,
				LabelHeight: 16,
			},
			{
				From:      "n2",
				To:        "n3",
				Style:     model.EdgeSolid,
				Direction: model.EdgeBoth,
				Points:    []model.Point{{X: 300, Y: 70}, {X: 300, Y: 130}},
			},
		},
		Groups: []model.PositionedGroup{
			{ID: "g1", Label: "Backend", X: 200, Y: 10, Width: 300, Height: 200, Contains: []string{"n2", "n3"}, Children: []string{}, Depth: 1},
		},
		Width:  600,
		Height: 400,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	js := string(data)
	// Verify enum values are strings, not integers.
	assert.True(t, strings.Contains(js, `"shape":"rect"`), "ShapeRect should marshal as \"rect\"")
	assert.True(t, strings.Contains(js, `"shape":"rounded"`), "ShapeRounded should marshal as \"rounded\"")
	assert.True(t, strings.Contains(js, `"shape":"cylinder"`), "ShapeCylinder should marshal as \"cylinder\"")
	assert.True(t, strings.Contains(js, `"style":"dashed"`), "EdgeDashed should marshal as \"dashed\"")
	assert.True(t, strings.Contains(js, `"style":"solid"`), "EdgeSolid should marshal as \"solid\"")
	assert.True(t, strings.Contains(js, `"direction":"forward"`), "EdgeForward should marshal as \"forward\"")
	assert.True(t, strings.Contains(js, `"direction":"both"`), "EdgeBoth should marshal as \"both\"")

	var got model.PositionedGraph
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, orig.Width, got.Width)
	assert.Equal(t, orig.Height, got.Height)
	require.Len(t, got.Nodes, len(orig.Nodes))
	for i, n := range orig.Nodes {
		assert.Equal(t, n.ID, got.Nodes[i].ID)
		assert.Equal(t, n.Label, got.Nodes[i].Label)
		assert.Equal(t, n.Shape, got.Nodes[i].Shape)
		assert.Equal(t, n.X, got.Nodes[i].X)
		assert.Equal(t, n.Y, got.Nodes[i].Y)
		assert.Equal(t, n.Lines, got.Nodes[i].Lines)
	}
	require.Len(t, got.Edges, len(orig.Edges))
	for i, e := range orig.Edges {
		assert.Equal(t, e.From, got.Edges[i].From)
		assert.Equal(t, e.To, got.Edges[i].To)
		assert.Equal(t, e.Style, got.Edges[i].Style)
		assert.Equal(t, e.Direction, got.Edges[i].Direction)
		assert.Equal(t, e.Points, got.Edges[i].Points)
		if e.LabelPos != nil {
			require.NotNil(t, got.Edges[i].LabelPos)
			assert.Equal(t, *e.LabelPos, *got.Edges[i].LabelPos)
		}
	}
	require.Len(t, got.Groups, 1)
	assert.Equal(t, orig.Groups[0].ID, got.Groups[0].ID)
	assert.Equal(t, orig.Groups[0].Depth, got.Groups[0].Depth)
}

func TestPositionedSequence_JSONRoundTrip(t *testing.T) {
	orig := model.PositionedSequence{
		Actors: []model.PositionedActor{
			{ID: "client", Label: "Client", BoxX: 10, BoxY: 10, BoxW: 80, BoxH: 30, LineX: 50, LineTop: 40, LineBottom: 300},
			{ID: "server", Label: "Server", BoxX: 200, BoxY: 10, BoxW: 80, BoxH: 30, LineX: 240, LineTop: 40, LineBottom: 300},
		},
		Interactions: []model.PositionedInteraction{
			{From: "client", To: "server", Label: "GET /", Style: model.InteractionSolid, Y: 80, FromX: 50, ToX: 240},
			{From: "server", To: "client", Label: "200 OK", Style: model.InteractionDashed, Y: 120, FromX: 240, ToX: 50},
			{From: "client", To: "client", Label: "think", Style: model.InteractionAsync, Y: 160, FromX: 50, ToX: 50,
				IsSelf: true, SelfPoints: []model.Point{{X: 50, Y: 160}, {X: 90, Y: 160}, {X: 90, Y: 180}, {X: 50, Y: 180}}},
		},
		Fragments: []model.PositionedFragment{
			{
				Type:      model.FragmentAlt,
				X:         0,
				Y:         70,
				Width:     300,
				Height:    120,
				TabWidth:  38,
				TabHeight: 24,
				TabCut:    6,
				Sections: []model.PositionedFragmentSection{
					{Label: "success", Y: 70, LabelX: 46, LabelY: 82, LabelWidth: 60},
					{Label: "error", Y: 130, LabelX: 8, LabelY: 142, LabelWidth: 44},
				},
			},
		},
		Activations: []model.PositionedActivation{
			{ActorID: "server", X: 235, Y: 80, Width: 10, Height: 40},
		},
		Width:  350,
		Height: 320,
	}

	data, err := json.Marshal(orig)
	require.NoError(t, err)

	js := string(data)
	// Verify enum values are strings.
	assert.True(t, strings.Contains(js, `"style":"solid"`), "InteractionSolid should marshal as \"solid\"")
	assert.True(t, strings.Contains(js, `"style":"dashed"`), "InteractionDashed should marshal as \"dashed\"")
	assert.True(t, strings.Contains(js, `"style":"async"`), "InteractionAsync should marshal as \"async\"")
	assert.True(t, strings.Contains(js, `"type":"alt"`), "FragmentAlt should marshal as \"alt\"")

	var got model.PositionedSequence
	require.NoError(t, json.Unmarshal(data, &got))

	assert.Equal(t, orig.Width, got.Width)
	assert.Equal(t, orig.Height, got.Height)
	require.Len(t, got.Actors, len(orig.Actors))
	for i, a := range orig.Actors {
		assert.Equal(t, a.ID, got.Actors[i].ID)
		assert.Equal(t, a.BoxX, got.Actors[i].BoxX)
		assert.Equal(t, a.LineBottom, got.Actors[i].LineBottom)
	}
	require.Len(t, got.Interactions, len(orig.Interactions))
	for i, ia := range orig.Interactions {
		assert.Equal(t, ia.Style, got.Interactions[i].Style)
		assert.Equal(t, ia.IsSelf, got.Interactions[i].IsSelf)
		assert.Equal(t, ia.SelfPoints, got.Interactions[i].SelfPoints)
	}
	require.Len(t, got.Fragments, 1)
	assert.Equal(t, orig.Fragments[0].Type, got.Fragments[0].Type)
	assert.Equal(t, orig.Fragments[0].Sections, got.Fragments[0].Sections)
	require.Len(t, got.Activations, 1)
	assert.Equal(t, orig.Activations[0].ActorID, got.Activations[0].ActorID)
}
