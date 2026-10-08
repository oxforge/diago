# Diago

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

**Documentation** at [diago.dev](https://diago.dev):
[getting started](https://diago.dev/docs/),
[guides](https://diago.dev/docs/guides/flow/),
[agents](https://diago.dev/docs/agents/claude-code/),
[reference](https://diago.dev/docs/reference/cli/) and the
[gallery](https://diago.dev/examples/).

## Install

Download the archive for your platform from the
[latest release](https://github.com/oxforge/diago/releases/latest) (Linux,
macOS and Windows, on amd64 and arm64) and put `diago` on `PATH`. On macOS, a
binary downloaded with a browser is quarantined; clear it with
`xattr -d com.apple.quarantine diago`. Or, with Go 1.26+:

```bash
go install github.com/oxforge/diago/cmd/diago@latest
```

PNG output also needs [resvg](https://github.com/linebender/resvg)
(`brew install resvg` or `cargo install resvg`); every other format is pure Go.

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
