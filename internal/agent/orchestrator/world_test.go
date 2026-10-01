package orchestrator_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// fakeLLM 是确定性的假模型：叙事请求依次返回 narr 中的脚本，世界更新重试请求依次返回 retry 中的脚本。
type fakeLLM struct {
	mu     sync.Mutex
	narr   []string
	retry  []string
	status int // 非 0：叙事请求返回该 HTTP 状态（模拟内容审核）
	calls  int
}

func sse(text string) string {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": text}}}, "usage": map[string]any{"prompt_tokens": 1200, "completion_tokens": 300}})
	return "data: " + string(b) + "\n\ndata: [DONE]\n\n"
}

func (f *fakeLLM) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	resp := func(code int, s string) *http.Response {
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(s)), Header: http.Header{}}
	}
	if strings.Contains(string(body), "世界更新器") {
		out := `{"v":1}`
		if len(f.retry) > 0 {
			out, f.retry = f.retry[0], f.retry[1:]
		}
		return chat(out), nil
	}
	if f.status != 0 {
		return resp(f.status, `{"error":{"code":"data_inspection_failed","message":"Output data may contain inappropriate content."}}`), nil
	}
	out := "你环顾四周。\n<<<WORLD>>>\n{\"v\":1}\n<<<END>>>"
	if len(f.narr) > 0 {
		out, f.narr = f.narr[0], f.narr[1:]
	}
	return resp(200, sse(out)), nil
}

func onlineBrass(t *testing.T, f *fakeLLM) *orchestrator.Session {
	t.Helper()
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}, ShowUsage: true})
	if _, err := s.NewGameIn(context.Background(), "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	return s
}

func look(t *testing.T, s *orchestrator.Session) dto.TurnV1 {
	t.Helper()
	v, err := s.Quick(context.Background(), "", dto.QuickActionV1{Kind: "action", Action: "brass:action/look", Label: "环顾四周"})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Accepted {
		t.Fatalf("look rejected: %+v", v.Entries)
	}
	return v
}

func worldEntry(v dto.TurnV1) *dto.WorldLogV1 {
	for _, e := range v.Entries {
		if e.Kind == "world" {
			return e.World
		}
	}
	return nil
}

// TestMergedWorldUpdate：叙事 + WORLD 段合并输出 → 校验 → 事件 → 叙事流 chip；token 用量按回合记录。
func TestMergedWorldUpdate(t *testing.T) {
	f := &fakeLLM{narr: []string{"欧琳抬起头，护目镜下的眼神很严厉。“先学会修，再学会打。”\n<<<WORLD>>>\n" +
		`{"v":1,"changes":[{"op":"patch","target":"brass:location/workshop","path":"fields.description","value":"炉火通红，墙上新挂了一张试炼名单。","reason":"叙事细节"}],` +
		`"reveals":[{"entity":"brass:character/orin","fields":["personality"],"channel":"dialogue","source":"brass:character/orin"}],` +
		`"scene_facts":["墙上挂着试炼名单"],"impact":{"level":"minor"},"suggestions":["去竞技场","问问试炼的规矩"]}` + "\n<<<END>>>"}}
	s := onlineBrass(t, f)
	v := look(t, s)
	w := worldEntry(v)
	if w == nil || len(w.Changes) != 1 || len(w.Reveals) != 1 {
		t.Fatalf("world entry %+v (entries %s)", w, kinds(v))
	}
	for _, e := range v.Entries {
		if e.Kind == "narration" && strings.Contains(e.Text, "<<<WORLD>>>") {
			t.Fatal("WORLD section leaked into narration")
		}
	}
	if v.Usage == nil || v.Usage.Calls == 0 || v.Usage.PromptTokens == 0 {
		t.Fatalf("usage %+v", v.Usage)
	}
	if len(v.Scene.AISuggestions) != 2 {
		t.Fatalf("ai suggestions %v", v.Scene.AISuggestions)
	}
	chs, _ := s.WorldChanges("", "", 0)
	if len(chs) != 1 || chs[0].SourceLabel != "AI 叙事" {
		t.Fatalf("change log %+v", chs)
	}
	if _, err := s.RevertChange(context.Background(), chs[0].ID); err != nil {
		t.Fatal(err)
	}
	if chs, _ = s.WorldChanges("", "", 0); len(chs) != 2 || chs[1].RevertedBy == "" && chs[0].Reverts == "" {
		t.Fatalf("revert not logged: %+v", chs)
	}
}

