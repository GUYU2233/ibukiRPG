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
	actor := map[string]any{
		"id": loader.PlayerID, "name": s.Player.Name, "gold": int64(s.Player.Gold),
		"can_speak": true, "location": s.Player.Location, "conditions": conds, "skills": skills,
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
	world := map[string]any{
		"turn": int64(s.Turn + 1), "minute": s.Minute, "day": int64(worldtime.Day(s.Minute)),
		"hour": int64(worldtime.Hour(s.Minute)), "flags": flags, "player_location": s.Player.Location,
	}
	npcs := map[string]any{}
	for _, id := range p.NPCIDs {
		n := s.NPCs[id]
		npcs[id] = map[string]any{"location": n.Location, "trust": int64(n.Trust), "fear": int64(n.Fear), "name": p.Characters[id].Name()}
	}
	return expression.Vars{"actor": actor, "target": tgt, "item": itm, "scene": scene, "world": world, "npcs": npcs}
}

func (w *work) vars(target, item string) expression.Vars { return Vars(w.pkg(), w.s, target, item) }
