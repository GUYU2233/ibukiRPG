package main

import (
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
)

const rpgHelp = `
数值 RPG（带战斗内容的故事包）：
  /fight [遭遇]  开始遭遇战（无参数时列出可用遭遇）
  /attack [目标]  普通攻击      /skill <技能> [目标]  使用技能
  /item <物品> [目标]  战斗道具  /defend  防御      /flee  逃跑
  /mech   启动机甲形态          /eject   脱离机甲   /combat  战斗面板
  /grow   成长面板（等级/属性/装备/技能）
  /alloc <属性>  分配属性点     /learn <技能>  修习技能
  /equip <物品>  装备           /unequip <槽位>  卸下
  /use <物品>    战斗外使用道具
  /mechs  机械甲胄卡列表         /mechcard <ID或名称>  查看机甲卡
  /install <部件> [槽位]  安装改装件 / 挂载武器   /uninstall <槽位>  卸下
  /codex [关键词]  图鉴          /card <ID或名称>  查看介绍卡
  /rel [人物]  关系网            /cards  角色卡
  /main   主线状态              /mode return|free  回到主线 / 进入自由推演
  /sens relaxed|standard|strict  主线敏感度`

// rpgCommand 处理数值 RPG 相关命令；返回 false 表示不是这类命令。
func (c *client) rpgCommand(cmd, arg string) bool {
	args := strings.Fields(arg)
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	rest := ""
	if len(args) > 1 {
		rest = strings.Join(args[1:], " ")
	}
	switch cmd {
	case "/fight":
		c.fight(arg)
	case "/attack", "/a":
		c.combatAct("attack", "", "", c.unitID(arg))
	case "/skill":
		c.combatAct("skill", c.lookupAction("skills", first), "", c.unitID(rest))
	case "/item":
		c.combatAct("item", "", c.lookupAction("items", first), c.unitID(rest))
	case "/defend", "/d":
		c.combatAct("defend", "", "", "")
	case "/flee":
		c.combatAct("flee", "", "", "")
	case "/mech":
		c.combatAct("mech", "", "", "")
	case "/eject":
		c.combatAct("eject", "", "", "")
	case "/combat":
		c.showCombat()
	case "/grow":
		c.showGrowth()
	case "/alloc":
		c.manage("allocate", c.attrID(first), "", "")
	case "/learn":
		c.manage("learn", "", "", c.skillID(first))
	case "/equip":
		c.manage("equip", "", c.itemID(first), "")
	case "/unequip":
		c.manage("unequip", c.slotID(first), "", "")
	case "/use":
		c.manage("use", "", c.itemID(first), "")
	case "/mechs":
		c.showMechs()
	case "/mechcard":
		c.showMech(c.mechID(arg))
	case "/install":
		c.installPart(first, rest)
	case "/uninstall":
		c.uninstallPart(first)
	case "/codex":
		c.showCodex(arg)
	case "/card":
		c.showCard(c.cardID(arg))
	case "/rel":
		c.showRelations(arg)
	case "/cards":
		c.showCards()
	case "/main":
		c.showMainline()
	case "/mode":
		switch first {
		case "return", "back", "回到主线":
			c.quick(dto.QuickActionV1{Kind: "mainline", Action: "return", Label: "回到主线"})
		case "free", "自由推演":
			c.quick(dto.QuickActionV1{Kind: "mainline", Action: "free", Label: "进入自由推演"})
		default:
			c.printf("用法：/mode return|free\n")
		}
	case "/sens":
		if first == "" {
			first = "standard"
		}
		c.manage("thresholds", first, "", "")
	default:
		return false
	}
	return true
}

func (c *client) scene() (dto.SceneV1, bool) {
	var sc dto.SceneV1
	if err := c.call("get_scene", "", nil, &sc); err != nil {
		c.printf("%v\n", err)
		return sc, false
	}
	return sc, true
}

