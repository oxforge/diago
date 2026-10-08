---
name: diagramming
description: Use when drawing an architecture, flow, sequence, class, state or data-model diagram with diago, rendering one to SVG, PNG, terminal text art, draw.io or Excalidraw, converting a Mermaid diagram, or showing how a diagram changed between two versions
---

# Diagramming with diago

diago renders a typed JSON spec to a diagram: you say what is connected to
what, diago lays it out and draws it. This skill is how to draw one well.
When and where to show one is diago:illustrating; the JSON shapes and enums
are in `cheatsheet.md` beside this file.

**Prerequisite:** `command -v diago`, else
`go install github.com/oxforge/diago/cmd/diago@latest`. PNG also needs
`resvg` (`command -v resvg`); without it, render SVG (or text in a terminal)
and tell the user that `brew install resvg` or `cargo install resvg` enables
PNG. Install nothing without asking, and never fall back to Mermaid or
hand-drawn ASCII.

## The loop

1. Write the spec to a file. Give every node and edge an explicit `id`, a
   short kebab-case name: ids keep an element the same across versions.
   A diagram you show in chat goes in diago:illustrating's topic folder
   (`${TMPDIR:-/tmp}/diago/<topic>/<slug>.v1.json`), so a later change can be
   shown as a diff.
2. `diago check -strict spec.json`: advisories only, no layout, exit 1 on
   any finding. Fix the spec; add a rule to the top-level `ignore` list only
   when the finding is deliberate.
3. `diago render -format text spec.json` and read the art yourself before
   anyone else sees it: crossings, a label beside the wrong wire, a dropped
   label (`text-label-dropped`), the width.
4. Adjust (direction, fewer or shorter labels, a split) and render again.
5. Render the final format.

## Choosing the type

| The reader needs to see | Type |
|---|---|
| Structure: components and how they connect, dependencies, task order, states, business rules and their branches | `flow` |
| Who calls whom, in what order: a request path, a protocol, a handshake | `sequence` |
| Types, their members and relations: a data model, a type hierarchy | `class` |

When it is ambiguous, draw a reasonable first attempt rather than ask: a
diagram the user can react to beats a question.

## Size

- 3 to 12 nodes in a chat reply, 5 to 15 in a document. `too-large` fires
  above 20. Split a bigger subject into an
  overview and one zoom-in per area, and say how they fit together.
- Labels of 1 to 3 words, role names over identifiers ("Auth Service", not
  `auth-svc-v2`). A word over 24 characters trips `unbreakable-token`: for
  `internal/layout/layered/route` write `route`. A label over 60 characters
  trips `long-label`.

## Direction

Flow and class diagrams take a `direction`. Pick it by the medium's tight
axis, not by habit:

| Content | Text art in a terminal (width is scarce) | Image in a document or slide (height is scarce) |
|---|---|---|
| A long chain: a pipeline, steps | `DOWN`: 6 steps are 18 columns wide, 114 in `RIGHT` | `RIGHT`, a banner |
| A wide fan-out: one node, many children | `RIGHT`: 5 children are 34 columns wide, 72 in `DOWN` | `DOWN` |
| A hierarchy, a decision tree, layers | `DOWN` | `DOWN` |

Set it yourself: `AUTO` picks `DOWN` for any connected graph and `RIGHT`
only when there are more nodes than edges plus one. `UP` fits only when the
reader expects foundations at the bottom; `LEFT` almost never. When in doubt,
render both and keep the one that fits and reads better.

## Shapes

Most nodes are `rect`. Give each other shape one role and keep it:

| Shape | Role | In text art |
|---|---|---|
| `rect` | services, processes, steps (the default) | a box |
| `rounded` | outside actors (users, clients, external systems); a flowchart's start and end | rounded corners |
| `cylinder` | anything that holds state: a database, cache, queue, bucket | a double top border |
| `diamond` | decisions only; label every exit (`unlabeled-branch`) | `◇` marks |
| `hexagon` | gateways, proxies | a box, like `rect` |
| `parallelogram` | input and output | a box, like `rect` |
| `circle` | events, start and end markers, short labels | like `rounded` |

At most 4 distinct shapes per diagram (`shape-soup` fires above 5). Text art
keeps only `rect`, `rounded`, `cylinder` and `diamond` apart: when a diagram
may be read as text, a role shown by `hexagon`, `parallelogram` or `circle`
must also be in its label.

## Edges

The line style carries meaning, and it survives in text art:

| Style | Meaning | In text art |
|---|---|---|
| `solid` | the main or synchronous path (the default) | `─ │` |
| `dashed` | async calls, responses, events | `╌ ╎` |
| `dotted` | optional links, monitoring, logging | `┈ ┊` |
| `thick` | the one path the reader should follow, at most one per diagram | `━ ┃` |

- `direction`: `forward` (the default), `backward`, `both` for a
  request/response pair (one edge, not two), `none` for a peer link with no
  flow.
- Labels: a verb phrase (`publishes order`, not `data`: `vague-edge-label`),
  only where the two ends do not make the meaning obvious, and always on a
  diamond's exits. Each label is one more thing to place: fewer labels read
  cleaner.
