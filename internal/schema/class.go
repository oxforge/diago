package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/oxforge/diago/internal/model"
)

// ClassSpec is the JSON-tagged input struct for class diagram specs.
type ClassSpec struct {
	Type         string            `json:"type"`
	Title        string            `json:"title,omitempty" jsonschema:"description=Diagram title\\, rendered as a band above the diagram."`
	Theme        string            `json:"theme,omitempty" jsonschema:"enum=default,enum=dark,enum=midnight,enum=sketch,description=Theme the render uses unless the -theme flag overrides it. Defaults to the default theme."`
	Direction    string            `json:"direction,omitempty" jsonschema:"enum=DOWN,enum=UP,enum=RIGHT,enum=LEFT,enum=AUTO,description=Layout direction. Defaults to DOWN."`
	Classes      []ClassNodeSpec   `json:"classes"`
	Relations    []RelationSpec    `json:"relations,omitempty"`
	Packages     []GroupSpec       `json:"packages,omitempty" jsonschema:"description=Packages group classes like flow groups: contains holds class ids or package ids (max 3 levels deep)."`
	Legend       bool              `json:"legend,omitempty" jsonschema:"description=Render a legend row per relation kind present in the diagram."`
	LegendLabels map[string]string `json:"legend_labels,omitempty" jsonschema:"description=Legend row label per relation kind; an empty value hides that row."`
	Ignore       []string          `json:"ignore,omitempty" jsonschema:"description=Advisory rules to silence for this spec (see diago check). Each entry must be a rule name."`
}

// ClassNodeSpec is the JSON representation of a class.
type ClassNodeSpec struct {
	ID         string       `json:"id"`
	Label      string       `json:"label,omitempty" jsonschema:"description=Class name. Defaults to the id."`
	Stereotype string       `json:"stereotype,omitempty" jsonschema:"description=Rendered as «stereotype» above the name; abstract also italicises the name."`
	TypeParams []string     `json:"type_params,omitempty" jsonschema:"description=Generic parameters. Rendered after the class name as a comma-separated list inside angle brackets."`
	Color      string       `json:"color,omitempty"`
	Attributes []MemberSpec `json:"attributes,omitempty"`
	Methods    []MemberSpec `json:"methods,omitempty"`
}

// MemberSpec is one attribute or method.
type MemberSpec struct {
	Visibility string `json:"visibility,omitempty" jsonschema:"enum=+,enum=-,enum=#,enum=~"`
	Text       string `json:"text"`
	Static     bool   `json:"static,omitempty"`
	Abstract   bool   `json:"abstract,omitempty"`
}

// RelationSpec is the JSON representation of a relation between classes.
type RelationSpec struct {
	ID       *string `json:"id,omitempty" jsonschema:"description=Optional stable identifier. Keep ids stable across edits because diffs and layout anchoring key on them. Defaults to from->to#n over the endpoints as written."`
	From     string  `json:"from"`
	To       string  `json:"to"`
	Kind     string  `json:"kind,omitempty" jsonschema:"enum=association,enum=inheritance,enum=realization,enum=dependency,enum=aggregation,enum=composition,description=Relation kind. Defaults to association. For inheritance and realization from is the subtype and to is the supertype or interface."`
	Label    string  `json:"label,omitempty"`
	FromCard string  `json:"from_card,omitempty" jsonschema:"description=Cardinality at the from end. Examples: 1 or 0..*."`
	ToCard   string  `json:"to_card,omitempty" jsonschema:"description=Cardinality at the to end."`
	Directed *bool   `json:"directed,omitempty" jsonschema:"description=Arrowhead at the to end. Default true for dependency and false for association; not allowed on adorned kinds."`
	Color    string  `json:"color,omitempty"`
}

// relationKinds lists the relation kinds in spec order; the canonical
// strings of the model.Relation enum. Shared by the legend-label check
// below and, from Task 3, checkClass.
var relationKinds = []string{"association", "inheritance", "realization", "dependency", "aggregation", "composition"}

var relationKindSet = func() map[string]bool {
	m := make(map[string]bool, len(relationKinds))
	for _, k := range relationKinds {
		m[k] = true
	}
	return m
}()

var visibilities = map[string]bool{"+": true, "-": true, "#": true, "~": true}

// ParseClass parses and validates a JSON class diagram spec into a flow
// graph whose nodes carry Members and whose edges carry a Relation. Every
// error is collected into ValidationErrors.
func ParseClass(data []byte) (*model.Graph, error) {
	var spec ClassSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse class: %w", err)
	}
	var errs ValidationErrors

	if spec.Type != "class" {
		errs = append(errs, ValidationError{Field: "type", Message: fmt.Sprintf("must be %q, got %q", "class", spec.Type)})
	}

	direction := model.Down
	if spec.Direction != "" {
		d, err := model.ParseDirection(spec.Direction)
		if err != nil {
			errs = append(errs, ValidationError{Field: "direction",
				Message: fmt.Sprintf("invalid direction %q; must be one of: DOWN, UP, RIGHT, LEFT, AUTO", spec.Direction)})
		} else {
			direction = d
		}
	}

	nodes, classIDs, nodeErrs := validateClasses(spec.Classes)
	errs = append(errs, nodeErrs...)

	edges, edgeErrs := validateRelations(spec.Relations, classIDs)
	errs = append(errs, edgeErrs...)

	groups, groupErrs := validateGroupsPrefixed(spec.Packages, classIDs, "packages")
	errs = append(errs, groupErrs...)

	var legend *model.Legend
	if spec.Legend || len(spec.LegendLabels) > 0 {
		keys := make([]string, 0, len(spec.LegendLabels))
		for k := range spec.LegendLabels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !relationKindSet[k] {
				errs = append(errs, ValidationError{Field: "legend_labels." + k, Message: fmt.Sprintf("unknown relation kind %q", k)})
			}
		}
	}
	if spec.Legend {
		legend = &model.Legend{Labels: spec.LegendLabels}
	}

	errs = append(errs, validateIgnore(spec.Ignore)...)

	if len(errs) > 0 {
		return nil, errs
	}
	return &model.Graph{
		Title:     strings.TrimSpace(spec.Title),
		Direction: direction,
		Nodes:     nodes,
		Edges:     edges,
		Groups:    groups,
		Legend:    legend,
	}, nil
}