func (c *client) fight(arg string) {
	if arg == "" {
		var sugg []dto.SuggestionV1
		_ = c.call("get_suggestions", "", nil, &sugg)
		n := 0
		for _, s := range sugg {
			if s.Action.Kind == "combat" && s.Action.Action == "start" {
				n++
				c.printf("  %s  (/fight %s)\n", s.Label, s.Action.Target)
			}
		}
		if n == 0 {
			c.printf("这里没有可以主动发起的战斗。\n")
		}
		return
	}
	var sugg []dto.SuggestionV1
	_ = c.call("get_suggestions", "", nil, &sugg)
	for _, s := range sugg {
		if s.Action.Kind == "combat" && s.Action.Action == "start" && (strings.Contains(s.Action.Target, arg) || strings.Contains(s.Label, arg)) {
			arg = s.Action.Target
			break
		}
	}
	c.quick(dto.QuickActionV1{Kind: "combat", Action: "start", Target: arg, Label: "开战"})
}

func (c *client) combatAct(action, skill, item, target string) {
	label := map[string]string{"attack": "攻击", "skill": "技能", "item": "道具", "defend": "防御", "flee": "逃跑", "mech": "启动机甲", "eject": "脱离机甲"}[action]
	c.quick(dto.QuickActionV1{Kind: "combat", Action: action, Skill: skill, Item: item, Target: target, Label: label})
}

func (c *client) manage(action, target, item, skill string) {
	c.quick(dto.QuickActionV1{Kind: "manage", Action: action, Target: target, Item: item, Skill: skill, Label: action})
}

// lookupAction 在战斗面板的技能 / 道具里按 ID 或名称查找。
func (c *client) lookupAction(kind, key string) string {
	var cb *dto.CombatV1
	if err := c.call("get_combat", "", nil, &cb); err != nil || cb == nil || key == "" {
		return key
	}
	list := cb.Skills
	if kind == "items" {
		list = cb.Items
	}
	for _, a := range list {
		if a.ID == key || a.Label == key || strings.Contains(a.Label, key) {
			return a.ID
		}
	}
	return key
}

// unitID 把名称解析为战斗单位 ID。
func (c *client) unitID(key string) string {
	if key == "" {
		return ""
	}
	var cb *dto.CombatV1
	if err := c.call("get_combat", "", nil, &cb); err != nil || cb == nil {
		return key
	}
	for _, u := range append(append([]dto.CombatUnitV1{}, cb.Enemies...), cb.Party...) {
		if u.ID == key || u.Name == key {
			return u.ID
		}
	}
	for _, u := range append(append([]dto.CombatUnitV1{}, cb.Enemies...), cb.Party...) {
		if !u.Down && strings.Contains(u.Name, key) {
			return u.ID
		}
	}
	return key
}

func (c *client) growth() *dto.GrowthV1 {
	var g *dto.GrowthV1
	_ = c.call("get_growth", "", nil, &g)
	return g
}

func (c *client) attrID(key string) string {
	if g := c.growth(); g != nil {
		for _, a := range g.Attributes {
			if a.ID == key || a.Name == key {
				return a.ID
			}
		}
	}
	return key
}

func (c *client) skillID(key string) string {
	if g := c.growth(); g != nil {
		for _, s := range g.Skills {
			if s.Card.ID == key || s.Card.Name == key {
				return s.Card.ID
			}
		}
	}
	return key
}

func (c *client) slotID(key string) string {
	if g := c.growth(); g != nil {
		for _, s := range g.Equipment {
			if s.Slot == key || s.Name == key || (s.Item != nil && s.Item.Name == key) {
				return s.Slot
			}
		}
	}
	return key
}

func (c *client) itemID(key string) string {
	var inv dto.InventoryV1
	if err := c.call("get_inventory", "", nil, &inv); err == nil {
		for _, it := range inv.Items {
			if it.ID == key || it.Name == key {
				return it.ID
			}
		}
		for _, it := range inv.Items {
			if strings.Contains(it.Name, key) {
				return it.ID
			}
		}
	}
	return key
}

