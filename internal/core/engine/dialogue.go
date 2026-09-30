package engine

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
)

// DialogueVars 是台词条件的变量：通用变量（target = 说话者）+ npc（说话者的关系与记忆）。
func DialogueVars(p *loader.Package, s *state.State, npc string) expression.Vars {
	v := Vars(p, s, npc, "")
	v["npc"] = NPCVars(p, s, npc)
	return v
}

// ChooseLine 从 NPC 的台词池中挑选下一句台词（确定性，不消耗随机数）：
//
//  1. 过滤：when 条件成立，且 once 台词没说过；
//  2. 优先没说过的台词：优先级高者优先，同优先级按声明顺序；
//  3. 如果能说的都说过了：挑优先级最高、最久没说的一条，并标记 repeat（叙事会据此“提起说过的话”）。
//
// 没有任何可说的台词时返回 nil。
func ChooseLine(ev *expression.Evaluator, p *loader.Package, s *state.State, npc string) (*loader.DialogueLine, bool, error) {
	c, ok := p.Characters[npc]
	n := s.NPCs[npc]
	if !ok || n == nil {
		return nil, false, nil
	}
	vars := DialogueVars(p, s, npc)
	var fresh, used *loader.DialogueLine
	usedTurn := 0
	for i := range c.Dialogue.Lines {
		l := &c.Dialogue.Lines[i]
		if l.When != "" {
			ok, err := ev.EvalBool(l.When, vars)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				continue
			}
		}
		turn, said := n.Said[l.ID]
		if said && l.Once {
			continue
		}
		if !said {
			if fresh == nil || l.Priority > fresh.Priority {
				fresh = l
			}
			continue
		}
		if used == nil || l.Priority > used.Priority || (l.Priority == used.Priority && turn < usedTurn) {
			used, usedTurn = l, turn
		}
	}
	if fresh != nil {
		return fresh, false, nil
	}
	return used, used != nil, nil
}

// LineMemo 返回台词写入对话记忆的摘要：memory > 话题名 > 台词开头。
func LineMemo(c *loader.Character, l *loader.DialogueLine, player string) string {
	memo := l.Memory
	if memo == "" && l.Topic != "" {
		if name, ok := c.Dialogue.Topics[l.Topic]; ok {
			memo = "聊到了" + name
		}
	}
	if memo == "" {
		memo = l.Text
		if i := strings.Index(memo, "“"); i >= 0 {
			memo = memo[i:]
		}
		memo = strings.Trim(memo, "“”")
		if utf8.RuneCountInString(memo) > 24 {
			memo = string([]rune(memo)[:24]) + "……"
		}
	}
	return FillLine(memo, c, player)
}

// FillLine 替换台词中与说话者相关的占位符。
func FillLine(text string, c *loader.Character, player string) string {
	return strings.NewReplacer("{player}", player, "{name}", c.Name()).Replace(text)
}

// converse 为交谈动作挑选台词并记录 DialogueOccurred（以及可能的情节记忆）。
func (w *work) converse(npc string) error {
	p := w.pkg()
	c := p.Characters[npc]
	line, repeat, err := ChooseLine(w.eng.Eval, p, w.s, npc)
	if err != nil {
		return err
	}
	if line == nil {
		return w.emit(event.DialogueOccurred, event.Data{Target: npc, Text: "随便聊了几句", Location: w.s.Player.Location})
	}
	if err := w.emit(event.DialogueOccurred, event.Data{
		Target: npc, Step: line.ID, Key: line.Topic, Text: LineMemo(c, line, w.s.Player.Name),
		Repeat: repeat, Location: w.s.Player.Location,
	}); err != nil {
		return err
	}
	if !repeat {
		for _, rv := range line.Reveals {
			from, to, ok := strings.Cut(rv, ">")
			if !ok {
				continue
			}
			if err := w.emit(event.RelationRevealed, event.Data{Actor: from, Target: to, Values: w.s.Edge(from, to), Source: "dialogue", Witness: npc}); err != nil {
				return err
			}
		}
		if len(line.Relation) > 0 {
			vals := map[string]any{"from": npc, "to": loader.PlayerID, "reason": "交谈：" + LineMemo(c, line, w.s.Player.Name)}
			for k, v := range line.Relation {
				vals[k] = v
			}
			if _, err := w.applyRPGEffect("relation", npc, func(k string) string { return fmt.Sprint(vals[k]) }, func(k string, def int) (int, error) {
				if v, ok := vals[k].(int); ok {
					return v, nil
				}
				return def, nil
			}, vals); err != nil {
				return err
			}
		}
	}
	if line.Remember != "" && !w.s.NPCs[npc].HasEpisode("dialogue:"+line.ID) {
		return w.remember(npc, "dialogue:"+line.ID, FillLine(line.Remember, c, w.s.Player.Name), "dialogue", 2)
	}
	return nil
}
