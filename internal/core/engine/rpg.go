package engine

import (
	"fmt"
	"slices"

	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// ---------- 成长与装备 ----------

func (w *work) execManage() error {
	p := w.pkg()
	r := w.s.R()
	w.passive = true
	switch w.cmd.Action {
	case "equip":
		it, ok := p.Items[w.cmd.Item]
		if !ok || it.Slot == "" {
			return reject("这件东西不能装备。")
		}
		if r.Combat != nil {
			return reject("战斗中无法更换装备。")
		}
		if w.s.Player.Inventory[it.ID] <= 0 {
			return reject("你身上没有%s。", it.Name)
		}
		if it.Level > w.s.PlayerLevel(p) {
			return reject("需要等级 %d 才能装备%s。", it.Level, it.Name)
		}
		if r.Equipment[it.Slot] == it.ID {
			return reject("%s已经装备着了。", it.Name)
		}
		return w.withHPKept(func() error {
			return w.emit(event.ItemEquipped, event.Data{Item: it.ID, Key: it.Slot})
		})
	case "unequip":
		slot := w.cmd.Target
		if slot == "" {
			if it, ok := p.Items[w.cmd.Item]; ok {
				slot = it.Slot
			}
		}
		if r.Combat != nil {
			return reject("战斗中无法更换装备。")
		}
		if _, ok := r.Equipment[slot]; !ok {
			return reject("这个槽位没有装备。")
		}
		return w.withHPKept(func() error {
			return w.emit(event.ItemUnequipped, event.Data{Key: slot, Item: r.Equipment[slot]})
		})
	case "allocate":
		attr := w.cmd.Target
		if _, ok := p.Rules.Attributes[attr]; !ok {
			return reject("没有这项属性。")
		}
		if r.AttrPoints <= 0 {
			return reject("没有可分配的属性点。")
		}
		return w.emit(event.AttributeAllocated, event.Data{Key: attr, Delta: 1})
	case "learn":
		sk, ok := p.Combat.Skills[w.cmd.Skill]
		if !ok || sk.Learn == nil {
			return reject("这个技能无法通过修习获得。")
		}
		if slices.Contains(r.Skills, sk.ID) {
			return reject("你已经掌握了%s。", sk.Name)
		}
		if why := LearnBlocked(p, w.s, sk); why != "" {
			return reject("%s", why)
		}
		return w.emit(event.SkillLearned, event.Data{Skill: sk.ID, Qty: sk.Learn.Cost})
	case "use":
		it, ok := p.Items[w.cmd.Item]
		if !ok || it.Combat == nil || !it.Combat.Field {
			return reject("这件东西现在用不上。")
		}
		if w.s.Player.Inventory[it.ID] <= 0 {
			return reject("你身上没有%s。", it.Name)
		}
		if r.Combat != nil {
			return reject("战斗中请使用战斗面板里的“物品”。")
		}
		hp, mx := PlayerHP(p, w.s)
		heal := min(it.Combat.Heal+mx*it.Combat.HealPct/100, mx-hp)
		merc := min(it.Combat.Mercury, p.Combat.Config.MercuryMax-r.Mercury)
		if heal <= 0 && merc <= 0 {
			return reject("现在用%s没有效果。", it.Name)
		}
		if err := w.emit(event.ItemRemoved, event.Data{Item: it.ID, Qty: 1}); err != nil {
			return err
		}
		return w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"hp": max(heal, 0), "mercury": max(merc, 0)}, Reason: it.ID})
	case "mech_install", "mech_remove":
		w.passive = true
		return w.execMechManage()
	}
	return reject("未知的操作。")
}

// withHPKept 在更换装备时保持当前生命不超过新的上限。
func (w *work) withHPKept(f func() error) error {
	if err := f(); err != nil {
		return err
	}
	hp, mx := PlayerHP(w.pkg(), w.s)
	if hp <= 0 && mx > 0 {
		return w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"hp": 1}})
	}
	return nil
}

// LearnBlocked 返回技能不能学习的原因（空串表示可以学习）。
func LearnBlocked(p *loader.Package, s *state.State, sk *combat.SkillDef) string {
	r := s.R()
	if sk.Learn == nil {
		return "无法修习"
	}
	if lv := s.PlayerLevel(p); sk.Learn.Level > lv {
		return fmt.Sprintf("需要等级 %d", sk.Learn.Level)
	}
	for _, need := range sk.Learn.Requires {
		if !slices.Contains(r.Skills, need) {
			return "需要先掌握" + p.Combat.Skills[need].Name
		}
	}
	if r.SkillPoints < sk.Learn.Cost {
		return fmt.Sprintf("需要 %d 技能点", sk.Learn.Cost)
	}
	return ""
}

