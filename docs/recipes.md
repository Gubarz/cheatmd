# CheatMD Recipes

Copy-pasteable patterns. Each recipe is a complete cheat. Drop it into any
`.md` file under your cheats path and it works.

> Convention: I show the fenced code block as `\`\`\`sh` so it renders cleanly
> on GitHub. In your file, use a single backtick fence.

---

## Editable default

Show a default value the user can confirm or overwrite.

```markdown
## curl: GET with timeout

\`\`\`sh title:"GET a URL with a timeout"
curl --max-time $timeout $url
\`\`\`
<!-- cheat
var url     --- --header "URL"
var timeout = echo "10" --- --header "Timeout (seconds)"
-->
```

`echo "default"` produces a single line, which cheatmd pre-fills and lets you
edit before pressing `Enter`.

---

## Pick from a list, return a different column

User sees a friendly description; the command gets the short key.

```markdown
## SSH: connect with auth method

\`\`\`sh title:"SSH with chosen auth method"
ssh $ssh_flags $user@$host
\`\`\`
<!-- cheat
var host = --- --header "Hostname"
var user = echo "$USER" --- --header "Username"
var auth_method = printf 'key\tUse SSH key (default)\npassword\tUse password\n' \
    --- --delimiter '\t' --column 2 --select-column 1 --header "Auth method"

if $auth_method == key
  var ssh_flags := -o PreferredAuthentications=publickey
fi

if $auth_method == password
  var ssh_flags := -o PreferredAuthentications=password
fi
-->
```

---

## Pick a Docker container

```markdown
## Docker: exec

\`\`\`sh title:"Open a shell in a container"
docker exec -it $container /bin/sh
\`\`\`
<!-- cheat
var container = docker ps --format "{{.Names}}" --- --header "Container"
-->
```

If no containers are running the picker shows nothing, and cheatmd falls back
to a manual prompt so the cheat still works.

---

## Pick a Kubernetes context + namespace

```markdown
## Kubernetes: get pods

\`\`\`sh title:"List pods in a namespace"
kubectl get pods -n $namespace --context $context
\`\`\`
<!-- cheat
var context   = kubectl config get-contexts -o name --- --header "Context"
var namespace = kubectl --context $context get ns -o name --- \
    --map "cut -d/ -f2" --header "Namespace"
-->
```

`$context` is resolved before `$namespace`'s shell runs, so the second var can
reference the first. `--map "cut -d/ -f2"` strips the `namespace/` prefix
`kubectl` outputs.

---

## Reusable module: shared variable across cheats

Define once:

```markdown
<!-- common.md -->

## (module) Docker container

\`\`\`text
\`\`\`
<!-- cheat
export docker_container
var container = docker ps --format "{{.Names}}" --- --header "Container"
-->
```

Use anywhere:

```markdown
## Docker: tail logs

\`\`\`sh title:"Follow logs"
docker logs -f $container
\`\`\`
<!-- cheat
import docker_container
-->

## Docker: stop

\`\`\`sh title:"Stop container"
docker stop $container
\`\`\`
<!-- cheat
import docker_container
-->
```

The picker for `$container` runs once per cheat selection.

---

## Fast path entry with Tab completion

```markdown
## tar: extract

\`\`\`sh title:"Extract an archive"
tar -xvf $file -C $dest
\`\`\`
<!-- cheat
var file = find . -maxdepth 1 \( -name "*.tar*" -o -name "*.tgz" \) 2>/dev/null \
    --- --header "Archive"
var dest = echo "." --- --header "Destination"
-->
```

When `$file` or `$dest` is being resolved, type a path prefix and press `Tab`:

```text
./ar<Tab>        -> ./archive.tar.gz
/tm<Tab>         -> /tmp/
$HOME/Down<Tab>  -> $HOME/Downloads/
```

If there are several matches, cheatmd expands to the shared prefix and shows
the possible completions inline. Spaces are escaped automatically.

---

## Branching: dev vs prod URL

```markdown
## API: ping

\`\`\`sh title:"Ping a service"
curl -s $url/health
\`\`\`
<!-- cheat
var env = printf 'dev\nstaging\nprod\n' --- --header "Environment"

if $env == dev
  var url := https://api.dev.example.com
fi

if $env == staging
  var url := https://api.staging.example.com
fi

if $env == prod
  var url := https://api.example.com
fi
-->
```

---

## Transform with `--map`

When a column extract isn't enough.

```markdown
## AWS: pick bucket

\`\`\`sh title:"List bucket contents"
aws s3 ls s3://$bucket
\`\`\`
<!-- cheat
var bucket = aws s3 ls --- --map "awk '{print \$3}'" --header "Bucket"
-->
```

`aws s3 ls` outputs `2024-01-15 12:00:00 my-bucket` per line. `--map "awk
'{print $3}'"` returns just the name.

---

## Chain: multi-step workflow

Use chains when a workflow should advance one cheat at a time across separate
cheatmd launches.

```markdown
## Release: choose version

\`\`\`sh title:"Show release version"
echo $version
\`\`\`
<!-- cheat
chain release 1
var version --- --header "Version"
-->

## Release: build

\`\`\`sh title:"Build release artifact"
make build VERSION=$version
\`\`\`
<!-- cheat
chain release 2
var version --- --header "Version"
-->

## Release: publish

\`\`\`sh title:"Publish release artifact"
make publish VERSION=$version
\`\`\`
<!-- cheat
chain release 3
var version --- --header "Version"
-->
```

Run `/chain release` in the picker. Step 1 runs and exits; the next plain
`cheatmd` launch resumes at step 2. Reset progress with:

```bash
cheatmd chain reset release
```

---

## Dump metadata for tools

Use `dump` when you want to feed cheats into another index, search tool, or
reporting script.

```bash
cheatmd dump ~/cheats --json
cheatmd dump ~/cheats --csv
```

Each entry includes filename, tags, title, description, command, chain fields,
and defined variables.
