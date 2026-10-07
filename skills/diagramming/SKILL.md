---
name: diagramming
description: Use when drawing an architecture, flow, sequence, class or data-model diagram, rendering a diagram to SVG, PNG, terminal text art, draw.io or Excalidraw, converting a Mermaid diagram, or showing how a diagram changed between two versions
---

# Diagramming with diago

diago is a CLI that renders a typed JSON spec to a diagram. You describe what is
connected to what; diago does layout and rendering. Write the spec to a file,
run `diago`, and show the result.

**Prerequisite:** `command -v diago`. If missing:
`go install github.com/oxforge/diago/cmd/diago@latest` (PNG output also needs
`resvg` on `PATH`).

## Commands

| Command | Does |
|---------|------|
| `diago render [flags] spec.json > out.svg` | Render (stdin when no path) |
| `diago check [-strict] [-json] spec.json` | Advisories only, no layout; `-strict` exits 1 on any finding |
| `diago diff old.json new.json [flags]` | One diagram of both versions, each element marked added/removed/changed |
| `diago import diagram.mmd > spec.json` | Mermaid (`flowchart`/`graph`, `sequenceDiagram`, `classDiagram`) to diago JSON |

Render flags: `-format svg|png|text|drawio|excalidraw` (default `svg`; `txt` is
an alias of `text`; `drawio`/`excalidraw` are flow-only; `diff` takes
`svg|png|text`), `-theme`, `-scale n` / `-width px` (PNG),
`-previous <old spec | old SVG | layout JSON>` (anchor the layout so unchanged
elements keep their places), `-debug`. Every verb also accepts a `.mmd` file.
Edges are always orthogonal.

Exit codes: 0 ok, 1 validation error (structured JSON on stderr naming the
`field`), 2 usage or internal error. Advisories print on stderr as
`warning: <rule> <field>: <message>`: fix the spec, or add the rule name to the
top-level `"ignore": [...]` list when the finding is deliberate.

## Choosing a Diagram Type

| If the user wants to see... | Use |
|-----------------------------|-----|
| System architecture, component relationships, data flow, decision trees | `"type": "flow"` |
| Request/response sequences, API calls over time, protocol handshakes | `"type": "sequence"` |
| Types, members, inheritance, composition, interfaces | `"type": "class"` |

When ambiguous (e.g., "show me how auth works"), ask one question:
"Do you want to see the system architecture or the request flow?"

Prefer rendering a reasonable first attempt over asking multiple clarifying questions.
A concrete diagram the user can react to is more useful than an interrogation.

## Guidelines

- **Keep it focused.** Aim for 5-12 nodes. If a diagram needs 15+, split into multiple focused diagrams and explain the split.
- **Pick the right type.** Interactions over time = sequence. Structure and connections = flow.
- **Use shape variety.** Databases get `cylinder`, gateways get `hexagon`, clients get `rounded`. Shape communicates role at a glance.
- **Group when it helps.** Groups show deployment boundaries or logical domains. Don't group if every node would be alone in its group.
- **Label concisely.** 2-3 word labels. Prefer role names ("Auth Service") over technical names ("auth-svc-v2"). Only label edges when the connection type isn't obvious.

## Flow Diagrams

### Direction

Choose direction based on what the diagram communicates:

| Direction | When to use |
|-----------|-------------|
| `DOWN` | Default. Hierarchies, pipelines, decision trees (top-down flow) |
| `RIGHT` | Client-server architectures, data pipelines, request flow (left-to-right reading) |
| `UP` | Bottom-up compositions, dependency graphs (depends-on points up) |
| `LEFT` | Reverse flows, response paths |
| `AUTO` | Let the layout engine choose based on graph shape (default) |

### Node Shapes

Pick shapes that communicate the node's role at a glance:

| Shape | Semantic meaning | Examples |
|-------|-----------------|----------|
| `rect` | Services, processes, generic components | API Server, Worker, Processor |
| `rounded` | User-facing entry points, clients, external systems | Web Client, Mobile App, Browser |
| `diamond` | Decision points, conditions, branching logic | Is Valid?, Check Auth, Retry? |
| `cylinder` | Data stores, databases, caches, queues | PostgreSQL, Redis, S3, Kafka |
| `hexagon` | Infrastructure, middleware, proxies, gateways | Load Balancer, Reverse Proxy, API Gateway |
| `parallelogram` | I/O operations, external interfaces, data transforms | HTTP Request, File Upload, CSV Export |
| `circle` | Events, triggers, signals, small connectors | Start, End, Webhook, Timer |

