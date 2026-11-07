package core

import (
	"strings"
	"testing"
)

// A button click on a card platform must not post a "✅ question: answer"
// message (the clicked card already shows the answer); a typed answer still
// gets the confirmation.
func TestHandlePendingPermission_AskUserQuestion_EchoOnlyTypedAnswers(t *testing.T) {
	cases := []struct {
		name    string
		content string
		echo    bool
	}{
		{"button click", "askq:0:2", false},
		{"typed number", "2", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newTestEngine()
			p := &stubCardPlatform{stubPlatformEngine: stubPlatformEngine{n: "feishu"}}
			rec := &recordingAgentSession{}
			state := &interactiveState{
				agentSession: rec,
				platform:     p,
				replyCtx:     "ctx",
				pending: &pendingPermission{
					RequestID: "req-1",
					ToolName:  "AskUserQuestion",
					ToolInput: map[string]any{"questions": []any{map[string]any{"question": "Which?"}}},
					Questions: testQuestions(),
					Resolved:  make(chan struct{}),
				},
			}
			e.interactiveMu.Lock()
			e.interactiveStates["feishu:chat:user1"] = state
			e.interactiveMu.Unlock()

			if !e.handlePendingPermission(p, &Message{
				SessionKey: "feishu:chat:user1", UserID: "user1", Content: c.content, ReplyCtx: "ctx",
			}, c.content, "") {
				t.Fatal("expected the answer to be handled")
			}
			if rec.calls != 1 {
				t.Fatalf("expected 1 RespondPermission call, got %d", rec.calls)
			}
			echoed := false
			for _, m := range p.getSent() {
				if strings.HasPrefix(m, "✅ ") {
					echoed = true
				}
			}
			if echoed != c.echo {
				t.Fatalf("echo=%v want %v (sent %q)", echoed, c.echo, p.getSent())
			}
		})
	}
}
