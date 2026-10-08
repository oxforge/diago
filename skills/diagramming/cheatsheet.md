# diago JSON cheatsheet

Three diagram types, selected by `type`. Every id is a stable identity: keep it
across versions when the thing is the same. How to draw well: `SKILL.md` beside
this file; full reference: https://diago.dev/docs/.

## Flow (architecture, states, task DAGs)

```json
{ "type": "flow", "title": "Login", "direction": "DOWN",
  "nodes": [
    { "id": "req", "label": "Request", "shape": "rounded", "color": "purple" },
    { "id": "ok", "label": "Authed?", "shape": "diamond" },
    { "id": "app", "label": "Handler" },
    { "id": "deny", "label": "401 page" }
  ],
  "edges": [
    { "id": "e-req-ok", "from": "req", "to": "ok" },
    { "id": "e-ok-app", "from": "ok", "to": "app", "label": "yes", "style": "thick" },
    { "id": "e-ok-deny", "from": "ok", "to": "deny", "label": "no" }
  ],
  "groups": [ { "id": "backend", "label": "Backend", "contains": ["app", "deny"] } ] }
```

Groups nest up to 3 levels (a group id inside another group's `contains`).

## Sequence (a runtime path)

```json
{ "type": "sequence", "title": "Submit form",
  "actors": [ { "id": "u", "label": "User" }, { "id": "web", "label": "Web App" } ],
  "interactions": [
    { "from": "u", "to": "web", "label": "submit form" },
    { "from": "web", "to": "web", "label": "validate input" },
    { "from": "web", "to": "u", "label": "redirect", "style": "dashed" },
    { "from": "web", "to": "u", "label": "error 422", "style": "dashed" }
  ],
  "fragments": [
    { "type": "alt", "over": ["u", "web"], "sections": [
      { "label": "valid", "start": 2, "end": 2 },
      { "label": "invalid", "start": 3, "end": 3 }
    ] }
  ] }
```

Fragment sections cover interactions by zero-based index, `start` to `end`
inclusive. Self-messages use the same actor for `from` and `to`.

## Class (data model)

```json
{ "type": "class", "title": "Orders", "legend": true,
  "classes": [
    { "id": "order", "label": "Order",
      "attributes": [ { "visibility": "-", "text": "id: string" } ],
      "methods": [ { "visibility": "+", "text": "total(): Money" } ] },
    { "id": "item", "label": "LineItem" },
    { "id": "priced", "label": "Priced", "stereotype": "interface" }
  ],
  "relations": [
    { "id": "r-order-item", "from": "order", "to": "item", "kind": "composition", "from_card": "1", "to_card": "*" },
    { "id": "r-item-priced", "from": "item", "to": "priced", "kind": "realization" }
  ] }
```

For `inheritance` and `realization`, `from` is the subtype, `to` the supertype.

## Enums

| Field | Values |
|---|---|
| `direction` (flow, class) | `DOWN`, `UP`, `RIGHT`, `LEFT`, `AUTO` (default) |
| node `shape` | `rect` (default), `rounded`, `circle`, `diamond`, `cylinder`, `hexagon`, `parallelogram` |
| `color` (nodes, edges, groups, actors, interactions, classes, relations, packages) | `red`, `green`, `blue`, `yellow`, `orange`, `purple`, `gray`, or `#rrggbb`; use 0–2 per diagram |
| edge `style` | `solid`, `dashed`, `dotted`, `thick` |
| edge `direction` | `forward` (default), `backward`, `both`, `none` |
| edge `flat` | `true`: a link that implies no order, laid out side by side when it can, between nodes other edges already place (a node whose only edges are flat is placed as an unconnected node: keep one of its links ordinary); not on a self-loop |
| interaction `style` | `solid`, `dashed`, `async` |
| fragment `type` | `alt`, `opt`, `loop`, `par`, `break` |
| relation `kind` | `association` (default), `inheritance`, `realization`, `dependency`, `aggregation`, `composition` |
| member `visibility` | `+`, `-`, `#`, `~` |

Optional top-level keys on every type: `title` (a band above the diagram, a
centered first line in text art) and `ignore` (advisory rule names to silence).
Class diagrams take `packages` (groups under another name) and `legend: true`.

## Advisories you will hit

`vague-edge-label` / `vague-message-label` ("data", "calls"), `edge-without-id`,
`long-label` (over 60 characters), `unbreakable-token` (a word over 24
characters), `shape-soup` (more than 5 shapes), `too-large` (more than 20
nodes), `deep-nesting` (sequence fragments nested over 3 levels), `unlabeled-branch` (a diamond
whose out-edges have no labels), `unlabeled-alt-section`, `isolated-node`,
`seq-too-many-participants` (more than 8), `god-class` (more than 15 members),
`overlong-member` (over 40 characters), and `text-label-dropped` (text art had
no room for a label: shorten it). Fix the JSON; add a rule to `ignore` only
when the finding is deliberate, never to silence it.

`removed-field` means a leftover top-level `style`, `hints` or `alignment`
key from before diago went orthogonal-only; under `--strict` it fails the
lint, so delete the key.

`flat-edge-ranked` means a `flat` edge had no clean side route and was laid
out as an ordinary edge (render/diff only, not `diago check` alone); the
diagram still renders, its two ends just don't sit side by side. The warning
belongs to the format rendered: text and SVG lay out under different
profiles and can decide a flat edge differently, and the lint is the text
render's, so the SVG can draw flat an edge the lint warns about. A message
that says the edge was *kept* as an ordinary edge, rather than that no clear
side route exists, means the previous version's own layout fell back on it
and `scripts/diago-render`'s `-previous` anchoring carried that fallback
into this version. It lasts one version: the next version is anchored on
this version's own layout and keeps it only if this version, laid out on its
own, has no side route for the edge either. Under `--strict`, add
`flat-edge-ranked` to this version's `ignore` list and delete it again in
the next version (a deliberate, one-version ignore: copied forward, it would
hide real fallbacks), or drop `flat` from the edge and keep it ordinary.
