package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectEnvOptions_HasCurrentEnv(t *testing.T) {
	t.Setenv("CHEATMD_TEST_VAR", "hello-world")
	opts := collectEnvOptions()
	if len(opts) == 0 {
		t.Fatal("expected at least one env option")
	}
	var found bool
	for _, o := range opts {
		if strings.Contains(o.Display, "CHEATMD_TEST_VAR=hello-world") && o.Value == "hello-world" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected to find CHEATMD_TEST_VAR=hello-world in env options")
	}
}

func TestCollectHistoryOptions_OnlyAssignments(t *testing.T) {
	dir := t.TempDir()
	histPath := filepath.Join(dir, "history")
	content := strings.Join([]string{
		"ls -la",                               // plain command, ignored
		"export DOMAIN=corp.example.com",       // export-style
		"USER=alice ssh prod.example.com",      // leading inline assignment
		"grep foo bar",                         // ignored
		"export TOKEN=\"abc 123\"",             // double-quoted value
		"FOO=bar BAR=baz some-command --flag",  // two assignments in one line
		"declare -x DB_HOST=db.example.com",    // declare with flag
		": 1700000000:0;export ZSH_VAR=zvalue", // zsh-prefixed
		"ls -la",                               // ignored
		"DOMAIN=corp.example.com",              // duplicate value, deduped
	}, "\n") + "\n"
	if err := os.WriteFile(histPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HISTFILE", histPath)

	opts := collectHistoryOptions()

	// Build a map name -> value for easy assertions.
	got := make(map[string]string, len(opts))
	for _, o := range opts {
		// Display is "hist: NAME=value"; split out the key for indexing.
		body := strings.TrimPrefix(o.Display, "hist: ")
		eq := strings.IndexByte(body, '=')
		if eq <= 0 {
			t.Fatalf("unexpected display shape: %q", o.Display)
		}
		got[body[:eq]] = o.Value
	}

	want := map[string]string{
		"DOMAIN":  "corp.example.com",
		"USER":    "alice",
		"TOKEN":   "abc 123",
		"FOO":     "bar",
		"BAR":     "baz",
		"DB_HOST": "db.example.com",
		"ZSH_VAR": "zvalue",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("expected %s=%q, got %q", k, v, got[k])
		}
	}
	if _, ok := got["ls"]; ok {
		t.Errorf("plain command 'ls' should not appear as an assignment")
	}
	// Dedup: DOMAIN=corp.example.com appears twice in the file but should
	// produce a single row.
	domainCount := 0
	for _, o := range opts {
		if o.Display == "hist: DOMAIN=corp.example.com" {
			domainCount++
		}
	}
	if domainCount != 1 {
		t.Errorf("expected DOMAIN=corp.example.com deduped to 1 row, got %d", domainCount)
	}
}

func TestCollectSubstituteOptions_RespectsSources(t *testing.T) {
	t.Setenv("HISTFILE", "/dev/null/does-not-exist")
	t.Setenv("CHEATMD_TEST_ONLY", "yes")

	envOnly := collectSubstituteOptions([]string{"env"})
	if len(envOnly) == 0 {
		t.Fatal("expected env entries with sources=[env]")
	}
	for _, o := range envOnly {
		if !strings.HasPrefix(o.Display, "env: ") {
			t.Errorf("env-only mode returned non-env row: %q", o.Display)
		}
	}

	none := collectSubstituteOptions([]string{})
	if len(none) != 0 {
		t.Errorf("empty sources should return empty, got %d rows", len(none))
	}
}
