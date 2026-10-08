package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/oxforge/diago/internal/diff"
	seqlayout "github.com/oxforge/diago/internal/layout/sequence"
	"github.com/oxforge/diago/internal/model"
	pngrender "github.com/oxforge/diago/internal/render/png"
	svgrender "github.com/oxforge/diago/internal/render/svg"
	textrender "github.com/oxforge/diago/internal/render/text"
	"github.com/oxforge/diago/internal/schema"
	"github.com/oxforge/diago/internal/theme"
)

// DiffOptions parameterizes RenderDiff.
type DiffOptions struct {
	Format     string // "svg" (default) | "png" | "text" ("txt" alias)
	Theme      string // theme name override (empty = use the after spec's theme field)
	PNG        pngrender.Options
	Caption    string             // names the two sides, the change list's first line ("" for none); see DiffCaption
	Advisories *[]schema.Advisory // optional sink: both specs checked as anchored, fields prefixed before./after., plus text-label-dropped
}

// DiffCaption is the first line of a diff's change list: the names of its
// two sides, old → new.
func DiffCaption(oldName, newName string) string { return oldName + " → " + newName }

// withFooter appends a text diff's change list to its art after one blank
// line: the caption, when there is one, then the change lines. With neither
// the art is returned as it is.
func withFooter(art, caption string, legend []string) string {
	lines := legend
	if caption != "" {
		lines = append([]string{caption}, legend...)
	}
	if len(lines) == 0 {
		return art
	}
	return strings.TrimRight(art, "\n") + "\n\n" + strings.Join(lines, "\n") + "\n"
}

// prefixFields prepends prefix to every field path of a ValidationErrors.
func prefixFields(err error, prefix string) error {
	var ve schema.ValidationErrors
	if !errors.As(err, &ve) {
		return err
	}
	out := make(schema.ValidationErrors, len(ve))
	for i, e := range ve {
		out[i] = schema.ValidationError{Field: prefix + e.Field, Message: e.Message}
	}
	return out
}

func validationError(field, msg string) error {
	return schema.ValidationErrors{{Field: field, Message: msg}}
}

// diffInputs is the parsed pair; exactly one of the flow/sequence pairs is
// set. A class pair rides on the flow fields (a class spec parses to a
// model.Graph).
type diffInputs struct {
	kind                  string
	flowBefore, flowAfter *model.Graph
	seqBefore, seqAfter   *model.SequenceDiagram
}

// parseDiffInputs probes both types, parses both specs with before./after.
// field prefixes, and rejects mixed types on after.type.
func parseDiffInputs(before, after []byte) (diffInputs, error) {
	var pb, pa specProbe
	if err := json.Unmarshal(before, &pb); err != nil {
		return diffInputs{}, validationError("before", "invalid JSON: "+err.Error())
	}
	if err := json.Unmarshal(after, &pa); err != nil {
		return diffInputs{}, validationError("after", "invalid JSON: "+err.Error())
	}
	if pb.Type != pa.Type {
		return diffInputs{}, validationError("after.type", fmt.Sprintf("must match before (%q), got %q", pb.Type, pa.Type))
	}
	switch pb.Type {
	case "flow":
		bg, err := schema.ParseFlow(before)
		if err != nil {
			return diffInputs{}, prefixFields(err, "before.")
		}
		ag, err := schema.ParseFlow(after)
		if err != nil {
			return diffInputs{}, prefixFields(err, "after.")
		}
		return diffInputs{kind: "flow", flowBefore: bg, flowAfter: ag}, nil
	case "class":
		bg, err := schema.ParseClass(before)
		if err != nil {
			return diffInputs{}, prefixFields(err, "before.")
		}
		ag, err := schema.ParseClass(after)
		if err != nil {
			return diffInputs{}, prefixFields(err, "after.")
		}
		return diffInputs{kind: "class", flowBefore: bg, flowAfter: ag}, nil
	case "sequence":
		bd, err := schema.ParseSequence(before)
		if err != nil {
			return diffInputs{}, prefixFields(err, "before.")
		}
		ad, err := schema.ParseSequence(after)
		if err != nil {
			return diffInputs{}, prefixFields(err, "after.")
		}
		return diffInputs{kind: "sequence", seqBefore: bd, seqAfter: ad}, nil
	default:
		return diffInputs{}, validationError("before.type", fmt.Sprintf("unknown diagram type %q", pb.Type))
	}
}

