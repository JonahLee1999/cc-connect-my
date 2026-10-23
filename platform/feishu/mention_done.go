package feishu

import (
	"regexp"
	"strings"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

// Mentions in cards and a "task done" marker.
//
// mention_in_card = true
//
//	Upstream sends any reply containing an <at> tag as plain text, because
//	Feishu only raises mention *events* (bot-to-bot) for text messages. For
//	mentioning people that drops all markdown. With this option a reply that
//	has both mentions and markdown is sent as a card using the card mention
//	syntax <at id=ou_xxx></at>, which notifies people and keeps formatting.
//	Replies with mentions but no markdown are still sent as text, using
//	<at user_id="ou_xxx"></at>. Either syntax may be written by the agent; it is
//	normalised to whatever the chosen message type needs. Mention by open_id
//	only — name-based resolution (resolve_mentions) can hit the wrong person
//	when display names differ from real names.
//
// thread_done_marker = "[[DONE]]"   thread_done_emoji = "DONE" (default)
//
//	When a reply contains a line that is exactly the marker, the line is
//	removed before sending and, once the reply is delivered, the thread's
//	root message gets the done emoji. Lets the agent close a topic without a
//	helper script or an extra message.

var atTagRe = regexp.MustCompile(`<at\s+(?:user_id="([^"]+)"|id=["']?([^\s"'>]+)["']?)\s*>[^<]*</at>`)

func hasAtTag(content string) bool { return atTagRe.MatchString(content) }

// normalizeAtTags rewrites every <at> tag into card (<at id=X></at>) or text
// (<at user_id="X"></at>) syntax.
func normalizeAtTags(content string, card bool) string {
	return atTagRe.ReplaceAllStringFunc(content, func(tag string) string {
		m := atTagRe.FindStringSubmatch(tag)
		id := m[1]
		if id == "" {
			id = m[2]
		}
		if card {
			return "<at id=" + id + "></at>"
		}
		return `<at user_id="` + id + `"></at>`
	})
}

// buildReplyContentFor is buildReplyContent plus the mention_in_card policy.
func (p *Platform) buildReplyContentFor(content string) (msgType string, body string) {
	if !p.mentionInCard || !hasAtTag(content) {
		return buildReplyContent(content)
	}
	if containsMarkdown(content) && countMarkdownTables(content) <= maxCardTables {
		card := normalizeAtTags(content, true)
		return larkim.MsgTypeInteractive, buildCardJSON(sanitizeMarkdownURLs(preprocessFeishuMarkdown(card)))
	}
	return buildReplyContent(normalizeAtTags(content, false))
}

// takeDoneMarker removes marker lines; reports whether any were present.
func (p *Platform) takeDoneMarker(content string) (string, bool) {
	marker := p.threadDoneMarker
	if marker == "" || !strings.Contains(content, marker) {
		return content, false
	}
	lines := strings.Split(content, "\n")
	kept := lines[:0]
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == marker {
			found = true
			continue
		}
		kept = append(kept, line)
	}
	if !found {
		return content, false
	}
	return strings.TrimSpace(strings.Join(kept, "\n")), true
}

// stripDoneMarker removes marker lines without side effects (preview paths).
func (p *Platform) stripDoneMarker(content string) string {
	out, _ := p.takeDoneMarker(content)
	return out
}

// markThreadRootDone adds the done emoji to the thread root of sessionKey.
func (p *Platform) markThreadRootDone(sessionKey string) {
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) != 3 {
		return
	}
	root, ok := parseThreadRootID(parts[2])
	if !ok {
		return
	}
	if _, dup := p.doneRoots.LoadOrStore(root, true); dup {
		return
	}
	emoji := p.threadDoneEmoji
	if emoji == "" {
		emoji = "DONE"
	}
	p.addReactionWithEmoji(root, emoji)
}

func optBool(opts map[string]any, key string) bool {
	v, _ := opts[key].(bool)
	return v
}

func optString(opts map[string]any, key string) string {
	v, _ := opts[key].(string)
	return strings.TrimSpace(v)
}
