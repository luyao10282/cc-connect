package core

import (
	"testing"
	"time"
)

func histEntry(role, content string) HistoryEntry {
	return HistoryEntry{Role: role, Content: content, Timestamp: time.Now()}
}

func TestPickLastAnswersReturnsFinalAnswerWithPrompt(t *testing.T) {
	entries := []HistoryEntry{
		histEntry("user", "first question"),
		histEntry("assistant", "first answer"),
		histEntry("user", "second question\nwith a newline"),
		histEntry("assistant", "let me check"),
		histEntry("assistant", "second answer"),
	}

	answers, prompt := pickLastAnswers(entries, 1)
	if len(answers) != 1 {
		t.Fatalf("answers = %d, want 1", len(answers))
	}
	if answers[0].Content != "second answer" {
		t.Fatalf("answer = %q, want %q", answers[0].Content, "second answer")
	}
	// The preamble text block sits between the prompt and the final answer, so
	// the prompt lookup must skip assistant entries, not stop at them.
	if prompt != "second question\nwith a newline" {
		t.Fatalf("prompt = %q", prompt)
	}
}

func TestPickLastAnswersMultipleInChronologicalOrder(t *testing.T) {
	entries := []HistoryEntry{
		histEntry("user", "q1"),
		histEntry("assistant", "a1"),
		histEntry("user", "q2"),
		histEntry("assistant", "a2"),
		histEntry("user", "q3"),
		histEntry("assistant", "a3"),
	}

	answers, prompt := pickLastAnswers(entries, 2)
	if len(answers) != 2 {
		t.Fatalf("answers = %d, want 2", len(answers))
	}
	if answers[0].Content != "a2" || answers[1].Content != "a3" {
		t.Fatalf("answers = %q, %q; want a2, a3", answers[0].Content, answers[1].Content)
	}
	if prompt != "q2" {
		t.Fatalf("prompt = %q, want q2", prompt)
	}
}

func TestPickLastAnswersSkipsBlankAndNoisePrompts(t *testing.T) {
	entries := []HistoryEntry{
		histEntry("user", "real question"),
		histEntry("assistant", "   "),
		histEntry("user", "<command-name>/compact</command-name>"),
		histEntry("assistant", "real answer"),
	}

	answers, prompt := pickLastAnswers(entries, 1)
	if len(answers) != 1 || answers[0].Content != "real answer" {
		t.Fatalf("answers = %+v", answers)
	}
	if prompt != "" {
		t.Fatalf("prompt = %q, want empty (plumbing entry must not be shown)", prompt)
	}
}

func TestPickLastAnswersEmpty(t *testing.T) {
	answers, prompt := pickLastAnswers([]HistoryEntry{histEntry("user", "hi")}, 1)
	if len(answers) != 0 || prompt != "" {
		t.Fatalf("answers = %+v, prompt = %q; want empty", answers, prompt)
	}
	if answers, _ := pickLastAnswers(nil, 0); len(answers) != 0 {
		t.Fatalf("n=0 must return nothing")
	}
}

func TestCountAssistantEntriesIgnoresBlank(t *testing.T) {
	entries := []HistoryEntry{
		histEntry("assistant", "a"),
		histEntry("assistant", "\n\t "),
		histEntry("user", "u"),
	}
	if got := countAssistantEntries(entries); got != 1 {
		t.Fatalf("count = %d, want 1", got)
	}
}

func TestLastCommandIsRegistered(t *testing.T) {
	if got := matchPrefix("last", builtinCommands); got != "last" {
		t.Fatalf("matchPrefix(last) = %q, want last", got)
	}
	if got := matchPrefix("recap", builtinCommands); got != "last" {
		t.Fatalf("matchPrefix(recap) = %q, want last", got)
	}
	// Every builtin renders its help text via MsgKey(id); a missing entry
	// would surface as a raw key in /help and in platform command menus.
	i18n := NewI18n(LangChinese)
	if desc := i18n.T(MsgKey("last")); desc == "" || desc == "last" {
		t.Fatalf("missing i18n description for /last: %q", desc)
	}
}
