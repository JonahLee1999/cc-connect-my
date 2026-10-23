package feishu

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
)

// Test helpers that upstream defines in card_allow_from_test.go (from #1854).
// This branch does not carry #1854 yet, but tests picked from #2018 use them.

// stubOpenAPIDomain serves the Open API calls a dispatched card action makes
// before reaching the handler (tenant token, user and chat name lookups) and
// returns its URL for the "domain" option.
func stubOpenAPIDomain(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			writeJSON(t, w, map[string]any{"code": 0, "msg": "ok", "expire": 7200, "tenant_access_token": "t-test"})
			return
		}
		writeJSON(t, w, map[string]any{"code": 0, "msg": "success"})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// cardActionEvent builds a minimal card action event for tests.
func cardActionEvent(openID, action string) *callback.CardActionTriggerEvent {
	return &callback.CardActionTriggerEvent{
		Event: &callback.CardActionTriggerRequest{
			Operator: &callback.Operator{OpenID: openID},
			Action:   &callback.CallBackAction{Value: map[string]any{"action": action}},
			Context:  &callback.Context{OpenChatID: "oc_test_chat", OpenMessageID: "om_test_message"},
		},
	}
}
