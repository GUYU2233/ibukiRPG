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
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/overlay"
)

// toolLLM 是支持函数调用的确定性假模型：带 tools 的请求先依次返回脚本化的 tool_calls，
// 工具结果回来后回答“好”；叙事（流式）请求返回不改世界的叙事。
type toolLLM struct {
	mu        sync.Mutex
	calls     [][2]string // {函数名, 参数 JSON}
	results   []string    // 工具结果（回灌给模型的内容）
	toolReqs  int
	narration string
	answer    string
}

func toolCallResp(name, args string, n int) *http.Response {
	msg := map[string]any{"content": "", "tool_calls": []any{map[string]any{"id": "call_" + string(rune('a'+n)), "type": "function",
		"function": map[string]any{"name": name, "arguments": args}}}}
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": msg}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}

func (f *toolLLM) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	var req struct {
		Stream   bool `json:"stream"`
		Tools    []any
		Messages []provider.Message
	}
	_ = json.Unmarshal(body, &req)
	if req.Stream {
		n := f.narration
		if n == "" {
			n = "你在炉边忙了一下午。\n<<<WORLD>>>\n{\"v\":1}\n<<<END>>>"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(sse(n))), Header: http.Header{}}, nil
	}
	if len(req.Tools) > 0 && strings.Contains(string(body), "entity_generate") {
		f.toolReqs++
		if last := req.Messages[len(req.Messages)-1]; last.Role == "tool" {
			f.results = append(f.results, last.Content)
		}
		if len(f.calls) > 0 {
			c := f.calls[0]
			f.calls = f.calls[1:]
			return toolCallResp(c[0], c[1], f.toolReqs), nil
		}
		a := f.answer
		if a == "" {
			a = "好"
		}
		return chat(a), nil
	}
	return chat(`{"v":1}`), nil
}

// TestInGameWorldTools：支持函数调用的模型在“打造”回合里通过写入工具提交修改；不合规的提案被拒绝并把原因
// 回灌给模型；数值超出强度预算被缩小；通过的修改与回合一起写入事件日志（来源 AI 叙事，可撤销）。
func TestInGameWorldTools(t *testing.T) {
	ctx := context.Background()
	f := &toolLLM{calls: [][2]string{
		{"world_propose_change", `{"changes":[{"op":"patch","target":"brass:faction/nobody","path":"fields.description","value":"不存在的势力","reason":"编造"}]}`},
		{"entity_generate", `{"kind":"weapon","slug":"spark_knife","name":"电火短刀","description":"用旧扳手的钢料打出来的短刀，刀背嵌着一节小电池。","rarity":"common","level":1,"stats":{"atk":400,"def":50},"reason":"玩家在工坊打造"}`},
	}}
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(ctx, "", "我想在工坊打造一把短刀"); err != nil {
		t.Fatal(err)
	}
	if f.toolReqs < 3 || len(f.results) < 2 {
		t.Fatalf("tool loop: reqs=%d results=%v", f.toolReqs, f.results)
	}
	if !strings.Contains(f.results[0], "rejected") && !strings.Contains(f.results[0], "拒绝") {
		t.Fatalf("rejection not fed back: %s", f.results[0])
	}
	st := mustState(t, s)
	var d *overlay.EntityDoc
	for _, c := range mustChanges(t, s) {
		if strings.HasSuffix(c.Target, "item/spark_knife") {
			d = st.Doc(s.Package(), c.Target)
		}
	}
	if d == nil {
		chs, _ := s.WorldChanges("", "", 0)
		t.Fatalf("generated card not committed (changes %+v, results %v)", chs, f.results)
	}
	if d.Stats["atk"] >= 400 || d.Stats["atk"] <= 0 {
		t.Fatalf("stats not fitted to the power budget: %+v", d.Stats)
	}
	chs, _ := s.WorldChanges("", change.SourceNarrate, 0)
	if len(chs) == 0 {
		t.Fatal("tool change not in the world log with source narrate")
	}
	if _, err := s.RevertChange(ctx, chs[len(chs)-1].ID); err != nil {
		t.Fatal(err)
	}

	// 关闭游戏内写入工具：不再出现带写入工具的请求（WORLD 段 JSON 兜底）
	f2 := &toolLLM{calls: [][2]string{{"entity_generate", `{"kind":"weapon","name":"x","description":"x","reason":"x"}`}}}
	s2 := openPacks(t, f2)
	s2.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}, NoWorldTools: true})
	if _, err := s2.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Submit(ctx, "", "我想在工坊打造一把短刀"); err != nil {
		t.Fatal(err)
	}
	if f2.toolReqs != 0 {
		t.Fatalf("write tools offered while disabled (%d requests)", f2.toolReqs)
	}
}

func mustChanges(t *testing.T, s *orchestrator.Session) []dto.WorldChangeV1 {
	t.Helper()
	chs, err := s.WorldChanges("", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	return chs
}

// TestCardGenViaTools：卡片生成在支持函数调用时走 entity_generate（数值按预算缩放），返回预览，确认后写入。
func TestCardGenViaTools(t *testing.T) {
	ctx := context.Background()
	f := &toolLLM{calls: [][2]string{
		{"rules_power_budget", `{"kind":"weapon","rarity":"rare","level":3}`},
		{"entity_generate", `{"kind":"weapon","slug":"storm_hammer","name":"风暴锤","description":"齿轮城的老式蒸汽锤，锤头里装着压力阀。","rarity":"rare","level":3,"stats":{"atk":999},"reason":"玩家要求"}`},
	}, answer: "做了一把稀有的蒸汽锤"}
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	pv, err := s.RequestEdit(ctx, "", "给我做一把稀有的蒸汽锤")
	if err != nil {
		t.Fatal(err)
	}
	if pv.Via != "tools" || pv.Token == "" || len(pv.Changes) != 1 || pv.Note != "做了一把稀有的蒸汽锤" {
		t.Fatalf("preview %+v (results %v)", pv, f.results)
	}
	if !strings.Contains(f.results[0], "budget") {
		t.Fatalf("power budget result %s", f.results[0])
	}
	if chs, _ := s.WorldChanges("", "", 0); len(chs) != 0 {
		t.Fatal("card gen must not commit before confirmation")
	}
	if _, err := s.ApplyPreview(ctx, pv.Token, 0); err != nil {
		t.Fatal(err)
	}
	d := mustState(t, s).Doc(s.Package(), pv.Changes[0].Target)
	if !strings.HasSuffix(pv.Changes[0].Target, "item/storm_hammer") || d == nil || d.Stats["atk"] >= 999 || d.Stats["level"] != 3 {
		t.Fatalf("card %+v", d)
	}
}
