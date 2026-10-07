package size

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/font"
	"github.com/oxforge/diago/internal/layoutdbg"
	"github.com/oxforge/diago/internal/model"
)

var screen = Options{
	Measure:  font.MeasureText,
	NodeFont: Font{Family: "Inter", Size: 14}, MemberFont: Font{Family: "Inter", Size: 12},
	WrapWidth: 200, PadX: 20, PadY: 12, MinW: 80, MinH: 40,
	CylinderCap: 16, ParallelogramPad: 20,
	ClassPadX: model.ClassPadX, ClassPadY: model.ClassPadY, ClassMinW: model.ClassMinWidth,
}

var cells = Options{Cells: true, WrapWidth: 24, PadX: 2, PadY: 1, MinW: 7, MinH: 3, DiamondPad: 4}

func textWidth(s string, size float64) float64 {
	w, _ := font.MeasureText(s, size, "Inter")
	return w
}

func lineHeight(size float64) float64 {
	_, h := font.MeasureText("X", size, "Inter")
	return h
}

func one(o Options, n model.Node) Size {
	return Measure(context.Background(), []model.Node{n}, o)[0]
}

func TestLabel(t *testing.T) {
	o := screen
	o.LabelFont = Font{Family: "Pangolin", Size: 11}
	w, h := Label("1..*", o)
	fw, fh := font.MeasureText("1..*", 11, "Pangolin")
	assert.Equal(t, [2]float64{fw, fh}, [2]float64{w, h}, "one line in the label font")

	w, h = Label("wait für", cells)
	assert.Equal(t, [2]float64{8, 1}, [2]float64{w, h}, "one cell per rune, one row")
}

func TestTitle(t *testing.T) {
	o := screen
	o.GroupFont = Font{Family: "Pangolin", Size: 13}
	w, h := Title("Data plane", o)
	fw, fh := font.MeasureText("Data plane", 13, "Pangolin")
	assert.Equal(t, [2]float64{fw, fh}, [2]float64{w, h}, "one line in the group font")

	w, h = Title("Data plane", cells)
	assert.Equal(t, [2]float64{10, 1}, [2]float64{w, h}, "one cell per rune, one row")
}

func TestMeasure_ScreenBox(t *testing.T) {
	got := one(screen, model.Node{ID: "n", Label: "Start"})
	assert.Nil(t, got.Lines)
	assert.Equal(t, max(40, lineHeight(14)+24), got.H)
	assert.Equal(t, max(80, textWidth("Start", 14)+40, got.H), got.W)

	empty := one(screen, model.Node{ID: "e"})
	assert.Equal(t, Size{W: 80, H: max(40, lineHeight(14)+24)}, empty)
}

func TestMeasure_WrapsOnlyPastTheWrapWidth(t *testing.T) {
	short := one(screen, model.Node{ID: "s", Label: "Validate the request"})
	require.Less(t, textWidth("Validate the request", 14), 200.0)
	assert.Nil(t, short.Lines, "a label under 200 px stays one line")

	label := "Reconcile the payment ledger against the settlement report every night"
	long := one(screen, model.Node{ID: "l", Label: label})
	require.GreaterOrEqual(t, len(long.Lines), 2)
	assert.Equal(t, label, strings.Join(long.Lines, " "))
	widest := 0.0
	for _, l := range long.Lines {
		assert.LessOrEqual(t, textWidth(l, 14), 200.0, l)
		widest = max(widest, textWidth(l, 14))
	}
	assert.Equal(t, widest+40, long.W)
	assert.Equal(t, lineHeight(14)*float64(len(long.Lines))+24, long.H)
}

func TestMeasure_AnUnbreakableWordGrowsTheNode(t *testing.T) {
	label := "Supercalifragilisticexpialidocious-extraordinarily-long-token"
	got := one(screen, model.Node{ID: "w", Label: label})
	assert.Nil(t, got.Lines)
	assert.Equal(t, textWidth(label, 14)+40, got.W)
	assert.Greater(t, got.W, 240.0)
}

func TestMeasure_ExplicitNewlines(t *testing.T) {
	got := one(screen, model.Node{ID: "n", Label: "Line one\n  Line two"})
	assert.Equal(t, []string{"Line one", "Line two"}, got.Lines)
	assert.Equal(t, 2*lineHeight(14)+24, got.H)
	assert.InDelta(t, 2.5*got.H, got.W, 1e-9, "a short two-line box widens to 2.5:1")

	d := one(screen, model.Node{ID: "d", Label: "Line one\nLine two", Shape: model.ShapeDiamond})
	assert.InDelta(t, math.Sqrt(3)*d.H, d.W, 1e-9, "fixed-ratio shapes skip the 2.5:1 widening")
}

