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
cheatmd --lint ~/cheats    # Check cheats for syntax/reference issues
cheatmd dump ~/cheats      # Dump parsed cheat metadata as JSON
```

### Shell Widget

```bash
eval "$(cheatmd widget bash)"   # Add to ~/.bashrc
eval "$(cheatmd widget zsh)"    # Add to ~/.zshrc
```

Press `Ctrl+G` to open the selector. The chosen command is inserted into your
shell prompt; press `Enter` again to run it. Change the trigger key by setting
`key_widget: "\\C-g"` (or any readline keyspec) in your config.

### tmux

Add to `~/.tmux.conf`:

```tmux
bind-key -n C-n split-window "$SHELL --login -i -c 'cheatmd --print | tr -d \"\\r\\n\" | tmux load-buffer -b tmp - ; tmux paste-buffer -t {last} -b tmp -d'"
```

Press `Ctrl+n` to open cheatmd in a split; the chosen command is pasted into
the previous pane.

### Zellij

Add to your Zellij config:

```kdl
bind "Ctrl n" {
    Run "sh" "-c" "content=$(cheatmd --print); zellij action toggle-floating-panes; zellij action write-chars \"$content\"" {
        floating true
        close_on_exit true
    };
}
```

Press `Ctrl+n` to open cheatmd in a floating pane.

## Writing Cheats

### Basic

````markdown
## Docker: list containers

```sh title:"Show all running containers"
docker ps
```
````

### With Variables

````markdown
## Docker: exec into container

```sh title:"Execute shell in container"
docker exec -it $container /bin/sh
```
<!-- cheat
var container = docker ps --format "{{.Names}}" --- --header "Select container"
-->
````

Variables are populated from shell command output:
- **0 lines** → manual input prompt
- **1 line** → pre-filled, confirm with Enter
- **2+ lines** → selection list

While resolving variables, `Tab` completes filesystem paths inline:

```text
/us<Tab>              -> /usr/
/usr/sh<Tab>          -> /usr/share/
/var/lo<Tab>          -> /var/log/
$HOME/pro<Tab>        -> $HOME/projects/
```

Directories get a trailing `/`, spaces are shell-escaped, and multiple matches
expand to their shared prefix and show candidates inline. `Ctrl-T` still opens
the substitute picker for environment variables and shell-history values.

By default only `$name` is recognized as a variable reference and undeclared
references are silently skipped. Two config knobs relax that:

- `var_syntax`: which variable syntax cheatmd recognizes in commands.
  - `dollar` (default): only `$name`
  - `angle`: only `<name>`
  - `both`: accept both, mixed in one command resolves to the same variable
- `allow_undeclared_vars: true`: prompt for any referenced variable that
  has no `<!-- cheat -->` declaration, instead of skipping it.

With `var_syntax: both` and `allow_undeclared_vars: true`, this cheat works
with no metadata block:

````markdown
## SSH

```sh title:"SSH to a host"
ssh $user@<host> -p $port
```
````

The user is prompted for `user`, `host`, and `port` in order. Strict defaults
are kept so existing cheats stay backwards compatible.

### Modules

Export reusable variables:

```markdown
<!-- cheat
export docker_container
var container = docker ps --format "{{.Names}}" --- --header "Select container"
-->
```

Import them elsewhere:

````markdown
## Docker: view logs

```sh title:"Follow container logs"
docker logs -f $container
```
<!-- cheat
import docker_container
-->
````

### Chains

Chains are ordered cheats for multi-step workflows. Mark each step in its
cheat block:

````markdown
## Release: build

```sh title:"Build release artifact"
make build VERSION=$version
```
<!-- cheat
chain release 1
var version
-->

## Release: publish

```sh title:"Publish release artifact"
make publish VERSION=$version
```
<!-- cheat
chain release 2
var version
-->
````

In the picker, search chains with `/chain`:

```text
/chain release
```

Selecting the chain runs the next step and exits. The next plain `cheatmd`
launch resumes the active chain at the following step, so you can run other
cheats in between and come back. After the last step, the active chain is
cleared and the next step resets to `1`.

Reset chain progress with:

```bash
cheatmd chain reset          # Reset all chains for this cheats path
cheatmd chain reset release  # Reset one chain
```

## Configuration

`~/.config/cheatmd/cheatmd.yaml`:

```yaml
path: ~/cheats
output: print          # print, copy, exec
shell: /bin/bash
require_cheat_block: false
auto_continue: false   # Auto-accept env vars without prompting

# Substitute search (while resolving a variable, press Ctrl-T to fuzzy-search
# environment variables and shell history for a value to insert)
key_substitute: "ctrl+t"
substitute_sources: ["env", "history"]   # set to [] to disable

