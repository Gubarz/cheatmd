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

```markdown
## Docker: list containers

` ` `sh title:"Show all running containers"
docker ps
` ` `
```

### With Variables

```markdown
## Docker: exec into container

` ` `sh title:"Execute shell in container"
docker exec -it $container /bin/sh
` ` `
<!-- cheat
var container = docker ps --format "{{.Names}}" --- --header "Select container"
-->
```

Variables are populated from shell command output:
- **0 lines** → manual input prompt
- **1 line** → pre-filled, confirm with Enter
- **2+ lines** → selection list

### Modules

Export reusable variables:

```markdown
<!-- cheat
export docker_container
var container = docker ps --format "{{.Names}}" --- --header "Select container"
-->
```

Import them elsewhere:

```markdown
## Docker: view logs

` ` `sh title:"Follow container logs"
docker logs -f $container
` ` `
<!-- cheat
import docker_container
-->
```

## Configuration

`~/.config/cheatmd/cheatmd.yaml`:

```yaml
path: ~/cheats
output: print          # print, copy, exec
shell: /bin/bash
require_cheat_block: false
auto_continue: false   # Auto-accept env vars without prompting
```

## DSL

```
var <name>                         # Prompt user for a value
var <name> = <shell>               # Populate from shell output
var <name> = <shell> --- <opts>    # With selector options (see below)
var <name> := <value>              # Literal value (with $var substitution)
export <name>                      # Make module importable
import <name>                      # Use an exported module

if $var == value                   # Conditional block (any var form works inside)
  var <name> := <value>
fi
```

`--- --header "..."` works on both `=` and `:=`. Lines beginning with `#` inside
a `<!-- cheat -->` block are comments.

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

## License

MIT
