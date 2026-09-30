package engine_test

import (
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/api/query"
	"github.com/GUYU2233/ibukiRPG/internal/combat/sim"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
)

const (
	warden = "brass:mech/warden"
	golem  = "brass:mech/golem"
)

// TestMechVisibility：机甲卡按字段遮蔽；遭遇时只解锁外观，击败并检查后揭示部分字段，驾驶后揭示同步率。
func TestMechVisibility(t *testing.T) {
	eng, s := brassSetup(t, 21)
	q := &query.Q{Pkg: eng.Pkg, Eval: eng.Eval}
	g, _ := q.Mech(s, golem)
	if g.Known || g.Name != "？？？" {
		t.Fatalf("golem card must be locked before meeting it: %+v", g)
	}
	w, _ := q.Mech(s, warden)
	if !w.Known || !w.Owned {
		t.Fatal("player's mech card should be unlocked")
	}
	for _, sp := range w.Specs {
		if sp.ID == "sync" && sp.Known {
			t.Fatal("sync rate should be unknown before piloting")
		}
	}
	if w.LoreKnown || w.Status.Name != "未激活" {
		t.Fatalf("warden lore hidden / status inactive: lore=%v status=%s", w.LoreKnown, w.Status.Name)
	}
	// 遭遇傀儡：卡片解锁，但数值 / 制造方 / 锅炉舱未知
	s.Flags["warden_granted"] = true
	s.R().Mech(warden).Status = "ready"
	s.R().Level = 5
	s.R().Mercury = 100
	s.Player.Location = "brass:location/sewer"
	r := &runner{t: t, eng: eng, s: s}
	mustAccept(t, r, command.Command{Kind: command.KindCombat, Action: "start", Target: "brass:encounter/golem"})
	g, _ = q.Mech(r.s, golem)
	if !g.Known || g.StatsKnown || g.Slots[0].Known {
		t.Fatalf("golem after meeting: known=%v stats=%v slot=%v", g.Known, g.StatsKnown, g.Slots[0].Known)
	}
	for _, f := range g.Fields {
		if f.ID == "maker" && (f.Known || f.Value != query.Unknown) {
			t.Fatalf("maker should be 未知: %+v", f)
		}
	}
	unknownBefore := g.Unknown
	// 驾驶黄铜守卫：揭示同步率
	res := mustAccept(t, r, command.Command{Kind: command.KindCombat, Action: "mech"})
	if !has(res, event.MechRevealed) {
		t.Fatal("piloting should reveal hidden specs")
	}
	w, _ = q.Mech(r.s, warden)
	for _, sp := range w.Specs {
		if sp.ID == "sync" && (!sp.Known || sp.Value != 62) {
			t.Fatalf("sync after piloting: %+v", sp)
		}
	}
	for i := 0; i < 80 && r.s.RPG.Combat != nil; i++ {
		mustAccept(t, r, sim.Policy(eng, r.s))
	}
	if r.s.RPG.Encounters["brass:encounter/golem"] == nil || r.s.RPG.Encounters["brass:encounter/golem"].Wins == 0 {
		t.Fatal("level-5 player should beat the golem with this seed")
	}
	g, _ = q.Mech(r.s, golem)
	if !g.StatsKnown || !g.Slots[0].Known || !g.LoreKnown || g.Unknown >= unknownBefore {
		t.Fatalf("victory should reveal stats/boiler/lore: %+v", g)
	}
	if g.Status.ID != "sealed" {
		t.Fatalf("golem should be sealed after defeat, got %q", g.Status.ID)
	}
	for _, f := range g.Fields {
		if f.ID == "maker" && f.Known {
			t.Fatal("maker was never revealed")
		}
	}
}

