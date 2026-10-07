package sequence

import (
	"testing"

	"github.com/oxforge/diago/internal/schema"
)

var testFonts = LayoutFonts{Family: "Inter", HeaderSize: 14, LabelSize: 12, FragLabelSize: 11}

// A message label between adjacent actors must fit between their lifelines
// with a cell of slack each side: the text profile's actor spacing.
func TestTextProfile_LabelFitsBetweenLifelines(t *testing.T) {
	spec := `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
	          "interactions":[{"from":"a","to":"b","label":"a rather long message label here"}]}`
	d, err := schema.ParseSequence([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	ps := LayoutWithProfile(*d, TextProfile())
	gap := ps.Actors[1].LineX - ps.Actors[0].LineX
	need := 8 * float64(len([]rune("a rather long message label here"))+4)
	if gap < need {
		t.Fatalf("lifeline gap %g < needed %g", gap, need)
	}
}

// Nested fragments that end on the same message close on successive rows, and
// the next message sits below the outermost closing line (text profile).
func TestTextProfile_NestedFragmentsClearTheNextMessage(t *testing.T) {
	spec := `{"type":"sequence","actors":[{"id":"a","label":"A"},{"id":"b","label":"B"}],
	  "interactions":[{"from":"a","to":"b","label":"m1"},{"from":"b","to":"a","label":"m2"},
	                  {"from":"a","to":"b","label":"m3"},{"from":"b","to":"a","label":"m4"},
	                  {"from":"a","to":"b","label":"m5-after"}],
	  "fragments":[{"type":"alt","over":["a","b"],"sections":[{"label":"outer","start":0,"end":3}]},
	               {"type":"opt","over":["a","b"],"sections":[{"label":"inner","start":1,"end":3}]}]}`
	d, err := schema.ParseSequence([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	p := TextProfile()
	ps := LayoutWithProfile(*d, p)
	NormalizeSequenceWithProfile(ps, p)
	nextRow := int(ps.Interactions[4].Y / 16)
	for i, f := range ps.Fragments {
		bottomRow := int((f.Y + f.Height) / 16) // the row the closing line is drawn on
		if bottomRow >= nextRow {
			t.Errorf("fragment %d closes on row %d, not above the next message on row %d", i, bottomRow, nextRow)
		}
	}
	if b0, b1 := int((ps.Fragments[0].Y+ps.Fragments[0].Height)/16), int((ps.Fragments[1].Y+ps.Fragments[1].Height)/16); b0 == b1 {
		t.Errorf("outer and inner frames close on the same row %d", b0)
	}
}
