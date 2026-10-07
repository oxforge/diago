# Diago

Diago is a command-line tool that renders diagrams from structured JSON specs.
Describe *what's connected to what* in typed JSON; diago handles layout and
rendering and writes SVG, PNG, Unicode text art, draw.io XML or Excalidraw JSON.

It is built for AI agents (Claude Code, Codex, Cursor and the like) as much as
for people: agents produce valid JSON far more reliably than valid DSL syntax
(Mermaid, D2, PlantUML), so each diagram type has a typed JSON schema and the
engine is opinionated, with good defaults and nothing to configure. Three
diagram types: **flow** (architecture, pipelines, decision trees, state
machines), **sequence** and **class**. Output is deterministic: the same spec
and theme give the same bytes on every machine.

The repository also ships a Claude Code plugin with two skills, see
[Skills for Claude Code](#skills-for-claude-code).

## Install

```bash
go install github.com/oxforge/diago/cmd/diago@latest
```

Requires Go 1.26+. PNG output additionally needs
[resvg](https://github.com/linebender/resvg) on `PATH` (`brew install resvg`);
every other format is pure Go with embedded fonts, no system dependencies.

## Usage

`diago render [flags] [spec.json]` reads a JSON spec and writes the
rendered diagram to stdout. The spec path is optional (`-` also means stdin);
when omitted, the spec is read from stdin. The bare form (no verb, or a
first argument starting with `-`) is kept as an implicit `render`, so
`diago < spec.json` still works. `diago diff` (below) renders what
changed between two flow, sequence or class specs. A CLI render is subject to a
10 second timeout, including flagless invocations such as
`diago render spec.json` and `diago < spec.json`.

`diago check` (below) prints advisories without rendering. `diago import`
(below) translates a Mermaid source into a diago JSON spec.

**Mermaid sources.** Every spec argument, and stdin, may be a Mermaid
`flowchart`/`graph`, `sequenceDiagram` or `classDiagram` instead of JSON: a
path ending in `.mmd` or `.mermaid`, or a body whose first meaningful line is
one of those headers, is translated before parsing. This holds for `render`,
`check`, both `diff` arguments (a `git diff` of a `.mmd` is still a valid
second argument) and `-previous`. Constructs diago cannot carry are
translated to their nearest form and reported on stderr as
`warning: import line <n>: <message>`; statements with no meaning in diago
fail with `diago <verb>: line <n>: <message>` and exit 1. The mapping tables
are under `diago import` below.

```bash
echo '{
  "type": "flow",
  "nodes": [
    {"id": "a", "label": "Hello", "shape": "rounded"},
    {"id": "b", "label": "World"}
  ],
  "edges": [{"from": "a", "to": "b"}]
}' | diago > hello.svg

# A spec from a file (any flow, sequence or class spec, see the JSON Spec
# Reference below), with a theme override
diago render -theme midnight spec.json > output.svg

# PNG at 3x, and Unicode text art
diago render -format png -scale 3 spec.json > output.png
diago render -format text spec.json

# Anchor a second render on the first, so unrelated nodes don't reshuffle
diago render -previous v1.svg v2.json > v2.svg
```

**Themes:** `default`, `dark`, `midnight`, `sketch`

| Flag | Default | Notes |
|------|---------|-------|
| `-theme` | spec value, else `default` | `default`, `dark`, `midnight`, `sketch` |
| `-format` | `svg` | `svg`, `png` (requires resvg), `text` (alias `txt`), `drawio`, `excalidraw` (the last two flow only); an unknown value is an error, not a silent fallback |
| `-scale` | `2.0` | PNG only; ignored when `-width` is set |
| `-width` | — | PNG output width in pixels; overrides `-scale` |
| `-previous` | — | Anchor the layout on a previous render: an SVG rendered by diago (its `<metadata id="diago-layout">` element is the exact anchor; an SVG from an older diago, rendered before its current layered layout engine, anchors as a best effort), a layout-carrier JSON, or the previous spec (exact for one step). Flow and class; sequence renders warn and ignore it. Layout rules spec C18 |
| `-debug` | off | JSON Lines layout/routing decisions on stderr — see [Debug logging](#debug-logging) |

`text` output lays out under its own cell-aligned profile (layout rules spec
C0 and S14): the profile snaps every position to character cells, so node positions
differ from the SVG's even though both are orthogonal. An edge label sits
on the row next to a wire that runs along its row, and may be written over
a group frame's left or right side when nothing nearer is free; one that
cannot be placed without overwriting other ink is dropped and reported as
the `text-label-dropped` advisory (`warning: text-label-dropped: label …`
on stderr; see `diago check`).

Every flow and class SVG embeds its layout as a `<metadata id="diago-layout">`
element: `version`, per-scope `layers` and `order`, `reversed`, and `ranked`
when a flat edge fell back to an ordinary one (ids only, never labels). That
embedded carrier is exactly what `-previous` reads back when handed an SVG
(or the same JSON shape saved on its own). Passing it, or the previous spec,
to a later render of the same structure keeps surviving nodes close to where
they were instead of the layout re-optimizing from scratch. A previous spec
is laid out fresh as the render lays out its own spec: under the text
profile for `-format text`, else under the screen profile of the theme the
render uses, so a render anchored on an unchanged spec draws what the fresh
render draws. A flat edge that fell back stays ranked in every render
anchored on that carrier, and its `flat-edge-ranked` warning says it was
kept, not that no clear side route exists; a render without `-previous`
tries the side route again and decides fresh. Anchored on a previous spec, a render keeps only the fallbacks that
spec's own layout has, laid out as above. So under `scripts/diago-render`,
which anchors each document version on the previous version's spec, laid
out fresh, a kept fallback lasts one version: the next version is anchored
on this version's own layout, and keeps the fallback only if this version,
laid out on its own, has no side route for the edge either. Only a chain of
`-previous <svg>` renders, each anchored on the SVG before it, carries a
fallback on indefinitely. Under `--strict`, add `flat-edge-ranked` to that
version's `ignore` list and drop it from the next version, never keep it
(copied forward, it would hide real fallbacks later); or keep the edge
ordinary.

### diago diff

Renders one picture of what changed between two flow, sequence or class specs: the union of both,
laid out anchored on the old layout, with added elements highlighted, changed
ones marked, unchanged ones dimmed and removed ones kept in their reserved
space (never overlapping a survivor).

```bash
diago diff v1.json v2.json > diff.svg
diago diff v1.json v2.json -format text
git diff v1.json > change.patch && diago diff v1.json change.patch   # the second argument may be a unified diff of the first
```

`-format` accepts `svg` (default), `png`, `text`; `-theme` as for `render`,
over the new spec's `theme` field (the old spec's is not read).
For sequence specs the union keeps every message row, and a removed message
opens no activation bar. Text output stamps `+ `, `- `, `~ ` on flow node and
group labels, and on sequence actor labels, message labels and section
guards (an unlabeled changed message shows the bare marker); both list
every change in a footer legend. For class specs every member line of every
class carries a two-character status column before the visibility glyph
(`+ `, `- `, `~ `, or two spaces when unchanged), removed members keep their
row, and the footer adds one line per changed member (`changed: class
Payment, removed attribute - amount: Money`); relations are named in spec
orientation. Exit 0 on identical inputs (everything
dimmed, no legend), 1 on a validation error (fields are prefixed `before.` /
`after.`, a bad patch reports on `new`), 2 on usage or internal errors.
draw.io and Excalidraw output are not available for diffs.

### diago check

Prints advisories: things a spec does that parse fine but read badly. Every
`render` and `diff` prints the same findings on stderr, each line prefixed
`warning: `, after the output. `check` never lays out, so two findings only
appear on `render` and `diff`: `text-label-dropped` (text format) and
`flat-edge-ranked` (any format, only when a flat edge falls back).

```bash
diago check spec.json              # one line per finding, exit 0
diago check -strict < spec.json    # exit 1 when any finding remains
diago check -json spec.json        # {"warnings": [{"rule", "message", "field"}]}
```

`-strict`'s exit 1 means findings survived the spec's ignore list; it is not
the same exit-1 condition `render` and `diff` use, which is a structured
validation error.

Node and actor ids are interpolated into advisory messages verbatim, so an id
containing unusual characters (quotes, control characters) can make the plain
text line awkward to parse; `-json` is the machine-readable surface for that
case.

Line format: `<rule> <field>: <message>`, or `<rule>: <message>` for findings
about the whole diagram. Findings are sorted by rule, then field in natural
order, then message. A spec
silences rules with `"ignore": ["rule", …]` at the top level; an unknown rule
name there is a validation error. Findings are returned, never logged.

| Rule | Fires when |
|------|-----------|
| `unknown-field` | a JSON key the schema does not know (silently ignored otherwise); suggests the nearest known key. `format` and `store` are allowed at the top level |
| `removed-field` | a spec still carries a removed top-level field (`style`, `hints`, `alignment` on flow; `style` on class); the field is ignored |
| `duplicate-edge` | two edges with the same `from`, `to` and label |
| `isolated-node` | a node no edge touches (two or more nodes) |
| `empty-group` | a group or package whose `contains` is empty |
| `unlabeled-branch` | a `diamond` with two or more outgoing edges and one unlabeled (a `flat` edge is not a branch and is not counted) |
| `vague-edge-label`, `vague-message-label` | a label in `uses`, `has`, `is`, `does`, `calls`, `handles`, `data`, `flow` |
| `long-label` | a node, group, actor, message or section label over 60 characters |
| `unbreakable-token` | a whitespace-free run over 24 characters in such a label |
| `too-large` | more than 20 nodes (groups never count) |
| `no-entry` | every node has an incoming edge (self-loops excluded; a `flat` edge implies no order and does not count as incoming) |
| `shape-soup` | more than 5 distinct node shapes |
| `edge-without-id` | a labeled edge, or any relation, without `id`, only under `render -previous` and `diff` |
| `flat-edge-ranked` | a `flat` edge with no clean side route, laid out as an ordinary edge, or kept as one from an earlier anchored layout (render/diff only, layout rules spec S9 and S13) |
| `seq-too-many-participants` | more than 8 actors |
| `unlabeled-alt-section` | an `alt` section with no guard label |
| `deep-nesting` | a fragment nested more than 3 levels deep (by range containment) |
| `god-class` | a class with more than 15 attributes and methods together |
| `isolated-class` | a class no relation touches (two or more classes) |
| `overlong-member` | a member line (visibility, space, text) over 40 characters |
| `oversized-class-diagram` | more than 20 classes |
| `text-label-dropped` | the text renderer could not place an edge label or a cardinality (render only) |

### diago import

```bash
diago import diagram.mmd > spec.json     # JSON on stdout, report lines on stderr
diago import < diagram.mmd | diago check
```

The dialect is sniffed from the header. The output is the JSON an agent
would have written: two-space indent, derived edge ids, `rect` omitted.
Exit 1 on a rejected line (plain text, with the line number) or on a
non-Mermaid input; exit 0 otherwise, even with report lines.

**Flowchart** (`graph`/`flowchart` + `TD|TB|LR|BT|RL`): every node bracket
form; `-->` `---` `-.->` `-.-` `==>` `===` `<-->` with `|label|` or
`-- label -->`; a trailing `;` terminator is accepted; `&` fan-out; chains;
nested `subgraph … end` (a fourth level is flattened into its depth-3
ancestor, reported); a `title:` frontmatter.
Lossy, reported: stadium and subroutine (rounded, rect), doublecircle and
trapezoid (circle, parallelogram). Rejected: `classDef`, `class`, `style`,
`linkStyle`, `click`, `accTitle`, `accDescr`, `direction`, an edge whose
endpoint is a subgraph.

**Sequence** (`sequenceDiagram`): `participant`/`actor` with `as`, implicit
declaration on first use; `->>` `-->>` `-)` `--)`; `loop` `opt` `alt`/`else`
`par`/`and` `break` blocks, nested, as fragments over interaction ranges.
Lossy, reported: `actor` renders as a participant; `+`/`-` marks and
`activate`/`deactivate` lines are ignored (bars are engine-computed); a
`Note` is folded into the label of the nearest preceding message in its
block (dropped when there is none); an empty branch or block is dropped.
Rejected: `critical`, `autonumber`, `create`, `destroy`, `rect`, `box`,
`link`, `links`, `properties`, `details`, lost-message arrows (`-x`).

**Class** (`classDiagram`): `class X~T~ { … }` blocks and bare
declarations, `<<stereotype>>` inside a block or standalone, `X : +member`
inline members, visibility prefixes, `*` abstract and `$` static suffixes,
`~T~` generics, all twelve relation operators with cardinalities and labels
(the JSON is written with `from` as the subtype, implementer, owner or
dependent), `direction TB|LR|BT|RL`. Rejected: `<-->`, `()--`, `note`,
`namespace`, `cssClass`, `click`.

---

### Debug logging

The CLI's `--debug` flag emits one JSON Lines record per layout/routing
decision on **stderr**, so a run separates cleanly into artifact and trace:

```bash
diago --debug < spec.json > out.svg 2> debug.ndjson
```

Without `--debug`, stderr stays empty on success. This is the supported way to
investigate a layout or routing bug — do not add ad-hoc `fmt.Printf`.

**Schema** — one object per line. Always present: `time`, `level` (`DEBUG`),
`msg` (always `layout decision`), `phase`, `decision` (snake_case), and
`module` (`diago`). Optional `spec_ref` names the layout rule the decision
followed, by its id: an output-contract rule (C1–C18) or a stage of the
layered engine (S0–S14). The remaining keys vary per decision:

```json
{
  "time": "2026-09-28T00:22:32.411625+02:00",
  "level": "DEBUG",
  "msg": "layout decision",
  "decision": "layers_assigned",
  "phase": "rank",
  "module": "diago",
  "spec_ref": "S3",
  "layers": 3,
  "span_longest_path": 2,
  "span": 2
}
```

**Phases**, in pipeline order: the layered engine's stages `size` (S1),
`cycle` (S2), `anchoring` (S13, runs per level before `rank` and again
before `order`, only when a previous layout is given via `-previous` or
`diago diff`: it emits `previous_read` once, when the previous carrier is
read, and `previous_ranked` once too, but only when the carrier lists flat
edges that fell back), `rank` (S3), `equalize` (S4), `lgraph` (S5), `order` (S6),
`place` (S7), `route` (S8), `nest` (S9, groups laid out level by level),
`congruence` (S12), `flat` (S9, flat edges routed on the composed layout:
`flat_edge_routed`, `flat_edge_unrouted`, and `flat_edge_ranked` when one
falls back to an ordinary edge and the layout is redone, whose stages then
emit their records again), `labels` (S10) and `frame` (S11); then `contract`
(violations of the output contract, C2-C17, from `internal/layout/contract`,
run on the final positioned graph in its render profile, screen or text;
each record's `spec_ref` is the violated rule, e.g. `C5`, `subject` the
edge, node or group id it applies to, and `detail` the checker's message;
emitted only when `--debug` is armed). Sequence diagrams emit no records.

**Useful queries:**

```bash
# Every decision citing one spec rule
jq 'select(.spec_ref == "S8")' debug.ndjson

# Only one stage's decisions
jq 'select(.phase == "route")' debug.ndjson

# Everything mentioning one node, across the differing key conventions
jq 'select(.node == "db" or .source == "db" or .subject == "db")' debug.ndjson

# Every output-contract violation --debug found in this render
jq 'select(.decision == "contract_violation")' debug.ndjson

# What ran, and how much
jq -r '.phase' debug.ndjson | sort | uniq -c | sort -rn
jq -r '(.module // "-") + " | " + .phase + " | " + .decision' debug.ndjson | sort -u
```

**Workflow:** save the failing spec to a file → render it with `--debug` →
identify the misplaced node or edge in the SVG → grep the ndjson for that id →
walk the decision chain backward to the phase that made the wrong call. If no
record explains it, the gap is the bug report: extend the
`internal/layoutdbg.Decision` call sites at that spot rather than
printing.

**Cost:** when debug is off, each call site is one
`slog.Logger.Enabled(LevelDebug)` comparison and allocates nothing. When on,
expect thousands of lines — always redirect stderr to a file.


## Configuration

| Variable | Description |
|----------|-------------|
| `DIAGO_RESVG_PATH` | Path to the resvg binary, overriding the `PATH` lookup (PNG only). |
| `DIAGO_THEME_DIR` | Directory searched for theme JSON files whose name is not a built-in theme (built-ins always win). |

## JSON Spec Reference

### Flow Diagram

```json
{
  "type": "flow",
  "title": "Request Path",
  "direction": "DOWN",
  "nodes": [
    {"id": "client", "label": "Client App", "shape": "rounded"},
    {"id": "api", "label": "API Gateway", "shape": "rect"},
    {"id": "db", "label": "PostgreSQL", "shape": "cylinder"}
  ],
  "edges": [
    {"from": "client", "to": "api", "label": "HTTPS"},
    {"from": "api", "to": "db", "label": "query", "style": "dashed"}
  ],
  "groups": [
    {"id": "backend", "label": "Backend", "contains": ["api", "db"]}
  ]
}
```

**`title`:** optional string, rendered as a band above the diagram (SVG, PNG) or a centered first line (text); never part of the layout.
**`theme`:** optional theme name (`default`, `dark`, `midnight`, `sketch`, or one in `DIAGO_THEME_DIR`): the theme the render uses unless `-theme` overrides it; text art takes none. A name that does not load is a validation error on `theme`.
**Directions:** `DOWN`, `UP`, `RIGHT`, `LEFT`, `AUTO` (default — the engine picks based on graph shape)
**Routing:** every edge is routed orthogonally (90° segments); there is no diagram-level style to choose.
**Shapes:** `rect`, `rounded`, `circle`, `diamond`, `cylinder`, `hexagon`, `parallelogram`
**Edge styles:** `solid`, `dashed`, `dotted`, `thick`
**Edge directions:** `forward` (default), `backward`, `both`, `none` (no arrowhead at either end)
**Edge id:** optional `id` field, stable across edits (anchoring via `-previous` and `diago diff` key on it). Defaults to `from->to#n`, where `n` counts every edge between the same pair in spec order.
**`flat`:** optional boolean, default `false`. This link does not imply order: its ends stay wherever the rest of the diagram puts them, side by side when it allows, instead of taking part in ranking, and the edge is routed on the finished layout. Use it for links between peers that would otherwise be forced into a hierarchy they don't have: replication between two sites that each serve their own traffic, a sync between two services at one level. It helps only between nodes that other edges already place: a node whose only edges are flat is laid out as an unconnected node (layout rules spec S3), wherever there is room rather than beside its partner, so keep one of its links ordinary. A standby database with nothing else attached, for one, keeps its replication edge from the primary ordinary, which puts it under the primary. Not allowed on a self-loop (validation error). When no clean route exists between the ends, the edge is laid out as an ordinary edge and the render warns (`flat-edge-ranked`, see `diago check`). The warning belongs to the format rendered: text and SVG lay out under different profiles and can decide a flat edge differently, so the SVG can draw flat an edge the text art warns about (`scripts/diago-render` lints the text render).

```json
{"from": "east_db", "to": "west_db", "label": "async", "style": "dashed", "flat": true}
```

The typical case is two peer sites, each with its own web, API and database, joined by one flat edge between their databases: without it, the two sites stack instead of sitting side by side (the role dot's `constraint=false` plays).
**`ignore`:** optional array of advisory rule names to silence for this spec (see `diago check`); an unknown name is a validation error.

### Sequence Diagram

```json
{
  "type": "sequence",
  "activations": true,
  "actors": [
    {"id": "client", "label": "Client"},
    {"id": "server", "label": "Server"}
  ],
  "interactions": [
    {"from": "client", "to": "server", "label": "GET /api", "style": "solid"},
    {"from": "server", "to": "client", "label": "200 OK", "style": "dashed"}
  ]
}
```

**`title`:** optional string, rendered as a band above the diagram (SVG, PNG) or a centered first line (text); never part of the layout.
**`theme`:** optional theme name (`default`, `dark`, `midnight`, `sketch`, or one in `DIAGO_THEME_DIR`): the theme the render uses unless `-theme` overrides it; text art takes none. A name that does not load is a validation error on `theme`.
**Interaction styles:** `solid`, `dashed`, `async`
**Fragment types:** `alt`, `opt`, `loop`, `par`, `break`
**`activations`:** boolean, default `true` — set `false` to hide the activation bars on lifelines
**`ignore`:** optional array of advisory rule names to silence for this spec (see `diago check`)

### Class Diagram

```json
{
  "type": "class",
  "direction": "DOWN",
  "theme": "default",
  "classes": [
    { "id": "order", "label": "Order", "stereotype": "entity", "type_params": ["T"],
      "color": "blue",
      "attributes": [ { "visibility": "-", "text": "id: string" } ],
      "methods":    [ { "visibility": "+", "text": "total(): Money", "static": false, "abstract": false } ] },
    { "id": "item", "label": "Item" }
  ],
  "relations": [
    { "id": "r1", "from": "order", "to": "item", "kind": "composition",
      "label": "contains", "from_card": "1", "to_card": "*", "color": "red" }
  ],
  "packages": [ { "id": "domain", "label": "Domain", "contains": ["order", "item"], "color": "gray" } ],
  "legend": true,
  "legend_labels": { "inheritance": "is-a" },
  "ignore": ["god-class"]
}
```

**`title`:** optional string, rendered as a band above the diagram (SVG, PNG) or a centered first line (text); never part of the layout.
**`theme`:** optional theme name (`default`, `dark`, `midnight`, `sketch`, or one in `DIAGO_THEME_DIR`): the theme the render uses unless `-theme` overrides it; text art takes none. A name that does not load is a validation error on `theme`.
**Relation kinds:** `association` (default), `inheritance`, `realization`, `dependency`, `aggregation`, `composition`. For `inheritance` and `realization`, `from` is the subtype and `to` the supertype or interface; the engine lays the supertype out first and draws the hollow triangle at its end.
**`directed`:** arrowhead at `to`; default `true` for `dependency`, `false` for `association`; not allowed on the four adorned kinds.
**Cardinalities:** `from_card`, `to_card`, placed beside the wire ends.
**Members:** `visibility` in `+ - # ~`; `static` underlines, `abstract` italicises (text art: ` $` and ` *` suffixes).
**Packages:** the flow group rules under the key `packages`.
**Legend:** `legend: true` adds one row per relation kind present; `legend_labels` overrides a row's text, an empty value hides it.
**Relation id:** as for flow edges, derived from `from` and `to` as written (`from->to#n`).
draw.io and Excalidraw export cover flow diagrams only.

## Skills for Claude Code

The repository is a Claude Code plugin (`.claude-plugin/`) with two skills:

- **`diago:diagramming`**: how to write good diago specs (choosing a diagram
  type, shapes, groups, edge styles, themes) and drive the CLI.
- **`diago:spec-diagrams`**: diagram-augmented design specs and implementation
  plans for human review. When a superpowers brainstorming or writing-plans
  document is written or revised, the agent draws its structure (architecture,
  key sequences, data model, the plan's task graph), prints the text art in the
  terminal before asking for review, embeds it in the markdown, and on every
  revision shows a **diff diagram**: additions and removals marked, changes
  flagged, everything else pinned in place by layout anchoring. The mechanics
  are `scripts/diago-render`; `skills/spec-diagrams/example/` is a worked example.

Install from GitHub, or from a local checkout (needs `diago` on `PATH`):

```bash
claude plugin marketplace add oxforge/diago    # or: /path/to/diago
claude plugin install diago@diago
```

The install copies the repo at its current commit into Claude Code's plugin
cache, and `claude plugin update` only refreshes that copy when the version in
`.claude-plugin/plugin.json` and `marketplace.json` changes. After changing a
skill, bump that version and run `claude plugin update diago@diago`.

## Development

Needs [just](https://just.systems) (`brew install just`; bare `just` lists every
recipe) and optionally [golangci-lint](https://golangci-lint.run).

```bash
just build          # bin/diago
just install        # go install ./cmd/diago
just test           # all Go tests
just lint           # golangci-lint
just schemas        # regenerate schemas/{flow,sequence,class}.json; required after any change under internal/schema/
```

`AGENTS.md` describes the code layout and the conventions, for people and
coding agents alike. To investigate a layout or routing bug, start from
[Debug logging](#debug-logging).

## License

MIT, see `LICENSE`.
