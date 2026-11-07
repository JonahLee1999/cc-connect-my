package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chenhg5/cc-connect/core"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func TestBuildRichCard_HidesThreadDoneMarker(t *testing.T) {
	p := &Platform{threadDoneMarker: "[[DONE]]"}
	card := p.BuildRichCard(core.CardStatusDone, "", nil, "**Verified**\n[[DONE]]", false, "")
	if strings.Contains(card, "[[DONE]]") {
		t.Fatal("completion marker leaked into rich card JSON")
	}
	if !strings.Contains(card, "Verified") {
		t.Fatal("reply content was lost")
	}
}

func TestFinalResponseDelivered_MarksThreadRootOnceAndRetriesFailure(t *testing.T) {
	var attempts atomic.Int32
	requests := make(chan string, 4)
	p := receiptTestPlatform(t, map[string]any{"thread_done_marker": "[[DONE]]"}, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ReactionType struct {
				EmojiType string `json:"emoji_type"`
			} `json:"reaction_type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/open-apis/im/v1/messages/om_root/reactions" || body.ReactionType.EmojiType != "DONE" {
			t.Errorf("unexpected reaction request: %s %s %+v", r.Method, r.URL.Path, body)
		}
		if attempts.Add(1) == 1 {
			fmt.Fprint(w, `{"code":999,"msg":"temporary failure"}`)
		} else {
			fmt.Fprint(w, `{"code":0,"data":{"reaction_id":"reaction-done"}}`)
		}
		requests <- r.URL.Path
	})
	rc := replyContext{sessionKey: "feishu:oc_chat:root:om_root"}
	observer, ok := any(p).(interface {
		OnFinalResponseDelivered(context.Context, any, string)
	})
	if !ok {
		t.Fatal("rich/preview completion callback is missing")
	}
	// Non-task replies, inline markers, and private replies do not close topics.
	for _, content := range []string{"Waiting for approval", "example [[DONE]] inline"} {
		observer.OnFinalResponseDelivered(context.Background(), rc, content)
	}
	observer.OnFinalResponseDelivered(context.Background(), replyContext{sessionKey: "feishu:oc_private:ou_user"}, "Verified\n[[DONE]]")
	if attempts.Load() != 0 {
		t.Fatal("non-completion reply marked a topic done")
	}
	// Wait for each asynchronous callback to settle before retrying.
	p.markThreadRootDone(rc.sessionKey)
	<-requests
	if _, cached := p.doneRoots.Load("om_root"); cached {
		t.Fatal("failed reaction poisoned completion deduplication")
	}
	observer.OnFinalResponseDelivered(context.Background(), rc, "Verified\n[[DONE]]")
	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("final delivered reply did not mark root done")
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, marked := p.doneRoots.Load("om_root"); marked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("successful reaction was not deduplicated")
		}
		time.Sleep(time.Millisecond)
	}
	p.markThreadRootDone(rc.sessionKey)
	if attempts.Load() != 2 {
		t.Fatalf("reaction attempts = %d, want failed + success only", attempts.Load())
	}
}

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
