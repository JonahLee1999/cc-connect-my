package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/chenhg5/cc-connect/core"
)

// Message filter: an optional external command that decides, before anything
// visible happens (typing reaction, agent spawn, reply), whether an inbound
// message should reach the agent at all. Typical use: drop small talk in group
// chats where the bot answers without @mention.
//
// Config (platform options):
//
//	message_filter_cmd        = "/usr/bin/python3 /path/filter.py"  # split on whitespace
//	message_filter_timeout_ms = 4000                                # optional, default 4000
//
// The command receives one JSON object on stdin (see messageFilterInput) and
// must print {"action":"pass"} or {"action":"skip","reason":"..."} on stdout.
// Any error, timeout, non-zero exit or unparseable output passes the message
// through (fail-open). Reaction approvals are never filtered.

const defaultMessageFilterTimeout = 4 * time.Second

type messageFilter struct {
	argv    []string
	timeout time.Duration
}

type messageFilterInput struct {
	Platform       string `json:"platform"`
	SessionKey     string `json:"session_key"`
	ChatID         string `json:"chat_id"`
	MessageID      string `json:"message_id"`
	ThreadRootID   string `json:"thread_root_id,omitempty"`
	IsThread       bool   `json:"is_thread"`
	IsFollowUp     bool   `json:"is_follow_up"` // in a thread and not the thread's root message
	UserID         string `json:"user_id"`
	UserName       string `json:"user_name,omitempty"`
	Text           string `json:"text"`
	HasAttachments bool   `json:"has_attachments"`
}

type messageFilterOutput struct {
	Action string `json:"action"`
	Reason string `json:"reason"`
}

func newMessageFilter(opts map[string]any) *messageFilter {
	cmd, _ := opts["message_filter_cmd"].(string)
	argv := strings.Fields(cmd)
	if len(argv) == 0 {
		return nil
	}
	timeout := defaultMessageFilterTimeout
	switch v := opts["message_filter_timeout_ms"].(type) {
	case int64:
		if v > 0 {
			timeout = time.Duration(v) * time.Millisecond
		}
	case int:
		if v > 0 {
			timeout = time.Duration(v) * time.Millisecond
		}
	case float64:
		if v > 0 {
			timeout = time.Duration(v) * time.Millisecond
		}
	}
	return &messageFilter{argv: argv, timeout: timeout}
}

func buildMessageFilterInput(platform string, msg *core.Message) messageFilterInput {
	in := messageFilterInput{
		Platform:   platform,
		SessionKey: msg.SessionKey,
		MessageID:  msg.MessageID,
		UserID:     msg.UserID,
		UserName:   msg.UserName,
		Text:       msg.Content,
		HasAttachments: len(msg.Images) > 0 || len(msg.Files) > 0 ||
			msg.Audio != nil || msg.Location != nil,
	}
	if rc, ok := msg.ReplyCtx.(replyContext); ok {
		in.ChatID = rc.chatID
	}
	if parts := strings.SplitN(msg.SessionKey, ":", 3); len(parts) == 3 {
		if root, ok := parseThreadRootID(parts[2]); ok {
			in.IsThread = true
			in.ThreadRootID = root
			in.IsFollowUp = msg.MessageID != "" && msg.MessageID != root
		}
	}
	return in
}

// allow reports whether msg should be dispatched to the agent.
func (f *messageFilter) allow(platform string, msg *core.Message) (bool, string) {
	payload, err := json.Marshal(buildMessageFilterInput(platform, msg))
	if err != nil {
		return true, "marshal error"
	}
	ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.argv[0], f.argv[1:]...)
	cmd.Stdin = bytes.NewReader(payload)
	// Bound the wait for pipes after a timeout kill: a child process that
	// inherited stdout would otherwise keep Run() blocked until it exits.
	cmd.WaitDelay = 200 * time.Millisecond
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return true, "filter error: " + err.Error()
	}
	var out messageFilterOutput
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &out); err != nil {
		return true, "unparseable filter output"
	}
	if strings.EqualFold(out.Action, "skip") {
		return false, out.Reason
	}
	return true, out.Reason
}

// passesMessageFilter applies the configured filter; true = dispatch.
func (p *Platform) passesMessageFilter(msg *core.Message) bool {
	f := p.messageFilter
	if f == nil || msg == nil || strings.HasPrefix(msg.Content, ReactionApprovalMarker) {
		return true
	}
	start := time.Now()
	ok, reason := f.allow(p.platformName, msg)
	elapsed := time.Since(start)
	if !ok {
		slog.Info(p.tag()+": message skipped by filter",
			"message_id", msg.MessageID, "session_key", msg.SessionKey, "reason", reason, "elapsed", elapsed)
		return false
	}
	if strings.HasPrefix(reason, "filter error") || reason == "unparseable filter output" || reason == "marshal error" {
		slog.Warn(p.tag()+": message filter failed, passing message through",
			"message_id", msg.MessageID, "reason", reason, "elapsed", elapsed)
	}
	return true
}
