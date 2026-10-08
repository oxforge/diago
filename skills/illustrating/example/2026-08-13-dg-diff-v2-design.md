# dg diff v2 — union-layout diff, patch input, spec-anchored incremental renders

_A real design spec (for a sibling diagram tool, called dg here) used as the worked example for the `illustrating` skill's `documents.md`: every diagram is a diago spec in `2026-08-13-dg-diff-v2-design.diagrams/`, rendered and embedded by `scripts/diago-render`. `architecture` has two versions, so the document carries its revision diff._

Date: 2026-08-13. Status: approved design, pending implementation plan.

## Motivation

The driving use case: a service diagram spec lives in the repo (JSON or Mermaid);
as the service evolves, the user wants (1) to re-render the updated spec without
reshuffling the layout, so old and new renders are easy to compare, and (2) to
render the *change itself* — what was added, removed, changed.

Both workflows already exist (`dg render --previous`, `dg diff`). A road
test against the use case (small web-service diagram, one evolution step with
adds + a removal + a rename) confirmed they work and found these gaps, all in
scope for this design:

1. **Bug:** `dg render -f text` silently ignores `--previous` — the CLI's
   text branch never threads the layout options through.
2. **Ghost overlap:** `renderDiffSvg` overlays removed elements at their old
   positions on top of the *new* layout; a new node placed where a removed node
   used to be overlaps its ghost (observed in the road test).
3. **Text diff is second-class:** added nodes get a `+` prefix and a legend,
   but removed elements vanish from the drawing and changed nodes/added edges
   are unmarked.
4. **Shallow change detection:** node changes = label/shape only; edge changes
   (label, style, class) are not detected at all.
5. **No PNG output** for `dg diff`.
6. **`--previous` requires the rendered SVG**; users who think in specs want to
   anchor on the previous *spec* file.

Additionally, users may have only the old spec plus a textual diff of the spec
(e.g. from `git diff`) — `dg diff` should accept that directly.

## Changes since v1

- The ghost overlay path in the SVG renderer is gone; removed elements now
  occupy real space in a single union layout.
- `dg diff` accepts a unified diff as its second argument
  (`applyUnifiedDiff`), and gains PNG output via `@dg/png`.

<!-- diago:begin architecture.diff -->
```text
  ┌──────────────┐     ┌─ cli ──────────┐      ┌────────────────────────┐
  │ before spec  │     │                │      │ ~ after spec or patch  │
  └──────────────┘     │  ╭──────────╮  │      └────────────────────────┘
          │            │  │ dg diff  │  │                │      │
          │            │  ╰──────────╯  │                │      │
          │            └────────┼───────┘                │      │ if unified diff
          │                     │                        │      │
          │                     │                        │      │
          └───────────────┐     │     ┌──────────────────┘      │
                          │     │     │                ┌────────┘
                          │     │     │                │
        ┌─ core ──────────┼─────┼─────┼────────────────┼────────────┐
        │                 │     │     │                ▼            │
        │                 │     │     │     ┌────────────────────┐  │
        │                 │     │     │     │ + applyUnifiedDiff │  │
        │                 │     │     │     └────────────────────┘  │
        │                 │     │     │  new spec text │            │
        │                 │     │     │                │            │
        │                 │     └──┐  │  ┌─────────────┘            │
        │                 └───────┐│  │  │                          │
        │                         ▼▼  ▼  ▼                          │
        │                   ┌────────────────┐                      │
        │                   │ decodeDocument │                      │
        │                   └────────────────┘                      │
        │          reads old + new   │                              │
        │                            │ IR graphs                    │
        │                            ▼                              │
        │                   ┌────────────────┐                      │
        │                   │ diffGraphs v2  │                      │
        │                   └────────────────┘                      │
        │                            │                              │
        │                            │ GraphDiff                    │
        │                            ▼                              │
        │                  ┌──────────────────┐                     │
        │                  │ buildUnionGraph  │                     │
        │                  └──────────────────┘                     │
        │                            │                              │
        │                            │ union + status               │
        │                            ▼                              │
        │                ┌──────────────────────┐                   │
        │                │ layout (incremental) │                   │
        │                └──────────────────────┘                   │
        │                       │        │                          │
        │                       │        │                          │
        │           ┌───────────┘        └─────┐                    │
        │           ▼                          ▼                    │
        │  ┌────────────────┐         ┌────────────────┐            │
        │  │ renderDiffSvg  │         │ renderDiffText │            │
        │  └────────────────┘         └────────────────┘            │
        │       ╎      ╎ overlays removed                           │
        │       ╎      ╎                                            │
        │       ╎      └╌╌╌╌╌╌╌╌┐                                   │
        │       ╎               ▼                                   │
        │       ╎     ┌──────────────────┐                          │
        │       ╎     │ - ghost overlay  │                          │
        │       ╎     └──────────────────┘                          │
        └───────┼───────────────────────────────────────────────────┘
                ╎ rasterizes
                ╎
                ╎
     ┌─ + png ──┼─────────┐
     │          ▼         │
     │  ┌──────────────┐  │
     │  │ + renderPng  │  │
     │  └──────────────┘  │
     └────────────────────┘

architecture.v1.json → architecture.v2.json
added: node applyUnifiedDiff
added: node renderPng
added: edge after spec or patch -> applyUnifiedDiff
added: edge applyUnifiedDiff -> decodeDocument
added: edge renderDiffSvg -> renderPng
added: group png
removed: node ghost overlay
removed: edge renderDiffSvg -> ghost overlay
changed: node after spec or patch: label "after spec" → "after spec or patch"
```
[architecture.v1-v2.diff.png](2026-08-13-dg-diff-v2-design.diagrams/architecture.v1-v2.diff.png)
<!-- diago:end architecture.diff -->

