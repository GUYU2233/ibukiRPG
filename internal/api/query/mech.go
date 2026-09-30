package query

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// Unknown 是未知字段的显示文字。
const Unknown = "未知"

// mechPartStats 是改装件 / 挂载武器卡上的机甲属性。
func (q *Q) mechPartStats(it *loader.Item) []dto.KVV1 {
	if it.Mech == nil {
		return nil
	}
	cfg := &q.Pkg.Combat.Config
	var out []dto.KVV1
	pos := "改装槽"
	if it.IsMechWeapon() {
		pos = "武器挂点"
	}
	if it.Mech.Slot != "" {
		pos += "（" + it.Mech.Slot + "）"
	}
	out = append(out, dto.KVV1{Label: "安装位置", Value: pos})
	for _, k := range combat.StatKeys {
		if v := it.Mech.Stats[k]; v != 0 {
			out = append(out, dto.KVV1{Label: "机体" + combat.StatNames[k], Value: fmt.Sprintf("%+d", v)})
		}
	}
	for _, sp := range cfg.MechSpecs {
		if v := it.Mech.Specs[sp.ID]; v != 0 {
			out = append(out, dto.KVV1{Label: sp.Name, Value: fmt.Sprintf("%+d%s", v, sp.Unit)})
		}
	}
	if it.Mech.HeatMax != 0 {
		out = append(out, dto.KVV1{Label: "过热上限", Value: fmt.Sprintf("%+d", it.Mech.HeatMax)})
	}
	if it.Mech.PerTurn != 0 {
		out = append(out, dto.KVV1{Label: "每回合消耗", Value: fmt.Sprintf("%+d %s", it.Mech.PerTurn, cfg.ResourceName)})
	}
	for _, sk := range it.Mech.Skills {
		out = append(out, dto.KVV1{Label: "提供技能", Value: q.Pkg.EntityName(sk)})
	}
	if len(it.Mech.Mechs) > 0 {
		var names []string
		for _, m := range it.Mech.Mechs {
			names = append(names, q.Pkg.EntityName(m))
		}
		out = append(out, dto.KVV1{Label: "适配机体", Value: strings.Join(names, "、")})
	}
	return out
}

// gateMechCard 对图鉴中的机甲介绍卡做知识遮蔽。
func (q *Q) gateMechCard(s *state.State, c *dto.CardV1) {
	if !c.Known || q.Pkg.Combat == nil || q.Pkg.Combat.Mechs[c.ID] == nil {
		return
	}
	if !s.MechFieldKnown(q.Pkg, c.ID, combat.MechFieldStats) {
		c.Stats = []dto.KVV1{{Label: "数值", Value: Unknown}}
	}
	if !s.MechFieldKnown(q.Pkg, c.ID, combat.MechFieldLore) {
		c.Lore = ""
	}
	c.Tags = append(c.Tags, "mech_card")
}

