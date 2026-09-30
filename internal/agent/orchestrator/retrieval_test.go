package orchestrator_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/transport"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/packages"
)

// 记忆 Agent：离线打 24 个回合后应有滚动摘要（记录了压缩掉什么）与长期记忆，并能被 memory.search 检到；
// 默认在后台运行，Close 会等它结束。
func TestMemoryAgentRollingAndLongTerm(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "mem.db")
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: path, Package: packages.Demo()})
	if err != nil {
		t.Fatal(err)
	}
	slot, err := s.NewGame(ctx, "", "阿澈", 3)
	if err != nil {
		t.Fatal(err)
	}
	inputs := []string{"环顾四周", "和伯林聊聊", "点一杯麦酒", "和米拉聊聊", "看看告示板", "和莉娜聊聊"}
	for i := 0; i < 36; i++ {
		if _, err := s.Submit(ctx, "", inputs[i%len(inputs)]); err != nil {
			t.Fatal(err)
		}
	}
	ms, err := s.Memories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, m := range ms {
		kinds[m.Kind]++
		if len(m.Compressed) == 0 || m.Text == "" || m.FromTurn > m.ToTurn {
			t.Errorf("bad summary: %+v", m)
		}
	}
	t.Logf("summaries: %v", kinds)
	for _, m := range ms {
		t.Logf("%s %s %d-%d %s %v", m.Kind, m.Subject, m.FromTurn, m.ToTurn, m.Text, m.Compressed)
	}
	if kinds["rolling"] == 0 || kinds["longterm"] == 0 || kinds["rolling"] > 4 {
		t.Fatalf("expected rolling + longterm summaries, got %v", kinds)
	}
	env, err := s.ToolEnv(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := tools.Call(env, tools.Player(), "memory.search", `{"query":"麦酒"}`)
	if !strings.Contains(out, "摘要") || !strings.Contains(out, "已压缩省略") {
		t.Fatalf("memory.search should find compressed summaries: %s", out)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// 读档后摘要仍在
	s2, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: path, Package: packages.Demo()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s2.Close() }()
	if err := s2.LoadGame(ctx, slot); err != nil {
		t.Fatal(err)
	}
	ms2, _ := s2.Memories(ctx)
	if len(ms2) != len(ms) {
		t.Fatalf("summaries lost after reload: %d vs %d", len(ms2), len(ms))
	}
}

type reqLog struct {
	mu   sync.Mutex
	reqs []string
}

func (l *reqLog) add(b string) { l.mu.Lock(); l.reqs = append(l.reqs, b); l.mu.Unlock() }

func chatMsg(msg map[string]any) *http.Response {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": msg}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}

// 自由推演模式下叙述者先检索再叙述（录制传输）：导演提案前查设定；叙述者调用 pack_search 后给出叙事，
// 叙事经过 Guard 后落盘；同一录音回放得到相同结果。
func TestNarratorLooksUpInFreeMode(t *testing.T) {
	dir := t.TempDir()
	const answer = "你在广场上停下脚步，想起欧琳说过的话。钟楼的影子斜斜落在石板上，远处传来齿轮的低鸣。"
	run := func(rt http.RoundTripper, log *reqLog) dto.TurnV1 {
		s := openPacks(t, rt)
		ctx := context.Background()
		if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 5); err != nil {
			t.Fatal(err)
		}
		deviate(t, s)
		s.ConfigureAI(provider.Config{Kind: provider.KindDeepSeek, APIKey: "sk-test"})
		if _, err := s.Quick(ctx, "", dto.QuickActionV1{Kind: "mainline", Action: "free", Label: "进入自由推演"}); err != nil {
			t.Fatal(err)
		}
		v, err := s.Quick(ctx, "", dto.QuickActionV1{Kind: "action", Action: "brass:action/look", Label: "打听一下欧琳"})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	upstream := func(log *reqLog) http.RoundTripper {
		return transport.Func(func(r *http.Request) (*http.Response, error) {
			b, _ := io.ReadAll(r.Body)
			body := string(b)
			log.add(body)
			var req struct {
				Tools    []any `json:"tools"`
				Messages []struct {
					Role string `json:"role"`
				} `json:"messages"`
			}
			_ = json.Unmarshal(b, &req)
			last := req.Messages[len(req.Messages)-1].Role
			switch {
			case strings.Contains(body, "剧情导演") && last != "tool":
				return chatMsg(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "d1", "type": "function", "function": map[string]any{"name": "story_anchors", "arguments": "{}"}}}}), nil
			case strings.Contains(body, "剧情导演"):
				return chatMsg(map[string]any{"role": "assistant", "content": `{"title":"托克的请求","objective":"去竞技场找托克问问下水道的事。","goal":"talk","ref":"brass:character/tock","reward_xp":40,"summary":"托克似乎知道内情。"}`}), nil
			case strings.Contains(body, "叙述者") && len(req.Tools) > 0 && last != "tool":
				return chatMsg(map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "n1", "type": "function", "function": map[string]any{"name": "pack_search", "arguments": `{"query":"欧琳"}`}}}}), nil
			case strings.Contains(body, "叙述者") && last == "tool":
				return chatMsg(map[string]any{"role": "assistant", "content": answer}), nil
			}
			return &http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader("{}")), Header: http.Header{}}, nil
		})
	}
	log := &reqLog{}
	v := run(&transport.Recorded{Dir: dir, Upstream: upstream(log)}, log)
	if got := narration(v); got != answer {
		t.Fatalf("narration = %q (source?) entries=%+v", got, v.Entries)
	}
	var sawDirTool, sawNarTool, sawPolicy bool
	for _, b := range log.reqs {
		sawDirTool = sawDirTool || (strings.Contains(b, "剧情导演") && strings.Contains(b, `"role":"tool"`))
		sawNarTool = sawNarTool || (strings.Contains(b, "叙述者") && strings.Contains(b, `"role":"tool"`) && strings.Contains(b, "欧琳"))
		sawPolicy = sawPolicy || (strings.Contains(b, "[RETRIEVAL_POLICY]") && strings.Contains(b, "自由推演模式"))
	}
	if !sawDirTool || !sawNarTool || !sawPolicy {
		t.Fatalf("director tool=%v narrator tool=%v policy=%v", sawDirTool, sawNarTool, sawPolicy)
	}
	v2 := run(&transport.Recorded{Dir: dir}, nil)
	if narration(v2) != answer {
		t.Fatalf("replay differs: %q", narration(v2))
	}
}

// 主线模式、没有未知名词时不检索：提示词保持不变（旧录音仍然有效），也不多一次请求。
func TestNoLookupWhenContextSuffices(t *testing.T) {
	var n int
	var mu sync.Mutex
	rt := transport.Func(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), `"tools"`) || strings.Contains(string(b), "[RETRIEVAL_POLICY]") {
			t.Errorf("unexpected retrieval request: %.200s", b)
		}
		mu.Lock()
		n++
		mu.Unlock()
		return chat("你环顾四周，酒馆里暖烘烘的。"), nil
	})
	s := open(t, filepath.Join(t.TempDir(), "x.db"), rt)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	if _, err := s.NewGame(ctx, "", "阿澈", 3); err != nil {
		t.Fatal(err)
	}
	s.ConfigureAI(provider.Config{Kind: provider.KindDeepSeek, APIKey: "sk-test"})
	if _, err := s.Quick(ctx, "", dto.QuickActionV1{Kind: "action", Action: "demo:action/look", Label: "环顾四周"}); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("requests = %d, want 1 (narration only)", n)
	}
}
