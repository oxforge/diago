package excalidraw

import (
	"fmt"

	"github.com/oxforge/diago/internal/model"
)

// Element ids. Every kind of element has its own prefix, so no user id
// meets an exporter-made one or another kind's.

func shapeID(node string) string         { return "n:" + node }
func nodeTextID(node string) string      { return "nt:" + node }
func groupBackingID(group string) string { return "gb:" + group }
func groupLabelID(group string) string   { return "gt:" + group }

// arrowID is the id of the edge at index i: its own id when it has one,
// else its index.
func arrowID(i int, e model.PositionedEdge) string {
	if e.ID != "" {
		return "e:" + e.ID
	}
	return fmt.Sprintf("e#%d", i)
}

// arrowLabelID is the id of the edge's label text.
func arrowLabelID(i int, e model.PositionedEdge) string { return "l" + arrowID(i, e) }
