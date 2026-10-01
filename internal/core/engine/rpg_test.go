package engine_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/api/query"
	"github.com/GUYU2233/ibukiRPG/internal/combat/sim"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/packages"
)

const (
	bOrin     = "brass:character/orin"
	bTock     = "brass:character/tock"
	bRivet    = "brass:character/rivet"
	bArena    = "brass:location/arena"
	bPlaza    = "brass:location/plaza"
	bWorkshop = "brass:location/workshop"
)

func brassSetup(t *testing.T, seed uint64) (*engine.Engine, *state.State) {
	t.Helper()
	ev, err := expression.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, fs := range packages.Builtin() {
		p, err := loader.Load(fs, ev)
		if err != nil {
			t.Fatal(err)
		}
		if p.Manifest.ID == "brass_trial" {
			return engine.New(p, ev), state.New(p, seed, "阿砾")
		}
	}
	t.Fatal("brass pack missing")
	return nil, nil
}

func bAct(id, target string) command.Command {
	return command.Command{Kind: command.KindAction, Action: "brass:action/" + id, Target: target}
}

func mustAccept(t *testing.T, r *runner, c command.Command) *engine.Result {
	t.Helper()
	res := r.do(c)
	if !res.Accepted {
		t.Fatalf("command %+v rejected: %s", c, res.Reason)
	}
	return res
}

// TestGrowth：经验 → 升级 → 属性点 / 技能点 → 加点、修习、装备都会改变数值。
func TestGrowth(t *testing.T) {
	eng, s := brassSetup(t, 3)
	p := eng.Pkg
	s.Player.Location = bArena
	if s.PlayerLevel(p) != 1 {
		t.Fatalf("start level %d", s.PlayerLevel(p))
	}
	var err error
	for i := 0; s.PlayerLevel(p) < 2 && i < 6; i++ {
		_, s, err = sim.Fight(eng, s, "brass:encounter/practice", "p"+string(rune('a'+i)))
		if err != nil {
			t.Fatal(err)
		}
	}
	if s.PlayerLevel(p) != 2 || s.RPG.AttrPoints != 2 || s.RPG.SkillPoints != 1 {
		t.Fatalf("after level up: lv %d attr %d skill %d", s.PlayerLevel(p), s.RPG.AttrPoints, s.RPG.SkillPoints)
	}
	r := &runner{t: t, eng: eng, s: s, n: 100, all: make([]event.Event, s.LastSeq)}
	atk0 := engine.PlayerStats(p, r.s)["atk"]
	str0 := r.s.Player.Attributes["strength"]
	mustAccept(t, r, command.Command{Kind: command.KindManage, Action: "allocate", Target: "strength"})
	if r.s.Player.Attributes["strength"] != str0+1 || engine.PlayerStats(p, r.s)["atk"] <= atk0 {
		t.Fatalf("allocate did not raise str/atk")
	}
	mustAccept(t, r, command.Command{Kind: command.KindManage, Action: "learn", Skill: "brass:skill/flurry"})
	if r.s.RPG.SkillPoints != 0 {
		t.Fatalf("skill points not spent")
	}
	if res := r.do(command.Command{Kind: command.KindManage, Action: "learn", Skill: "brass:skill/rivet_shot"}); res.Accepted {
		t.Fatal("level-3 skill must be blocked at level 2")
	}
	r.s.Player.Inventory["brass:item/brass_plate"] = 1
	def0 := engine.PlayerStats(p, r.s)["def"]
	mustAccept(t, r, command.Command{Kind: command.KindManage, Action: "equip", Item: "brass:item/brass_plate"})
	if engine.PlayerStats(p, r.s)["def"] <= def0 {
		t.Fatal("armor did not raise def")
	}
	if r.s.RPG.Equipment["armor"] != "brass:item/brass_plate" {
		t.Fatalf("equipment: %v", r.s.RPG.Equipment)
	}
	mustAccept(t, r, command.Command{Kind: command.KindManage, Action: "unequip", Target: "armor"})
	if r.s.RPG.Equipment["armor"] != "" {
		t.Fatal("unequip failed")
	}
}

