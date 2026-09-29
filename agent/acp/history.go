package acp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

// This file implements core.HistoryProvider for ACP backends that persist
// transcripts locally but define no history retrieval RPC — concretely Kimi
// Code CLI (wire.jsonl) and the legacy kimi-cli (context.jsonl). It is a
// self-contained reader: agent packages must not import each other, so the
// equivalent logic in agent/kimi cannot be reused from here.

// maxHistoryLineSize is 8MB, matching the Claude transcript scanner cap
// to tolerate large tool outputs and thinking payloads without silent truncation.
const maxHistoryLineSize = 8 * 1024 * 1024

// getKimiSessionHistory finds and reads the transcript for a Kimi session
// (either legacy kimi-cli or modern Kimi Code CLI).
func getKimiSessionHistory(sessionID string, limit int) ([]core.HistoryEntry, error) {
	sessionDir, err := findKimiSessionDir(sessionID)
	if err != nil {
		return nil, err
	}
	return readSessionHistory(sessionDir, limit)
}

// kimiHistoryRoots returns the known session storage roots: the legacy
// kimi-cli root and the Kimi Code CLI root ($KIMI_CODE_HOME, default
// ~/.kimi-code).
func kimiHistoryRoots() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	codeHome := os.Getenv("KIMI_CODE_HOME")
	if codeHome == "" {
		codeHome = filepath.Join(homeDir, ".kimi-code")
	}
	return []string{
		filepath.Join(homeDir, ".kimi", "sessions"),
		filepath.Join(codeHome, "sessions"),
	}
}

// findKimiSessionDir locates a session directory by ID across known storage roots.
func findKimiSessionDir(sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", fmt.Errorf("acp: empty session id")
	}

	normID := strings.TrimPrefix(sessionID, "session_")

	for _, base := range kimiHistoryRoots() {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			projectDir := filepath.Join(base, entry.Name())
			subEntries, err := os.ReadDir(projectDir)
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if !se.IsDir() {
					continue
				}
				name := se.Name()
				if name == sessionID || strings.TrimPrefix(name, "session_") == normID {
					return filepath.Join(projectDir, name), nil
				}
			}
		}
	}

	return "", fmt.Errorf("acp: session directory not found for %q", sessionID)
}

// readSessionHistory parses conversation history from a Kimi session directory.
func readSessionHistory(sessionDir string, limit int) ([]core.HistoryEntry, error) {
	wirePath := filepath.Join(sessionDir, "agents", "main", "wire.jsonl")
	if f, err := os.Open(wirePath); err == nil {
		defer func() { _ = f.Close() }()
		return parseWireHistory(f, limit)
	}

	contextPath := filepath.Join(sessionDir, "context.jsonl")
	if f, err := os.Open(contextPath); err == nil {
		defer func() { _ = f.Close() }()
		return parseContextHistory(f, limit)
	}

	return nil, fmt.Errorf("acp: no transcript found in %q", sessionDir)
}

func parseWireHistory(r io.Reader, limit int) ([]core.HistoryEntry, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 256*1024), maxHistoryLineSize)

	var entries []core.HistoryEntry

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event struct {
			Type    string          `json:"type"`
			Message json.RawMessage `json:"message"`
			Time    int64           `json:"time"`
		}

		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}

		var role string
		var rawContent json.RawMessage
		var ts time.Time

		switch event.Type {
		case "agent.message.appended":
			var v2 struct {
				Message struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"message"`
				Meta struct {
					Source    string    `json:"source"`
					CreatedAt time.Time `json:"createdAt"`
					Origin    struct {
						Kind string `json:"kind"`
					} `json:"origin"`
				} `json:"meta"`
			}
			if err := json.Unmarshal(event.Message, &v2); err != nil {
				continue
			}
			role = v2.Message.Role
			rawContent = v2.Message.Content
			ts = v2.Meta.CreatedAt
			if role == "user" {
				// Only include genuine user inputs, not injected reminders or tool results
				if v2.Meta.Origin.Kind != "user" && v2.Meta.Source != "input" {
					continue
				}
			}

		case "context.append_message":
			var v1 struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}
			if err := json.Unmarshal(event.Message, &v1); err != nil {
				continue
			}
			role = v1.Role
			rawContent = v1.Content
			if event.Time > 0 {
				ts = time.UnixMilli(event.Time)
			}

		default:
			continue
		}

		if role != "user" && role != "assistant" {
			continue
		}

		text := extractTextContent(rawContent)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}

		// Skip system prompt injections that appear as user turns
		if role == "user" && strings.HasPrefix(text, "<system-reminder>") {
			continue
		}

		entries = append(entries, core.HistoryEntry{
			Role:      role,
			Content:   text,
			Timestamp: ts,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("acp: scan wire.jsonl: %w", err)
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	return entries, nil
}

func parseContextHistory(r io.Reader, limit int) ([]core.HistoryEntry, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 256*1024), maxHistoryLineSize)

	var entries []core.HistoryEntry

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var row struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Timestamp int64  `json:"timestamp"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			continue
		}

		if row.Role != "user" && row.Role != "assistant" {
			continue
		}
		text := strings.TrimSpace(row.Content)
		if text == "" || (row.Role == "user" && strings.HasPrefix(text, "<system-reminder>")) {
			continue
		}

		var ts time.Time
		if row.Timestamp > 0 {
			ts = time.Unix(row.Timestamp, 0)
		}

		entries = append(entries, core.HistoryEntry{
			Role:      row.Role,
			Content:   text,
			Timestamp: ts,
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("acp: scan context.jsonl: %w", err)
	}

	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	return entries, nil
}

func extractTextContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct
	}

	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var sb strings.Builder
		for _, p := range parts {
			if p.Type == "text" && p.Text != "" {
				sb.WriteString(p.Text)
			}
		}
		return sb.String()
	}

	return ""
}
