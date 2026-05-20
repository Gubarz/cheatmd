package parser

import (
	"regexp"
)

var (
	dollarVarRegex = regexp.MustCompile(`\$([a-zA-Z_][a-zA-Z0-9_]*)`)
	angleVarRegex  = regexp.MustCompile(`<([a-zA-Z_][a-zA-Z0-9_]*)>`)
)

// ExtractVars finds all variables in a command string using both dollar ($var)
// and angle bracket (<var>) syntaxes. It returns a deduplicated list of
// variable names.
func ExtractVars(command string) []string {
	varMap := make(map[string]bool)
	var vars []string

	// Find dollar variables
	dollarMatches := dollarVarRegex.FindAllStringSubmatch(command, -1)
	for _, match := range dollarMatches {
		if len(match) > 1 {
			name := match[1]
			if !varMap[name] {
				varMap[name] = true
				vars = append(vars, name)
			}
		}
	}

	// Find angle bracket variables
	angleMatches := angleVarRegex.FindAllStringSubmatch(command, -1)
	for _, match := range angleMatches {
		if len(match) > 1 {
			name := match[1]
			if !varMap[name] {
				varMap[name] = true
				vars = append(vars, name)
			}
		}
	}

	return vars
}
