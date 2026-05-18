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
var domain = printf '%s\n' '$domain' "$(grep -v '^[[:space:]]*#' /etc/hosts \
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

func TestLintAcceptsPromptOnlyVarWithArgs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.md")
	writeFile(t, path, `# Deploy

## Sync

`+"```sh"+`
rsync -a $source $dest
`+"```"+`
<!-- cheat
var sync_method = printf 'fast\tFast\nslow\tSlow\n' --- --delimiter '\t'

if $sync_method != slow
var dest --- --header "Destination"
fi

if $sync_method == fast
var dest := /tmp/sync
fi
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "missing an assignment operator") {
		t.Fatalf("prompt-only var with args should be valid\nfindings:\n%s", formatFindings(findings))
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

func TestLintAcceptsChainAndReportsDuplicateSteps(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "one.md"), `## Step one

`+"```sh"+`
echo one
`+"```"+`
<!-- cheat
chain demo 1
-->
`)
	writeFile(t, filepath.Join(dir, "two.md"), `## Step one duplicate

`+"```sh"+`
echo dup
`+"```"+`
<!-- cheat
chain demo 1
-->
`)

	findings, err := Lint(dir)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "unknown DSL keyword \"chain\"") {
		t.Fatalf("chain should be a valid DSL keyword\nfindings:\n%s", formatFindings(findings))
	}
	if !hasFinding(findings, "duplicate chain step \"demo\" 1") {
		t.Fatalf("missing duplicate chain step finding\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintReportsInvalidChainLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad_chain.md")
	writeFile(t, path, `## Bad

`+"```sh"+`
echo bad
`+"```"+`
<!-- cheat
chain demo nope
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "`chain` step must be a positive number") {
		t.Fatalf("missing invalid chain step finding\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintReportsChainGaps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gap.md")
	writeFile(t, path, `## Later

`+"```sh"+`
echo later
`+"```"+`
<!-- cheat
chain demo 2
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "chain \"demo\" is missing step 1") {
		t.Fatalf("missing chain gap finding\nfindings:\n%s", formatFindings(findings))
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
	writeFile(t, path, `# Server

<!-- cheat
export interface
var interface
-->

## Server

`+"```sh"+`
python3 -m http.server -b $interface
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

func TestLintShellSyntaxDeclarationsAndTemplateRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.md")
	writeFile(t, path, `## Loop

`+"```sh"+`
for i in {1..10}; do echo <a>.$i; done
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "variable \"a\" referenced") {
		t.Fatalf("missing template ref finding\nfindings:\n%s", formatFindings(findings))
	}
	if hasFinding(findings, "variable \"i\" referenced") {
		t.Fatalf("shell for variable should be syntax-declared\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintShellSpecialsDoNotApplyToAngleRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "home.md")
	writeFile(t, path, `## Home

`+"```sh"+`
echo "$HOME" "<HOME>" "$1" "${10}"
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "variable \"HOME\" referenced") {
		t.Fatalf("<HOME> should warn even though $HOME is shell-special\nfindings:\n%s", formatFindings(findings))
	}
	if countFindings(findings, "variable \"HOME\" referenced") != 1 {
		t.Fatalf("only <HOME> should warn, not $HOME\nfindings:\n%s", formatFindings(findings))
	}
	if hasFinding(findings, "variable \"1\" referenced") || hasFinding(findings, "variable \"10\" referenced") {
		t.Fatalf("numeric shell positional params should be special\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintPowerShellSyntaxDeclarationsAndAutomatics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ps.md")
	writeFile(t, path, `## Compare

`+"```powershell"+`
while($true) {
  $process = Get-WmiObject Win32_Process
  $process2 = Get-WmiObject Win32_Process
  Compare-Object $process $process2
}
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	for _, name := range []string{"true", "process", "process2"} {
		if hasFinding(findings, "variable \""+name+"\" referenced") {
			t.Fatalf("PowerShell %s should not warn\nfindings:\n%s", name, formatFindings(findings))
		}
	}
}

func TestLintPowerShellWarnsForUndeclaredInputButNotAssignment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ps_input.md")
	writeFile(t, path, `## Parse

`+"```ps1"+`
$obj = ConvertFrom-Json $input_data
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "variable \"input_data\" referenced") {
		t.Fatalf("missing undeclared PowerShell input finding\nfindings:\n%s", formatFindings(findings))
	}
	if hasFinding(findings, "variable \"obj\" referenced") {
		t.Fatalf("assignment-declared PowerShell variable should not warn\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintPowerShellProviderNamespacesDoNotWarn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ps_env.md")
	writeFile(t, path, `## AppData

`+"```powershell"+`
Get-ChildItem $env:APPDATA\MyApp\
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "variable \"env\" referenced") {
		t.Fatalf("PowerShell provider namespace $env: should not warn\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintInfersPowerShellInShellFence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ps_sh.md")
	writeFile(t, path, `## Filter

`+"```sh"+`
Get-Process | Where-Object { $_.Responding -eq $false -or $_.Name -ne $null }
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	for _, name := range []string{"_", "false", "null"} {
		if hasFinding(findings, "variable \""+name+"\" referenced") {
			t.Fatalf("PowerShell-looking sh fence should not warn for %s\nfindings:\n%s", name, formatFindings(findings))
		}
	}
}

func TestLintEmbeddedTclDeclarationsInShellFence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tcl.md")
	writeFile(t, path, `## Tcl

`+"```sh"+`
tclsh
set s value
gets $s c
set e $c
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	for _, name := range []string{"s", "c", "e"} {
		if hasFinding(findings, "variable \""+name+"\" referenced") {
			t.Fatalf("embedded Tcl variable %s should not warn\nfindings:\n%s", name, formatFindings(findings))
		}
	}
}

func TestLintEmbeddedPerlAndPHPDeclarations(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "embedded.md")
	writeFile(t, path, `## Perl

`+"```sh"+`
perl -e '$s="$server"; my $fh = undef; $content = <$fh>; print $s;'
perl -e 'open(my $handle, ">", "$file_out"); print $handle "ok";'
`+"```"+`
<!-- cheat
var server
var file_out
-->

## PHP

`+"```sh"+`
php -r '$p = array(); $h = proc_open("$cmd", $p, $pipes); echo $pipes[1];'
`+"```"+`
<!-- cheat
var cmd
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	for _, name := range []string{"s", "fh", "content", "handle", "p", "h", "pipes"} {
		if hasFinding(findings, "variable \""+name+"\" referenced") {
			t.Fatalf("embedded interpreter local %s should not warn\nfindings:\n%s", name, formatFindings(findings))
		}
	}
}

func TestLintEmbeddedPowerShellInCmdFence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cmd_ps.md")
	writeFile(t, path, `## Cmd PS

`+"```cmd"+`
powershell.exe -c "$e=New-Object -ComObject wscript.shell;$e.Popup('$file_out')"
`+"```"+`
<!-- cheat
var file_out
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if hasFinding(findings, "variable \"e\" referenced") {
		t.Fatalf("embedded PowerShell assignment should declare e\nfindings:\n%s", formatFindings(findings))
	}
}

func TestLintMethodChainsDoNotWarn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "methods.md")
	writeFile(t, path, `## Methods

`+"```powershell"+`
$obj.Document.Application.ShellExecute("cmd.exe","/c $command","C:\Windows\System32",$null,0)
$com.Application.ActivateMicrosoftApp("5")
`+"```"+`
<!-- cheat
var command
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	for _, name := range []string{"obj", "com"} {
		if hasFinding(findings, "variable \""+name+"\" referenced") {
			t.Fatalf("method chain object %s should not warn\nfindings:\n%s", name, formatFindings(findings))
		}
	}
}

func TestLintShellSingleQuotedRegexDoesNotWarn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grep.md")
	writeFile(t, path, `## Regex

`+"```sh"+`
grep -e '\($_GET\|$REQUEST\)' --color
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	for _, name := range []string{"_GET", "REQUEST"} {
		if hasFinding(findings, "variable \""+name+"\" referenced") {
			t.Fatalf("single-quoted shell regex %s should not warn\nfindings:\n%s", name, formatFindings(findings))
		}
	}
}

func TestLintDoesNotTreatHeredocXMLTagsAsAngleRefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "heredoc.md")
	writeFile(t, path, `## XML

`+"```sh"+`
cat >$tmp_file <<EOF
<domain>
  <name>x</name>
  <script path='$cmd_file'/>
</domain>
EOF
`+"```"+`
<!-- cheat
var cmd_file
var tmp_file
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	for _, name := range []string{"domain", "name", "script"} {
		if hasFinding(findings, "variable \""+name+"\" referenced") {
			t.Fatalf("heredoc XML tag %s should not be an angle template ref\nfindings:\n%s", name, formatFindings(findings))
		}
	}
}

func TestLintUnknownLanguageDollarRefsAreStrict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown.md")
	writeFile(t, path, `## Unknown

`+"```python"+`
print($HOME)
`+"```"+`
<!-- cheat
-->
`)

	findings, err := Lint(path)
	if err != nil {
		t.Fatalf("Lint returned error: %v", err)
	}

	if !hasFinding(findings, "variable \"HOME\" referenced") {
		t.Fatalf("unknown-language $HOME should be strict, not shell-special\nfindings:\n%s", formatFindings(findings))
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

func countFindings(findings []Finding, substr string) int {
	count := 0
	for _, f := range findings {
		if strings.Contains(f.Message, substr) {
			count++
		}
	}
	return count
}

func formatFindings(findings []Finding) string {
	var b strings.Builder
	for _, f := range findings {
		b.WriteString(f.Format())
		b.WriteByte('\n')
	}
	return b.String()
}
