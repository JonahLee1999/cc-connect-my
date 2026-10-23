package feishu

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"

	"github.com/chenhg5/cc-connect/core"
)

// Reaction approvals: lets designated users approve or reject a pending agent
// request by reacting to the bot's message with an emoji, instead of typing.
//
// When an approver reacts to a message sent by this bot, the reaction is routed
// into the session that message belongs to as an agent-only message (nothing
// is posted to the chat). Reactions by anyone else, on other users' messages,
// with unlisted emojis, or added by the bot itself are ignored.
//
// Config (platform options):
//
//	reaction_approvers      = "ou_xxx,ou_yyy"   # required; empty disables the feature
//	reaction_approve_emojis = "THUMBSUP,OK,..."  # optional, defaults below
//	reaction_reject_emojis  = "ThumbsDown,No,..." # optional, defaults below

// ReactionApprovalMarker prefixes the synthesized message content so agents
// and wrappers can recognise it.
const ReactionApprovalMarker = "[cc-connect reaction-approval]"

const (
	defaultApproveEmojis = "THUMBSUP,OK,Yes,CheckMark,LGTM,DONE"
	defaultRejectEmojis  = "ThumbsDown,No,CrossMark,MinusOne"
)

type reactionApproval struct {
	approvers map[string]bool
	approve   map[string]bool
	reject    map[string]bool
}

func parseCSVSet(s string) map[string]bool {
	out := make(map[string]bool)
	for _, part := range strings.Split(s, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out[v] = true
		}
	}
	return out
}

func newReactionApproval(opts map[string]any) *reactionApproval {
	approvers, _ := opts["reaction_approvers"].(string)
	set := parseCSVSet(approvers)
	if len(set) == 0 {
		return nil
	}
	approve, _ := opts["reaction_approve_emojis"].(string)
	if strings.TrimSpace(approve) == "" {
		approve = defaultApproveEmojis
	}
	reject, _ := opts["reaction_reject_emojis"].(string)
	if strings.TrimSpace(reject) == "" {
		reject = defaultRejectEmojis
	}
	return &reactionApproval{approvers: set, approve: parseCSVSet(approve), reject: parseCSVSet(reject)}
}

// chatMode returns "group", "topic" or "p2p" for a chat (cached).
func (p *Platform) chatMode(ctx context.Context, chatID string) string {
	if chatID == "" || p.client == nil {
		return ""
	}
	if cached, ok := p.chatModeCache.Load(chatID); ok {
		return cached.(string)
	}
	resp, err := p.client.Im.Chat.Get(ctx, larkim.NewGetChatReqBuilder().ChatId(chatID).Build())
	if err != nil || !resp.Success() || resp.Data == nil || resp.Data.ChatMode == nil {
		slog.Debug(p.tag()+": reaction approval: resolve chat mode failed", "chat_id", chatID, "error", err)
		return ""
	}
	mode := *resp.Data.ChatMode
	p.chatModeCache.Store(chatID, mode)
	return mode
}

func (p *Platform) onReactionCreated(ctx context.Context, event *larkim.P2MessageReactionCreatedV1) error {
	ra := p.reactionApproval
	if ra == nil || event == nil || event.Event == nil {
		return nil
	}
	ev := event.Event
	if stringValue(ev.OperatorType) != "user" || ev.UserId == nil || ev.ReactionType == nil {
		return nil // reactions added by apps (including our own OnIt/DONE) are ignored
	}
	userID := stringValue(ev.UserId.OpenId)
	if userID == "" || !ra.approvers[userID] {
		return nil
	}
	emoji := stringValue(ev.ReactionType.EmojiType)
	var decision string
	switch {
	case ra.approve[emoji]:
		decision = "approve"
	case ra.reject[emoji]:
		decision = "reject"
	default:
		slog.Debug(p.tag()+": reaction approval: emoji not mapped", "emoji", emoji)
		return nil
	}
	messageID := stringValue(ev.MessageId)
	if messageID == "" {
		return nil
	}
	go p.dispatchReactionApproval(context.Background(), messageID, userID, emoji, decision)
	return nil
}

func (p *Platform) dispatchReactionApproval(ctx context.Context, messageID, userID, emoji, decision string) {
	resp, err := p.client.Im.Message.Get(ctx, larkim.NewGetMessageReqBuilder().MessageId(messageID).Build())
	if err != nil || !resp.Success() || resp.Data == nil || len(resp.Data.Items) == 0 {
		slog.Debug(p.tag()+": reaction approval: fetch message failed", "message_id", messageID, "error", err)
		return
	}
	m := resp.Data.Items[0]
	if m.Sender == nil || stringValue(m.Sender.SenderType) != "app" || stringValue(m.Sender.Id) != p.appID {
		return // only reactions on this bot's own messages count
	}
	chatID := stringValue(m.ChatId)
	mode := p.chatMode(ctx, chatID)
	chatType := "p2p"
	if mode == "group" || mode == "topic" {
		chatType = "group"
	}
	if chatType == "group" && !core.AllowList(p.allowChat, chatID) {
		return
	}
	evMsg := &larkim.EventMessage{MessageId: m.MessageId, RootId: m.RootId, ChatId: m.ChatId, ChatType: &chatType}
	sessionKey := p.makeSessionKey(evMsg, chatID, userID)

	quoted := ""
	if cm := p.fetchSingleMessage(ctx, messageID); cm != nil {
		quoted = cm.text
	}
	if r := []rune(quoted); len(r) > 200 {
		quoted = string(r[:200]) + "…"
	}
	verdict := "同意（批准执行）"
	if decision == "reject" {
		verdict = "拒绝（不要执行）"
	}
	content := fmt.Sprintf("%s 用户在你的消息上点了表情 %s，表示%s。被点表情的消息：%s",
		ReactionApprovalMarker, emoji, verdict, quoted)

	slog.Info(p.tag()+": reaction approval routed",
		"message_id", messageID, "emoji", emoji, "decision", decision, "session_key", sessionKey)
	p.dispatchCoreMessage(&core.Message{
		SessionKey: sessionKey, Platform: p.platformName,
		MessageID: messageID,
		UserID:    userID, UserName: p.resolveUserName(userID), ChatName: p.resolveChatName(chatID),
		Content:  content,
		ReplyCtx: replyContext{messageID: messageID, chatID: chatID, sessionKey: sessionKey},
	})
}