func (c *client) cardID(key string) string {
	var cx dto.CodexV1
	if err := c.call("get_codex", "", nil, &cx); err == nil {
		for _, cat := range cx.Categories {
			for _, e := range cat.Entries {
				if e.ID == key || (e.Known && e.Name == key) {
					return e.ID
				}
			}
		}
	}
	return key
}

func bar(cur, max, width int) string {
	if max <= 0 {
		return ""
	}
	n := cur * width / max
	if cur > 0 && n == 0 {
		n = 1
	}
	return strings.Repeat("█", n) + strings.Repeat("░", width-n)
}

func (c *client) printUnit(u dto.CombatUnitV1) {
	mark := "  "
	if u.Current {
		mark = "▶ "
	}
	name := u.Name
	if u.Mech && u.MechName != "" && u.MechName != u.Name {
		name += "〔" + u.MechName + "〕"
	}
	line := fmt.Sprintf("%s%-10s Lv%-2d HP %s %d/%d", mark, name, u.Level, bar(u.HP, u.MaxHP, 12), u.HP, u.MaxHP)
	if u.MaxSP > 0 {
		line += fmt.Sprintf("  SP %d/%d", u.SP, u.MaxSP)
	}
	if u.HeatMax > 0 {
		line += fmt.Sprintf("  过热 %d/%d", u.Heat, u.HeatMax)
	}
	if u.Mech && u.PilotHP > 0 {
		line += fmt.Sprintf("  驾驶员 %d", u.PilotHP)
	}
	var st []string
	for _, s := range u.Statuses {
		st = append(st, fmt.Sprintf("%s%d", s.Name, s.Turns))
	}
	if u.Defending {
		st = append(st, "防御")
	}
	if u.Down {
		st = append(st, "倒下")
	}
	if len(st) > 0 {
		line += "  [" + strings.Join(st, " ") + "]"
	}
	c.printf("%s  (%s)\n", line, u.ID)
}

func (c *client) printCombat(cb *dto.CombatV1) {
	c.printf("\n┌─ ⚔ %s · 第 %d 回合 ─ %s %d/%d\n", cb.Title, cb.Round, cb.ResourceName, cb.Mercury, cb.MercuryMax)
	c.printf("│ 我方\n")
	for _, u := range cb.Party {
		c.printUnit(u)
	}
	c.printf("│ 敌方\n")
	for _, u := range cb.Enemies {
		c.printUnit(u)
	}
	var acts []string
	for _, a := range cb.Actions {
		if !a.Disabled {
			acts = append(acts, a.Label)
		}
	}
	var sk []string
	for _, a := range cb.Skills {
		s := a.Label
		if a.Hint != "" {
			s += "(" + a.Hint + ")"
		}
		if a.Disabled {
			s += "✗"
		}
		sk = append(sk, s)
	}
	var it []string
	for _, a := range cb.Items {
		it = append(it, fmt.Sprintf("%s×%d", a.Label, a.Qty))
	}
	c.printf("└ 行动：%s\n  技能：%s\n  道具：%s\n", strings.Join(acts, " / "), strings.Join(sk, " / "), strings.Join(it, " / "))
}

func (c *client) showCombat() {
	var cb *dto.CombatV1
	if err := c.call("get_combat", "", nil, &cb); err != nil {
		c.printf("%v\n", err)
		return
	}
	if cb == nil {
		c.printf("当前不在战斗中。\n")
		return
	}
	c.printCombat(cb)
}

