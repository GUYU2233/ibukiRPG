package freeform

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/checks"
)

// 限制。
const (
	MaxDescriptionRunes = 60
	MaxFactRunes        = 40
	MaxMinutes          = 60
	DefaultMinutes      = 2
	MaxEffects          = 3
	MaxNudge            = 2
)

// 建议效果类型（映射到第 6.1 节白名单事件）。
const (
	EffectNoise        = "noise"              // NoiseGenerated
	EffectAttention    = "attention"          // AttentionChanged
	EffectSceneFact    = "scene_fact"         // SceneFactChanged
	EffectRelationship = "relationship_nudge" // RelationshipNudge
	EffectCondition    = "minor_condition"    // MinorConditionApplied
	EffectNone         = "none"               // NoMechanicalEffect
)

// MinorConditions 是 FreeformAction 允许施加的轻微状态。
var MinorConditions = []string{"tired", "soaked", "embarrassed", "inspired"}

// Sanitize 校验并夹紧 AI / 规则解析器提出的 FreeformAction。
// AI 的技能、难度和效果都只是建议，这里是 Core 的最终裁决（第 6 节）。
func Sanitize(ff command.Freeform, p *loader.Package, s *state.State) command.Freeform {
	out := command.Freeform{Reasonability: ff.Reasonability}
	out.Description = truncate(oneLine(ff.Description), MaxDescriptionRunes)
	if out.Description == "" {
		out.Description = "做了点什么"
	}
	present := s.NPCsAt(p, s.Player.Location)
	for _, t := range ff.Targets {
		if slices.Contains(present, t) && !slices.Contains(out.Targets, t) {
			out.Targets = append(out.Targets, t)
		}
	}
	if ff.Check != nil {
		if _, ok := p.SkillByID(ff.Check.Skill); ok {
			dc := ff.Check.Difficulty
			if dc == 0 {
				dc = 12
			}
			out.Check = &command.SuggestedCheck{Skill: ff.Check.Skill, Difficulty: checks.ClampDC(dc)}
		}
	}
	out.EstimatedMinutes = ff.EstimatedMinutes
	if out.EstimatedMinutes <= 0 {
		out.EstimatedMinutes = DefaultMinutes
	}
	if out.EstimatedMinutes > MaxMinutes {
		out.EstimatedMinutes = MaxMinutes
	}
	for _, t := range ff.Tags {
		t = truncate(oneLine(t), 12)
		if t != "" && len(out.Tags) < 5 && !slices.Contains(out.Tags, t) {
			out.Tags = append(out.Tags, t)
		}
	}
	seen := map[string]bool{}
	for _, e := range ff.Effects {
		if len(out.Effects) >= MaxEffects {
			break
		}
		key := e.Type + "|" + e.Target
		if seen[key] {
			continue
		}
		switch e.Type {
		case EffectNoise:
			e.Value = clamp(e.Value, 1, 10, 4)
			e.Target, e.Text = "", ""
		case EffectAttention:
			e.Text = truncate(oneLine(e.Text), MaxFactRunes)
			if !slices.Contains(present, e.Target) {
				e.Target = ""
			}
			e.Value = 0
		case EffectSceneFact:
			e.Text = truncate(oneLine(e.Text), MaxFactRunes)
			if e.Text == "" {
				continue
			}
			e.Target, e.Value = "", 0
		case EffectRelationship:
			if !slices.Contains(present, e.Target) || e.Value == 0 {
				continue
			}
			e.Value = clamp(e.Value, -MaxNudge, MaxNudge, 0)
			e.Text = ""
		case EffectCondition:
			e.Text = strings.TrimSpace(e.Text)
			if !slices.Contains(MinorConditions, e.Text) {
				continue
			}
			e.Target, e.Value = "", 0
		default:
			continue // 白名单外的效果（死亡、任务完成、传奇物品……）一律丢弃
		}
		switch e.When {
		case "success", "failure", "always":
		default:
			e.When = ""
		}
		seen[key] = true
		out.Effects = append(out.Effects, e)
	}
	return out
}

func clamp(v, lo, hi, def int) int {
	if v == 0 && def != 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func oneLine(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// Applies 判断一个（已校验的）建议效果在给定检定结果下是否生效。
// 默认：噪音与注意力总是发生；负向关系变化（冒犯）总是发生；正向变化、场景事实与状态只在成功时发生；
// “窘迫”只在失败时发生。
func Applies(e command.ProposedEffect, success bool) bool {
	switch e.When {
	case "always":
		return true
	case "success":
		return success
	case "failure":
		return !success
	}
	switch e.Type {
	case EffectNoise, EffectAttention:
		return true
	case EffectRelationship:
		return success || e.Value < 0
	case EffectCondition:
		if e.Text == "embarrassed" {
			return !success
		}
	}
	return success
}
