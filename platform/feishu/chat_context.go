package feishu

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

// Group context sharing for reply-all mode.
//
// With group_reply_all + thread_isolation every group message belongs to a
// thread session (a top-level message is the root of its own thread), and the
// agent of one thread never sees the rest of the group. This records inbound
// group messages per chat and, when a message is dispatched to a thread's
// agent, prepends what happened elsewhere since that thread's previous turn.
//
//	skipped_history_share = true
//	    Thread scope: only messages of the same thread that the message filter
//	    skipped (defaults: 30 entries, 24h).
//	chat_history_share = true            (supersedes the thread scope)
//	    Chat scope: also top-level messages and other threads' messages.
//	chat_history_max = 20                 entries per dispatch (default 20)
//	chat_history_window_minutes = 120     only messages this recent (default 120)
//
// The bot's own replies are not inbound events and are therefore not included.
// Memory only: lost on restart. Plays the role of group_chat_history_share,
// whose upstream implementation only applies in mention mode.

const (
	chatContextBufferPerChat = 300
	chatContextRetention     = 24 * time.Hour
	chatContextTextLimit     = 500 // runes per entry
)

type chatEntry struct {
	seq        uint64
	at         time.Time
	sessionKey string
	messageID  string
	rootID     string
	userID     string
	userName   string
	text       string
	skipped    bool
}

type chatContext struct {
	chatScope bool
	max       int
	window    time.Duration

	mu       sync.Mutex
	seq      uint64
	byChat   map[string][]chatEntry // chat_id -> entries, oldest first
	lastSeen map[string]uint64      // session key -> highest seq already covered
	now      func() time.Time
}

func optInt(opts map[string]any, key string, def int) int {
	switch v := opts[key].(type) {
	case int64:
		if v > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	}
	return def
}

func newChatContext(opts map[string]any) *chatContext {
	c := &chatContext{byChat: make(map[string][]chatEntry), lastSeen: make(map[string]uint64), now: time.Now}
	switch {
	case optBool(opts, "chat_history_share"):
		c.chatScope = true
		c.max = optInt(opts, "chat_history_max", 20)
		c.window = time.Duration(optInt(opts, "chat_history_window_minutes", 120)) * time.Minute
	case optBool(opts, "skipped_history_share"):
		c.max = 30
		c.window = chatContextRetention
	default:
		return nil
	}
	return c
}

// threadParts returns chat id and thread root for a thread session key.
func threadParts(sessionKey string) (chatID, rootID string, ok bool) {
	parts := strings.SplitN(sessionKey, ":", 3)
	if len(parts) != 3 {
		return "", "", false
	}
	root, ok := parseThreadRootID(parts[2])
	if !ok {
		return "", "", false
	}
	return parts[1], root, true
}

// record stores an inbound group message; skipped = not dispatched to the agent.
func (c *chatContext) record(msg *core.Message, skipped bool) {
	if c == nil || msg == nil || strings.HasPrefix(msg.Content, ReactionApprovalMarker) {
		return
	}
	chatID, rootID, ok := threadParts(msg.SessionKey)
	if !ok {
		return
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" {
		if len(msg.Images) == 0 && len(msg.Files) == 0 && msg.Audio == nil {
			return
		}
		text = "[附件]"
	}
	if r := []rune(text); len(r) > chatContextTextLimit {
		text = string(r[:chatContextTextLimit]) + "…"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	list := c.pruneLocked(c.byChat[chatID])
	list = append(list, chatEntry{
		seq: c.seq, at: c.now(), sessionKey: msg.SessionKey, messageID: msg.MessageID, rootID: rootID,
		userID: msg.UserID, userName: msg.UserName, text: text, skipped: skipped,
	})
	if len(list) > chatContextBufferPerChat {
		list = list[len(list)-chatContextBufferPerChat:]
	}
	c.byChat[chatID] = list
}

func (c *chatContext) pruneLocked(list []chatEntry) []chatEntry {
	cutoff := c.now().Add(-chatContextRetention)
	i := 0
	for i < len(list) && list[i].at.Before(cutoff) {
		i++
	}
	return list[i:]
}

// take builds the context block for a message about to be dispatched and marks
// everything up to now as covered for its session. "" when nothing applies.
func (c *chatContext) take(msg *core.Message) string {
	if c == nil || msg == nil {
		return ""
	}
	chatID, _, ok := threadParts(msg.SessionKey)
	if !ok {
		return ""
	}
	c.mu.Lock()
	list := c.byChat[chatID]
	last := c.lastSeen[msg.SessionKey]
	cutoff := c.now().Add(-c.window)
	var picked []chatEntry
	for _, e := range list {
		if e.seq <= last || e.at.Before(cutoff) || e.messageID == msg.MessageID {
			continue
		}
		same := e.sessionKey == msg.SessionKey
		if same && !e.skipped {
			continue // already part of this thread's conversation
		}
		if !same && !c.chatScope {
			continue
		}
		picked = append(picked, e)
	}
	c.lastSeen[msg.SessionKey] = c.seq
	c.mu.Unlock()

	if len(picked) == 0 {
		return ""
	}
	if len(picked) > c.max {
		picked = picked[len(picked)-c.max:]
	}
	var b strings.Builder
	if c.chatScope {
		b.WriteString("[群里近期的其他消息（未交给你处理，仅供参考上下文）]\n")
	} else {
		b.WriteString("[期间话题里的其他消息（未交给你处理，仅供参考上下文）]\n")
	}
	for _, e := range picked {
		where := "其他话题"
		switch {
		case e.sessionKey == msg.SessionKey:
			where = "本话题"
		case e.messageID == e.rootID:
			where = "群里"
		}
		who := e.userID
		if e.userName != "" && e.userName != e.userID {
			who = fmt.Sprintf("%s(%s)", e.userName, e.userID)
		}
		if c.chatScope {
			fmt.Fprintf(&b, "- %s【%s】%s: %s\n", e.at.Format("15:04"), where, who, e.text)
		} else {
			fmt.Fprintf(&b, "- %s %s: %s\n", e.at.Format("15:04"), who, e.text)
		}
	}
	b.WriteString("[以上为背景消息，以下是当前消息]")
	return b.String()
}

// attachChatContext prepends the context block to msg.ExtraContent.
func (p *Platform) attachChatContext(msg *core.Message) {
	block := p.chatContext.take(msg)
	if block == "" {
		return
	}
	if msg.ExtraContent == "" {
		msg.ExtraContent = block
	} else {
		msg.ExtraContent = block + "\n" + msg.ExtraContent
	}
}
