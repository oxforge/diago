# Diagrams in documents for review

Specs, plans and design notes are long. Their structure (what talks to what,
which tasks depend on which) is what a reviewer needs first, and what a
revision changes. This makes that structure visible as diago diagrams in the
terminal and in the Markdown, and shows each revision as diff diagrams.

You do the judgement (what to draw, stable ids). The script does the
mechanics (render, lint, diff, embed). The rules of `SKILL.md` beside this
file apply; diago:diagramming says how to draw, and its `cheatsheet.md`
(`${CLAUDE_PLUGIN_ROOT}/skills/diagramming/cheatsheet.md`) has the JSON
shapes and enums.

## Where files go

For a document `<dir>/<name>.md` the diagram folder is `<dir>/<name>.diagrams/`.
Each diagram is `<slug>.v<N>.json` (slug: lowercase kebab-case), a diago spec.
The script writes `<slug>.v<N>.txt`, `<slug>.v<N>.svg` and, when a lower version
exists, `<slug>.v<P>-v<N>.diff.txt` / `.svg`.

Script: `${CLAUDE_PLUGIN_ROOT}/scripts/diago-render <diagrams-dir> [--embed <doc.md>] [--strict] [--png]`
(the session's start names its full path; without it, the script is
`../../scripts/diago-render` from this file's folder).

It prints one line per slug: `architecture v2 (diff v1->v2) width=87 diff-width=95 ok`.
Lint warnings arrive on stderr as `diago-render: lint <slug>.v<N>: <rule> <field>: …`.
`--png` writes and links PNG instead of SVG (needs `resvg`). Use it when the
diagrams folder is tracked by git (`git check-ignore -q <dir>` fails), since
each SVG embeds its font (~500 KB); a gitignored folder keeps the default SVG.
Without `resvg`, `--png` stops before rendering: run without it (SVG) and tell
the user that installing resvg (`brew install resvg` or `cargo install resvg`)
gives PNGs, which are far smaller to commit.

## What to draw

**Design spec.** Draw only rows whose condition holds.

| Spec content | Slug | Type | Condition |
|---|---|---|---|
| Components / packages and how they call each other | `architecture` | flow, groups per package | the spec names ≥ 3 components |
| A described runtime path (request, CLI run, pipeline) | a path slug, e.g. `diff-run` | sequence | one per *key* path, at most 2 |
| Types / records / interfaces the spec defines | `data-model` | class | ≥ 3 types with relations between them |
| Explicit states and transitions | `states` | flow, diamonds for guards | the spec has a state machine |

**Implementation plan.** Always exactly one diagram:

| Plan content | Slug | Type | Rule |
|---|---|---|---|
| `### Task N` headings + `**Interfaces:**` blocks | `tasks` | flow | node per task, id `t<N>` (`Task 1a` → `t1a`), label `T<N>` plus the task's name in 1–3 words; edge `t<M> → t<N>` when Task N's *Consumes* line names Task M (`(Task 2)`, `(Tasks 4–5)`: expand ranges) **or names a symbol that Task M's *Produces* line defines**, then drop every edge a longer path already implies (with `t1 → t3 → t4`, no `t1 → t4`): the reduction says the same order with far fewer crossings; a group per package when two or more tasks share one (by their `**Files:**` paths), labelled with the package's last path segment; without groups if the render exceeds 120 columns |

The symbol rule matters: plans often write "Consumes: `buildUnionGraph` from
`./union`" without a task number. Resolve each consumed symbol to the task whose
*Produces* line names it; when two tasks name it, the edge goes to both. A
package reference with no symbol resolves by the `**Files:**` paths. The plan
draws `tasks` even when it has fewer than five tasks; the 5-node minimum below
applies to spec diagrams only.

**Any other document for review** (a design note, a report, an RFC): what
`SKILL.md`'s *When a diagram earns its place* allows, at most 4 diagrams, each
slug named for what it shows.

No diagram for requirements lists, constraints, copy rules, or testing sections.

## Rules

- 5–15 nodes per diagram. Over 15: split into two slugs. Under 5: do not draw.
- At most 4 diagrams per document.
- Labels 1–3 words, group labels one short word (a long group label widens
  its frame, and with it the art: see terminal fit below); edge labels
  only when the connection needs explaining, and then a specific verb phrase
  (`validates input`, not `data`: the `vague-edge-label` advisory flags the
  latter).
- `color` only when it carries meaning: `purple` for the entry point,
  `orange` for something the document flags, never `green`, `blue` or `red`
  (a revision's diff draws added elements green, changed ones blue and
  removed ones red). Most elements uncolored.
- **Ids are stable**, as `SKILL.md`'s *Changes are diff diagrams* says: a
  renamed thing keeps its id, a replaced or split thing gets new ids, every flow edge and class relation has an explicit `id` (sequence
  interactions take none). This is what makes revision diffs meaningful;
  state it to yourself before editing a v<N+1>.
- Lint clean. Fix the JSON and re-run until stderr is empty. Add a rule to the
  spec's `ignore` list only when the finding is deliberate, never to silence it.
- Terminal fit: the summary prints `width=<cols>` and, for a revision,
  `diff-width=<cols>`; both count. Flow and class diagrams: over
  100, shorten labels or switch `direction` (`DOWN` narrows a long chain;
  `RIGHT` narrows a wide fan-out). Sequence diagrams are wide by nature (one
  column per actor): up to 120 is fine, above that drop an actor or split the
  path. Over 120 the script warns and you must act, whatever the type.
- Markers are matched anywhere in the document, including inside code fences.
  Do not put an example marker in a document that also has a real diagram with
  that slug.

Worked example: `${CLAUDE_PLUGIN_ROOT}/skills/illustrating/example/` (a real
spec with `architecture` v1→v2, `diff-run`, `data-model`).

## Protocol: new document

1. Write the document as you normally would, or as the skill that asked for it
   instructs.
2. Pick the diagrams from the tables. Author `<slug>.v1.json` files in
   `<name>.diagrams/`.
3. Run `diago-render <diagrams-dir>`. Fix lint warnings and widths. Re-run.
4. Put marker pairs in the document under the section each diagram illustrates
   (for plans: a `## Task graph` section after Global Constraints):

   ```markdown
   <!-- diago:begin architecture -->
   <!-- diago:end architecture -->
   ```

5. Run `diago-render <diagrams-dir> --embed <doc.md>`.
6. At the review gate, **print every diagram's text art in chat first**, each
   under a line naming its slug (in a graphical host, the image's path after
   its art), then the usual "written to `<path>`, please review" line, then:
   "Node ids are stable, so you can refer to a box by its id."

## Protocol: revision after feedback

1. Revise the markdown as usual.
2. For each diagram whose *structure* changed: copy `<slug>.v<N>.json` to
   `<slug>.v<N+1>.json` and edit it, keeping ids for things that are the same.
   Unchanged diagrams are not bumped.
3. Run `diago-render <diagrams-dir>` (fix lint), then `--embed`. A flow or class
   diagram's new version is laid out anchored on the old one, so unchanged
   boxes stay put.
4. Add or update a `## Changes since v<N>` section directly under the
   document's title block (before its first section): one line of prose per
   change, then a `<slug>.diff` marker pair per changed diagram:

   ```markdown
   <!-- diago:begin architecture.diff -->
   <!-- diago:end architecture.diff -->
   ```

   Re-run `--embed`.
5. At the review gate, **print each changed diagram's diff text art first**
   (it ends with diago's `added:` / `removed:` / `changed:` footer), each under a
   line naming its slug followed by `.diff`, then the file path, then the
   node-ids line. The new version's own art and unchanged diagrams are printed
   only if asked. The diff art omits the diagram `title`; the slug line names
   it.

## Bump discipline

One version per review round, never per edit. If you revise the same diagram
three times before the human sees it, that is still `v2`.

The `## Changes since v<N>` heading counts document review rounds. Individual
diagrams may sit at different versions (one was bumped in round 2, another not
until round 4); the `.diff` markers always show each diagram's latest diff, so
the heading number and the diagram versions need not match.
