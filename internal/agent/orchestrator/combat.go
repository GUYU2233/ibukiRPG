package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/combat/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
)

const combatSystem = `你是文字 RPG 的战斗意图解析器。把玩家对行动的自然语言描述解析成 JSON，不要写任何数值，不要判断成败。
输出格式：{"actions":[{"kind":"attack|skill|maneuver|move|defend|item|mech|talk|flee|env","target":目标ID,"part":部位ID,"means":武器ID,"skill":技能ID,"maneuver":战技ID,"env":"环境要素","item":物品ID,"to":"移动去向"}],
"circumstances":[{"tag":修正标签ID,"why":"玩家描述中的依据"}],"absurd":"如果描述违背物理或世界能力边界，写出原因，否则留空","self_check":"一句话"}
- 每回合最多 1 次移动 + 1 个主动作。只能使用 [CONTEXT] 里出现的 ID；修正标签只能从 tags 里选，且必须有玩家描述的依据。
- 玩家说的部位如果不在已知部位里，写 part 为玩家原话（引擎会当作试探）。`

// combatIntent 解析自由战斗意图：AI（combat_adjudicate）优先，失败 / 拒答 / 离线时回退到规则解析器。
func (s *Session) combatIntent(ctx context.Context, g *game, st *state.State, input string) (freeform.Intent, string) {
	u := st.RPG.Combat.Current()
	fc := engine.FreeContext(g.pkg, st, u)
	rules := freeform.RulesIntent(input, fc)
	p, tgt, ok := s.providerFor(router.TaskCombat)
	if !ok {
		return rules, "rules"
	}
	// 玩家视角：未知部位不告诉 AI 名字之外的信息
	type tview struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Parts    []string `json:"known_parts,omitempty"`
		Statuses []string `json:"statuses,omitempty"`
		Size     string   `json:"size,omitempty"`
	}
	var targets []tview
	for _, t := range fc.Targets {
		v := tview{ID: t.ID, Name: t.Name, Statuses: t.Statuses, Size: t.Size}
		for _, pt := range t.Parts {
			if pt.Known {
				v.Parts = append(v.Parts, pt.ID+"="+pt.Name)
			}
		}
		targets = append(targets, v)
	}
	var means, skills, mans, tags []string
	for _, m := range fc.Means {
		means = append(means, m.ID+"="+m.Name)
	}
	for _, sk := range fc.Skills {
		skills = append(skills, sk.ID+"="+sk.Name)
	}
	for _, m := range fc.Maneuvers {
		mans = append(mans, m.ID+"="+m.Name)
	}
	for _, t := range fc.Tags {
		tags = append(tags, t.ID+"="+t.Label)
	}
	ctxJSON, _ := json.Marshal(map[string]any{"targets": targets, "means": means, "skills": skills, "maneuvers": mans, "tags": tags, "scene_facts": fc.SceneFacts,
		"limits": g.pkg.Balance.AbilityLimits})
	msgs := []provider.Message{{Role: "system", Content: combatSystem}, {Role: "user", Content: fmt.Sprintf("[CONTEXT]\n%s\n[PLAYER_ACTION]\n%s", ctxJSON, input)}}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	resp, err := p.Generate(cctx, provider.Request{Messages: msgs, Temperature: 0.2, MaxTokens: 400, JSON: true})
	if err != nil || structured.IsRefusal(resp.Text, nil) {
		return rules, "rules"
	}
	raw := strings.TrimSpace(resp.Text)
	var in freeform.Intent
	if json.Unmarshal([]byte(raw), &in) != nil {
		if json.Unmarshal([]byte(structured.Repair(raw)), &in) != nil {
			return rules, "rules"
		}
	}
	if len(in.Actions) == 0 {
		return rules, "rules"
	}
	in.Raw = input
	in.Source = "ai"
	if tgt.Tier != router.TierFull {
		in.Source = "local"
	}
	return in, in.Source
}
