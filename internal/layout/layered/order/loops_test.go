package order

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/oxforge/diago/internal/layout/layered/lgraph"
)

// flowLoop is the flow-loop example's graph: i++ -> check closes the
// loop check -> Process -> i++ and is reversed, so its chain runs from
// check through a dummy beside Process and End down to i++. Declared
// before End, Process takes the seed's first slot in its layer, and the
// dummy the last.
var flowLoop = []string{
	"start:circle->init", "init->check:diamond", "check->process", "process->increment",
	"increment->check", "check->end:circle",
}

// TestMinimize_KeepsALoopTogether pins S6's Loops kept together on
// flow-loop: the seed Process, End, dummy has no crossing, and i++'s two
// upper neighbors, Process and the dummy, enclose End. The loop
// transposition swaps End and Process, which crosses nothing, side-aware
// included (check's end on the dummy faces right, past both of check's
// other children), and End leaves the loop. Declared before Process, End
// is outside it already and nothing moves.
func TestMinimize_KeepsALoopTogether(t *testing.T) {
	g := graph(t, flowLoop...)
	require.Equal(t, []string{"process", "end", "increment->check#0@3"}, ids(g)[3], "the seed")
	require.Equal(t, 1, newSweeper(g).enclosed(), "i++'s upper neighbors enclose End")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, []string{"end", "process", "increment->check#0@3"}, ids(g)[3])
	assert.Zero(t, g.Crossings())
	assert.Zero(t, newSweeper(g).enclosed())

	g = graph(t, "start:circle->init", "init->check:diamond", "check->end:circle", "check->process",
		"process->increment", "increment->check")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, []string{"end", "process", "increment->check#0@3"}, ids(g)[3])
}

// TestMinimize_PicksTheOrderingThatEnclosesNone pins S6's Loops kept
// together on neat/incident-response's loop: Escalate -> Investigate
// closes Investigate -> Mitigate -> Resolved? -> Escalate, and Mitigate's
// other child, Status page, is no part of it. Resolved?, Status page,
// return dummy, and Status page, Resolved?, return dummy, both cross
// nothing, and differ only in Status page, which the first puts between
// Escalate's two upper neighbors. From either, S6 takes the second.
func TestMinimize_PicksTheOrderingThatEnclosesNone(t *testing.T) {
	specs := []string{"inv->mit", "mit->fix:diamond", "mit->st", "fix->close", "fix->esc", "esc->inv"}
	inside := [][]string{{"inv"}, {"mit", "esc->inv#0@1"}, {"fix", "st", "esc->inv#0@2"}, {"close", "esc"}}
	outside := [][]string{{"inv"}, {"mit", "esc->inv#0@1"}, {"st", "fix", "esc->inv#0@2"}, {"close", "esc"}}
	for _, seed := range [][][]string{inside, outside} {
		g := graph(t, specs...)
		setLayers(t, g, seed)
		require.Zero(t, g.Crossings(), "%v", seed)
		g.Layers = Minimize(context.Background(), g, false)
		assert.Equal(t, outside, ids(g), "from %v", seed)
	}
	g := graph(t, specs...)
	setLayers(t, g, inside)
	assert.Equal(t, 1, newSweeper(g).enclosed(), "Status page inside the loop")
	setLayers(t, g, outside)
	assert.Zero(t, newSweeper(g).enclosed(), "Status page outside it")
}

// TestMinimize_AnAnchoredLevelKeepsItsLoop pins S6's yield to S13: seeded
// from a previous layout, flow-loop keeps the order that puts End inside
// its loop, since there the previous order wins the tie; fresh, the loop
// transposition takes End out. Seeded from that fresh result, as a level
// anchored on it is, S6 keeps it (C18.1).
func TestMinimize_AnAnchoredLevelKeepsItsLoop(t *testing.T) {
	g := graph(t, flowLoop...)
	seed := ids(g)
	assert.Equal(t, seed, idsOf(g, Minimize(context.Background(), g, true)), "anchored")
	fresh := Minimize(context.Background(), g, false)
	assert.Equal(t, []string{"end", "process", "increment->check#0@3"}, idsOf(g, fresh)[3], "fresh")
	g.Layers = fresh
	assert.Equal(t, fresh, Minimize(context.Background(), g, true), "anchored on the fresh result")
}