# Markdown preview (press Ctrl-Y to open the current cheat's source file
# rendered as markdown, scrolled to the cheat's section)
key_preview: "ctrl+y"
```

### Substitute search

When a cheat asks for a variable (say `$host`), press `Ctrl-T` to open a
fuzzy-search picker over your environment variables and any assignments
(`VAR=value`, `export VAR=value`, `declare -x VAR=value`, leading inline
assignments) found in shell history. Plain commands in history are ignored.
Pick a row, its value is loaded into the prompt; press `Enter` to accept or
edit it first. `Esc` cancels back to the var prompt. History is read from
`$HISTFILE`, falling back to `~/.bash_history` or `~/.zsh_history`.

Path completion is inline and keyboard-native: type a path prefix and press
`Tab`. Use substitute search when you want to pull from environment variables
or past assignments instead of browsing the filesystem.

### Markdown preview

Press `Ctrl-Y` at any cheat (from the picker or while resolving variables) to
open the cheat's source file rendered as markdown in a full-screen overlay,
auto-scrolled to the cheat's heading. `↑/↓`/`PgUp/PgDn` scroll, `Esc` or `q`
returns. Useful for reading the surrounding notes (descriptions, links,
warnings) without leaving the TUI.

### Execution history

Every time you run a cheat, the final substituted command plus the cheat's
file/header reference and the resolved variable values are appended to
`$XDG_DATA_HOME/cheatmd/history.jsonl` (falling back to
`~/.local/share/cheatmd/history.jsonl`). Press `Ctrl-H` in the cheat picker
to open the history overlay, or launch directly with `cheatmd --history`.
Pick an entry with `Enter` to re-open the original cheat with its previous
values pre-filled, so you can confirm or edit any variable before running
again. `Esc` cancels.

The main picker also uses history for frecency ranking: cheats you run often
and recently float toward the top of normal search results. If there is no
history yet, results keep their parsed order.

### TUI keys

Useful defaults:

| Key | Action |
| --- | ------ |
| `Ctrl-H` | Open execution history |
| `Ctrl-T` | Substitute search while resolving a variable |
| `Ctrl-Y` | Markdown preview |
| `Ctrl-O` | Open source file in editor/viewer |
| `Tab` | Complete paths in variable inputs; otherwise copy highlighted selector option |
| `Ctrl-Delete` / `Alt-D` | Delete next word in text inputs |
| `Ctrl-W` / `Alt-Backspace` | Delete previous word in text inputs |

### Linting

Run `cheatmd --lint [path]` to validate a cheats file or directory without
opening the picker. Findings are printed in GCC style:

```text
file.md:12:1: error: import "common" does not resolve to any exported module
```

The linter checks DSL syntax, missing imports, duplicate exports, duplicate or
missing chain steps, undeclared command variables, empty or duplicate `##`
headings, and cheats with no code block. Warnings do not fail the command
unless you pass `--strict`.

Undeclared-variable checks are syntax-aware. Shell `$HOME` and PowerShell
`$true` are treated as language-defined variables, but template refs stay
strict: `<HOME>` warns unless `HOME` is declared with `var HOME`. Shell loop
vars, assignments, `read`, `local`, `declare`, `export name=value`, and similar
syntax declarations are recognized. PowerShell assignments, `foreach`,
`param(...)`, and function parameters are recognized too.

### Dump

`dump` emits parsed cheat metadata for indexing, debugging, or external tools.
JSON is the default; CSV is available with `--csv`.

```bash
cheatmd dump ~/cheats
cheatmd dump ~/cheats --json
cheatmd dump ~/cheats --csv
```

Each row includes filename, tags, title, description, command, chain metadata,
and defined variables. Variable `kind` is one of:

| Kind | DSL form |
| ---- | -------- |
| `prompt` | `var name` |
| `shell` | `var name = command` |
| `literal` | `var name := value` |

## DSL

```
var <name>                         # Prompt user for a value
var <name> --- <opts>              # Prompt with options, e.g. --header
var <name> = <shell>               # Populate from shell output
var <name> = <shell> --- <opts>    # With selector options (see below)
var <name> := <value>              # Literal value (with $var substitution)
export <name>                      # Make module importable
import <name>                      # Use an exported module
chain <name> <step>                # Mark this cheat as a chain step

if $var == value                   # Conditional block (any var form works inside)
  var <name> := <value>
fi
```

`--- --header "..."` works on prompt-only, `=`, and `:=` vars. Lines beginning
with `#` inside a `<!-- cheat -->` block are comments.

### Selector Options

```
--header "Title"           # Picker header text
--delimiter "\t"           # Split lines by delimiter
--column 2                 # Show this column in the picker
--select-column 1          # Return this column as the value
--map "cmd"                # Pipe selected value through a shell command
```

Full reference: [docs/dsl.md](docs/dsl.md). Patterns and copy-pasteable
examples: [docs/recipes.md](docs/recipes.md).

## Tags

Cheats are searchable by tag. Tags can come from five places: the folder/file
path, YAML front matter, a hashtag or YAML block at the end of the file, an
inline `#tag` in prose under a cheat, or the heading itself.

````markdown
---
tags: [aws, cloud]
---

# AWS

## list buckets

#s3

```sh title:"List S3 buckets"
aws s3 ls
```

---
#quickref #production
````

Tags are merged, lowercased, and folded into the regular search index; type any
of them in the picker. Full details: [docs/tags.md](docs/tags.md).

## License

MIT