## Design

### 1. Change detection (`diffGraphs` v2)

<!-- diago:begin data-model -->
```text
                         ┌──────────────────────────┐
                         │          Layout          │
                         ├──────────────────────────┤
                         │ nodes: PositionedNode[]  │
                         │ edges: PositionedEdge[]  │
                         └──────────────────────────┘
                                       ╎
                                       ╎ laid out from
                                       ▼
                       ┌──────────────────────────────┐
                       │          UnionGraph          │
                       ├──────────────────────────────┤
                       │ status: Map<Id, DiffStatus>  │
                       └──────────────────────────────┘
                   built from ╎        ◆ 1     │ per id
                              ╎        │       │
               ┌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┘        │       └────────────┐
               ▼                       │ 1                  │
  ┌────────────────────────┐     ┌──────────┐      ┌────────────────┐
  │       GraphDiff        │     │ IrGraph  │      │ «enumeration»  │
  ├────────────────────────┤     └──────────┘      │   DiffStatus   │
  │ added: Id[]            │                       ├────────────────┤
  │ removed: Id[]          │                       │ added          │
  │ changedEdges: EdgeId[] │                       │ removed        │
  └────────────────────────┘                       │ changed        │
                                                   │ unchanged      │
                                                   └────────────────┘

─── association
◆── composition
╌╌► dependency
```
[data-model.v1.png](2026-08-13-dg-diff-v2-design.diagrams/data-model.v1.png)
<!-- diago:end data-model -->

Identity is the stable id (nodes: explicit; edges: explicit or derived
`source->target#n`). `GraphDiff` gains `changedEdges: EdgeId[]`.

- **Node changed** when any of: label text, shape, `class`, or `parent`
  (container membership) differ.
- **Edge changed** when the same id differs in label text, `style`, or `class`.
- **Edge endpoints differ** (possible with explicit ids): treated as
  **removed + added**, not changed — a moved connection is visually a different
  connection, and the union layout needs one geometry per edge.

### 2. Union graph + layout