**Defaults:** Use `rect` when unsure. Don't overthink shape selection — a diagram with all `rect` nodes is fine. Shapes add clarity but aren't required.

### Groups

Use groups to show **deployment boundaries** or **logical domains**:

```json
"groups": [
  {"id": "frontend", "label": "Frontend", "contains": ["web", "mobile"]},
  {"id": "backend", "label": "Backend Services", "contains": ["api", "auth", "db"]}
]
```

**When to group:**
- Nodes share a deployment boundary (same server, same cluster, same VPC)
- Nodes belong to the same team/domain
- The boundary matters for understanding the system

**When NOT to group:**
- Every node would be in its own group (adds clutter, no information)
- Groups would have only 1 node (use a label instead)
- The grouping doesn't help the reader understand the system

**Nesting is supported.** Diago supports up to 3 levels of group nesting. To nest groups, reference a group ID inside another group's `contains` array. Each depth level is visually differentiated with distinct fill shading.

```json
"groups": [
  {"id": "cloud", "label": "Cloud", "contains": ["services", "db"]},
  {"id": "services", "label": "Services", "contains": ["api", "auth"]}
]
```

In this example, the `services` group is nested inside `cloud`. Only use nesting when the hierarchy genuinely represents containment (e.g., a cluster inside a VPC, a domain inside a bounded context). Nesting supports up to 3 levels deep — deeper nesting is visually differentiated via fill shading.

### Edges

**Style by meaning:**

| Style | Use for |
|-------|---------|
| `solid` | Primary data flow, requests, main path (default) |
| `dashed` | Responses, callbacks, async results, secondary flows |
| `dotted` | Optional paths, monitoring, logging, weak dependencies |
| `thick` | Critical path, high-throughput connections, emphasis |

**Direction:**

| Direction | Use for |
|-----------|---------|
| `forward` | One-way flow: requests, events, pushes (default) |
| `backward` | Reverse flow: responses flowing "upstream" |
| `both` | Bidirectional: request/response pairs, sync channels, two-way communication |
| `none` | Undirected association: peers, replication pairs, any link with no flow |

**Prefer `"direction": "both"`** over two separate edges when a connection is inherently bidirectional (e.g., database query/result, API request/response). It produces cleaner diagrams with fewer edge crossings.

**Label edges** when the connection type isn't obvious from context. Skip labels for self-evident connections (e.g., a single edge from "Client" to "Server" probably doesn't need "request").

**Mark a link `"flat": true`** when it connects peers that should sit side by side, not one implying the other comes first or last, and that other edges already place: replication between two sites that each serve their own traffic, a sync between two services at the same level. A node whose only edges are flat is laid out as an unconnected node, wherever there is room rather than beside its partner, so keep one of its links ordinary: a standby database with nothing else attached keeps its replication edge from the primary ordinary, which puts it under the primary. Don't mark a link flat when it is part of the flow itself (a request, a dependency, a pipeline step): that would just hide the order the diagram is meant to show. A flat edge that has no clean side route once the rest of the diagram is placed is laid out as an ordinary edge instead, with a warning (`flat-edge-ranked`); not allowed on a self-loop. Anchored on a previous layout (`render -previous`, or `diago diff`'s union), a flat edge that carrier already lists as fallen back stays an ordinary edge without its side route being retried, and the warning says it was kept from that earlier layout, not that no clean route exists.

### Colors

Optional `color` field on nodes, edges, and groups. Use color to highlight critical paths, distinguish domains, or draw attention to specific components.

**Named colors:** `red`, `green`, `blue`, `yellow`, `orange`, `purple`, `gray`
**Custom hex:** any `#rrggbb` value

```json
"nodes": [
  {"id": "db", "label": "PostgreSQL", "shape": "cylinder", "color": "red"},
  {"id": "cache", "label": "Redis", "shape": "cylinder", "color": "#2ecc71"}
],
"edges": [
  {"from": "api", "to": "db", "label": "query", "color": "blue"}
],
"groups": [
  {"id": "critical", "label": "Critical Path", "contains": ["api", "db"], "color": "orange"}
]
```

Colors auto-derive fill, stroke, and text colors for visual consistency with the active theme. Use sparingly — one or two colored elements draw attention; coloring everything defeats the purpose.

### Theme

| Theme | When to use |
|-------|-------------|
| `default` | Light backgrounds, documents, GitHub READMEs |
| `midnight` | Dark backgrounds, slides with dark themes |
| `dark` | Dark mode UIs |
| `sketch` | Hand-drawn feel, informal presentations, brainstorming |

