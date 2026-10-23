package feishu

import (
	"context"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func raStr(s string) *string { return &s }

func reactionEvent(operatorType, openID, emoji string) *larkim.P2MessageReactionCreatedV1 {
	return &larkim.P2MessageReactionCreatedV1{Event: &larkim.P2MessageReactionCreatedV1Data{
		MessageId:    raStr("om_test"),
		OperatorType: raStr(operatorType),
		UserId:       &larkim.UserId{OpenId: raStr(openID)},
		ReactionType: &larkim.Emoji{EmojiType: raStr(emoji)},
	}}
}

func TestNewReactionApproval(t *testing.T) {
	if ra := newReactionApproval(map[string]any{}); ra != nil {
		t.Fatal("expected nil when reaction_approvers is not configured")
	}
	ra := newReactionApproval(map[string]any{"reaction_approvers": " ou_a , ou_b "})
	if ra == nil || !ra.approvers["ou_a"] || !ra.approvers["ou_b"] {
		t.Fatalf("approvers not parsed: %+v", ra)
	}
	if !ra.approve["THUMBSUP"] || !ra.reject["ThumbsDown"] {
		t.Fatal("default emoji sets missing")
	}
	ra = newReactionApproval(map[string]any{"reaction_approvers": "ou_a", "reaction_approve_emojis": "OK"})
	if ra.approve["THUMBSUP"] || !ra.approve["OK"] {
		t.Fatal("custom approve emojis not applied")
	}
}

// Events that must be dropped before any network call (client is nil here, so a
// dispatch attempt would panic in the goroutine only after the filters pass).
func TestOnReactionCreatedFilters(t *testing.T) {
	p := &Platform{platformName: "feishu", reactionApproval: newReactionApproval(map[string]any{"reaction_approvers": "ou_owner"})}
	cases := map[string]*larkim.P2MessageReactionCreatedV1{
		"app operator":   reactionEvent("app", "ou_owner", "THUMBSUP"),
		"not approver":   reactionEvent("user", "ou_other", "THUMBSUP"),
		"unmapped emoji": reactionEvent("user", "ou_owner", "HEART"),
		"nil event":      {},
	}
	for name, ev := range cases {
		if err := p.onReactionCreated(context.Background(), ev); err != nil {
			t.Fatalf("%s: unexpected error %v", name, err)
		}
	}
	disabled := &Platform{platformName: "feishu"}
	if err := disabled.onReactionCreated(context.Background(), reactionEvent("user", "ou_owner", "THUMBSUP")); err != nil {
		t.Fatal(err)
	}
}
