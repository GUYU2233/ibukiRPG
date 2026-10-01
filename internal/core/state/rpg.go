package state

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// 角色卡状态。
const (
	CardActive   = "active"
	CardArchived = "archived"
	CardDead     = "dead"
)

// 主线模式。
const (
	ModeMain    = "main"
	ModeFree    = "free"
	ModeSandbox = "sandbox"
)

// MaxRelLog 是关系变化历史的保留条数。
const MaxRelLog = 160

// RPG 是 v0.1.1-rc2 引入的角色扮演状态：成长、装备、战斗、图鉴、关系网、角色卡与主线贴合度。
// 旧存档没有这一段（nil），读取时按零值补齐——零值的含义都经过设计（例如 Wounds=0 表示满血）。
type RPG struct {
	Level       int `json:"level,omitempty"`
	XP          int `json:"xp,omitempty"`
	AttrPoints  int `json:"attr_points,omitempty"`
	SkillPoints int `json:"skill_points,omitempty"`
	// Wounds 是已损失的生命（当前生命 = 最大生命 - Wounds），升级时伤势保留。
	Wounds    int               `json:"wounds,omitempty"`
	Mercury   int               `json:"mercury,omitempty"`
	Equipment map[string]string `json:"equipment,omitempty"`
	Skills    []string          `json:"skills,omitempty"`
	Codex     map[string]int    `json:"codex,omitempty"`
	// Relations：from>to → 维度 → 数值（NPC→玩家 的 trust / fear 仍存于 NPC 结构）。
	Relations map[string]map[string]int `json:"relations,omitempty"`
	// Known：玩家所知道的 NPC↔NPC 关系（观察 / 听说时的快照，可能已经过时）。
	Known      map[string]*KnownEdge       `json:"known,omitempty"`
	RelLog     []RelChange                 `json:"rel_log,omitempty"`
	Cards      map[string]*Card            `json:"cards,omitempty"`
	Combat     *Combat                     `json:"combat,omitempty"`
	Encounters map[string]*EncounterRecord `json:"encounters,omitempty"`
	// Mechs 是机甲卡的运行时状态（只存与故事包定义不同的部分）。
	Mechs map[string]*MechState `json:"mechs,omitempty"`
}

// MechState 是一台机甲的运行时状态。Installed / Mounted 覆盖故事包的初始安装（值为空串表示已卸下）。
type MechState struct {
	Status    string            `json:"status,omitempty"`
	Installed map[string]string `json:"installed,omitempty"`
	Mounted   map[string]string `json:"mounted,omitempty"`
	// Pilot 覆盖驾驶者（"-" 表示无人驾驶）。
	Pilot string `json:"pilot,omitempty"`
	// Known：玩家已经得知的字段 → 回合（"*" 表示全部）。
	Known   map[string]int `json:"known,omitempty"`
	History []MechLog      `json:"history,omitempty"`
}

