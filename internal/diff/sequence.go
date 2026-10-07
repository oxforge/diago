package diff

import "github.com/oxforge/diago/internal/model"

// AlignedStep is one row of the union timeline: which before and after
// interaction it comes from (-1 for an absent side) and its status.
type AlignedStep struct {
	Status    Status
	BeforeIdx int
	AfterIdx  int
}

// ActorStatus is one column of the union: an actor id and its status.
type ActorStatus struct {
	ID     string
	Status Status
}

// SeqAlignment is the result of AlignSequences: Steps in union order,
// Actors in column order.
type SeqAlignment struct {
	Steps  []AlignedStep
	Actors []ActorStatus
}

func strongKey(it model.Interaction) string {
	return it.From + "|" + it.To + "|" + it.Label + "|" + it.Style.String()
}

func weakKey(it model.Interaction) string { return it.From + "|" + it.To }

// alignActors takes after's order and splices each removed before actor
// after its nearest surviving left neighbour (consecutive removed actors
// keep before order; none puts it at the front).
func alignActors(before, after *model.SequenceDiagram) []ActorStatus {
	afterByID := make(map[string]model.Actor, len(after.Actors))
	for _, a := range after.Actors {
		afterByID[a.ID] = a
	}
	beforeByID := make(map[string]model.Actor, len(before.Actors))
	for _, a := range before.Actors {
		beforeByID[a.ID] = a
	}
	// removedAfter[""] holds the removed actors with no surviving left
	// neighbour; removedAfter[id] those spliced after survivor id.
	removedAfter := map[string][]string{}
	lastSurvivor := ""
	for _, a := range before.Actors {
		if _, ok := afterByID[a.ID]; ok {
			lastSurvivor = a.ID
			continue
		}
		removedAfter[lastSurvivor] = append(removedAfter[lastSurvivor], a.ID)
	}
	var out []ActorStatus
	for _, id := range removedAfter[""] {
		out = append(out, ActorStatus{id, Removed})
	}
	for _, a := range after.Actors {
		s := Added
		if o, ok := beforeByID[a.ID]; ok {
			s = Same
			if o.Label != a.Label || o.Color != a.Color {
				s = Changed
			}
		}
		out = append(out, ActorStatus{a.ID, s})
		for _, id := range removedAfter[a.ID] {
			out = append(out, ActorStatus{id, Removed})
		}
	}
	return out
}

// AlignSequences aligns two interaction lists by an LCS over the strong key
// from|to|label|style, then pairs leftovers inside each gap on from|to as
// changed. Ties prefer the earliest before index.
func AlignSequences(before, after *model.SequenceDiagram) SeqAlignment {
	b, a := before.Interactions, after.Interactions
	n, m := len(b), len(a)
	// dp[i][j] = LCS length of b[i:] and a[j:].
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if strongKey(b[i]) == strongKey(a[j]) {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	// Walk from the front: a strong match is taken greedily. Otherwise the
	// side that keeps the LCS advances; on a tie between advancing sides the
	// before side is consumed first, so the duplicate-key case matches the
	// earliest before index.
	type match struct{ bi, ai int }
	var matches []match
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case strongKey(b[i]) == strongKey(a[j]):
			matches = append(matches, match{i, j})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			i++
		default:
			j++
		}
	}
	matches = append(matches, match{n, m}) // sentinel closes the last gap

	var steps []AlignedStep
	bi, ai := 0, 0
	for _, mt := range matches {
		// Gap: b[bi:mt.bi] and a[ai:mt.ai].
		used := make([]bool, mt.ai-ai)
		for x := bi; x < mt.bi; x++ {
			paired := -1
			for y := ai; y < mt.ai; y++ {
				if !used[y-ai] && weakKey(b[x]) == weakKey(a[y]) {
					used[y-ai] = true
					paired = y
					break
				}
			}
			if paired >= 0 {
				steps = append(steps, AlignedStep{Changed, x, paired})
			} else {
				steps = append(steps, AlignedStep{Removed, x, -1})
			}
		}
		for y := ai; y < mt.ai; y++ {
			if !used[y-ai] {
				steps = append(steps, AlignedStep{Added, -1, y})
			}
		}
		if mt.bi < n {
			s := Same
			if b[mt.bi].Color != a[mt.ai].Color {
				s = Changed
			}
			steps = append(steps, AlignedStep{s, mt.bi, mt.ai})
		}
		bi, ai = mt.bi+1, mt.ai+1
	}
	return SeqAlignment{Steps: steps, Actors: alignActors(before, after)}
}