func TestMeasure_ExplicitLineWiderThanWrapWidthStaysWhole(t *testing.T) {
	long := "Reconcile the payment ledger against the settlement report every night"
	require.Greater(t, textWidth(long, 14), 200.0)
	got := one(screen, model.Node{ID: "n", Label: long + "\nok"})
	assert.Equal(t, []string{long, "ok"}, got.Lines, "an explicit line stays whole however wide the screen box")

	longCell := "one two three four five six seven eight"
	require.Greater(t, runes(longCell), 24.0)
	c := one(cells, model.Node{ID: "n", Label: longCell + "\nok"})
	assert.Equal(t, []string{longCell, "ok"}, c.Lines, "an explicit line stays whole however wide the text box")
}

func TestMeasure_CollapsesRunsOfSpacesButKeepsTheLineListWhenItChanges(t *testing.T) {
	collapsed := one(screen, model.Node{ID: "n", Label: "a    b"})
	assert.Equal(t, []string{"a b"}, collapsed.Lines, "the collapsed line differs from the label, so the list stays")

	plain := one(screen, model.Node{ID: "n", Label: "a b"})
	assert.Nil(t, plain.Lines, "already a single line identical to the label as written")
}

func TestMeasure_NeverNarrowerThanTall(t *testing.T) {
	// no two of these words fit on one 200 px line, so the label stacks
	// eight lines whose widest, padded, is narrower than the box is tall
	label := "Authorization Authentication Reconciliation Transformation Serialization Normalization Deduplication Tokenization"
	got := one(screen, model.Node{ID: "n", Label: label})
	require.Len(t, got.Lines, 8)
	widest := 0.0
	for _, l := range got.Lines {
		widest = max(widest, textWidth(l, 14))
	}
	assert.Equal(t, 8*lineHeight(14)+24, got.H)
	require.Less(t, widest+40, got.H)
	assert.Equal(t, got.H, got.W, "the box widens to its height")

	cy := one(screen, model.Node{ID: "c", Label: label, Shape: model.ShapeCylinder})
	assert.Equal(t, got.W, cy.W, "the rule comes before the shape adds its cap")
	assert.Equal(t, got.H+16, cy.H)
}

func TestMeasure_Shapes(t *testing.T) {
	const label = "Approve release?"
	box := one(screen, model.Node{ID: "r", Label: label})
	at := func(s model.Shape) Size { return one(screen, model.Node{ID: "n", Label: label, Shape: s}) }

	// the fixed-ratio shapes take the label onto two lines (Compact labels),
	// so their shape rules start from the padded two-line text box
	lines := []string{"Approve", "release?"}
	wrapped := Size{W: max(textWidth("Approve", 14), textWidth("release?", 14)) + 40, H: 2*lineHeight(14) + 24}

	d := at(model.ShapeDiamond)
	assert.Equal(t, lines, d.Lines)
	assert.InDelta(t, math.Sqrt(3)*d.H, d.W, 1e-9)
	assert.GreaterOrEqual(t, d.W, 1.5*wrapped.W-1e-9)
	assert.GreaterOrEqual(t, d.H, 1.5*wrapped.H-1e-9)

	c := at(model.ShapeCircle)
	assert.Equal(t, lines, c.Lines)
	assert.Equal(t, max(wrapped.W, wrapped.H), c.W)
	assert.Equal(t, c.W, c.H)

	h := at(model.ShapeHexagon)
	assert.Equal(t, lines, h.Lines)
	assert.InDelta(t, h.W*math.Sqrt(3)/2, h.H, 1e-9)
	assert.GreaterOrEqual(t, h.W, wrapped.W)

	cy := at(model.ShapeCylinder)
	assert.Equal(t, Size{W: box.W, H: box.H + 16}, cy)

	p := at(model.ShapeParallelogram)
	assert.Equal(t, box.H, p.H)
	assert.InDelta(t, box.W+20+0.3*box.H, p.W, 1e-9)

	assert.Equal(t, box, at(model.ShapeRounded))
}

