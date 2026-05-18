// Package history records and reads the user's cheat execution history.
//
// Entries are stored newline-delimited JSON in $XDG_DATA_HOME/cheatmd/history.jsonl
// (falling back to ~/.local/share/cheatmd/history.jsonl). Each entry captures
// the final substituted command plus the source cheat reference and the
// resolved scope, so re-running can re-prefill variables.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one recorded execution of a cheat.
type Entry struct {
	Timestamp time.Time         `json:"ts"`
	Command   string            `json:"cmd"`
	File      string            `json:"file"`
	Header    string            `json:"header"`
	Scope     map[string]string `json:"scope,omitempty"`
}

const frecencyHalfLife = 14 * 24 * time.Hour

// DefaultPath returns the canonical history file path. The override is used
// verbatim if non-empty; otherwise $XDG_DATA_HOME/cheatmd/history.jsonl is
// preferred, with ~/.local/share/cheatmd/history.jsonl as the fallback.
func DefaultPath(override string) (string, error) {
	if override = strings.TrimSpace(override); override != "" {
		if strings.HasPrefix(override, "~/") {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			return filepath.Join(home, override[2:]), nil
		}
		return override, nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "cheatmd", "history.jsonl"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "cheatmd", "history.jsonl"), nil
}

// Append writes one entry to the history file. The file and any parent
// directories are created on demand. Errors writing history are non-fatal
// to the caller; surface them only for logging/diagnostics.
func Append(path string, e Entry) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return enc.Encode(e)
}

// Load returns up to maxEntries most-recent entries from path, newest first.
// A missing file is not an error; an empty slice is returned.
func Load(path string, maxEntries int) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	entries := make([]Entry, 0, 256)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip corrupt lines silently
		}
		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return entries, err
	}

	// Newest first.
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	if maxEntries > 0 && len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}
	return entries, nil
}

// CheatKey returns the stable history identity for a parsed cheat. It matches
// recorded executions by source file and section header, so command edits still
// keep useful ranking history.
func CheatKey(file, header string) string {
	return file + "\x00" + header
}

// FrecencyScores aggregates execution history into zoxide-style scores:
// repeated use increases the score while older executions decay over time.
func FrecencyScores(entries []Entry, now time.Time) map[string]float64 {
	if now.IsZero() {
		now = time.Now()
	}
	scores := make(map[string]float64)
	for _, entry := range entries {
		if entry.File == "" || entry.Header == "" {
			continue
		}
		age := now.Sub(entry.Timestamp)
		if entry.Timestamp.IsZero() || age < 0 {
			age = 0
		}
		weight := math.Pow(0.5, float64(age)/float64(frecencyHalfLife))
		scores[CheatKey(entry.File, entry.Header)] += weight
	}
	return scores
}

// Display renders an entry as a single line for picker display. Long commands
// are truncated to keep each row to one screen line.
func (e Entry) Display(maxWidth int) string {
	ts := e.Timestamp.Local().Format("2006-01-02 15:04")
	cmd := strings.ReplaceAll(e.Command, "\n", " ")
	prefix := fmt.Sprintf("%s  ", ts)
	avail := maxWidth - len(prefix)
	if avail < 10 {
		avail = 10
	}
	if len(cmd) > avail {
		cmd = cmd[:avail-1] + "…"
	}
	return prefix + cmd
}
