package feishu

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

// Skipped-message history (skipped_history_share = true).
//
// With group_reply_all the bot sees every group message, but messages the
// message filter skips never reach the agent. When a later message in the
// same session is dispatched, the agent would otherwise be missing whatever
// was said in between (e.g. a colleague stating a finding without asking a
// question). This keeps skipped group messages per session and prepends them
// to the next dispatched message of that session as context, then clears
// them. This plays the role of group_chat_history_share for reply-all mode,
// where upstream's implementation (mention mode only) does not apply.
//
// Memory only: entries are lost on restart. Bounded per session.

const (
	maxSkippedHistoryEntries = 30
	skippedHistoryTTL        = 24 * time.Hour
	maxSkippedHistoryText    = 500 // runes per entry
)

type skippedEntry struct {
	at       time.Time
	userID   string
	userName string
	text     string
}

type skippedHistory struct {
	mu      sync.Mutex
	entries map[string][]skippedEntry // session key -> entries (oldest first)
	now     func() time.Time
}

func newSkippedHistory(opts map[string]any) *skippedHistory {
	if !optBool(opts, "skipped_history_share") {
		return nil
	}
	return &skippedHistory{entries: make(map[string][]skippedEntry), now: time.Now}
}

func isGroupSessionKey(sessionKey string) bool {
	// p2p sessions are "<tag>:<chat>:<user>"; thread sessions carry a root;
	// group chats without thread isolation use the chat (plus user) key too,
	// so only thread sessions are treated as group context here.
	return isThreadSessionKey(sessionKey)
}

func (h *skippedHistory) remember(msg *core.Message) {
	if h == nil || msg == nil || !isGroupSessionKey(msg.SessionKey) {
		return
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		if len(msg.Images) > 0 || len(msg.Files) > 0 {
			text = "[附件]"
		} else {
			return
		}
	}
	if r := []rune(text); len(r) > maxSkippedHistoryText {
		text = string(r[:maxSkippedHistoryText]) + "…"
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	list := append(h.prune(h.entries[msg.SessionKey]), skippedEntry{
		at: h.now(), userID: msg.UserID, userName: msg.UserName, text: text,
	})
	if len(list) > maxSkippedHistoryEntries {
		list = list[len(list)-maxSkippedHistoryEntries:]
	}
	h.entries[msg.SessionKey] = list
}

// prune drops expired entries; caller holds mu.
func (h *skippedHistory) prune(list []skippedEntry) []skippedEntry {
	cutoff := h.now().Add(-skippedHistoryTTL)
	i := 0
	for i < len(list) && list[i].at.Before(cutoff) {
		i++
	}
	return list[i:]
}

// take returns and clears the formatted history for a session ("" if none).
func (h *skippedHistory) take(sessionKey string) string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	list := h.prune(h.entries[sessionKey])
	delete(h.entries, sessionKey)
	h.mu.Unlock()
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("[期间话题里的其他消息（未交给你处理，仅供参考上下文）]\n")
	for _, e := range list {
		who := e.userName
		if who == "" || who == e.userID {
			who = e.userID
		} else if e.userID != "" {
			who = fmt.Sprintf("%s(%s)", e.userName, e.userID)
		}
		fmt.Fprintf(&b, "- %s %s: %s\n", e.at.Format("15:04"), who, e.text)
	}
	b.WriteString("[以上为期间消息，以下是当前消息]")
	return b.String()
}

// attachSkippedHistory prepends pending skipped history to msg.ExtraContent.
func (p *Platform) attachSkippedHistory(msg *core.Message) {
	block := p.skippedHistory.take(msg.SessionKey)
	if block == "" {
		return
	}
	if msg.ExtraContent == "" {
		msg.ExtraContent = block
	} else {
		msg.ExtraContent = block + "\n" + msg.ExtraContent
	}
}
