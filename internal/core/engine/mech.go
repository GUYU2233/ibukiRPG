package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// MechFormStats 是机甲在当前改装 / 状态下的战斗形态。
type MechFormStats struct {
	Stats   combat.Stats
	Specs   map[string]int
	Skills  []string
	HeatMax int
	PerTurn int
	Status  combat.MechStatusDef
}

// MechForm 计算机甲的有效数值：基础数值 + 改装件 / 挂载武器修正，再乘以状态百分比。
func MechForm(p *loader.Package, s *state.State, id string) (MechFormStats, bool) {
	if p.Combat == nil {
		return MechFormStats{}, false
	}
	m := p.Combat.Mechs[id]
	if m == nil {
		return MechFormStats{}, false
	}
	v := s.MechOf(p, id)
	f := MechFormStats{Stats: combat.Stats{combat.ACC: 85, combat.EVA: 5, combat.CRIT: 5}.Add(nil), Specs: map[string]int{},
		Skills: slices.Clone(m.Skills), HeatMax: m.HeatMax, PerTurn: m.PerTurn, Status: p.Combat.Config.MechStatus(v.Status)}
	for k, x := range m.Stats {
		f.Stats[k] = x
	}
	for k, x := range m.Specs {
		f.Specs[k] = x
	}
	parts := make([]string, 0, len(v.Installed)+len(v.Mounted))
	for _, sl := range m.ModSlots {
		if it := v.Installed[sl.ID]; it != "" {
			parts = append(parts, it)
		}
	}
	for _, sl := range m.Hardpoints {
		if it := v.Mounted[sl.ID]; it != "" {
			parts = append(parts, it)
		}
	}
	for _, it := range parts {
		item := p.Items[it]
		if item == nil || item.Mech == nil {
			continue
		}
		f.Stats = f.Stats.Add(item.Mech.Stats)
		for k, x := range item.Mech.Specs {
			f.Specs[k] += x
		}
		for _, sk := range item.Mech.Skills {
			if !slices.Contains(f.Skills, sk) {
				f.Skills = append(f.Skills, sk)
			}
		}
		f.HeatMax += item.Mech.HeatMax
		f.PerTurn = max(f.PerTurn+item.Mech.PerTurn, 0)
	}
	if pct := f.Status.StatPct; pct > 0 && pct != 100 {
		for _, k := range []string{combat.HP, combat.ATK, combat.DEF, combat.SPD} {
			f.Stats[k] = max(f.Stats[k]*pct/100, 1)
		}
	}
	return f, true
}

// mechUseBlocked 返回机甲当前不能启动的原因。
func mechUseBlocked(p *loader.Package, s *state.State, id string) string {
	f, ok := MechForm(p, s, id)
	if !ok {
		return "没有这台机甲"
	}
	if !f.Status.Engage {
		return fmt.Sprintf("%s处于「%s」状态，无法启动", p.Combat.Mechs[id].Name, f.Status.Name)
	}
	return ""
}

// PlayerMech 返回玩家可驾驶的机甲 ID（可能为空）。
func PlayerMech(p *loader.Package) string {
	if !p.HasCombat() {
		return ""
	}
	return p.PlayerCombat().Mech
}

// revealMech 揭示机甲字段（只揭示仍然未知的字段）。
func (w *work) revealMech(id string, fields []string, source string) error {
	p := w.pkg()
	m := p.Combat.Mechs[id]
	if m == nil {
		return nil
	}
	if _, ok := w.s.R().Codex[id]; !ok {
		if err := w.emit(event.CodexUnlocked, event.Data{Key: id, Reason: "mech"}); err != nil {
			return err
		}
	}
	var fresh []string
	v := w.s.MechOf(p, id)
	for _, f := range fields {
		if f == "*" {
			if _, ok := v.Known["*"]; !ok {
				fresh = append(fresh, "*")
			}
			continue
		}
		if !slices.Contains(m.Hidden, f) {
			continue
		}
		if _, ok := v.Known[f]; ok {
			continue
		}
		if _, ok := v.Known["*"]; ok {
			continue
		}
		fresh = append(fresh, f)
	}
	if len(fresh) == 0 {
		return nil
	}
	return w.emit(event.MechRevealed, event.Data{Target: id, Tags: fresh, Source: source})
}

// pilotReveal 是亲自驾驶后得知的字段：数值、规格、状态、技能与全部槽位。
func pilotReveal(m *combat.MechDef) []string {
	out := []string{combat.MechFieldStats, combat.MechFieldStatus, combat.MechFieldPilot}
	for _, h := range m.Hidden {
		if strings.HasPrefix(h, combat.MechFieldSpec) || strings.HasPrefix(h, combat.MechFieldSlot) ||
			strings.HasPrefix(h, combat.MechFieldHardpoint) || strings.HasPrefix(h, combat.MechFieldSkill) {
			out = append(out, h)
		}
	}
	return out
}

