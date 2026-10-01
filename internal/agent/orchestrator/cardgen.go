package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
)

// ---------- 卡片生成 / 修改（card_gen，第 14.2 / 14.3 节） ----------

const cardGenSystem = `你是文字 RPG 的“卡片编辑器”，按玩家的要求生成或修改设定卡（人物 / 武器 / 物品 / 机甲 / 势力 / 地点 / 设定条目）。
规则：
- 只输出一个 JSON 对象：{"changes":[变更...],"note":"一句话说明（可空）"}，不要任何其它文字。
- 修改已有卡片用 patch：{"op":"patch","target":"实体ID","path":"fields.<字段>" 或 "stats.<数值>" 或 "tags","value":新值,"reason":"玩家要求：……"}。
- 新建卡片用 create：{"op":"create","target":"<命名空间>:<类型>/<slug>","kind":"character|item|mech|faction|location|lore","value":{"importance":2,"fields":{"name":"…","description":"…"},"public":["name","description"],"stats":{…}},"reason":"玩家要求：……"}。slug 只用小写字母、数字、下划线。
- 数值必须符合世界的强度体系（不要给出远超同类的数值）；设定必须符合世界观（没有魔法的世界不能出现魔法）。
- 不要修改玩家没有要求的字段。`

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
	fmt.Fprintf(&sb, "[NAMESPACE]\n%s\n[REQUEST]\n%s\n", validate.GenNamespace(g.pkg), instruction)
	cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	resp, err := pr.Generate(cctx, provider.Request{Messages: []provider.Message{{Role: "system", Content: cardGenSystem}, {Role: "user", Content: sb.String()}},
		Temperature: 0.4, MaxTokens: 900, JSON: true})
	if err != nil {
		return dto.ChangePreviewV1{}, err
	}
	if structured.IsRefusal(resp.Text, nil) {
		return dto.ChangePreviewV1{}, errors.New("模型拒绝了这个修改要求，换个说法试试")
	}
	raw := strings.TrimSpace(resp.Text)
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
	pv.Note = out.Note
	return pv, err
}
