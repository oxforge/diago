package schema

import (
	"fmt"
	"sort"
	"strings"
)

// checkFlow returns the content findings for a flow spec, unsorted and
// before the ignore list; Check sorts and filters.
func checkFlow(spec *FlowSpec, opts CheckOptions) []Advisory {
	ids := deriveEdgeIDs(spec.Edges)
	var out []Advisory
	out = append(out, checkFlowNodes(spec, ids)...)
	out = append(out, checkFlowEdges(spec, opts)...)
	out = append(out, checkFlowGroups(spec)...)
	return out
}

// describeEdge names an edge the way messages do: a->b "label".
func describeEdge(e EdgeSpec) string {
	s := e.From + "->" + e.To
	if l := strings.TrimSpace(e.Label); l != "" {
		s += fmt.Sprintf(" %q", l)
	}
	return s
}

func checkFlowNodes(spec *FlowSpec, ids []string) []Advisory {
	var out []Advisory
	touched := make(map[string]bool)
	incoming := make(map[string]bool)
	for _, e := range spec.Edges {
		touched[e.From], touched[e.To] = true, true
		// A flat edge implies no order (schema flow.go, "flat"), so it
		// cannot give its target an "entry": isolated-node still counts it
		// above (connectivity, not order), but no-entry below must not.
		if e.From != e.To && !e.Flat {
			incoming[e.To] = true
		}
	}
	shapes := make(map[string]bool)
	for i, n := range spec.Nodes {
		field := fmt.Sprintf("nodes[%d]", i)
		out = append(out, labelAdvisories(field, fmt.Sprintf("node %q label", n.ID), "the node", n.Label)...)
		if len(spec.Nodes) >= 2 && !touched[n.ID] {
			out = append(out, Advisory{Rule: RuleIsolatedNode, Field: field,
				Message: fmt.Sprintf("node %q is connected to nothing; connect it or remove it", n.ID)})
		}
		shape := n.Shape
		if shape == "" {
			shape = "rect"
		}
		shapes[shape] = true
		if shape == "diamond" {
			outgoing := 0
			var unlabeled []string
			for j, e := range spec.Edges {
				// A flat edge is not a branch (it implies no order), so it
				// neither counts toward outgoing nor is listed unlabeled.
				if e.From != n.ID || e.Flat {
					continue
				}
				outgoing++
				if strings.TrimSpace(e.Label) == "" {
					unlabeled = append(unlabeled, ids[j])
				}
			}
			if outgoing >= 2 && len(unlabeled) > 0 {
				out = append(out, Advisory{Rule: RuleUnlabeledBranch, Field: field,
					Message: fmt.Sprintf("decision %q has unlabeled outgoing edges: %s; label every branch so each path's condition is clear",
						n.ID, strings.Join(unlabeled, ", "))})
			}
		}
	}
	if len(spec.Nodes) > MaxNodes {
		out = append(out, Advisory{Rule: RuleTooLarge,
			Message: fmt.Sprintf("%d nodes is a lot for one diagram (advice threshold: %d); consider decomposing into linked sub-diagrams",
				len(spec.Nodes), MaxNodes)})
	}
	if len(spec.Nodes) >= 2 && len(spec.Edges) >= 1 {
		all := true
		for _, n := range spec.Nodes {
			if !incoming[n.ID] {
				all = false
				break
			}
		}
		if all {
			out = append(out, Advisory{Rule: RuleNoEntry,
				Message: "flow has no clear entry: every node has an incoming edge; add a start node or break a cycle so readers know where to begin"})
		}
	}
	if len(shapes) > MaxShapes {
		names := make([]string, 0, len(shapes))
		for s := range shapes {
			names = append(names, s)
		}
		sort.Strings(names)
		out = append(out, Advisory{Rule: RuleShapeSoup,
			Message: fmt.Sprintf("%d distinct shapes (%s); more than %d dilutes meaning, reserve each shape for one consistent role",
				len(names), strings.Join(names, ", "), MaxShapes)})
	}
	return out
}

func checkFlowEdges(spec *FlowSpec, opts CheckOptions) []Advisory {
	var out []Advisory
	seen := make(map[[3]string]int)
	for i, e := range spec.Edges {
		field := fmt.Sprintf("edges[%d]", i)
		label := strings.TrimSpace(e.Label)
		key := [3]string{e.From, e.To, label}
		if j, dup := seen[key]; dup {
			out = append(out, Advisory{Rule: RuleDuplicateEdge, Field: field,
				Message: fmt.Sprintf("edge %s repeats edges[%d]; drop one or give them different labels", describeEdge(e), j)})
		} else {
			seen[key] = i
		}
		if isVague(label) {
			out = append(out, Advisory{Rule: RuleVagueEdgeLabel, Field: field,
				Message: fmt.Sprintf("edge %s->%s label %q says little; prefer a specific verb phrase like \"validates order\" or \"publishes event\"",
					e.From, e.To, label)})
		}
		if opts.Anchored && label != "" && (e.ID == nil || *e.ID == "") {
			out = append(out, Advisory{Rule: RuleEdgeWithoutID, Field: field,
				Message: fmt.Sprintf("edge %s has no id; anchoring and diffs key on ids, so give it one that survives edits", describeEdge(e))})
		}
	}
	return out
}

func checkFlowGroups(spec *FlowSpec) []Advisory {
	var out []Advisory
	for i, g := range spec.Groups {
		field := fmt.Sprintf("groups[%d]", i)
		out = append(out, labelAdvisories(field, fmt.Sprintf("group %q label", g.ID), "the group", g.Label)...)
		if len(g.Contains) == 0 {
			out = append(out, Advisory{Rule: RuleEmptyGroup, Field: field,
				Message: fmt.Sprintf("group %q contains nothing and renders as an empty box", g.ID)})
		}
	}
	return out
}
