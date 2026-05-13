package linter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintReportsDSLAndReferenceProblems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cheats.md")
	writeFile(t, path, `# Cheats

## Broken

`+"```sh"+`
echo "$missing $ok"
`+"```"+`
<!-- cheat
var ok
import nope
wat
if
fi extra
-->
`)

	findings, err := Lint(dir)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	want := []string{
		"import \"nope\" does not resolve",
		"variable \"missing\" referenced",
		"unknown DSL keyword \"wat\"",
		"`if` requires a condition",
		"`fi` takes no arguments",
	}
	for _, msg := range want {
		if !hasFinding(findings, msg) {
			t.Fatalf("missing finding containing %q\nfindings:\n%s", msg, formatFindings(findings))
		}
	}
}

func TestLintAcceptsContinuedVarShellPipelines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "network.md")
	writeFile(t, path, `# Network

<!-- cheat
export domain
var domain = printf '%s\n' '$op_engagement_domain' "$(grep -v '^[[:space:]]*#' /etc/hosts \
  | sed -E 's/^[[:space:]]+//; s/[[:space:]]+/ /g' \
  | cut -d' ' -f2- \
  | tr ' ' '\n' \
  | sort -u)" --- --header 'Domains'
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "unknown DSL keyword \"|\"") {
		t.Fatalf("continued shell pipelines should stay part of the var line\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintReportsDuplicateExportsAndSingleLineSyntax(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "one.md"), `## Module One

`+"```sh"+`
:
`+"```"+`
<!-- cheat export shared -->
`)
	writeFile(t, filepath.Join(dir, "two.md"), `## Module Two

`+"```sh"+`
:
`+"```"+`
<!-- cheat export shared too-many -->
`)
	writeFile(t, filepath.Join(dir, "three.md"), `## Module Three

`+"```sh"+`
:
`+"```"+`
<!-- cheat export shared -->
`)

	findings, err := Lint(dir)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "`export` name must be a single token") {
		t.Fatalf("missing single-line syntax finding\nfindings:\n%s", formatFindings(findings))
	}
	if !hasFinding(findings, "duplicate export \"shared\"") {
		t.Fatalf("missing duplicate export finding\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintReportsStructuralWarnings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "structural.md")
	writeFile(t, path, `##

### Repeat

Some notes.

### Repeat

`+"```sh"+`
echo ok
`+"```"+`
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	for _, msg := range []string{"empty markdown header"} {
		if !hasFinding(findings, msg) {
			t.Fatalf("missing structural finding containing %q\nfindings:\n%s", msg, formatFindings(findings))
		}
	}
}

func TestLintReportsDuplicateCheatNamesAtAnyHeaderLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "duplicates.md")
	writeFile(t, path, `# whoami

`+"```sh"+`
whoami
`+"```"+`
<!-- cheat
-->

##### whoami

`+"```sh"+`
id
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "duplicate cheat name \"whoami\"") {
		t.Fatalf("missing duplicate finding for repeated cheat names\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintAllowsSameHeaderTextWhenOnlyOneIsACheat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page_title.md")
	writeFile(t, path, `# Responder

<!-- cheat
export interface
var interface
-->

## Responder

`+"```sh"+`
sudo responder -I $interface
`+"```"+`
<!-- cheat
import interface
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "duplicate") {
		t.Fatalf("same header text should only duplicate when both entries are cheats\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintAllowsHeadingWithoutCodeBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases.md")
	writeFile(t, path, `## apt

Alias of [apt-get](#apt_get). All techniques from apt-get apply.

## apt-get

### apt-get shell

`+"```sh"+`
apt-get update
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "cheat has no code block") {
		t.Fatalf("plain heading should not require a code block\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintReportsCheatWithoutH2Header(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing_header.md")
	writeFile(t, path, `Some intro text with no markdown header.

`+"```sh"+`
whoami
`+"```"+`
<!-- cheat
-->

`+"```sh"+`
id
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "cheat has no markdown header") {
		t.Fatalf("missing header finding\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintDoesNotWarnUndeclaredVarsWithoutCheatBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scratch.md")
	writeFile(t, path, `# Scratch

`+"```sh"+`
if [ "$INSTALLED" = 1 ]; then
  echo installed
fi
`+"```"+`
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "variable \"INSTALLED\" referenced") {
		t.Fatalf("plain code fences should not require cheat vars\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintAcceptsAnyMarkdownHeaderLevelForCheat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep_headers.md")
	writeFile(t, path, `#### Deep cheat

`+"```sh"+`
whoami
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "cheat has no markdown header") {
		t.Fatalf("any markdown header should name a cheat\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintDoesNotWarnForExportOnlyBlocks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "modules.md")
	writeFile(t, path, `# Modules

<!-- cheat
export net_target
var host
var port
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "has no preceding code block") {
		t.Fatalf("export-only module should not require a preceding code block\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintSkipsUndeclaredCommandRefsForExportedModules(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "module_command.md")
	writeFile(t, path, `## Shell helper

`+"```sh"+`
echo "$provided_by_consumer"
`+"```"+`
<!-- cheat
export shell_helper
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "variable \"provided_by_consumer\" referenced") {
		t.Fatalf("exported module should not warn on consumer-provided vars\nfindings:\n%s", formatFindings(findings))
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}

func hasFinding(findings []Finding, substr string) bool {
	for _, f := range findings {
		if strings.Contains(f.Message, substr) {
			return true
		}
	}
	return false
}

func formatFindings(findings []Finding) string {
	var b strings.Builder
	for _, f := range findings {
		b.WriteString(f.Format())
		b.WriteByte('\n')
	}
	return b.String()
}
