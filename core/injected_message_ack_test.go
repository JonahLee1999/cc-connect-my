package core

import (
	"context"
	"errors"
	"sync"
	"testing"
)

type injectionReactionPlatform struct {
	*stubPlatformEngine
	reactionMu sync.Mutex
	reactions  []string
}

func (p *injectionReactionPlatform) AcknowledgeMessageInjection(_ context.Context, msg *Message) {
	p.reactionMu.Lock()
	defer p.reactionMu.Unlock()
	p.reactions = append(p.reactions, msg.MessageID)
}

func (p *injectionReactionPlatform) getReactions() []string {
	p.reactionMu.Lock()
	defer p.reactionMu.Unlock()
	return append([]string(nil), p.reactions...)
}

func TestBusyMessage_ReactionAcknowledgementReplacesTextOnlyAfterSuccessfulInjection(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "injected", true: "queue fallback"}[fail], func(t *testing.T) {
			sess := newMidTurnSession("reaction-injection")
			if fail {
				sess.sendErr = errors.New("send failed")
			}
			e, base, _, _ := setupBusySession(t, sess)
			p := &injectionReactionPlatform{stubPlatformEngine: base}
			e.ReceiveMessage(p, busyMessage("include the new information"))
			if sentContains(p.getSent(), e.i18n.T(MsgMessageInjected)) {
				t.Fatalf("unexpected text injection receipt: %v", p.getSent())
			}
			reactions := p.getReactions()
			if fail {
				if len(reactions) != 0 || !sentContains(p.getSent(), e.i18n.T(MsgMessageQueued)) {
					t.Fatalf("failed injection: reactions=%v replies=%v", reactions, p.getSent())
				}
			} else if len(reactions) != 1 || reactions[0] != "msg-busy" || len(p.getSent()) != 0 {
				t.Fatalf("successful injection: reactions=%v replies=%v", reactions, p.getSent())
			}
		})
	}
}