// layoutFlowDiff lays before out fresh, takes its carrier, and lays the
// union out anchored on it (text: under the text profile, on the stamped
// copy so boxes fit their prefixes). Removed elements are real participants.
// It also returns the before layout's own carrier (basePG.LayoutHints), the
// anchor the union was laid out on: flatRankedDiffFields checks a union flat
// edge's id against its Ranked to tell a kept fallback (S13, keepRanked)
// from a fresh one.
func layoutFlowDiff(ctx context.Context, u diff.UnionGraph, before *model.Graph, th theme.Theme, text bool) (*model.PositionedGraph, *model.LayoutHints, error) {
	union := u.Graph
	if text {
		union = diff.StampLabels(u)
	}

	if text {
		b := *before
		basePG, err := layoutWith(ctx, b, th, true, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("diff: layout before: %w", err)
		}
		pg, err := layoutWith(ctx, union, th, true, basePG.LayoutHints)
		if err != nil {
			return nil, nil, fmt.Errorf("diff: layout union: %w", err)
		}
		reportContract(ctx, pg, union, text)
		return pg, basePG.LayoutHints, nil
	}

	basePG, err := graphLayout(ctx, *before, th, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("diff: layout before: %w", err)
	}
	pg, err := graphLayout(ctx, union, th, basePG.LayoutHints)
	if err != nil {
		return nil, nil, fmt.Errorf("diff: layout union: %w", err)
	}
	reportContract(ctx, pg, union, text)
	return pg, basePG.LayoutHints, nil
}

// statusClass maps union statuses to CSS classes for the SVG renderer. A
// member line of a class node without a member union follows its box
// (ruling 2); with one it carries its own per-member status, or no class at
// all when that member is unchanged.
func statusClass(u diff.UnionGraph) func(kind, id string) string {
	return func(kind, id string) string {
		var s diff.Status
		switch kind {
		case "node":
			s = u.Nodes[id]
		case "edge":
			s = u.Edges[id]
		case "group":
			s = u.Groups[id]
		case "member":
			node, rest, _ := strings.Cut(id, "/")
			mu, ok := u.Members[node]
			if !ok {
				s = u.Nodes[node] // ruling 2: no union, follow the box
				break
			}
			comp, idx, _ := strings.Cut(rest, "/")
			i, err := strconv.Atoi(idx)
			if err != nil {
				return ""
			}
			statuses := mu.AttrStatus
			if comp == "methods" {
				statuses = mu.MethodStatus
			}
			if i < 0 || i >= len(statuses) || statuses[i] == diff.Same {
				return ""
			}
			return string(statuses[i])
		}
		if s == "" || s == diff.Same {
			return svgrender.ClassDimmed
		}
		return string(s)
	}
}

// layoutSequenceDiff lays the union out with the activation mask: under the
// text profile on the stamped copy (so boxes and rows fit the prefixes), or
// under the theme's screen profile.
func layoutSequenceDiff(u diff.UnionSequence, th theme.Theme, text bool) *model.PositionedSequence {
	opts := seqlayout.LayoutOptions{InactiveInteractions: u.Removed}
	if text {
		p := seqlayout.TextProfile()
		ps := seqlayout.LayoutWithOptions(diff.StampSequenceLabels(u), p, opts)
		seqlayout.NormalizeSequenceWithProfile(ps, p)
		return ps
	}
	p := sequenceScreenProfile(th)
	ps := seqlayout.LayoutWithOptions(u.Diagram, p, opts)
	seqlayout.NormalizeSequenceWithProfile(ps, p)
	return ps
}

// sequenceStatusClass maps union statuses to CSS classes: same elements
// are dimmed, except a same section, which defers to its frame.
func sequenceStatusClass(u diff.UnionSequence) func(kind, id string) string {
	return func(kind, id string) string {
		var s diff.Status
		switch kind {
		case "actor", "lifeline":
			s = u.Actors[id]
		case "interaction":
			if i, err := strconv.Atoi(id); err == nil && i >= 0 && i < len(u.Interactions) {
				s = u.Interactions[i]
			}
		case "fragment":
			if i, err := strconv.Atoi(id); err == nil && i >= 0 && i < len(u.Fragments) {
				s = u.Fragments[i].Status
			}
		case "section":
			f, k, ok := strings.Cut(id, "/")
			fi, err1 := strconv.Atoi(f)
			ki, err2 := strconv.Atoi(k)
			if ok && err1 == nil && err2 == nil && fi >= 0 && fi < len(u.Fragments) && ki >= 0 && ki < len(u.Fragments[fi].Sections) {
				s = u.Fragments[fi].Sections[ki]
			}
			if s == "" || s == diff.Same {
				return ""
			}
			return string(s)
		}
		if s == "" || s == diff.Same {
			return svgrender.ClassDimmed
		}
		return string(s)
	}
}

// RenderDiff renders the union of two specs with per-element status. When
// opts.Advisories is set, both specs are checked as anchored and their
// findings are merged with fields prefixed before./after.
func RenderDiff(ctx context.Context, before, after []byte, opts DiffOptions) ([]byte, error) {
	data, dropped, flatRanked, err := renderDiff(ctx, before, after, opts)
	if err != nil {
		return nil, err
	}
	if opts.Advisories != nil {
		advs, err := diffAdvisories(before, after, dropped, flatRanked)
		if err != nil {
			return nil, err
		}
		*opts.Advisories = advs
	}
	return data, nil
}

