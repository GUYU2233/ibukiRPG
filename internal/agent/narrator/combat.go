package narrator

import (
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// UnitName 返回战斗单位的显示名（从行动前 / 后的战斗状态中查找）。
func UnitName(before, after *state.State, id string) string {
	for _, s := range []*state.State{after, before} {
		if s != nil && s.RPG != nil && s.RPG.Combat != nil {
			if u := s.RPG.Combat.Unit(id); u != nil {
				if u.ID == loader.PlayerID {
					return "你"
				}
				return u.Name
			}
		}
	}
	if id == loader.PlayerID {
		return "你"
	}
	return id
}

// rpgParts 生成战斗 / 成长 / 主线相关的模板叙事段落。
func rpgParts(p *loader.Package, before, after *state.State, cmd command.Command, res *engine.Result) []string {
	var parts []string
	var blow []string
	name := func(id string) string { return UnitName(before, after, id) }
	for _, e := range res.Events {
		d := e.Data
		switch e.Type {
		case event.CombatStarted:
			t := "战斗开始：" + d.Title + "。"
			if d.Text != "" {
				t = d.Text
			}
			parts = append(parts, t)
		case event.CombatActed:
			if len(blow) >= 3 {
				continue
			}
			blow = append(blow, ActText(p, name, d))
		case event.MechEngaged:
			blow = append(blow, fmt.Sprintf("%s的机体轰然苏醒——%s！", name(d.Target), d.Units[0].Name))
		case event.UnitOverheated:
			blow = append(blow, name(d.Target)+"的机体越过了热量红线，泄压阀尖啸着强制冷却。")
		case event.UnitDefeated:
			if d.Target != loader.PlayerID {
				blow = append(blow, name(d.Target)+"倒下了。")
			}
		case event.CombatEnded:
			parts = append(parts, strings.Join(blow, ""))
			blow = nil
			parts = append(parts, EndText(p, d))
		case event.LevelUp:
			parts = append(parts, fmt.Sprintf("你感到浑身的力气又涨了一截——等级提升到了 %d。", d.Delta))
		case event.ItemEquipped:
			parts = append(parts, "你换上了"+p.EntityName(d.Item)+"。")
		case event.ItemUnequipped:
			parts = append(parts, "你卸下了"+p.EntityName(d.Item)+"。")
		case event.SkillLearned:
			if sk, ok := p.Combat.Skills[d.Skill]; ok {
				parts = append(parts, "你掌握了新的技巧："+sk.Name+"。")
			}
		case event.AttributeAllocated:
			parts = append(parts, fmt.Sprintf("你的%s提升了。", p.Rules.Attributes[d.Key]))
		case event.PlayerVitals:
			if d.Reason != "" && d.Reason != "level_up" {
				if it, ok := p.Items[d.Reason]; ok {
					parts = append(parts, "你用了"+it.Name+"，感觉好多了。")
				}
			}
		case event.MainlineNudged:
			parts = append(parts, d.Text)
		case event.MainlineModeChanged:
			switch d.To {
			case state.ModeFree:
				parts = append(parts, "你决定不再沿着既定的轨迹前进。命运的齿轮换了一个方向转动——接下来的故事，由你和这个世界一起写。")
			case state.ModeSandbox:
				parts = append(parts, "你决定按自己的方式活下去。镇上总有人需要帮手——新的委托会一件件找上门来。")
			}
		case event.MainlineNodeCanonized:
			if d.Node != nil {
				parts = append(parts, "新的目标浮现："+d.Node.Title+"——"+d.Node.Objective)
			}
		case event.MainlineNodeCompleted:
			parts = append(parts, "目标达成："+d.Title+"。")
		case event.MainlineAnchorReached:
			parts = append(parts, "【主线】"+d.Title+" 完成。")
		case event.CharacterDied:
			parts = append(parts, p.EntityName(d.Target)+"永远地离开了。")
		}
	}
	if len(blow) > 0 {
		parts = append(parts, strings.Join(blow, ""))
	}
	_ = cmd
	return parts
}

// ActText 把一次战斗行动翻译成一句话。
func ActText(p *loader.Package, name func(string) string, d event.Data) string {
	who := name(d.Actor)
	switch d.Action {
	case "stunned":
		return who + "站立不稳，错过了出手的机会。"
	case "defend":
		return who + "摆出了防御姿态。"
	case "flee":
		if d.Success {
			return who + "抓住空隙撤出了战斗。"
		}
		return who + "想要撤退，却被死死缠住。"
	case "mech":
		return ""
	case "eject":
		return who + "打开舱门，跳出了机体。"
	case "wait":
		return who + "犹豫了一下。"
	case "item":
		return who + "用了" + p.EntityName(d.Item) + "。"
	}
	verb := "发动了攻击"
	if sk, ok := p.Combat.Skills[d.Skill]; ok {
		verb = "使出" + sk.Name
		if sk.Verb != "" {
			verb = sk.Verb
		}
	}
	if d.Target == "" || strings.Contains(d.Target, ",") || d.Delta == 0 && d.Success {
		return who + verb + "。"
	}
	tgt := name(d.Target)
	if !d.Success {
		return fmt.Sprintf("%s%s，被%s躲开了。", who, verb, tgt)
	}
	if d.Critical {
		return fmt.Sprintf("%s%s，正中%s的要害！", who, verb, tgt)
	}
	return fmt.Sprintf("%s%s，击中了%s。", who, verb, tgt)
}

// EndText 返回战斗结束的叙事。
func EndText(p *loader.Package, d event.Data) string {
	enc := p.Combat.Encounters[d.Story]
	switch d.Outcome {
	case "victory":
		if enc != nil && enc.VictoryText != "" {
			return enc.VictoryText
		}
		return "最后一个对手倒下了。你喘着粗气，握紧了手里的武器——这一战，你赢了。"
	case "defeat":
		if enc != nil && enc.Defeat.Text != "" {
			return enc.Defeat.Text
		}
		switch d.Reason {
		case "captured":
			return "眼前一黑。等你醒来时，手腕上已经套上了冰冷的镣铐。"
		case "rescued":
			return "你倒在地上，意识模糊之际，有人把你拖离了战场。"
		}
		return "你伤得很重，被迫退出了战斗。"
	}
	if d.Reason == "stalemate" {
		return "双方都已精疲力竭，战斗不了了之。"
	}
	return "你脱离了战斗。"
}
