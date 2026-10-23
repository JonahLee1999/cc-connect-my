package feishu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

func writeFilterScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "filter.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestNewMessageFilter(t *testing.T) {
	if newMessageFilter(map[string]any{}) != nil {
		t.Fatal("expected nil filter without message_filter_cmd")
	}
	f := newMessageFilter(map[string]any{"message_filter_cmd": "/usr/bin/python3 -I /x/filter.py", "message_filter_timeout_ms": int64(1500)})
	if f == nil || len(f.argv) != 3 || f.argv[2] != "/x/filter.py" || f.timeout.Milliseconds() != 1500 {
		t.Fatalf("unexpected filter: %+v", f)
	}
}

func TestBuildMessageFilterInput(t *testing.T) {
	root := &core.Message{SessionKey: "feishu:oc_1:root:om_root", MessageID: "om_root", UserID: "ou_a", Content: "hi",
		ReplyCtx: replyContext{chatID: "oc_1"}}
	in := buildMessageFilterInput("feishu", root)
	if !in.IsThread || in.IsFollowUp || in.ThreadRootID != "om_root" || in.ChatID != "oc_1" {
		t.Fatalf("root message: %+v", in)
	}
	follow := &core.Message{SessionKey: "feishu:oc_1:root:om_root", MessageID: "om_2", Images: []core.ImageAttachment{{}}}
	in = buildMessageFilterInput("feishu", follow)
	if !in.IsFollowUp || !in.HasAttachments {
		t.Fatalf("follow-up message: %+v", in)
	}
	p2p := &core.Message{SessionKey: "feishu:oc_2:ou_a", MessageID: "om_3"}
	if in = buildMessageFilterInput("feishu", p2p); in.IsThread || in.IsFollowUp {
		t.Fatalf("p2p message: %+v", in)
	}
}

func TestMessageFilterAllow(t *testing.T) {
	msg := &core.Message{SessionKey: "feishu:oc_1:root:om_root", MessageID: "om_root", Content: "收到"}
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"pass", `cat >/dev/null; echo '{"action":"pass"}'`, true},
		{"skip", `cat >/dev/null; echo '{"action":"skip","reason":"small talk"}'`, false},
		{"reads stdin", `grep -q '"text":"收到"' && echo '{"action":"skip"}' || echo '{"action":"pass"}'`, false},
		{"non-zero exit fails open", `exit 3`, true},
		{"garbage fails open", `echo nope`, true},
		{"timeout fails open", `sleep 5; echo '{"action":"skip"}'`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newMessageFilter(map[string]any{"message_filter_cmd": writeFilterScript(t, c.body), "message_filter_timeout_ms": int64(500)})
			if got, _ := f.allow("feishu", msg); got != c.want {
				t.Fatalf("allow=%v want %v", got, c.want)
			}
		})
	}
}

func TestPassesMessageFilterBypassesReactionApprovals(t *testing.T) {
	p := &Platform{platformName: "feishu", messageFilter: newMessageFilter(map[string]any{
		"message_filter_cmd": writeFilterScript(t, `echo '{"action":"skip"}'`)})}
	if !p.passesMessageFilter(&core.Message{Content: ReactionApprovalMarker + " 同意"}) {
		t.Fatal("reaction approvals must bypass the filter")
	}
	if p.passesMessageFilter(&core.Message{Content: "收到"}) {
		t.Fatal("expected skip")
	}
	if !(&Platform{platformName: "feishu"}).passesMessageFilter(&core.Message{Content: "x"}) {
		t.Fatal("no filter configured must pass")
	}
}
