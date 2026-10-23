package feishu

import (
	"strings"
	"testing"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

func TestSkippedHistoryRememberTake(t *testing.T) {
	h := newSkippedHistory(map[string]any{"skipped_history_share": true})
	now := time.Date(2026, 10, 24, 10, 0, 0, 0, time.Local)
	h.now = func() time.Time { return now }
	key := "feishu:oc_1:root:om_r"
	h.remember(&core.Message{SessionKey: key, UserID: "ou_a", UserName: "王珈铭", Content: "我查了，是 A 账号的问题"})
	h.remember(&core.Message{SessionKey: key, UserID: "ou_b", UserName: "ou_b", Content: "好的"})
	h.remember(&core.Message{SessionKey: "feishu:oc_1:ou_a", UserID: "ou_a", Content: "p2p 不记"})
	block := h.take(key)
	for _, want := range []string{"王珈铭(ou_a): 我查了，是 A 账号的问题", "ou_b: 好的", "10:00"} {
		if !strings.Contains(block, want) {
			t.Fatalf("missing %q in %q", want, block)
		}
	}
	if h.take(key) != "" {
		t.Fatal("take must clear the session")
	}
	if h.take("feishu:oc_1:ou_a") != "" {
		t.Fatal("non-thread sessions must not be recorded")
	}
}

func TestSkippedHistoryBoundsAndTTL(t *testing.T) {
	h := newSkippedHistory(map[string]any{"skipped_history_share": true})
	now := time.Now()
	h.now = func() time.Time { return now }
	key := "feishu:oc_1:root:om_r"
	h.remember(&core.Message{SessionKey: key, UserID: "ou_old", Content: "过期"})
	now = now.Add(skippedHistoryTTL + time.Minute)
	for i := 0; i < maxSkippedHistoryEntries+5; i++ {
		h.remember(&core.Message{SessionKey: key, UserID: "ou_x", Content: "m"})
	}
	block := h.take(key)
	if strings.Contains(block, "过期") {
		t.Fatal("expired entry kept")
	}
	if n := strings.Count(block, "\n- "); n != maxSkippedHistoryEntries {
		t.Fatalf("expected %d entries, got %d", maxSkippedHistoryEntries, n)
	}
}

func TestAttachSkippedHistory(t *testing.T) {
	p := &Platform{skippedHistory: newSkippedHistory(map[string]any{"skipped_history_share": true})}
	key := "feishu:oc_1:root:om_r"
	p.skippedHistory.remember(&core.Message{SessionKey: key, UserID: "ou_a", Content: "背景信息"})
	msg := &core.Message{SessionKey: key, Content: "帮我看下", ExtraContent: "引用内容"}
	p.attachSkippedHistory(msg)
	if !strings.HasPrefix(msg.ExtraContent, "[期间话题里的其他消息") || !strings.HasSuffix(msg.ExtraContent, "\n引用内容") {
		t.Fatalf("unexpected ExtraContent: %q", msg.ExtraContent)
	}
	off := &Platform{}
	m2 := &core.Message{SessionKey: key}
	off.attachSkippedHistory(m2)
	if m2.ExtraContent != "" {
		t.Fatal("disabled feature must not touch ExtraContent")
	}
}
