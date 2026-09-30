package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/transport"
	"github.com/GUYU2233/ibukiRPG/internal/api/query"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func demoEnv(t *testing.T) *tools.Env {
	t.Helper()
	ev, _ := expression.New()
	p, err := loader.Load(packages.Demo(), ev)
	if err != nil {
		t.Fatal(err)
	}
	s := state.New(p, 7, "测试者")
	return &tools.Env{Pkg: p, State: s, Q: &query.Q{Pkg: p, Eval: ev}}
}

func call(t *testing.T, e *tools.Env, sc tools.Scope, name, args string) (string, error) {
	t.Helper()
	out, err := tools.Call(e, sc, name, args)
	if !json.Valid([]byte(out)) {
		t.Fatalf("%s: result is not JSON: %s", name, out)
	}
	if len(out) > tools.MaxResultBytes {
		t.Fatalf("%s: result too large (%d bytes)", name, len(out))
	}
	return out, err
}

// NPC 读不到别人的秘密；自己的秘密自己知道；director 全可见；未解锁时叙述者（玩家范围）也看不到。
func TestScopeFiltering(t *testing.T) {
	e := demoEnv(t)
	const secret = "家族的私生女"
	cases := []struct {
		sc   tools.Scope
		want bool
	}{
		{tools.NPC("demo:character/lena"), false},
		{tools.NPC("demo:character/borin"), false},
		{tools.Player(), false},
		{tools.NPC("demo:character/mira"), true},
		{tools.Director(), true},
	}
	for _, c := range cases {
		var all strings.Builder
		for _, q := range []struct{ name, args string }{
			{"pack.search", `{"query":"私生女 没落贵族"}`},
			{"pack.search", `{"query":"米拉"}`},
			{"pack.get_entity", `{"id":"demo:character/mira"}`},
			{"pack.get_entity", `{"id":"米拉"}`},
			{"character.get_card", `{"id":"demo:character/mira"}`},
			{"pack.list", `{"type":"character"}`},
		} {
			out, _ := call(t, e, c.sc, q.name, q.args)
			all.WriteString(out)
		}
		if got := strings.Contains(all.String(), secret); got != c.want {
			t.Errorf("scope %s: secret visible=%v want %v\n%s", c.sc, got, c.want, all.String())
		}
	}
	// NPC 不能读别人的记忆，也看不到剧本锚点 / 故事节点
	if _, err := call(t, e, tools.NPC("demo:character/lena"), "memory.search", `{"query":"","who":"demo:character/mira"}`); !errors.Is(err, tools.ErrNotVisible) {
		t.Errorf("npc reading another npc's memory: err=%v", err)
	}
	out, _ := call(t, e, tools.NPC("demo:character/lena"), "pack.list", `{"type":"story"}`)
	if strings.Contains(out, `"id"`) {
		t.Errorf("npc sees story nodes: %s", out)
	}
	// 不存在与无权查看返回同样的错误（不泄露“有这个东西”）
	_, e1 := call(t, e, tools.NPC("demo:character/lena"), "pack.get_entity", `{"id":"demo:secret/mira_noble"}`)
	_, e2 := call(t, e, tools.NPC("demo:character/lena"), "pack.get_entity", `{"id":"demo:nothing/here"}`)
	if e1 == nil || e2 == nil || e1.Error() != e2.Error() {
		t.Errorf("missing vs secret distinguishable: %v / %v", e1, e2)
	}
}

// 工具是只读、确定性的：同样的调用两次结果相同，且不改变状态。
func TestDeterministicReadOnly(t *testing.T) {
	e := demoEnv(t)
	before, _ := json.Marshal(e.State)
	for _, tl := range tools.All() {
		args := `{"query":"酒馆","id":"demo:character/borin","type":"character","a":"demo:character/borin","b":"player"}`
		for _, sc := range []tools.Scope{tools.Player(), tools.Director(), tools.NPC("demo:character/borin")} {
			if !tl.Allowed(sc) {
				continue
			}
			a, _ := call(t, e, sc, tl.Name, args)
			b, _ := call(t, e, sc, tl.FuncName(), args)
			if a != b {
				t.Errorf("%s not deterministic", tl.Name)
			}
		}
	}
	after, _ := json.Marshal(e.State)
	if string(before) != string(after) {
		t.Error("tools mutated state")
	}
	if len(tools.Specs(tools.Director())) < 11 {
		t.Errorf("director specs: %d", len(tools.Specs(tools.Director())))
	}
}

