// Package simulator 是场外世界模拟 Agent（v0.2.0-rc1）的确定性部分：系统提示词（完整 / 本地精简两种格式）、
// 输出解析与清洗，以及离线规则的选择。调用模型、校验与提交由 orchestrator 负责——
// 世界模拟的提案和叙事、审查一样走同一个校验器与事件日志（来源 world_sim，可撤销）。
package simulator

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// 单次模拟的上限。
const (
	MaxNews           = 3
	MaxChanges        = 4
	MaxCompactChanges = 2
	maxNewsRunes      = 60
)

// System 是世界模拟 Agent 的系统提示词（完整格式）。
const System = `你是文字 RPG 的“世界模拟者”。玩家不在场的这段时间里，世界上的势力、NPC 和事件也在按自己的目标推进。
根据 [TIME]（经过了多久）、[FACTIONS]、[OFFSCREEN]（不在玩家身边的人物）、[TIMELINE]（即将发生的事件）和 [RECENT]（最近的世界变更），
推演这段时间里场外发生的 0–3 件小事，并给出对应的世界变更。要求：
- 只推进与各自目标一致、规模合理的小变化（立场、动向、关系、位置、事件安排）；不要替玩家做决定，不要改动玩家（target 不能是 player）。
- 不要揭示隐藏真相，不要杀死重要人物，不要让重大事件提前结束。
- news 是玩家之后可能听到的传闻（每条一句话，不超过 40 字），不要剧透隐藏信息。
只输出一个 JSON 对象，不要任何其它文字：
{"news":["传闻"],"changes":[{"op":"patch|link|unlink|timeline_patch|timeline_add","target":"实体ID","path":"字段路径","value":新值,"reason":"场外：原因"}]}
人物移动用 path "location"；人物之间的关系用 {"op":"patch","target":"A>B","path":"dims.trust|dims.affection|dims.hostility|dims.fear","value":增量}。
只使用 [ENTITIES] 里出现的 ID；changes 最多 4 项；什么都没发生时输出 {"news":[],"changes":[]}。`

// SystemCompact 是本地小模型使用的精简格式。
const SystemCompact = `你是文字 RPG 的“世界模拟者”。推演玩家不在场时场外发生的 0–2 件小事。
不要改动玩家，不要揭示秘密，不要杀死人物。只输出一个 JSON 对象：
{"news":["一句话传闻"],"edits":[{"id":实体ID,"field":"stance|description|mood","text":"新内容"}],"rel":[{"a":人物ID,"b":人物ID,"dim":"trust|hostility","delta":-10到10}]}
只使用 [ENTITIES] 里的 ID；edits 与 rel 合计最多 2 项；没有变化时输出 {"news":[]}。`

// Output 是一次模拟的提案。
type Output struct {
	News    []string        `json:"news"`
	Changes []change.Change `json:"changes"`
}

// Empty 报告提案是否为空。
func (o Output) Empty() bool { return len(o.News) == 0 && len(o.Changes) == 0 }

// Parse 解析模型输出（容忍前后多余文字与轻微损坏）。compact 时接受精简格式并限制为 2 项。
func Parse(text string, compact bool) (Output, error) {
	raw := strings.TrimSpace(text)
	if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	var in struct {
		News []string `json:"news"`
		structured.World
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		if err2 := json.Unmarshal([]byte(structured.Repair(raw)), &in); err2 != nil {
			return Output{}, fmt.Errorf("世界模拟输出不是有效 JSON：%w", err)
		}
	}
	in.Expand()
	limit := MaxChanges
	if compact {
		limit = MaxCompactChanges
	}
	return Clean(Output{News: in.News, Changes: in.AllChanges()}, limit), nil
}

// Clean 清洗提案：去掉针对玩家的修改与不允许的操作，截断传闻与数量；没有原因的补上“场外”。
func Clean(o Output, limit int) Output {
	var out Output
	for _, n := range o.News {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if utf8.RuneCountInString(n) > maxNewsRunes {
			n = string([]rune(n)[:maxNewsRunes-1]) + "…"
		}
		if !slices.Contains(out.News, n) && len(out.News) < MaxNews {
			out.News = append(out.News, n)
		}
	}
	for _, c := range o.Changes {
		if len(out.Changes) >= limit {
			break
		}
		if c.Target == "" || c.Target == loader.PlayerID || strings.HasPrefix(c.Target, loader.PlayerID+">") || change.IsPlayerPath(c.Target, c.Path) {
			continue
		}
		switch c.Op {
		case change.OpPatch, change.OpLink, change.OpUnlink, change.OpTimelinePatch, change.OpTimelineAdd:
		default:
			continue // 场外模拟不新建 / 不让人物退场（这些交给叙事与玩家）
		}
		if strings.TrimSpace(c.Reason) == "" {
			c.Reason = "场外推进"
		}
		if !strings.HasPrefix(c.Reason, "场外") {
			c.Reason = "场外：" + c.Reason
		}
		c.ID, c.Source, c.Summary = "", "", ""
		out.Changes = append(out.Changes, c)
	}
	return out
}

// Pick 按触发方式选一条离线规则：先筛出触发方式匹配、条件成立、（once 规则）尚未触发过的规则，
// 再按时间段序号轮流选择。没有可用规则时返回 nil。
func Pick(sim loader.Simulation, trigger string, period int64, eval func(expr string) (bool, error), fired func(id string) bool) *loader.SimRule {
	var ok []*loader.SimRule
	for i := range sim.Rules {
		r := &sim.Rules[i]
		if !slices.Contains(r.On, trigger) && (trigger != "wait" || !slices.Contains(r.On, "time")) {
			continue
		}
		if r.Once && fired != nil && fired(r.ID) {
			continue
		}
		if strings.TrimSpace(r.When) != "" {
			if pass, err := eval(r.When); err != nil || !pass {
				continue
			}
		}
		ok = append(ok, r)
	}
	if len(ok) == 0 {
		return nil
	}
	if period < 0 {
		period = -period
	}
	return ok[period%int64(len(ok))]
}

// RuleOutput 把规则转成提案。
func RuleOutput(r *loader.SimRule) Output {
	o := Output{}
	if r.News != "" {
		o.News = []string{r.News}
	}
	for _, c := range r.Changes {
		v, _ := json.Marshal(c.Value)
		reason := c.Reason
		if reason == "" {
			reason = "场外：" + r.News
		}
		o.Changes = append(o.Changes, change.Change{Op: c.Op, Target: c.Target, Kind: c.Kind, Path: c.Path, Value: v, Reason: reason})
	}
	return Clean(o, MaxChanges)
}

// RuleReason 返回规则第一项变更的原因（用于判断 once 规则是否已经触发过）。
func RuleReason(r *loader.SimRule) string {
	o := RuleOutput(r)
	if len(o.Changes) == 0 {
		return ""
	}
	return o.Changes[0].Reason
}
