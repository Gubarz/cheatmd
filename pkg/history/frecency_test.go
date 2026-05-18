package history

import (
	"testing"
	"time"
)

func TestFrecencyScoresPreferRecentRepeatedCheats(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	recent := CheatKey("recent.md", "Recent")
	old := CheatKey("old.md", "Old")

	scores := FrecencyScores([]Entry{
		{Timestamp: now.Add(-time.Hour), File: "recent.md", Header: "Recent"},
		{Timestamp: now.Add(-2 * time.Hour), File: "recent.md", Header: "Recent"},
		{Timestamp: now.Add(-90 * 24 * time.Hour), File: "old.md", Header: "Old"},
		{Timestamp: now.Add(-91 * 24 * time.Hour), File: "old.md", Header: "Old"},
	}, now)

	if scores[recent] <= scores[old] {
		t.Fatalf("recent score %.4f should beat old score %.4f", scores[recent], scores[old])
	}
}

func TestFrecencyScoresIgnoreEntriesWithoutCheatRef(t *testing.T) {
	scores := FrecencyScores([]Entry{
		{Timestamp: time.Now(), Command: "echo hi"},
	}, time.Now())

	if len(scores) != 0 {
		t.Fatalf("scores = %v, want empty", scores)
	}
}
