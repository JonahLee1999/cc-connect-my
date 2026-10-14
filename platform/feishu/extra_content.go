package feishu

import "strings"

// joinFeishuExtraContent joins non-empty parts with a blank line. Copied from
// upstream main (introduced alongside group_chat_history_share); the picked
// inbound coalescing (#2022) depends on it.
func joinFeishuExtraContent(parts ...string) string {
	var nonEmpty []string
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			nonEmpty = append(nonEmpty, strings.TrimSpace(part))
		}
	}
	return strings.Join(nonEmpty, "\n\n")
}
