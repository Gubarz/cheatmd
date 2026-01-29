# CheatMD

Executable Markdown cheatsheets. Write readable docs, run interactive commands.

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

Press `Ctrl+G` to open the selector.

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
var <name> = <shell>               # Variable from shell output
var <name> = <shell> --- <opts>    # With options (e.g. --header "Title")
var <name> := <value>              # Literal value (no shell, with $var substitution)
export <name>                      # Make module importable
import <name>                      # Use exported module

# Conditionals
if $var == value
var <name> := <value>
fi

if $var != value
var <name> = <shell>
fi
```

### Selector Options

```
--header "Title"           # Custom header text
--delimiter "\t"           # Split lines by delimiter
--column 2                 # Display specific column
--map "cut -f1"            # Transform selected value
```

## License

MIT