// TestMeasure_CompactLabels pins S1's Compact labels: a diamond's,
// circle's or hexagon's label without "\n" takes the balanced wrap of the
// line count whose box is narrowest, which the shape rule makes its larger
// extent. The sizes are golayout's for the same labels.
func TestMeasure_CompactLabels(t *testing.T) {
	h2 := 2*lineHeight(14) + 24
	tests := []struct {
		name    string
		label   string
		shape   model.Shape
		lines   []string
		w, h    float64 // the box, to 0.05 px
		oneLine float64 // the one-line box's width, which the box undercuts
	}{
		{"a diamond at its two-line floor", "Metrics healthy?", model.ShapeDiamond,
			[]string{"Metrics", "healthy?"}, 179.8, 103.8, 1.5 * (textWidth("Metrics healthy?", 14) + 40)},
		{"a hexagon", "Redis Cache", model.ShapeHexagon,
			[]string{"Redis", "Cache"}, 96.7, 83.8, textWidth("Redis Cache", 14) + 40},
		{"a circle", "git push", model.ShapeCircle,
			[]string{"git", "push"}, 83.4, 83.4, textWidth("git push", 14) + 40},
		{"a diamond on the narrower of two splits", "i < array.length?", model.ShapeDiamond,
			[]string{"i <", "array.length?"}, 1.5 * (textWidth("array.length?", 14) + 40), 1.5 * (textWidth("array.length?", 14) + 40) / math.Sqrt(3),
			1.5 * (textWidth("i < array.length?", 14) + 40)},
		{"a wrapped label takes a line more", "Is the payment settled?", model.ShapeHexagon,
			[]string{"Is the", "payment", "settled?"}, textWidth("payment", 14) + 40, (textWidth("payment", 14) + 40) * math.Sqrt(3) / 2,
			textWidth("Is the payment", 14) + 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := one(screen, model.Node{ID: "n", Label: tt.label, Shape: tt.shape})
			assert.Equal(t, tt.lines, got.Lines)
			assert.InDelta(t, tt.w, got.W, 0.05)
			assert.InDelta(t, tt.h, got.H, 0.05)
			assert.Less(t, got.W, tt.oneLine)
		})
	}
	d := one(screen, model.Node{ID: "d", Label: "Metrics healthy?", Shape: model.ShapeDiamond})
	assert.InDelta(t, 1.5*math.Sqrt(3)*h2, d.W, 1e-9, "the diamond's two-line height floor sets its width")
}

// TestMeasure_CompactLabelsInTheSketchFont pins S1's Compact labels under
// the sketch theme's node font, Pangolin 14, whose words run narrower than
// Inter's and whose line runs taller: the same rule picks the lines and
// the box from Pangolin's metrics. "All green?" wraps to two lines in
// Inter but keeps one in Pangolin, where a diamond's two-line height floor
// comes out wider than its one-line box.
func TestMeasure_CompactLabelsInTheSketchFont(t *testing.T) {
	sketch := screen
	sketch.NodeFont = Font{Family: "Pangolin", Size: 14}
	width := func(s string) float64 {
		w, _ := font.MeasureText(s, 14, "Pangolin")
		return w
	}
	_, lineH := font.MeasureText("X", 14, "Pangolin")
	h2 := 2*lineH + 24
	tests := []struct {
		name    string
		label   string
		shape   model.Shape
		lines   []string
		w, h    float64
		oneLine float64 // the one-line box's width
	}{
		{"a diamond at its two-line floor", "Metrics healthy?", model.ShapeDiamond,
			[]string{"Metrics", "healthy?"}, 1.5 * math.Sqrt(3) * h2, 1.5 * h2, 1.5 * (width("Metrics healthy?") + 40)},
		{"a diamond whose floor keeps one line", "All green?", model.ShapeDiamond,
			nil, 1.5 * (width("All green?") + 40), 1.5 * (width("All green?") + 40) / math.Sqrt(3), 1.5 * (width("All green?") + 40)},
		{"a circle", "Order shipped", model.ShapeCircle,
			[]string{"Order", "shipped"}, width("shipped") + 40, width("shipped") + 40, width("Order shipped") + 40},
		{"a hexagon", "Redis Cache", model.ShapeHexagon,
			[]string{"Redis", "Cache"}, width("Cache") + 40, (width("Cache") + 40) * math.Sqrt(3) / 2, width("Redis Cache") + 40},
		{"a wrapped label takes a line more", "Is the payment settled?", model.ShapeHexagon,
			[]string{"Is the", "payment", "settled?"}, width("payment") + 40, (width("payment") + 40) * math.Sqrt(3) / 2,
			width("Is the payment") + 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := one(sketch, model.Node{ID: "n", Label: tt.label, Shape: tt.shape})
			assert.Equal(t, tt.lines, got.Lines)
			assert.InDelta(t, tt.w, got.W, 1e-9)
			assert.InDelta(t, tt.h, got.H, 1e-9)
			assert.LessOrEqual(t, got.W, tt.oneLine)
		})
	}
	inter := one(screen, model.Node{ID: "n", Label: "All green?", Shape: model.ShapeDiamond})
	assert.Equal(t, []string{"All", "green?"}, inter.Lines, "Inter wraps what Pangolin keeps")
	assert.Greater(t, 1.5*math.Sqrt(3)*h2, 1.5*(width("All green?")+40), "Pangolin's two-line floor is wider than its one line")
}