- `"flat": true` marks a link that implies no order, between peers that other
  edges already place (replication between two sites that each serve their
  own traffic, a sync between two services at one level); diago puts its ends
  side by side when it can. It is independent of `direction`: a flat edge
  keeps the arrowheads its `direction` gives it. A node whose only edges are
  flat is placed as an unconnected node, not beside its partner, so keep one
  of its links ordinary: a standby database with nothing else attached keeps
  its replication edge from the primary ordinary, which places it after the
  primary in the flow's direction. Do not mark a link flat when it is part of
  the flow (a request, a dependency, a pipeline step), and never on a
  self-loop. With no clean side route, diago lays it out as an ordinary edge
  and warns `flat-edge-ranked`.

## Color

Color is the weakest channel, since text art drops it: never let it carry
meaning alone.

- 0 to 2 accents per diagram: one for the focus (the entry point, the
  component under discussion), `orange` for a problem. Leave the
  rest uncolored.
- Say what a color means in the sentence above the diagram: a flow diagram
  has no color legend.
- To tell domains apart, color the groups, not their nodes.
- No `green`, `blue` or `red` in a spec you will diff: the diff draws added
  elements green, changed ones blue and removed ones red. An element keeps
  its own fill in a diff; its outline and label show the status.
- Values: `red`, `green`, `blue`, `yellow`, `orange`, `purple`, `gray`, or
  `#rrggbb`, on nodes, edges and groups, actors and interactions, classes,
  relations and packages.

## Groups and packages

A group (a class diagram's package) is for a boundary that matters to the
reader: a deployment boundary, a package, a team, a trust zone. At least 2
members each; nesting at most 3 levels (deeper is a validation error; a group id in another
group's `contains` nests it). In text art keep group labels to one short
word: a long one widens the frame, and the art with it.

## Sequence diagrams

- The initiator leftmost, then the actors in order of first appearance,
  named by role ("Auth Service"), not technology.
- `solid` for calls, `dashed` for returns, `async` for fire-and-forget (an
  open arrowhead). A self-message (the same `from` and `to`) for an internal
  step.
- Fragments: `alt` (if/else, a section each), `opt`, `loop`, `par`, `break`.
  Label every section (`unlabeled-alt-section`). Sections cover interactions
  by zero-based index, `start` to `end` inclusive.
- At most 8 participants (`seq-too-many-participants`), about 6 when the art
  must fit a terminal: each actor is a column.
- `"activations": false` drops the activation bars for a lighter drawing.

## Class diagrams

- `inheritance` and `realization`: `from` is the subtype, `to` the supertype
  or interface. Otherwise pick the kind by ownership: `composition` for owned
  parts (the part cannot outlive the whole), `aggregation` for shared parts,
  `dependency` for uses (a parameter, a call), `association` for anything
  else.
- Members: only the attributes and methods the reader needs (`god-class`
  fires above 15; a member over 40 characters trips `overlong-member`).
- `"legend": true` once 3 or more relation kinds appear.

## Themes and title

| Theme | For |
|---|---|
| `default` | light documents and READMEs, an SVG opened from a chat |
| `dark`, `midnight` | dark pages and slides |
| `sketch` | a draft: it says "not final" |

Set it with the spec's top-level `theme` or `-theme` (the flag wins). Text
art takes no theme. Add a `title` when the diagram stands alone in a file: a
band above the diagram, a centered first line in text art.

## CLI reference

| Command | Does |
|---|---|
| `diago render [flags] SPEC > out` | Render. SPEC is a JSON spec, a Mermaid file (`.mmd`, rendered as it is), `REV:PATH` (the file at a git revision, `HEAD~1:spec.json`), or stdin when omitted |
| `diago check [-strict] [-json] SPEC` | Advisories only, no layout; `-strict` exits 1 on any finding |
| `diago diff old.json new.json [flags] > out` | One diagram of both versions, each element marked added, removed or changed, with the list of changes under it (a footer in text art) and a first line naming both sides; either side may be `REV:PATH`, and `diago diff spec.json` alone draws a git-tracked spec's latest change |
| `diago import diagram.mmd > spec.json` | Mermaid (`flowchart`/`graph`, `sequenceDiagram`, `classDiagram`) to diago JSON, only to keep editing it as JSON: every verb reads a `.mmd` file directly |

Always name the verb: `diago spec.json` and `diago -format text spec.json` are errors.

Flags: `-format svg|png|text|drawio|excalidraw` (default `svg`; `txt` is an
alias of `text`; `drawio` and `excalidraw` are flow only and in beta, so
check the file they write; `diff` takes
`svg|png|text`), `-theme`, `-scale n` and `-width px` (PNG),
`-previous <old spec | old SVG | layout JSON, any of them as a path or REV:PATH>` (anchor a flow or class
layout so unchanged elements keep their places; a sequence render ignores
it with a warning), `-debug`.

Exit codes: 0 ok, 1 a validation error (JSON on stderr naming the `field`),
2 a usage or internal error. Advisories print on stderr as
`warning: <rule> <field>: <message>`; `cheatsheet.md` lists the common ones. Output is
deterministic: the same spec and theme give the same bytes.
