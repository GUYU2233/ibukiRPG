// Package canon 是 Dynamic Canon 提议 Agent：在“自由推演”模式下请求大模型提出下一个主线节点。
// 模型输出只是提案（command.NodeProposal），必须经过 engine.ValidateProposal 与 Core 才会成为正史。
package canon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

const system = `[CORE_RULES]
你是文字 RPG 的“剧情导演”。玩家已经偏离预设主线，进入“自由推演”模式。
请根据世界观、当前局势与最近事件，提出**一个**新的主线节点，推动故事继续。
- 你不是真相源：只能使用下方列出的地点 / 人物 / 敌人 / 物品 ID，不能编造新 ID。
- 目标类型 goal 只能是 reach（抵达地点）/ talk（与人物交谈）/ defeat（击败 1–4 个敌人）/ obtain（取得物品）之一。
- 敌人等级合计不得超过“敌人预算”，不能安排首领。
- 奖励克制：reward_xp 不超过上限，reward_gold 不超过 200。
- 标题 2–24 字，目标 4–80 字，概要不超过 240 字，全部用中文原创表述。
- 可选 new_character：只在剧情确实需要一位新的重要人物时提出（名字 2–12 字，不得与已有人物重名）。

[OUTPUT_SCHEMA]
只输出一个 JSON 对象：
{"title": "", "objective": "", "goal": "reach|talk|defeat|obtain", "ref": "地点/人物/物品 ID（defeat 时为空）",
 "location": "地点 ID 或空", "enemies": ["敌人 ID"], "reward_xp": 0, "reward_gold": 0, "summary": "",
 "new_character": {"name": "", "role": "", "description": ""} 或 null}`

// BuildMessages 构造提示词（只含玩家已知信息与内容包公开数据）。
func BuildMessages(p *loader.Package, s *state.State, recent []string) []provider.Message {
	var b strings.Builder
	if p.Mainline.Style != "" {
		fmt.Fprintf(&b, "[WORLD]\n%s\n", p.Mainline.Style)
	}
	loc := p.Locations[s.Player.Location]
	fmt.Fprintf(&b, "[STATE]\n玩家：%s，等级 %d，位于 %s（%s）\n", p.Player.Name(), s.PlayerLevel(p), loc.Name, loc.ID)
	fmt.Fprintf(&b, "敌人预算：%d；奖励经验上限：%d\n", s.PlayerLevel(p)*3+3, p.Mainline.MaxRewardXP)
	b.WriteString("[LOCATIONS]\n")
	for _, id := range p.LocationIDs {
		fmt.Fprintf(&b, "%s：%s\n", id, p.Locations[id].Name)
	}
	b.WriteString("[CHARACTERS]\n")
	for _, id := range p.NPCIDs {
		np := s.NPCs[id]
		if np == nil || np.Location == "" {
			continue
		}
		c := p.Characters[id]
		fmt.Fprintf(&b, "%s：%s（%s）\n", id, c.Name(), c.Identity.Role)
	}
	if p.Combat != nil {
		b.WriteString("[ENEMIES]\n")
		for _, id := range p.Combat.EnemyIDs {
			e := p.Combat.Enemies[id]
			if e.Tier == "boss" {
				continue
			}
			fmt.Fprintf(&b, "%s：%s（Lv.%d）\n", id, e.Name, e.Level)
		}
	}
	b.WriteString("[ITEMS]\n")
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		if it.Kind == "key" || (p.Combat != nil && p.Combat.Config.RarityRank(it.Rarity) > p.Combat.Config.RarityRank("rare")) {
			continue
		}
		fmt.Fprintf(&b, "%s：%s\n", id, it.Name)
	}
	if s.RPG != nil {
		b.WriteString("[PREVIOUS_NODES]\n")
		for _, n := range s.RPG.Main.Nodes {
			fmt.Fprintf(&b, "%s（%s）\n", n.Title, n.Status)
		}
	}
	if len(recent) > 0 {
		b.WriteString("[RECENT]\n" + strings.Join(recent, "\n") + "\n")
	}
	return []provider.Message{{Role: "system", Content: system}, {Role: "user", Content: b.String()}}
}

type output struct {
	Title        string   `json:"title"`
	Objective    string   `json:"objective"`
	Goal         string   `json:"goal"`
	Ref          string   `json:"ref"`
	Location     string   `json:"location"`
	Enemies      []string `json:"enemies"`
	RewardXP     int      `json:"reward_xp"`
	RewardGold   int      `json:"reward_gold"`
	Summary      string   `json:"summary"`
	NewCharacter *struct {
		Name        string `json:"name"`
		Role        string `json:"role"`
		Description string `json:"description"`
	} `json:"new_character"`
}

// Parse 把模型输出解析为提案（不做语义校验，语义校验在 Core）。
func Parse(text string) (*command.NodeProposal, error) {
	raw := strings.TrimSpace(text)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	var o output
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		return nil, fmt.Errorf("canon output is not valid JSON: %w", err)
	}
	pr := &command.NodeProposal{Title: o.Title, Objective: o.Objective, Goal: o.Goal, Ref: o.Ref, Location: o.Location,
		Enemies: o.Enemies, RewardXP: o.RewardXP, RewardGold: o.RewardGold, Summary: o.Summary}
	if c := o.NewCharacter; c != nil && strings.TrimSpace(c.Name) != "" {
		pr.NewCharacter = &command.CharacterProposal{Name: c.Name, Role: c.Role, Description: c.Description}
	}
	return pr, nil
}

// Proposer 调用 Provider 生成提案。
type Proposer struct {
	Provider provider.Provider
	// Env 非 nil 时先做只读检索（DirectorScope）：自由推演总是信息不足的场景，导演先查设定再提案。
	Env *tools.Env
	// Retrieval 是 [TOOLS] + [RETRIEVAL_POLICY] 段（故事包可修补）。
	Retrieval string
	Budget    tools.Budget
	// Steps 记录最近一次提案的检索步骤（调试 / 测试）。
	Steps []tools.Step
	Mode  string
}

// Propose 请求一个新节点提案。
func (x *Proposer) Propose(ctx context.Context, p *loader.Package, s *state.State, recent []string) (*command.NodeProposal, error) {
	msgs := BuildMessages(p, s, recent)
	x.Steps, x.Mode = nil, "none"
	if x.Env != nil {
		if x.Retrieval != "" {
			msgs[0].Content += "\n" + strings.TrimRight(x.Retrieval, "\n")
		}
		msgs[1].Content += "\n[LOOKUP]\n玩家已进入自由推演：先检索与当前地点、在场人物、最近事件相关的设定，再提出节点。"
		loop := &tools.Loop{Provider: x.Provider, Env: x.Env, Scope: tools.Director(), Budget: x.Budget, Temperature: 0.7, JSON: true}
		o := loop.Run(ctx, msgs, p.Locations[s.Player.Location].Name+" "+strings.Join(recent, " "))
		x.Steps, x.Mode = o.Steps, o.Mode
		if o.Answer != nil {
			if pr, err := Parse(o.Answer.Text); err == nil {
				return pr, nil
			}
		}
		msgs = tools.Flatten(msgs, o)
	}
	resp, err := x.Provider.Generate(ctx, provider.Request{Messages: msgs, Temperature: 0.7, MaxTokens: 600, JSON: true})
	if err != nil {
		return nil, err
	}
	return Parse(resp.Text)
}