func TestMeasure_CompactLabelsBreakTiesTowardFewerAndFullerLines(t *testing.T) {
	// "a b" fits a circle's and a hexagon's MinW on one line and on two, so
	// both counts are 80 wide: the tie keeps one line
	for _, s := range []model.Shape{model.ShapeCircle, model.ShapeHexagon} {
		split := one(screen, model.Node{ID: "n", Label: "a\nb", Shape: s})
		got := one(screen, model.Node{ID: "n", Label: "a b", Shape: s})
		require.Equal(t, split.W, got.W, "%s: one line and two tie", s)
		assert.Nil(t, got.Lines, "%s: the tie keeps fewer lines", s)
	}

	// "go go go" on two lines splits either way with the same widest line:
	// the first line takes the most words
	got := one(screen, model.Node{ID: "n", Label: "go go go", Shape: model.ShapeCircle})
	assert.Equal(t, []string{"go go", "go"}, got.Lines)
	assert.Equal(t, textWidth("go go", 14)+40, got.W)
}

func TestMeasure_CompactLabelsTryUpToThreeMoreLines(t *testing.T) {
	const label = "one two three four five six seven eight nine ten eleven twelve"
	// the wrap keeps three lines of four words; a circle's box is narrowest
	// on six, three more
	got := one(screen, model.Node{ID: "n", Label: label, Shape: model.ShapeCircle})
	assert.Equal(t, []string{"one two", "three four", "five six seven", "eight nine ten", "eleven", "twelve"}, got.Lines)

	// with the whole label on one line, the circle tries four lines at most,
	// although five would be narrower still
	wide := screen
	wide.WrapWidth = 1000
	four := []string{"one two three", "four five six", "seven eight nine", "ten eleven twelve"}
	five := []string{"one two", "three four", "five six seven", "eight nine ten", "eleven twelve"}
	require.Less(t,
		one(wide, model.Node{ID: "n", Label: strings.Join(five, "\n"), Shape: model.ShapeCircle}).W,
		one(wide, model.Node{ID: "n", Label: strings.Join(four, "\n"), Shape: model.ShapeCircle}).W)
	got = one(wide, model.Node{ID: "n", Label: label, Shape: model.ShapeCircle})
	assert.Equal(t, four, got.Lines)

	// and never fewer lines than the wrap: under a 40 px wrap width "go go go"
	// stays on three lines, though a circle is narrower on two
	narrow := screen
	narrow.WrapWidth = 40
	got = one(narrow, model.Node{ID: "n", Label: "go go go", Shape: model.ShapeCircle})
	assert.Equal(t, []string{"go", "go", "go"}, got.Lines)
}

