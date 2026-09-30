package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/story/director"

	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// applyOutcome 把内容包声明的效果翻译成事件。LLM 永远不能直接走到这里。
func (w *work) applyOutcome(o definition.Outcome, target, item string) error {
	if o.When != "" {
		ok, err := w.eng.Eval.EvalBool(o.When, w.vars(target, item))
		if err != nil {
			return fmt.Errorf("effect when: %w", err)
		}
		if !ok {
			return nil
		}
	}
	eff := o.Effect
	tgt := eff.Target
	switch tgt {
	case "target":
		tgt = target
	case "actor", "":
		tgt = loader.PlayerID
	case "scene":
		tgt = w.s.Player.Location
	}
	str := func(k string) string { return w.tmpl(fmt.Sprint(eff.Values[k]), target, item, "") }
	num := func(k string, def int) (int, error) {
		v, ok := eff.Values[k]
		if !ok {
			return def, nil
		}
		switch n := v.(type) {
		case int:
			return n, nil
		case float64:
			return int(n), nil
		case string:
			x, err := w.eng.Eval.EvalInt(n, w.vars(target, item))
			return int(x), err
		}
		return 0, fmt.Errorf("effect %s: bad number %v", eff.Type, v)
	}
	switch eff.Type {
	case "relationship_delta":
		if _, ok := w.s.NPCs[tgt]; !ok {
			return fmt.Errorf("relationship_delta: unknown npc %q", tgt)
		}
		vals := map[string]int{}
		for _, k := range []string{"trust", "fear"} {
			n, err := num(k, 0)
			if err != nil {
				return err
			}
			if n != 0 {
				vals[k] = n
			}
		}
		if len(vals) == 0 {
			return nil
		}
		return w.emit(event.RelationshipChanged, event.Data{Target: tgt, Values: vals})
	case "gold_delta":
		n, err := num("gold", 0)
		if err != nil {
			return err
		}
		if n < 0 && -n > w.s.Player.Gold {
			n = -w.s.Player.Gold
		}
		if n == 0 {
			return nil
		}
		return w.emit(event.GoldChanged, event.Data{Delta: n})
	case "item_add", "item_remove":
		id := str("item")
		if _, ok := w.pkg().Items[id]; !ok {
			return fmt.Errorf("%s: unknown item %q", eff.Type, id)
		}
		q, err := num("qty", 1)
		if err != nil {
			return err
		}
		if eff.Type == "item_add" {
			return w.emit(event.ItemAdded, event.Data{Item: id, Qty: q})
		}
		if w.s.Player.Inventory[id] < q {
			return reject("你身上没有足够的%s。", w.name(id))
		}
		return w.emit(event.ItemRemoved, event.Data{Item: id, Qty: q})
	case "scene_fact":
		fact := str("fact")
		if _, ok := w.pkg().Locations[tgt]; !ok {
			tgt = w.s.Player.Location
		}
		persist := eff.Values["persist"] == true
		if persist {
			if err := w.emit(event.SceneFactPromoted, event.Data{Location: tgt, Text: fact}); err != nil {
				return err
			}
		}
		if tgt != w.s.Player.Location {
			return nil // 不在场景中的短期事实没有意义（持久事实已进入 Canon）
		}
		return w.emit(event.SceneFactChanged, event.Data{Location: tgt, Text: fact})
	case "var_add", "var_set":
		key := str("var")
		if key == "" || strings.Contains(key, "{") {
			return fmt.Errorf("%s: missing var", eff.Type)
		}
		n, err := num("value", 0)
		if err != nil {
			return err
		}
		// 旧存档里可能还没有这个变量：语义值取包内初始值，但 Delta 相对状态中的实际值（0）计算。
		stored, ok := w.s.Vars[key]
		cur := stored
		if !ok {
			cur = w.pkg().Variables[key]
		}
		want := cur + n
		if eff.Type == "var_set" {
			want = n
		}
		delta := want - stored
		if delta == 0 {
			return nil
		}
		return w.emit(event.VarChanged, event.Data{Key: key, Delta: delta})
	case "flag_set":
		return w.emit(event.FlagSet, event.Data{Flag: str("flag"), Remove: eff.Values["value"] == false})
	case "condition_add":
		c := str("condition")
		if c == "" || strings.Contains(c, "{") {
			return nil
		}
		return w.emit(event.ConditionApplied, event.Data{Condition: c})
	case "condition_remove":
		c := str("condition")
		if !w.s.HasCondition(c) {
			return nil
		}
		return w.emit(event.ConditionRemoved, event.Data{Condition: c})
	case "npc_move":
		to := str("location")
		n, ok := w.s.NPCs[tgt]
		if !ok {
			return fmt.Errorf("npc_move: unknown npc %q", tgt)
		}
		if _, ok := w.pkg().Locations[to]; !ok {
			return fmt.Errorf("npc_move: unknown location %q", to)
		}
		return w.emit(event.NPCMoved, event.Data{Target: tgt, From: n.Location, To: to})
	case "move_player":
		to := str("location")
		if _, ok := w.pkg().Locations[to]; !ok {
			return fmt.Errorf("move_player: unknown location %q", to)
		}
		return w.movePlayer(to)
	}
	if ok, err := w.applyRPGEffect(eff.Type, tgt, str, num, eff.Values); ok {
		return err
	}
	return fmt.Errorf("unknown effect type %q", eff.Type)
}

