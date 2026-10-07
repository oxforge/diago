package text

import (
	"strings"
	"unicode/utf8"
)

// artWidth is the widest line of art in runes, trailing blanks excluded.
func artWidth(art string) int {
	width := 0
	for _, l := range strings.Split(art, "\n") {
		if n := utf8.RuneCountInString(strings.TrimRight(l, " ")); n > width {
			width = n
		}
	}
	return width
}

// withTitle prepends the title, centered over the art's width, and one
// blank line. An empty title returns art unchanged. The
// second return value is the number of lines prepended (0 or 2), which the
// caller must subtract from its TopRow so grid-row math stays correct.
func withTitle(art, title string) (string, int) {
	if title == "" {
		return art, 0
	}
	pad := max(0, (artWidth(art)-utf8.RuneCountInString(title))/2)
	return strings.Repeat(" ", pad) + title + "\n\n" + art, 2
}