func chatResp(msg map[string]any) *http.Response {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": msg}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}

func toolCallMsg(id, name, args string) map[string]any {
	return map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": id, "type": "function", "function": map[string]any{"name": name, "arguments": args}}}}
}

var onlineCfg = provider.Config{Kind: provider.KindDeepSeek, APIKey: "sk-test"}

// 录制的工具调用对话：模型先查 pack_search，再根据结果回答；同一录音目录回放得到同样的结果。
func TestRecordedToolConversation(t *testing.T) {
	e := demoEnv(t)
	dir := t.TempDir()
	n := 0
	upstream := transport.Func(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		n++
		var req struct {
			Tools    []any `json:"tools"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		if len(req.Tools) == 0 {
			t.Errorf("request %d carries no tools", n)
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role != "tool" {
			return chatResp(toolCallMsg("call_1", "pack_search", `{"query":"莉娜"}`)), nil
		}
		if !strings.Contains(last.Content, "莉娜") {
			t.Errorf("tool result lacks the entity: %s", last.Content)
		}
		return chatResp(map[string]any{"role": "assistant", "content": "莉娜是退役佣兵，酒馆常客。"}), nil
	})
	run := func(rt http.RoundTripper) tools.Outcome {
		l := &tools.Loop{Provider: provider.NewOpenAICompatible(onlineCfg, rt), Env: e, Scope: tools.Player()}
		return l.Run(context.Background(), []provider.Message{{Role: "system", Content: "你是叙述者"}, {Role: "user", Content: "莉娜是谁？"}}, "莉娜")
	}
	o := run(&transport.Recorded{Dir: dir, Upstream: upstream})
	if o.Mode != "tools" || o.Answer == nil || !strings.Contains(o.Answer.Text, "退役佣兵") || len(o.Steps) != 1 || o.Steps[0].Tool != "pack_search" {
		t.Fatalf("outcome: %+v", o)
	}
	if n != 2 {
		t.Fatalf("upstream calls = %d", n)
	}
	o2 := run(&transport.Recorded{Dir: dir}) // 纯回放
	if o2.Answer == nil || o2.Answer.Text != o.Answer.Text {
		t.Fatalf("replay differs: %+v", o2)
	}
}

// 迭代 / 调用次数上限：模型一直要求查资料，循环必须停下并标记 Capped。
func TestLoopCaps(t *testing.T) {
	e := demoEnv(t)
	n := 0
	rt := transport.Func(func(r *http.Request) (*http.Response, error) {
		n++
		return chatResp(map[string]any{"role": "assistant", "content": "", "tool_calls": []any{
			map[string]any{"id": "a", "type": "function", "function": map[string]any{"name": "pack_search", "arguments": `{"query":"酒馆"}`}},
			map[string]any{"id": "b", "type": "function", "function": map[string]any{"name": "pack_list", "arguments": `{"type":"item"}`}},
			map[string]any{"id": "c", "type": "function", "function": map[string]any{"name": "no_such_tool", "arguments": `{}`}},
		}}), nil
	})
	l := &tools.Loop{Provider: provider.NewOpenAICompatible(onlineCfg, rt), Env: e, Scope: tools.Player(), Budget: tools.Budget{MaxIters: 3, MaxCalls: 4, MaxResultRunes: 3000}}
	o := l.Run(context.Background(), []provider.Message{{Role: "user", Content: "说说酒馆"}}, "酒馆")
	if !o.Capped || o.Answer != nil || n != 3 {
		t.Fatalf("capped=%v answer=%v calls=%d", o.Capped, o.Answer, n)
	}
	executed := 0
	for _, s := range o.Steps {
		if !strings.Contains(s.Result, "已用完") {
			executed++
		}
	}
	if executed > 4 {
		t.Fatalf("executed %d tool calls, cap 4", executed)
	}
	total := 0
	for _, m := range o.Messages {
		if m.Role == "tool" {
			total += len([]rune(m.Content))
		}
	}
	if total > 3000+200 {
		t.Fatalf("tool results %d runes exceed budget", total)
	}
}

// 降级：本地模型（不支持函数调用）与服务端拒绝 tools 时，都改为关键词预检索并注入 [RETRIEVED]。
func TestFallbackPrefetch(t *testing.T) {
	e := demoEnv(t)
	msgs := []provider.Message{{Role: "system", Content: "你是叙述者"}, {Role: "user", Content: "我想找伯林聊聊"}}
	check := func(name string, o tools.Outcome) {
		t.Helper()
		last := o.Messages[len(o.Messages)-1].Content
		if o.Mode != "prefetch" || !strings.Contains(last, "[RETRIEVED]") || !strings.Contains(last, "伯林") {
			t.Errorf("%s: mode=%s last=%s", name, o.Mode, last)
		}
		if strings.Contains(msgs[1].Content, "[RETRIEVED]") {
			t.Errorf("%s: input messages mutated", name)
		}
	}
	never := transport.Func(func(*http.Request) (*http.Response, error) {
		t.Error("local model must not receive a tools request")
		return nil, errors.New("unreachable")
	})
	local := &tools.Loop{Provider: provider.NewOpenAICompatible(provider.Config{Kind: provider.KindMediaPipe, BaseURL: "http://127.0.0.1:1", Model: "gemma"}, never), Env: e, Scope: tools.Player()}
	check("mediapipe", local.Run(context.Background(), msgs, "伯林"))

	reject := transport.Func(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"tools is not supported for this model"}}`)), Header: http.Header{}}, nil
	})
	remote := &tools.Loop{Provider: provider.NewOpenAICompatible(onlineCfg, reject), Env: e, Scope: tools.Player()}
	check("400", remote.Run(context.Background(), msgs, "伯林"))

	// NPC 范围的预检索同样不泄露秘密
	sec := tools.PrefetchSection(e, tools.NPC("demo:character/lena"), "米拉 私生女 没落贵族", 3000)
	if strings.Contains(sec, "家族的私生女") {
		t.Errorf("prefetch leaked secret: %s", sec)
	}
}

