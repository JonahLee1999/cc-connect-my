package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

func TestMessageInjectionAcknowledgement_ReactsToSupplementWithoutPostingText(t *testing.T) {
	requests := make(chan receiptRequest, 4)
	p := receiptTestPlatform(t, map[string]any{}, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ReactionType struct {
				Emoji string `json:"emoji_type"`
			} `json:"reaction_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- receiptRequest{method: r.Method, path: r.URL.Path, emoji: body.ReactionType.Emoji}
		fmt.Fprint(w, `{"code":0,"data":{"reaction_id":"injected-receipt"}}`)
	})
	msg := receiptMessage()
	// Attachment batches may retain an older reply context. The reaction must
	// acknowledge the actual canonical supplement that reached the agent.
	msg.MessageID = "om_supplement"
	p.AcknowledgeMessageInjection(context.Background(), msg)
	req := awaitReceiptRequest(t, requests)
	if req.method != http.MethodPost || req.path != "/open-apis/im/v1/messages/om_supplement/reactions" || req.emoji != "Get" {
		t.Fatalf("request = %+v, want a received reaction on the supplement", req)
	}
}

func TestMessageInjectionAcknowledgement_ReusesPersistentReceiptAndSkipsSyntheticMessages(t *testing.T) {
	p := receiptTestPlatform(t, map[string]any{}, func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected duplicate or synthetic reaction: %s", r.URL.Path)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg := receiptMessage()
	msg.ReplyCtx = replyContext{messageID: msg.MessageID, receiptMessageID: msg.MessageID, receiptEmoji: "Get"}
	p.AcknowledgeMessageInjection(ctx, msg)
	p.AcknowledgeMessageInjection(ctx, nil)
	p.AcknowledgeMessageInjection(ctx, &core.Message{})
	p.AcknowledgeMessageInjection(ctx, &core.Message{MessageID: "om_recalled", Recalled: true})
}