func (c *client) showGrowth() {
	g := c.growth()
	if g == nil {
		c.printf("这个故事包没有数值成长。\n")
		return
	}
	c.printf("Lv.%d  经验 %d/%d  HP %d/%d  %s %d/%d  属性点 %d  技能点 %d\n", g.Level, g.XP, g.XPNext, g.HP, g.MaxHP, g.ResourceName, g.Mercury, g.MercuryMax, g.AttrPoints, g.SkillPoints)
	var st []string
	for _, s := range g.Stats {
		v := fmt.Sprintf("%s %d", s.Name, s.Value)
		if s.Modifier != 0 {
			v += fmt.Sprintf("(%+d)", s.Modifier)
		}
		st = append(st, v)
	}
	c.printf("数值：%s\n", strings.Join(st, "  "))
	for _, a := range g.Attributes {
		c.printf("  属性 %s %d  %s\n", a.Name, a.Value, a.Effect)
	}
	for _, e := range g.Equipment {
		name := "—"
		if e.Item != nil {
			name = e.Item.Name + "〔" + e.Item.RarityName + "〕"
		}
		c.printf("  装备 %s：%s\n", e.Name, name)
	}
	for _, s := range g.Skills {
		state := "已掌握"
		if !s.Learned {
			state = fmt.Sprintf("可修习（%d 点）", s.Cost)
			if s.Blocked != "" {
				state = "未解锁：" + s.Blocked
			}
		}
		c.printf("  技能 %s — %s\n", s.Card.Name, state)
	}
}

func (c *client) printCard(e dto.CardV1) {
	if !e.Known {
		c.printf("  ？？？（%s，尚未解锁）\n", e.KindName)
		return
	}
	rar := ""
	if e.RarityName != "" {
		rar = "〔" + e.RarityName + "〕"
	}
	c.printf("  %s%s · %s  (%s)\n", e.Name, rar, e.KindName, e.ID)
	var kv []string
	for _, s := range e.Stats {
		kv = append(kv, s.Label+" "+s.Value)
	}
	if len(kv) > 0 {
		c.printf("    %s\n", strings.Join(kv, "  "))
	}
	if e.Description != "" {
		c.printf("    %s\n", e.Description)
	}
	if e.Lore != "" {
		c.printf("    “%s”\n", e.Lore)
	}
}

func (c *client) showCodex(filter string) {
	var cx dto.CodexV1
	if err := c.call("get_codex", "", nil, &cx); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printf("图鉴：已解锁 %d/%d\n", cx.Unlocked, cx.Total)
	for _, cat := range cx.Categories {
		if filter != "" && !strings.Contains(cat.Name, filter) && cat.Kind != filter {
			continue
		}
		c.printf("【%s】%d/%d\n", cat.Name, cat.Unlocked, len(cat.Entries))
		for _, e := range cat.Entries {
			if filter == "" {
				if e.Known {
					c.printf("  %s（%s）\n", e.Name, e.ID)
				} else {
					c.printf("  ？？？\n")
				}
			} else {
				c.printCard(e)
			}
		}
	}
}

func (c *client) showCard(id string) {
	var e dto.CardV1
	if err := c.call("get_card", "", map[string]string{"id": id}, &e); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printCard(e)
}

func (c *client) showRelations(filter string) {
	var r dto.RelationsV1
	if err := c.call("get_relations", "", nil, &r); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printf("关系网（你所知道的）：\n")
	for _, e := range r.Edges {
		if filter != "" && !strings.Contains(e.FromName+e.ToName, filter) {
			continue
		}
		var dv []string
		for _, d := range e.Values {
			dv = append(dv, fmt.Sprintf("%s %d", d.Name, d.Value))
		}
		src := ""
		if e.Source != "" {
			src = "（" + e.Source + "）"
		}
		c.printf("  %s → %s：%s  %s%s\n", e.FromName, e.ToName, e.Label, strings.Join(dv, " "), src)
	}
	n := len(r.History)
	start := 0
	if n > 8 {
		start = n - 8
	}
	if n > 0 {
		c.printf("最近变化：\n")
	}
	for _, h := range r.History[start:] {
		c.printf("  第%d回合 %s\n", h.Turn, h.Text)
	}
}

