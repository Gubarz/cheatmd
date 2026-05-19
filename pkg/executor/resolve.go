package executor

import (
	"strings"
	"regexp"

	"github.com/gubarz/cheatmd/pkg/config"
	"github.com/gubarz/cheatmd/pkg/parser"
)

// CollectDependencies gathers all variable definitions and their topological ordering.
func CollectDependencies(cheat *parser.Cheat, index *parser.CheatIndex) ([]string, map[string][]parser.VarDef) {
	varDefs := CollectVarDefinitions(cheat, index)
	usedVars := FindAllVars(cheat.Command, config.GetVarSyntax())

	if config.GetAllowUndeclaredVars() {
		for _, name := range usedVars {
			if _, ok := varDefs[name]; !ok {
				varDefs[name] = []parser.VarDef{{Name: name}}
			}
		}
	}

	allNeeded := FindAllDependencies(usedVars, varDefs)
	orderedVars := TopologicalSort(usedVars, varDefs, allNeeded)

	return orderedVars, varDefs
}

// CollectVarDefinitions gathers all var definitions from imports and local cheat.
func CollectVarDefinitions(cheat *parser.Cheat, index *parser.CheatIndex) map[string][]parser.VarDef {
	varDefs := make(map[string][]parser.VarDef)

	var collectFromImports func(imports []string, seen map[string]bool)
	collectFromImports = func(imports []string, seen map[string]bool) {
		for _, importName := range imports {
			if seen[importName] {
				continue
			}
			seen[importName] = true
			if module, ok := index.Modules[importName]; ok {
				collectFromImports(module.Imports, seen)
				for _, v := range module.Vars {
					varDefs[v.Name] = append(varDefs[v.Name], v)
				}
			}
		}
	}
	collectFromImports(cheat.Imports, make(map[string]bool))

	for _, v := range cheat.Vars {
		varDefs[v.Name] = append(varDefs[v.Name], v)
	}
	return varDefs
}

func varDefDependencies(def parser.VarDef) []string {
	var deps []string
	deps = append(deps, FindAllVars(def.Shell, "dollar")...)
	deps = append(deps, FindAllVars(def.Literal, "dollar")...)
	deps = append(deps, FindAllVars(def.Condition, "dollar")...)
	return deps
}

// FindAllDependencies finds transitive closure of all needed variables.
func FindAllDependencies(usedVars []string, varDefs map[string][]parser.VarDef) map[string]bool {
	allNeeded := make(map[string]bool)
	queue := make([]string, len(usedVars))
	copy(queue, usedVars)

	for len(queue) > 0 {
		varName := queue[0]
		queue = queue[1:]

		if allNeeded[varName] {
			continue
		}
		allNeeded[varName] = true

		for _, def := range varDefs[varName] {
			for _, dep := range varDefDependencies(def) {
				if !allNeeded[dep] {
					queue = append(queue, dep)
				}
			}
		}
	}
	return allNeeded
}

// TopologicalSort orders variables by their dependencies.
func TopologicalSort(usedVars []string, varDefs map[string][]parser.VarDef, allNeeded map[string]bool) []string {
	var orderedVars []string
	added := make(map[string]bool)
	visiting := make(map[string]bool)

	var addWithDeps func(varName string)
	addWithDeps = func(varName string) {
		if added[varName] || !allNeeded[varName] || visiting[varName] {
			return
		}
		visiting[varName] = true
		for _, def := range varDefs[varName] {
			for _, dep := range varDefDependencies(def) {
				addWithDeps(dep)
			}
		}
		visiting[varName] = false
		added[varName] = true
		orderedVars = append(orderedVars, varName)
	}

	for _, v := range usedVars {
		addWithDeps(v)
	}
	return orderedVars
}

// EvaluateCondition evaluates a condition expression against the scope.
func EvaluateCondition(condition string, scope map[string]string) bool {
	condition = strings.TrimSpace(condition)

	condition = SubstituteVars(condition, scope, "dollar")

	if strings.Contains(condition, "==") {
		parts := strings.SplitN(condition, "==", 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			return left == right
		}
	}

	if strings.Contains(condition, "!=") {
		parts := strings.SplitN(condition, "!=", 2)
		if len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(parts[1])
			return left != right
		}
	}

	return condition != ""
}

// ReplaceVar replaces variable references in cmd with replacement.
func ReplaceVar(cmd, varName, replacement string, syntax string) string {
	q := regexp.QuoteMeta(varName)
	var parts []string
	if syntax == "dollar" || syntax == "both" {
		parts = append(parts, `\$`+q+`\b`)
	}
	if syntax == "angle" || syntax == "both" {
		parts = append(parts, `<`+q+`>`)
	}
	if len(parts) == 0 {
		return cmd
	}
	pattern := strings.Join(parts, "|")
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllLiteralString(cmd, replacement)
}

// FindAllVars finds ALL variable references in a command, ignoring quoting.
func FindAllVars(cmd string, syntax string) []string {
	allowDollar := syntax == "dollar" || syntax == "both"
	allowAngle := syntax == "angle" || syntax == "both"

	var vars []string
	seen := make(map[string]bool)
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		vars = append(vars, name)
	}

	for i := 0; i < len(cmd); i++ {
		switch cmd[i] {
		case '$':
			if !allowDollar {
				continue
			}
			if i+1 >= len(cmd) {
				continue
			}
			if i > 0 && cmd[i-1] == '\\' {
				continue
			}
			j := i + 1
			for j < len(cmd) && parser.IsVarChar(cmd[j], j == i+1) {
				j++
			}
			if j > i+1 {
				add(cmd[i+1 : j])
			}
			i = j - 1
		case '<':
			if !allowAngle {
				continue
			}
			j := i + 1
			if j >= len(cmd) {
				continue
			}
			if !parser.IsVarChar(cmd[j], true) {
				continue
			}
			j++
			for j < len(cmd) && parser.IsVarChar(cmd[j], false) {
				j++
			}
			if j >= len(cmd) || cmd[j] != '>' {
				continue
			}
			add(cmd[i+1 : j])
			i = j
		}
	}

	return vars
}
