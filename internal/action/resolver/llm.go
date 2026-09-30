package resolver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// LLM 是基于大模型的 Action Resolver：一次结构化 JSON 调用同时完成意图解析、
// 合理性判断与 Action 匹配（第 7 节）。输出只是建议，之后仍要经过 Validator 与 Core。
type LLM struct {
	Provider provider.Provider
}

// Name 返回解析器名。
func (l *LLM) Name() string { return "ai:" + l.Provider.Name() }

// 稳定的提示词段落放在最前面，便于 Provider 的 Prefix Cache（第 35 节）。
const resolverSystem = `[CORE_RULES]
你是文字 RPG 的“动作解析器”。你的任务是把玩家的中文输入解析成结构化 JSON，交给确定性的游戏引擎执行。
- 你不是真相源：不能决定结果、不能修改金币/物品/关系/时间，只能提出建议。
- 合理性只判断“能不能尝试”，而不是“玩家说的话是不是真的”。例如“我告诉守卫我是国王派来的”是欺瞒尝试（ALLOW_WITH_CHECK），不是知识违规。
- 只有在物理上不可能、这个世界不存在的能力（魔法、飞行、瞬移）、或试玩版不支持的战斗行为时才 REJECT，并给出简短中文理由。
- 能匹配下方“动作目录”时优先匹配（kind=action）；想去别的地点用 kind=move；其余合理行为用 kind=freeform（自然语言长尾）。
- 目标只能是“在场人物”中的 ID；物品只能用“物品”中的 ID；地点只能用“出口”中的 ID。
- 需要目标但玩家没说清、且在场不止一人时，用 kind=clarify 并在 reason 里提问。

[OUTPUT_SCHEMA]
只输出一个 JSON 对象，不要输出任何其他文字：
{
  "intent": "简短英文意图，如 persuade / distraction / move",
  "kind": "action | move | freeform | clarify | reject",
  "matched_action": "动作 ID 或 null",
  "targets": ["人物 ID"],
  "item": "物品 ID 或 null",
  "destination": "地点 ID 或 null",
  "reasonability": "ALLOW | ALLOW_WITH_CHECK | ALLOW_WITH_CONSEQUENCE | REJECT",
  "reason": "拒绝或澄清时给玩家看的一句中文，否则为空",
  "description": "freeform 时：以第三人称省略主语的动作描述，如“在雨里大声唱歌”",
  "suggested_check": {"skill": "技能 ID", "difficulty": 5-25} 或 null,
  "estimated_time": 分钟整数,
  "tags": ["noise" 等],
  "proposed_effects": [{"type": "noise|attention|scene_fact|relationship_nudge|minor_condition|none", "target": "人物 ID", "text": "场景事实或状态 ID", "value": 整数, "when": "success|failure|always"}],
  "confidence": 0.0-1.0
}
freeform 的 proposed_effects 只能使用上面列出的类型；minor_condition 的 text 只能是 tired / soaked / embarrassed / inspired；relationship_nudge 的 value 在 -2..2。`

// BuildMessages 构造解析请求（公开给 Eval Runner 复用，确保录音哈希一致）。
func BuildMessages(in Input) []provider.Message {
	return []provider.Message{
		{Role: "system", Content: resolverSystem},
		{Role: "user", Content: sceneContext(in.Pkg, in.State) + "\n[PLAYER_INPUT]\n" + strings.TrimSpace(in.Text)},
	}
}