func (c *client) showCards() {
	var cs dto.CardsV1
	if err := c.call("get_cards", "", nil, &cs); err != nil {
		c.printf("%v\n", err)
		return
	}
	pr := func(title string, list []dto.CharacterCardV1) {
		if len(list) == 0 {
			return
		}
		c.printf("【%s】\n", title)
		for _, cc := range list {
			var rel []string
			for _, d := range cc.Relation {
				rel = append(rel, fmt.Sprintf("%s %d", d.Name, d.Value))
			}
			pic := ""
			if cc.Portrait {
				pic = " 🖼"
			}
			c.printf("  %s（%s）Lv%d%s  %s\n", cc.Name, cc.Role, cc.Level, pic, strings.Join(rel, " "))
		}
	}
	pr("角色卡", cs.Active)
	pr("已归档", cs.Archived)
	pr("已故", cs.Dead)
}

func (c *client) showMainline() {
	sc, ok := c.scene()
	if !ok || sc.Mainline == nil {
		c.printf("这个故事包没有主线追踪。\n")
		return
	}
	m := sc.Mainline
	c.printf("模式：%s  贴合度 %d%%（偏离 %d，轻微 %d / 严重 %d）\n", m.ModeLabel, m.Adherence, m.Deviation, m.Mild, m.Heavy)
	if m.Anchor != "" {
		c.printf("主线章节 %d/%d：%s\n", m.AnchorIdx+1, m.Anchors, m.Anchor)
	}
	if m.Objective != "" {
		c.printf("目标：%s\n", m.Objective)
	}
	if m.Node != nil {
		c.printf("当前节点：%s — %s（%s）\n", m.Node.Title, m.Node.Objective, m.Node.Source)
	}
	if m.Pending {
		c.printf("⚠ 你已严重偏离主线：/mode return 回到主线，或 /mode free 进入自由推演。\n")
	}
}

// showRPG 在每回合后打印战斗面板、通知与主线提示。
func (c *client) showRPG(v dto.TurnV1) {
	for _, n := range v.Notices {
		c.printf("  ✦ %s\n", n.Text)
	}
	if v.Scene.Combat != nil {
		c.printCombat(v.Scene.Combat)
	}
	if m := v.Scene.Mainline; m != nil && m.Pending {
		label := "进入自由推演（AI 生成新主线）"
		if !m.FreeOnline {
			label = "进入沙盒模式（离线：模板委托）"
		}
		c.printf("\n  ⚠ 你已严重偏离主线（贴合度 %d%%）。\n    /mode return  回到主线\n    /mode free    %s\n", m.Adherence, label)
	}
}

func (c *client) mechs() dto.MechsV1 {
	var v dto.MechsV1
	_ = c.call("get_mechs", "", nil, &v)
	return v
}

func (c *client) mechID(key string) string {
	for _, m := range c.mechs().Mechs {
		if m.ID == key || (m.Known && (m.Name == key || strings.Contains(m.Name, key))) {
			return m.ID
		}
	}
	return key
}

func (c *client) ownedMech() *dto.MechCardV1 {
	for _, m := range c.mechs().Mechs {
		if m.Owned {
			m := m
			return &m
		}
	}
	return nil
}

func (c *client) showMechs() {
	v := c.mechs()
	if len(v.Mechs) == 0 {
		c.printf("这个故事包没有机甲。\n")
		return
	}
	for _, m := range v.Mechs {
		if !m.Known {
			c.printf("  ？？？\n")
			continue
		}
		own := ""
		if m.Owned {
			own = " ★你的机体"
		}
		c.printf("  %s〔%s〕 %s  未知字段 %d%s  (%s)\n", m.Name, m.RarityName, m.Status.Name, m.Unknown, own, m.ID)
	}
}

