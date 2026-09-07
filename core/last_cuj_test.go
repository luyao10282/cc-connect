package core

import (
	"context"
	"strings"
	"testing"
	"time"
)

// cujHistoryAgent is a cujAgent that also serves transcript history, the way
// the real Claude Code / Codex agents do. It is what makes the "switched into
// a session this process never ran" journey testable.
type cujHistoryAgent struct {
	*cujAgent
	entries []HistoryEntry
	err     error
}

func (a *cujHistoryAgent) GetSessionHistory(_ context.Context, _ string, limit int) ([]HistoryEntry, error) {
	if a.err != nil {
		return nil, a.err
	}
	entries := a.entries
	if limit > 0 && len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	return entries, nil
}

// CUJ-LAST-1 · The user chats, gets a long answer, then asks /last. They must
// see the answer in full — /history truncates every entry to 1000 runes, which
// is what made it useless for "what did it just tell me?".
func TestCUJ_LAST1_ReplaysFullAnswer(t *testing.T) {
	env := newCUJEnv(t)

	longAnswer := "MIGRATION REPORT\n" + strings.Repeat("detail line about the migration\n", 120) + "\nFINAL: 3 tables rebuilt."
	if len([]rune(longAnswer)) <= defaultHistoryMaxLen {
		t.Fatalf("fixture must exceed the /history truncation budget (%d runes)", defaultHistoryMaxLen)
	}
	env.agent.setNextSessionEvents([]Event{{Type: EventResult, Content: longAnswer, Done: true}}, 0)

	key := env.userSends("alice", "run the migration")
	env.waitFor("agent replied", 3*time.Second, func() bool {
		return len(env.activeSession(key).GetHistory(0)) >= 2
	})

	env.plat.clearSent()
	env.userSends("alice", "/last")
	env.waitFor("/last answered", 3*time.Second, func() bool {
		return len(env.plat.getSent()) >= 1
	})

	seen := strings.Join(env.plat.getSent(), "")
	if !strings.Contains(seen, "FINAL: 3 tables rebuilt.") {
		t.Fatalf("/last dropped the tail of the answer; got %d chars", len(seen))
	}
	if !strings.Contains(seen, "run the migration") {
		t.Fatal("/last should echo the prompt that produced the answer")
	}
	// Long answers must arrive as several platform messages, not one oversized
	// blob the platform would reject.
	for _, msg := range env.plat.getSent() {
		if n := len([]rune(msg)); n > maxPlatformMessageLen {
			t.Fatalf("chunk of %d runes exceeds the platform limit %d", n, maxPlatformMessageLen)
		}
	}
}

// CUJ-LAST-2 · The user switches into a session that was created before this
// process started (the /list → /switch journey). In-memory history is empty, so
// /last must fall back to the agent's transcript — otherwise the user still has
// no idea what that session was doing, which is the whole reason for /last.
func TestCUJ_LAST2_FallsBackToAgentTranscriptAfterSwitch(t *testing.T) {
	env := newCUJEnv(t)
	agent := &cujHistoryAgent{cujAgent: env.agent, entries: []HistoryEntry{
		{Role: "user", Content: "why is the report empty?", Timestamp: time.Now()},
		{Role: "assistant", Content: "let me look", Timestamp: time.Now()},
		{Role: "assistant", Content: "Root cause: the ODS snapshot is a day late.", Timestamp: time.Now()},
	}}
	env.engine.agent = agent

	key := "test:alice"
	s := env.engine.sessions.GetOrCreateActive(key)
	s.SetAgentInfo("resumed-session-id", "cuj", "resumed")

	env.userSends("alice", "/last")
	env.waitFor("/last answered", 3*time.Second, func() bool {
		return len(env.plat.getSent()) >= 1
	})

	seen := strings.Join(env.plat.getSent(), "")
	if !strings.Contains(seen, "Root cause: the ODS snapshot is a day late.") {
		t.Fatalf("/last did not read the agent transcript; user saw: %q", seen)
	}
	if !strings.Contains(seen, "why is the report empty?") {
		t.Fatal("/last should show the prompt from the transcript too")
	}
}

// CUJ-LAST-3 · When the transcript cannot be read, the user must be told. The
// silent alternative — printing whatever older message is still reachable — is
// worse than an error, because it looks like a valid answer.
func TestCUJ_LAST3_ReportsTranscriptReadFailure(t *testing.T) {
	env := newCUJEnv(t)
	agent := &cujHistoryAgent{cujAgent: env.agent, err: context.DeadlineExceeded}
	env.engine.agent = agent

	key := "test:alice"
	env.engine.sessions.GetOrCreateActive(key).SetAgentInfo("resumed-session-id", "cuj", "resumed")

	env.userSends("alice", "/last")
	env.waitFor("/last answered", 3*time.Second, func() bool {
		return len(env.plat.getSent()) >= 1
	})

	seen := strings.Join(env.plat.getSent(), "")
	if !strings.Contains(seen, "transcript") && !strings.Contains(seen, context.DeadlineExceeded.Error()) {
		t.Fatalf("expected a read-failure message, user saw: %q", seen)
	}
}