Pick one with the spec's top-level `theme` field or `-theme` (the flag wins; `diago diff` reads the new spec's field). Text art takes no theme.

**Title:** optional `title` string on every diagram type, rendered as a band above the diagram (SVG/PNG) or a centered first line (text); render-time only, never in layout or the anchoring carrier.

## Sequence Diagrams

### Actors

Name actors by their **role**, not their technology:

```json
"actors": [
  {"id": "client", "label": "Client"},
  {"id": "gateway", "label": "API Gateway"},
  {"id": "auth", "label": "Auth Service"}
]
```

Order actors left-to-right matching the primary flow direction.

### Interactions

**Style by meaning:**

| Style | Use for |
|-------|---------|
| `solid` | Synchronous calls, requests |
| `dashed` | Responses, return values |
| `async` | Async messages, events, fire-and-forget (open arrowhead) |

**Fragments** model conditional/loop logic:

| Type | Use for |
|------|---------|
| `alt` | If/else branches (multiple sections) |
| `opt` | Optional path (single section) |
| `loop` | Repeated interactions |
| `par` | Parallel execution |
| `break` | Early exit / error handling |

### Colors

Optional `color` field on actors and interactions. Same named colors (`red`, `green`, `blue`, `yellow`, `orange`, `purple`, `gray`) and custom hex (`#rrggbb`) as flow diagrams.

```json
"actors": [
  {"id": "auth", "label": "Auth Service", "color": "blue"}
],
"interactions": [
  {"from": "client", "to": "auth", "label": "login", "style": "solid", "color": "red"}
]
```

### Self-Messages

When an actor sends a message to itself (e.g., internal processing, validation), use the same actor ID for `from` and `to`:

```json
{"from": "auth", "to": "auth", "label": "validate token"}
```

**Title:** optional `title` string on every diagram type, rendered as a band above the diagram (SVG/PNG) or a centered first line (text); render-time only, never in layout or the anchoring carrier.

## Class Diagrams

```json
{
  "type": "class",
  "classes": [
    {"id": "vehicle", "label": "Vehicle", "stereotype": "abstract",
     "methods": [{"visibility": "+", "text": "range(): km", "abstract": true}]},
    {"id": "truck", "label": "Truck",
     "attributes": [{"visibility": "-", "text": "payload: t"}]},
    {"id": "engine", "label": "Engine"}
  ],
  "relations": [
    {"from": "truck", "to": "vehicle", "kind": "inheritance"},
    {"from": "truck", "to": "engine", "kind": "composition", "from_card": "1", "to_card": "1"}
  ],
  "legend": true
}
```

**Relation kinds:** for `inheritance` and `realization`, `from` is always the
subtype and `to` the supertype or interface. Otherwise pick the kind by
ownership: `composition` for owned parts (the part cannot outlive the
whole), `aggregation` for shared parts (the part can outlive the whole),
`dependency` for uses (a method parameter or a call, nothing owned).
`association` is the plain default for everything else.

**Members:** keep each class to the attributes and methods the reader
actually needs to follow the diagram, not the whole real type. The
`god-class` advisory warns above 15 attributes and methods combined.

**Legend:** set `legend: true` once three or more relation kinds appear in
the same diagram, so the reader doesn't have to infer the adornments.

**Title:** optional `title` string on every diagram type, rendered as a band above the diagram (SVG/PNG) or a centered first line (text); render-time only, never in layout or the anchoring carrier.

## Presenting Results

- **In a terminal or chat** (Claude Code, Codex CLI), render `-format text` and
  paste the art in a fenced `text` block. Text art is always laid out
  orthogonally and works for all three types.
- **For documents and READMEs**, render `svg` (canonical, self-contained, embeds
  its font) or `png` (2x scale by default).
- **For dark backgrounds**, use `-theme midnight` or `-theme dark`.
- **When a human wants to edit it further**, render `-format drawio` (diagrams.net,
  also importable by Lucidchart and yEd) or `-format excalidraw` (excalidraw.com,
  Obsidian, VS Code). Flow diagrams only.
- **When revising a diagram the user has already seen**, keep every id that still
  names the same thing, render the new version with `-previous old.json` so the
  layout does not reshuffle, and show `diago diff old.json new.json -format text`:
  its footer lists what was added, removed and changed.
- Output is deterministic: the same spec and theme give the same bytes.

For diagrams inside design specs and implementation plans, use
diago:spec-diagrams.