<!-- diago:begin architecture -->
```text
  ┌──────────────┐     ┌─ cli ──────────┐      ┌──────────────────────┐
  │ before spec  │     │                │      │ after spec or patch  │
  └──────────────┘     │  ╭──────────╮  │      └──────────────────────┘
          │            │  │ dg diff  │  │               │      │
          │            │  ╰──────────╯  │               │      │
          │            └────────┼───────┘               │      │ if unified diff
          │                     │                       │      │
          │                     │                       │      │
          └───────────────┐     │     ┌─────────────────┘      │
                          │     │     │               ┌────────┘
                          │     │     │               │
            ┌─ core ──────┼─────┼─────┼───────────────┼───────────┐
            │             │     │     │               ▼           │
            │             │     │     │     ┌──────────────────┐  │
            │             │     │     │     │ applyUnifiedDiff │  │
            │             │     │     │     └──────────────────┘  │
            │             │     │     │ new spec text │           │
            │             │     │     │               │           │
            │             │     └──┐  │  ┌────────────┘           │
            │             └───────┐│  │  │                        │
            │                     ▼▼  ▼  ▼                        │
            │               ┌────────────────┐                    │
            │               │ decodeDocument │                    │
            │               └────────────────┘                    │
            │      reads old + new   │                            │
            │                        │ IR graphs                  │
            │                        ▼                            │
            │               ┌────────────────┐                    │
            │               │ diffGraphs v2  │                    │
            │               └────────────────┘                    │
            │                        │                            │
            │                        │ GraphDiff                  │
            │                        ▼                            │
            │              ┌──────────────────┐                   │
            │              │ buildUnionGraph  │                   │
            │              └──────────────────┘                   │
            │                        │                            │
            │                        │ union + status             │
            │                        ▼                            │
            │            ┌──────────────────────┐                 │
            │            │ layout (incremental) │                 │
            │            └──────────────────────┘                 │
            │                   │        │                        │
            │                   │        │                        │
            │           ┌───────┘        └─────┐                  │
            │           ▼                      ▼                  │
            │  ┌────────────────┐     ┌────────────────┐          │
            │  │ renderDiffSvg  │     │ renderDiffText │          │
            │  └────────────────┘     └────────────────┘          │
            └───────────┼─────────────────────────────────────────┘
                        ╎
                        ╎ rasterizes
                        ╎
              ┌─ png ───┼────────┐
              │         ▼        │
              │  ┌────────────┐  │
              │  │ renderPng  │  │
              │  └────────────┘  │
              └──────────────────┘
```
[architecture.v2.png](2026-08-13-dg-diff-v2-design.diagrams/architecture.v2.png)
<!-- diago:end architecture -->

Build one IR graph: everything from *after*, plus the removed nodes/edges from
*before* (removed subtrees keep their structure and `ord`; for id collisions
the *after* version wins — those are changed/unchanged elements). Alongside it,
a status map: id → `added | removed | changed | same`, for nodes and edges.

Lay the union out **once** with the normal pipeline, `mode: 'incremental'`
anchored on the old spec's layout. Consequences:

- Unchanged elements stay near their old positions (comparable with the old
  render).
- Removed elements occupy honestly reserved space — ghost overlap becomes
  structurally impossible. The `ghostOverlay` code path and its canvas
  expansion are deleted.
- All output formats share one layout and one code path.

Presentation stripping (drop title, disable legend) is retained from v1: diff
renders show change, not presentation.

### 3. Rendering

All three formats consume the same union layout + status map.

- **SVG:** one `renderSvg` call using the existing `nodeClass`/`edgeClass`
  hooks. Classes: `added` (green, bold stroke), `changed` (amber — now applied
  to edges too), `removed` (dashed, ~35% opacity, as CSS — replaces the overlay
  markup), everything else `dimmed`.
- **Text:** label prefixes `+ ` (added), `- ` (removed), `~ ` (changed), plus
  the existing legend footer listing every change including edge changes
  (edges cannot carry marks in text art). Dashed borders for removed nodes are
  a stretch goal, not core.
- **PNG:** the CLI rasterizes the diff SVG via `@dg/png` (native dep
  stays out of core).

### 4. Patch input (unified diff)

