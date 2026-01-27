# CheatMD

Executable Markdown cheatsheets. Write readable docs, run interactive commands.

## Install

```bash
go install github.com/gubarz/cheatmd/cmd/cheatmd@latest
```

Requires [fzf](https://github.com/junegunn/fzf).

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

> Show all running containers.

` ` `sh
docker ps
` ` `
```

### With Variables

```markdown
## Docker: exec into container

` ` `sh
docker exec -it $container /bin/sh
` ` `
<!-- cheat
var container = docker ps --format "{{.Names}}"
-->
```

Variables are populated from shell command output:
- **0 lines** → manual input prompt
- **1 line** → pre-filled, confirm with Enter
- **2+ lines** → fzf selection

### Custom Headers

```markdown
<!-- cheat
var branch = git branch --format="%(refname:short)" --- --header "Select branch"
-->
```

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

` ` `sh
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
```

## DSL

```
var <name> = <shell>               # Variable from shell output
var <name> = <shell> --- <fzf>     # With fzf options
export <name>                      # Make module importable
import <name>                      # Use exported module
```

## License

MIT
