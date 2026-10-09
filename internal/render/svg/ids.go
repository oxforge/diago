package svg

import "fmt"

// idSet hands out the element ids of one SVG document, each unique: the
// first claim of an id keeps it, a later one takes -2, -3 and so on.
type idSet map[string]bool

// claim returns id, or id numbered past every claim made of it before.
func (s idSet) claim(id string) string {
	out := id
	for n := 2; s[out]; n++ {
		out = fmt.Sprintf("%s-%d", id, n)
	}
	s[out] = true
	return out
}
