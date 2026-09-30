package sim_test

import (
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/combat/sim"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func brass(t *testing.T) *engine.Engine {
	t.Helper()
	ev, err := expression.New()
	if err != nil {
		t.Fatal(err)
	}
	var p *loader.Package
	for _, fs := range packages.Builtin() {
		q, err := loader.Load(fs, ev)
		if err != nil {
			t.Fatal(err)
		}
		if q.Manifest.ID == "brass_trial" {
			p = q
		}
	}
	if p == nil {
		t.Fatal("brass pack missing")
	}
	return engine.New(p, ev)
}

type stats struct {
	wins, total, hpSum, rounds int
}

func (s stats) rate() int { return s.wins * 100 / max(s.total, 1) }

// TestBalanceBrass 用 200 个种子模拟示例包的主线战斗：必须能赢（胜率足够高），但不能毫无压力（平均剩余生命不能太高）。
func TestBalanceBrass(t *testing.T) {
	eng := brass(t)
	var b1, b2, amb, golem stats
	for seed := uint64(1); seed <= 200; seed++ {
		s := state.New(eng.Pkg, seed, "")
		s.Player.Location = "brass:location/arena"
		r, s2, err := sim.Fight(eng, s, "brass:encounter/trial_bout1", "b1")
		if err != nil {
			t.Fatal(err)
		}
		b1.add(r)
		r, s3, err := sim.Fight(eng, s2, "brass:encounter/trial_bout2", "b2")
		if err != nil {
			t.Fatal(err)
		}
		b2.add(r)
		// 下水道：假设玩家已通过试炼并休整（满血、领到燃料），等级按实际经验
		s3 = heal(eng, s3)
		s3.Flags["warden_granted"] = true
		s3.R().Mech("brass:mech/warden").Status = "ready"
		s3.Player.Location = "brass:location/sewer"
		s3.NPCs["brass:character/rivet"].Location = "brass:location/sewer"
		r, s4, err := sim.Fight(eng, s3, "brass:encounter/sewer_ambush", "am")
		if err != nil {
			t.Fatal(err)
		}
		amb.add(r)
		s4 = heal(eng, s4)
		s4.Player.Location = "brass:location/sewer"
		r, _, err = sim.Fight(eng, s4, "brass:encounter/golem", "go")
		if err != nil {
			t.Fatal(err)
		}
		golem.add(r)
	}
	for name, st := range map[string]stats{"bout1": b1, "bout2": b2, "ambush": amb, "golem": golem} {
		t.Logf("%-7s win %3d%%  avg hp left %3d%%  avg rounds %d", name, st.rate(), st.hpSum/max(st.total, 1), st.rounds/max(st.total, 1))
	}
	check := func(name string, st stats, minWin, maxWin, maxHP int) {
		if st.rate() < minWin || st.rate() > maxWin {
			t.Errorf("%s: win rate %d%% outside [%d, %d]", name, st.rate(), minWin, maxWin)
		}
		if avg := st.hpSum / max(st.total, 1); avg > maxHP {
			t.Errorf("%s: too easy, avg hp left %d%% > %d%%", name, avg, maxHP)
		}
	}
	check("bout1", b1, 90, 100, 95)
	check("bout2", b2, 60, 97, 75)
	check("ambush", amb, 65, 100, 85)
	check("golem", golem, 60, 99, 90)
}

func (s *stats) add(r sim.Result) {
	s.total++
	s.rounds += r.Rounds
	if r.Outcome == "victory" {
		s.wins++
		s.hpSum += r.HPLeftPct
	}
}

func heal(eng *engine.Engine, s *state.State) *state.State {
	s = s.Clone()
	s.R().Wounds = 0
	s.R().Mercury = 100
	s.Player.Inventory["brass:item/bandage"] = 2
	return s
}

// TestFightDeterministic：同一种子、同一指令序列必须产生逐字节相同的事件。
func TestFightDeterministic(t *testing.T) {
	eng := brass(t)
	run := func() []event.Event {
		s := state.New(eng.Pkg, 7, "")
		s.Player.Location = "brass:location/arena"
		var all []event.Event
		res, ns, err := eng.Execute(s, command.Command{ID: "x0", Kind: command.KindCombat, Action: "start", Target: "brass:encounter/trial_bout1"})
		if err != nil || !res.Accepted {
			t.Fatal(err, res.Reason)
		}
		all = append(all, res.Events...)
		s = ns
		for i := 0; s.RPG.Combat != nil && i < 50; i++ {
			c := sim.Policy(eng, s)
			c.ID = "x" + string(rune('a'+i))
			res, ns, err := eng.Execute(s, c)
			if err != nil || !res.Accepted {
				t.Fatal(err, res.Reason)
			}
			all = append(all, res.Events...)
			s = ns
		}
		return all
	}
	a, b := run(), run()
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("event counts differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Type != b[i].Type || a[i].Data.Roll != b[i].Data.Roll || a[i].Data.Delta != b[i].Data.Delta {
			t.Fatalf("event %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}
