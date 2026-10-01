package query

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

var kindNames = map[string]string{
	"weapon": "武器", "armor": "护甲", "accessory": "饰品", "mech_part": "机甲部件", "consumable": "消耗品",
	"key": "关键物品", "material": "材料", "item": "物品", "skill": "技能", "enemy": "敌人", "mech": "机甲",
	"location": "地点", "character": "人物", "faction": "势力", "lore": "传说", "tech": "科技",
}

var codexOrder = []string{"character", "faction", "location", "item", "skill", "enemy", "mech", "tech", "lore"}

var codexNames = map[string]string{
	"character": "人物", "faction": "势力", "location": "地点", "item": "物品与装备", "skill": "技能",
	"enemy": "敌人", "mech": "机甲", "tech": "科技", "lore": "传说",
}

var targetNames = map[string]string{
	combat.TargetEnemy: "单个敌人", combat.TargetAllEnemies: "全体敌人", combat.TargetSelf: "自身",
	combat.TargetAlly: "单个友方", combat.TargetAllAllies: "全体友方",
}

var tierNames = map[string]string{"minion": "杂兵", "elite": "精英", "boss": "首领"}

func modsText(m combat.Stats) []string {
	var out []string
	for _, k := range combat.StatKeys {
		if v := m[k]; v != 0 {
			out = append(out, fmt.Sprintf("%s %+d", combat.StatNames[k], v))
		}
	}
	return out
}

func (q *Q) rarity(c *dto.CardV1, id string) {
	if id == "" {
		return
	}
	r := q.Pkg.Combat.Config.RarityOf(id)
	c.Rarity, c.RarityName, c.RarityColor = r.ID, r.Name, r.Color
}

// autoCard 构造由故事包定义自动生成的介绍卡（不考虑是否已解锁）。
func (q *Q) autoCard(id string) (dto.CardV1, bool) {
	p := q.Pkg
	cb := p.Combat
	var c dto.CardV1
	switch {
	case p.Items[id] != nil:
		it := p.Items[id]
		kind := it.Kind
		if kind == "" {
			kind = "item"
		}
		c = dto.CardV1{ID: id, Kind: "item", KindName: kindNames[kind], Name: it.Name, Icon: it.Icon, Description: it.Description, Lore: it.Lore}
		if c.Icon == "" {
			c.Icon = "inventory_2"
		}
		q.rarity(&c, it.Rarity)
		if it.Slot != "" {
			c.Stats = append(c.Stats, dto.KVV1{Label: "槽位", Value: cb.Config.SlotName(it.Slot)})
		}
		if m := modsText(it.Mods); len(m) > 0 {
			c.Stats = append(c.Stats, dto.KVV1{Label: "加成", Value: strings.Join(m, "，")})
		}
		if it.Level > 0 {
			c.Stats = append(c.Stats, dto.KVV1{Label: "需求等级", Value: fmt.Sprint(it.Level)})
		}
		if ic := it.Combat; ic != nil {
			var eff []string
			if ic.Heal > 0 {
				eff = append(eff, fmt.Sprintf("恢复 %d 生命", ic.Heal))
			}
			if ic.HealPct > 0 {
				eff = append(eff, fmt.Sprintf("恢复 %d%% 生命", ic.HealPct))
			}
			if ic.SP > 0 {
				eff = append(eff, fmt.Sprintf("恢复 %d 体力", ic.SP))
			}
			if ic.Mercury > 0 {
				eff = append(eff, fmt.Sprintf("补充 %d %s", ic.Mercury, cb.Config.ResourceName))
			}
			if ic.Cool > 0 {
				eff = append(eff, fmt.Sprintf("降温 %d", ic.Cool))
			}
			if ic.Damage > 0 {
				eff = append(eff, fmt.Sprintf("造成 %d 伤害", ic.Damage))
			}
			if len(eff) > 0 {
				c.Stats = append(c.Stats, dto.KVV1{Label: "效果", Value: strings.Join(eff, "，")})
			}
		}
		if it.Price > 0 {
			c.Stats = append(c.Stats, dto.KVV1{Label: "价格", Value: fmt.Sprint(it.Price)})
		}
	case cb.Skills[id] != nil:
		sk := cb.Skills[id]
		c = dto.CardV1{ID: id, Kind: "skill", KindName: "技能", Name: sk.Name, Icon: sk.Icon, Description: sk.Description, Lore: sk.Lore}
		if c.Icon == "" {
			c.Icon = "bolt"
		}
		q.rarity(&c, sk.Rarity)
		if sk.Power > 0 {
			v := fmt.Sprintf("%d%%", sk.Power)
			if sk.Hits > 1 {
				v += fmt.Sprintf(" ×%d", sk.Hits)
			}
			c.Stats = append(c.Stats, dto.KVV1{Label: "威力", Value: v})
		}
		c.Stats = append(c.Stats, dto.KVV1{Label: "目标", Value: targetNames[sk.Target]})
		if cost := costText(cb, sk.Cost); cost != "" {
			c.Stats = append(c.Stats, dto.KVV1{Label: "消耗", Value: cost})
		}
		if sk.MechOnly {
			c.Tags = append(c.Tags, "机甲专用")
		}
		if sk.Learn != nil {
			c.Stats = append(c.Stats, dto.KVV1{Label: "修习", Value: fmt.Sprintf("等级 %d · %d 技能点", sk.Learn.Level, sk.Learn.Cost)})
		}
	case cb.Enemies[id] != nil:
		e := cb.Enemies[id]
		st := p.EnemyStats(e)
		c = dto.CardV1{ID: id, Kind: "enemy", KindName: "敌人", Name: e.Name, Icon: e.Icon, Description: e.Description, Lore: e.Lore}
		if c.Icon == "" {
			c.Icon = "skull"
		}
		q.rarity(&c, e.Rarity)
		c.Stats = append(c.Stats, dto.KVV1{Label: "等级", Value: fmt.Sprintf("%d · %s", e.Level, tierNames[e.Tier])})
		c.Stats = append(c.Stats, dto.KVV1{Label: "数值", Value: fmt.Sprintf("生命 %d  攻击 %d  防御 %d  速度 %d", st[combat.HP], st[combat.ATK], st[combat.DEF], st[combat.SPD])})
		var sks []string
		for _, s := range e.Skills {
			sks = append(sks, cb.Skills[s].Name)
		}
		if len(sks) > 0 {
			c.Stats = append(c.Stats, dto.KVV1{Label: "技能", Value: strings.Join(sks, "、")})
		}
		c.Stats = append(c.Stats, dto.KVV1{Label: "奖励", Value: fmt.Sprintf("经验 %d · 铜币 %d", e.XP, e.Gold)})
		if e.Mech {
			c.Tags = append(c.Tags, "机甲")
		}
		if e.Faction != "" {
			c.Tags = append(c.Tags, p.CodexName(e.Faction))
		}
	case cb.Mechs[id] != nil:
		m := cb.Mechs[id]
		c = dto.CardV1{ID: id, Kind: "mech", KindName: "机甲", Name: m.Name, Icon: m.Icon, Description: m.Description, Lore: m.Lore}
		if c.Icon == "" {
			c.Icon = "precision_manufacturing"
		}
		q.rarity(&c, m.Rarity)
		c.Stats = append(c.Stats, dto.KVV1{Label: "数值", Value: fmt.Sprintf("耐久 %d  攻击 %d  防御 %d  速度 %d", m.Stats[combat.HP], m.Stats[combat.ATK], m.Stats[combat.DEF], m.Stats[combat.SPD])})
		c.Stats = append(c.Stats, dto.KVV1{Label: cb.Config.ResourceName, Value: fmt.Sprintf("启动 %d · 每回合 %d", m.EngageCost, m.PerTurn)})
		c.Stats = append(c.Stats, dto.KVV1{Label: "热量", Value: fmt.Sprintf("上限 %d · 普攻 +%d · 每回合冷却 %d", m.HeatMax, m.AttackHeat, m.CoolPerTurn)})
	case p.Locations[id] != nil:
		l := p.Locations[id]
		c = dto.CardV1{ID: id, Kind: "location", KindName: "地点", Name: l.Name, Icon: "place", Description: l.Description, Lore: l.Short}
	case p.Characters[id] != nil:
		ch := p.Characters[id]
		c = dto.CardV1{ID: id, Kind: "character", KindName: "人物", Name: ch.Name(), Icon: ch.Icon, Description: ch.Description, Lore: ch.Lore}
		if c.Icon == "" {
			c.Icon = "person"
		}
		if ch.Identity.Role != "" {
			c.Stats = append(c.Stats, dto.KVV1{Label: "身份", Value: ch.Identity.Role})
		}
		if ch.Faction != "" {
			c.Stats = append(c.Stats, dto.KVV1{Label: "势力", Value: p.CodexName(ch.Faction)})
		}
	default:
		return c, false
	}
	return c, true
}

