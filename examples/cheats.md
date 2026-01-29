# Example Cheats

Simple examples demonstrating cheatmd features.

## Git: checkout branch

```sh title:"Switch to an existing branch."
git checkout $branch
```
<!-- cheat
import git_branch
-->

## Git: delete branch

```sh title:"Delete a local branch."
git branch -d $branch
```
<!-- cheat
import git_branch
-->

## Docker: exec into container

```sh title:"Open a shell in a running container."
docker exec -it $container /bin/sh
```
<!-- cheat
import docker_container
-->

## Docker: view logs

```sh title:"Follow logs from a container."
docker logs -f $container
```
<!-- cheat
import docker_container
-->

## Kubernetes: get pods

```sh title:"List pods in a namespace."
kubectl get pods -n $namespace --context $context
```
<!-- cheat
import kube_context
-->

## Files: find by name

```sh title:"Find files matching a pattern."
find $dir -name "$pattern"
```
<!-- cheat
var dir = printf '%s\n' '' '.' '~' '/tmp' --- --header "Search directory"
var pattern = echo "" --- --header "File pattern (e.g., *.txt)"
-->

## Archive: extract tar

```sh title:"Extract a tar archive."
tar -xvf $file -C $dest
```
<!-- cheat
var file = find . -maxdepth 1 -name "*.tar*" -o -name "*.tgz" 2>/dev/null --- --header "Select archive"
var dest = echo "." --- --header "Destination directory"
-->

## SSH: connect with auth method

```sh title:"SSH with flexible authentication."
ssh $ssh_flags $user@$host
```
<!-- cheat
var host = --- --header "Hostname"
var user = echo "$USER" --- --header "Username"
var auth_method = printf 'key\tUse SSH key (default)\npassword\tUse password\n' --- --delimiter '\t' --column 2 --map "cut -f1"

if $auth_method == key
var ssh_flags := -o PreferredAuthentications=publickey
fi

if $auth_method == password
var ssh_flags := -o PreferredAuthentications=password
fi
-->