func TestMeasure_CompactLabelsKeepTheOtherLabels(t *testing.T) {
	const label = "Metrics healthy?"
	w := textWidth(label, 14)

	// explicit lines stay as written, though "Metrics" / "healthy? ok" is narrower
	d := one(screen, model.Node{ID: "n", Label: label + "\nok", Shape: model.ShapeDiamond})
	assert.Equal(t, []string{label, "ok"}, d.Lines)
	assert.InDelta(t, 1.5*(w+40), d.W, 1e-9)

	// one word never splits, however wide
	for _, word := range []string{"Approved?", "Supercalifragilisticexpialidocious"} {
		for _, s := range []model.Shape{model.ShapeDiamond, model.ShapeCircle, model.ShapeHexagon} {
			assert.Nil(t, one(screen, model.Node{ID: "n", Label: word, Shape: s}).Lines, "%s %s", s, word)
		}
	}

	// every other shape keeps its one line
	for _, s := range []model.Shape{model.ShapeRect, model.ShapeRounded, model.ShapeCylinder, model.ShapeParallelogram} {
		got := one(screen, model.Node{ID: "n", Label: label, Shape: s})
		assert.Nil(t, got.Lines, "%s", s)
		assert.GreaterOrEqual(t, got.W, w+40, "%s", s)
	}
	assert.Equal(t, []model.PositionedLine{{Text: label}}, one(screen, model.Node{ID: "n", Label: label, Members: &model.Members{}}).Members.Header, "a record box")

	// and the text box has no shape rule: one line of 16 runes
	for _, tt := range []struct {
		shape model.Shape
		w     float64
	}{{model.ShapeDiamond, 24}, {model.ShapeCircle, 20}, {model.ShapeHexagon, 20}} {
		assert.Equal(t, Size{W: tt.w, H: 3}, one(cells, model.Node{ID: "n", Label: label, Shape: tt.shape}), "%s", tt.shape)
	}
}

func TestMeasure_CompactLabelsRecordTheCountsTried(t *testing.T) {
	var buf bytes.Buffer
	ctx := layoutdbg.NewContext(context.Background(), slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	Measure(ctx, []model.Node{
		{ID: "gate", Label: "Metrics healthy?", Shape: model.ShapeDiamond},
		{ID: "box", Label: "Metrics healthy?"},
		{ID: "one", Label: "Approved?", Shape: model.ShapeDiamond},
	}, screen)
	var got []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec))
		if rec["decision"] == "label_compacted" {
			got = append(got, rec)
		}
	}
	require.Len(t, got, 1, "only the diamond's label of two words compacts")
	assert.Equal(t, "gate", got[0]["node"])
	assert.Equal(t, "S1", got[0]["spec_ref"])
	assert.Equal(t, 1.0, got[0]["wrap_lines"])
	assert.Equal(t, 2.0, got[0]["lines"])
	widths, ok := got[0]["widths"].([]any)
	require.True(t, ok)
	require.Len(t, widths, 2, "one line and two")
	assert.InDelta(t, 1.5*(textWidth("Metrics healthy?", 14)+40), widths[0], 1e-9)
	assert.InDelta(t, 179.8, widths[1], 0.05)
}

// TestMeasure_CompactLabelsMeasureBoundedRuns pins the bound on S1's
// Compact labels: a diamond's label of 3,000 distinct words sizes by
// measuring each word's runs only up to the first wider than the wrap's
// widest line, a few per word. Measuring every run of words, 4.5 million
// here, took 36 s and 2.8 GB at 1,000 words. The measurer counts the
// texts S1 measures, each once, and stops the test at 10 per word.
func TestMeasure_CompactLabelsMeasureBoundedRuns(t *testing.T) {
	words := make([]string, 3000)
	for i := range words {
		words[i] = fmt.Sprintf("word%d", i)
	}
	o := screen
	measured := 0
	o.Measure = func(text string, sizePt float64, family string) (float64, float64) {
		measured++
		require.LessOrEqual(t, measured, 10*len(words), "S1 measures more than a few runs per word")
		return font.MeasureText(text, sizePt, family)
	}
	got := one(o, model.Node{ID: "n", Label: strings.Join(words, " "), Shape: model.ShapeDiamond})
	assert.Equal(t, words, strings.Fields(strings.Join(got.Lines, " ")), "every word, in order")
	t.Logf("%d texts measured for %d words on %d lines", measured, len(words), len(got.Lines))
}

func TestBalanced(t *testing.T) {
	tests := []struct {
		name  string
		words []string
		count int
		want  []string
	}{
		{"one line", []string{"a", "bb", "c"}, 1, []string{"a bb c"}},
		{"a word a line", []string{"a", "bb", "c"}, 3, []string{"a", "bb", "c"}},
		{"the narrowest widest line", []string{"aaaa", "b", "c", "dddd"}, 2, []string{"aaaa b", "c dddd"}},
		{"a tie fills the first line", []string{"aa", "aa", "aa"}, 2, []string{"aa aa", "aa"}},
		{"a tie fills the earlier lines", []string{"a", "a", "a", "aaaaa", "a"}, 3, []string{"a a a", "aaaaa", "a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, balanced(tt.words, tt.count, math.Inf(1), runes))
		})
	}
}