func costText(cb *combat.Content, c combat.Cost) string {
	var out []string
	if c.SP > 0 {
		out = append(out, fmt.Sprintf("%d 体力", c.SP))
	}
	if c.Mercury > 0 {
		out = append(out, fmt.Sprintf("%d %s", c.Mercury, cb.Config.ResourceName))
	}
	if c.Heat > 0 {
		out = append(out, fmt.Sprintf("%d 热量", c.Heat))
	}
	return strings.Join(out, " · ")
}

// Card 返回一张介绍卡（未解锁的条目只显示“？？？”）。
func (q *Q) Card(s *state.State, id string) (dto.CardV1, bool) {
	c, ok := q.autoCard(id)
	for _, e := range q.Pkg.Codex {
		if e.ID != id {
			continue
		}
		if !ok {
			c = dto.CardV1{ID: id, Kind: e.Kind, KindName: kindNames[e.Kind]}
			ok = true
		}
		if e.Name != "" {
			c.Name = e.Name
		}
		if e.Icon != "" {
			c.Icon = e.Icon
		}
		if e.Description != "" {
			c.Description = e.Description
		}
		if e.Lore != "" {
			c.Lore = e.Lore
		}
		q.rarity(&c, e.Rarity)
		keys := make([]string, 0, len(e.Stats))
		for k := range e.Stats {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			c.Stats = append(c.Stats, dto.KVV1{Label: k, Value: e.Stats[k]})
		}
		c.Tags = append(c.Tags, e.Tags...)
	}
	if !ok {
		return c, false
	}
	if c.Icon == "" {
		c.Icon = "menu_book"
	}
	_, known := s.R().Codex[id]
	c.Known = known
	if it := q.Pkg.Items[id]; it != nil && it.Mech != nil {
		c.Stats = append(c.Stats, q.mechPartStats(it)...)
	}
	if !known {
		c.Name, c.Description, c.Lore, c.Stats, c.Tags = "？？？", "尚未解锁。", "", nil, nil
	}
	if c.Kind == "mech" {
		q.gateMechCard(s, &c)
	}
	return c, true
}