// renderDiff renders the union of before and after with per-element
// status. Validation errors carry before./after. prefixes. flatRanked names
// the union edges the layout ranked (flatRankedDiffFields); nil for a
// sequence diff, which has no flat edges.
func renderDiff(ctx context.Context, before, after []byte, opts DiffOptions) ([]byte, []textrender.DroppedLabel, []flatRankedField, error) {
	format := opts.Format
	if format == "" {
		format = "svg"
	}
	if format == "txt" {
		format = "text"
	}
	switch format {
	case "svg", "png", "text":
	case "drawio", "excalidraw":
		return nil, nil, nil, validationError("format", "format not supported for diff")
	default:
		return nil, nil, nil, validationError("format", fmt.Sprintf("unsupported format %q: want svg, png, or text", opts.Format))
	}
	in, err := parseDiffInputs(before, after)
	if err != nil {
		return nil, nil, nil, err
	}
	// Text art takes no theme; the others take the after spec's (specTheme).
	var th theme.Theme
	if format != "text" {
		if th, err = specTheme(after, opts.Theme); err != nil {
			return nil, nil, nil, fmt.Errorf("diff: theme: %w", prefixFields(err, "after."))
		}
	}

	if in.kind == "sequence" {
		u := diff.BuildUnionSequence(in.seqBefore, in.seqAfter)
		if format == "text" {
			ps := layoutSequenceDiff(u, th, true)
			res, err := textrender.RenderSequence(ps, u.Diagram.Activations)
			if err != nil {
				return nil, nil, nil, fmt.Errorf("diff: render text: %w", err)
			}
			return []byte(withFooter(res.Art, opts.Caption, diff.SequenceLegend(u))), res.DroppedLabels, nil, nil
		}
		ps := layoutSequenceDiff(u, th, false)
		svg := svgrender.RenderSequenceWithOptions(ps, th, &svgrender.RenderOptions{
			Class:       sequenceStatusClass(u),
			Styles:      svgrender.SequenceDiffStyles(th),
			MarkerFills: statusFills(th),
			StatusPaint: true,
			Changes:     changeLines(diff.SequenceChanges(u), th),
			Caption:     opts.Caption,
		})

		if format == "png" {
			png, err := pngrender.Render(ctx, []byte(svg), opts.PNG)
			return png, nil, nil, err
		}
		return []byte(svg), nil, nil, nil
	}

	bg, ag := in.flowBefore, in.flowAfter
	u := diff.BuildUnion(bg, ag)

	if format == "text" {
		pg, basePrev, err := layoutFlowDiff(ctx, u, bg, th, true)
		if err != nil {
			return nil, nil, nil, err
		}
		flatRanked := flatRankedDiffFields(pg, bg, ag, basePrev)
		res, err := textrender.Render(pg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("diff: render text: %w", err)
		}
		return []byte(withFooter(res.Art, opts.Caption, diff.FlowLegend(u))), res.DroppedLabels, flatRanked, nil
	}

	pg, basePrev, err := layoutFlowDiff(ctx, u, bg, th, false)
	if err != nil {
		return nil, nil, nil, err
	}
	flatRanked := flatRankedDiffFields(pg, bg, ag, basePrev)
	pg.LayoutHints = nil // a diff picture is not a render to anchor on
	styles := svgrender.DiffStyles(th)
	if in.kind == "class" {
		styles = svgrender.ClassDiffStyles(th)
	}
	svg := svgrender.RenderWithOptions(pg, th, &svgrender.RenderOptions{
		Class:       statusClass(u),
		Styles:      styles,
		MarkerFills: statusFills(th),
		StatusPaint: true,
		Changes:     changeLines(diff.FlowChanges(u), th),
		Caption:     opts.Caption,
	})

	if format == "png" {
		png, err := pngrender.Render(ctx, []byte(svg), opts.PNG)
		return png, nil, flatRanked, err
	}
	return []byte(svg), nil, flatRanked, nil
}

// statusFills maps each status class to its color in the theme's diff
// palette: the markers a status edge or message takes.
func statusFills(th theme.Theme) map[string]string {
	return map[string]string{
		string(diff.Added): th.Diff.Added, string(diff.Changed): th.Diff.Changed, string(diff.Removed): th.Diff.Removed,
	}
}

// changeLines maps the diff's change list to the SVG renderer's rows,
// colored by the theme's diff palette; a removed line's sample is dashed.
func changeLines(changes []diff.Change, th theme.Theme) []svgrender.ChangeLine {
	fills := statusFills(th)
	out := make([]svgrender.ChangeLine, len(changes))
	for i, c := range changes {
		out[i] = svgrender.ChangeLine{Color: fills[string(c.Status)], Dashed: c.Status == diff.Removed, Wire: c.Wire, Text: c.Text}
	}
	return out
}
