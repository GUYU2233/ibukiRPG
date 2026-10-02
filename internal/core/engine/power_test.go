package engine_test

import (
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/power"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
)

// TestPowerBudgetValidator：新建卡片的数值按“种类 × 稀有度 × 等级”预算等比缩小；
// 之后修改数值也不能超出预算。
func TestPowerBudgetValidator(t *testing.T) {
	eng, s0 := brassSetup(t, 7)
	r := &runner{t: t, eng: eng, s: s0}
	ns := validate.GenNamespace(eng.Pkg)
	create := change.Change{Op: change.OpCreate, Target: ns + ":item/thunder_wrench", Kind: "item", Reason: "测试",
		Value: raw(map[string]any{"fields": map[string]any{"name": "雷鸣扳手", "description": "改装扳手"}, "tags": []string{"weapon", "rare"},
			"stats": map[string]int{"atk": 80, "def": 10, "level": 1}})}
	res := mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Source: change.SourceNarrate, Changes: []change.Change{create}})
	if len(res.Rejects) > 0 {
		t.Fatalf("rejected: %+v", res.Rejects)
	}
	id := r.s.World.Log[len(r.s.World.Log)-1].Target
	d := r.s.Doc(eng.Pkg, id)
	if d == nil {
		t.Fatal("created doc missing")
	}
	pp, budget := validate.DocBudget(eng.Pkg, d)
	if pp.Kind != "weapon" || pp.Rarity != "rare" {
		t.Fatalf("params %+v", pp)
	}
	if want := power.Budget(validate.PowerRef(eng.Pkg), power.Params{Kind: "weapon", Rarity: "rare", Level: 1}); budget != want || budget == 0 {
		t.Fatalf("budget %d want %d", budget, want)
	}
	if sc := power.Score(d.Stats); sc > budget {
		t.Fatalf("score %d > budget %d (%v)", sc, budget, d.Stats)
	}
	// 修改数值：超出预算的部分被截掉
	patch := change.Change{Op: change.OpPatch, Target: id, Path: "stats.atk", Value: raw(999), Reason: "测试"}
	res = mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Source: change.SourceNarrate, Changes: []change.Change{patch}})
	d = r.s.Doc(eng.Pkg, id)
	if sc := power.Score(d.Stats); sc > budget {
		t.Fatalf("after patch score %d > budget %d", sc, budget)
	}
	_ = res
}
