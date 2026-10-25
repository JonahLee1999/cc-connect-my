package feishu

import (
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

func TestFilterNoteStoredOnPassAndTakenOnce(t *testing.T) {
	p := &Platform{platformName: "feishu", messageFilter: newMessageFilter(map[string]any{
		"message_filter_cmd": writeFilterScript(t, `cat >/dev/null; echo '{"action":"pass","note":"Jev p=0.87"}'`)})}
	msg := &core.Message{SessionKey: "feishu:oc_1:root:om_r", MessageID: "om_r", Content: "帮我查下"}
	if !p.passesMessageFilter(msg) {
		t.Fatal("expected pass")
	}
	if got := p.takeFilterNote(msg.SessionKey); got != "Jev p=0.87" {
		t.Fatalf("note = %q", got)
	}
	if got := p.takeFilterNote(msg.SessionKey); got != "" {
		t.Fatalf("note must be used once, got %q", got)
	}
}

func TestFilterNoteClearedByLaterMessageWithoutNote(t *testing.T) {
	p := &Platform{platformName: "feishu", messageFilter: newMessageFilter(map[string]any{
		"message_filter_cmd": writeFilterScript(t, `cat >/dev/null; echo '{"action":"pass"}'`)})}
	key := "feishu:oc_1:root:om_r"
	p.setFilterNote(key, "Jev p=0.10") // stale note from a silent turn
	if !p.passesMessageFilter(&core.Message{SessionKey: key, MessageID: "om_2", Content: "x"}) {
		t.Fatal("expected pass")
	}
	if got := p.takeFilterNote(key); got != "" {
		t.Fatalf("stale note leaked: %q", got)
	}
}

func TestFilterNoteNotStoredWhenSkipped(t *testing.T) {
	p := &Platform{platformName: "feishu", messageFilter: newMessageFilter(map[string]any{
		"message_filter_cmd": writeFilterScript(t, `cat >/dev/null; echo '{"action":"skip","note":"Jev p=0.05"}'`)})}
	key := "feishu:oc_1:root:om_r"
	if p.passesMessageFilter(&core.Message{SessionKey: key, MessageID: "om_r", Content: "收到"}) {
		t.Fatal("expected skip")
	}
	if got := p.takeFilterNote(key); got != "" {
		t.Fatalf("skipped message must not leave a note, got %q", got)
	}
}

func TestJoinFooters(t *testing.T) {
	if got := joinFooters("", " Jev p=0.9 "); got != "Jev p=0.9" {
		t.Fatalf("got %q", got)
	}
	if got := joinFooters("model · ctx 4%", "Jev p=0.9"); got != "model · ctx 4%\nJev p=0.9" {
		t.Fatalf("got %q", got)
	}
}