// MechLog 是机甲履历中的一条。
type MechLog struct {
	Turn   int    `json:"turn"`
	Minute int64  `json:"minute"`
	Type   string `json:"type"`
	Key    string `json:"key,omitempty"`
	Item   string `json:"item,omitempty"`
	Actor  string `json:"actor,omitempty"`
	Remove bool   `json:"remove,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// MaxMechLog 是每台机甲保留的履历条数。
const MaxMechLog = 60

// Mech 返回（必要时创建）机甲运行时状态。
func (r *RPG) Mech(id string) *MechState {
	if r.Mechs == nil {
		r.Mechs = map[string]*MechState{}
	}
	m := r.Mechs[id]
	if m == nil {
		m = &MechState{}
		r.Mechs[id] = m
	}
	if m.Installed == nil {
		m.Installed = map[string]string{}
	}
	if m.Mounted == nil {
		m.Mounted = map[string]string{}
	}
	if m.Known == nil {
		m.Known = map[string]int{}
	}
	return m
}

// MechView 是机甲的有效状态（故事包定义 + 运行时覆盖），只读。
type MechView struct {
	Status    string
	Pilot     string
	Installed map[string]string
	Mounted   map[string]string
	Known     map[string]int
	History   []MechLog
}

// MechOf 计算机甲的有效状态。
func (s *State) MechOf(p *loader.Package, id string) MechView {
	v := MechView{Installed: map[string]string{}, Mounted: map[string]string{}, Known: map[string]int{}}
	if p.Combat == nil {
		return v
	}
	d := p.Combat.Mechs[id]
	if d == nil {
		return v
	}
	v.Status, v.Pilot = d.Status, d.Pilot
	if v.Status == "" {
		v.Status = "ready"
	}
	for k, it := range d.Installed {
		v.Installed[k] = it
	}
	for k, it := range d.Mounted {
		v.Mounted[k] = it
	}
	if s.RPG != nil && s.RPG.Mechs != nil {
		if m := s.RPG.Mechs[id]; m != nil {
			if m.Status != "" {
				v.Status = m.Status
			}
			switch m.Pilot {
			case "":
			case "-":
				v.Pilot = ""
			default:
				v.Pilot = m.Pilot
			}
			for k, it := range m.Installed {
				if it == "" {
					delete(v.Installed, k)
				} else {
					v.Installed[k] = it
				}
			}
			for k, it := range m.Mounted {
				if it == "" {
					delete(v.Mounted, k)
				} else {
					v.Mounted[k] = it
				}
			}
			for k, t := range m.Known {
				v.Known[k] = t
			}
			v.History = m.History
		}
	}
	return v
}

// MechFieldKnown 报告玩家是否知道机甲的某个字段：卡片已解锁，且字段不在 hidden 中或已被揭示。
func (s *State) MechFieldKnown(p *loader.Package, id, field string) bool {
	if p.Combat == nil || p.Combat.Mechs[id] == nil {
		return false
	}
	if _, ok := s.R().Codex[id]; !ok {
		return false
	}
	if !slices.Contains(p.Combat.Mechs[id].Hidden, field) {
		return true
	}
	v := s.MechOf(p, id)
	if _, ok := v.Known["*"]; ok {
		return true
	}
	_, ok := v.Known[field]
	return ok
}

// KnownEdge 是玩家对一段关系的认知。
type KnownEdge struct {
	Values map[string]int `json:"values"`
	Turn   int            `json:"turn"`
	Source string         `json:"source"` // public / witness / dialogue / reveal
}

// RelChange 是关系变化历史中的一条。
type RelChange struct {
	Turn   int            `json:"turn"`
	Minute int64          `json:"minute"`
	From   string         `json:"from"`
	To     string         `json:"to"`
	Values map[string]int `json:"values"`
	Reason string         `json:"reason,omitempty"`
	Known  bool           `json:"known,omitempty"`
}

// Card 是一张角色卡。
type Card struct {
	Status  string `json:"status"`
	Created int    `json:"created"`
	Reason  string `json:"reason,omitempty"`
	Changed int    `json:"changed,omitempty"`
	// 动态角色（自由推演中新登场，没有故事包定义）。
	Dynamic     bool   `json:"dynamic,omitempty"`
	Name        string `json:"name,omitempty"`
	Role        string `json:"role,omitempty"`
	Description string `json:"description,omitempty"`
}

// EncounterRecord 是某场遭遇战的战绩。
type EncounterRecord struct {
	Result string `json:"result"`
	Wins   int    `json:"wins,omitempty"`
	Losses int    `json:"losses,omitempty"`
	Fled   int    `json:"fled,omitempty"`
}

// UnitStatus 是单位身上的状态效果。
type UnitStatus struct {
	ID    string `json:"id"`
	Turns int    `json:"turns"`
}

// MechForm 是单位的机甲形态（独立生命、数值与热量）。
type MechForm struct {
	Ref     string         `json:"ref"`
	Name    string         `json:"name"`
	HP      int            `json:"hp"`
	MaxHP   int            `json:"max_hp"`
	Heat    int            `json:"heat"`
	HeatMax int            `json:"heat_max"`
	Stats   map[string]int `json:"stats"`
	Skills  []string       `json:"skills,omitempty"`
}

// Unit 是战斗中的单位。
type Unit struct {
	event.Unit
	Statuses  []UnitStatus `json:"statuses,omitempty"`
	Defending bool         `json:"defending,omitempty"`
	Down      bool         `json:"down,omitempty"`
	Mech      *MechForm    `json:"mech,omitempty"`
	// PartDamage / Broken：自由战斗的部位累计伤害与已破坏部位。
	PartDamage map[string]int `json:"part_damage,omitempty"`
	Broken     []string       `json:"broken,omitempty"`
}

// HasStatus 报告单位是否有某状态。
func (u *Unit) HasStatus(id string) bool {
	return slices.ContainsFunc(u.Statuses, func(x UnitStatus) bool { return x.ID == id })
}

// Combat 是进行中的战斗。
type Combat struct {
	Encounter string   `json:"encounter"`
	Title     string   `json:"title"`
	Node      string   `json:"node,omitempty"` // 由动态主线节点发起
	Round     int      `json:"round"`
	Order     []string `json:"order"`
	Cursor    int      `json:"cursor"`
	Units     []*Unit  `json:"units"`
	AllowMech bool     `json:"allow_mech,omitempty"`
	NoFlee    bool     `json:"no_flee,omitempty"`
}

// Unit 按 ID 查找单位。
func (c *Combat) Unit(id string) *Unit {
	for _, u := range c.Units {
		if u.ID == id {
			return u
		}
	}
	return nil
}

// Clone 深拷贝战斗状态。
func (c *Combat) Clone() *Combat {
	b, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	var out Combat
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return &out
}

// Current 返回当前行动单位（没有则 nil）。
func (c *Combat) Current() *Unit {
	if c.Cursor < 0 || c.Cursor >= len(c.Order) {
		return nil
	}
	return c.Unit(c.Order[c.Cursor])
}

// Alive 返回某一方仍能战斗的单位（按单位声明顺序）。
func (c *Combat) Alive(side string) []*Unit {
	var out []*Unit
	for _, u := range c.Units {
		if u.Side == side && !u.Down {
			out = append(out, u)
		}
	}
	return out
}

// EdgeKey 返回有向关系边的键。
func EdgeKey(from, to string) string { return from + ">" + to }

// R 返回 RPG 状态（旧存档为 nil 时补齐）。
func (s *State) R() *RPG {
	if s.RPG == nil {
		s.RPG = &RPG{}
	}
	r := s.RPG
	if r.Equipment == nil {
		r.Equipment = map[string]string{}
	}
	if r.Codex == nil {
		r.Codex = map[string]int{}
	}
	if r.Relations == nil {
		r.Relations = map[string]map[string]int{}
	}
	if r.Known == nil {
		r.Known = map[string]*KnownEdge{}
	}
	if r.Cards == nil {
		r.Cards = map[string]*Card{}
	}
	if r.Encounters == nil {
		r.Encounters = map[string]*EncounterRecord{}
	}
	return r
}

// PlayerLevel 返回玩家等级（零值按故事包初始等级）。
func (s *State) PlayerLevel(p *loader.Package) int {
	if s.RPG != nil && s.RPG.Level > 0 {
		return s.RPG.Level
	}
	return max(p.PlayerCombat().Level, 1)
}

// Edge 返回 from→to 的关系数值（含 NPC→玩家 的 trust / fear）。
func (s *State) Edge(from, to string) map[string]int {
	out := map[string]int{}
	if s.RPG != nil {
		for k, v := range s.RPG.Relations[EdgeKey(from, to)] {
			out[k] = v
		}
	}
	if to == loader.PlayerID {
		if n, ok := s.NPCs[from]; ok {
			out["trust"], out["fear"] = n.Trust, n.Fear
		}
	}
	return out
}

// initRPG 在新游戏时初始化 RPG 状态。
func (s *State) initRPG(p *loader.Package) {
	r := s.R()
	pc := p.PlayerCombat()
	r.Level = max(pc.Level, 1)
	r.Mercury = pc.Mercury
	for slot, it := range pc.Equipment {
		r.Equipment[slot] = it
		if s.Player.Inventory[it] == 0 {
			s.Player.Inventory[it] = 1
		}
	}
	r.Skills = append(r.Skills, pc.Skills...)
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		if c.CardTier() == "major" {
			r.Cards[id] = &Card{Status: CardActive, Reason: "主要角色"}
		}
		for k, v := range c.RelationshipToPlayer {
			if k != "trust" && k != "fear" && v != 0 {
				s.setEdge(id, loader.PlayerID, k, v)
			}
		}
	}
	for _, e := range p.Relations.Edges {
		for k, v := range e.Values {
			if e.To == loader.PlayerID && (k == "trust" || k == "fear") {
				if n, ok := s.NPCs[e.From]; ok {
					if k == "trust" {
						n.Trust = v
					} else {
						n.Fear = v
					}
				}
				continue
			}
			s.setEdge(e.From, e.To, k, v)
		}
		if e.Public {
			r.Known[EdgeKey(e.From, e.To)] = &KnownEdge{Values: s.Edge(e.From, e.To), Source: "public"}
		}
	}
	r.Codex[s.Player.Location] = 0
	for it := range s.Player.Inventory {
		r.Codex[it] = 0
	}
	for _, sk := range r.Skills {
		r.Codex[sk] = 0
	}
	if pc.Mech != "" {
		r.Codex[pc.Mech] = 0
	}
	if p.Combat != nil {
		for _, id := range p.Combat.MechIDs {
			if p.Combat.Mechs[id].Known {
				r.Codex[id] = 0
			}
		}
	}
}

func (s *State) setEdge(from, to, dim string, v int) {
	r := s.R()
	k := EdgeKey(from, to)
	if r.Relations[k] == nil {
		r.Relations[k] = map[string]int{}
	}
	r.Relations[k][dim] = v
}

func (s *State) unit(id string) (*Unit, error) {
	r := s.R()
	if r.Combat == nil {
		return nil, fmt.Errorf("no combat in progress")
	}
	u := r.Combat.Unit(id)
	if u == nil {
		return nil, fmt.Errorf("unknown unit %q", id)
	}
	return u, nil
}

func (s *State) logRel(e event.Event, from, to string, vals map[string]int, known bool) {
	r := s.R()
	r.RelLog = append(r.RelLog, RelChange{Turn: e.Turn, Minute: e.Minute, From: from, To: to, Values: vals, Reason: e.Data.Reason, Known: known})
	if len(r.RelLog) > MaxRelLog {
		r.RelLog = r.RelLog[len(r.RelLog)-MaxRelLog:]
	}
}

// applyRPG 处理 v0.1.1-rc2 的事件；handled=false 表示不是 RPG 事件。
func applyRPG(s *State, e event.Event) (handled bool, err error) {
	d := e.Data
	switch e.Type {
	case event.CombatStarted:
		c := &Combat{Encounter: d.Story, Title: d.Title, Node: d.Key, Cursor: -1, AllowMech: d.Values["mech"] == 1, NoFlee: d.Values["no_flee"] == 1}
		for _, u := range d.Units {
			c.Units = append(c.Units, &Unit{Unit: u})
		}
		s.R().Combat = c
	case event.CombatRoundStarted:
		c := s.R().Combat
		if c == nil {
			return true, fmt.Errorf("no combat")
		}
		c.Round, c.Order, c.Cursor = d.Delta, slices.Clone(d.Tags), -1
	case event.CombatTurnStarted:
		c := s.R().Combat
		if c == nil {
			return true, fmt.Errorf("no combat")
		}
		c.Cursor = d.Delta
		if u := c.Unit(d.Actor); u != nil {
			u.Defending = false
		}
	case event.CombatActed, event.UnitOverheated:
	case event.UnitHPChanged:
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		if u.Mech != nil && d.Key != "pilot" {
			u.Mech.HP = clamp(u.Mech.HP+d.Delta, 0, u.Mech.MaxHP)
		} else {
			u.HP = clamp(u.HP+d.Delta, 0, u.MaxHP)
		}
	case event.UnitResource:
		if d.Key == "mercury" {
			s.R().Mercury = max(s.R().Mercury+d.Delta, 0)
			return true, nil
		}
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		switch d.Key {
		case "sp":
			u.SP = clamp(u.SP+d.Delta, 0, u.MaxSP)
		case "heat":
			if u.Mech != nil {
				u.Mech.Heat = clamp(u.Mech.Heat+d.Delta, 0, u.Mech.HeatMax)
			}
		default:
			return true, fmt.Errorf("unknown resource %q", d.Key)
		}
	case event.UnitStatusApplied:
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		if i := slices.IndexFunc(u.Statuses, func(x UnitStatus) bool { return x.ID == d.Condition }); i >= 0 {
			u.Statuses[i].Turns = max(u.Statuses[i].Turns, d.Delta)
		} else {
			u.Statuses = append(u.Statuses, UnitStatus{ID: d.Condition, Turns: d.Delta})
		}
	case event.UnitStatusTicked:
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		if i := slices.IndexFunc(u.Statuses, func(x UnitStatus) bool { return x.ID == d.Condition }); i >= 0 {
			u.Statuses[i].Turns--
			if u.Statuses[i].Turns <= 0 {
				u.Statuses = slices.Delete(u.Statuses, i, i+1)
			}
		}
	case event.UnitStatusRemoved:
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		u.Statuses = slices.DeleteFunc(u.Statuses, func(x UnitStatus) bool { return x.ID == d.Condition })
	case event.UnitDefending:
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		u.Defending = true
	case event.UnitDefeated:
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		u.Down = true
	case event.MechEngaged:
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		if len(d.Units) != 1 {
			return true, fmt.Errorf("MechEngaged needs one unit snapshot")
		}
		m := d.Units[0]
		u.Mech = &MechForm{Ref: m.Ref, Name: m.Name, HP: m.HP, MaxHP: m.MaxHP, HeatMax: m.HeatMax, Stats: m.Stats, Skills: m.Skills}
	case event.MechDisengaged:
		u, err := s.unit(d.Target)
		if err != nil {
			return true, err
		}
		u.Mech = nil
	case event.CombatEnded:
		r := s.R()
		rec := r.Encounters[d.Story]
		if rec == nil {
			rec = &EncounterRecord{}
			r.Encounters[d.Story] = rec
		}
		rec.Result = d.Outcome
		switch d.Outcome {
		case "victory":
			rec.Wins++
		case "defeat":
			rec.Losses++
		default:
			rec.Fled++
		}
		r.Combat = nil
	case event.XPGained:
		s.R().XP += d.Delta
	case event.LevelUp:
		r := s.R()
		r.Level = d.Delta
		r.XP = max(r.XP-d.Values["cost"], 0)
		r.AttrPoints += d.Values["attr_points"]
		r.SkillPoints += d.Values["skill_points"]
	case event.AttributeAllocated:
		r := s.R()
		if r.AttrPoints < d.Delta {
			return true, fmt.Errorf("not enough attribute points")
		}
		r.AttrPoints -= d.Delta
		s.Player.Attributes[d.Key] += d.Delta
	case event.SkillLearned:
		r := s.R()
		if r.SkillPoints < d.Qty {
			return true, fmt.Errorf("not enough skill points")
		}
		r.SkillPoints -= d.Qty
		if !slices.Contains(r.Skills, d.Skill) {
			r.Skills = append(r.Skills, d.Skill)
		}
	case event.ItemEquipped:
		if s.Player.Inventory[d.Item] <= 0 {
			return true, fmt.Errorf("equip %s: not owned", d.Item)
		}
		s.R().Equipment[d.Key] = d.Item
	case event.ItemUnequipped:
		delete(s.R().Equipment, d.Key)
	case event.PlayerVitals:
		r := s.R()
		r.Wounds = max(r.Wounds-d.Values["hp"], 0)
		r.Mercury = max(r.Mercury+d.Values["mercury"], 0)
	case event.CodexUnlocked:
		s.R().Codex[d.Key] = e.Turn
	case event.MechStatusChanged, event.MechPartChanged, event.MechRevealed, event.MechPilotChanged:
		m := s.R().Mech(d.Target)
		switch e.Type {
		case event.MechStatusChanged:
			m.Status = d.Key
		case event.MechPartChanged:
			target := m.Installed
			if strings.HasPrefix(d.Key, "hp:") {
				target = m.Mounted
			}
			slot := strings.TrimPrefix(d.Key, "hp:")
			if d.Remove {
				target[slot] = ""
			} else {
				target[slot] = d.Item
			}
		case event.MechRevealed:
			for _, f := range d.Tags {
				m.Known[f] = e.Turn
			}
		case event.MechPilotChanged:
			m.Pilot = d.Actor
			if d.Actor == "" {
				m.Pilot = "-"
			}
		}
		m.History = append(m.History, MechLog{Turn: e.Turn, Minute: e.Minute, Type: e.Type, Key: d.Key, Item: d.Item, Actor: d.Actor, Remove: d.Remove, Reason: d.Reason})
		if len(m.History) > MaxMechLog {
			m.History = m.History[len(m.History)-MaxMechLog:]
		}
	case event.RelationEdgeChanged:
		for k, v := range d.Values {
			if d.Target == loader.PlayerID && (k == "trust" || k == "fear") {
				n, ok := s.NPCs[d.Actor]
				if !ok {
					return true, fmt.Errorf("unknown npc %q", d.Actor)
				}
				if k == "trust" {
					n.Trust = clamp(n.Trust+v, RelMin, RelMax)
				} else {
					n.Fear = clamp(n.Fear+v, 0, RelMax)
				}
				continue
			}
			cur := s.Edge(d.Actor, d.Target)[k]
			s.setEdge(d.Actor, d.Target, k, cur+v)
		}
		s.logRel(e, d.Actor, d.Target, d.Values, d.Notable)
		if d.Notable && d.Target != loader.PlayerID && d.Actor != loader.PlayerID {
			s.R().Known[EdgeKey(d.Actor, d.Target)] = &KnownEdge{Values: s.Edge(d.Actor, d.Target), Turn: e.Turn, Source: "witness"}
		}
	case event.RelationRevealed:
		vals := map[string]int{}
		for k, v := range d.Values {
			vals[k] = v
		}
		src := d.Source
		if src == "" {
			src = "reveal"
		}
		s.R().Known[EdgeKey(d.Actor, d.Target)] = &KnownEdge{Values: vals, Turn: e.Turn, Source: src}
	case event.CharacterCardCreated:
		c := &Card{Status: CardActive, Created: e.Turn, Reason: d.Reason}
		if d.Node != nil {
			c.Dynamic, c.Name, c.Role, c.Description = true, d.Node.Title, d.Node.Role, d.Node.Summary
		}
		s.R().Cards[d.Target] = c
	case event.CharacterCardArchived, event.CharacterCardRestored:
		c := s.R().Cards[d.Target]
		if c == nil {
			return true, fmt.Errorf("no card for %q", d.Target)
		}
		c.Status, c.Changed = map[bool]string{true: CardArchived, false: CardActive}[e.Type == event.CharacterCardArchived], e.Turn
		if d.Reason != "" {
			c.Reason = d.Reason
		}
	case event.CharacterDied:
		r := s.R()
		c := r.Cards[d.Target]
		if c == nil {
			c = &Card{Created: e.Turn}
			r.Cards[d.Target] = c
		}
		c.Status, c.Changed, c.Reason = CardDead, e.Turn, d.Reason
		if n, ok := s.NPCs[d.Target]; ok {
			n.Location = ""
		}
	default:
		return false, nil
	}
	return true, nil
}
