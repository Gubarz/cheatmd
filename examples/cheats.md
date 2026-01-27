# Example Cheats

Simple examples demonstrating cheatmd features.

## Git: checkout branch

> Switch to an existing branch.

```sh
git checkout $branch
```
<!-- cheat
import git_branch
-->

## Git: delete branch

> Delete a local branch.

```sh
git branch -d $branch
```
<!-- cheat
import git_branch
-->

## Docker: exec into container

> Open a shell in a running container.

```sh
docker exec -it $container /bin/sh
```
<!-- cheat
import docker_container
-->

## Docker: view logs

> Follow logs from a container.

```sh
docker logs -f $container
```
<!-- cheat
import docker_container
-->

## Kubernetes: get pods

> List pods in a namespace.

```sh
kubectl get pods -n $namespace --context $context
```
<!-- cheat
import kube_context
-->

## Files: find by name

> Find files matching a pattern.

```sh
find $dir -name "$pattern"
```
<!-- cheat
var dir = printf '%s\n' '' '.' '~' '/tmp' --- --header "Search directory"
var pattern = echo "" --- --header "File pattern (e.g., *.txt)"
-->

## Archive: extract tar

> Extract a tar archive.

```sh
tar -xvf $file -C $dest
```
<!-- cheat
var file = find . -maxdepth 1 -name "*.tar*" -o -name "*.tgz" 2>/dev/null --- --header "Select archive"
var dest = echo "." --- --header "Destination directory"
-->
