package narrator

import (
	"context"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/narrative/guard"
)

// WorldOutput 是“叙事 + 世界更新”合并调用的结果（第 11.3 节）。
type WorldOutput struct {
	Output
	World *structured.World `json:"world,omitempty"`
	// WorldErr：parse（格式错误，已重试一次）/ refused（内容审核拒绝）/ missing / 空 = 成功。
	WorldErr string `json:"world_error,omitempty"`
	Repaired bool   `json:"repaired,omitempty"`
	Retried  bool   `json:"retried,omitempty"`
	Refused  bool   `json:"refused,omitempty"`
}

// worldRules 是 WORLD 段的输出约定（按能力档位精简）。
func worldRules(tier string) string {
	var sb strings.Builder
	sb.WriteString(`[WORLD_UPDATE]
叙事写完后，另起一行输出 <<<WORLD>>>，然后输出一个 JSON 对象，最后一行输出 <<<END>>>。JSON 只交给引擎，玩家看不到。
- 只记录本回合叙事里确实发生的事。没有变化就输出 {"v":1}。
- "reveals"：玩家在本回合得知的信息。格式 {"entity":ID,"fields":[字段],"channel":"witness|dialogue|document|inference|rumor","source":说话人ID}。
  只能从 [REVEALABLE] 中选；隐藏真相字段写作 "hidden:<id>"。
- "scene_facts"：新出现、之后还要保持一致的场景细节（短句）。
- "impact"：{"level":"none|minor|major|break","why":"一句话"}，诚实自评这回合对世界的影响。
- "suggestions"：给玩家的 2–3 个下一步行动建议（短句）。
`)
	switch tier {
	case "minimal", "limited":
		// 本地小模型：精简格式（引擎转换成标准变更，校验规则相同）
		sb.WriteString(`- 世界修改用精简格式（可省略）：
  "edits":[{"id":实体ID,"field":"description|appearance|mood|status","text":"新的描述","why":"原因"}]
  "rel":[{"a":人物ID,"b":人物ID,"dim":"trust|affection|hostility|fear","delta":-10到10,"why":"原因"}]
`)
		if tier == "limited" {
			sb.WriteString(`  "moves":[{"id":角色ID,"to":地点ID,"why":"原因"}]
- 你是本地模型：每回合最多 3 项修改，只修改重要度低的实体，不要杀死重要人物，不要新建实体，不要改玩家的金钱和物品。
`)
		} else {
			sb.WriteString("- 你是本地小模型：每回合最多 2 项修改（只用 edits / rel），只写叙事里确实发生的小变化。\n")
		}
		return sb.String()
	}
	sb.WriteString(`- "changes"：世界设定的修改。格式 {"op":"create|patch|retire|restore|link|unlink|timeline_add|timeline_patch|timeline_cancel","target":ID,"path":"fields.<字段>","value":值,"reason":"原因","evidence":"叙事原句"}。
  玩家金钱 / 经验 / 物品用 {"op":"patch","target":"player","path":"gold|xp|inventory.<物品ID>","value":增量}；人物移动用 path "location"；
  人物死亡 / 离开用 {"op":"retire","target":ID,"value":{"reason":"death|left|lost|destroyed","text":"经过"}}。
  新实体的 target 用 "<命名空间>.gen:<种类>/<英文短名>"。数值修改会被引擎封顶；不合规的修改会被拒绝。
`)
	return sb.String()
}

// NarrateWorld 实现合并输出：流式叙事（遇到分隔符停止显示）→ 解析 WORLD → 本地修复 → 重试一次 → 仅叙事降级。
// 内容审核拒绝时不写任何 AI 内容（叙事用模板，Refused=true）。
func (l *LLM) NarrateWorld(ctx context.Context, b Brief, sections, tier string, onDelta func(string)) WorldOutput {
	msgs := BuildMessages(b)
	msgs[0].Content += "\n" + worldRules(tier)
	if strings.TrimSpace(sections) != "" {
		msgs[1].Content = sections + "\n" + msgs[1].Content
	}
	f := &structured.Filter{Out: onDelta}
	resp, err := l.Provider.Stream(ctx, provider.Request{Messages: msgs, Temperature: 0.8, MaxTokens: 1200}, f.Write)
	f.Flush()
	if err != nil {
		out := WorldOutput{Output: Output{Text: b.Base, Source: "template(fallback)", Corrected: true, Err: err.Error(), Guard: guard.Check(b.Base, b.Facts, b.Guard)}}
		if structured.IsRefusal("", err) {
			out.Refused, out.WorldErr = true, "refused"
		}
		return out
	}
	if structured.IsRefusal(resp.Text, nil) {
		return WorldOutput{Output: Output{Text: b.Base, Source: "template(refused)", Corrected: true, Err: "content refused", Guard: guard.Check(b.Base, b.Facts, b.Guard)}, Refused: true, WorldErr: "refused"}
	}
	narr, raw, _ := structured.Split(resp.Text)
	out := WorldOutput{Output: l.finish(b, narr)}
	w, repaired, perr := structured.Parse(raw)
	out.Repaired = repaired
	if perr != nil {
		// 重试 1 次：只请求 WORLD 段（JSON 模式）
		out.Retried = true
		retry := []provider.Message{
			{Role: "system", Content: "你是游戏引擎的世界更新器。只输出一个 JSON 对象，不要任何其他文字。\n" + worldRules(tier)},
			{Role: "user", Content: sections + "\n[NARRATION]\n" + narr + "\n[PARSE_ERROR]\n" + perr.Error() + "\n请只输出这段叙事对应的 WORLD JSON。"},
		}
		r2, err2 := l.Provider.Generate(ctx, provider.Request{Messages: retry, Temperature: 0.2, MaxTokens: 800, JSON: true})
		if err2 == nil {
			_, raw2, found := structured.Split(r2.Text)
			if !found {
				raw2 = r2.Text
			}
			w, _, perr = structured.Parse(raw2)
		} else {
			perr = err2
		}
	}
	if perr != nil {
		out.WorldErr = "parse"
		return out
	}
	out.World = w
	return out
}
