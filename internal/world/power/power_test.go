package power

import "testing"

func TestBudgetFormula(t *testing.T) {
	cases := []struct {
		p    Params
		want int
	}{
		{Params{Kind: "character"}, 90},
		{Params{Kind: "weapon"}, 31},                            // 90×0.35 = 31.5（浮点略小于 31.5）→ 31
		{Params{Kind: "weapon", Rarity: "rare", Level: 3}, 60},  // 90×0.35×1.6×1.2 = 60.48
		{Params{Kind: "mech", Rarity: "legendary"}, 468},        // 90×2×2.6
		{Params{Kind: "item", Rarity: "bogus", Level: 0}, 18},   // 未知稀有度 → common；等级 0 → 1
		{Params{Kind: "armor", Rarity: "epic", Level: 99}, 157}, // 等级封顶 20：90×0.3×2×2.9 = 156.6
		{Params{Kind: "lore"}, 0},                               // 没有数值预算
	}
	for _, c := range cases {
		if got := Budget(90, c.p); got != c.want {
			t.Errorf("Budget(%+v) = %d, want %d", c.p, got, c.want)
		}
	}
	if Budget(0, Params{Kind: "character"}) != DefaultRef {
		t.Fatal("ref 0 should use DefaultRef")
	}
}

func TestScoreAndFit(t *testing.T) {
	st := map[string]int{"hp": 40, "atk": 10, "def": 6, "level": 5}
	if s := Score(st); s != 10+20+12 { // level 不计
		t.Fatalf("score %d", s)
	}
	fit, changed := Fit(st, 30)
	if !changed || Score(fit) > 30 {
		t.Fatalf("fit %v score %d", fit, Score(fit))
	}
	if fit["level"] != 5 {
		t.Fatal("level must not be scaled")
	}
	same, changed := Fit(st, 100)
	if changed || same["atk"] != 10 {
		t.Fatal("within budget must be unchanged")
	}
	if m := MaxFor(map[string]int{"atk": 5, "def": 5}, "atk", 30); m != 10 { // (30-10)/2
		t.Fatalf("MaxFor %d", m)
	}
}

func TestParamsFrom(t *testing.T) {
	p := ParamsFrom("item", []string{"weapon", "rare"}, nil, map[string]any{"level": float64(3)}, nil)
	if p.Kind != "weapon" || p.Rarity != "rare" || p.Level != 3 {
		t.Fatalf("%+v", p)
	}
	p = ParamsFrom("character", nil, map[string]string{"rarity": "epic", "level": "4"}, nil, map[string]int{"level": 2})
	if p.Rarity != "epic" || p.Level != 2 {
		t.Fatalf("%+v", p)
	}
}