// TestBalanced_MatchesAnExhaustiveSearch checks balanced against every split
// of random labels, in runes: its widest line is the narrowest there is, and
// of the splits that tie it, it is the one whose earlier lines hold the most
// words. Bounded by the widest line of the label's wrap under a random wrap
// width, as S1 bounds it, it splits every count from the wrap's own on the
// same way.
func TestBalanced_MatchesAnExhaustiveSearch(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		words := make([]string, 1+rng.IntN(8))
		for i := range words {
			words[i] = strings.Repeat("x", 1+rng.IntN(6))
		}
		wrapped := wrap(strings.Join(words, " "), float64(1+rng.IntN(20)), runes)
		for count := 1; count <= len(words); count++ {
			var want []string
			for _, s := range splits(words, count) {
				if want == nil || widest(s) < widest(want) || (widest(s) == widest(want) && fuller(s, want)) {
					want = s
				}
			}
			require.Equal(t, want, balanced(words, count, math.Inf(1), runes), "%v on %d lines", words, count)
			if count >= len(wrapped) {
				require.Equal(t, want, balanced(words, count, widest(wrapped), runes), "%v on %d lines, bounded by %v", words, count, wrapped)
			}
		}
	}
}

// splits lists every split of words into count lines, in order.
func splits(words []string, count int) [][]string {
	if count == 1 {
		return [][]string{{strings.Join(words, " ")}}
	}
	var out [][]string
	for b := 1; b <= len(words)-(count-1); b++ {
		for _, rest := range splits(words[b:], count-1) {
			out = append(out, append([]string{strings.Join(words[:b], " ")}, rest...))
		}
	}
	return out
}

func widest(lines []string) float64 {
	w := 0.0
	for _, l := range lines {
		w = max(w, runes(l))
	}
	return w
}

// fuller reports whether a's first line that differs from b's holds more
// words.
func fuller(a, b []string) bool {
	for i := range a {
		if na, nb := len(strings.Fields(a[i])), len(strings.Fields(b[i])); na != nb {
			return na > nb
		}
	}
	return false
}

func TestMeasure_Cells(t *testing.T) {
	tests := []struct {
		name string
		node model.Node
		want Size
	}{
		{"a short label", model.Node{ID: "n", Label: "abc"}, Size{W: 8, H: 3}},
		{"a diamond adds its decoration", model.Node{ID: "n", Label: "abc", Shape: model.ShapeDiamond}, Size{W: 12, H: 3}},
		{"wraps past 24 cells", model.Node{ID: "n", Label: "one two three four five six seven eight"},
			Size{W: 28, H: 4, Lines: []string{"one two three four five", "six seven eight"}}},
		{"explicit lines and the minimum", model.Node{ID: "n", Label: "a\nb"}, Size{W: 8, H: 4, Lines: []string{"a", "b"}}},
		{"shape rules do not apply", model.Node{ID: "n", Label: "abcdefgh", Shape: model.ShapeHexagon}, Size{W: 12, H: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, one(cells, tt.node))
		})
	}
}

func order() model.Node {
	return model.Node{ID: "order", Label: "Order", Members: &model.Members{
		Attributes: []model.Member{{Visibility: "-", Text: "id: string"}},
		Methods:    []model.Member{{Visibility: "+", Text: "total(): Money"}},
	}}
}

func TestMeasure_RecordBoxScreen(t *testing.T) {
	got := one(screen, order())
	require.NotNil(t, got.Members)
	assert.Equal(t, []model.PositionedLine{{Text: "Order"}}, got.Members.Header)
	assert.Equal(t, []model.PositionedLine{{Text: "- id: string"}}, got.Members.Attributes)
	assert.Equal(t, []model.PositionedLine{{Text: "+ total(): Money"}}, got.Members.Methods)
	headerH, memberH := lineHeight(14), lineHeight(12)
	assert.Equal(t, headerH, got.Members.HeaderLineHeight)
	assert.Equal(t, memberH, got.Members.MemberLineHeight)
	widest := max(textWidth("Order", 14), textWidth("- id: string", 12), textWidth("+ total(): Money", 12))
	assert.Equal(t, max(model.ClassMinWidth, widest)+2*model.ClassPadX, got.W)
	assert.Equal(t, (headerH+2*model.ClassPadY)+2*(memberH+2*model.ClassPadY), got.H)
	assert.Nil(t, got.Lines)
}

