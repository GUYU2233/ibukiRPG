package engine_test

import (
	"bytes"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
)

func lastID(s *state.State) string { return s.World.Log[len(s.World.Log)-1].ID }

func applyOne(t *testing.T, r *runner, c change.Change) string {
	t.Helper()
	res := mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Source: change.SourceNarrate, Changes: []change.Change{c}})
	if len(res.Rejects) > 0 || !has(res, event.WorldChangeApplied) {
		t.Fatalf("not applied: %+v", res.Rejects)
	}
	return lastID(r.s)
}

// TestCascadingRevert：撤销有依赖的变更时列出依赖链；默认拒绝；chain 连同撤销；single 只撤销本条（被覆盖的值保持不变，
// 之后撤销覆盖它的变更会回到最初的值）；新建实体被依赖时不能单独撤销；事件流可重放。
func TestCascadingRevert(t *testing.T) {
	eng, s0 := brassSetup(t, 7)
	r := &runner{t: t, eng: eng, s: s0}
	orig := r.s.Doc(eng.Pkg, bPlaza).Field("description")
	desc := func(v string) change.Change {
		return change.Change{Op: change.OpPatch, Target: bPlaza, Path: "fields.description", Value: raw(v), Reason: "测试"}
	}
	a := applyOne(t, r, desc("广场上落满了铜屑。"))
	b := applyOne(t, r, desc("广场上的铜屑被扫干净了。"))
	ns := validate.GenNamespace(eng.Pkg)
	npc := ns + ":character/sweeper"
	c := applyOne(t, r, change.Change{Op: change.OpCreate, Target: npc, Kind: "character", Reason: "测试",
		Value: raw(map[string]any{"fields": map[string]any{"name": "扫街人", "description": "拿着铜扫帚的老人"}})})
	d := applyOne(t, r, change.Change{Op: change.OpPatch, Target: npc, Path: "fields.description", Value: raw("拿着铜扫帚、哼着歌的老人"), Reason: "测试"})

	// 依赖链
	if got := r.s.World.Chain(a); len(got) != 1 || got[0] != b {
		t.Fatalf("chain(a) = %v", got)
	}
	if got := r.s.World.Chain(c); len(got) != 1 || got[0] != d {
		t.Fatalf("chain(c) = %v", got)
	}
	if ok, shadow, _ := r.s.World.SinglePlan(a); !ok || shadow != b {
		t.Fatalf("single plan a: ok=%v shadow=%s", ok, shadow)
	}
	if ok, _, why := r.s.World.SinglePlan(c); ok || why == "" {
		t.Fatal("create with dependents must not be single-revertable")
	}
	// 默认拒绝
	if res := r.do(command.Command{Kind: command.KindWorldChange, Action: "revert", Target: a}); res.Accepted {
		t.Fatal("revert with dependents must be rejected without mode")
	}
	if res := r.do(command.Command{Kind: command.KindWorldChange, Action: "revert", Target: c, Item: "single"}); res.Accepted {
		t.Fatal("single revert of referenced create must be rejected")
	}
	// single：当前值保持 b，之后撤销 b 回到最初
	mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Action: "revert", Target: a, Item: "single"})
	if got := r.s.Doc(eng.Pkg, bPlaza).Field("description"); got != "广场上的铜屑被扫干净了。" {
		t.Fatalf("single revert changed the shadowed value: %v", got)
	}
	mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Action: "revert", Target: b})
	if got := r.s.Doc(eng.Pkg, bPlaza).Field("description"); got != orig {
		t.Fatalf("revert b after single(a) = %v, want original %v", got, orig)
	}
	// chain：先撤销 d，再撤销 c
	res := mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Action: "revert", Target: c, Item: "chain"})
	n := 0
	for _, e := range res.Events {
		if e.Type == event.WorldChangeReverted {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("chain revert emitted %d reverts, want 2", n)
	}
	if doc := r.s.Doc(eng.Pkg, npc); doc != nil && doc.Retired == nil {
		t.Fatal("created npc still present after chain revert")
	}
	// 重放
	_, re := brassSetup(t, 7)
	for _, e := range r.all {
		if err := state.Apply(re, e); err != nil {
			t.Fatal(err)
		}
	}
	x, _ := r.s.Marshal()
	y, _ := re.Marshal()
	if !bytes.Equal(x, y) {
		t.Fatal("cascading revert replay differs")
	}
}
