package engine_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/combat/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/combat/sim"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
)

func raw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }

func freeCmd(eng *engine.Engine, s *state.State, text string) command.Command {
	u := s.RPG.Combat.Current()
	in := freeform.RulesIntent(text, engine.FreeContext(eng.Pkg, s, u))
	return command.Command{Kind: command.KindCombat, Action: "freeform", Intent: &in}
}

// TestFreeCombat：自由战斗走合理性检查 → 掷骰 → 程度判定，事件流可重放；荒谬行动被拒绝或降级。
func TestFreeCombat(t *testing.T) {
	eng, s0 := brassSetup(t, 5)
	s0.Player.Location = bArena
	r := &runner{t: t, eng: eng, s: s0}
	mustAccept(t, r, command.Command{Kind: command.KindCombat, Action: "start", Target: "brass:encounter/practice"})
	adjudicated := 0
	for i := 0; i < 60 && r.s.RPG.Combat != nil; i++ {
		var res *engine.Result
		if r.s.RPG.Combat.Current().ID == "player" {
			res = r.do(freeCmd(eng, r.s, "扫腿绊倒发条鼠，再抡扳手砸它背上的发条钥匙"))
		}
		if res == nil || !res.Accepted {
			res = mustAccept(t, r, sim.Policy(eng, r.s))
		}
		if has(res, event.ActionAdjudicated) {
			adjudicated++
		}
	}
	if adjudicated == 0 {
		t.Fatal("no freeform adjudication happened")
	}
	_, re := brassSetup(t, 5)
	re.Player.Location = bArena
	for _, e := range r.all {
		if err := state.Apply(re, e); err != nil {
			t.Fatal(err)
		}
	}
	a, _ := r.s.Marshal()
	b, _ := re.Marshal()
	if !bytes.Equal(a, b) {
		t.Fatal("freeform combat replay differs")
	}

	// 荒谬行动
	eng, s0 = brassSetup(t, 6)
	s0.Player.Location = bArena
	r = &runner{t: t, eng: eng, s: s0}
	mustAccept(t, r, command.Command{Kind: command.KindCombat, Action: "start", Target: "brass:encounter/practice"})
	for r.s.RPG.Combat.Current().ID != "player" {
		mustAccept(t, r, sim.Policy(eng, r.s))
	}
	res := r.do(freeCmd(eng, r.s, "我念咒语放一个火球烧死发条鼠"))
	if res.Accepted {
		for _, e := range res.Events {
			if e.Type == event.ActionAdjudicated && e.Data.Adjudication != nil && !e.Data.Adjudication.Check.Downgraded {
				t.Fatalf("absurd action was neither rejected nor downgraded: %+v", e.Data.Adjudication.Check)
			}
		}
	}
}

// TestFreeformProfileFromPack：随机性档位只来自故事包。
func TestFreeformProfileFromPack(t *testing.T) {
	eng, _ := brassSetup(t, 1)
	if p := engine.Profile(eng.Pkg); p.Randomness != 45 || p.Band == "" {
		t.Fatalf("profile %+v", p)
	}
}

