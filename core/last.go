package core

import (
	"log/slog"
	"strconv"
	"strings"
)

const (
	// lastAnswerLookback bounds how many recent transcript entries /last pulls
	// from the agent when the in-memory history cannot satisfy the request.
	lastAnswerLookback = 80
	// lastAnswerMaxCount caps /last <n> so a single command cannot flood the
	// channel with an unbounded number of full-length answers.
	lastAnswerMaxCount = 5
	// lastPromptPreviewLen is the rune budget for the "what was asked" line.
	// The answer itself is never truncated; only this preview is.
	lastPromptPreviewLen = 200
)

// cmdLast replays the most recent assistant reply of the active session in
// full. It exists because neither /current nor /list tells the user what a
// session was actually doing, and /history truncates every entry to
// display.history_max_len (1000 runes by default) while interleaving ten
// user/assistant turns — so a long answer is never readable there.
//
// Argument: optional count, /last 3 replays the last three assistant replies
// (oldest first), capped at lastAnswerMaxCount.
func (e *Engine) cmdLast(p Platform, msg *Message, args []string) {
	n := 1
	if len(args) > 0 {
		if v, err := strconv.Atoi(args[0]); err == nil && v > 0 {
			n = v
		}
	}
	if n > lastAnswerMaxCount {
		n = lastAnswerMaxCount
	}

	agent, sessions, _, workspaceDir, err := e.commandContextWithWorkspace(p, msg)
	if err != nil {
		e.reply(p, msg.ReplyCtx, e.i18n.Tf(MsgWsResolutionError, err))
		return
	}
	s := sessions.GetOrCreateActive(msg.SessionKey)

	entries := s.GetHistory(0)
	// A session resumed via /switch has an empty in-memory history — its turns
	// happened in an earlier process — so fall back to the agent's own
	// transcript, the same source /history uses. This is the whole point of
	// the command: "I just switched here, what was going on?".
	if countAssistantEntries(entries) < n {
		if agentSID := s.GetAgentSessionID(); agentSID != "" && agentSID != ContinueSession {
			if hp, ok := agent.(HistoryProvider); ok {
				agentEntries, herr := hp.GetSessionHistory(e.ctx, agentSID, lastAnswerLookback)
				switch {
				case herr != nil:
					slog.Warn("last: agent history unavailable", "session", agentSID, "error", herr)
					// Reporting the failure beats printing a stale answer as if
					// it were the latest one.
					if countAssistantEntries(entries) == 0 {
						e.reply(p, msg.ReplyCtx, e.i18n.Tf(MsgLastReadError, herr))
						return
					}
				case countAssistantEntries(agentEntries) > countAssistantEntries(entries):
					entries = agentEntries
				}
			}
		}
	}

	answers, prompt := pickLastAnswers(entries, n)
	if len(answers) == 0 {
		e.reply(p, msg.ReplyCtx, e.i18n.T(MsgLastEmpty))
		return
	}

	displayName := e.currentSessionDisplayName(agent, sessions, s.GetAgentSessionID())

	var sb strings.Builder
	sb.WriteString(e.i18n.Tf(MsgLastHeader, displayName,
		answers[len(answers)-1].Timestamp.Local().Format("01-02 15:04")))
	if prompt != "" {
		sb.WriteString("\n")
		sb.WriteString(e.i18n.Tf(MsgLastPrompt,
			truncateHistoryEntry(collapseWhitespace(prompt), lastPromptPreviewLen)))
	}
	for _, a := range answers {
		sb.WriteString("\n\n---\n\n")
		sb.WriteString(a.Content)
	}

	for _, chunk := range SplitMessageCodeFenceAware(sb.String(), maxPlatformMessageLen) {
		if err := e.sendWithErrorForWorkspace(p, msg.ReplyCtx, chunk, workspaceDir); err != nil {
			return
		}
	}
}

// countAssistantEntries counts entries that carry actual assistant text.
func countAssistantEntries(entries []HistoryEntry) int {
	count := 0
	for _, h := range entries {
		if h.Role == "assistant" && strings.TrimSpace(h.Content) != "" {
			count++
		}
	}
	return count
}

// pickLastAnswers returns up to n trailing assistant entries in chronological
// order, plus the user prompt that preceded the earliest of them (empty when
// there is none). Agent transcripts store one entry per content block, so the
// entries between two assistant texts (tool calls, tool results, thinking) are
// already filtered out by the HistoryProvider and simply do not appear here.
func pickLastAnswers(entries []HistoryEntry, n int) ([]HistoryEntry, string) {
	if n <= 0 {
		return nil, ""
	}
	var picked []HistoryEntry
	firstIdx := -1
	for i := len(entries) - 1; i >= 0 && len(picked) < n; i-- {
		if entries[i].Role != "assistant" || strings.TrimSpace(entries[i].Content) == "" {
			continue
		}
		picked = append(picked, entries[i])
		firstIdx = i
	}
	if len(picked) == 0 {
		return nil, ""
	}
	// Reverse into chronological order.
	for l, r := 0, len(picked)-1; l < r; l, r = l+1, r-1 {
		picked[l], picked[r] = picked[r], picked[l]
	}

	prompt := ""
	for i := firstIdx - 1; i >= 0; i-- {
		if entries[i].Role != "user" {
			continue
		}
		if text := strings.TrimSpace(entries[i].Content); text != "" && !isNoisePrompt(text) {
			prompt = text
		}
		break
	}
	return picked, prompt
}

// isNoisePrompt reports whether a user entry is agent plumbing rather than
// something the user typed (slash-command envelopes, injected reminders,
// command stdout replayed back into the transcript).
func isNoisePrompt(text string) bool {
	switch {
	case strings.HasPrefix(text, "<command-name>"),
		strings.HasPrefix(text, "<local-command-stdout>"),
		strings.HasPrefix(text, "<system-reminder>"),
		strings.HasPrefix(text, "Caveat:"):
		return true
	}
	return false
}

// collapseWhitespace folds a multi-line prompt into a single display line.
func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