// ---------- 回合末：主线 / 角色卡 / 图鉴 ----------

func (w *work) turnEvents(typ string) []event.Event {
	var out []event.Event
	for _, e := range w.events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func (w *work) runRPG() error {
	if err := w.runCards(); err != nil {
		return err
	}
	return w.runCodex()
}

// InteractionScore 是 NPC 与玩家的互动分：交谈 ×2 + 情节记忆 + 与玩家相关的关系变化次数。
func InteractionScore(s *state.State, id string) int {
	n := s.NPCs[id]
	if n == nil {
		return 0
	}
	score := n.Talks*2 + len(n.Episodes)
	if s.RPG != nil {
		for _, c := range s.RPG.RelLog {
			if (c.From == id && c.To == loader.PlayerID) || (c.To == id && c.From == loader.PlayerID) {
				score++
			}
		}
	}
	return score
}

func (w *work) runCards() error {
	p := w.pkg()
	r := w.s.R()
	talked := map[string]bool{}
	for _, e := range w.turnEvents(event.DialogueOccurred) {
		talked[e.Data.Target] = true
	}
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		card := r.Cards[id]
		n := w.s.NPCs[id]
		if n == nil || n.Location == "" {
			continue
		}
		switch {
		case card == nil && c.CardTier() == "major":
			if err := w.emit(event.CharacterCardCreated, event.Data{Target: id, Reason: "主要角色"}); err != nil {
				return err
			}
		case card == nil && c.CardTier() == "minor" && c.Promote != nil:
			hit, reason := false, c.Promote.Reason
			if c.Promote.Score > 0 && InteractionScore(w.s, id) >= c.Promote.Score {
				hit = true
				if reason == "" {
					reason = "与你往来频繁"
				}
			}
			if !hit && c.Promote.When != "" {
				ok, err := w.eng.Eval.EvalBool(c.Promote.When, w.vars(id, ""))
				if err != nil {
					return fmt.Errorf("%s promote: %w", id, err)
				}
				hit = ok
				if reason == "" {
					reason = "在故事中变得重要"
				}
			}
			if hit {
				if err := w.emit(event.CharacterCardCreated, event.Data{Target: id, Reason: reason}); err != nil {
					return err
				}
			}
		case card != nil && card.Status == state.CardArchived && talked[id]:
			if err := w.emit(event.CharacterCardRestored, event.Data{Target: id, Reason: "再次相遇"}); err != nil {
				return err
			}
		case card != nil && card.Status == state.CardActive && card.Changed == 0 && c.ArchiveWhen != "":
			ok, err := w.eng.Eval.EvalBool(c.ArchiveWhen, w.vars(id, ""))
			if err != nil {
				return fmt.Errorf("%s archive_when: %w", id, err)
			}
			if ok {
				if err := w.emit(event.CharacterCardArchived, event.Data{Target: id, Reason: "淡出了故事"}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *work) runCodex() error {
	p := w.pkg()
	r := w.s.R()
	unlock := func(id, reason string) error {
		if _, ok := r.Codex[id]; ok {
			return nil
		}
		return w.emit(event.CodexUnlocked, event.Data{Key: id, Reason: reason})
	}
	for _, id := range p.ItemIDs {
		if w.s.Player.Inventory[id] > 0 {
			if err := unlock(id, "item"); err != nil {
				return err
			}
		}
	}
	for _, id := range slices.Clone(r.Skills) {
		if err := unlock(id, "skill"); err != nil {
			return err
		}
	}
	for _, e := range w.turnEvents(event.CombatActed) {
		if e.Data.Skill != "" {
			if err := unlock(e.Data.Skill, "seen"); err != nil {
				return err
			}
		}
	}
	if err := unlock(w.s.Player.Location, "visit"); err != nil {
		return err
	}
	for _, id := range p.NPCIDs {
		if c := r.Cards[id]; (c != nil && c.Status != "") || w.s.NPCs[id].Talks > 0 {
			if err := unlock(id, "met"); err != nil {
				return err
			}
		}
	}
	for _, e := range p.Codex {
		if _, ok := r.Codex[e.ID]; ok {
			continue
		}
		ok := false
		if e.Unlock == "" {
			ok = e.Kind == "faction" || e.Kind == "lore" || e.Kind == "tech"
		} else {
			v, err := w.eng.Eval.EvalBool(e.Unlock, w.vars("", ""))
			if err != nil {
				return fmt.Errorf("codex %s: %w", e.ID, err)
			}
			ok = v
		}
		if ok {
			if err := unlock(e.ID, "codex"); err != nil {
				return err
			}
		}
	}
	return nil
}
