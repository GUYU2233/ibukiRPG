package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/power"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
)

// ---------- 卡片生成 / 修改（card_gen，第 14.2 / 14.3 节） ----------

const cardGenSystem = `你是文字 RPG 的“卡片编辑器”，按玩家的要求生成或修改设定卡（人物 / 武器 / 物品 / 机甲 / 势力 / 地点 / 设定条目）。
规则：
- 只输出一个 JSON 对象：{"changes":[变更...],"note":"一句话说明（可空）"}，不要任何其它文字。
- 修改已有卡片用 patch：{"op":"patch","target":"实体ID","path":"fields.<字段>" 或 "combat.stats.<数值>","value":新值,"reason":"玩家要求：……"}。
- 新建卡片用 create：{"op":"create","target":"<命名空间>:<类型>/<slug>","kind":"character|item|mech|faction|location|lore","value":{"importance":2,"fields":{"name":"…","description":"…"},"public":["name","description"],"tags":["weapon|armor|enemy|skill（可选）","common|uncommon|rare|epic|legendary"],"stats":{…,"level":1}},"reason":"玩家要求：……"}。slug 只用小写字母、数字、下划线。
- 数值必须在 [POWER] 的强度预算之内（超出的会被等比缩小）；设定必须符合世界观（没有魔法的世界不能出现魔法）。
- 不要修改玩家没有要求的字段。`

// cardGenToolSystem 是支持函数调用时的卡片编辑器提示词：卡片通过写入工具生成 / 修改（每次调用都经校验）。
const cardGenToolSystem = `你是文字 RPG 的“卡片编辑器”，按玩家的要求生成或修改设定卡（人物 / 敌人 / 武器 / 护甲 / 物品 / 技能 / 机甲 / 势力 / 地点 / 设定条目）。
- 新卡片调用 entity_generate（数值按“种类 × 稀有度 × 等级”的强度预算自动缩放；不确定时先调用 rules_power_budget）。
- 修改已有卡片调用 world_propose_change，用 patch：path 为 "fields.<字段>" 或 "combat.stats.<数值>"。
- 工具会告诉你哪些修改被拒绝以及原因；按原因调整后可以再提交一次。设定必须符合世界观；不要修改玩家没有要求的字段。
- 完成后用一句话说明你做了什么（不要输出 JSON）。`

