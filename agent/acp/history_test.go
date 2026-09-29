package acp

import (
	"strings"
	"testing"
)

func TestParseWireHistory_V2(t *testing.T) {
	wireJSONL := `
{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"hello world"}]},"meta":{"origin":{"kind":"user"},"source":"input"}}}
{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"think","think":"internal thought"},{"type":"text","text":"hi there"}]}}}
{"type":"agent.message.appended","message":{"message":{"role":"user","content":[{"type":"text","text":"<system-reminder>ignore me"}]},"meta":{"origin":{"kind":"user"}}}}
{"type":"agent.message.appended","message":{"message":{"role":"assistant","content":[{"type":"text","text":"second reply"}]}}}
`
	entries, err := parseWireHistory(strings.NewReader(strings.TrimSpace(wireJSONL)), 10)
	if err != nil {
		t.Fatalf("parseWireHistory error: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	if entries[0].Role != "user" || entries[0].Content != "hello world" {
		t.Errorf("entry 0: %+v", entries[0])
	}
	if entries[1].Role != "assistant" || entries[1].Content != "hi there" {
		t.Errorf("entry 1: %+v", entries[1])
	}
	if entries[2].Role != "assistant" || entries[2].Content != "second reply" {
		t.Errorf("entry 2: %+v", entries[2])
	}
}

func TestParseWireHistory_V1(t *testing.T) {
	wireJSONL := `
{"type":"context.append_message","message":{"role":"user","content":[{"type":"text","text":"v1 question"}]},"time":1700000000000}
{"type":"context.append_message","message":{"role":"assistant","content":[{"type":"text","text":"v1 answer"}]},"time":1700000001000}
`
	entries, err := parseWireHistory(strings.NewReader(strings.TrimSpace(wireJSONL)), 10)
	if err != nil {
		t.Fatalf("parseWireHistory error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Content != "v1 question" || entries[1].Content != "v1 answer" {
		t.Errorf("entries mismatch: %+v", entries)
	}
}

func TestParseWireHistory_Limit(t *testing.T) {
	wireJSONL := `
{"type":"context.append_message","message":{"role":"user","content":"q1"}}
{"type":"context.append_message","message":{"role":"assistant","content":"a1"}}
{"type":"context.append_message","message":{"role":"user","content":"q2"}}
{"type":"context.append_message","message":{"role":"assistant","content":"a2"}}
`
	entries, err := parseWireHistory(strings.NewReader(strings.TrimSpace(wireJSONL)), 2)
	if err != nil {
		t.Fatalf("parseWireHistory error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Content != "q2" || entries[1].Content != "a2" {
		t.Errorf("entries mismatch: %+v", entries)
	}
}