func TestMeasure_RecordBoxCells(t *testing.T) {
	got := one(cells, order())
	assert.Equal(t, 20.0, got.W, "16 runes plus 2 padding cells a side")
	assert.Equal(t, 7.0, got.H, "2 borders, the name, 2 separators, 2 members")
	assert.Equal(t, 1.0, got.Members.HeaderLineHeight)
	assert.Equal(t, 1.0, got.Members.MemberLineHeight)
}

func TestMeasure_RecordBoxCellsFloorAndEvenWidth(t *testing.T) {
	// "A" padded is 5 cells: the MinW floor lifts it to 7, then the width
	// rounds up to 8; a 9-rune name padded to 13 rounds up to 14
	small := one(cells, model.Node{ID: "a", Label: "A", Members: &model.Members{}})
	assert.Equal(t, 8.0, small.W)
	assert.Equal(t, 3.0, small.H, "2 borders and the name")
	odd := one(cells, model.Node{ID: "b", Label: "Abcdefghi", Members: &model.Members{}})
	assert.Equal(t, 14.0, odd.W)
}

func TestMeasure_RecordBoxScreenTakesNoNodeMinimums(t *testing.T) {
	// a header-only record box is its one compartment tall, under MinH
	small := one(screen, model.Node{ID: "a", Label: "A", Members: &model.Members{}})
	require.Less(t, lineHeight(14)+2*model.ClassPadY, screen.MinH)
	assert.Equal(t, lineHeight(14)+2*model.ClassPadY, small.H)
	assert.Equal(t, model.ClassMinWidth+2*model.ClassPadX, small.W)

	// ClassMinW plus its padding equals the screen MinW (80), so only a
	// larger MinW shows that the width floor is not MinW either
	wide := screen
	wide.MinW = 120
	assert.Equal(t, model.ClassMinWidth+2*model.ClassPadX, one(wide, model.Node{ID: "a", Label: "A", Members: &model.Members{}}).W)

	// a tall record box is not widened to its height
	var attrs []model.Member
	for range 8 {
		attrs = append(attrs, model.Member{Text: "x: int"})
	}
	tall := one(screen, model.Node{ID: "t", Label: "T", Members: &model.Members{Attributes: attrs}})
	require.Greater(t, tall.H, model.ClassMinWidth+2*model.ClassPadX)
	assert.Equal(t, model.ClassMinWidth+2*model.ClassPadX, tall.W)
}

func TestMeasure_RecordLines(t *testing.T) {
	n := model.Node{ID: "r", Label: "Repo", Members: &model.Members{
		Stereotype: "abstract", TypeParams: []string{"K", "V"},
		Methods: []model.Member{
			{Visibility: "+", Text: "get(k: K): V", Abstract: true},
			{Text: "count(): int", Static: true, Mark: "+ "},
		},
	}}
	got := one(cells, n)
	assert.Equal(t, []model.PositionedLine{
		{Text: "«abstract»", Stereotype: true},
		{Text: "Repo<K,V>", Italic: true},
	}, got.Members.Header)
	assert.Equal(t, []model.PositionedLine{
		{Text: "+ get(k: K): V *", Italic: true},
		{Text: "+ count(): int $", Underline: true},
	}, got.Members.Methods)
	assert.Nil(t, got.Members.Attributes)

	screenLines := one(screen, n).Members.Methods
	assert.Equal(t, "+ get(k: K): V", screenLines[0].Text, "the suffixes are text-only")
}

// BenchmarkSize_ACompactLabelOf3000Words is the record of S1's Compact
// labels on a diamond whose label has 3,000 distinct words: about 0.2 s with the balanced wrap's dynamic
// program bounded by each word's measured runs, 55 s without that bound,
// which TestMeasure_CompactLabelsMeasureBoundedRuns does not notice, since
// the texts measured stay bounded either way.
func BenchmarkSize_ACompactLabelOf3000Words(b *testing.B) {
	words := make([]string, 3000)
	for i := range words {
		words[i] = fmt.Sprintf("word%d", i)
	}
	n := model.Node{ID: "n", Label: strings.Join(words, " "), Shape: model.ShapeDiamond}
	for b.Loop() {
		one(screen, n)
	}
}
