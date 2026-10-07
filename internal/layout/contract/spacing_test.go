package contract

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/oxforge/diago/internal/model"
)

// pair lays out two free wires; C9 reads only their geometry.
func pair(e1, e2 model.PositionedEdge, nodes ...model.PositionedNode) []Violation {
	return check(layout(400, 300, nodes, e1, e2))
}

func TestC9_1_NoSharedRuns(t *testing.T) {
	tests := []struct {
		name string
		e2   model.PositionedEdge
		want bool
	}{
		{"collinear, 60 px shared", wire("r", "s", 60, 50, 160, 50), true},
		{"0.8 px apart, 60 px shared", wire("r", "s", 60, 50.8, 160, 50.8), true},
		{"touching end to end", wire("r", "s", 120, 50, 200, 50), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, has(pair(wire("p", "q", 20, 50, 120, 50), tt.e2), "C9.1", "p->q#0"))
		})
	}
}

func TestC9_1_SharedVertexBundle(t *testing.T) {
	d := shaped("d", model.ShapeDiamond, 100, 43, 80, 46) // bottom vertex (100,66)
	e1 := wire("d", "q", 100, 66, 100, 100, 60, 100)
	e2 := wire("d", "s", 100, 66, 100, 100, 140, 100)
	assert.False(t, has(pair(e1, e2, d), "C9.1", "d->q#0"), "stubs bundled on one vertex")

	d.Shape = model.ShapeRect
	assert.True(t, has(pair(e1, e2, d), "C9.1", "d->q#0"), "a rect has no shared vertex")
}

func TestC9_2_TrackGap(t *testing.T) {
	tests := []struct {
		name string
		e2   model.PositionedEdge
		want bool
	}{
		{"5 px apart, side by side", wire("r", "s", 60, 55, 160, 55), true},
		{"7.5 px apart", wire("r", "s", 60, 57.5, 160, 57.5), false},
		{"3 px end to end on one track", wire("r", "s", 123, 50, 200, 50), true},
		{"perpendicular", wire("r", "s", 70, 20, 70, 120), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, has(pair(wire("p", "q", 20, 50, 120, 50), tt.e2), "C9.2", "p->q#0"))
		})
	}
}

func TestC9_2_SameFaceBundle(t *testing.T) {
	// Two wires from p: their first segments are a rake (terminal at p),
	// 5 px apart, within the port gap. Their second segments are near p's
	// face too but neither is terminal there, so the full track gap
	// applies and they now violate. Their third segments share no
	// endpoint node and always violate.
	e1 := wire("p", "q", 20, 50, 20, 70, 60, 70, 60, 120)
	e2 := wire("p", "s", 25, 50, 25, 65, 65, 65, 65, 120)
	vs := pair(e1, e2)
	var segs []string
	for _, v := range vs {
		if v.Rule == "C9.2" {
			segs = append(segs, v.Detail[:9])
		}
	}
	assert.Equal(t, []string{"segment 1", "segment 2"}, segs)

	e2.From = "r"
	assert.Len(t, filter(pair(e1, e2), "C9.2"), 3, "different sources: no bundle")
}

func TestC9_2_PortGap(t *testing.T) {
	parallelAB := wire("a", "b", 20, 53, 120, 53)
	parallelAB.ID = "a->b#1"
	tests := []struct {
		name string
		e1   model.PositionedEdge
		e2   model.PositionedEdge
		want bool
	}{
		{
			"rake, terminal segments 5 px apart: within the port gap",
			wire("p", "q", 20, 50, 120, 50),
			wire("p", "s", 20, 55, 120, 55),
			false,
		},
		{
			"rake, 325 px terminal runs 2 px apart: under the port gap",
			wire("p", "q", 20, 50, 345, 50),
			wire("p", "s", 20, 52, 345, 52),
			true,
		},
		{
			"two parallel a->b wires 3 px apart along their whole length",
			wire("a", "b", 20, 50, 120, 50),
			parallelAB,
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, has(pair(tt.e1, tt.e2), "C9.2", tt.e1.ID))
		})
	}
}

func TestC9_2_Comb(t *testing.T) {
	// d's top vertex is (100,120) (see fan()'s diamond geometry). e1 and e2
	// both enter it; their second-to-last segments are near d's face
	// without being terminal there.
	d := shaped("d", model.ShapeDiamond, 100, 143, 80, 46)
	e1 := wire("m1", "d", 60, 50, 60, 100, 100, 100, 100, 120)
	e2 := wire("m2", "d", 140, 50, 140, 100, 100, 100, 100, 120)
	assert.False(t, has(pair(e1, e2, d), "C9.2", "m1->d#0"),
		"second-to-last segments meet end to end on the vertex column: a comb, no floor")

	e2side := wire("m2", "d", 70, 45, 70, 105, 100, 105, 100, 120)
	assert.True(t, has(pair(e1, e2side, d), "C9.2", "m1->d#0"),
		"the same segments moved side by side, 5 px apart: the full track gap applies")
}

// filter returns the violations of one rule.
func filter(vs []Violation, rule string) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Rule == rule {
			out = append(out, v)
		}
	}
	return out
}

func TestC9_1_Rail(t *testing.T) {
	// d's top vertex is (100,120). m1 and m2 converge on it along the lane
	// at y 100, sharing the run from x 70 to 100 before the vertex.
	d := shaped("d", model.ShapeDiamond, 100, 143, 80, 46)
	m1 := wire("m1", "d", 40, 50, 40, 100, 100, 100, 100, 120)
	m2 := wire("m2", "d", 70, 50, 70, 100, 100, 100, 100, 120)
	assert.False(t, has(pair(m1, m2, d), "C9.1", "m1->d#0"), "a rail into one vertex")

	d.Shape = model.ShapeRect
	assert.True(t, sharesRun(pair(m1, m2, d), 1), "a rect has no shared vertex: the run is shared")

	d.Shape = model.ShapeDiamond
	past := wire("m2", "q", 70, 50, 70, 100, 100, 100, 100, 200)
	assert.True(t, sharesRun(pair(m1, past, d, rect("q", 100, 220)), 1),
		"an edge that passes the vertex to another node shares the run")

	// d's bottom vertex is (100,166): d->p and d->s leave it along one run.
	p := wire("d", "p", 100, 166, 100, 190, 40, 190, 40, 240)
	q := wire("d", "s", 100, 166, 100, 190, 70, 190, 70, 240)
	assert.False(t, has(pair(p, q, d), "C9.1", "d->p#0"), "a rail out of one vertex")
}

// sharesRun reports whether vs holds a C9.1 violation of segment i.
func sharesRun(vs []Violation, i int) bool {
	for _, v := range filter(vs, "C9.1") {
		if strings.HasPrefix(v.Detail, fmt.Sprintf("segment %d shares", i)) {
			return true
		}
	}
	return false
}
