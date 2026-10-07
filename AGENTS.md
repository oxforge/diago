# Diago

Instructions for people and coding agents working on this repository. Usage
documentation, the CLI reference and the JSON spec reference are in
`README.md`.

## What This Project Is

Diago is a command-line tool that renders diagrams from structured JSON specs,
plus a Claude Code plugin of skills that use it. The primary consumer is AI
agents (Claude Code, Codex, Cursor, etc.) that write a typed JSON spec
describing *what's connected to what*; diago handles layout and rendering and
writes SVG, PNG, Unicode text art, draw.io XML or Excalidraw JSON to stdout.

The core thesis: AI agents produce valid JSON far more reliably than valid DSL
syntax (Mermaid, D2, PlantUML). By accepting typed JSON per diagram type, diago
eliminates syntax errors and gives agents structured validation errors with a
field path. The rendering engine is opinionated: beautiful defaults, no
configuration required.

The repository has three parts:

1. **The CLI** (`cmd/diago`, `internal/`): `render`, `diff`, `check`, `import`.
2. **The README**: all usage documentation.
3. **The plugin** (`.claude-plugin/`, `skills/`, `scripts/`):
   `diago:diagramming` (writing specs and driving the CLI) and
   `diago:spec-diagrams` (diagram-augmented design specs and plans with
   revision diffs). `scripts/diago-render` does the render/lint/diff/embed
   mechanics; `skills/spec-diagrams/example/` is the worked example,
   regenerated with `scripts/diago-render <dir> --png --strict --embed <doc>`.

## Tech Stack

