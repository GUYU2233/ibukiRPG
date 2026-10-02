package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/power"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
)

// ---------- 游戏内函数调用（v0.2.0-rc1） ----------
//
// 支持函数调用的模型（云端、未关闭工具）在可能改变世界的回合里，先用写入工具（world.propose_change /
// entity.generate / rules.power_budget）提交修改：每次调用都用同一校验器演练、立即告诉模型哪些被拒绝以及原因；
// 通过的提案在回合提交 B 时和 WORLD 段一起写入事件日志。不支持函数调用的模型（本地模型、关闭了工具的服务）
// 继续使用 WORLD 段 JSON。

// newWriter 返回在 st 之上演练提案的写入后端。
func newWriter(g *game, st *state.State, source, tier string) *tools.Writer {
	w := &tools.Writer{Namespace: validate.GenNamespace(g.pkg), PowerRef: validate.PowerRef(g.pkg), Max: min(validate.MaxChanges(tier), 6)}
	w.Dry = func(cs []change.Change) (tools.DryResult, error) {
		res, after, err := g.eng.Execute(st, command.Command{ID: "dry-run", Kind: command.KindWorldChange, Changes: cs, Source: source, Tier: tier})
		if err != nil {
			return tools.DryResult{}, err
		}
		out := tools.DryResult{}
		rejected := make([]bool, len(cs))
		for _, r := range res.Rejects {
			if r.Kind != "" && r.Kind != "change" {
				continue
			}
			out.Rejected = append(out.Rejected, strings.TrimSpace(fmt.Sprintf("%s %s：%s", r.Target, r.Path, r.Reason)))
			for i, c := range cs {
				raw, _ := json.Marshal(c)
				if !rejected[i] && ((len(r.Raw) > 0 && bytes.Equal(bytes.TrimSpace(r.Raw), raw)) || (c.Op == r.Op && c.Target == r.Target && c.Path == r.Path)) {
					rejected[i] = true
					break
				}
			}
		}
		if !res.Accepted {
			if res.Reason != "" {
				out.Rejected = append(out.Rejected, res.Reason)
			}
			return out, nil
		}
		for i, c := range cs {
			if !rejected[i] {
				out.Accepted = append(out.Accepted, c)
			}
		}
		for _, e := range res.Events {
			if e.Type == event.WorldChangeApplied && e.Data.Change != nil {
				out.Summary = append(out.Summary, e.Data.Change.Summary)
			}
		}
		if res.Impact != nil {
			out.Impact = res.Impact.Score
		}
		_ = after
		return out, nil
	}
	return w
}

// worldToolCues 是“这回合可能改变世界”的输入线索（自由推演之外，打造 / 招募 / 建造之类的行动）。
var worldToolCues = []string{"打造", "锻造", "制作", "造一", "做一", "改装", "组装", "发明", "建造", "盖一", "召集", "招募", "雇佣", "收买", "买下", "烧掉", "炸掉", "拆掉", "毁掉", "创建", "生成"}

// wantsWorldTools 报告本回合是否先跑写入工具阶段。
func (s *Session) wantsWorldTools(p provider.Provider, tier string, cmd command.Command, input string) bool {
	if s.router.Settings().NoWorldTools || tier != router.TierFull {
		return false
	}
	tc, ok := p.(provider.ToolCaller)
	if !ok || !tc.SupportsTools() {
		return false
	}
	if cmd.Kind == command.KindFreeform {
		return true
	}
	for _, c := range worldToolCues {
		if strings.Contains(input, c) {
			return true
		}
	}
	return false
}

const worldToolSystem = `你是文字 RPG 的“世界编辑”。下面是本回合已经裁定的结果。判断这回合是否改变了世界设定（新出现的人物 / 物品 / 卡片、
地点或势力的变化、人物移动或关系变化、玩家获得的东西）：
- 需要修改时调用 world_propose_change（格式同 WORLD 段的 changes）或 entity_generate（新卡片，数值按强度预算自动缩放；不确定时先调用 rules_power_budget）。
- 工具会告诉你哪些修改被拒绝以及原因；可以按原因调整后再提交一次，或者放弃。
- 只提交 [RESOLVED_EVENTS] 与玩家输入里确实发生的事；不要编造，不要揭示隐藏真相。
- 完成后（或不需要修改时）只回答一个字：好。不要写叙事。`

// worldToolPhase 运行写入工具阶段，返回给叙事看的 [TOOL_CHANGES] 段（没有提案时为空）。
func (s *Session) worldToolPhase(ctx context.Context, p provider.Provider, env *tools.Env, b narrator.Brief, sections string) string {
	msgs := narrator.BuildMessages(b)
	msgs[0].Content = worldToolSystem + "\n" + power.Table(env.Writer.PowerRef)
	msgs[1].Content = sections + "\n" + msgs[1].Content
	cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	(&tools.Loop{Provider: p, Env: env, Scope: tools.Player().WithWrite(), Temperature: 0.3,
		Budget: tools.Budget{MaxIters: 3, MaxCalls: 5, MaxResultRunes: 4000, MaxTokens: 700}}).Run(cctx, msgs, "")
	cs := env.Writer.Proposed()
	if len(cs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[TOOL_CHANGES]（本回合已经通过工具提交并通过校验的世界修改；叙事要与之一致，WORLD 段不要重复写这些 changes）\n")
	for _, c := range cs {
		fmt.Fprintf(&sb, "- %s %s %s：%s\n", c.Op, c.Target, c.Path, c.Reason)
	}
	return sb.String()
}

// mergeToolChanges 把工具提交的提案并入 WORLD 段（去掉 WORLD 里重复的同一修改）。
func mergeToolChanges(wo *narrator.WorldOutput, extra []change.Change) {
	if len(extra) == 0 || wo.Refused {
		return
	}
	if wo.World == nil {
		wo.World = &structured.World{V: 1}
		wo.WorldErr = ""
	}
	key := func(c change.Change) string { return c.Op + "|" + c.Target + "|" + c.Path }
	seen := map[string]bool{}
	for _, c := range extra {
		seen[key(c)] = true
	}
	var keep []change.Change
	for _, c := range wo.World.Changes {
		if !seen[key(c)] {
			keep = append(keep, c)
		}
	}
	wo.World.Changes = append(append([]change.Change{}, extra...), keep...)
}