// TestWorldChangeGateway：AI 世界变更经校验 → 事件 → 覆盖层；数值被封顶；可撤销；全部被拒时记录失败。
func TestWorldChangeGateway(t *testing.T) {
	eng, s0 := brassSetup(t, 7)
	r := &runner{t: t, eng: eng, s: s0}
	mustAccept(t, r, bAct("look", ""))
	gold0 := r.s.Player.Gold
	res := mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Source: change.SourceNarrate, Changes: []change.Change{
		{Op: change.OpPatch, Target: bPlaza, Path: "fields.description", Value: raw("钟楼广场的石板缝里长出了铜绿色的苔藓。"), Reason: "叙事细节"},
		{Op: change.OpPatch, Target: "player", Path: "gold", Value: raw(9999), Reason: "捡到钱袋"},
		{Op: change.OpPatch, Target: "brass:location/nowhere", Path: "fields.description", Value: raw("x"), Reason: "坏引用"},
	}})
	if !has(res, event.WorldChangeApplied) || !has(res, event.GoldChanged) {
		t.Fatal("changes not applied")
	}
	if len(res.Rejects) != 1 {
		t.Fatalf("rejects %+v", res.Rejects)
	}
	if got := r.s.Player.Gold - gold0; got <= 0 || got > 60 {
		t.Fatalf("gold cap not applied: +%d", got)
	}
	d := r.s.Doc(eng.Pkg, bPlaza)
	if d.Field("description") != "钟楼广场的石板缝里长出了铜绿色的苔藓。" {
		t.Fatalf("overlay not applied: %v", d.Field("description"))
	}
	var id string
	for _, e := range res.Events {
		if e.Type == event.WorldChangeApplied && e.Data.Change.Target == bPlaza {
			id = e.Data.Change.ID
		}
	}
	mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Action: "revert", Target: id})
	if r.s.Doc(eng.Pkg, bPlaza).Field("description") == "钟楼广场的石板缝里长出了铜绿色的苔藓。" {
		t.Fatal("revert did not restore description")
	}
	res = mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Changes: []change.Change{{Op: change.OpPatch, Target: "nope", Path: "fields.x", Value: raw(1)}}})
	if !has(res, event.WorldUpdateFailed) {
		t.Fatal("all-rejected must record WorldUpdateFailed")
	}
}

// TestKnowledgeLayer：一切从未知开始；start.known 预置；目击揭示；隐藏真相不能被普通揭示泄露。
func TestKnowledgeLayer(t *testing.T) {
	eng, s0 := brassSetup(t, 8)
	if err := engine.Setup(eng.Pkg, s0, engine.Creation{}); err != nil {
		t.Fatal(err)
	}
	if s0.Knowledge.Knows(bTock, "name") {
		t.Fatal("tock must start unknown")
	}
	if !s0.Knowledge.Knows(bOrin, "name") || s0.Knowledge.Knows(bOrin, "background") {
		t.Fatalf("start.known not applied correctly")
	}
	r := &runner{t: t, eng: eng, s: s0}
	// 欧琳在场：对白揭示她的性格（她知道自己的事）
	res := mustAccept(t, r, command.Command{Kind: command.KindWorldChange, Reveals: []knowledge.Reveal{
		{Entity: bOrin, Fields: []string{"personality"}, Channel: knowledge.ChannelDialogue, Source: bOrin},
		{Entity: bTock, Fields: []string{"background"}, Channel: knowledge.ChannelWitness}, // 托克不在场
	}})
	if !r.s.Knowledge.Knows(bOrin, "personality") {
		t.Fatal("dialogue reveal not applied")
	}
	if r.s.Knowledge.Knows(bTock, "background") || len(res.Rejects) == 0 {
		t.Fatal("witness reveal of an absent NPC must be rejected")
	}
	// 隐藏真相有 reveal_when：条件不满足时拒绝
	res = r.do(command.Command{Kind: command.KindWorldChange, Reveals: []knowledge.Reveal{
		{Entity: bOrin, Fields: []string{"hidden:blueprints"}, Channel: knowledge.ChannelDialogue, Source: bOrin},
	}})
	if r.s.Knowledge.Knows(bOrin, "hidden:blueprints") {
		t.Fatal("gated hidden truth leaked")
	}
	_ = res
}

// TestFreeHiddenTruthImpact：没有 reveal_when 的隐藏真相可以揭示（决定 1），并计入 story_impact。
func TestFreeHiddenTruthImpact(t *testing.T) {
	eng, s0 := brassSetup(t, 9)
	s0.Player.Location = "brass:location/plaza"
	r := &runner{t: t, eng: eng, s: s0}
	mustAccept(t, r, bAct("look", ""))
	if r.s.NPCs[bRivet] != nil && r.s.NPCs[bRivet].Location != r.s.Player.Location {
		r.s.NPCs[bRivet].Location = r.s.Player.Location
	}
	res := r.do(command.Command{Kind: command.KindWorldChange, Reveals: []knowledge.Reveal{
		{Entity: bRivet, Fields: []string{"hidden:scav_kin"}, Channel: knowledge.ChannelDialogue, Source: bRivet},
	}})
	if !res.Accepted || !r.s.Knowledge.Knows(bRivet, "hidden:scav_kin") {
		t.Fatalf("free hidden truth should be revealable: %+v", res.Rejects)
	}
	if res.Impact == nil || res.Impact.HiddenReveals != 1 {
		t.Fatalf("impact %+v", res.Impact)
	}
}