// sceneContext 只包含 PlayerScope 可见的信息（不含 NPC 秘密）。
func sceneContext(p *loader.Package, s *state.State) string {
	var b strings.Builder
	loc := p.Locations[s.Player.Location]
	fmt.Fprintf(&b, "[SCENE]\n地点：%s（%s）\n", loc.Name, loc.ID)
	if facts := s.SceneFacts[loc.ID]; len(facts) > 0 {
		fmt.Fprintf(&b, "场景事实：%s\n", strings.Join(facts, "；"))
	}
	b.WriteString("出口：")
	for i, e := range loc.Exits {
		if i > 0 {
			b.WriteString("，")
		}
		fmt.Fprintf(&b, "%s（%s）", e.Label, e.To)
	}
	b.WriteString("\n在场人物：")
	present := s.NPCsAt(p, loc.ID)
	if len(present) == 0 {
		b.WriteString("无")
	}
	for i, id := range present {
		if i > 0 {
			b.WriteString("；")
		}
		c := p.Characters[id]
		fmt.Fprintf(&b, "%s（%s，%s）", c.Name(), id, c.Identity.Role)
	}
	fmt.Fprintf(&b, "\n玩家：%s，铜币 %d", s.Player.Name, s.Player.Gold)
	var inv []string
	for _, id := range p.ItemIDs {
		if n := s.Player.Inventory[id]; n > 0 {
			inv = append(inv, fmt.Sprintf("%s×%d（%s）", p.Items[id].Name, n, id))
		}
	}
	if len(inv) > 0 {
		fmt.Fprintf(&b, "，背包：%s", strings.Join(inv, "、"))
	}
	if loc.Shop != nil {
		var shop []string
		for _, id := range loc.Shop.Items {
			shop = append(shop, fmt.Sprintf("%s %d 铜币（%s）", p.Items[id].Name, p.Items[id].Price, id))
		}
		fmt.Fprintf(&b, "\n可购买：%s", strings.Join(shop, "、"))
	}
	for _, id := range p.StoryIDs {
		if st := s.Stories[id]; st.Status == state.StoryActive {
			fmt.Fprintf(&b, "\n进行中的事件：%s", p.Stories[id].Title)
		}
	}
	b.WriteString("\n[ACTIONS]\n")
	for _, id := range p.ActionIDs {
		d := p.Actions[id]
		fmt.Fprintf(&b, "- %s：%s（目标：%s）%s\n", id, d.Name, d.Target, d.Description)
	}
	b.WriteString("[SKILLS]\n")
	for _, sk := range p.Rules.Skills {
		fmt.Fprintf(&b, "%s=%s ", sk.ID, sk.Name)
	}
	b.WriteString("\n")
	return b.String()
}

type llmOutput struct {
	Intent         string   `json:"intent"`
	Kind           string   `json:"kind"`
	MatchedAction  *string  `json:"matched_action"`
	Targets        []string `json:"targets"`
	Item           *string  `json:"item"`
	Destination    *string  `json:"destination"`
	Reasonability  string   `json:"reasonability"`
	Reason         string   `json:"reason"`
	Description    string   `json:"description"`
	SuggestedCheck *struct {
		Skill      string  `json:"skill"`
		Difficulty float64 `json:"difficulty"`
	} `json:"suggested_check"`
	EstimatedTime   float64  `json:"estimated_time"`
	Tags            []string `json:"tags"`
	ProposedEffects []struct {
		Type   string  `json:"type"`
		Target string  `json:"target"`
		Text   string  `json:"text"`
		Value  float64 `json:"value"`
		When   string  `json:"when"`
	} `json:"proposed_effects"`
	Confidence float64 `json:"confidence"`
}

// Resolve 调用模型并把输出映射回内容包 ID。无法映射时返回错误，由调用方降级到离线解析。
func (l *LLM) Resolve(ctx context.Context, in Input) (Resolution, error) {
	resp, err := l.Provider.Generate(ctx, provider.Request{Messages: BuildMessages(in), Temperature: 0.1, MaxTokens: 500, JSON: true})
	if err != nil {
		return Resolution{}, err
	}
	return ParseOutput(resp.Text, in)
}

