package engine

import (
	"fmt"
	"strings"

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
	return fmt.Errorf("unknown effect type %q", eff.Type)
}
