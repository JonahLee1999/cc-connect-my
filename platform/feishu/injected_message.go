package feishu

import (
	"context"
	"log/slog"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

// AcknowledgeMessageInjection leaves a persistent "received" reaction on a
// successfully injected supplement. It replaces the engine's text receipt.
func (p *Platform) AcknowledgeMessageInjection(ctx context.Context, msg *core.Message) {
	if msg == nil || msg.MessageID == "" || msg.Recalled {
		return
	}
	if rc, ok := msg.ReplyCtx.(replyContext); ok && rc.receiptMessageID == msg.MessageID && rc.receiptEmoji != "" {
		// OnAccepted already owns a persistent receipt on this message. Reuse
		// it instead of adding another emoji or another API request.
		return
	}
	messageID := msg.MessageID
	go func() {
		if p.isMessageRecalled(messageID) {
			return
		}
		reactionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if p.addReactionWithEmojiContext(reactionCtx, messageID, "Get") == "" {
			slog.Warn(p.tag()+": injected message acknowledgement failed", "message_id", messageID)
		}
	}()
}

var _ core.MessageInjectionAcknowledger = (*Platform)(nil)