// TestMechStateTransitions：状态变化、改装都是事件；锁定状态不能启动 / 改装；数值随改装与战损变化。
func TestMechStateTransitions(t *testing.T) {
	eng, s := brassSetup(t, 4)
	p := eng.Pkg
	s.Flags["warden_granted"] = true
	s.Player.Location = "brass:location/sewer"
	r := &runner{t: t, eng: eng, s: s}
	// 未激活：无法启动
	mustAccept(t, r, command.Command{Kind: command.KindCombat, Action: "start", Target: "brass:encounter/golem"})
	if res := r.do(command.Command{Kind: command.KindCombat, Action: "mech"}); res.Accepted {
		t.Fatal("inactive mech must not engage")
	}
	// 回到非战斗状态做改装测试
	eng2, s2 := brassSetup(t, 4)
	r = &runner{t: t, eng: eng2, s: s2}
	r.s.Player.Inventory["brass:item/relief_valve"] = 1
	r.s.Player.Inventory["brass:item/rivet_cannon"] = 1
	f0, _ := engine.MechForm(p, r.s, warden)
	res := mustAccept(t, r, command.Command{Kind: command.KindManage, Action: "mech_install", Target: "valve", Item: "brass:item/relief_valve"})
	if !has(res, event.MechPartChanged) || r.s.Player.Inventory["brass:item/relief_valve"] != 0 {
		t.Fatal("install should consume the part and emit MechPartChanged")
	}
	f1, _ := engine.MechForm(p, r.s, warden)
	if f1.Stats["def"] != f0.Stats["def"]+2 || f1.HeatMax != f0.HeatMax+15 || f1.Specs["heat_threshold"] != f0.Specs["heat_threshold"]+15 {
		t.Fatalf("relief valve mods not applied: %+v vs %+v", f1, f0)
	}
	if res := r.do(command.Command{Kind: command.KindManage, Action: "mech_install", Target: "hp:arm", Item: "brass:item/relief_valve"}); res.Accepted {
		t.Fatal("part must not fit into a hardpoint")
	}
	mustAccept(t, r, command.Command{Kind: command.KindManage, Action: "mech_install", Target: "hp:shoulder", Item: "brass:item/rivet_cannon"})
	f2, _ := engine.MechForm(p, r.s, warden)
	if f2.PerTurn != f1.PerTurn+1 || !contains(f2.Skills, "brass:skill/rivet_volley") {
		t.Fatalf("weapon not mounted: %+v", f2)
	}
	mustAccept(t, r, command.Command{Kind: command.KindManage, Action: "mech_remove", Target: "hp:arm"})
	f3, _ := engine.MechForm(p, r.s, warden)
	if contains(f3.Skills, "brass:skill/steam_punch") || r.s.Player.Inventory["brass:item/steam_fist"] != 1 {
		t.Fatal("removing the fist should drop its skill and return it to the bag")
	}
	// 状态：战损降低数值；封印禁止改装
	dmg := event.Event{Seq: r.s.LastSeq + 1, Turn: r.s.Turn, Type: event.MechStatusChanged, Data: event.Data{Target: warden, Key: "damaged"}}
	if err := state.Apply(r.s, dmg); err != nil {
		t.Fatal(err)
	}
	f4, _ := engine.MechForm(p, r.s, warden)
	if f4.Stats["atk"] >= f3.Stats["atk"] || !f4.Status.Engage {
		t.Fatalf("damaged should reduce stats but stay usable: %+v", f4)
	}
	seal := event.Event{Seq: r.s.LastSeq + 1, Turn: r.s.Turn, Type: event.MechStatusChanged, Data: event.Data{Target: warden, Key: "sealed"}}
	if err := state.Apply(r.s, seal); err != nil {
		t.Fatal(err)
	}
	r.all = make([]event.Event, r.s.LastSeq)
	if res := r.do(command.Command{Kind: command.KindManage, Action: "mech_install", Target: "hp:arm", Item: "brass:item/steam_fist"}); res.Accepted {
		t.Fatal("sealed mech cannot be modified")
	}
	v := r.s.MechOf(p, warden)
	if len(v.History) < 5 {
		t.Fatalf("history should record every change: %d", len(v.History))
	}
	// 故事效果：通过试炼后整备完成
	eng3, s3 := brassSetup(t, 8)
	s3.Player.Location = "brass:location/arena"
	for _, enc := range []string{"brass:encounter/trial_bout1", "brass:encounter/trial_bout2"} {
		var err error
		s3.R().Wounds = 0
		_, s3, err = sim.Fight(eng3, s3, enc, enc[len(enc)-4:])
		if err != nil {
			t.Fatal(err)
		}
	}
	if s3.RPG.Encounters["brass:encounter/trial_bout2"].Wins > 0 && s3.MechOf(p, warden).Status != "ready" {
		t.Fatalf("passing the trial should make the warden ready, got %q", s3.MechOf(p, warden).Status)
	}
}

func contains(list []string, x string) bool {
	for _, v := range list {
		if v == x {
			return true
		}
	}
	return false
}
