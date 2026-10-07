package sequence

import (
	"sort"

	"github.com/oxforge/diago/internal/model"
)

// computeActivations builds activation boxes from the interaction list.
// An actor becomes active (receives a box) when it appears as "to" of any interaction.
// The box closes (its bottom is set) when that actor next appears as "from" of any interaction.
// Self-messages do not create activation boxes.
// inactive is index-parallel to interactions: a masked interaction (inactive[i] true)
// keeps its row but neither opens nor closes an activation bar; a nil or shorter
// slice means every interaction is active.
func computeActivations(
	interactions []model.Interaction,
	interactionY []float64,
	actorIndex map[string]int,
	posActors []model.PositionedActor,
	p Profile,
	inactive []bool,
) []model.PositionedActivation {
	// p.ActivationWidth is the logical width of an activation box in pixels
	// used for layout positioning. The theme's activation stroke width is
	// separate and controls only the visual rendering; the two are
	// intentionally decoupled so that theme changes don't affect layout
	// geometry.
	var result []model.PositionedActivation
	// activeBoxes maps actorID → the current open activation box top Y.
	activeBoxes := make(map[string]float64)

	for i, it := range interactions {
		y := interactionY[i]

		if i < len(inactive) && inactive[i] {
			continue // masked: keeps its row, touches no bar
		}

		if it.From == it.To {
			// Self-messages don't affect activations.
			continue
		}

		// Async (fire-and-forget) messages don't close the sender's activation.
		// Only synchronous messages (solid/dashed) close the "from" actor's box.
		if it.Style != model.InteractionAsync {
			// Close the "from" actor's activation box if it has one.
			if topY, ok := activeBoxes[it.From]; ok {
				ai := actorIndex[it.From]
				lx := posActors[ai].LineX
				result = append(result, model.PositionedActivation{
					ActorID: it.From,
					X:       lx - p.ActivationWidth/2,
					Y:       topY,
					Width:   p.ActivationWidth,
					Height:  y - topY,
				})
				delete(activeBoxes, it.From)
			}
		}

		// Open an activation box on the "to" actor.
		// Async messages are fire-and-forget — they don't activate the target.
		// Only open if not already active (avoid nesting for simplicity).
		if it.Style != model.InteractionAsync {
			if _, already := activeBoxes[it.To]; !already {
				activeBoxes[it.To] = y
			}
		}
	}

	// Close any remaining open activation boxes with a default height.
	// Sort actor IDs for deterministic output.
	type unclosed struct {
		id   string
		topY float64
	}
	var remaining []unclosed
	for id, topY := range activeBoxes {
		remaining = append(remaining, unclosed{id, topY})
	}
	sort.Slice(remaining, func(i, j int) bool {
		return actorIndex[remaining[i].id] < actorIndex[remaining[j].id]
	})
	for _, r := range remaining {
		ai := actorIndex[r.id]
		lx := posActors[ai].LineX
		result = append(result, model.PositionedActivation{
			ActorID: r.id,
			X:       lx - p.ActivationWidth/2,
			Y:       r.topY,
			Width:   p.ActivationWidth,
			Height:  p.InteractionSpacingY,
		})
	}

	return result
}
