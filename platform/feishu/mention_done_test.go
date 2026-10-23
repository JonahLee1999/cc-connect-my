package feishu

import (
	"encoding/json"
	"strings"
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestNormalizeAtTags(t *testing.T) {
	in := `<at user_id="ou_a">张三</at> 和 <at id=ou_b></at> 看下`
	if got := normalizeAtTags(in, true); got != `<at id=ou_a></at> 和 <at id=ou_b></at> 看下` {
		t.Fatalf("card: %q", got)
	}
	if got := normalizeAtTags(in, false); got != `<at user_id="ou_a"></at> 和 <at user_id="ou_b"></at> 看下` {
		t.Fatalf("text: %q", got)
	}
}

func TestBuildReplyContentForMentions(t *testing.T) {
	md := "<at user_id=\"ou_a\"></at> 结果如下：\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n**完成**"
	off := &Platform{}
	if mt, _ := off.buildReplyContentFor(md); mt != larkim.MsgTypeText {
		t.Fatalf("option off must keep upstream text behaviour, got %s", mt)
	}
	on := &Platform{mentionInCard: true}
	mt, body := on.buildReplyContentFor(md)
	var card struct {
		Body struct {
			Elements []struct {
				Content string `json:"content"`
			} `json:"elements"`
		} `json:"body"`
	}
	_ = json.Unmarshal([]byte(body), &card)
	if mt != larkim.MsgTypeInteractive || len(card.Body.Elements) == 0 || !strings.HasPrefix(card.Body.Elements[0].Content, "<at id=ou_a></at>") {
		t.Fatalf("expected card with card mention, got %s %s", mt, body)
	}
	mt, body = on.buildReplyContentFor(`<at id=ou_a></at> 好了`)
	var text map[string]string
	if mt != larkim.MsgTypeText || json.Unmarshal([]byte(body), &text) != nil || text["text"] != `<at user_id="ou_a"></at> 好了` {
		t.Fatalf("expected text with text mention, got %s %s", mt, body)
	}
	if mt, _ = on.buildReplyContentFor("**无 @**"); mt != larkim.MsgTypeInteractive {
		t.Fatalf("no mention: upstream behaviour expected, got %s", mt)
	}
}

func TestTakeDoneMarker(t *testing.T) {
	p := &Platform{threadDoneMarker: "[[DONE]]"}
	out, done := p.takeDoneMarker("结果\n\n[[DONE]]\n")
	if !done || out != "结果" {
		t.Fatalf("got %q %v", out, done)
	}
	if out, done = p.takeDoneMarker("说明 [[DONE]] 不是单独一行"); done || out != "说明 [[DONE]] 不是单独一行" {
		t.Fatalf("inline marker must be kept: %q %v", out, done)
	}
	if _, done = (&Platform{}).takeDoneMarker("x\n[[DONE]]"); done {
		t.Fatal("marker disabled must not match")
	}
}

func TestMarkThreadRootDoneIgnoresNonThread(t *testing.T) {
	p := &Platform{threadDoneMarker: "[[DONE]]"}
	p.markThreadRootDone("feishu:oc_1:ou_a") // p2p key: must return before any API call
	if _, ok := p.doneRoots.Load("ou_a"); ok {
		t.Fatal("non-thread session must not be marked")
	}
}
