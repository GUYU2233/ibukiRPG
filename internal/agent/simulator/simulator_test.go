package simulator

import (
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

func TestParseFullAndClean(t *testing.T) {
	out, err := Parse("好的：\n"+`{"news":["码头加了岗哨。",""],"changes":[
	 {"op":"patch","target":"x:faction/a","path":"fields.stance","value":"戒备","reason":"加强戒备"},
	 {"op":"patch","target":"player","path":"gold","value":99,"reason":"x"},
	 {"op":"retire","target":"x:character/b","value":{"reason":"death"},"reason":"x"},
	 {"op":"patch","target":"x:character/b>x:character/c","path":"dims.trust","value":-5}]}`, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.News) != 1 || len(out.Changes) != 2 {
		t.Fatalf("%+v", out)
	}
	if out.Changes[0].Reason != "场外：加强戒备" || out.Changes[1].Reason != "场外推进" {
		t.Fatalf("reasons %q %q", out.Changes[0].Reason, out.Changes[1].Reason)
	}
}

// 本地模型的精简格式：edits / rel 转成标准变更，最多 2 项。
func TestParseCompact(t *testing.T) {
	out, err := Parse(`{"news":["行会在招人。"],"edits":[{"id":"x:faction/a","field":"stance","text":"招人"},{"id":"x:faction/b","field":"mood","text":"紧张"}],
	 "rel":[{"a":"x:character/b","b":"x:character/c","dim":"hostility","delta":3}]}`, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Changes) != MaxCompactChanges {
		t.Fatalf("compact limit: %+v", out.Changes)
	}
	if c := out.Changes[0]; c.Op != change.OpPatch || c.Path != "fields.stance" || string(c.Value) != `"招人"` {
		t.Fatalf("edit → %+v", c)
	}
	if _, err := Parse("not json at all", true); err == nil {
		t.Fatal("garbage must fail")
	}
}

// 规则选择是确定性的：同一时间段选同一条；once 规则触发过就跳过；条件不成立的跳过。
func TestPick(t *testing.T) {
	sim := loader.Simulation{Rules: []loader.SimRule{
		{ID: "a", On: []string{"time", "wait"}, News: "A"},
		{ID: "b", On: []string{"wait"}, News: "B"},
		{ID: "c", On: []string{"time"}, When: "false", News: "C"},
		{ID: "d", On: []string{"time"}, Once: true, News: "D", Changes: []loader.SimChange{{Op: "patch", Target: "x", Path: "fields.y", Value: "z"}}},
	}}
	eval := func(e string) (bool, error) { return e != "false", nil }
	fired := map[string]bool{}
	f := func(id string) bool { return fired[id] }
	if r := Pick(sim, "time", 0, eval, f); r == nil || r.ID != "a" {
		t.Fatalf("time p0 → %+v", r)
	}
	if r := Pick(sim, "time", 1, eval, f); r == nil || r.ID != "d" {
		t.Fatalf("time p1 → %+v", r)
	}
	fired["d"] = true
	if r := Pick(sim, "time", 1, eval, f); r == nil || r.ID != "a" {
		t.Fatalf("once rule repeated: %+v", r)
	}
	// wait 也接受 time 规则
	if r := Pick(sim, "wait", 1, eval, f); r == nil || r.ID != "b" {
		t.Fatalf("wait p1 → %+v", r)
	}
	o := RuleOutput(&sim.Rules[3])
	if len(o.Changes) != 1 || o.Changes[0].Reason != "场外：D" || string(o.Changes[0].Value) != `"z"` {
		t.Fatalf("rule output %+v", o)
	}
}