// idsOf is ids of g with its layers ordered as layers.
func idsOf(g *lgraph.Graph, layers [][]int) [][]string {
	work := *g
	work.Layers = layers
	return ids(&work)
}

// TestSided_CountsACounterFlowEndAtItsSide pins S6's side-aware
// crossings: under check, the order Process, dummy, End has no crossing
// counted at the centers, but check's end on the return dummy faces right
// (one of check's other children lies on each side of the dummy, and a tie
// goes right), where S8 attaches it, and check -> End leaves below it:
// one crossing. With the dummy on the right no end crosses.
func TestSided_CountsACounterFlowEndAtItsSide(t *testing.T) {
	g := graph(t, flowLoop...)
	setLayers(t, g, [][]string{{"start"}, {"init"}, {"check"}, {"process", "increment->check#0@3", "end"}, {"increment"}})
	require.Zero(t, g.Crossings())
	assert.Equal(t, 1, newSweeper(g).sided(3))
	setLayers(t, g, [][]string{{"start"}, {"init"}, {"check"}, {"end", "process", "increment->check#0@3"}, {"increment"}})
	assert.Zero(t, newSweeper(g).sided(3))
}

// TestSided_ASideHoldsOneEnd pins two clauses of S6's side-aware
// crossings on a two-cycle t <-> u beside a longer loop w -> t, whose
// chain passes u and v. With v, u, dummy under t, both of t's counter-flow
// ends face right; u -> t, the earlier edge, holds that side, and w -> t's
// end counts at t's center, where S8 leaves it on t's face: u -> t's wire
// leaves t's right side for u, w -> t's leaves t's face for the dummy,
// right of u, and they cross, though no crossing counts at the centers.
// With u, v, dummy, u -> t's end faces left and w -> t's right, and none
// crosses; u's end, the lower end of a one-hop edge, takes t's side, the
// left, and so never crosses t -> u, its own two-cycle's other edge.
func TestSided_ASideHoldsOneEnd(t *testing.T) {
	g := graph(t, "s->t", "t->u", "u->t", "t->v", "v->w", "w->t")
	setLayers(t, g, [][]string{{"s"}, {"t"}, {"v", "u", "w->t#0@2"}, {"w"}})
	require.Zero(t, g.Crossings())
	assert.Equal(t, 1, newSweeper(g).sided(2))
	setLayers(t, g, [][]string{{"s"}, {"t"}, {"u", "v", "w->t#0@2"}, {"w"}})
	require.Zero(t, g.Crossings())
	assert.Zero(t, newSweeper(g).sided(2))
}

// TestMinimize_KeepsMLTrainingsSides pins why S6 counts crossings
// side-aware in the loop transposition, on neat/ml-training: Metrics
// pass?'s upper neighbors, Evaluate and the return dummy of its "no" edge
// back to Feature eng, enclose Hyperparam tune, which forms a two-cycle
// with Train model. Swapping Hyperparam tune and Evaluate would free it,
// with no crossing counted at the centers; but then both of Train model's
// counter-flow ends face right, and the router attaches the two-cycle's
// there and leaves Schedule retrain's return wire on the face below it,
// across the two-cycle's wire (S8). The side-aware count sees that
// crossing, and Hyperparam tune stays.
func TestMinimize_KeepsMLTrainingsSides(t *testing.T) {
	g := graph(t, "raw->split", "split->feat", "feat->train", "train->tune", "tune->train", "train->eval",
		"eval->good:diamond", "good->reg", "good->feat", "reg->deploy", "deploy->monitor", "monitor->retrain",
		"retrain->train", "eval->report")
	g.Layers = Minimize(context.Background(), g, false)
	assert.Equal(t, []string{"good->feat#0@4", "tune", "eval", "retrain->train#0@4"}, ids(g)[4])
	assert.Zero(t, g.Crossings())
	for l := range g.Layers {
		assert.Zero(t, newSweeper(g).sided(l), "layer %d", l)
	}
}