// Mech 构造一张机械甲胄卡（按玩家知识逐字段遮蔽）。
func (q *Q) Mech(s *state.State, id string) (dto.MechCardV1, bool) {
	p := q.Pkg
	if p.Combat == nil {
		return dto.MechCardV1{}, false
	}
	m := p.Combat.Mechs[id]
	if m == nil {
		return dto.MechCardV1{}, false
	}
	cfg := &p.Combat.Config
	v := s.MechOf(p, id)
	_, unlocked := s.R().Codex[id]
	known := func(f string) bool { return s.MechFieldKnown(p, id, f) }
	c := dto.MechCardV1{ID: id, Icon: m.Icon, Known: unlocked, Enemy: m.Enemy, Owned: id == engine.PlayerMech(p),
		Fields: []dto.MechFieldV1{}, Specs: []dto.MechStatV1{}, Stats: []dto.MechStatV1{}, Energy: []dto.MechFieldV1{},
		Slots: []dto.MechSlotV1{}, Hardpoints: []dto.MechSlotV1{}, Skills: []dto.CardV1{}, History: []dto.MechLogV1{}}
	if c.Icon == "" {
		c.Icon = "precision_manufacturing"
	}
	r := cfg.RarityOf(m.Rarity)
	c.Rarity, c.RarityName, c.RarityColor = r.ID, r.Name, r.Color
	if !unlocked {
		c.Name, c.Description = "？？？", "尚未解锁。"
		c.Status = dto.MechStatusV1{ID: "", Name: Unknown, Tone: "neutral"}
		return c, true
	}
	c.Name, c.Description = m.Name, m.Description
	c.Portrait = m.Portrait != "" && known(combat.MechFieldImage)
	unknown := 0
	field := func(fid, label, val string) {
		k := known(fid)
		if val == "" {
			val = "—"
		}
		if !k {
			val = Unknown
			unknown++
		}
		c.Fields = append(c.Fields, dto.MechFieldV1{ID: fid, Label: label, Value: val, Known: k})
	}
	field(combat.MechFieldModel, "型号", m.Model)
	field(combat.MechFieldMaker, "制造方", m.Maker)
	c.Fields = append(c.Fields, dto.MechFieldV1{ID: "class", Label: "分类", Value: orDash(m.Class), Known: true})
	pilot := ""
	if v.Pilot != "" {
		pilot = q.personName(s, v.Pilot)
	}
	field(combat.MechFieldPilot, "驾驶者", pilot)
	if known(combat.MechFieldPilot) {
		c.PilotID = v.Pilot
	}
	st := cfg.MechStatus(v.Status)
	c.Status = dto.MechStatusV1{ID: st.ID, Name: st.Name, Tone: st.Tone, Engage: st.Engage, Known: known(combat.MechFieldStatus)}
	if !c.Status.Known {
		c.Status = dto.MechStatusV1{Name: Unknown, Tone: "neutral"}
		unknown++
	}
	form, _ := engine.MechForm(p, s, id)
	for _, sp := range cfg.MechSpecs {
		base, ok := m.Specs[sp.ID]
		if !ok {
			continue
		}
		k := known(combat.MechFieldSpec + sp.ID)
		ms := dto.MechStatV1{ID: sp.ID, Name: sp.Name, Max: sp.Max, Unit: sp.Unit, Known: k}
		if k {
			ms.Value, ms.Bonus = form.Specs[sp.ID], form.Specs[sp.ID]-base
		} else {
			unknown++
		}
		c.Specs = append(c.Specs, ms)
	}
	c.StatsKnown = known(combat.MechFieldStats)
	if !c.StatsKnown {
		unknown++
	}
	maxes := map[string]int{combat.HP: 400, combat.ATK: 60, combat.DEF: 60, combat.SPD: 20, combat.ACC: 120, combat.EVA: 40, combat.CRIT: 50}
	for _, k := range []string{combat.HP, combat.ATK, combat.DEF, combat.SPD, combat.ACC, combat.EVA, combat.CRIT} {
		if _, ok := m.Stats[k]; !ok && k != combat.ACC && k != combat.EVA && k != combat.CRIT {
			continue
		}
		ms := dto.MechStatV1{ID: k, Name: combat.StatNames[k], Max: max(maxes[k], form.Stats[k]), Known: c.StatsKnown}
		if c.StatsKnown {
			ms.Value = form.Stats[k]
			ms.Bonus = form.Stats[k] - m.Stats[k]
			if _, ok := m.Stats[k]; !ok {
				ms.Bonus = 0
			}
		}
		c.Stats = append(c.Stats, ms)
	}
	if c.StatsKnown && !m.Enemy {
		c.Energy = append(c.Energy,
			dto.MechFieldV1{ID: "engage", Label: "启动消耗", Value: fmt.Sprintf("%d %s", m.EngageCost, cfg.ResourceName), Known: true},
			dto.MechFieldV1{ID: "per_turn", Label: "每回合消耗", Value: fmt.Sprintf("%d %s", form.PerTurn, cfg.ResourceName), Known: true},
			dto.MechFieldV1{ID: "heat", Label: "过热上限", Value: fmt.Sprint(form.HeatMax), Known: true})
	}
	slotView := func(sl combat.MechSlotDef, hardpoint bool) dto.MechSlotV1 {
		f := combat.MechFieldSlot + sl.ID
		cur := v.Installed[sl.ID]
		key := sl.ID
		if hardpoint {
			f = combat.MechFieldHardpoint + sl.ID
			cur = v.Mounted[sl.ID]
			key = "hp:" + sl.ID
		}
		sv := dto.MechSlotV1{ID: key, Name: sl.Name, Kind: sl.Kind, Hardpoint: hardpoint, Known: known(f)}
		if !sv.Known {
			unknown++
			return sv
		}
		if cur != "" {
			pc, _ := q.autoCard(cur)
			pc.Known = true
			pc.Stats = q.mechPartStats(p.Items[cur])
			sv.Part = &pc
		}
		if c.Owned && v.Status != "lost" && v.Status != "sealed" {
			if cur != "" {
				sv.Remove = &dto.QuickActionV1{Kind: "manage", Action: "mech_remove", Target: key, Skill: id, Label: "卸下" + p.EntityName(cur)}
			}
			for _, it := range p.ItemIDs {
				item := p.Items[it]
				if s.Player.Inventory[it] <= 0 || item.Mech == nil || it == cur {
					continue
				}
				if loader.MechFits(m, sl, item, hardpoint) == "" {
					sv.Install = append(sv.Install, dto.QuickActionV1{Kind: "manage", Action: "mech_install", Target: key, Item: it, Skill: id, Label: "安装" + item.Name})
				}
			}
		}
		return sv
	}
	for _, sl := range m.ModSlots {
		c.Slots = append(c.Slots, slotView(sl, false))
	}
	for _, sl := range m.Hardpoints {
		c.Hardpoints = append(c.Hardpoints, slotView(sl, true))
	}
	for _, sk := range form.Skills {
		sc, _ := q.autoCard(sk)
		sc.Known = c.StatsKnown || known(combat.MechFieldSkill+sk)
		if slices.Contains(m.Hidden, combat.MechFieldSkill+sk) {
			sc.Known = known(combat.MechFieldSkill + sk)
		}
		if !sc.Known {
			sc.Name, sc.Description, sc.Lore, sc.Stats = "？？？", Unknown, "", nil
			unknown++
		}
		c.Skills = append(c.Skills, sc)
	}
	c.LoreKnown = known(combat.MechFieldLore)
	if c.LoreKnown {
		c.Lore = m.Lore
	} else if m.Lore != "" {
		unknown++
	}
	for i := len(v.History) - 1; i >= 0; i-- {
		h := v.History[i]
		c.History = append(c.History, dto.MechLogV1{Turn: h.Turn, Time: worldtime.Format(h.Minute), Text: q.mechLogText(s, m, h)})
	}
	c.Unknown = unknown
	return c, true
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func (q *Q) mechSlotName(m *combat.MechDef, key string) string {
	if sl, _, ok := loader.MechSlot(m, strings.TrimPrefix(key, "hp:")); ok {
		return sl.Name
	}
	return key
}

func (q *Q) mechLogText(s *state.State, m *combat.MechDef, h state.MechLog) string {
	p := q.Pkg
	reason := ""
	if h.Reason != "" {
		reason = "（" + h.Reason + "）"
	}
	switch h.Type {
	case event.MechStatusChanged:
		return "状态变为「" + p.Combat.Config.MechStatus(h.Key).Name + "」" + reason
	case event.MechPartChanged:
		if h.Remove {
			return fmt.Sprintf("从%s卸下了%s%s", q.mechSlotName(m, h.Key), p.EntityName(h.Item), reason)
		}
		return fmt.Sprintf("在%s装上了%s%s", q.mechSlotName(m, h.Key), p.EntityName(h.Item), reason)
	case event.MechRevealed:
		return "你了解到了新的情报" + reason
	case event.MechPilotChanged:
		if h.Actor == "" {
			return "驾驶席空了出来" + reason
		}
		return "驾驶者变为" + q.personName(s, h.Actor) + reason
	}
	return h.Type
}

// Mechs 返回全部机甲卡（未解锁的显示为 ？？？）。
func (q *Q) Mechs(s *state.State) dto.MechsV1 {
	v := dto.MechsV1{Mechs: []dto.MechCardV1{}}
	if q.Pkg.Combat == nil {
		return v
	}
	v.ResourceName = q.Pkg.Combat.Config.ResourceName
	for _, id := range q.Pkg.Combat.MechIDs {
		if c, ok := q.Mech(s, id); ok {
			v.Mechs = append(v.Mechs, c)
		}
	}
	return v
}

// pilotedMechs 返回玩家已知由某角色驾驶的机甲。
func (q *Q) pilotedMechs(s *state.State, who string) []string {
	var out []string
	if q.Pkg.Combat == nil {
		return nil
	}
	for _, id := range q.Pkg.Combat.MechIDs {
		if s.MechOf(q.Pkg, id).Pilot == who && s.MechFieldKnown(q.Pkg, id, combat.MechFieldPilot) {
			out = append(out, id)
		}
	}
	return out
}
