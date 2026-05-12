package ui

import (
	"regexp"
	"strings"

	"github.com/gubarz/cheatmd/internal/config"
	"github.com/gubarz/cheatmd/internal/executor"
	"github.com/gubarz/cheatmd/internal/parser"
)

// findMatchingCheat finds a cheat whose command pattern matches the input.
// It builds a regex from the cheat command (replacing $var with capture groups)
// and returns the first match.
func findMatchingCheat(cheats []*parser.Cheat, input string) *parser.Cheat {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}

	for _, cheat := range cheats {
		pattern, _ := buildMatchPattern(cheat.Command)
		if pattern.MatchString(input) {
			return cheat
		}
	}
	return nil
}

// buildMatchPattern converts a command template to a regex pattern for matching.
//
//	"echo $name"     -> "^echo (\S+)$"
//	'echo "$name"'   -> '^echo "([^"]*)"$'
//
// Returns the pattern and a list of variable names for each capture group.
// The slice may have duplicates (one entry per capture group) because Go
// regex doesn't support backreferences.
func buildMatchPattern(cmd string) (*regexp.Regexp, []string) {
	var parts []string
	if config.VarSyntaxAllowsDollar() {
		parts = append(parts, `\$(\w+)`)
	}
	if config.VarSyntaxAllowsAngle() {
		parts = append(parts, `<(\w+)>`)
	}
	if len(parts) == 0 {
		parts = append(parts, `\$(\w+)`)
	}
	varPattern := regexp.MustCompile(strings.Join(parts, "|"))
	allMatches := varPattern.FindAllStringSubmatchIndex(cmd, -1)

	var varOrder []string

	var result strings.Builder
	result.WriteString(`^\s*`)
	lastEnd := 0

	for i, match := range allMatches {
		varStart := match[0]
		varEnd := match[1]
		
		var varName string
		for j := 2; j < len(match); j += 2 {
			if match[j] != -1 {
				varName = cmd[match[j]:match[j+1]]
				break
			}
		}

		if varStart > lastEnd {
			result.WriteString(regexp.QuoteMeta(cmd[lastEnd:varStart]))
		}

		varOrder = append(varOrder, varName)

		beforeVar := cmd[:varStart]
		afterVar := cmd[varEnd:]

		if strings.HasSuffix(beforeVar, `"`) && strings.HasPrefix(afterVar, `"`) {
			// Inside double quotes - don't include the quotes in capture.
			current := result.String()
			if strings.HasSuffix(current, `"`) {
				result.Reset()
				result.WriteString(current[:len(current)-1])
			}
			result.WriteString(`"([^"]*)"`)
			lastEnd = varEnd + 1
			continue
		} else if strings.HasSuffix(beforeVar, `'`) && strings.HasPrefix(afterVar, `'`) {
			current := result.String()
			if strings.HasSuffix(current, `'`) {
				result.Reset()
				result.WriteString(current[:len(current)-1])
			}
			result.WriteString(`'([^']*)'`)
			lastEnd = varEnd + 1
			continue
		}

		isLastVar := i == len(allMatches)-1
		remainingText := strings.TrimSpace(cmd[varEnd:])
		if isLastVar && remainingText == "" {
			// Last variable at end of command - greedy to capture multi-word values.
			result.WriteString(`(.+)`)
		} else {
			nextLiteralStart := varEnd
			nextLiteralEnd := len(cmd)
			if i+1 < len(allMatches) {
				nextLiteralEnd = allMatches[i+1][0]
			}
			nextLiteral := strings.TrimSpace(cmd[nextLiteralStart:nextLiteralEnd])

			if nextLiteral != "" {
				result.WriteString(`(.+?)`)
			} else {
				result.WriteString(`(\S+)`)
			}
		}
		lastEnd = varEnd
	}

	if lastEnd < len(cmd) {
		result.WriteString(regexp.QuoteMeta(cmd[lastEnd:]))
	}
	result.WriteString(`\s*$`)

	re, err := regexp.Compile(result.String())
	if err != nil {
		return regexp.MustCompile(`^$`), nil
	}
	return re, varOrder
}