<!-- diago:begin diff-run -->
```text
               ┌──────┐             ┌──────┐      ┌────────┐   ┌──────┐             ┌────────┐         ┌────────┐
               │ User │             │ cli  │      │ patch  │   │ core │             │ layout │         │ render │
               └──────┘             └──────┘      └────────┘   └──────┘             └────────┘         └────────┘
                   │                    │              │           │                     │                  │
                   │                    │              │           │                     │                  │
                   ├─── old + patch ────►              │           │                     │                  │
                   │                    ┃              │           │                     │                  │
                   │                    ┃───┐          │           │                     │                  │
                   │                    ┃   │ sniff kind           │                     │                  │
                   │                    ◄───┘          │           │                     │                  │
                   │                    ┃              │           │                     │                  │
  ╔═ alt ══════════╪ [patch applies] ═══╪══════════════╪════╗      │                     │                  │
  ║                │                    ├ apply patch ─►    ║      │                     │                  │
  ║                │                    │              ┃    ║      │                     │                  │
  ║                │                    ◄╌ new spec ╌╌╌┤    ║      │                     │                  │
  ║                │                    ┃              │    ║      │                     │                  │
  ╟╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╪ [hunk mismatch] ╌╌╌╪╌╌╌╌╌╌╌╌╌╌╌╌╌╌╪╌╌╌╌╢      │                     │                  │
  ║                │                    ◄╌ bad hunk ╌╌╌┤    ║      │                     │                  │
  ║                │                    ┃              │    ║      │                     │                  │
  ║                ◄╌╌ error, exit 1 ╌╌╌┤              │    ║      │                     │                  │
  ╚════════════════╪════════════════════╪══════════════╪════╝      │                     │                  │
                   │                    │              │           │                     │                  │
                   │                    ├── decode, diff, union ───►                     │                  │
                   │                    │              │           ┃                     │                  │
                   │                    │              │           ├ union, incremental ─►                  │
                   │                    │              │           │                     ┃                  │
                   │                    │              │           │                     ├ layout + status ─►
                   │                    │              │           │                     │                  ┃
                   │                    ◄╌╌╌╌╌╌╌╌╌╌╌╌╌╌┼╌╌╌╌╌╌╌╌╌╌╌ svg/text/png ╌╌╌╌╌╌╌╌┼╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌┤
                   │                    ┃              │           │                     │                  │
                   ◄ writes -o, exit 0 ╌┤              │           │                     │                  │
                   │                    │              │           │                     │                  │
               ┌──────┐             ┌──────┐      ┌────────┐   ┌──────┐             ┌────────┐         ┌────────┐
               │ User │             │ cli  │      │ patch  │   │ core │             │ layout │         │ render │
               └──────┘             └──────┘      └────────┘   └──────┘             └────────┘         └────────┘
```
[diff-run.v1.png](2026-08-13-dg-diff-v2-design.diagrams/diff-run.v1.png)
<!-- diago:end diff-run -->

`dg diff` accepts as its second argument either a new spec **or a unified
diff** of the spec file (`git diff` / `diff -u` output). Recognition: extension
`.patch`/`.diff`, or content starting with `diff `, `--- `, or `@@`; anything
else is treated as a spec.

The patch is applied to the old spec's raw text, yielding the new spec in
memory; decoding then proceeds exactly as the two-spec form. The applier is a
small pure function in `core` (zero-dep): strict unified-diff subset — hunks
must apply cleanly at their stated positions (allowing only the standard
context match), no fuzz; a hunk that does not apply is an error, never a guess.
Multi-file patches: apply only the hunks whose `---`/`+++` paths end with the
old spec's basename; if no file matches, error. A single-file patch applies
as-is regardless of its paths.

### 5. CLI surface

```sh
# diff: two specs (JSON or Mermaid, mixable) or spec + unified patch
dg diff <old> <new-or-patch> [-f svg|png|text] [-o out] [--theme light|dark]

# incremental render: --previous accepts a SPEC or a rendered SVG / layout JSON
dg render new.mmd --previous old.mmd  -o new.svg   # spec → fresh deterministic anchor
dg render new.mmd --previous old.svg  -o new.svg   # SVG → exact anchor from metadata
dg render new.mmd -f text --previous old.mmd       # NEW: works for text (see below)
```

- `--previous` input kind is sniffed the same way `readDocument` sniffs specs:
  SVG content (`<`-prefixed) → extract embedded layout; JSON containing both
  `hints` and `canvas` keys → layout JSON; otherwise parse as spec (JSON or
  Mermaid) and lay it out fresh. A spec anchor is exact for the first update; for chained updates the
  rendered SVG remains the exact carrier (replaying anchor chains from specs
  alone is out of scope here).
- `dg diff` takes **no** `--previous`: it always anchors on the old spec's
  own fresh layout — deterministic, no external state.
