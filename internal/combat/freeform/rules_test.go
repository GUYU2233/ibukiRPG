package freeform

import "testing"

func TestRulesIntent(t *testing.T) {
	ctx := Context{
		Targets:   []Target{{ID: "rat", Name: "铜鼠"}, {ID: "golem", Name: "铆钉傀儡", Parts: []Part{{ID: "core", Name: "蒸汽核心", Mult: 150}}}},
		Skills:    []SkillInfo{{ID: "x/skill:spark", Name: "电火花", Usable: true}},
		Maneuvers: []Maneuver{{ID: "trip", Name: "绊倒", Keywords: []string{"扫腿"}}},
		Tags:      []Tag{{ID: "cover", Label: "掩体"}},
	}
	in := RulesIntent("我躲到掩体后面，扫腿绊铆钉傀儡", ctx)
	m := in.Main()
	if m.Kind != KindManeuver || m.Target != "golem" || m.Maneuver != "trip" {
		t.Fatalf("got %+v", in)
	}
	if len(in.Actions) != 2 || len(in.Circumstances) != 1 {
		t.Fatalf("move/cover not parsed: %+v", in)
	}
	in = RulesIntent("用电火花打铆钉傀儡的蒸汽核心", ctx)
	if m := in.Main(); m.Kind != KindSkill || m.Part != "core" {
		t.Fatalf("got %+v", m)
	}
	in = RulesIntent("一拳打爆太阳", ctx)
	if in.Absurd == "" {
		t.Fatal("absurd not detected")
	}
	if RulesIntent("撤退！", ctx).Main().Kind != KindFlee {
		t.Fatal("flee")
	}
}