// prefillScopeFromMatch extracts variable values from the matched command and
// writes them into cheat.Scope.
func prefillScopeFromMatch(cheat *parser.Cheat, input string) {
	input = strings.TrimSpace(input)
	pattern, varNames := buildMatchPattern(cheat.Command)
	if pattern == nil || len(varNames) == 0 {
		return
	}

	matches := pattern.FindStringSubmatch(input)
	if matches == nil {
		return
	}

	if cheat.Scope == nil {
		cheat.Scope = make(map[string]string)
	}

	for i, name := range varNames {
		if i+1 < len(matches) {
			if _, exists := cheat.Scope[name]; !exists {
				cheat.Scope[name] = matches[i+1]
			}
		}
	}
}

// inferDependentVars reverse-engineers dependent variables from literal values.
// Example: if auth_flags=-k and we have "if $auth_method == kerberos then
// auth_flags := -k", we can infer auth_method=kerberos.
func inferDependentVars(cheat *parser.Cheat, index *parser.CheatIndex) {
	if len(cheat.Scope) == 0 {
		return
	}

	varDefs := collectVarDefinitions(cheat, index)

	changed := true
	for changed {
		changed = false
		for varName, prefillValue := range cheat.Scope {
			defs, ok := varDefs[varName]
			if !ok {
				continue
			}

			for _, def := range defs {
				if def.Literal == "" || def.Condition == "" {
					continue
				}

				condVar, condOp, condValue := parseCondition(def.Condition)
				if condVar == "" {
					continue
				}

				if _, exists := cheat.Scope[condVar]; exists {
					continue
				}

				literalResult := executor.SubstituteVars(def.Literal, cheat.Scope, "dollar")

				if strings.Contains(literalResult, "$") {
					extracted := extractEmbeddedVars(def.Literal, prefillValue, cheat.Scope)
					for k, v := range extracted {
						if _, exists := cheat.Scope[k]; !exists {
							cheat.Scope[k] = v
							changed = true
						}
					}
					literalResult = executor.SubstituteVars(def.Literal, cheat.Scope, "dollar")
				}

				if literalResult == prefillValue && condOp == "==" {
					cheat.Scope[condVar] = condValue
					changed = true
				}
			}
		}
	}
}

// parseCondition parses "$var == value" or "$var != value".
func parseCondition(cond string) (varName, op, value string) {
	cond = strings.TrimSpace(cond)

	if idx := strings.Index(cond, "=="); idx != -1 {
		left := strings.TrimSpace(cond[:idx])
		right := strings.TrimSpace(cond[idx+2:])
		if strings.HasPrefix(left, "$") {
			return left[1:], "==", right
		}
	}

	if idx := strings.Index(cond, "!="); idx != -1 {
		left := strings.TrimSpace(cond[:idx])
		right := strings.TrimSpace(cond[idx+2:])
		if strings.HasPrefix(left, "$") {
			return left[1:], "!=", right
		}
	}

	return "", "", ""
}

// extractEmbeddedVars extracts variable values embedded in a literal template.
// Example: template="-p $credential", actual="-p mypass" -> {credential: mypass}.
func extractEmbeddedVars(template, actual string, existingScope map[string]string) map[string]string {
	result := make(map[string]string)

	pattern := template
	for k, v := range existingScope {
		pattern = strings.ReplaceAll(pattern, "$"+k, regexp.QuoteMeta(v))
	}

	varPattern := regexp.MustCompile(`\$(\w+)`)
	varMatches := varPattern.FindAllStringSubmatchIndex(pattern, -1)
	if len(varMatches) == 0 {
		return result
	}

	var regexParts strings.Builder
	regexParts.WriteString("^")
	lastEnd := 0
	var varNames []string

	for i, match := range varMatches {
		varStart := match[0]
		varEnd := match[1]
		varName := pattern[match[2]:match[3]]

		if varStart > lastEnd {
			regexParts.WriteString(regexp.QuoteMeta(pattern[lastEnd:varStart]))
		}

		// Greedy for the last variable at end of string, non-greedy otherwise.
		if i == len(varMatches)-1 && varEnd == len(pattern) {
			regexParts.WriteString(`(.+)`)
		} else {
			regexParts.WriteString(`(.+?)`)
		}
		varNames = append(varNames, varName)
		lastEnd = varEnd
	}
	if lastEnd < len(pattern) {
		regexParts.WriteString(regexp.QuoteMeta(pattern[lastEnd:]))
	}
	regexParts.WriteString("$")

	re, err := regexp.Compile(regexParts.String())
	if err != nil {
		return result
	}

	matches := re.FindStringSubmatch(actual)
	if matches == nil {
		return result
	}

	for i, name := range varNames {
		if i+1 < len(matches) {
			result[name] = matches[i+1]
		}
	}

	return result
}
