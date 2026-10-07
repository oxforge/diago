// Package layout holds what the layout engines share: the text profile's
// rune measure. The engines live in its subpackages (layered, sequence),
// beside the output-contract checker, the metrics and the corpus.
package layout

// Cell dimensions of the text renderer's character grid: the text
// profile's 8 × 16 px cell (C0).
const (
	textCellW = 8.0
	textCellH = 16.0
)

// RuneMeasure measures text in cells, one per rune and one row per line, as
// the text profile does (S1), in px: 8 per rune, 16 per line, independent
// of font family and size. The sequence engine's text profile uses it.
func RuneMeasure(text string, _ float64, _ string) (float64, float64) {
	widest := 0
	lines := 1
	cur := 0
	for _, r := range text {
		if r == '\n' {
			lines++
			if cur > widest {
				widest = cur
			}
			cur = 0
			continue
		}
		cur++
	}
	if cur > widest {
		widest = cur
	}
	return float64(widest) * textCellW, float64(lines) * textCellH
}
