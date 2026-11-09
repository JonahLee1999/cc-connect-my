package feishu

import (
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

// Question and permission cards must be shared (update_multi) so an answer
// clicked by one person updates the card for everyone in the group.
func TestRenderCardMap_DecisionCardsAreShared(t *testing.T) {
	shared := func(m map[string]any) bool {
		cfg, _ := m["config"].(map[string]any)
		v, _ := cfg["update_multi"].(bool)
		return v
	}
	ask := core.NewCard().Markdown("q").Buttons(core.CardButton{Text: "A", Value: "askq:0:1"}).Build()
	perm := core.NewCard().Markdown("p").Buttons(core.CardButton{Text: "Allow", Value: "perm:allow"}).Build()
	nav := core.NewCard().Markdown("n").Buttons(core.CardButton{Text: "Next", Value: "nav:/list 2"}).Build()
	if !shared(renderCardMap(ask, "")) {
		t.Error("askq card should be shared")
	}
	if !shared(renderCardMap(perm, "")) {
		t.Error("perm card should be shared")
	}
	if shared(renderCardMap(nav, "")) {
		t.Error("navigation card should keep the default (per-user updates)")
	}
	if !shared(markCardShared(renderCardMap(core.NewCard().Markdown("✅ A").Build(), ""))) {
		t.Error("markCardShared should set update_multi")
	}
}