func (c *client) showMech(id string) {
	var m dto.MechCardV1
	if err := c.call("get_mech", "", map[string]string{"id": id}, &m); err != nil {
		c.printf("%v\n", err)
		return
	}
	if !m.Known {
		c.printf("  ？？？（尚未解锁）\n")
		return
	}
	pic := "无立绘"
	if m.Portrait {
		pic = "有立绘"
	}
	c.printf("╔ %s〔%s〕 状态：%s · %s\n", m.Name, m.RarityName, m.Status.Name, pic)
	for _, f := range m.Fields {
		c.printf("║ %s：%s\n", f.Label, f.Value)
	}
	for _, sp := range m.Specs {
		if sp.Known {
			b := ""
			if sp.Bonus != 0 {
				b = fmt.Sprintf("（改装 %+d）", sp.Bonus)
			}
			c.printf("║ %-6s %s %d%s%s\n", sp.Name, bar(sp.Value, sp.Max, 10), sp.Value, sp.Unit, b)
		} else {
			c.printf("║ %-6s ?????????? 未知\n", sp.Name)
		}
	}
	if m.StatsKnown {
		var st []string
		for _, x := range m.Stats {
			v := fmt.Sprintf("%s %d", x.Name, x.Value)
			if x.Bonus != 0 {
				v += fmt.Sprintf("(%+d)", x.Bonus)
			}
			st = append(st, v)
		}
		c.printf("║ 战斗数值：%s\n", strings.Join(st, "  "))
	} else {
		c.printf("║ 战斗数值：未知\n")
	}
	for _, e := range m.Energy {
		c.printf("║ %s：%s\n", e.Label, e.Value)
	}
	slot := func(title string, list []dto.MechSlotV1) {
		for _, sl := range list {
			part := "（空）"
			switch {
			case !sl.Known:
				part = "未知"
			case sl.Part != nil:
				part = sl.Part.Name
			}
			c.printf("║ %s·%s [%s]：%s\n", title, sl.Name, sl.ID, part)
			for _, in := range sl.Install {
				c.printf("║     可%s\n", in.Label)
			}
		}
	}
	slot("改装槽", m.Slots)
	slot("武器挂点", m.Hardpoints)
	var sk []string
	for _, s := range m.Skills {
		sk = append(sk, s.Name)
	}
	c.printf("║ 技能：%s\n", strings.Join(sk, " / "))
	if m.Description != "" {
		c.printf("║ %s\n", m.Description)
	}
	if m.LoreKnown && m.Lore != "" {
		c.printf("║ “%s”\n", m.Lore)
	} else if !m.LoreKnown {
		c.printf("║ 来历：未知\n")
	}
	for i, h := range m.History {
		if i >= 6 {
			break
		}
		c.printf("║ · 第%d回合 %s\n", h.Turn, h.Text)
	}
	c.printf("╚ 仍有 %d 项未知\n", m.Unknown)
}

func (c *client) installPart(part, slot string) {
	m := c.ownedMech()
	if m == nil {
		c.printf("你没有可以改装的机甲。\n")
		return
	}
	item := c.itemID(part)
	for _, list := range [][]dto.MechSlotV1{m.Slots, m.Hardpoints} {
		for _, sl := range list {
			if slot != "" && sl.ID != slot && sl.Name != slot && sl.ID != "hp:"+slot {
				continue
			}
			for _, in := range sl.Install {
				if in.Item == item {
					c.quick(in)
					return
				}
			}
		}
	}
	c.printf("没有可以安装 %s 的槽位。\n", part)
}

func (c *client) uninstallPart(slot string) {
	m := c.ownedMech()
	if m == nil {
		c.printf("你没有可以改装的机甲。\n")
		return
	}
	for _, list := range [][]dto.MechSlotV1{m.Slots, m.Hardpoints} {
		for _, sl := range list {
			if (sl.ID == slot || sl.Name == slot || sl.ID == "hp:"+slot || (sl.Part != nil && sl.Part.Name == slot)) && sl.Remove != nil {
				c.quick(*sl.Remove)
				return
			}
		}
	}
	c.printf("没有找到可以卸下的槽位 %s。\n", slot)
}
