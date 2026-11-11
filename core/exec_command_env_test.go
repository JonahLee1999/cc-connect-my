package core

import (
	"strings"
	"testing"
)

// Exec custom commands get CC_PROJECT / CC_SESSION_KEY / CC_USER_ID so they
// can act on the chat they were run from.
func TestExecuteShellCommand_PassesSessionEnv(t *testing.T) {
	p := &stubPlatformEngine{n: "feishu"}
	e := NewEngine("mac", &stubAgent{}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "feishu:oc_123:root:om_1", UserID: "ou_admin", ReplyCtx: "ctx"}
	cmd := &CustomCommand{Name: "whereami", Exec: `echo "P=$CC_PROJECT K=$CC_SESSION_KEY U=$CC_USER_ID"`, WorkDir: t.TempDir()}

	e.executeShellCommand(p, msg, cmd, nil)

	out := strings.Join(p.getSent(), "\n")
	for _, want := range []string{"P=mac", "K=feishu:oc_123:root:om_1", "U=ou_admin"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output %q", want, out)
		}
	}
}