// TestBrokenWorldJSON：坏 JSON 先本地修复，再重试一次，最后降级为仅叙事（叙事保留，世界更新跳过）。
func TestBrokenWorldJSON(t *testing.T) {
	// 截断 JSON：本地修复即可
	f := &fakeLLM{narr: []string{"你看了看。\n<<<WORLD>>>\n{\"v\":1,\"scene_facts\":[\"炉火很旺\"],\"reveals\":[{\"entity\":\"brass:character/orin\",\"fields\":[\"personality\"],\"channel\":\"dialogue\",\"source\":\"brass:character/orin\"},{\"entity\":\"brass:char"}}
	s := onlineBrass(t, f)
	v := look(t, s)
	if w := worldEntry(v); w == nil || len(w.Reveals) != 1 {
		t.Fatalf("repaired world %+v", w)
	}
	// 修不好 → 重试成功
	f = &fakeLLM{narr: []string{"你看了看。\n<<<WORLD>>>\n完全不是 JSON"}, retry: []string{`{"v":1,"reveals":[{"entity":"brass:character/orin","fields":["personality"],"channel":"dialogue","source":"brass:character/orin"}]}`}}
	s = onlineBrass(t, f)
	if w := worldEntry(look(t, s)); w == nil || len(w.Reveals) != 1 {
		t.Fatalf("retry world %+v", w)
	}
	// 重试也失败 → 仅叙事 + 失败记录
	f = &fakeLLM{narr: []string{"你看了看。\n<<<WORLD>>>\n完全不是 JSON"}, retry: []string{"还是不是"}}
	s = onlineBrass(t, f)
	v = look(t, s)
	w := worldEntry(v)
	if w == nil || w.Failed != "parse" {
		t.Fatalf("expected parse failure, got %+v", w)
	}
	var narr string
	for _, e := range v.Entries {
		if e.Kind == "narration" {
			narr = e.Text
		}
	}
	if !strings.Contains(narr, "你看了看") || !strings.Contains(narr, "世界变化没能记录") {
		t.Fatalf("narration %q", narr)
	}
}

// TestContentRefusal：内容审核拒绝时不写任何 AI 内容，叙事用模板，记录 refused。
func TestContentRefusal(t *testing.T) {
	f := &fakeLLM{status: 400}
	s := onlineBrass(t, f)
	v := look(t, s)
	w := worldEntry(v)
	if w == nil || w.Failed != "refused" {
		t.Fatalf("expected refused, got %+v", w)
	}
	for _, e := range v.Entries {
		if e.Kind == "narration" && !strings.Contains(e.Text, "内容审核") {
			t.Fatalf("narration should explain the refusal: %q", e.Text)
		}
	}
}

