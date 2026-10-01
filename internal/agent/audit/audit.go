// Package audit 是一致性自审（架构 V0.3 第 12.3 节）的确定性部分：解析审查 Agent 的输出，
// 并按第 19 节决定 4 把修复分成三类：
//
//   - 设定 / 文字类修复：自动提交（审查来源，可单项撤销）；
//   - 机械状态修复（物品、金钱、经验、生死）：只作为建议，玩家确认后才提交；
//   - 幻觉类发现（fix 为空）：不改世界，在下一次叙事里注入 [CORRECTION] 让叙事自然更正。
//
// 调用模型、提交事件由 orchestrator 负责。
package audit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// MaxFixes 是一次审查最多处理的修复数（自动 + 建议）。
const MaxFixes = 5

// 发现类型。
const (
	KindOmission      = "omission"
	KindHallucination = "hallucination"
	KindContradiction = "contradiction"
)

// System 是审查 Agent 的系统提示词。
const System = `你是文字 RPG 的“一致性审查员”。对照下面的叙事原文、世界变更记录与实体清单，找出三类问题：
- omission：叙事里发生了，但世界状态没有记账（例如 NPC 把物品交给了玩家，背包里却没有；地点被烧毁，描述没变）。
- hallucination：叙事编造了与设定 / 已有变更冲突的内容。优先不改世界，fix 留空，在 note 里写一句给叙事者的更正提示。
- contradiction：两条世界变更互相矛盾，修正较新的一条。
只输出一个 JSON 对象，不要任何其它文字：
{"findings":[{"kind":"omission|hallucination|contradiction","turn":回合号,"text":"问题描述","fix":[变更...],"note":"给叙事者的更正提示（可空）"}]}
变更格式与 WORLD 段相同：{"op":"patch|create|retire|restore|link|unlink","target":"实体ID 或 player","path":"字段路径","value":新值,"reason":"补记：……"}。
只使用 [ENTITIES] 里出现的 ID；新建实体使用给定命名空间。全部修复加起来最多 5 项；没有问题时输出 {"findings":[]}。`

// Finding 是审查 Agent 的一条发现。
type Finding struct {
	Kind string          `json:"kind"`
	Turn int             `json:"turn"`
	Text string          `json:"text"`
	Fix  []change.Change `json:"fix"`
	Note string          `json:"note"`
}

// Parse 解析审查输出（容忍前后多余文字；JSON 损坏时做一次本地修复）。
func Parse(text string) ([]Finding, error) {
	raw := strings.TrimSpace(text)
	if i, j := strings.Index(raw, "{"), strings.LastIndex(raw, "}"); i >= 0 && j > i {
		raw = raw[i : j+1]
	}
	var out struct {
		Findings []Finding `json:"findings"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		if err2 := json.Unmarshal([]byte(structured.Repair(raw)), &out); err2 != nil {
			return nil, fmt.Errorf("审查输出不是有效 JSON：%w", err)
		}
	}
	return out.Findings, nil
}

// Plan 是一次审查的处理方案。
type Plan struct {
	Findings    []Finding
	Autos       []change.Change
	Suggestions []change.Change
	Corrections []string
}

// Classify 把发现分为自动修复 / 建议 / 更正提示。kindOf 返回实体类型（判断“生死”这类机械状态）。
func Classify(findings []Finding, kindOf func(string) string) Plan {
	var p Plan
	budget := MaxFixes
	for _, f := range findings {
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		p.Findings = append(p.Findings, f)
		if f.Kind == KindHallucination && len(f.Fix) == 0 {
			note := f.Note
			if note == "" {
				note = f.Text
			}
			p.Corrections = append(p.Corrections, note)
			continue
		}
		for _, c := range f.Fix {
			if budget == 0 {
				break
			}
			budget--
			if c.Reason == "" {
				c.Reason = "审查补记：" + f.Text
			}
			if c.IsMechanical(kindOf) {
				p.Suggestions = append(p.Suggestions, c)
			} else {
				p.Autos = append(p.Autos, c)
			}
		}
	}
	return p
}
