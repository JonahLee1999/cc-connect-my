package feishu

import (
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

type ctxClock struct{ t time.Time }

func (c *ctxClock) now() time.Time { return c.t }

func newTestChatContext(opts map[string]any, clk *ctxClock) *chatContext {
	c := newChatContext(opts)
	c.now = clk.now
	return c
}

func gmsg(chat, root, id, user, text string) *core.Message {
	return &core.Message{SessionKey: "feishu:" + chat + ":root:" + root, MessageID: id, UserID: user, UserName: user, Content: text}
}

func TestChatContextDisabled(t *testing.T) {
	if newChatContext(map[string]any{}) != nil {
		t.Fatal("expected nil when neither option is set")
	}
}

func TestChatContextThreadScope(t *testing.T) {
	clk := &ctxClock{t: time.Date(2026, 10, 24, 10, 0, 0, 0, time.Local)}
	c := newTestChatContext(map[string]any{"skipped_history_share": true}, clk)
	c.record(gmsg("oc_1", "om_a", "om_a1", "ou_x", "本话题被跳过"), true)
	c.record(gmsg("oc_1", "om_b", "om_b", "ou_y", "其他话题"), false)
	cur := gmsg("oc_1", "om_a", "om_a2", "ou_me", "现在的问题")
	c.record(cur, false)
	block := c.take(cur)
	if !strings.Contains(block, "本话题被跳过") || strings.Contains(block, "其他话题") || strings.Contains(block, "现在的问题") {
		t.Fatalf("thread scope block: %q", block)
	}
	if c.take(gmsg("oc_1", "om_a", "om_a3", "ou_me", "再问")) != "" {
		t.Fatal("already covered messages must not repeat")
	}
}

func TestChatContextChatScope(t *testing.T) {
	clk := &ctxClock{t: time.Date(2026, 10, 24, 10, 0, 0, 0, time.Local)}
	c := newTestChatContext(map[string]any{"chat_history_share": true, "chat_history_max": int64(3), "chat_history_window_minutes": int64(120)}, clk)
	c.record(gmsg("oc_1", "om_old", "om_old", "ou_z", "太早的消息"), false)
	clk.t = clk.t.Add(3 * time.Hour)
	c.record(gmsg("oc_1", "om_top", "om_top", "ou_y", "群里一句话"), true)
	c.record(gmsg("oc_1", "om_b", "om_b2", "ou_y", "其他话题的跟进"), false)
	c.record(gmsg("oc_2", "om_c", "om_c", "ou_y", "别的群"), false)
	c.record(gmsg("oc_1", "om_a", "om_a1", "ou_x", "本话题已交给AI"), false)
	cur := gmsg("oc_1", "om_a", "om_a2", "ou_me", "现在的问题")
	c.record(cur, false)
	block := c.take(cur)
	for _, want := range []string{"【群里】ou_y: 群里一句话", "【其他话题】ou_y: 其他话题的跟进"} {
		if !strings.Contains(block, want) {
			t.Fatalf("missing %q in %q", want, block)
		}
	}
	for _, bad := range []string{"太早的消息", "别的群", "本话题已交给AI", "现在的问题"} {
		if strings.Contains(block, bad) {
			t.Fatalf("unexpected %q in %q", bad, block)
		}
	}
	// only new messages next time, capped at max
	for i := 0; i < 5; i++ {
		c.record(gmsg("oc_1", "om_b", "om_bx", "ou_y", "新消息"), false)
	}
	block = c.take(gmsg("oc_1", "om_a", "om_a3", "ou_me", "再问"))
	if n := strings.Count(block, "\n- "); n != 3 || strings.Contains(block, "群里一句话") {
		t.Fatalf("expected 3 new entries only, got %d: %q", n, block)
	}
}

func TestAttachChatContext(t *testing.T) {
	clk := &ctxClock{t: time.Now()}
	p := &Platform{chatContext: newTestChatContext(map[string]any{"chat_history_share": true}, clk)}
	p.chatContext.record(gmsg("oc_1", "om_b", "om_b", "ou_y", "背景"), false)
	msg := gmsg("oc_1", "om_a", "om_a", "ou_me", "问题")
	msg.ExtraContent = "引用内容"
	p.attachChatContext(msg)
	if !strings.HasPrefix(msg.ExtraContent, "[群里近期的其他消息") || !strings.HasSuffix(msg.ExtraContent, "\n引用内容") {
		t.Fatalf("unexpected ExtraContent: %q", msg.ExtraContent)
	}
	off := &Platform{}
	m2 := gmsg("oc_1", "om_a", "om_x", "ou_me", "x")
	off.attachChatContext(m2)
	if m2.ExtraContent != "" {
		t.Fatal("disabled feature must not touch ExtraContent")
	}
}
