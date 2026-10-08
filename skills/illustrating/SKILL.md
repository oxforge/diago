---
name: illustrating
description: Use before presenting the user anything with structure, such as how code or a system works, a design or the options for one, a debugging finding, a plan's task order, or a change to code, architecture or business logic (one you made or propose, or a commit, branch or diff you summarize); and when writing or revising a document for human review, such as a design spec, an implementation plan or a design note
---

# Illustrating with diago

When you show a person something with structure, a diagram is often the
fastest way in: what connects to what, what happens in which order, what
changed. This skill decides when a diagram earns its place and in which
format the person's medium can show it. How to draw a good one is
diago:diagramming: read it before your first spec in a session.

## When a diagram earns its place

The moments: explaining how existing code works; proposing a design; asking
the user to choose between approaches; reporting a debugging root cause;
summarizing a change you made or propose; presenting a plan's task order;
writing a document for review.

Draw when the content is:

- connections among 3 or more things;
- a path through 3 or more parties or steps, especially one with branches;
- an order or a set of dependencies;
- a type model with relations;
- a state machine;
- a before/after change to any of these (*Changes are diff diagrams* below).

A tell: when you catch yourself writing an arrow chain in prose
(`A → B → C`) or walking the reader through components one numbered
section at a time, draw it instead.

Do not draw lists, single facts, numbers (use a table), two-step sequences,
or code-level detail that reads better as code.

**Budget:** at most 2 diagrams per chat reply, 3 to 12 nodes each (a document's
review gate is exempt: `documents.md` prints every diagram the document has). The
diagram replaces prose rather than repeating it: one sentence of takeaway,
the diagram, then only what the diagram cannot show. Draw only what you
verified (code you read, commands you ran), and say so when a part is
inferred.

## Format by medium

| The user reads it in | Format |
|---|---|
| A chat reply in a terminal | Text art in a fenced `text` block, at most 100 columns |
| A chat reply in a graphical host (a desktop app, an IDE, the web) or an unknown one | The same text art, then the SVG file's path on its own line: clicking it opens the colored diagram |
| A Markdown file tracked by git | A PNG beside it (`-format png`; SVG when `resvg` is missing) with alt text, and its spec JSON beside it so it can be regenerated |
| A Markdown file written for review (a spec, a plan, a design note) | `documents.md` beside this file: text art and an image link, versions, diffs |
| An HTML page or artifact | SVG files beside the page, shown with `<img>`; for a page that follows light and dark mode, a `<picture>` with a `prefers-color-scheme: dark` source for the `dark` theme. When the page must be one file, use `data:image/svg+xml;base64,` sources. Never paste SVG markup inline: diagrams share ids, and a diff's styles would apply to the whole page |
| A diagram the user will edit by hand | `-format drawio` (diagrams.net) or `-format excalidraw`, flow only, in beta: say so, and check the file |

The session's start says whether you run in a terminal; when nothing says,
treat the host as graphical.

## Diagrams in chat

1. Pick a topic folder `${TMPDIR:-/tmp}/diago/<topic>/`, `<topic>` a short
   kebab-case name for the subject. A new subject gets a new folder; if the
   folder already holds specs you did not write in this conversation, pick
   another name. Write each diagram as `<slug>.v1.json` there.