- **Go 1.26+**. Dependencies: `golang.org/x/image` (font metrics), `github.com/invopop/jsonschema` (generating `schemas/*.json` in `internal/schema/jsonschema.go`) and testify for tests; nothing else, and no runtime service.
- **Layout engines**: the layered engine (`internal/layout/layered/`, a pure Go port of neat's Sugiyama pipeline) for flow and class diagrams; a custom timeline engine (`internal/layout/sequence/`) for sequence diagrams. See *Where layout code lives* below.
- **SVG rendering**: Go-native SVG generation (structured types, no string concatenation).
- **PNG export**: resvg subprocess rasterizing the SVG (`internal/render/png`).
- **Export formats**: draw.io XML and Excalidraw JSON (`internal/export/`), flow diagrams only, derived from the laid-out model, never from the SVG.
- **Font metrics**: embedded TTF files (Inter, Pangolin) + `golang.org/x/image/font/sfnt` (no system font dependency).
- **Themes**: JSON-defined theme files (colors, typography, spacing, strokes), embedded.

## Key Commands

- `just`: list every recipe with its description
- `just build`: build `bin/diago`
- `just install`: `go install ./cmd/diago`
- `just test`: run all Go unit tests
- `just lint`: run golangci-lint
- `just schemas`: regenerate `schemas/{flow,sequence,class}.json` from the Go spec structs. **Required after any change to `internal/schema/`**

## Project Structure

```
diago/
├── cmd/
│   ├── diago/            # CLI entrypoint (verbs: render, diff, check, import; every verb also accepts a Mermaid source by .mmd extension or header; stdin or a spec path → stdout; advisories and import reports on stderr)
│   └── generate-enums/   # Codegen for internal/model/enums_gen.go ("DO NOT EDIT": change the generator, not the output)
├── internal/
│   ├── schema/           # JSON schema definitions and validation per diagram type (flow, sequence, class), plus the advisory checker (schema.Check, the ignore list, rule constants)
│   ├── layout/
│   │   ├── contract/     # Output-contract checker: rules C2–C17 on any PositionedGraph, screen or text profile; imports no engine
│   │   ├── metrics/      # Quality metrics (Q1–Q5) and defect counts of any PositionedGraph, and the churn of an anchored layout against its previous one (Q6, `churn.go`); imports no engine
│   │   ├── layered/      # The layered engine, the only flow and class engine: one package per stage (size, cycle, rank, equalize, lgraph, order, ports, place, route, nest, flat, labels, frame)
│   │   └── sequence/     # Custom timeline layout; `LayoutWithOptions` takes an activation mask for diff renders
│   ├── layoutdbg/        # Decision-record logging (layoutdbg.go): extend this, never fmt.Printf
│   ├── diff/             # Structural diff and union graphs for flow, sequence and class specs, text stamping and legends, member-level class union (`members.go`), strict unified-diff applier; depends on model only
│   ├── mermaid/          # Mermaid importer: Sniff/Parse, flowchart, sequence and class dialects → schema spec structs; depends on schema only
│   ├── render/
│   │   ├── svg/          # SVG rendering (flow.go, sequence.go, class.go, elements, shapes, text, sketch, arrows, hops, diffstyle, metadata)
│   │   ├── png/          # SVG→PNG rasterization via resvg subprocess
│   │   └── text/         # Unicode box-drawing text art renderer (flag-based char grid; flow, sequence, class; laid out under a text profile, always orthogonal)
│   ├── export/
│   │   ├── drawio/       # draw.io XML export
│   │   └── excalidraw/   # Excalidraw JSON export
│   ├── theme/            # Theme loading, validation, conversion
│   ├── model/            # Internal graph model (nodes, edges, groups, positioned variants) + enums_gen.go
│   ├── font/             # Embedded Inter + Pangolin fonts and text measurement
│   └── pipeline/         # Orchestration: pipeline.go (RenderWithOptions, every format), diff.go (RenderDiff), previous.go (-previous parsing), advisories.go, engine.go (`layoutWith`: the layered engine under the text Config or a theme's screen Config)
├── schemas/              # Generated JSON schema files
├── themes/               # Built-in theme JSON files (default, dark, midnight, sketch)
├── .claude-plugin/       # plugin.json + marketplace.json (the plugin is the repo root)
├── skills/               # diagramming/ and spec-diagrams/ (SKILL.md + cheatsheet.md; spec-diagrams/example/ is its worked example)
└── scripts/diago-render  # Render/lint/diff/embed a document's diagrams (used by spec-diagrams)
```

Skill changes ship through the plugin cache: bump `version` in both
`.claude-plugin/plugin.json` and `marketplace.json` whenever a skill, the
script or the cheatsheet changes, or installed copies never see it.

## Themes

Themes are JSON files in `themes/`. Each theme defines a complete visual
system. When authoring one, **copy a real file such as `themes/midnight.json`**
and check the structs in `internal/theme/theme.go`; unknown keys are silently
ignored, so a typo degrades rather than errors.

Two things `internal/theme/validate.go` **rejects**: a `style` other than
`clean` or `sketch` (the field is required: it selects the sketch renderer),
and a font family other than `Inter` or `Pangolin`, the only two embedded
faces. `Validate` also requires the `class` section (member font from the two
embedded faces, positive separator width).

An optional `"diff": {"added", "changed"}` block sets the diff palette; absent, it falls back to `colors.green` / `colors.orange`.

**Built-in themes:** `default` (light), `dark` (dark neutral), `midnight` (dark vibrant), `sketch` (hand-drawn feel)

**Choosing one:** every diagram type takes an optional top-level `theme`
field; `-theme` overrides it, else `default` (`pipeline.RenderTheme`).
`diago diff` reads the new spec's field, and a `-previous` spec is laid out
under the render's theme, never its own field. Text art takes no theme. A
name the spec gives that does not load is a validation error on `theme`;
an unknown `-theme` stays an invocation error.

Themes must be self-contained JSON: no inheritance, no imports, no overrides
between themes. Each theme is a complete standalone definition.

## Code Conventions

- **Go**: stdlib first; add a dependency only with a reason. Use `slog` for structured logging. Use `encoding/json` for schema handling. Wrap errors with `fmt.Errorf("operation: %w", err)`. Table-driven tests. Never use `panic`; the sole exception is a package-init assertion that an embedded asset is intact (`font/metrics.go`, `theme/theme.go`), where a failure means a corrupt binary and there is no caller to return an error to.
- **SVG**: Generate SVG as structured Go types, then serialize to XML. Do not use string concatenation to build SVG. Each visual primitive (rect, text, path, line, group) should be a Go struct with a `Render() string` method. Status-aware renders pass `svg.RenderOptions`; nil is the plain render and must stay byte-identical.
- **Naming**: Packages are short, lowercase, single-word where possible (`layout`, `render`, `theme`, `model`, `schema`).
- **Config**: Env vars with `DIAGO_` prefix. Only two exist: `DIAGO_RESVG_PATH` (override the `PATH` lookup for the resvg binary) and `DIAGO_THEME_DIR` (fallback directory for theme names not in the embedded set; built-ins always win).
- **CLI exit codes**: 0 ok, 1 validation error (structured JSON on stderr with a `field`), 2 usage or internal error. Advisories go to stderr as `warning: <rule> <field>: <message>`; findings are returned, never logged.
- **Testing**: Every package needs `_test.go`. A layout or routing rule is tested on the JSON layout (positioned nodes, edges, groups, ports), never on rendered SVG bytes; the tests live beside the implementation in `internal/layout/.../*_test.go`. An output-contract rule changes `internal/layout/contract` and its tests in the same commit.
- **Probe**: the random-graph probe (`internal/layout/layered/probe_test.go`, `TestProbe_Contract`, `TestProbe_Anchor`, `TestProbe_Dump`) lays out random flow graphs and counts their contract violations by rule; it runs only with `-probe` (`go test ./internal/layout/layered/ -run TestProbe_Contract -probe -v`; the flags are listed at the top of the file). It is a dev tool for finding layout and routing gaps, not a gate.

## Architecture Rules

- Rendering must be fully deterministic. Same spec + same theme = same SVG byte-for-byte. No random IDs, no timestamps in output. Layout determinism (C1) is guarded by the tests in `internal/layout/layered/determinism_test.go` (50 in-process runs, JSON layouts compared); extend their cases with every new tie-break. `scripts/diago-render` relies on this: identical inputs give byte-identical artifacts.
- Layout anchoring (C18, S13) is `render -previous` and `diago diff`, which anchors the union on the old layout; the carrier rides on `PositionedGraph.LayoutHints` and in every flow and class SVG's `<metadata id="diago-layout">`. The layered engine reads and writes it; its guarantees are checked per fixture in `internal/layout/layered/anchor_test.go`.
- Layout and rendering are separate stages. The layout engine produces positioned coordinates. The renderer takes positions and theme and produces SVG. They do not know about each other.
- Font metrics must be computed without system font dependencies. TTF files for the supported faces (**Inter** and **Pangolin**, the only two, whitelisted in `theme/validate.go`) are embedded, and `golang.org/x/image/font/sfnt` computes metrics, so text sizing is identical everywhere. Adding a face means embedding the TTF *and* extending the whitelist.
- SVG output must be valid, well-formed, and render correctly in browsers, GitHub markdown, and Figma import.
- PNG rasterization is a post-processing step on the SVG. The SVG is the canonical output.
- Export formats (draw.io, Excalidraw) are derived from the internal graph model, not from the SVG. The exporter receives the laid-out model and converts it to the target format's schema.

## Settled Design Decisions

These were decided with evidence and are not open questions. Do not re-propose
the rejected alternative without new information that invalidates the reason.

- **CLI only.** No server, MCP server, persistence or accounts.
- **resvg over headless Chromium for PNG.** An audit of what diago actually
  emits (basic shapes, plain `<text>`, embedded `@font-face`, simple
  `<marker>`s, one `feTurbulence` filter) found all of it natively supported by
  resvg. resvg is ~10–20× faster per render and needs no browser management.
  Pure-Go rasterizers (`oksvg` + `rasterx`) were rejected in the same audit: no
  `@font-face`, no `<marker>`, no `<style>` blocks, no filters.
- **The layered engine is the only flow and class engine**: neat's design,
  global placement and routing inside the layered structure. An elkjs
  fallback (a Node.js subprocess) was removed: it was flaky under golden
  comparison and kept the stack from being pure Go.
- **Font metrics are embedded, never system-derived and never hardcoded width
  tables**: the same spec must produce byte-identical SVG on every machine.
- **SVG is built from structured Go types with `Render() string`**, never string
  concatenation: it is what makes the output verifiable and diffable.
- **Debug decision records go to the local CLI's stderr only** (`-debug`), never to a file or a network sink by default.

## Debugging layout: `--debug`, never `fmt.Printf`

When debugging a layout or routing bug, use `--debug` (CLI) to emit JSON Lines
decision records for every layout/routing decision. The schema, the phase list
and the `jq` recipes are in `README.md` → *Debug logging*; a record's `phase`
names the stage that emitted it, and `spec_ref` the rule id it followed.

Do not add ad-hoc `fmt.Printf` instrumentation. If a bug is hard to
investigate, the existing debug logging doesn't cover that decision yet:
extend the `internal/layoutdbg.Decision` call sites at the spot you need
visibility, with the same gating as the rest, then turn on `--debug` to see
the new decision appear.

## Where layout code lives

Flow and class layout lives in `internal/layout/layered/`; sequence layout in
`internal/layout/sequence/`. Beside them: `internal/layout/contract/` (the
output-contract checker) and `internal/layout/metrics/` (the quality metrics).

| Package | Owns |
|---------|------|
| `internal/layout/layered/` | The layered engine, the only flow and class engine: one package per stage, the Config (S14) in `config.go`. `Layout` runs S1–S11, laying groups out level by level (`nested.go`, the `nest` package: S9), routing the flat edges on the composed layout with their fallback (`flat.go`, the `flat` package: S9), copying congruent sibling groups (`congruent.go`: S12) and anchoring on a previous carrier (`anchor.go`: S13), `Arrange` S1–S8 on a graph without groups or flat edges; the pipeline calls it from `internal/pipeline/engine.go`. Debug records carry `module: diago` and the stage's name as `phase`. |
| `internal/layout/sequence/` | The timeline engine for sequence diagrams; `LayoutWithOptions` takes an activation mask for diff renders. Sequence diagrams emit no debug records. |

Put a change where its stage already lives; the debug `phase` name tells you
which package emitted a decision.
