package orchestrator_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/transport"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func openPacks(t *testing.T, rt http.RoundTripper) *orchestrator.Session {
	t.Helper()
	s, err := orchestrator.Open(context.Background(), orchestrator.Options{DBPath: filepath.Join(t.TempDir(), "rpg.db"), Packs: packages.Builtin(), Transport: rt})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func chat(content string) *http.Response {
	b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}}
}

// deviate 在离线模式下反复动粗，直到弹出“严重偏离”。
func deviate(t *testing.T, s *orchestrator.Session) dto.TurnV1 {
	t.Helper()
	ctx := context.Background()
	var v dto.TurnV1
	for i := 0; i < 20; i++ {
		var err error
		v, err = s.Submit(ctx, "", "动手砸烂工作台")
		if err != nil {
			t.Fatal(err)
		}
		if v.Scene.Mainline != nil && v.Scene.Mainline.Pending {
			return v
		}
	}
	t.Fatalf("no heavy-deviation prompt: %+v", v.Scene.Mainline)
	return v
}

// TestFreeModeRecorded：严重偏离 → 选择自由推演 → AI 提案（录制传输）→ Core 校验 → 成为正史节点；
// 用同一录音目录回放必须得到同样的节点。非法提案回退为模板节点。
func TestFreeModeRecorded(t *testing.T) {
	dir := t.TempDir()
	var calls int
	upstream := transport.Func(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "剧情导演") {
			t.Errorf("unexpected AI request: %.120s", body)
		}
		calls++
		return chat(`{"title":"托克的请求","objective":"去竞技场找托克，问问他为什么总往下水道跑。","goal":"talk","ref":"brass:character/tock","reward_xp":40,"summary":"托克似乎知道钟楼停摆的内情。","new_character":{"name":"灰帽子","role":"神秘的旅人","description":"在广场角落观察你的陌生人。"}}`), nil
	})
	run := func(rt http.RoundTripper) dto.TurnV1 {
		s := openPacks(t, rt)
		ctx := context.Background()
		if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 5); err != nil {
			t.Fatal(err)
		}
		deviate(t, s)
		s.ConfigureAI(provider.Config{Kind: provider.KindDeepSeek, APIKey: "sk-test"})
		sc, _ := s.Scene(ctx)
		if !sc.Mainline.FreeOnline {
			t.Fatal("online session should offer AI free mode")
		}
		v, err := s.Quick(ctx, "", dto.QuickActionV1{Kind: "mainline", Action: "free", Label: "进入自由推演"})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	v := run(&transport.Recorded{Dir: dir, Upstream: upstream})
	if calls != 1 {
		t.Fatalf("expected exactly one canon call, got %d", calls)
	}
	m := v.Scene.Mainline
	if m.Mode != "free" || m.Node == nil || m.Node.Title != "托克的请求" || m.Node.Source != "ai" {
		t.Fatalf("AI node not canonized: %+v", m)
	}
	var sawNode, sawCard bool
	for _, n := range v.Notices {
		sawNode = sawNode || n.Kind == "node"
		sawCard = sawCard || (n.Kind == "card" && strings.Contains(n.Text, "灰帽子"))
	}
	if !sawNode || !sawCard {
		t.Fatalf("notices: %+v", v.Notices)
	}
	// 回放：没有上游，只能命中录音
	v2 := run(&transport.Recorded{Dir: dir})
	if v2.Scene.Mainline.Node == nil || v2.Scene.Mainline.Node.Title != m.Node.Title {
		t.Fatalf("replay differs: %+v", v2.Scene.Mainline)
	}
	// 非法提案（首领战）→ 回退为模板节点
	bad := transport.Func(func(r *http.Request) (*http.Response, error) {
		return chat(`{"title":"屠龙","objective":"立刻击败蒸汽傀儡。","goal":"defeat","enemies":["brass:enemy/steam_golem"]}`), nil
	})
	v3 := run(bad)
	if n := v3.Scene.Mainline.Node; n == nil || n.Source != "template" {
		t.Fatalf("invalid proposal should fall back to a template node: %+v", n)
	}
}

// TestOfflineSandboxAndCombatText：离线选择“自由推演”进入沙盒模式；战斗中的自然语言输入被解析为战斗动作。
func TestOfflineSandboxAndCombatText(t *testing.T) {
	ctx := context.Background()
	s := openPacks(t, nil)
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	for _, dest := range []string{"brass:location/plaza", "brass:location/arena"} {
		if _, err := s.Quick(ctx, "", dto.QuickActionV1{Kind: "move", Destination: dest}); err != nil {
			t.Fatal(err)
		}
	}
	v, err := s.Quick(ctx, "", dto.QuickActionV1{Kind: "combat", Action: "start", Target: "brass:encounter/trial_bout1"})
	if err != nil || v.Scene.Combat == nil {
		t.Fatalf("combat not started: %v %+v", err, v.Scene.Combat)
	}
	v, err = s.Submit(ctx, "", "用扳手重击砸发条鼠甲")
	if err != nil {
		t.Fatal(err)
	}
	var combatEntry bool
	for _, e := range v.Entries {
		if e.Kind == "combat" && e.Combat != nil && e.Combat.SkillID == "brass:skill/wrench_smash" {
			combatEntry = len(e.Combat.Chips) > 0
		}
	}
	if !v.Accepted || !combatEntry {
		t.Fatalf("natural-language skill not resolved: %+v", v.Entries)
	}
	v, _ = s.Submit(ctx, "", "唱一首歌")
	if v.Accepted {
		t.Fatal("non-combat input during combat should be rejected with options")
	}
	var opts int
	for _, e := range v.Entries {
		opts += len(e.Options)
	}
	if opts == 0 {
		t.Fatal("rejection should offer combat options")
	}
	for i := 0; i < 30 && v.Scene.Combat != nil; i++ {
		v, err = s.Quick(ctx, "", dto.QuickActionV1{Kind: "combat", Action: "attack"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if v.Scene.Combat != nil {
		t.Fatal("fight did not finish")
	}
	deviate(t, s)
	v, err = s.Quick(ctx, "", dto.QuickActionV1{Kind: "mainline", Action: "free"})
	if err != nil {
		t.Fatal(err)
	}
	if m := v.Scene.Mainline; m.Mode != "sandbox" || m.Node == nil {
		t.Fatalf("offline free mode should become sandbox with a node: %+v", m)
	}
	// 新增查询接口
	if cx, err := s.Codex(); err != nil || cx.Total == 0 {
		t.Fatalf("codex: %v %+v", err, cx)
	}
	if ms, err := s.Mechs(); err != nil || len(ms.Mechs) != 2 {
		t.Fatalf("mechs: %v %d", err, len(ms.Mechs))
	}
	if rel, err := s.Relations(); err != nil || len(rel.Edges) == 0 {
		t.Fatalf("relations: %v", err)
	}
	if _, ok, _ := s.Portrait(ctx, "brass:character/orin"); ok {
		t.Fatal("sample pack has no portraits")
	}
}