// Codex 构造图鉴。
func (q *Q) Codex(s *state.State) dto.CodexV1 {
	p := q.Pkg
	ids := map[string][]string{}
	seen := map[string]bool{}
	add := func(kind, id string) {
		if !seen[id] {
			seen[id] = true
			ids[kind] = append(ids[kind], id)
		}
	}
	for _, e := range p.Codex {
		if _, ok := q.autoCard(e.ID); !ok {
			add(e.Kind, e.ID)
		}
	}
	for _, id := range p.NPCIDs {
		add("character", id)
	}
	for _, id := range p.LocationIDs {
		add("location", id)
	}
	for _, id := range p.ItemIDs {
		add("item", id)
	}
	for _, id := range p.Combat.SkillIDs {
		add("skill", id)
	}
	for _, id := range p.Combat.EnemyIDs {
		add("enemy", id)
	}
	for _, id := range p.Combat.MechIDs {
		add("mech", id)
	}
	v := dto.CodexV1{Categories: []dto.CodexCategoryV1{}}
	for _, k := range codexOrder {
		if len(ids[k]) == 0 {
			continue
		}
		cat := dto.CodexCategoryV1{Kind: k, Name: codexNames[k], Entries: []dto.CardV1{}}
		for _, id := range ids[k] {
			c, _ := q.Card(s, id)
			if c.Known {
				cat.Unlocked++
			}
			cat.Entries = append(cat.Entries, c)
		}
		v.Unlocked += cat.Unlocked
		v.Total += len(cat.Entries)
		v.Categories = append(v.Categories, cat)
	}
	return v
}

// ---------- 战斗面板 ----------

func (q *Q) unitView(s *state.State, c *state.Combat, u *state.Unit) dto.CombatUnitV1 {
	p := q.Pkg
	hp, mx := engine.UnitHP(u)
	v := dto.CombatUnitV1{Portrait: q.HasPortrait(u.Ref), ID: u.ID, Ref: u.Ref, Name: u.Name, Side: u.Side, Level: u.Level, HP: hp, MaxHP: mx, SP: u.SP, MaxSP: u.MaxSP, Down: u.Down, Defending: u.Defending, Statuses: []dto.StatusChipV1{}}
	if u.ID == loader.PlayerID {
		v.Name = s.Player.Name
	}
	if cur := c.Current(); cur != nil && cur.ID == u.ID {
		v.Current = true
	}
	if u.Mech != nil {
		v.Mech, v.MechName, v.PilotHP, v.Heat, v.HeatMax = true, u.Mech.Name, u.HP, u.Mech.Heat, u.Mech.HeatMax
	}
	if d := p.Combat.Enemies[u.Ref]; d != nil {
		v.Icon, v.Tier = d.Icon, d.Tier
		v.Mech = v.Mech || d.Mech
	} else if ch := p.Characters[u.Ref]; ch != nil {
		v.Icon = ch.Icon
	} else {
		v.Icon = "person"
	}
	for _, st := range u.Statuses {
		def := engine.StatusDef(p, st.ID)
		v.Statuses = append(v.Statuses, dto.StatusChipV1{ID: st.ID, Name: def.Name, Icon: def.Icon, Turns: st.Turns, Debuff: def.Debuff, Note: def.Description})
	}
	return v
}