2. Run `diago-render <topic-dir>` (`${CLAUDE_PLUGIN_ROOT}/scripts/diago-render`;
   the session's start names its full path; without it, the script is
   `../../scripts/diago-render` from this file's folder). It renders each slug's latest
   version to `.txt` and `.svg`, lints it (warnings on stderr as
   `diago-render: lint …`) and prints `<slug> v<N> width=<cols> ok`. Fix every
   lint warning, keep the width at most 100 (diago:diagramming's direction
   table says how), and re-run.
3. Copy the `.txt` art into your reply in a fenced `text` block: what a tool
   prints is not reliably shown to the user. In a graphical host, put the
   `.svg` path on the line after it.
4. When the user asks to change a diagram they have seen, copy
   `<slug>.v<N>.json` to `<slug>.v<N+1>.json`, edit it keeping the id of
   everything that is the same, and re-run: a flow or class diagram's new
   version is laid out anchored on the old one (a sequence diagram follows
   its message order anyway), and `<slug>.v<N>-v<N+1>.diff.txt` shows what
   changed. Show the diff art; its `added:` / `removed:` / `changed:` footer
   is the change list.

## Changes are diff diagrams

When you changed, or propose to change, structure in code, architecture or
business logic, show it as a diff diagram: the before and the after in one
drawing, each element marked added, removed or changed, the rest pinned in
place.

**When:** after an implementation that adds, removes or rewires components or
modules, reroutes a call path, adds a decision branch, or changes types or
relations; when proposing such a change; when summarizing a commit, a branch
or a PR; when walking the user through someone else's diff. Not for a change
inside one function's body that alters nothing structural.

| The change | Type |
|---|---|
| Modules, services, packages, dependencies | flow |
| A request, protocol or call sequence | sequence |
| Types, fields, relations | class |
| Business rules, branches, states | flow, diamonds with labeled exits |

**How:**

1. Both sides come from the code, never from memory: v1 from the base
   (`git show <base>:<file>`, `git diff <base>`, or the files as they were
   before your edit), v2 from the result. For a proposal, v1 is the current
   code and v2 the proposal. For a commit you are asked about, v1 is the code
   at `<commit>^` and v2 at `<commit>`.
2. Scope: the changed elements and their direct neighbors, not the whole
   system.
3. Write `<slug>.v1.json`, then copy it to `<slug>.v2.json` and edit it.
   Everything that is the same on both sides keeps its id (nodes, edges,
   groups), so the diff marks only real changes: a renamed thing keeps its id
   (`changed`), a replaced thing gets a new id (`removed` and `added`), a
   thing split in two is replaced. Give every flow edge and class relation an explicit `id` (sequence
   interactions take none), so a relabeled edge is `changed`, not replaced. Keep v1's `type` and
   `direction` (`diago diff` refuses a type change, and a new direction lays
   everything out again), and use no `green`, `blue` or `red`: the diff draws
   added elements green, changed ones blue and removed ones red.
4. Run `diago-render <topic-dir>` as in *Diagrams in chat*. It writes
   `<slug>.v2.txt` and the diff `<slug>.v1-v2.diff.txt` (and their `.svg`).

**A spec tracked by git.** When the diagram's spec is itself a file in the
repository (`docs/arch.json`), do not write v1 and v2 by hand:
`diago diff docs/arch.json -format text` draws its latest change (uncommitted
edits against HEAD, or what its last commit did), and
`diago diff <base>:docs/arch.json docs/arch.json` draws it against a base.

**Showing it:** one line on what changed, the diff art
(`<slug>.v1-v2.diff.txt`) in a fenced `text` block, and in a graphical host
the diff SVG's path, where additions are green, changes blue and removals red, with the change list under the drawing. When most
elements changed, a diff is noise: show the v2 art alone and say it is a
rewrite.

## Documents for review

When you write or revise a document the user will review (a design spec, an
implementation plan, a design note, a report), follow `documents.md` beside
this file: which diagrams a spec or plan gets, how they are embedded, and how
each review round shows its changes as diff diagrams.

## Rules for every case

- Lint clean before the user sees anything: fix the JSON until
  `diago-render` prints no lint line. Add a rule to a spec's `ignore` list
  only when the finding is deliberate.
- One version per round the user sees, never per edit: three edits before
  the user looks are still one version.
- No diago, no diagram: if `command -v diago` fails, tell the user once that
  `go install github.com/oxforge/diago/cmd/diago@latest` enables diagrams.
  Never fall back to Mermaid or hand-drawn ASCII.
