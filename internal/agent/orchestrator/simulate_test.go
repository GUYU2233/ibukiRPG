package orchestrator_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// simLLM：世界模拟请求返回脚本化的提案；叙事请求返回不改世界的叙事。
type simLLM struct {
	mu    sync.Mutex
	sims  int
	reply string
}

func (f *simLLM) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(string(body), "世界模拟者") {
		f.sims++
		return chat(f.reply), nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(sse("时间过去了。\n<<<WORLD>>>\n{\"v\":1}\n<<<END>>>"))), Header: http.Header{}}, nil
}

const simReply = `{"news":["听说煤烟帮在北码头加了岗哨。"],"changes":[
 {"op":"patch","target":"brass:faction/soot_gang","path":"fields.stance","value":"在北码头加了岗哨，盯着每一辆煤车","reason":"场外：煤烟帮加强码头控制"},
 {"op":"patch","target":"player","path":"gold","value":500,"reason":"不应该被接受"}]}`

func waitHours(t *testing.T, s *orchestrator.Session, target string) dto.TurnV1 {
	t.Helper()
	v, err := s.Quick(context.Background(), "", dto.QuickActionV1{Kind: "wait", Target: target})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func simEntries(v dto.TurnV1) []dto.EntryV1 {
	var out []dto.EntryV1
	for _, e := range v.Entries {
		if e.Kind == "sim" {
			out = append(out, e)
		}
	}
	return out
}

// TestWorldSimOfflineRules：离线时等待会按故事包规则推进场外世界；提交走同一网关（来源 world_sim，可撤销）；
// 同一种子两次运行结果完全相同。
func TestWorldSimOfflineRules(t *testing.T) {
	ctx := context.Background()
	run := func() []dto.WorldChangeV1 {
		s := openPacks(t, nil)
		if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
			t.Fatal(err)
		}
		v := waitHours(t, s, "4h")
		se := simEntries(v)
		if len(se) != 1 || se[0].Text == "" || se[0].Source != "world_sim:rules" {
			t.Fatalf("sim entry %+v (entries %+v)", se, v.Entries)
		}
		chs, _ := s.WorldChanges("", change.SourceWorldSim, 0)
		if len(chs) == 0 {
			t.Fatal("offline simulation committed no change")
		}
		// 可撤销
		if _, err := s.RevertChange(ctx, chs[0].ID); err != nil {
			t.Fatal(err)
		}
		chs, _ = s.WorldChanges("", change.SourceWorldSim, 0)
		return chs
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("non-deterministic: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Summary != b[i].Summary || a[i].Target != b[i].Target {
			t.Fatalf("non-deterministic: %+v vs %+v", a[i], b[i])
		}
	}
	// 关闭后不再模拟
	s := openPacks(t, nil)
	s.ConfigureProviders(nil, router.Settings{SimEvery: -1})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	if se := simEntries(waitHours(t, s, "4h")); len(se) != 0 {
		t.Fatalf("simulation ran while disabled: %+v", se)
	}
}

// TestWorldSimAIAndCostCap：联网时用世界模拟模型（确定性的假模型），针对玩家的修改被丢弃；
// 超过每日 AI 次数上限后回退到规则；短暂的等待不触发模拟。
func TestWorldSimAIAndCostCap(t *testing.T) {
	ctx := context.Background()
	f := &simLLM{reply: simReply}
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}},
		router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}, SimAICalls: 1})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	gold0 := mustState(t, s).Player.Gold
	if se := simEntries(waitHours(t, s, "10m")); len(se) != 0 || f.sims != 0 {
		t.Fatal("a 10-minute wait must not trigger the simulation")
	}
	v := waitHours(t, s, "4h")
	se := simEntries(v)
	if f.sims != 1 || len(se) != 1 || se[0].Source != "world_sim:ai" || !strings.Contains(se[0].Text, "岗哨") {
		t.Fatalf("ai sim: calls=%d entries=%+v", f.sims, se)
	}
	if se[0].World == nil || len(se[0].World.Changes) != 1 || se[0].World.Changes[0].Source != change.SourceWorldSim {
		t.Fatalf("ai sim changes %+v", se[0].World)
	}
	if mustState(t, s).Player.Gold != gold0 {
		t.Fatal("world sim must never touch the player")
	}
	// 成本上限：同一游戏日第二次只用规则
	v = waitHours(t, s, "4h")
	se = simEntries(v)
	if f.sims != 1 || len(se) != 1 || se[0].Source != "world_sim:rules" {
		t.Fatalf("cost cap: calls=%d entries=%+v", f.sims, se)
	}
}

