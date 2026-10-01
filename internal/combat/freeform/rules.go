package freeform

import "strings"

func containsAny(text string, words ...string) bool {
	for _, w := range words {
		if w != "" && strings.Contains(text, w) {
			return true
		}
	}
	return false
}

func names(id, name string, aliases []string) []string {
	out := append([]string{name}, aliases...)
	if i := strings.LastIndexAny(id, "/:"); i >= 0 {
		out = append(out, id[i+1:])
	}
	return out
}

// RulesIntent 是离线（无 AI）的关键词意图解析：当 AI 不可用、拒答或输出无法修复时兜底。
// 只识别目标 / 部位 / 技能 / 战技 / 修正标签 / 武器 / 防御 / 逃跑等关键词，从不发明数值。
func RulesIntent(text string, ctx Context) Intent {
	in := Intent{Raw: text, Source: "rules"}
	t := strings.TrimSpace(text)
	for _, w := range append(append([]string{}, BuiltinAbsurd...), ctx.AbsurdWords...) {
		if w != "" && strings.Contains(t, w) {
			in.Absurd = w
		}
	}
	act := Action{Kind: KindAttack}
	switch {
	case containsAny(t, "逃跑", "撤退", "逃走", "脱离战斗"):
		act.Kind = KindFlee
	case containsAny(t, "防御", "格挡", "举盾", "守住", "护住"):
		act.Kind = KindDefend
	case containsAny(t, "劝降", "谈判", "喊话", "说服"):
		act.Kind = KindTalk
	}
	matched := act.Kind != KindAttack || containsAny(t, attackVerbs...)
	var tgt *Target
	for i := range ctx.Targets {
		x := &ctx.Targets[i]
		if containsAny(t, names(x.ID, x.Name, x.Aliases)...) {
			tgt, matched = x, true
			break
		}
	}
	if tgt == nil && len(ctx.Targets) > 0 {
		tgt = &ctx.Targets[0]
	}
	if tgt != nil && act.Kind != KindFlee && act.Kind != KindDefend {
		act.Target = tgt.ID
		for _, p := range tgt.Parts {
			if containsAny(t, p.Name, p.ID) {
				act.Part = p.ID
			}
		}
	}
	if act.Kind == KindAttack {
		for _, s := range ctx.Skills {
			if containsAny(t, names(s.ID, s.Name, s.Aliases)...) {
				act.Kind, act.Skill, matched = KindSkill, s.ID, true
				break
			}
		}
	}
	if act.Kind == KindAttack {
		for _, m := range ctx.Maneuvers {
			if containsAny(t, append([]string{m.Name}, m.Keywords...)...) {
				act.Kind, act.Maneuver, matched = KindManeuver, m.ID, true
				break
			}
		}
	}
	for _, m := range ctx.Means {
		if containsAny(t, names(m.ID, m.Name, m.Aliases)...) {
			act.Means, matched = m.ID, true
		}
	}
	if containsAny(t, "绕到", "冲到", "靠近", "后退", "拉开距离", "躲到") {
		in.Actions = append(in.Actions, Action{Kind: KindMove, To: "近处"})
	}
	in.Actions = append(in.Actions, act)
	for _, tg := range ctx.Tags {
		if containsAny(t, tg.Label, tg.ID) {
			in.Circumstances = append(in.Circumstances, Circumstance{Tag: tg.ID, Why: "玩家描述"})
		}
	}
	if !matched {
		// 没有任何战斗相关的关键词：交给调用方拒绝并给出战斗选项
		in.SelfCheck = Unmatched
	}
	return in
}

// Unmatched 标记规则解析器没有识别出任何战斗动作。
const Unmatched = "unmatched"

var attackVerbs = []string{"打", "砍", "刺", "砸", "攻击", "踢", "扑", "射", "捅", "挥", "劈", "撞", "冲", "揍", "拳", "斩", "戳", "敲"}
