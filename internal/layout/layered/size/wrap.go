package size

import (
	"math"
	"strings"
)

// wrap splits a label into lines (S1). A label with an explicit "\n"
// breaks only on it: each resulting line stays whole however wide it
// measures, its runs of spaces collapsed. A label without "\n" wraps
// greedily where the line would grow wider than limit; a word wider than
// limit stays whole on its own line. A blank line stays as an empty line.
func wrap(label string, limit float64, width func(string) float64) []string {
	if strings.Contains(label, "\n") {
		segments := strings.Split(label, "\n")
		lines := make([]string, len(segments))
		for i, s := range segments {
			lines[i] = strings.Join(strings.Fields(s), " ")
		}
		return lines
	}
	words := strings.Fields(label)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	line := words[0]
	for _, w := range words[1:] {
		if candidate := line + " " + w; width(candidate) <= limit {
			line = candidate
		} else {
			lines = append(lines, line)
			line = w
		}
	}
	lines = append(lines, line)
	return lines
}

// balanced splits words, in order, into count lines whose widest line is
// narrowest (S1, Compact labels); of the splits whose widest line is
// within noise of that, it takes the one whose earlier lines hold the most
// words. count runs from 1 to len(words). A run of words wider than bound
// (within noise) is never a line: bound is no narrower than the narrowest
// widest line on count lines, the wrap's widest line for a count at or
// above the wrap's own, or +Inf. A run is never narrower than a run it
// contains, so each word's runs are measured up to the first past bound,
// once each, and a dynamic program over them finds the narrowest widest
// line of every suffix on every count, so no split is enumerated.
func balanced(words []string, count int, bound float64, width func(string) float64) []string {
	n := len(words)
	// run[a][b-a-1] is the width of words[a:b] on one line, for every b up
	// to the first whose run is wider than bound; line is +Inf past it
	run := make([][]float64, n)
	for a := range run {
		for b := a + 1; b <= n; b++ {
			w := width(strings.Join(words[a:b], " "))
			run[a] = append(run[a], w)
			if w > bound+noise {
				break
			}
		}
	}
	line := func(a, b int) float64 {
		if b-a-1 < len(run[a]) {
			return run[a][b-a-1]
		}
		return math.Inf(1)
	}
	// narrowest[j][a] is the narrowest widest line of words[a:] on j lines,
	// +Inf where fewer words than lines remain
	narrowest := make([][]float64, count+1)
	for j := range narrowest {
		narrowest[j] = make([]float64, n+1)
		for a := range narrowest[j] {
			narrowest[j][a] = math.Inf(1)
		}
	}
	narrowest[0][n] = 0
	for j := 1; j <= count; j++ {
		for a := n - j; a >= 0; a-- {
			for b := a + 1; b <= min(n-(j-1), a+len(run[a])); b++ {
				narrowest[j][a] = min(narrowest[j][a], max(line(a, b), narrowest[j-1][b]))
			}
		}
	}
	limit := narrowest[count][0] + noise
	lines := make([]string, 0, count)
	for a, j := 0, count; j > 0; j-- {
		b := min(n-(j-1), a+len(run[a]))
		for b > a+1 && (line(a, b) > limit || narrowest[j-1][b] > limit) {
			b--
		}
		lines = append(lines, strings.Join(words[a:b], " "))
		a = b
	}
	return lines
}
