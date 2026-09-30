package engine

import (
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// Vars 为 CEL 构造变量（actor / target / item / scene / world / npcs）。公开给查询层复用。
func Vars(p *loader.Package, s *state.State, target, item string) expression.Vars {
	loc := p.Locations[s.Player.Location]
	conds := make([]any, 0, len(s.Player.Conditions))
	for _, c := range s.Player.Conditions {
		conds = append(conds, c)
	}
	skills := map[string]any{}
	for k, v := range s.Player.Skills {
		skills[k] = int64(v)
	}
	inv := map[string]any{}
	for k, v := range s.Player.Inventory {
		inv[k] = int64(v)
	}
	actor := map[string]any{
		"id": loader.PlayerID, "name": s.Player.Name, "gold": int64(s.Player.Gold),
		"can_speak": true, "location": s.Player.Location, "conditions": conds, "skills": skills, "inventory": inv,
	}
	tgt := map[string]any{"id": "", "key": "", "type": "none", "name": ""}
	if c, ok := p.Characters[target]; ok {
		n := s.NPCs[target]
		key := target
		if i := strings.LastIndex(target, "/"); i >= 0 {
			key = target[i+1:]
		}
		tgt = map[string]any{
			"id": target, "key": key, "type": "character", "name": c.Name(),
			"resolve": int64(c.Attributes["resolve"]), "trust": int64(n.Trust), "fear": int64(n.Fear),
			"location": n.Location, "present": n.Location == s.Player.Location,
		}
	}
	itm := map[string]any{"id": "", "name": "", "price": int64(0), "for_sale": false, "owned": int64(0), "usable": false, "condition": "", "verb": ""}
	if it, ok := p.Items[item]; ok {
		forSale := loc.Shop != nil && slices.Contains(loc.Shop.Items, item) &&
			s.NPCs[loc.Shop.Seller] != nil && s.NPCs[loc.Shop.Seller].Location == loc.ID
		itm = map[string]any{
			"id": it.ID, "name": it.Name, "price": int64(it.Price), "for_sale": forSale,
			"owned": int64(s.Player.Inventory[it.ID]), "usable": it.Use != nil, "condition": "", "verb": "",
		}
		if it.Use != nil {
			itm["condition"], itm["verb"] = it.Use.Condition, it.Use.Verb
		}
	}
	scene := map[string]any{
		"id": loc.ID, "name": loc.Name, "has_bartender": false, "has_shop": loc.Shop != nil,
		"search_dc": int64(12), "noise": int64(s.Noise[loc.ID]),
	}
	for k, v := range loc.Properties {
		switch n := v.(type) {
		case int:
			scene[k] = int64(n)
		default:
			scene[k] = v
		}
	}
	if loc.Shop != nil {
		seller := s.NPCs[loc.Shop.Seller]
		scene["has_shop"] = seller != nil && seller.Location == loc.ID
	}
	flags := map[string]any{}
	for k, v := range s.Flags {
		if v {
			flags[k] = true
		}
	}
	vars := map[string]any{}
	for k, v := range p.Variables {
		vars[k] = int64(v)
	}
	for k, v := range s.Vars {
		vars[k] = int64(v)
	}
	world := map[string]any{
		"turn": int64(s.Turn + 1), "minute": s.Minute, "day": int64(worldtime.Day(s.Minute)),
		"hour": int64(worldtime.Hour(s.Minute)), "flags": flags, "player_location": s.Player.Location, "vars": vars,
	}
	npcs := map[string]any{}
	cards := map[string]any{}
	if s.RPG != nil {
		for id, c := range s.RPG.Cards {
			cards[loader.Key(id)] = c.Status
		}
	}
	for _, id := range p.NPCIDs {
		n := s.NPCs[id]
		edge := map[string]any{}
		for k, v := range s.Edge(id, loader.PlayerID) {
			edge[k] = int64(v)
		}
		status := ""
		if s.RPG != nil && s.RPG.Cards[id] != nil {
			status = s.RPG.Cards[id].Status
		}
		npcs[id] = map[string]any{"location": n.Location, "trust": int64(n.Trust), "fear": int64(n.Fear), "name": p.Characters[id].Name(),
			"relation": edge, "card": status, "alive": n.Location != "", "interaction": int64(InteractionScore(s, id))}
	}
	rpgVars(p, s, actor, world)
	world["cards"] = cards
	return expression.Vars{"actor": actor, "target": tgt, "item": itm, "scene": scene, "world": world, "npcs": npcs, "stories": Stories(p, s)}
}

// rpgVars 加入成长 / 战斗 / 主线变量：actor.level/xp/hp/max_hp/mercury/in_combat/equipment/combat_skills，
// world.combats（遭遇短名 → {result, wins}）、world.mode、world.deviation、world.anchor。
func rpgVars(p *loader.Package, s *state.State, actor, world map[string]any) {
	hp, mx := PlayerHP(p, s)
	actor["level"] = int64(s.PlayerLevel(p))
	actor["hp"], actor["max_hp"] = int64(hp), int64(mx)
	eq := map[string]any{}
	skills := []any{}
	combats := map[string]any{}
	xp, merc, inCombat := 0, 0, false
	mode, dev, anchor := state.ModeMain, 0, ""
	if r := s.RPG; r != nil {
		xp, merc, inCombat = r.XP, r.Mercury, r.Combat != nil
		for k, v := range r.Equipment {
			eq[k] = v
		}
		for _, sk := range r.Skills {
			skills = append(skills, sk)
		}
		for id, rec := range r.Encounters {
			combats[loader.Key(id)] = map[string]any{"result": rec.Result, "wins": int64(rec.Wins), "losses": int64(rec.Losses)}
		}
		mode, dev = r.Main.CurrentMode(), r.Main.Deviation
	}
	if a := CurrentAnchor(p, s); a != nil {
		anchor = a.ID
	}
	for _, id := range p.Combat.EncIDs {
		k := loader.Key(id)
		if _, ok := combats[k]; !ok {
			combats[k] = map[string]any{"result": "", "wins": int64(0), "losses": int64(0)}
		}
	}
	actor["xp"], actor["mercury"], actor["in_combat"], actor["equipment"], actor["combat_skills"] = int64(xp), int64(merc), inCombat, eq, skills
	world["combats"], world["mode"], world["deviation"], world["anchor"] = combats, mode, int64(dev), anchor
}

// Stories 按故事短名（demo:story/lost_purse → lost_purse）构造全部故事状态，供 CEL 使用。
func Stories(p *loader.Package, s *state.State) map[string]any {
	out := map[string]any{}
	for _, id := range p.StoryIDs {
		st := s.Stories[id]
		if st == nil {
			st = &state.Story{Status: state.StoryInactive}
		}
		steps := make([]any, 0, len(st.Steps))
		for _, x := range st.Steps {
			steps = append(steps, x)
		}
		elapsed := int64(0)
		if st.Status == state.StoryActive {
			elapsed = s.Minute - st.StartedMinute
		}
		out[loader.Key(id)] = map[string]any{
			"id": id, "title": p.Stories[id].Title, "status": st.Status, "outcome": st.Outcome,
			"steps": steps, "elapsed_minutes": elapsed,
		}
	}
	return out
}

// NPCVars 构造台词条件中的 npc 变量：说话者的关系、对话记忆与情节记忆（NPCScope：只含该 NPC 自己知道的事）。
func NPCVars(p *loader.Package, s *state.State, id string) map[string]any {
	n := s.NPCs[id]
	c := p.Characters[id]
	if n == nil || c == nil {
		return map[string]any{}
	}
	said := []any{}
	for k := range n.Said {
		said = append(said, k)
	}
	slices.SortFunc(said, func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
	topics := map[string]any{}
	for k, v := range n.Topics {
		topics[k] = int64(v)
	}
	memories := []any{}
	for _, e := range n.Episodes {
		memories = append(memories, e.Key)
	}
	seen, seenAll := []any{}, []any{}
	for _, m := range n.Memories {
		if m.Action == "" {
			continue
		}
		k := loader.Key(m.Action)
		seenAll = append(seenAll, k)
		if n.Talks == 0 || m.Turn > n.LastTalkTurn {
			seen = append(seen, k)
		}
	}
	lastTopic, lastLine := "", ""
	if ex := n.LastExchange(); ex != nil {
		lastTopic, lastLine = ex.Topic, ex.Line
	}
	turnsSince, minutesSince := int64(-1), int64(-1)
	if n.Talks > 0 {
		turnsSince = int64(s.Turn + 1 - n.LastTalkTurn)
		minutesSince = s.Minute - n.LastTalkMinute
	}
	return map[string]any{
		"id": id, "key": loader.Key(id), "name": c.Name(), "trust": int64(n.Trust), "fear": int64(n.Fear),
		"attitude": n.Attitude(), "present": n.Location == s.Player.Location, "location": n.Location,
		"talks": int64(n.Talks), "turns_since_talk": turnsSince, "minutes_since_talk": minutesSince,
		"last_topic": lastTopic, "last_line": lastLine, "said": said, "topics": topics,
		"memories": memories, "seen": seen, "seen_all": seenAll,
	}
}

func (w *work) vars(target, item string) expression.Vars { return Vars(w.pkg(), w.s, target, item) }
