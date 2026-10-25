package feishu

import (
	"encoding/json"
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

func TestBuildCardJSONWithGreyFooter(t *testing.T) {
	out := buildCardJSONWithGreyFooter("**正文**", "model · ctx 4%\n\nJev p=0.9 · 0.6s")
	var card struct {
		Body struct {
			Elements []map[string]any `json:"elements"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(out), &card); err != nil {
		t.Fatal(err)
	}
	els := card.Body.Elements
	if len(els) != 2 || els[0]["tag"] != "markdown" {
		t.Fatalf("want markdown + one footer block, got %v", els)
	}
	if got := els[1]["text"].(map[string]any)["content"]; got != "model · ctx 4%\nJev p=0.9 · 0.6s" {
		t.Fatalf("footer lines must be joined without blank lines, got %q", got)
	}
	for _, e := range els[1:] {
		if e["tag"] != "div" {
			t.Fatalf("footer must be a div, got %v", e)
		}
		text := e["text"].(map[string]any)
		if text["tag"] != "plain_text" || text["text_size"] != footerTextSize || text["text_color"] != "grey" {
			t.Fatalf("footer must be grey %s plain_text, got %v", footerTextSize, text)
		}
	}
	for _, e := range els {
		if e["tag"] == "hr" {
			t.Fatal("no divider expected")
		}
	}
}
