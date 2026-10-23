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
2. Check and render it with `diago` itself:

   ```sh
   cd "${TMPDIR:-/tmp}/diago/<topic>"
   diago check -strict <slug>.v1.json
   diago render -format text <slug>.v1.json > <slug>.v1.txt
   diago render <slug>.v1.json > <slug>.v1.svg   # a graphical host only
   ```

   Fix every warning `check` or `render` prints, keep the art at most 100
   columns wide (diago:diagramming's direction table says how), and re-run.
3. Copy the `.txt` art into your reply in a fenced `text` block: what a tool
   prints is not reliably shown to the user. In a graphical host, put the
   `.svg` path on the line after it.
4. When the user asks to change a diagram they have seen, that is a change:
   follow *Changes are diff diagrams* from its step 2, with
   `<slug>.v<N>.json` as the before state and `<slug>.v<N+1>.json` as the
   after.

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

**How.** There is one way to draw a change, the same for a change you made,
one you propose, a commit, a branch, a PR or someone else's diff:

1. **v1 is the before state, from the code**, never from memory: the files
   as they were before your edit, `git show <base>:<file>` and
   `git diff <base>` for a branch, the code at `<commit>^` for a commit, the
   current code for a proposal. Scope it to the changed elements and their
   direct neighbors. Write it as `<slug>.v1.json` in the topic folder
   (*Diagrams in chat*), give every flow edge and class relation an explicit
   `id` (sequence interactions take none), and run
   `diago check -strict <slug>.v1.json` until it passes.
2. **v2 starts as a copy of v1:** `cp <slug>.v1.json <slug>.v2.json`.
3. **Edit v2 in place, one change at a time** (the Edit tool, or `jq`): add
   what the change adds, delete what it removes, edit the fields of what it
   changes. Leave everything else as it is, ids included: a renamed thing
   keeps its id (`changed`), a replaced thing gets a new id (`removed` and
   `added`), a thing split in two is replaced. Keep v1's `type` and
   `direction`: `diago diff` refuses a type change, and a new direction
   lays everything out again.
4. **Check v2 and draw the diff:**

   ```sh
   diago check -strict <slug>.v2.json
   diago diff <slug>.v1.json <slug>.v2.json -format text > <slug>.v1-v2.diff.txt
   diago diff <slug>.v1.json <slug>.v2.json > <slug>.v1-v2.diff.svg   # a graphical host only
   ```

5. **Read what it printed.** An `id-drift` warning means v2 gave something
   from v1 a new id, so the diff draws it twice, once removed and once
   added: give it back the old id the warning names, in v2, and draw the
   diff again. Fix any other warning on an `after.` field in v2. Then read
   the change list at the bottom of the `.txt`: it lists the changes you
   meant, and only those.
6. **Show it:** one line on what changed, the diff art in a fenced `text`
   block, and in a graphical host the diff SVG's path, where additions are
   green, changes blue and removals red, with the change list under the
   drawing. Always the diff, however much changed; the one exception is a
   rewrite: when more than half of v1's nodes and edges appear as `removed:`
   in the change list (changed ones do not count) and the diff printed no
   `id-drift`, render v2 alone, show that, and say it is a rewrite.

**Never draw a change any other way:**

| The shortcut | What goes wrong |
|---|---|
| Writing v2 yourself, from scratch or together with v1, because you already know both states | The ids drift, and the diff draws two copies of the diagram side by side. v2 is always a `cp` of v1, then edits. |
| One spec with the change marked in it: `(new)` or `(removed)` in labels, `green` or `red` on the new parts, a `status` key, a legend node | Only `diago diff` marks a change. Your own marks are not the diff's, text art drops color, and diago ignores a key it does not know. A spec you diff uses no `green`, `blue` or `red`: they are the diff's colors. |
| The diff looks broken, so you draw a single diagram instead | A diff that draws the same things twice has drifted ids: step 5 fixes it. |

**A spec tracked by git.** When the diagram's spec is itself a file in the
repository (`docs/arch.json`), do not write v1 and v2 by hand:
`diago diff docs/arch.json -format text` draws its latest change (uncommitted
edits against HEAD, or what its last commit did), and
`diago diff <base>:docs/arch.json docs/arch.json` draws it against a base.

## Documents for review

When you write or revise a document the user will review (a design spec, an
implementation plan, a design note, a report), follow `documents.md` beside
this file: which diagrams a spec or plan gets, how they are embedded, and how
each review round shows its changes as diff diagrams. Its script is
`${CLAUDE_PLUGIN_ROOT}/scripts/diago-render`: a file in this plugin, not a
command on `PATH`, so always run it by that full path.

## Rules for every case

- Lint clean before the user sees anything: fix the JSON until
  `diago check -strict` passes and nothing prints a warning. Add a rule to a
  spec's `ignore` list only when the finding is deliberate.
- One version per round the user sees, never per edit: three edits before
  the user looks are still one version.
- No diago, no diagram: if `command -v diago` fails, tell the user once that
  installing diago (https://diago.dev/docs/#install) enables diagrams.
  Never fall back to Mermaid or hand-drawn ASCII.