func mustState(t *testing.T, s *orchestrator.Session) *state.State {
	t.Helper()
	st, err := s.State()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestWorldSimSensitivity：场外模拟的影响分同样受偏离提示灵敏度约束；回滚只撤掉场外推进，不撤玩家的回合。
func TestWorldSimSensitivity(t *testing.T) {
	ctx := context.Background()
	f := &simLLM{reply: `{"news":["行会与议会闹翻了。"],"changes":[
 {"op":"patch","target":"brass:faction/guild","path":"fields.description","value":"行会已经与议会决裂，关上了工坊大门，不再收学徒。","reason":"场外：行会与议会决裂"},
 {"op":"patch","target":"brass:faction/council","path":"fields.description","value":"议会宣布接管钟楼，行会不再插手。","reason":"场外：议会接管钟楼"},
 {"op":"timeline_patch","target":"brass:event/trial_day","path":"summary","value":"试炼被议会叫停。","reason":"场外：试炼叫停"}]}`}
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}},
		router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}})
	high := change.Sensitivity{Level: change.SensHigh, Mode: change.ModeModal}
	s.SetPromptSettings(change.Settings{LoreDeviation: high, MajorDeath: high, StoryImpact: high, KeepAuto: 5})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	waitHours(t, s, "4h")
	d, err := s.PendingDecision()
	if err != nil {
		t.Fatal(err)
	}
	if d == nil || !d.World || !strings.HasPrefix(d.Title, "场外世界") {
		chs, _ := s.WorldChanges("", "", 0)
		t.Fatalf("expected a world-sim decision, got %+v (changes %+v)", d, chs)
	}
	turn := mustState(t, s).Turn
	if _, err := s.ResolveDecision(ctx, d.ID, "rollback", false); err != nil {
		t.Fatal(err)
	}
	st := mustState(t, s)
	if st.Turn != turn {
		t.Fatalf("rollback of a world-sim decision must keep the player's turn: %d → %d", turn, st.Turn)
	}
	chs, _ := s.WorldChanges("", change.SourceWorldSim, 0)
	if len(chs) != 3 {
		t.Fatalf("sim changes %+v", chs)
	}
	for _, c := range chs {
		if c.RevertedBy == "" {
			t.Fatalf("sim change survived rollback: %+v", c)
		}
	}
	if d, _ := s.PendingDecision(); d != nil {
		t.Fatalf("decision still pending: %+v", d)
	}
}

// localLLM 模拟 llama.cpp 本地模型：叙事带精简格式的 WORLD 段；世界模拟用精简格式回答。
type localLLM struct {
	mu            sync.Mutex
	compactPrompt bool
}

func (f *localLLM) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(string(body), "世界模拟者") {
		f.compactPrompt = strings.Contains(string(body), `\"edits\"`)
		return chat(`{"news":["拾荒者在管道口点起了篝火。"],"edits":[{"id":"brass:faction/scavengers","field":"stance","text":"在管道口聚集取暖"}]}`), nil
	}
	if !strings.Contains(string(body), `\"edits\"`) {
		return chat("（缺少精简格式说明）"), nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(sse("你看了看工坊，炉膛里的火更旺了。\n<<<WORLD>>>\n" +
		`{"v":1,"edits":[{"id":"brass:location/workshop","field":"description","text":"炉火正旺的工坊，墙上挂满齿轮。","why":"炉火更旺了"}]}` + "\n<<<END>>>"))), Header: http.Header{}}, nil
}

// TestLocalModelReducedSchema：本地 llama.cpp 模型用精简格式也能做有限的世界修改（叙事与场外模拟都一样走校验器）。
func TestLocalModelReducedSchema(t *testing.T) {
	ctx := context.Background()
	f := &localLLM{}
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "local", Kind: provider.KindLlamaCpp, BaseURL: "http://127.0.0.1:8080/v1", Model: "qwen2.5-7b", APIKey: "local-token"}},
		router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "local"}})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	look(t, s)
	chs, _ := s.WorldChanges("", change.SourceNarrate, 0)
	if len(chs) != 1 || chs[0].Path != "fields.description" {
		tr, _ := s.Transcript(ctx, 20, 0)
		all, _ := s.WorldChanges("", "", 0)
		t.Fatalf("local narrate compact world: %+v\n%+v\n%+v", chs, tr, all)
	}
	v := waitHours(t, s, "4h")
	se := simEntries(v)
	if !f.compactPrompt || len(se) != 1 || se[0].Source != "world_sim:ai" || se[0].World == nil || len(se[0].World.Changes) != 1 {
		t.Fatalf("local sim: compact=%v entries=%+v", f.compactPrompt, se)
	}
}
