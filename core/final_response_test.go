package core

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type finalResponseRecord struct {
	replyCtx any
	content  string
}

type finalResponsePlatform struct {
	stubRichCardSilentPlatform
	finalMu      sync.Mutex
	finals       []finalResponseRecord
	failDelivery bool
}

func (p *finalResponsePlatform) KeepPreviewOnFinish() bool { return true }

func (p *finalResponsePlatform) OnFinalResponseDelivered(_ context.Context, replyCtx any, content string) {
	p.finalMu.Lock()
	defer p.finalMu.Unlock()
	p.finals = append(p.finals, finalResponseRecord{replyCtx, content})
}

func (p *finalResponsePlatform) finalResponses() []finalResponseRecord {
	p.finalMu.Lock()
	defer p.finalMu.Unlock()
	return append([]finalResponseRecord(nil), p.finals...)
}

func (p *finalResponsePlatform) UpdateMessage(ctx context.Context, replyCtx any, content string) error {
	if p.failDelivery {
		return errors.New("delivery failed")
	}
	return p.stubRichCardSilentPlatform.UpdateMessage(ctx, replyCtx, content)
}

func (p *finalResponsePlatform) Send(ctx context.Context, replyCtx any, content string) error {
	if p.failDelivery {
		return errors.New("delivery failed")
	}
	return p.stubPlatformEngine.Send(ctx, replyCtx, content)
}

// A final rich-card/preview update bypasses Platform.Send; the observer must
// still see the original final text and the originating thread context.
func TestProcessInteractiveEvents_FinalResponseAfterSuccessfulDelivery(t *testing.T) {
	for _, tc := range []struct {
		name, cardMode, content string
		fail, agentError        bool
		want                    int
	}{
		{"rich", "rich", "Verified\n[[DONE]]", false, false, 1},
		{"preview", "legacy", "Verified\n[[DONE]]", false, false, 1},
		{"silent", "rich", "NO_REPLY", false, false, 0},
		{"delivery failure", "rich", "Verified\n[[DONE]]", true, false, 0},
		{"agent failure", "rich", "Verified\n[[DONE]]", false, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &finalResponsePlatform{failDelivery: tc.fail}
			p.n = "test"
			e := NewEngine("test", &stubAgent{}, []Platform{p}, "", LangEnglish)
			e.SetDisplayConfig(DisplayCfg{Mode: "full", CardMode: tc.cardMode, ToolMessages: true})
			e.SetStreamPreviewCfg(StreamPreviewCfg{Enabled: true, IntervalMs: 1, MinDeltaChars: 1})
			key := "test:chat:root:task"
			session := e.sessions.GetOrCreateActive(key)
			as := newControllableSession("task-session")
			state := &interactiveState{agentSession: as, platform: p, replyCtx: "original-thread"}
			e.interactiveStates[key] = state
			as.events <- Event{Type: EventText, Content: tc.content}
			if tc.agentError {
				as.events <- Event{Type: EventError, Error: errors.New("agent failed")}
			} else {
				as.events <- Event{Type: EventResult, Content: tc.content, Done: true}
			}
			e.processInteractiveEvents(state, session, e.sessions, key, "task-message", time.Now(), nil, nil, state.replyCtx, 0)
			got := p.finalResponses()
			if len(got) != tc.want {
				t.Fatalf("final responses = %#v, want %d", got, tc.want)
			}
			if tc.want == 1 && (got[0].replyCtx != "original-thread" || got[0].content != tc.content) {
				t.Fatalf("lost original final response/context: %#v", got)
			}
			if tc.want == 1 {
				_, _, updates, _ := p.snapshot()
				if !strings.Contains(strings.Join(updates, "\n"), "Verified") {
					t.Fatal("expected final delivery through card/preview update")
				}
			}
		})
	}
}