// ParseOutput 解析并校验模型输出。
func ParseOutput(text string, in Input) (Resolution, error) {
	p, s := in.Pkg, in.State
	raw := strings.TrimSpace(text)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	var o llmOutput
	if err := json.Unmarshal([]byte(raw), &o); err != nil {
		return Resolution{}, fmt.Errorf("resolver output is not valid JSON: %w", err)
	}
	res := Resolution{Source: "ai", Intent: o.Intent, Reasonability: strings.ToUpper(o.Reasonability), Reason: strings.TrimSpace(o.Reason)}
	res.Confidence = int(o.Confidence * 1000)
	switch res.Reasonability {
	case Allow, AllowWithCheck, AllowWithConsequence, Reject:
	default:
		res.Reasonability = Allow
	}
	present := s.NPCsAt(p, s.Player.Location)
	var targets []string
	for _, t := range o.Targets {
		if id := LookupID(p, "npc", t); id != "" && !slices.Contains(targets, id) {
			targets = append(targets, id)
		}
	}
	deref := func(x *string) string {
		if x == nil {
			return ""
		}
		return *x
	}
	kind := strings.ToLower(strings.TrimSpace(o.Kind))
	if res.Reasonability == Reject {
		kind = KindReject
	}
	switch kind {
	case KindReject:
		res.Kind, res.Reasonability = KindReject, Reject
		if res.Reason == "" {
			res.Reason = "这件事现在做不到。"
		}
	case KindClarify:
		res.Kind = KindClarify
		if res.Reason == "" {
			res.Reason = "能说得更具体一点吗？"
		}
	case KindMove:
		res.Kind, res.Destination = KindMove, LookupID(p, "location", deref(o.Destination))
		if res.Destination == "" {
			return Resolution{}, errors.New("resolver: unknown destination")
		}
	case KindAction:
		res.Kind, res.Action = KindAction, LookupID(p, "action", deref(o.MatchedAction))
		if res.Action == "" {
			return Resolution{}, fmt.Errorf("resolver: unknown action %q", deref(o.MatchedAction))
		}
		if len(targets) > 0 {
			res.Target = targets[0]
			if !slices.Contains(present, res.Target) {
				res.Kind, res.Reasonability = KindReject, Reject
				res.Reason = p.EntityName(res.Target) + "不在这里。"
			}
		}
		res.Item = LookupID(p, "item", deref(o.Item))
	case KindFreeform:
		fr := &command.Freeform{Description: strings.TrimSpace(o.Description), Targets: targets, Tags: o.Tags,
			EstimatedMinutes: int(o.EstimatedTime), Reasonability: res.Reasonability}
		if fr.Description == "" {
			fr.Description = describe(normalize(in.Text))
		}
		if o.SuggestedCheck != nil && o.SuggestedCheck.Skill != "" {
			fr.Check = &command.SuggestedCheck{Skill: o.SuggestedCheck.Skill, Difficulty: int(o.SuggestedCheck.Difficulty)}
		}
		for _, e := range o.ProposedEffects {
			pe := command.ProposedEffect{Type: e.Type, Target: LookupID(p, "npc", e.Target), Text: e.Text, Value: int(e.Value), When: e.When}
			fr.Effects = append(fr.Effects, pe)
		}
		res.Kind, res.Freeform = KindFreeform, fr
	default:
		return Resolution{}, fmt.Errorf("resolver: unknown kind %q", o.Kind)
	}
	return res, nil
}

// WithFallback 先用 primary，失败（超时、网络、无效 JSON）时用 fallback。
// 解析发生在任何 Command 执行之前，因此降级不会重复执行、也不会重新掷骰。
type WithFallback struct {
	Primary, Fallback Resolver
	// OnFallback 记录降级原因（诊断用）。
	OnFallback func(err error)
}

// Name 返回解析器名。
func (w *WithFallback) Name() string { return w.Primary.Name() + "+" + w.Fallback.Name() }

// Resolve 实现 Resolver。
func (w *WithFallback) Resolve(ctx context.Context, in Input) (Resolution, error) {
	r, err := w.Primary.Resolve(ctx, in)
	if err == nil {
		return r, nil
	}
	if w.OnFallback != nil {
		w.OnFallback(err)
	}
	r, ferr := w.Fallback.Resolve(context.WithoutCancel(ctx), in)
	if ferr != nil {
		return r, ferr
	}
	r.Source += "(fallback)"
	return r, nil
}
