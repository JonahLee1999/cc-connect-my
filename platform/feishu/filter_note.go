package feishu

import (
	"context"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// Filter notes: a short line returned by message_filter_cmd (JSON field
// "note", e.g. "Jev p=0.87") is shown as a small grey footer on the reply to
// that message, like the context/token status footer.
//
// The note is stored per session when a message passes the filter and taken
// (used once) by the next reply sent to that session. Every dispatched
// message overwrites it, so a turn that ends silently (NO_REPLY) cannot leak
// its note onto a later reply.

func (p *Platform) setFilterNote(sessionKey, note string) {
	if sessionKey == "" {
		return
	}
	note = strings.TrimSpace(note)
	if note == "" {
		p.filterNotes.Delete(sessionKey)
		return
	}
	p.filterNotes.Store(sessionKey, note)
}

func (p *Platform) takeFilterNote(sessionKey string) string {
	if sessionKey == "" {
		return ""
	}
	v, ok := p.filterNotes.LoadAndDelete(sessionKey)
	if !ok {
		return ""
	}
	note, _ := v.(string)
	return note
}

func joinFooters(parts ...string) string {
	var kept []string
	for _, part := range parts {
		if s := strings.TrimSpace(part); s != "" {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, "\n")
}

// sendWithFooterCard sends content as a card with a notation footer. Mentions
// are rewritten to the card syntax (<at id=...>), which notifies people.
// Content too large for a card falls back to the regular path with the footer
// appended as plain text.
func (p *Platform) sendWithFooterCard(ctx context.Context, rc replyContext, content, footer string) error {
	if countMarkdownTables(content) > maxCardTables {
		msgType, msgBody := p.buildReplyContentFor(content + "\n\n" + footer)
		if !p.shouldUseThreadOrReplyAPI(rc) {
			return p.sendNewMessageToChat(ctx, rc, msgType, msgBody)
		}
		return p.replyMessage(ctx, rc, msgType, msgBody)
	}
	body := sanitizeMarkdownURLs(preprocessFeishuMarkdown(normalizeAtTags(content, true)))
	foot := sanitizeMarkdownURLs(preprocessFeishuMarkdown(footer))
	cardJSON := buildCardJSONWithStatusFooter(body, foot)
	if !p.shouldUseThreadOrReplyAPI(rc) {
		return p.sendNewMessageToChat(ctx, rc, larkim.MsgTypeInteractive, cardJSON)
	}
	return p.replyMessage(ctx, rc, larkim.MsgTypeInteractive, cardJSON)
}
