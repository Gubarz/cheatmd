# Modules

Reusable variable definitions that can be imported by other cheats.

<!-- cheat
export git_branch
var branch = git branch --format="%(refname:short)" --- --header "Select branch"
-->

<!-- cheat
export docker_container
var container = docker ps --format "{{.Names}}" --- --header "Select container"
-->

<!-- cheat
export kube_context
var context = kubectl config get-contexts -o name 2>/dev/null --- --header "Select context"
var namespace = kubectl get namespaces -o jsonpath='{.items[*].metadata.name}' 2>/dev/null | tr ' ' '\n' --- --header "Select namespace"
-->