func TestUnknownRefs(t *testing.T) {
	e := demoEnv(t)
	got := tools.UnknownRefs(e, tools.Director(), "我去问问莉娜关于伯林的事", []string{"伯林：酒馆老板"})
	if len(got) != 1 || got[0] != "莉娜" {
		t.Fatalf("unknown refs: %v", got)
	}
}

func TestMemorySearchSummaries(t *testing.T) {
	e := demoEnv(t)
	e.Summaries = []tools.Summary{
		{Kind: "rolling", FromTurn: 1, ToTurn: 8, Text: "玩家在酒馆替伯林修好了地窖的门。", Compressed: []string{"逐字对话"}},
		{Kind: "npc", Subject: "demo:character/mira", FromTurn: 1, ToTurn: 8, Text: "米拉记得玩家听过她的歌。"},
	}
	out, _ := call(t, e, tools.Player(), "memory.search", `{"query":"地窖"}`)
	if !strings.Contains(out, "地窖") || !strings.Contains(out, "已压缩省略") {
		t.Errorf("player memory.search: %s", out)
	}
	out, _ = call(t, e, tools.Player(), "memory.search", `{"query":"米拉 歌"}`)
	if strings.Contains(out, "听过她的歌") {
		t.Errorf("narrator reads npc-private summary: %s", out)
	}
	out, _ = call(t, e, tools.NPC("demo:character/mira"), "memory.search", `{"query":"歌"}`)
	if !strings.Contains(out, "听过她的歌") {
		t.Errorf("mira can't read her own summary: %s", out)
	}
	out, _ = call(t, e, tools.NPC("demo:character/lena"), "memory.search", `{"query":"歌"}`)
	if strings.Contains(out, "听过她的歌") {
		t.Errorf("lena reads mira's summary: %s", out)
	}
}