// execMechManage 处理改装：manage mech_install（Target=槽位，Item=部件）/ mech_remove（Target=槽位）。
// 槽位写作 "<slot>"（改装槽）或 "hp:<slot>"（武器挂点）；Skill 可指定机甲 ID（默认玩家机甲）。
func (w *work) execMechManage() error {
	p := w.pkg()
	id := w.cmd.Skill
	if id == "" {
		id = PlayerMech(p)
	}
	m := p.Combat.Mechs[id]
	if m == nil || m.Enemy || id != PlayerMech(p) {
		return reject("你没有可以改装的机甲。")
	}
	if w.s.R().Combat != nil {
		return reject("战斗中无法改装。")
	}
	v := w.s.MechOf(p, id)
	st := p.Combat.Config.MechStatus(v.Status)
	if v.Status == "lost" || v.Status == "sealed" {
		return reject("%s处于「%s」状态，无法改装。", m.Name, st.Name)
	}
	slotID := strings.TrimPrefix(w.cmd.Target, "hp:")
	slot, hardpoint, ok := loader.MechSlot(m, slotID)
	if !ok {
		return reject("%s没有这个槽位。", m.Name)
	}
	field := combat.MechFieldSlot + slotID
	if hardpoint {
		field = combat.MechFieldHardpoint + slotID
	}
	if !w.s.MechFieldKnown(p, id, field) {
		return reject("你还不了解这个槽位。")
	}
	key := slotID
	cur := v.Installed[slotID]
	if hardpoint {
		key = "hp:" + slotID
		cur = v.Mounted[slotID]
	}
	switch w.cmd.Action {
	case "mech_remove":
		if cur == "" {
			return reject("%s是空的。", slot.Name)
		}
		if err := w.emit(event.MechPartChanged, event.Data{Target: id, Key: key, Item: cur, Remove: true, Reason: "卸下"}); err != nil {
			return err
		}
		return w.emit(event.ItemAdded, event.Data{Item: cur, Qty: 1})
	case "mech_install":
		item := p.Items[w.cmd.Item]
		if item == nil || w.s.Player.Inventory[item.ID] <= 0 {
			return reject("你没有这个部件。")
		}
		if why := loader.MechFits(m, slot, item, hardpoint); why != "" {
			return reject("%s", why)
		}
		if cur == item.ID {
			return reject("已经装上了。")
		}
		if err := w.emit(event.ItemRemoved, event.Data{Item: item.ID, Qty: 1}); err != nil {
			return err
		}
		if cur != "" {
			if err := w.emit(event.ItemAdded, event.Data{Item: cur, Qty: 1}); err != nil {
				return err
			}
		}
		if _, ok := w.s.R().Codex[item.ID]; !ok {
			if err := w.emit(event.CodexUnlocked, event.Data{Key: item.ID, Reason: "item"}); err != nil {
				return err
			}
		}
		return w.emit(event.MechPartChanged, event.Data{Target: id, Key: key, Item: item.ID, Reason: "安装"})
	}
	return reject("未知的改装操作。")
}

// applyMechEffect 处理故事包的机甲效果。
func (w *work) applyMechEffect(typ string, str func(string) string) (bool, error) {
	p := w.pkg()
	switch typ {
	case "mech_status", "mech_reveal", "mech_install", "mech_remove", "mech_pilot":
	default:
		return false, nil
	}
	id := str("mech")
	m := p.Combat.Mechs[id]
	if m == nil {
		return true, fmt.Errorf("%s: unknown mech %q", typ, id)
	}
	v := w.s.MechOf(p, id)
	switch typ {
	case "mech_status":
		st := str("status")
		if !p.Combat.Config.HasMechStatus(st) {
			return true, fmt.Errorf("mech_status: unknown status %q", st)
		}
		if v.Status == st {
			return true, nil
		}
		return true, w.emit(event.MechStatusChanged, event.Data{Target: id, Key: st, Reason: str("reason")})
	case "mech_reveal":
		var fields []string
		for _, f := range strings.Split(str("fields"), ",") {
			if f = strings.TrimSpace(f); f != "" {
				fields = append(fields, f)
			}
		}
		if len(fields) == 0 {
			fields = []string{"*"}
		}
		return true, w.revealMech(id, fields, str("source"))
	case "mech_install", "mech_remove":
		slotID := strings.TrimPrefix(str("slot"), "hp:")
		slot, hardpoint, ok := loader.MechSlot(m, slotID)
		if !ok {
			return true, fmt.Errorf("%s: unknown slot %q", typ, slotID)
		}
		key := slotID
		if hardpoint {
			key = "hp:" + slotID
		}
		if typ == "mech_remove" {
			cur := v.Installed[slotID]
			if hardpoint {
				cur = v.Mounted[slotID]
			}
			if cur == "" {
				return true, nil
			}
			return true, w.emit(event.MechPartChanged, event.Data{Target: id, Key: key, Item: cur, Remove: true, Reason: str("reason")})
		}
		item := p.Items[str("item")]
		if item == nil {
			return true, fmt.Errorf("mech_install: unknown item %q", str("item"))
		}
		if why := loader.MechFits(m, slot, item, hardpoint); why != "" {
			return true, fmt.Errorf("mech_install: %s", why)
		}
		return true, w.emit(event.MechPartChanged, event.Data{Target: id, Key: key, Item: item.ID, Reason: str("reason")})
	case "mech_pilot":
		pilot := str("pilot")
		if pilot != "" && p.Character(pilot) == nil {
			return true, fmt.Errorf("mech_pilot: unknown character %q", pilot)
		}
		if v.Pilot == pilot {
			return true, nil
		}
		return true, w.emit(event.MechPilotChanged, event.Data{Target: id, Actor: pilot, Reason: str("reason")})
	}
	return true, nil
}