// RequestEdit 让 card_gen 按玩家的自然语言要求生成修改提案，并返回预览（玩家确认后用 ApplyPreview 提交）。
// target 为空表示新建卡片。玩家明确要求的修改一律先预览、后确认（第 14.3 节）。
func (s *Session) RequestEdit(ctx context.Context, target, instruction string) (dto.ChangePreviewV1, error) {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return dto.ChangePreviewV1{}, errors.New("请写下你想怎么改")
	}
	if len([]rune(instruction)) > 400 {
		return dto.ChangePreviewV1{}, errors.New("要求太长了（最多 400 字）")
	}
	pr, _, ok := s.providerFor(router.TaskCardGen)
	if !ok {
		return dto.ChangePreviewV1{}, errors.New("卡片生成需要联网模型（生成设置 → 卡片生成）")
	}
	_, st, g, err := s.current()
	if err != nil {
		return dto.ChangePreviewV1{}, err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "[WORLD]\n%s：%s\n", g.pkg.Manifest.Name, g.pkg.Manifest.Description)
	if lim := g.pkg.Balance.AbilityLimits; len(lim) > 0 {
		fmt.Fprintf(&sb, "能力边界：%s\n", strings.Join(lim, "；"))
	}
	if target != "" {
		d := st.Doc(g.pkg, target)
		if d == nil {
			return dto.ChangePreviewV1{}, fmt.Errorf("找不到实体 %s", target)
		}
		b, _ := json.Marshal(d)
		fmt.Fprintf(&sb, "[CARD]\n%s\n", b)
	}
	fmt.Fprintf(&sb, "[POWER]\n%s", power.Table(validate.PowerRef(g.pkg)))
	if target != "" {
		if d := st.Doc(g.pkg, target); d != nil {
			pp, _ := validate.DocBudget(g.pkg, d)
			fmt.Fprintf(&sb, "这张卡：%s（当前强度分 %d）\n", power.Describe(validate.PowerRef(g.pkg), pp), power.Score(d.Stats))
		}
	}
	fmt.Fprintf(&sb, "[NAMESPACE]\n%s\n[REQUEST]\n%s\n", validate.GenNamespace(g.pkg), instruction)
	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var text string
	if tc, ok := pr.(provider.ToolCaller); ok && tc.SupportsTools() && !s.router.Settings().NoWorldTools {
		// 支持函数调用：卡片通过 entity_generate / world_propose_change 生成（每次调用都经校验）
		slot, _, _, _ := s.current()
		env := s.toolEnv(ctx, slot, g, st)
		env.Writer = newWriter(g, st, change.SourceUserRequest, router.TierFull)
		o := (&tools.Loop{Provider: pr, Env: env, Scope: tools.Director().WithWrite(), Temperature: 0.4,
			Budget: tools.Budget{MaxIters: 4, MaxCalls: 6, MaxResultRunes: 5000, MaxTokens: 900}}).Run(cctx,
			[]provider.Message{{Role: "system", Content: cardGenToolSystem}, {Role: "user", Content: sb.String()}}, "")
		if cs := env.Writer.Proposed(); len(cs) > 0 {
			for i := range cs {
				if cs[i].Reason == "" {
					cs[i].Reason = "你的要求：" + instruction
				}
			}
			pv, err := s.PreviewChanges(ctx, change.SourceUserRequest, cs)
			// 编辑器可接触全知设定；自由生成的说明不能直接进入玩家预览。
			pv.Note = cardPreviewNote(pv)
			pv.Via = "tools"
			return pv, err
		}
		if o.Answer != nil && strings.Contains(o.Answer.Text, "{") {
			text = o.Answer.Text // 模型没有调用工具而是直接给了 JSON：按 JSON 兜底解析
		}
	}
	if text == "" {
		resp, err := pr.Generate(cctx, provider.Request{Messages: []provider.Message{{Role: "system", Content: cardGenSystem}, {Role: "user", Content: sb.String()}},
			Temperature: 0.4, MaxTokens: 900, JSON: true})
		if err != nil {
			return dto.ChangePreviewV1{}, err
		}
		text = resp.Text
	}
	if structured.IsRefusal(text, nil) {
		return dto.ChangePreviewV1{}, errors.New("模型拒绝了这个修改要求，换个说法试试")
	}
	raw := strings.TrimSpace(text)
	if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	var out struct {
		Changes []change.Change `json:"changes"`
		Note    string          `json:"note"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		if err2 := json.Unmarshal([]byte(structured.Repair(raw)), &out); err2 != nil {
			return dto.ChangePreviewV1{}, errors.New("模型返回的修改无法解析，请重试")
		}
	}
	if len(out.Changes) == 0 {
		return dto.ChangePreviewV1{}, errors.New("模型没有给出任何修改")
	}
	for i := range out.Changes {
		if out.Changes[i].Reason == "" {
			out.Changes[i].Reason = "你的要求：" + instruction
		}
	}
	pv, err := s.PreviewChanges(ctx, change.SourceUserRequest, out.Changes)
	pv.Note = cardPreviewNote(pv)
	pv.Via = "json"
	return pv, err
}

// cardPreviewNote 不复述模型输出或隐藏提案，只提示确认流程。
func cardPreviewNote(pv dto.ChangePreviewV1) string {
	if pv.Token == "" {
		return "没有可确认的修改；请查看拒绝原因或调整要求。"
	}
	return "修改预览已生成；请核对可见差异，确认后才会生效。"
}
