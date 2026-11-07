package codex

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"strings"

	"github.com/chenhg5/cc-connect/core"
)

// sessionPrompts reads the configured policy at startup and every new/resumed
// agent session. An absent/empty required file is an error, never a fallback to
// weaker instructions. Already-running sessions keep their instruction snapshot
// until restarted. The path is pinned at construction, outside workspace binds.
func (a *Agent) sessionPrompts() (string, string, error) {
	a.mu.RLock()
	system, inline, path := a.systemPrompt, a.appendPrompt, a.appendPromptFile
	a.mu.RUnlock()
	if path == "" {
		return system, inline, nil
	}
	skill, err := core.LoadSkillFile(path)
	if err != nil {
		return "", "", fmt.Errorf("codex: append_system_prompt_file: %w", err)
	}
	policy := "Project instructions from " + path + ":\n\n" + skill.Prompt
	if inline != "" {
		policy = inline + "\n\n" + policy
	}
	digest := sha256.Sum256([]byte(skill.Prompt))
	slog.Info("codex: project instruction file loaded", "path", path, "sha256", fmt.Sprintf("%x", digest), "chars", len([]rune(skill.Prompt)))
	return system, strings.TrimSpace(policy), nil
}
