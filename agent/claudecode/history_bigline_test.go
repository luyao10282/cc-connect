package claudecode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTranscript lays out ~/.claude/projects/<key>/<sid>.jsonl for a fake HOME
// and returns the session ID.
func writeTranscript(t *testing.T, homeDir, workDir string, lines []string) string {
	t.Helper()
	projectDir := filepath.Join(homeDir, ".claude", "projects", encodeClaudeProjectKey(workDir))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}
	sessionID := "session-under-test"
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, sessionID+".jsonl"), []byte(body), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}
	return sessionID
}

func transcriptLine(t *testing.T, typ, text string) string {
	t.Helper()
	entry := map[string]any{
		"type":      typ,
		"timestamp": "2026-09-07T10:00:00.000Z",
		"message": map[string]any{
			"role":    typ,
			"content": []map[string]any{{"type": "text", "text": text}},
		},
	}
	b, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// A single JSONL record holds a whole tool_result, and real transcripts do
// exceed 256KB (the old scanner cap). Before the fix bufio.Scanner stopped at
// the oversized line and everything after it — including the newest answer —
// was invisible, so /history and /last would show a stale message as "latest".
func TestGetSessionHistoryReadsPastOversizedLine(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := filepath.Join(homeDir, "Documents", "proj")

	huge := transcriptLine(t, "user", strings.Repeat("x", 400*1024))
	if len(huge) < 256*1024 {
		t.Fatalf("fixture line is %d bytes, expected > 256KB", len(huge))
	}
	sessionID := writeTranscript(t, homeDir, workDir, []string{
		transcriptLine(t, "user", "old question"),
		transcriptLine(t, "assistant", "old answer"),
		huge,
		transcriptLine(t, "assistant", "newest answer"),
	})

	a := &Agent{workDir: workDir}
	entries, err := a.GetSessionHistory(context.Background(), sessionID, 10)
	if err != nil {
		t.Fatalf("GetSessionHistory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no entries returned")
	}
	last := entries[len(entries)-1]
	if last.Content != "newest answer" {
		t.Fatalf("last entry = %q, want %q (scanner stopped early?)", last.Content, "newest answer")
	}
}

func TestGetSessionHistoryMissingFileErrors(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := filepath.Join(homeDir, "Documents", "proj")
	writeTranscript(t, homeDir, workDir, []string{transcriptLine(t, "assistant", "hi")})

	a := &Agent{workDir: workDir}
	if _, err := a.GetSessionHistory(context.Background(), "no-such-session", 10); err == nil {
		t.Fatal("expected an error for a missing transcript, got nil")
	}
}
