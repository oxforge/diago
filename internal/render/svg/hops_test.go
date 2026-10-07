package svg

import (
	"strings"
	"testing"

	"github.com/oxforge/diago/internal/model"
)

// plainPolylinePath is the expected SVG path for a two-point segment with no
// hop arcs, used as the "no-A-command" baseline in several tests below.
func plainPolylinePath(pts []model.Point) string {
	// matches the M…L… format produced by polylineWithHops when there are no
	// valid crossings on any segment.
	return polylineWithHops(pts, nil)
}

func TestPolylineWithHops_MidSegmentHorizontal(t *testing.T) {
	// Single horizontal segment (0,0)→(100,0).  A crossing at T=0.5 lands at
	// x=50, which is well clear of both endpoints (distance 50 >> hopRadius 5).
	// Expect an "A" arc command centred around x=50.
	points := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}
	crossings := []model.Crossing{{SegmentIndex: 0, T: 0.5}}

	got := polylineWithHops(points, crossings)

	if !strings.Contains(got, "A") {
		t.Errorf("expected an arc command in output, got: %q", got)
	}

	// Pre-crossing point: x = 50 - hopRadius*ux = 50 - 5 = 45, y = 0.
	if !strings.Contains(got, "L45,0") {
		t.Errorf("expected line to pre-crossing point L45,0, got: %q", got)
	}

	// Arc lands at post-crossing point: x = 55, y = 0.
	if !strings.Contains(got, "55,0") {
		t.Errorf("expected arc endpoint 55,0 in output, got: %q", got)
	}

	// Path must begin at origin and end at (100,0).
	if !strings.HasPrefix(got, "M0,0") {
		t.Errorf("expected path to start with M0,0, got: %q", got)
	}
	if !strings.HasSuffix(got, "L100,0") {
		t.Errorf("expected path to end with L100,0, got: %q", got)
	}
}

func TestPolylineWithHops_CrossingTooCloseToEndIsDropped(t *testing.T) {
	// hopRadius = 5; segLen = 100.
	// T = 0.04  → T*segLen = 4 < 5  → dropped (too close to start).
	// T = 0.96  → (1-T)*segLen = 4 < 5 → dropped (too close to end).
	points := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}
	plain := plainPolylinePath(points)

	for _, tc := range []struct {
		name string
		t    float64
	}{
		{"near start", 0.04},
		{"near end", 0.96},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := polylineWithHops(points, []model.Crossing{{SegmentIndex: 0, T: tc.t}})
			if strings.Contains(got, "A") {
				t.Errorf("crossing at T=%v should be dropped, but got arc: %q", tc.t, got)
			}
			if got != plain {
				t.Errorf("output should equal plain polyline\nwant: %q\ngot:  %q", plain, got)
			}
		})
	}
}

func TestPolylineWithHops_OutOfRangeValuesDroppedNoPanic(t *testing.T) {
	// Any out-of-range T or SegmentIndex must be silently dropped; the output
	// must equal the plain polyline and must not panic.
	points := []model.Point{{X: 0, Y: 0}, {X: 100, Y: 0}}
	plain := plainPolylinePath(points)

	cases := []struct {
		name     string
		crossing model.Crossing
	}{
		{"T negative", model.Crossing{SegmentIndex: 0, T: -0.1}},
		{"T > 1", model.Crossing{SegmentIndex: 0, T: 1.1}},
		{"SegmentIndex negative", model.Crossing{SegmentIndex: -1, T: 0.5}},
		// len(points)-1 == 1, so SegmentIndex 1 is out of bounds.
		{"SegmentIndex >= len-1", model.Crossing{SegmentIndex: 1, T: 0.5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := polylineWithHops(points, []model.Crossing{tc.crossing})
			if strings.Contains(got, "A") {
				t.Errorf("crossing %+v should be dropped, but got arc: %q", tc.crossing, got)
			}
			if got != plain {
				t.Errorf("output should equal plain polyline\nwant: %q\ngot:  %q", plain, got)
			}
		})
	}
}

func TestPolylineWithHops_DegenerateZeroLengthSegmentNoPanic(t *testing.T) {
	// A segment where both endpoints are identical (zero length).
	// The crossing must be silently dropped; the function must not panic.
	points := []model.Point{{X: 5, Y: 5}, {X: 5, Y: 5}}
	crossings := []model.Crossing{{SegmentIndex: 0, T: 0.5}}

	// Should not panic.
	got := polylineWithHops(points, crossings)

	if strings.Contains(got, "A") {
		t.Errorf("degenerate segment crossing should be dropped, got arc: %q", got)
	}
}

func TestPolylineWithHops_EmptyOrSinglePoint(t *testing.T) {
	// len(points) < 2 returns an empty string; no panic.
	if got := polylineWithHops(nil, nil); got != "" {
		t.Errorf("nil points: want \"\", got %q", got)
	}
	if got := polylineWithHops([]model.Point{{X: 1, Y: 2}}, nil); got != "" {
		t.Errorf("single point: want \"\", got %q", got)
	}
}

func TestPolylineWithHops_MultipleSegmentsHopOnSecond(t *testing.T) {
	// Three-point polyline: (0,0)→(0,50)→(100,50).
	// Crossing on segment 1 (the horizontal leg) at T=0.5 → x=50, y=50.
	points := []model.Point{
		{X: 0, Y: 0},
		{X: 0, Y: 50},
		{X: 100, Y: 50},
	}
	crossings := []model.Crossing{{SegmentIndex: 1, T: 0.5}}

	got := polylineWithHops(points, crossings)

	// First segment must be a plain line to (0,50).
	if !strings.Contains(got, "L0,50") {
		t.Errorf("expected L0,50 for first segment, got: %q", got)
	}
	// Second segment must contain an arc.
	if !strings.Contains(got, "A") {
		t.Errorf("expected arc on second segment, got: %q", got)
	}
}
