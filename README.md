# Diago

[![CI](https://github.com/oxforge/diago/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/oxforge/diago/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/oxforge/diago)](https://github.com/oxforge/diago/releases/latest)
[![License: MIT](https://img.shields.io/github/license/oxforge/diago)](LICENSE)

Diago renders diagrams from typed JSON specs. Describe *what's connected to
what*; diago lays it out and draws it as SVG, PNG, Unicode text art, draw.io XML
or Excalidraw JSON (the last two in beta), and it renders Mermaid files as they
are. It is built for AI agents as much as for people: agents write valid JSON
far more reliably than a diagram DSL, every spec has a JSON Schema, and the
same spec always renders to the same bytes.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="https://diago.dev/readme-dark.png">
  <img alt="An architecture diagram drawn by diago" src="https://diago.dev/readme-light.png">
</picture>

<img alt="A terminal session: cat prints a small flow spec, diago render -format text draws it as text art, vim adds a Redis node and an edge from the API to it, and diago diff -format text draws both versions as one diagram with the new node marked +" src="https://diago.dev/demo.gif">

`diago diff old.json new.json` draws two versions of a spec as one picture:
added elements in green, changed ones in blue, removed ones in red and struck
through, with every change listed under the picture. In flow and class
diagrams the unchanged boxes stay close to where they were
([this example on diago.dev](https://diago.dev/examples/service-change/)).

Every diagram and every diff also draws as Unicode text art, to read in a
terminal, a code block or a pull request.

**Documentation** at [diago.dev](https://diago.dev):
[getting started](https://diago.dev/docs/),
[guides](https://diago.dev/docs/guides/flow/),
[agents](https://diago.dev/docs/agents/claude-code/),
[reference](https://diago.dev/docs/reference/cli/) and the
[gallery](https://diago.dev/examples/).

## Install

On macOS or Linux:

```bash
curl -fsSL https://diago.dev/install.sh | sh
```

On Windows, in PowerShell:

```powershell
irm https://diago.dev/install.ps1 | iex
```

The script downloads the latest release for your system, checks it against the
release's `checksums.txt` and puts `diago` in `~/.local/bin` (on Windows it
also adds that folder to your `Path`); `DIAGO_INSTALL_DIR` changes the folder.
[Getting started](https://diago.dev/docs/#install) has the details and a
manual download. Or, with Go 1.26+:

```bash
go install github.com/oxforge/diago/cmd/diago@latest
```

diago is at 0.x: a release may reject a spec the previous one accepted, change
an exit code or change what a render draws, and its release notes list those
breaking changes first. To stay on one version, give the script that version
(`curl -fsSL https://diago.dev/install.sh | DIAGO_VERSION=vX.Y.Z sh`), or
`go install …@vX.Y.Z`.

PNG output also needs [resvg](https://github.com/linebender/resvg) 0.26 or
newer (`brew install resvg` or `cargo install resvg`); every other format is
pure Go.

## Usage

```bash
cat > hello.json <<'EOF'
{
  "type": "flow",
  "nodes": [
    {"id": "a", "label": "Hello", "shape": "rounded"},
    {"id": "b", "label": "World"}
  ],
  "edges": [{"id": "a-b", "from": "a", "to": "b"}]
}
EOF

diago render hello.json > hello.svg             # or -format png, text, drawio, excalidraw
diago render -format text hello.json            # Unicode text art, in the terminal
diago check -strict hello.json                  # advisories, without rendering
diago diff hello.json hello-v2.json > diff.svg  # what changed between two versions
diago import diagram.mmd > spec.json            # from Mermaid
```

`diago help <verb>` prints a verb's flags; the
[CLI reference](https://diago.dev/docs/reference/cli/) covers them all.

## For agents

This repository is also a Claude Code plugin. Its two skills teach an agent to
draw with diago and to show a diagram wherever it presents structure, and its
hook adds that rule to every session. With `diago` on `PATH`:

```bash
claude plugin marketplace add oxforge/diago
claude plugin install diago@diago
```

The plugin follows diago's releases; see
[updating it](https://diago.dev/docs/agents/claude-code/). Any other agent with
a shell: [diago.dev/docs/agents/other-agents](https://diago.dev/docs/agents/other-agents/).

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
[debug logging](https://diago.dev/docs/reference/debug-logging/).

## License

MIT, see `LICENSE`.