// Combat 构造战斗面板（没有战斗时返回 nil）。
func (q *Q) Combat(s *state.State) *dto.CombatV1 {
	if s.RPG == nil || s.RPG.Combat == nil {
		return nil
	}
	p := q.Pkg
	c := s.RPG.Combat
	cfg := &p.Combat.Config
	v := &dto.CombatV1{Encounter: c.Encounter, Title: c.Title, Round: c.Round, Party: []dto.CombatUnitV1{}, Enemies: []dto.CombatUnitV1{},
		Actions: []dto.CombatActionV1{}, Skills: []dto.CombatActionV1{}, Items: []dto.CombatActionV1{}, Order: []string{},
		ResourceName: cfg.ResourceName, Mercury: s.RPG.Mercury, MercuryMax: cfg.MercuryMax}
	for _, u := range c.Units {
		if u.Side == engine.SideParty {
			v.Party = append(v.Party, q.unitView(s, c, u))
		} else {
			v.Enemies = append(v.Enemies, q.unitView(s, c, u))
		}
	}
	for _, id := range c.Order {
		if u := c.Unit(id); u != nil && !u.Down {
			v.Order = append(v.Order, narrator.UnitName(nil, s, id))
		}
	}
	pu := c.Unit(loader.PlayerID)
	cur := c.Current()
	v.YourTurn = cur != nil && cur.ID == loader.PlayerID
	if pu == nil {
		return v
	}
	v.Actions = append(v.Actions, dto.CombatActionV1{Kind: "attack", Label: "攻击", Icon: "swords", Target: combat.TargetEnemy})
	for _, id := range engine.UnitSkills(p, pu) {
		sk := p.Combat.Skills[id]
		a := dto.CombatActionV1{Kind: "skill", ID: id, Label: sk.Name, Icon: sk.Icon, Hint: costText(p.Combat, sk.Cost), Target: sk.Target}
		if why := engine.SkillBlocked(p, s, pu, sk); why != "" {
			a.Disabled, a.Reason = true, why
		}
		if pu.Mech != nil && sk.Cost.Heat > 0 && pu.Mech.Heat+sk.Cost.Heat >= pu.Mech.HeatMax && a.Reason == "" {
			a.Reason = "会导致过热"
		}
		v.Skills = append(v.Skills, a)
	}
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		if n := s.Player.Inventory[id]; n > 0 && it.Combat != nil {
			t := it.Combat.Target
			if t == "" {
				t = combat.TargetSelf
				if it.Combat.Damage > 0 {
					t = combat.TargetEnemy
				}
			}
			v.Items = append(v.Items, dto.CombatActionV1{Kind: "item", ID: id, Label: it.Name, Icon: it.Icon, Qty: n, Target: t, Hint: fmt.Sprintf("×%d", n)})
		}
	}
	v.Actions = append(v.Actions, dto.CombatActionV1{Kind: "defend", Label: "防御", Icon: "shield", Target: "none", Hint: "减伤 50%"})
	if mid := p.PlayerCombat().Mech; mid != "" {
		if m := p.Combat.Mechs[mid]; m != nil {
			if pu.Mech == nil {
				a := dto.CombatActionV1{Kind: "mech", ID: mid, Label: "启动" + m.Name, Icon: "precision_manufacturing", Target: "none", Hint: fmt.Sprintf("%d %s", m.EngageCost, cfg.ResourceName)}
				ok, _ := q.check(s, m.Requires)
				switch {
				case !c.AllowMech:
					a.Disabled, a.Reason = true, "这场战斗无法启动"
				case !ok:
					a.Disabled, a.Reason = true, "尚未觉醒"
				case s.RPG.Mercury < max(m.EngageCost, 1):
					a.Disabled, a.Reason = true, cfg.ResourceName+"不足"
				default:
					if f, _ := engine.MechForm(p, s, mid); !f.Status.Engage {
						a.Disabled, a.Reason = true, "机体"+f.Status.Name
					}
				}
				v.Actions = append(v.Actions, a)
			} else {
				v.Actions = append(v.Actions, dto.CombatActionV1{Kind: "eject", Label: "脱离机甲", Icon: "eject", Target: "none"})
			}
		}
	}
	flee := dto.CombatActionV1{Kind: "flee", Label: "逃跑", Icon: "directions_run", Target: "none"}
	if c.NoFlee {
		flee.Disabled, flee.Reason = true, "无路可退"
	}
	v.Actions = append(v.Actions, flee)
	return v
}

func (q *Q) check(s *state.State, expr string) (bool, error) {
	if expr == "" {
		return true, nil
	}
	return q.Eval.EvalBool(expr, engine.Vars(q.Pkg, s, "", ""))
}

// ---------- 关系网 ----------

func edgeTone(p *loader.Package, vals map[string]int) (string, string) {
	pos, neg := 0, 0
	for _, d := range p.Relations.Dimensions {
		v := vals[d.ID]
		switch {
		case d.Negative:
			neg += max(v, 0)
		case v >= 0:
			pos += v
		default:
			neg -= v
		}
	}
	switch {
	case pos >= 20 && neg >= 20:
		return "复杂", "mixed"
	case pos >= 40:
		return "亲密", "positive"
	case pos >= 12:
		return "友好", "positive"
	case neg >= 40:
		return "敌对", "negative"
	case neg >= 12:
		return "戒备", "negative"
	}
	return "平淡", "neutral"
}

func (q *Q) dims(vals map[string]int) []dto.DimValueV1 {
	out := []dto.DimValueV1{}
	for _, d := range q.Pkg.Relations.Dimensions {
		if v := vals[d.ID]; v != 0 {
			out = append(out, dto.DimValueV1{ID: d.ID, Name: d.Name, Value: v, Negative: d.Negative})
		}
	}
	return out
}

func (q *Q) personName(s *state.State, id string) string {
	if id == loader.PlayerID {
		return "你"
	}
	if s.RPG != nil {
		if c := s.RPG.Cards[id]; c != nil && c.Dynamic {
			return c.Name
		}
	}
	return q.Pkg.EntityName(id)
}

// met 报告玩家是否认识这个人（交谈过、有角色卡或同处一地）。
func (q *Q) met(s *state.State, id string) bool {
	n := s.NPCs[id]
	if n == nil {
		return false
	}
	if n.Talks > 0 || n.Location == s.Player.Location {
		return true
	}
	if s.RPG != nil {
		if _, ok := s.RPG.Codex[id]; ok {
			return true
		}
	}
	return false
}

