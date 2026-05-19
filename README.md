# CheatMD

Executable Markdown cheatsheets. Write readable docs, run interactive commands.

![demo](assets/demo.gif)

## Install

```bash
go install github.com/gubarz/cheatmd/cmd/cheatmd@latest
```

## Quick Start

```bash
cheatmd                    # Browse current directory
cheatmd ~/cheats           # Browse specific directory
cheatmd -q "docker"        # Start with search query
cheatmd --history          # Re-run from execution history
```

## Features

### Write cheats as Markdown

Any heading with a code block is a cheat. Variables like `$container` become
interactive prompts powered by shell commands:

````markdown
## Docker: exec into container

```sh title:"Execute shell in container"
docker exec -it $container /bin/sh
```
<!-- cheat
var container = docker ps --format "{{.Names}}" --- --header "Select container"
-->
````

### Fuzzy search with frecency

Type to filter across titles, commands, descriptions, and tags. Cheats you
run often and recently float to the top.

### Variables from shell output

Variable values come from shell command output - 0 lines gives a text prompt,
1 line pre-fills, 2+ lines give a filterable picker.

### Reusable modules

Export a variable definition once, import it in any cheat:

```text
<!-- cheat
export docker_container
var container = docker ps --format "{{.Names}}"
-->
```

### Chains

Multi-step workflows that advance one step per launch:

```text
/chain release     <- search chains in the picker
```

### Tags

Cheats are tagged automatically from folder paths, YAML front matter, footer
blocks, inline `#hashtags`, and heading text - all searchable from the picker.

### Shell integration

Embed CheatMD directly into your shell prompt, tmux, or Zellij:

```bash
eval "$(cheatmd widget bash)"   # Ctrl+G opens the selector
```

### Linting

Validate DSL syntax, imports, chains, and undeclared variables:

```bash
cheatmd --lint ~/cheats
```

Language-aware: shell builtins like `$HOME` and PowerShell `$true` won't
trigger false positives.

### Dump

Export cheat metadata as JSON or CSV for indexing and tooling:

```bash
cheatmd dump ~/cheats --json
```

## TUI Keys

| Key | Action |
|-----|--------|
| `Ctrl-H` | Execution history |
| `Ctrl-T` | Substitute search (env + shell history) |
| `Ctrl-Y` | Markdown preview |
| `Ctrl-O` | Open source file in editor |
| `Tab` | Path completion / copy selection |

## Documentation

Full documentation lives in the **[Wiki](../../wiki)**:

- **[Writing Cheats](../../wiki/Writing-Cheats)** - heading structure, code blocks, metadata
- **[Variables](../../wiki/Variables)** - prompt, shell, and literal forms
- **[Selector Options](../../wiki/Selector-Options)** - `--header`, `--column`, `--map`
- **[Conditionals](../../wiki/Conditionals)** - `if` / `fi` branching
- **[Modules](../../wiki/Modules)** - `export` / `import`
- **[Chains](../../wiki/Chains)** - multi-step workflows
- **[Tags](../../wiki/Tags)** - five tag sources
- **[Configuration](../../wiki/Configuration)** - `cheatmd.yaml` reference
- **[Shell Integration](../../wiki/Shell-Integration)** - widget, tmux, zellij
- **[Linting](../../wiki/Linting)** - syntax and reference validation
- **[Dump](../../wiki/Dump)** - metadata export
- **[Recipes](../../wiki/Recipes)** - copy-pasteable patterns

## License

MIT
