package resolver

import (
	"sort"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// 战斗中的关键词（离线规则解析；AI 在线时也用它，保证战斗输入确定、快速）。
var (
	kwEject  = []string{"脱离机甲", "弹出", "脱离", "下机", "解除机甲"}
	kwMech   = []string{"启动", "驾驶", "机甲", "炽天使", "上机", "变身"}
	kwFlee   = []string{"逃跑", "撤退", "逃走", "跑路", "逃", "撤"}
	kwDefend = []string{"防御", "格挡", "防守", "招架", "守住", "护住"}
	kwAttack = []string{"攻击", "普攻", "砍", "刺", "打", "揍", "射击", "开枪", "开火", "斩", "劈", "踢", "冲上去", "杀"}
)

func containsAny(s string, kws []string) bool {
	for _, k := range kws {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// matchNames 在文本中找最长匹配的名字，返回 key。
func matchNames(text string, names map[string][]string) string {
	best, bestLen := "", 0
	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, n := range names[k] {
			if n != "" && strings.Contains(text, n) && len([]rune(n)) > bestLen {
				best, bestLen = k, len([]rune(n))
			}
		}
	}
	return best
}

// ResolveCombat 把战斗中的自然语言输入解析为战斗命令。
// ok=false 时 reason 给出提示，options 是可点选的战斗动作。
func ResolveCombat(text string, p *loader.Package, s *state.State) (command.Command, bool, string, []Option) {
	c := s.RPG.Combat
	me := c.Unit(loader.PlayerID)
	text = strings.TrimSpace(text)
	mk := func(action string) command.Command {
		return command.Command{Kind: command.KindCombat, Action: action, Input: text, Source: "resolver:combat"}
	}
	// 目标：敌人名（完整名或基础名）/ 队友名
	targets := map[string][]string{}
	for _, u := range c.Units {
		if u.Down || u.ID == loader.PlayerID {
			continue
		}
		names := []string{u.Name}
		if base := strings.TrimRight(u.Name, "甲乙丙丁戊己庚辛"); base != u.Name && base != "" {
			names = append(names, base)
		}
		if u.Side == engine.SideEnemy {
			if d, ok := p.Combat.Enemies[u.Ref]; ok {
				names = append(names, d.Aliases...)
			}
		}
		targets[u.ID] = names
	}
	target := matchNames(text, targets)
	withTarget := func(cmd command.Command) command.Command {
		cmd.Target = target
		return cmd
	}
	switch {
	case me != nil && me.Mech != nil && containsAny(text, kwEject):
		return mk("eject"), true, "", nil
	case me != nil && me.Mech == nil && c.AllowMech && containsAny(text, kwMech):
		return mk("mech"), true, "", nil
	case containsAny(text, kwFlee):
		return mk("flee"), true, "", nil
	}
	// 物品
	items := map[string][]string{}
	for id, n := range s.Player.Inventory {
		it := p.Items[id]
		if n <= 0 || it == nil || it.Combat == nil {
			continue
		}
		items[id] = append([]string{it.Name}, it.Aliases...)
	}
	if id := matchNames(text, items); id != "" {
		cmd := withTarget(mk("item"))
		cmd.Item = id
		return cmd, true, "", nil
	}
	// 技能
	skills := map[string][]string{}
	if me != nil {
		for _, id := range engine.UnitSkills(p, me) {
			if sk, ok := p.Combat.Skills[id]; ok {
				skills[id] = append([]string{sk.Name}, sk.Aliases...)
			}
		}
	}
	if id := matchNames(text, skills); id != "" {
		cmd := withTarget(mk("skill"))
		cmd.Skill = id
		return cmd, true, "", nil
	}
	if containsAny(text, kwDefend) {
		return mk("defend"), true, "", nil
	}
	if containsAny(text, kwAttack) || target != "" {
		return withTarget(mk("attack")), true, "", nil
	}
	opts := []Option{
		{Label: "攻击", Command: mk("attack")},
		{Label: "防御", Command: mk("defend")},
	}
	if !c.NoFlee {
		opts = append(opts, Option{Label: "逃跑", Command: mk("flee")})
	}
	return command.Command{}, false, "正在战斗中：可以攻击、使用技能或物品、防御" + map[bool]string{true: "", false: "或逃跑"}[c.NoFlee] + "。例如“用蒸汽锤砸向发条鼠”。", opts
}