// TestCombatReplay：战斗事件重放得到完全相同的状态（存档 = 事件流）。
func TestCombatReplay(t *testing.T) {
	eng, s0 := brassSetup(t, 11)
	s0.Player.Location = bArena
	r := &runner{t: t, eng: eng, s: s0}
	mustAccept(t, r, command.Command{Kind: command.KindCombat, Action: "start", Target: "brass:encounter/trial_bout1"})
	for i := 0; i < 40 && r.s.RPG.Combat != nil; i++ {
		mustAccept(t, r, sim.Policy(eng, r.s))
	}
	if r.s.RPG.Combat != nil {
		t.Fatal("fight did not end")
	}
	// 从初始状态重放全部事件
	_, re := brassSetup(t, 11)
	re.Player.Location = bArena
	for _, e := range r.all {
		if err := state.Apply(re, e); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := r.s.Marshal()
	b, _ := re.Marshal()
	if !bytes.Equal(a, b) {
		t.Fatalf("replayed state differs:\n%s\n---\n%s", a, b)
	}
}

// TestCardLifecycle：次要角色升格 → 归档（保留记忆与关系）→ 再次交谈恢复 → 死亡是独立状态。
func TestCardLifecycle(t *testing.T) {
	eng, s := brassSetup(t, 5)
	q := &query.Q{Pkg: eng.Pkg, Eval: eng.Eval}
	s.Player.Location = bPlaza
	r := &runner{t: t, eng: eng, s: s}
	if c := q.Cards(r.s); len(c.Active) != 1 || c.Active[0].ID != bOrin {
		t.Fatalf("initial cards: %+v", c.Active)
	}
	created := false
	for i := 0; i < 3 && !created; i++ {
		res := mustAccept(t, r, bAct("talk", bRivet))
		created = has(res, event.CharacterCardCreated)
	}
	if !created || r.s.RPG.Cards[bRivet].Status != state.CardActive {
		t.Fatalf("rivet should be promoted after talks: %+v", r.s.RPG.Cards[bRivet])
	}
	talks := r.s.NPCs[bRivet].Talks
	// 归档：卡片离场，但记忆与关系保留
	arch := event.Event{Seq: int64(len(r.all) + 1), Turn: r.s.Turn, Type: event.CharacterCardArchived, Data: event.Data{Target: bRivet, Reason: "离开了小镇"}}
	if err := state.Apply(r.s, arch); err != nil {
		t.Fatal(err)
	}
	r.all = append(r.all, arch)
	cards := q.Cards(r.s)
	if len(cards.Archived) != 1 || cards.Archived[0].ID != bRivet {
		t.Fatalf("archived: %+v", cards.Archived)
	}
	if r.s.NPCs[bRivet].Talks != talks || len(cards.Archived[0].Memories) == 0 {
		t.Fatal("archived card must keep memories")
	}
	res := mustAccept(t, r, bAct("talk", bRivet))
	if !has(res, event.CharacterCardRestored) || r.s.RPG.Cards[bRivet].Status != state.CardActive {
		t.Fatal("talking to an archived character should restore the card")
	}
	if r.s.NPCs[bRivet].Talks <= talks {
		t.Fatal("restored character is amnesiac")
	}
	die := event.Event{Seq: int64(len(r.all) + 1), Turn: r.s.Turn, Type: event.CharacterDied, Data: event.Data{Target: bRivet, Reason: "测试"}}
	if err := state.Apply(r.s, die); err != nil {
		t.Fatal(err)
	}
	if c := q.Cards(r.s); len(c.Dead) != 1 {
		t.Fatalf("dead: %+v", c.Dead)
	}
	if res := r.do(bAct("talk", bRivet)); res.Accepted {
		t.Fatal("cannot talk to a dead character")
	}
}

func findEdge(v []struct{ from, to string }, from, to string) bool {
	for _, e := range v {
		if e.from == from && e.to == to {
			return true
		}
	}
	return false
}

// TestRelationVisibility：NPC↔NPC 的隐藏关系在揭示前不可见；揭示后看到的是当时的快照。
func TestRelationVisibility(t *testing.T) {
	eng, s := brassSetup(t, 9)
	q := &query.Q{Pkg: eng.Pkg, Eval: eng.Eval}
	s.Player.Location = bArena
	r := &runner{t: t, eng: eng, s: s}
	edges := func() []struct{ from, to string } {
		var out []struct{ from, to string }
		for _, e := range q.Relations(r.s).Edges {
			out = append(out, struct{ from, to string }{e.From, e.To})
		}
		return out
	}
	if !findEdge(edges(), bOrin, bTock) {
		t.Fatal("public edge orin→tock must be visible")
	}
	if findEdge(edges(), bTock, bRivet) {
		t.Fatal("hidden edge tock→rivet must not be visible yet")
	}
	for i := 0; i < 3 && !findEdge(edges(), bTock, bRivet); i++ {
		mustAccept(t, r, bAct("talk", bTock))
	}
	if !findEdge(edges(), bTock, bRivet) {
		t.Fatal("dialogue reveal should expose tock→rivet")
	}
	snap := func() int {
		for _, e := range q.Relations(r.s).Edges {
			if e.From == bTock && e.To == bRivet {
				for _, d := range e.Values {
					if d.ID == "affection" {
						return d.Value
					}
				}
			}
		}
		return -1
	}
	before := snap()
	// 私下变化（玩家不在场、未被揭示）不会更新玩家看到的快照
	ch := event.Event{Seq: int64(len(r.all) + 1), Turn: r.s.Turn, Type: event.RelationEdgeChanged, Data: event.Data{Actor: bTock, Target: bRivet, Values: map[string]int{"affection": -25}}}
	if err := state.Apply(r.s, ch); err != nil {
		t.Fatal(err)
	}
	if snap() != before {
		t.Fatalf("unwitnessed change leaked into player view: %d → %d", before, snap())
	}
	if r.s.Edge(bTock, bRivet)["affection"] == before {
		t.Fatal("underlying edge should have changed")
	}
}

// TestLegacySaveLoads：没有 rpg 字段的旧存档可以读取，并在首次访问时惰性初始化。
func TestLegacySaveLoads(t *testing.T) {
	eng, s := setup(t)
	b, _ := s.Marshal()
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	delete(raw, "rpg")
	b, _ = json.Marshal(raw)
	old, err := state.Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	r := &runner{t: t, eng: eng, s: old}
	mustAccept(t, r, act("look", ""))
	if old.PlayerLevel(eng.Pkg) < 1 {
		t.Fatal("legacy state level")
	}
}