// Relations 构造玩家视角的关系网：与玩家有关的关系 + 玩家知道（看到 / 听说）的 NPC 之间的关系。
// NPC 之间的数值是玩家得知时的快照，之后的变化只有再次目睹或听说才会更新。
func (q *Q) Relations(s *state.State) dto.RelationsV1 {
	p := q.Pkg
	r := s.R()
	v := dto.RelationsV1{Dimensions: []dto.DimValueV1{}, People: []dto.PersonV1{{ID: loader.PlayerID, Name: s.Player.Name, Player: true, Icon: "person", Portrait: q.HasPortrait(loader.PlayerID)}}, Edges: []dto.EdgeV1{}, History: []dto.RelChangeV1{}}
	for _, d := range p.Relations.Dimensions {
		v.Dimensions = append(v.Dimensions, dto.DimValueV1{ID: d.ID, Name: d.Name, Negative: d.Negative})
	}
	inWeb := map[string]bool{}
	person := func(id string) {
		if inWeb[id] || id == loader.PlayerID {
			return
		}
		inWeb[id] = true
		pv := dto.PersonV1{ID: id, Name: q.personName(s, id), Icon: "person", Portrait: q.HasPortrait(id)}
		if c := p.Characters[id]; c != nil {
			pv.Role = c.Identity.Role
			if c.Icon != "" {
				pv.Icon = c.Icon
			}
		}
		if card := r.Cards[id]; card != nil {
			pv.Card = card.Status
			if card.Dynamic {
				pv.Role = card.Role
			}
		}
		v.People = append(v.People, pv)
	}
	for _, id := range p.NPCIDs {
		if !q.met(s, id) {
			continue
		}
		vals := s.Edge(id, loader.PlayerID)
		person(id)
		label, tone := edgeTone(p, vals)
		v.Edges = append(v.Edges, dto.EdgeV1{From: id, To: loader.PlayerID, FromName: q.personName(s, id), ToName: "你", Values: q.dims(vals), Label: label, Tone: tone, Source: "self"})
		if pv := s.Edge(loader.PlayerID, id); len(pv) > 0 {
			label, tone := edgeTone(p, pv)
			v.Edges = append(v.Edges, dto.EdgeV1{From: loader.PlayerID, To: id, FromName: "你", ToName: q.personName(s, id), Values: q.dims(pv), Label: label, Tone: tone, Source: "self"})
		}
	}
	keys := make([]string, 0, len(r.Known))
	for k := range r.Known {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	notes := map[string]string{}
	for _, e := range p.Relations.Edges {
		notes[state.EdgeKey(e.From, e.To)] = e.Note
	}
	for _, k := range keys {
		kn := r.Known[k]
		from, to, _ := strings.Cut(k, ">")
		if from == loader.PlayerID || to == loader.PlayerID {
			continue
		}
		person(from)
		person(to)
		label, tone := edgeTone(p, kn.Values)
		e := dto.EdgeV1{From: from, To: to, FromName: q.personName(s, from), ToName: q.personName(s, to), Values: q.dims(kn.Values), Label: label, Tone: tone, Source: kn.Source, Turn: kn.Turn}
		if kn.Source == "public" {
			e.Note = notes[k]
		}
		v.Edges = append(v.Edges, e)
	}
	for id, card := range r.Cards {
		if card.Dynamic {
			person(id)
		}
	}
	for i := len(r.RelLog) - 1; i >= 0 && len(v.History) < 40; i-- {
		c := r.RelLog[i]
		if !c.Known {
			continue
		}
		var parts []string
		for _, d := range q.dims(c.Values) {
			parts = append(parts, fmt.Sprintf("%s %+d", d.Name, d.Value))
		}
		text := fmt.Sprintf("%s → %s：%s", q.personName(s, c.From), q.personName(s, c.To), strings.Join(parts, "，"))
		if c.Reason != "" {
			text += "（" + c.Reason + "）"
		}
		v.History = append(v.History, dto.RelChangeV1{Turn: c.Turn, Time: worldtime.Format(c.Minute), From: c.From, To: c.To, Text: text})
	}
	return v
}

// ---------- 角色卡 ----------

// Cards 构造角色卡列表（进行中 / 已归档 / 已故）。归档的角色保留记忆与关系，重新登场时不会失忆。
func (q *Q) Cards(s *state.State) dto.CardsV1 {
	p := q.Pkg
	r := s.R()
	v := dto.CardsV1{Active: []dto.CharacterCardV1{}, Archived: []dto.CharacterCardV1{}, Dead: []dto.CharacterCardV1{}}
	ids := slices.Clone(p.NPCIDs)
	var dyn []string
	for id, c := range r.Cards {
		if c.Dynamic {
			dyn = append(dyn, id)
		}
	}
	slices.Sort(dyn)
	ids = append(ids, dyn...)
	for _, id := range ids {
		card := r.Cards[id]
		if card == nil {
			continue
		}
		cv := dto.CharacterCardV1{Mechs: q.pilotedMechs(s, id), Portrait: q.HasPortrait(id), ID: id, Status: card.Status, Reason: card.Reason, Turn: card.Created, Dynamic: card.Dynamic}
		if card.Dynamic {
			cv.Name, cv.Role, cv.Description, cv.Icon = card.Name, card.Role, card.Description, "person_add"
		} else {
			c := p.Characters[id]
			n := s.NPCs[id]
			cv.Name, cv.Role, cv.Description, cv.Lore, cv.Icon = c.Name(), c.Identity.Role, c.Description, c.Lore, c.Icon
			if cv.Icon == "" {
				cv.Icon = "person"
			}
			if c.Faction != "" {
				cv.Faction = p.CodexName(c.Faction)
			}
			if n.Location != "" {
				cv.Location = p.EntityName(n.Location)
			}
			if st, lvl := p.NPCStats(id); st != nil {
				cv.Level = lvl
				for _, k := range []string{combat.HP, combat.ATK, combat.DEF, combat.SPD} {
					cv.Stats = append(cv.Stats, dto.StatV1{ID: k, Name: combat.StatNames[k], Value: st[k]})
				}
			}
			cv.Relation = q.dims(s.Edge(id, loader.PlayerID))
			cv.Memories = npcMemories(n)
		}
		switch card.Status {
		case state.CardArchived:
			v.Archived = append(v.Archived, cv)
		case state.CardDead:
			v.Dead = append(v.Dead, cv)
		default:
			v.Active = append(v.Active, cv)
		}
	}
	return v
}

// ---------- 成长 ----------

// Growth 构造成长面板（故事包没有战斗内容时为 nil）。
func (q *Q) Growth(s *state.State) *dto.GrowthV1 {
	p := q.Pkg
	if !p.HasCombat() {
		return nil
	}
	r := s.R()
	cfg := &p.Combat.Config
	lvl := s.PlayerLevel(p)
	hp, mx := engine.PlayerHP(p, s)
	st := engine.PlayerStats(p, s)
	g := &dto.GrowthV1{Level: lvl, XP: r.XP, XPNext: cfg.XPToNext(lvl), AttrPoints: r.AttrPoints, SkillPoints: r.SkillPoints, HP: hp, MaxHP: mx,
		Mercury: r.Mercury, MercuryMax: cfg.MercuryMax, ResourceName: cfg.ResourceName, Stats: []dto.StatV1{}, Equipment: []dto.EquipSlotV1{}, Skills: []dto.SkillCardV1{}}
	if m := p.Combat.Mechs[p.PlayerCombat().Mech]; m != nil {
		g.HasMech, g.MechName = true, m.Name
	}
	base := p.StatsFor(p.PlayerCombat(), lvl, s.Player.Attributes, nil)
	for _, k := range combat.StatKeys {
		sv := dto.StatV1{ID: k, Name: combat.StatNames[k], Value: st[k]}
		if d := st[k] - base[k]; d != 0 {
			sv.Note = fmt.Sprintf("装备 %+d", d)
			sv.Modifier = d
		}
		g.Stats = append(g.Stats, sv)
	}
	for _, sl := range cfg.Slots {
		es := dto.EquipSlotV1{Slot: sl.ID, Name: sl.Name}
		if id := r.Equipment[sl.ID]; id != "" {
			c, _ := q.Card(s, id)
			es.Item = &c
			es.Unequip = &dto.QuickActionV1{Kind: "manage", Action: "unequip", Target: sl.ID, Label: "卸下" + c.Name}
		}
		g.Equipment = append(g.Equipment, es)
	}
	for _, id := range p.Combat.SkillIDs {
		sk := p.Combat.Skills[id]
		learned := slices.Contains(r.Skills, id)
		if !learned && sk.Learn == nil {
			continue
		}
		c, _ := q.autoCard(id)
		c.Known = true
		sc := dto.SkillCardV1{Card: c, Learned: learned}
		if !learned {
			sc.Cost = sk.Learn.Cost
			if why := engine.LearnBlocked(p, s, sk); why != "" {
				sc.Blocked = why
			} else {
				sc.Learn = &dto.QuickActionV1{Kind: "manage", Action: "learn", Skill: id, Label: "修习" + sk.Name}
			}
		}
		g.Skills = append(g.Skills, sc)
	}
	g.Attributes = []dto.AttributeV1{}
	attrs := make([]string, 0, len(p.Rules.Attributes))
	for id := range p.Rules.Attributes {
		attrs = append(attrs, id)
	}
	slices.Sort(attrs)
	for _, id := range attrs {
		av := dto.AttributeV1{ID: id, Name: p.Rules.Attributes[id], Value: s.Player.Attributes[id]}
		var eff []string
		for _, k := range combat.StatKeys {
			if v := cfg.AttributeEffects[id][k]; v != 0 {
				eff = append(eff, fmt.Sprintf("%s %+d", combat.StatNames[k], v))
			}
		}
		if len(eff) > 0 {
			av.Effect = "每点：" + strings.Join(eff, "，")
		}
		if r.AttrPoints > 0 {
			av.Allocate = &dto.QuickActionV1{Kind: "manage", Action: "allocate", Target: id, Label: "提升" + av.Name}
		}
		g.Attributes = append(g.Attributes, av)
	}
	return g
}

// ---------- 本回合条目与通知 ----------

// CombatEntries 把战斗事件翻译成带掷骰分解的战斗记录。
func (q *Q) CombatEntries(before, after *state.State, evs []event.Event, turn int) []dto.EntryV1 {
	p := q.Pkg
	name := func(id string) string {
		n := narrator.UnitName(before, after, id)
		if n == "你" {
			return after.Player.Name
		}
		return n
	}
	side := func(id string) string {
		if id == loader.PlayerID {
			return engine.SideParty
		}
		for _, s := range []*state.State{after, before} {
			if s != nil && s.RPG != nil && s.RPG.Combat != nil {
				if u := s.RPG.Combat.Unit(id); u != nil {
					return u.Side
				}
			}
		}
		if strings.HasPrefix(id, "e") {
			return engine.SideEnemy
		}
		return engine.SideParty
	}
	var out []dto.EntryV1
	add := func(text string, log *dto.CombatLogV1) {
		out = append(out, dto.EntryV1{Kind: "combat", Turn: turn, Text: text, Combat: log})
	}
	for _, e := range evs {
		d := e.Data
		switch e.Type {
		case event.CombatActed:
			if d.Action == "mech" {
				continue
			}
			log := &dto.CombatLogV1{Actor: name(d.Actor), Action: d.Action, SkillID: d.Skill, ItemID: d.Item, Hit: d.Success, Roll: d.Roll, Chance: d.DC, Damage: d.Delta, Critical: d.Critical, Side: side(d.Actor)}
			if d.Target != "" && !strings.Contains(d.Target, ",") {
				log.Target = name(d.Target)
			}
			if d.DC > 0 {
				mark := "✓"
				if !d.Success {
					mark = "✗"
				}
				label := "命中"
				if d.Action == "flee" {
					label = "逃跑"
				}
				log.Chips = append(log.Chips, fmt.Sprintf("%s %d/%d%% %s", label, d.Roll, d.DC, mark))
			}
			if d.Delta > 0 && d.Values != nil {
				v := d.Values
				log.Chips = append(log.Chips, fmt.Sprintf("攻 %d×%d%%", v["atk"], v["power"]), fmt.Sprintf("防 %d", v["def"]), fmt.Sprintf("浮动 %d%%", v["variance"]))
				if d.Critical {
					log.Chips = append(log.Chips, fmt.Sprintf("暴击 ×1.5（%d/%d%%）", v["crit_roll"], v["crit_chance"]))
				}
				if v["guarded"] == 1 {
					log.Chips = append(log.Chips, "防御 ×0.5")
				}
				log.Chips = append(log.Chips, fmt.Sprintf("= %d 伤害", d.Delta))
			}
			text := narrator.ActText(p, name, d)
			if d.Delta > 0 {
				text += fmt.Sprintf(" −%d", d.Delta)
			}
			add(text, log)
		case event.UnitHPChanged:
			switch {
			case strings.HasPrefix(d.Reason, "status:"):
				st := engine.StatusDef(p, strings.TrimPrefix(d.Reason, "status:"))
				add(fmt.Sprintf("%s受到%s影响，生命 %+d", name(d.Target), st.Name, d.Delta), &dto.CombatLogV1{Actor: name(d.Target), Action: "status", Hit: true, Damage: -d.Delta, Side: side(d.Target)})
			case d.Delta > 0:
				add(fmt.Sprintf("%s恢复了 %d 点生命", name(d.Target), d.Delta), &dto.CombatLogV1{Actor: name(d.Target), Action: "heal", Hit: true, Side: side(d.Target)})
			case d.Reason == "overheat":
				add(fmt.Sprintf("%s被过热的机体烫伤，生命 %d", name(d.Target), d.Delta), &dto.CombatLogV1{Actor: name(d.Target), Action: "overheat", Side: side(d.Target)})
			}
		case event.UnitStatusApplied:
			st := engine.StatusDef(p, d.Condition)
			add(fmt.Sprintf("%s陷入%s（%d 回合）", name(d.Target), st.Name, d.Delta), &dto.CombatLogV1{Actor: name(d.Target), Action: "status", Hit: true, Side: side(d.Target), Chips: []string{st.Name}})
		case event.UnitDefeated:
			add(name(d.Target)+"倒下了", &dto.CombatLogV1{Actor: name(d.Target), Action: "down", Side: side(d.Target)})
		case event.MechEngaged:
			add(fmt.Sprintf("%s启动了%s", name(d.Target), d.Units[0].Name), &dto.CombatLogV1{Actor: name(d.Target), Action: "mech", Hit: true, Side: side(d.Target)})
		case event.MechDisengaged:
			why := map[string]string{"mercury": p.Combat.Config.ResourceName + "耗尽，机体停机", "destroyed": "机体损毁，被迫脱离", "eject": "脱离了机体"}[d.Reason]
			add(name(d.Target)+why, &dto.CombatLogV1{Actor: name(d.Target), Action: "eject", Side: side(d.Target)})
		case event.UnitOverheated:
			add(name(d.Target)+"的机体过热！下一回合强制冷却", &dto.CombatLogV1{Actor: name(d.Target), Action: "overheat", Side: side(d.Target)})
		}
	}
	return out
}

// Notices 从本回合事件中提取需要弹出提示的通知。
func (q *Q) Notices(s *state.State, evs []event.Event) []dto.NoticeV1 {
	p := q.Pkg
	var out []dto.NoticeV1
	codex := 0
	for _, e := range evs {
		d := e.Data
		switch e.Type {
		case event.CharacterCardCreated:
			if d.Reason == "主要角色" {
				continue
			}
			out = append(out, dto.NoticeV1{Kind: "card", Text: "新角色卡：" + q.personName(s, d.Target), Ref: d.Target})
		case event.CharacterCardArchived:
			out = append(out, dto.NoticeV1{Kind: "card_archived", Text: "角色卡已归档：" + q.personName(s, d.Target), Ref: d.Target})
		case event.CharacterCardRestored:
			out = append(out, dto.NoticeV1{Kind: "card_restored", Text: q.personName(s, d.Target) + " 重新登场", Ref: d.Target})
		case event.CharacterDied:
			out = append(out, dto.NoticeV1{Kind: "death", Text: q.personName(s, d.Target) + " 死亡", Ref: d.Target})
		case event.LevelUp:
			out = append(out, dto.NoticeV1{Kind: "level", Text: fmt.Sprintf("升级！等级 %d · 属性点 +%d", d.Delta, d.Values["attr_points"])})
		case event.CodexUnlocked:
			if d.Reason != "visit" && d.Reason != "met" {
				codex++
			}
		case event.MechRevealed:
			out = append(out, dto.NoticeV1{Kind: "mech", Text: "机甲情报更新：" + p.EntityName(d.Target), Ref: d.Target})
		case event.MechStatusChanged:
			if s.MechFieldKnown(p, d.Target, combat.MechFieldStatus) {
				out = append(out, dto.NoticeV1{Kind: "mech", Text: fmt.Sprintf("%s：%s", p.EntityName(d.Target), p.Combat.Config.MechStatus(d.Key).Name), Ref: d.Target})
			}
		}
	}
	if codex > 0 {
		out = append(out, dto.NoticeV1{Kind: "codex", Text: fmt.Sprintf("图鉴新增 %d 条", codex)})
	}
	_ = p
	return out
}

// rpgChips 返回本回合成长 / 主线相关的系统条目标签。
func (q *Q) rpgChips(s *state.State, e event.Event) (string, bool) {
	p := q.Pkg
	d := e.Data
	switch e.Type {
	case event.MechPartChanged:
		verb := "装上"
		if d.Remove {
			verb = "卸下"
		}
		return fmt.Sprintf("%s %s%s", p.EntityName(d.Target), verb, p.EntityName(d.Item)), true
	case event.XPGained:
		return fmt.Sprintf("经验 +%d", d.Delta), true
	case event.LevelUp:
		return fmt.Sprintf("升到 %d 级", d.Delta), true
	case event.SkillLearned:
		if sk, ok := p.Combat.Skills[d.Skill]; ok {
			return "习得 " + sk.Name, true
		}
	case event.ItemEquipped:
		return "装备 " + p.EntityName(d.Item), true
	case event.ItemUnequipped:
		return "卸下 " + p.EntityName(d.Item), true
	case event.AttributeAllocated:
		return fmt.Sprintf("%s +%d", p.Rules.Attributes[d.Key], d.Delta), true
	case event.PlayerVitals:
		var parts []string
		if v := d.Values["hp"]; v != 0 && d.Reason != "level_up" {
			parts = append(parts, fmt.Sprintf("生命 %+d", v))
		}
		if v := d.Values["mercury"]; v != 0 {
			parts = append(parts, fmt.Sprintf("%s %+d", p.Combat.Config.ResourceName, v))
		}
		if len(parts) > 0 {
			return strings.Join(parts, " · "), true
		}
	case event.RelationEdgeChanged:
		if !d.Notable {
			return "", false
		}
		var parts []string
		for _, dv := range q.dims(d.Values) {
			parts = append(parts, fmt.Sprintf("%s %+d", dv.Name, dv.Value))
		}
		return fmt.Sprintf("%s→%s %s", q.personName(s, d.Actor), q.personName(s, d.Target), strings.Join(parts, "，")), true
	case event.RelationRevealed:
		return fmt.Sprintf("得知 %s→%s 的关系", q.personName(s, d.Actor), q.personName(s, d.Target)), true
	case event.UnitResource:
		if d.Key == "mercury" && d.Delta > 0 {
			return fmt.Sprintf("%s +%d", p.Combat.Config.ResourceName, d.Delta), true
		}
	}
	return "", false
}

// rpgStoryEntry 返回战斗开始 / 结束、主线等值得单独成行的条目。
func (q *Q) rpgStoryEntry(e event.Event) (string, bool) {
	d := e.Data
	switch e.Type {
	case event.CombatStarted:
		return "战斗开始：" + d.Title, true
	case event.CombatEnded:
		switch d.Outcome {
		case "victory":
			return "战斗胜利：" + d.Title, true
		case "defeat":
			return "战败（" + map[string]string{"captured": "被俘", "injured": "重伤", "rescued": "获救"}[d.Reason] + "）：" + d.Title, true
		}
		return "脱离战斗：" + d.Title, true
	case event.CharacterCardCreated:
		if d.Reason != "主要角色" {
			return "新角色卡：" + q.Pkg.EntityName(d.Target) + "（" + d.Reason + "）", true
		}
	case event.CharacterDied:
		return q.Pkg.EntityName(d.Target) + " 死亡", true
	}
	return "", false
}

// EncounterSuggestions 返回当前地点可挑战的遭遇战与动态节点的迎战按钮。
func (q *Q) EncounterSuggestions(s *state.State) []dto.SuggestionV1 {
	p := q.Pkg
	var out []dto.SuggestionV1
	if s.RPG != nil && s.RPG.Combat != nil {
		return nil
	}
	for _, id := range p.Combat.EncIDs {
		e := p.Combat.Encounters[id]
		if e.Available == "" || (e.Location != "" && e.Location != s.Player.Location) {
			continue
		}
		if ok, err := q.check(s, e.Available); err != nil || !ok {
			continue
		}
		label := e.Label
		if label == "" {
			label = "挑战：" + e.Title
		}
		out = append(out, dto.SuggestionV1{Label: label, Icon: "swords", Hint: e.Title, Action: dto.QuickActionV1{Kind: "combat", Action: "start", Target: id, Label: label}})
	}
	return out
}
