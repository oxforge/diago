package drawio

import (
	"fmt"

	"github.com/oxforge/diago/internal/model"
)

// Cell ids. Every kind of cell has its own prefix, so no user id meets an
// exporter-made one or another kind's.
const (
	rootCell  = "root"
	layerCell = "layer"
)

func nodeCell(id string) string  { return "n:" + id }
func groupCell(id string) string { return "g:" + id }
func titleCell(id string) string { return "t:" + id }

// edgeCell is the cell id of the edge at index i: its own id when it has
// one, else its index.
func edgeCell(i int, e model.PositionedEdge) string {
	if e.ID != "" {
		return "e:" + e.ID
	}
	return fmt.Sprintf("e#%d", i)
}
