# CheatMD DSL Reference

The DSL lives inside `<!-- cheat ... -->` HTML comments below a fenced code
block. Every line inside the block is one of:

- a **variable definition**
- an `import` / `export` statement
- an `if` / `fi` conditional
- a comment (line starting with `#`)
- blank

A trailing backslash continues a line: `var x = some-long-shell \` then on the
next line `--- --header "Pick one"`.

---

## Referencing variables in commands

The `var_syntax` config setting controls which forms cheatmd recognizes as
variable references in command text:

- `dollar` (default): only `$name`
- `angle`: only `<name>`
- `both`: accept both forms; mixed in one command they resolve to the same
  variable

The companion flag `allow_undeclared_vars: true` makes cheatmd prompt for any
referenced variable that isn't declared in a `<!-- cheat -->` block, instead
of silently skipping it.

With `var_syntax: both` and `allow_undeclared_vars: true`, this command needs
no metadata block:

```sh title:"SSH"
ssh $user@<host> -p $port
```

`<name|default>` is *not* auto-resolved; use a `var name = echo "default"`
declaration if you need an editable default.

---

## Variables

### Prompt-only

```text
var name
```

Asks the user to type a value. No options, no default. Combine with `--header`
on a separate `--- ...` line if you want a label:

```text
var name --- --header "Hostname"
```

### From shell output (`=`)

```text
var name = <shell command>
```

`<shell command>` runs through your configured shell. The output drives the
picker:

| Output           | UI                                |
| ---------------- | --------------------------------- |
| 0 lines (empty)  | Falls back to a manual prompt     |
| 1 line           | Pre-filled, confirm with `Enter`  |
| 2+ lines         | Filterable selection list         |

Examples:

```text
var container = docker ps --format "{{.Names}}"
var branch    = git branch --format='%(refname:short)'
var port      = printf '%s\n' 22 80 443 8080
```

### Literal (`:=`)

```text
var name := <value>
```

No shell. The value is used as-is, with `$other_var` substitution if those vars
are defined earlier in the same block:

```text
var user := admin
var url  := https://$host/api/v1
```

### Selector options (`--- ...`)

Add `--- <opts>` after prompt-only, `=`, or `:=` vars. Options are
space-separated; quote values that contain spaces.

```text
var name --- --header "Enter a value"
var name = <shell> --- --header "Pick one" --column 2 --select-column 1
```

| Option            | Effect                                                     |
| ----------------- | ---------------------------------------------------------- |
| `--header "..."`  | Header text shown above the picker                         |
| `--delimiter "X"` | Split each line by `X` (used by `--column`/`--select-column`) |
| `--column N`      | Show column `N` (1-indexed) in the picker                  |
| `--select-column N` | Return column `N` (1-indexed) as the value               |
| `--map "cmd"`     | Pipe the selected value through `cmd` via stdin            |

Order of operations on the chosen line: `--select-column` extracts the column,
then `--map` transforms it. `--column` is *display only*; it never affects
what's returned.

### Comparing `--column`, `--select-column`, and `--map`

A common pattern: a list of `key<TAB>description` lines where the user sees
both columns but you only want the key back.

```text
var auth = printf 'key\tUse SSH key\npassword\tUse password\n' \
    --- --delimiter '\t' --column 2 --select-column 1 --header "Auth method"
```

`--map` is for transformations a column extract can't do, like lower-casing,
regex extraction, or JSON field access:

```text
var bucket = aws s3 ls --- --map "awk '{print \$3}'"
var lower  = printf 'A\nB\nC' --- --map "tr '[:upper:]' '[:lower:]'"
```

---

## Modules

`export` makes a `<!-- cheat -->` block reusable; `import` pulls all of its
vars into another cheat.

A module:

```markdown
<!-- cheat
export docker_container
var container = docker ps --format "{{.Names}}" --- --header "Container"
-->
```

A consumer:

```markdown
## Docker: tail logs

` ` `sh title:"Follow container logs"
docker logs -f $container
` ` `
<!-- cheat
import docker_container
-->
```

Modules can export multiple vars; the importer gets all of them. A module name
must be unique across your cheats path. Duplicate exports are reported on
load.

---

## Conditionals

```text
if $var == value
  var name := one_thing
fi

if $var != value
  var name = some shell command
fi
```

Use any var form (`=`, `:=`, prompt-only) inside an `if` block. The condition
is evaluated after `$var` is resolved; truthy means "non-empty after
substitution" for bare `if $var`.

Example, branch SSH flags on auth method:

```text
var auth = printf 'key\npassword\n' --- --header "Auth"

if $auth == key
  var ssh_flags := -o PreferredAuthentications=publickey
fi

if $auth == password
  var ssh_flags := -o PreferredAuthentications=password
fi
```

---

## Comments and continuations

```text
<!-- cheat
# This is a comment, ignored by the parser
var long = some-shell-cmd \
    --with --many --flags \
    --- --header "Pick"
-->
```

Backslash-continuation joins lines before parsing, so options can wrap.
