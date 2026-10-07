package acp

import (
	"encoding/json"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

// Regression: during session/load the agent replays history as session/update
// notifications. Mapped replay events must not be emitted to s.events — the
// engine only starts draining that channel after the handshake returns, so on
// long histories the buffer fills, the transport pump blocks in emit, the
// agent blocks on a full stdout pipe, and the load response never arrives.
// State absorption (tool input cache) must still run during replay.
func TestSession_LoadReplayUpdatesAreAbsorbedNotEmitted(t *testing.T) {
	cb := &fakeCallbacks{}
	s, _, _ := newTestSession(t, cb)

	textUpdate := json.RawMessage(`{"sessionId":"test-session-id","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"replayed assistant text"}}}`)
	toolUpdate := json.RawMessage(`{"sessionId":"test-session-id","update":{"sessionUpdate":"tool_call","toolCallId":"tc-1","kind":"execute","rawInput":{"command":"ls"}}}`)

	// During session/load replay: nothing is emitted, state is still absorbed.
	s.replayInFlight.Add(1)
	s.onNotification("session/update", textUpdate)
	s.onNotification("session/update", toolUpdate)
	select {
	case ev := <-s.events:
		t.Fatalf("replay update leaked into events channel: %+v", ev)
	default:
	}
	if got := s.toolInputByID["tc-1"]; got == "" {
		t.Fatal("tool input was not cached during replay absorption")
	}

	// After the load settles: live updates emit again.
	s.replayInFlight.Add(-1)
	s.onNotification("session/update", textUpdate)
	select {
	case ev := <-s.events:
		if ev.Type != core.EventText || ev.Content != "replayed assistant text" {
			t.Fatalf("unexpected live event: %+v", ev)
		}
	default:
		t.Fatal("live update after replay was not emitted")
	}
}