func validateClasses(specs []ClassNodeSpec) ([]model.Node, map[string]bool, ValidationErrors) {
	var errs ValidationErrors
	if len(specs) == 0 {
		errs = append(errs, ValidationError{Field: "classes", Message: "must have at least one class"})
	}
	ids := make(map[string]bool, len(specs))
	nodes := make([]model.Node, 0, len(specs))
	for i, c := range specs {
		field := fmt.Sprintf("classes[%d]", i)
		switch {
		case c.ID == "":
			errs = append(errs, ValidationError{Field: field + ".id", Message: "must not be empty"})
		case ids[c.ID]:
			errs = append(errs, ValidationError{Field: field + ".id", Message: fmt.Sprintf("duplicate class id %q", c.ID)})
		default:
			ids[c.ID] = true
		}
		for j, tp := range c.TypeParams {
			if tp == "" {
				errs = append(errs, ValidationError{Field: fmt.Sprintf("%s.type_params[%d]", field, j), Message: "must not be empty"})
			}
		}
		if err := validateColor(field+".color", c.Color); err != nil {
			errs = append(errs, *err)
		}
		attrs, aErrs := validateMembers(c.Attributes, field+".attributes")
		methods, mErrs := validateMembers(c.Methods, field+".methods")
		errs = append(errs, aErrs...)
		errs = append(errs, mErrs...)
		label := c.Label
		if label == "" {
			label = c.ID
		}
		nodes = append(nodes, model.Node{
			ID: c.ID, Label: label, Shape: model.ShapeRect, Color: c.Color,
			Members: &model.Members{Stereotype: c.Stereotype, TypeParams: c.TypeParams, Attributes: attrs, Methods: methods},
		})
	}
	return nodes, ids, errs
}

func validateMembers(specs []MemberSpec, field string) ([]model.Member, ValidationErrors) {
	var errs ValidationErrors
	out := make([]model.Member, 0, len(specs))
	for j, m := range specs {
		f := fmt.Sprintf("%s[%d]", field, j)
		if m.Text == "" {
			errs = append(errs, ValidationError{Field: f + ".text", Message: "must not be empty"})
		}
		if m.Visibility != "" && !visibilities[m.Visibility] {
			errs = append(errs, ValidationError{Field: f + ".visibility", Message: "must be one of +, -, #, ~"})
		}
		out = append(out, model.Member{Visibility: m.Visibility, Text: m.Text, Static: m.Static, Abstract: m.Abstract})
	}
	return out, errs
}

func validateRelations(specs []RelationSpec, classIDs map[string]bool) ([]model.Edge, ValidationErrors) {
	explicit := make([]*string, len(specs))
	from := make([]string, len(specs))
	to := make([]string, len(specs))
	for i, r := range specs {
		explicit[i], from[i], to[i] = r.ID, r.From, r.To
	}
	ids, errs := validatePairIDs(explicit, from, to, "relations", "relation")

	edges := make([]model.Edge, 0, len(specs))
	for i, r := range specs {
		field := fmt.Sprintf("relations[%d]", i)
		for _, end := range []struct{ name, id string }{{".from", r.From}, {".to", r.To}} {
			switch {
			case end.id == "":
				errs = append(errs, ValidationError{Field: field + end.name, Message: "must not be empty"})
			case !classIDs[end.id]:
				errs = append(errs, ValidationError{Field: field + end.name, Message: fmt.Sprintf("references unknown class id %q", end.id)})
			}
		}
		kindName := r.Kind
		if kindName == "" {
			kindName = "association"
		}
		kind, kErr := model.ParseRelation(kindName)
		if kErr != nil || kind == model.RelationNone {
			errs = append(errs, ValidationError{Field: field + ".kind", Message: fmt.Sprintf("unknown relation kind %q", r.Kind)})
			kind = model.RelationAssociation
		}
		if r.Directed != nil && kind.Adorned() {
			errs = append(errs, ValidationError{Field: field + ".directed",
				Message: fmt.Sprintf("not allowed for kind %q: the adornment replaces the arrowhead", kind.String())})
		}
		if r.From != "" && r.From == r.To && kind.Swapped() {
			errs = append(errs, ValidationError{Field: field, Message: "a class cannot inherit from or realize itself"})
		}
		if err := validateColor(field+".color", r.Color); err != nil {
			errs = append(errs, *err)
		}

		directed := false
		switch {
		case kind.Adorned():
			directed = false
		case r.Directed != nil:
			directed = *r.Directed
		case kind == model.RelationDependency:
			directed = true
		}
		style := model.EdgeSolid
		if kind.Dashed() {
			style = model.EdgeDashed
		}
		e := model.Edge{
			ID: ids[i], From: r.From, To: r.To, Label: r.Label, Style: style, Direction: model.EdgeForward, Color: r.Color,
			Relation: kind, FromCard: r.FromCard, ToCard: r.ToCard, Directed: directed,
		}
		if kind.Swapped() {
			e.From, e.To = e.To, e.From
			e.FromCard, e.ToCard = e.ToCard, e.FromCard
		}
		edges = append(edges, e)
	}
	return edges, errs
}
