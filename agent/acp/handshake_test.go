package acp

import (
	"encoding/json"
	"fmt"
	"testing"
)

// kimi acp's session/load response carries configOptions/modes but no
// sessionId echo. The handshake must treat an error-free load reply as
// success for the requested session — falling through to session/new would
// silently discard the just-loaded history (observed: resume spawned a fresh
// empty session even though the load had succeeded).
func TestSession_HandshakeLoadWithoutSessionIDKeepsRequested(t *testing.T) {
	s, wResp, rReq := newTestSession(t, &fakeCallbacks{})

	go func() {
		dec := json.NewDecoder(rReq)
		for {
			var req struct {
				ID     int    `json:"id"`
				Method string `json:"method"`
			}
			if err := dec.Decode(&req); err != nil {
				return
			}
			var resp string
			switch req.Method {
			case "initialize":
				resp = `{"jsonrpc":"2.0","id":%d,"result":{"protocolVersion":1,"agentCapabilities":{"loadSession":true},"agentInfo":{"name":"Kimi Code CLI","version":"2.1.1"},"authMethods":[]}}`
			case "session/load":
				// kimi-shaped reply: no sessionId, just modes/configOptions.
				resp = `{"jsonrpc":"2.0","id":%d,"result":{"modes":{"currentModeId":"yolo","availableModes":[{"id":"yolo","name":"Yolo"}]},"configOptions":[]}}`
			case "session/new":
				t.Errorf("session/new must not be called after a successful session/load")
				resp = `{"jsonrpc":"2.0","id":%d,"result":{"sessionId":"should-not-be-used","modes":{},"configOptions":[]}}`
			default:
				t.Errorf("unexpected request during handshake: %s", req.Method)
				continue
			}
			if _, err := fmt.Fprintf(wResp, resp+"\n", req.ID); err != nil {
				return
			}
		}
	}()

	if err := s.handshake("session_resume_target", ""); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if got := s.currentACPSessionID(); got != "session_resume_target" {
		t.Fatalf("currentACPSessionID = %q, want session_resume_target", got)
	}
}