// applyRPGEffect 处理 v0.1.1-rc2 的效果类型：战斗、成长、关系网、角色卡、主线偏离、图鉴。
func (w *work) applyRPGEffect(typ, tgt string, str func(string) string, num func(string, int) (int, error), vals map[string]any) (bool, error) {
	p := w.pkg()
	r := w.s.R()
	if ok, err := w.applyMechEffect(typ, str); ok {
		return true, err
	}
	switch typ {
	case "combat_start":
		id := str("encounter")
		if _, ok := p.Combat.Encounters[id]; !ok {
			return true, fmt.Errorf("combat_start: unknown encounter %q", id)
		}
		if r.Combat != nil {
			return true, nil
		}
		return true, w.startCombat(id)
	case "xp":
		n, err := num("amount", 0)
		if err != nil {
			return true, err
		}
		return true, w.grantXP(n, str("reason"))
	case "heal":
		n, err := num("amount", 0)
		if err != nil {
			return true, err
		}
		pct, err := num("pct", 0)
		if err != nil {
			return true, err
		}
		hp, mx := PlayerHP(p, w.s)
		amt := min(n+mx*pct/100, mx-hp)
		if amt == 0 {
			return true, nil
		}
		if amt < 0 || -amt >= hp {
			amt = max(amt, 1-hp)
		}
		return true, w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"hp": amt}})
	case "mercury":
		n, err := num("amount", 0)
		if err != nil {
			return true, err
		}
		n = combat.Clamp(r.Mercury+n, 0, p.Combat.Config.MercuryMax) - r.Mercury
		if n == 0 {
			return true, nil
		}
		return true, w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"mercury": n}})
	case "relation":
		from, to := str("from"), str("to")
		if from == "<nil>" || from == "" {
			from = tgt
		}
		if to == "<nil>" || to == "" {
			to = loader.PlayerID
		}
		if _, ok := p.Characters[from]; !ok && from != loader.PlayerID {
			return true, fmt.Errorf("relation: unknown character %q", from)
		}
		if _, ok := p.Characters[to]; !ok && to != loader.PlayerID {
			return true, fmt.Errorf("relation: unknown character %q", to)
		}
		cur := w.s.Edge(from, to)
		deltas := map[string]int{}
		for _, d := range p.Relations.Dimensions {
			n, err := num(d.ID, 0)
			if err != nil {
				return true, err
			}
			if n == 0 {
				continue
			}
			lo, hi := d.Min, d.Max
			if lo == 0 && hi == 0 {
				lo, hi = -100, 100
			}
			if v := combat.Clamp(cur[d.ID]+n, lo, hi) - cur[d.ID]; v != 0 {
				deltas[d.ID] = v
			}
		}
		if len(deltas) == 0 {
			return true, nil
		}
		reason := str("reason")
		if reason == "<nil>" {
			reason = ""
		}
		known := from == loader.PlayerID || to == loader.PlayerID || vals["reveal"] == true || w.witnessed(from, to)
		return true, w.emit(event.RelationEdgeChanged, event.Data{Actor: from, Target: to, Values: deltas, Reason: reason, Notable: known})
	case "relation_reveal":
		from, to := str("from"), str("to")
		if _, ok := p.Characters[from]; !ok {
			return true, fmt.Errorf("relation_reveal: unknown character %q", from)
		}
		src := str("source")
		if src == "<nil>" {
			src = "dialogue"
		}
		return true, w.emit(event.RelationRevealed, event.Data{Actor: from, Target: to, Values: w.s.Edge(from, to), Source: src})
	case "card_create", "card_archive", "card_restore", "npc_death":
		id := str("npc")
		if id == "<nil>" || id == "" {
			id = tgt
		}
		if _, ok := p.Characters[id]; !ok {
			return true, fmt.Errorf("%s: unknown npc %q", typ, id)
		}
		reason := str("reason")
		if reason == "<nil>" {
			reason = ""
		}
		card := r.Cards[id]
		switch typ {
		case "card_create":
			if card != nil {
				return true, nil
			}
			return true, w.emit(event.CharacterCardCreated, event.Data{Target: id, Reason: reason})
		case "card_archive":
			if card == nil || card.Status != state.CardActive {
				return true, nil
			}
			return true, w.emit(event.CharacterCardArchived, event.Data{Target: id, Reason: reason})
		case "card_restore":
			if card == nil || card.Status != state.CardArchived {
				return true, nil
			}
			return true, w.emit(event.CharacterCardRestored, event.Data{Target: id, Reason: reason})
		default:
			if card != nil && card.Status == state.CardDead {
				return true, nil
			}
			return true, w.emit(event.CharacterDied, event.Data{Target: id, Reason: reason})
		}
	case "deviation":
		n, err := num("delta", 0)
		if err != nil {
			return true, err
		}
		reason := str("reason")
		if reason == "<nil>" {
			reason = "偏离主线的举动"
		}
		w.deviation = append(w.deviation, director.Hit{Weight: n, Reason: reason})
		return true, nil
	case "learn_skill":
		id := str("skill")
		if _, ok := p.Combat.Skills[id]; !ok {
			return true, fmt.Errorf("learn_skill: unknown skill %q", id)
		}
		if slices.Contains(r.Skills, id) {
			return true, nil
		}
		return true, w.emit(event.SkillLearned, event.Data{Skill: id, Reason: "story"})
	case "codex_unlock":
		id := str("id")
		if _, ok := r.Codex[id]; ok {
			return true, nil
		}
		return true, w.emit(event.CodexUnlocked, event.Data{Key: id, Reason: "story"})
	}
	return false, nil
}

// witnessed 报告玩家是否在场目睹两名角色之间的关系变化（两人都和玩家在同一地点）。
func (w *work) witnessed(a, b string) bool {
	here := w.s.Player.Location
	at := func(id string) bool {
		if id == loader.PlayerID {
			return true
		}
		n := w.s.NPCs[id]
		return n != nil && n.Location == here
	}
	return at(a) && at(b)
}