- PNG output is binary: `-f png` requires `-o` (same rule as `render`).

**Text `--previous` fix:** `renderText` gains an options parameter and threads
incremental mode through its grid-unit layout — feeding the pipeline **hints
only** (`LayoutHints`: layers, in-layer order, reversed edges — all unit-free),
never the pixel rects of a pixel-unit previous layout. This delivers the
no-reshuffle property (ordering stability is what prevents reshuffling) while
dodging the unit-corruption gotcha that has bitten twice before.

### 6. Error handling

- Identical inputs → valid render, empty legend, exit 0.
- `--previous` SVG without embedded layout → existing error message and exit 1.
- Patch that does not apply cleanly → error naming the first failing hunk,
  exit 1.
- Decode/parse failures → existing `DecodeError`/`MermaidParseError` path.

### 7. Testing (TDD, red first)

- **Detection:** changed edge (label/style/class), node class/parent change,
  endpoint move → removed + added; determinism with shuffled inputs.
- **Union layout property test:** no removed ghost intersects any other node —
  the exact bug this design kills.
- **Text:** prefixes and legend; text-incremental stability (add a node,
  unchanged rows keep their relative order); determinism fixtures.
- **Patch applier:** clean apply, context mismatch → error, multi-hunk,
  CRLF/no-trailing-newline edges.
- **CLI e2e under built Node artifacts:** `diff -f png`, `diff` with a patch
  argument, `render --previous <spec>`, `render -f text --previous`.
- **Corpus:** the evolving-service example set below; regenerate examples +
  gallery in the same commit (house rule).

### Examples corpus: evolving service

Three JSON specs of the same web service, committed under `examples/`, each
evolution step exercising a distinct slice of the diff feature:

- `service-v1.dg.json` — client, load balancer, API server, auth service,
  Postgres, mail sender (the road-test baseline).
- `service-v2.dg.json` — v1 plus: **added** Redis cache, job queue, email
  worker (+ their edges); **removed** mail sender (+ its edge); **changed**
  node (API server relabeled "API Gateway").
- `service-v3.dg.json` — v2 plus: **changed edge** (api→db gains a label and
  `dashed` style); **changed node class** (Postgres gains `danger` or similar);
  **endpoint move** (worker→db becomes worker→cache, rendering as removed +
  added); one **added** node (metrics) to keep growth visible.

Rendered artifacts (SVG + text each, via the examples generator, which learns
to invoke incremental and diff renders):

| artifact | invocation |
|---|---|
| `service-v1.{svg,txt}` | fresh render of v1 |
| `service-v2.{svg,txt}` | v2 with `--previous` v1 |
| `service-v3.{svg,txt}` | v3 with `--previous` v2 |
| `service-diff-v1-v2.{svg,txt}` | diff v1 → v2 |
| `service-diff-v2-v3.{svg,txt}` | diff v2 → v3 |

The gallery shows the set together, making the no-reshuffle property and the
diff rendering reviewable side by side. Note `service-v3` anchors on the
rendered v2 layout inside the generator (layout object passed directly), so
the chain is exact even though CLI spec-anchoring alone would drift — the
generator documents this distinction.

## Usage examples

```sh
# Day 0
dg render service.mmd -o service.svg

# Service evolved: update the render without reshuffling
dg render service.mmd --previous service-v1.mmd -o service.svg   # from old spec
dg render service.mmd --previous service.svg    -o service.svg   # or old render

# Show the change
dg diff service-v1.mmd service.mmd -o changes.svg
dg diff service-v1.mmd service.mmd -f text
dg diff service-v1.mmd service.mmd -f png -o changes.png

# Only have the old spec and a git patch of it
git diff HEAD~3 -- diagrams/service.mmd > service.patch
dg diff service-old.mmd service.patch -f png -o pr-diff.png
```

## Out of scope

- `--previous` on `dg diff` (ruled out 2026-08-13: diff is self-contained
  over specs).
- Replaying incremental anchor chains from specs / git revisions (a later
  change).
- A dg-specific structured changeset format (unified diff chosen instead).
- Side-by-side diff panes (union-in-place chosen instead).
- Dashed text-art borders for removed nodes (stretch goal only).