// TestDeviationRollbackBranch：重要人物死亡 → 自动检查点 + 偏离提示（输入禁用）→ 回到上一回合 →
// 下一次行动分出新分支，原分支保留为“已回滚”。
func TestDeviationRollbackBranch(t *testing.T) {
	ctx := context.Background()
	f := &fakeLLM{narr: []string{
		"你环顾工坊。\n<<<WORLD>>>\n{\"v\":1}\n<<<END>>>",
		"锅炉炸开了，欧琳没能逃出来。\n<<<WORLD>>>\n" + `{"v":1,"changes":[{"op":"retire","target":"brass:character/orin","value":{"reason":"dead","text":"死于锅炉爆炸"},"reason":"锅炉爆炸"}],"impact":{"level":"break"}}` + "\n<<<END>>>",
	}}
	s := onlineBrass(t, f)
	look(t, s)
	v := look(t, s)
	if v.Scene.Decision == nil || v.Scene.Decision.Type != change.TypeMajorDeath || v.Scene.Decision.Checkpoint == "" {
		t.Fatalf("decision %+v", v.Scene.Decision)
	}
	if r, _ := s.Quick(ctx, "", dto.QuickActionV1{Kind: "action", Action: "brass:action/look"}); r.Accepted {
		t.Fatal("turns must be blocked while a decision is pending")
	}
	tl, _ := s.Timeline(ctx)
	if len(tl.Checkpoints) == 0 || tl.Checkpoints[0].Kind != "auto" {
		t.Fatalf("auto checkpoint %+v", tl.Checkpoints)
	}
	if _, err := s.ResolveDecision(ctx, v.Scene.Decision.ID, "rollback", false); err != nil {
		t.Fatal(err)
	}
	sc, _ := s.Scene(ctx)
	if sc.PendingTurn != 1 || sc.Decision != nil {
		t.Fatalf("after rollback: pending %d decision %+v", sc.PendingTurn, sc.Decision)
	}
	look(t, s) // 分出新分支
	tl, _ = s.Timeline(ctx)
	if len(tl.Branches) != 2 {
		t.Fatalf("branches %+v", tl.Branches)
	}
	var rolled bool
	for _, b := range tl.Branches {
		if b.ID == eventstore.MainBranch && b.Status == eventstore.BranchRolledBack {
			rolled = true
		}
	}
	if !rolled {
		t.Fatalf("original branch must be kept as rolled_back: %+v", tl.Branches)
	}
	sc, _ = s.Scene(ctx)
	if sc.PendingTurn != 0 || sc.Turn != 2 {
		t.Fatalf("new branch head turn %d pending %d", sc.Turn, sc.PendingTurn)
	}
	// 欧琳在新分支上还活着
	if chs, _ := s.WorldChanges("", "", 0); len(chs) != 0 {
		t.Fatalf("new branch should not carry the death: %+v", chs)
	}
	// 切回原分支：死亡仍在
	if err := s.SwitchBranch(ctx, eventstore.MainBranch); err != nil {
		t.Fatal(err)
	}
	if chs, _ := s.WorldChanges("", "", 0); len(chs) != 1 {
		t.Fatalf("main branch lost its history: %+v", chs)
	}
}

// TestNotifyOnlyMode：仅通知模式不打断输入。
func TestNotifyOnlyMode(t *testing.T) {
	f := &fakeLLM{narr: []string{"欧琳倒下了。\n<<<WORLD>>>\n" + `{"v":1,"changes":[{"op":"retire","target":"brass:character/orin","value":{"reason":"dead","text":"x"},"reason":"x"}]}`}}
	s := onlineBrass(t, f)
	set := change.DefaultSettings()
	set.MajorDeath.Mode = change.ModeNotify
	s.SetPromptSettings(set)
	v := look(t, s)
	if v.Scene.Decision == nil || !v.Scene.Decision.Notify {
		t.Fatalf("notify decision %+v", v.Scene.Decision)
	}
	look(t, s) // 不被阻塞
}

// TestManualCheckpointRestore：手动检查点 → 回到检查点 → 取消回滚。
func TestManualCheckpointRestore(t *testing.T) {
	ctx := context.Background()
	s := onlineBrass(t, &fakeLLM{})
	look(t, s)
	cp, err := s.CreateCheckpoint(ctx, "进竞技场之前", 0)
	if err != nil || cp.Turn != 1 {
		t.Fatalf("checkpoint %+v %v", cp, err)
	}
	look(t, s)
	look(t, s)
	if err := s.RestoreCheckpoint(ctx, cp.ID); err != nil {
		t.Fatal(err)
	}
	if sc, _ := s.Scene(ctx); sc.Turn != 1 || sc.PendingTurn != 1 {
		t.Fatalf("restore: turn %d pending %d", sc.Turn, sc.PendingTurn)
	}
	if err := s.CancelRollback(ctx); err != nil {
		t.Fatal(err)
	}
	if sc, _ := s.Scene(ctx); sc.Turn != 3 || sc.PendingTurn != 0 {
		t.Fatalf("cancel: turn %d pending %d", sc.Turn, sc.PendingTurn)
	}
	if err := s.RollbackTo(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if sc, _ := s.Scene(ctx); sc.Turn != 0 {
		t.Fatalf("rollback to start: turn %d", sc.Turn)
	}
}