// TestDecisionBlocksTurns：偏离提示待决时只接受选择命令。
func TestDecisionBlocksTurns(t *testing.T) {
	eng, s0 := brassSetup(t, 10)
	r := &runner{t: t, eng: eng, s: s0}
	mustAccept(t, r, command.Command{Kind: command.KindDecision, Action: "request", Decision: &change.Decision{ID: "d1", Type: change.TypeLoreDeviation, Score: 70}})
	if res := r.do(bAct("look", "")); res.Accepted {
		t.Fatal("turn must be blocked while a decision is pending")
	}
	mustAccept(t, r, command.Command{Kind: command.KindDecision, Action: "accept", Target: "d1"})
	mustAccept(t, r, bAct("look", ""))
}

// TestTimelineRuns：等待推进时间，世界事件按日程开始 / 结算，传闻扩散到玩家所在地。
func TestTimelineRuns(t *testing.T) {
	eng, s0 := brassSetup(t, 12)
	r := &runner{t: t, eng: eng, s: s0}
	if r.s.Timeline.Status("brass:event/trial_day") == timeline.Resolved {
		t.Fatal("trial day resolved at start")
	}
	for i := 0; i < 12 && r.s.Timeline.Status("brass:event/trial_day") != timeline.Resolved; i++ {
		res := r.do(command.Command{Kind: command.KindWait, Target: "6h"})
		if !res.Accepted {
			t.Fatalf("wait rejected: %s", res.Reason)
		}
	}
	if r.s.Timeline.Status("brass:event/trial_day") != timeline.Resolved {
		t.Fatalf("trial day status %s", r.s.Timeline.Status("brass:event/trial_day"))
	}
	if r.s.Timeline.Status("brass:event/market") != timeline.Resolved {
		t.Fatalf("market status %s", r.s.Timeline.Status("brass:event/market"))
	}
}

// TestBoilerHoundParts：精英敌人锅炉看门犬的部位（膝关节弱点 / 锅炉未知）进入自由战斗上下文；
// 瞄准已知弱点的攻击会被解析到该部位并得到修正。
func TestBoilerHoundParts(t *testing.T) {
	eng, s0 := brassSetup(t, 11)
	s0.Player.Location = "brass:location/sewer"
	s0.RPG.Encounters["brass:encounter/sewer_ambush"] = &state.EncounterRecord{Result: "victory", Wins: 1}
	r := &runner{t: t, eng: eng, s: s0}
	mustAccept(t, r, command.Command{Kind: command.KindCombat, Action: "start", Target: "brass:encounter/boiler_hound"})
	for r.s.RPG.Combat != nil && r.s.RPG.Combat.Current().ID != "player" {
		mustAccept(t, r, sim.Policy(eng, r.s))
	}
	if r.s.RPG.Combat == nil {
		t.Skip("combat ended before the player acted")
	}
	fc := engine.FreeContext(eng.Pkg, r.s, r.s.RPG.Combat.Current())
	var parts map[string]bool
	for _, tg := range fc.Targets {
		if strings.Contains(tg.ID, "boiler_hound") || tg.Name == "锅炉看门犬" {
			parts = map[string]bool{}
			for _, p := range tg.Parts {
				parts[p.ID] = p.Known
			}
		}
	}
	if parts == nil || len(parts) != 3 {
		t.Fatalf("hound parts missing: %+v", fc.Targets)
	}
	if parts["boiler"] {
		t.Fatal("boiler should start unknown")
	}
	res := r.do(freeCmd(eng, r.s, "滑到它腿下面，用扳手卡进膝关节"))
	if !res.Accepted || !has(res, event.ActionAdjudicated) {
		t.Fatalf("knee attack not adjudicated: %+v", res)
	}
}
