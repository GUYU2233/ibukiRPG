package orchestrator_test

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/transport"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/packages"
)

const borin = "demo:character/borin"

func talk(t *testing.T, s *orchestrator.Session) string {
	t.Helper()
	v, err := s.Quick(context.Background(), "", dto.QuickActionV1{Kind: "action", Action: "demo:action/talk", Target: borin})
	if err != nil || !v.Accepted {
		t.Fatalf("talk: %v %+v", err, v)
	}
	return narration(v)
}

// 玩家反馈的问题：和 NPC 聊完、做点别的、再聊，NPC 重复同样的话。
// 修复后：第二次交谈必须不同，并且引用刚才发生的事；记忆随存档保存，读档后继续生效。
func TestNPCRemembersAcrossTurnsAndSaveLoad(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ibuki.db")
	s := open(t, path, nil)
	slot, err := s.NewGame(ctx, "", "阿澈", 3)
	if err != nil {
		t.Fatal(err)
	}
	first := talk(t, s)
	if _, err := s.Submit(ctx, "", "点一杯麦酒"); err != nil {
		t.Fatal(err)
	}
	second := talk(t, s)
	t.Logf("first:  %s\nsecond: %s", first, second)
	if first == second {
		t.Fatal("NPC repeated the same line")
	}
	if !strings.Contains(second, "刚才") {
		t.Fatalf("second talk should reference what just happened: %q", second)
	}
	st, _ := s.State()
	n := st.NPCs[borin]
	if n.Talks != 2 || len(n.Said) != 2 || len(n.Exchanges) != 2 {
		t.Fatalf("memory not recorded: talks=%d said=%v exchanges=%d", n.Talks, n.Said, len(n.Exchanges))
	}
	npcs, _ := s.NPCs()
	for _, v := range npcs {
		if v.ID == borin && (v.Talks != 2 || len(v.Memories) == 0) {
			t.Fatalf("people panel should show memories: %+v", v)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s = open(t, path, nil)
	defer func() { _ = s.Close() }()
	if err := s.LoadGame(ctx, slot); err != nil {
		t.Fatal(err)
	}
	st, _ = s.State()
	if st.NPCs[borin].Talks != 2 {
		t.Fatalf("memory lost after reload: %+v", st.NPCs[borin])
	}
	seen := map[string]bool{first: true, second: true}
	for i := range 3 {
		line := talk(t, s)
		t.Logf("after reload %d: %s", i, line)
		if seen[line] {
			t.Fatalf("NPC repeated a line after reload: %q", line)
		}
		seen[line] = true
	}
}

// 云端 AI / 本地 MediaPipe 共用同一条 Go 提示词路径：叙事提示词必须带上该 NPC 的记忆（NPCScope）。
func TestAINarratorPromptIncludesNPCMemory(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var prompts []string
	fake := transport.Func(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		var out string
		if strings.Contains(string(body), "动作解析器") {
			out = `{"choices":[{"message":{"content":"{\"kind\":\"action\",\"matched_action\":\"demo:action/talk\",\"targets\":[\"demo:character/borin\"],\"reasonability\":\"ALLOW\",\"confidence\":0.9}"}}]}`
		} else {
			mu.Lock()
			prompts = append(prompts, string(body))
			mu.Unlock()
			out = "data: {\"choices\":[{\"delta\":{\"content\":\"伯林点点头。\"}}]}\n\ndata: [DONE]\n\n"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(out)), Header: http.Header{}}, nil
	})
	s := open(t, filepath.Join(t.TempDir(), "x.db"), fake)
	defer func() { _ = s.Close() }()
	s.ConfigureAI(provider.Config{Kind: provider.KindDeepSeek, APIKey: "sk-test"})
	if _, err := s.NewGame(ctx, "", "阿澈", 3); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if _, err := s.Submit(ctx, "", "和伯林聊聊"); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(prompts) < 2 {
		t.Fatalf("narrator prompts: %d", len(prompts))
	}
	last := prompts[len(prompts)-1]
	for _, want := range []string{"NPC_MEMORY", "已和你交谈 1 次", "最近的交谈", "本回合要表达的意思"} {
		if !strings.Contains(last, want) {
			t.Fatalf("second narrator prompt lacks %q:\n%s", want, last)
		}
	}
	// NPCScope：不在场、没交谈过的人不应出现在记忆段落里
	if strings.Contains(last, "奥托（对你") {
		t.Fatal("memory of an unrelated NPC leaked into the prompt")
	}
}

func hudLabels(sc dto.SceneV1) map[string]string {
	m := map[string]string{}
	for _, h := range sc.Hud {
		m[h.Label] = h.Value
	}
	return m
}

func TestPackHUD(t *testing.T) {
	ctx := context.Background()
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: filepath.Join(t.TempDir(), "x.db"), Packs: packages.Builtin(), PackDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.NewGame(ctx, "", "阿澈", 3); err != nil {
		t.Fatal(err)
	}
	sc, _ := s.Scene(ctx)
	h := hudLabels(sc)
	for _, k := range []string{"位置", "时间", "铜币", "主线"} {
		if h[k] == "" {
			t.Fatalf("tavern HUD lacks %s: %+v", k, sc.Hud)
		}
	}
	if h["铜币"] != "15" || h["位置"] != "锈酒杯酒馆" {
		t.Fatalf("HUD values: %+v", h)
	}
	if _, ok := h["米拉的嫌疑"]; ok {
		t.Fatal("suspicion must stay hidden before the story starts")
	}
	if _, err := s.Submit(ctx, "", "点一杯麦酒"); err != nil {
		t.Fatal(err)
	}
	sc, _ = s.Scene(ctx)
	if hudLabels(sc)["铜币"] != "13" {
		t.Fatalf("HUD must update each turn: %+v", sc.Hud)
	}
	// 推进到《失窃的钱袋》开始：嫌疑出现在 HUD 上
	for i := 0; i < 6 && sc.Story == nil; i++ {
		_, _ = s.Submit(ctx, "", "环顾四周")
		sc, _ = s.Scene(ctx)
	}
	if sc.Story == nil {
		t.Fatal("story should have started")
	}
	if v := hudLabels(sc)["米拉的嫌疑"]; v != "70%" {
		t.Fatalf("suspicion = %q (%+v)", v, sc.Hud)
	}
	// 没有声明 HUD 的故事包使用默认状态栏
	if _, err := s.NewGameIn(ctx, "fog_lighthouse", "", "小雾", 1); err != nil {
		t.Fatal(err)
	}
	sc, _ = s.Scene(ctx)
	h = hudLabels(sc)
	if sc.PackID != "fog_lighthouse" || h["位置"] != "雾港码头" || h["铜币"] != "5" {
		t.Fatalf("default HUD: %+v %+v", sc.PackID, sc.Hud)
	}
}

// 存档绑定故事包 id + 版本（第 44 节）：导入包被删除后，存档列表标出问题，读档给出中文原因。
func TestSaveBoundToPack(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: filepath.Join(dir, "x.db"), Packs: packages.Builtin(), PackDir: filepath.Join(dir, "packs")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	res, err := s.ImportPack(ctx, zipLighthouseAs(t, "my_pack", "mine"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Pack.Builtin || !res.Pack.Playable {
		t.Fatalf("imported pack: %+v", res.Pack)
	}
	packs := s.Packs(ctx)
	if len(packs) != len(packages.BuiltinDirs)+1 {
		t.Fatalf("packs: %d", len(packs))
	}
	slot, err := s.NewGameIn(ctx, "my_pack", "", "小雾", 1)
	if err != nil {
		t.Fatal(err)
	}
	demoSlot, _ := s.NewGame(ctx, "", "阿澈", 1)
	saves, _ := s.ListSaves(ctx)
	byID := map[string]dto.SlotV1{}
	for _, sv := range saves {
		byID[sv.ID] = sv
	}
	if sv := byID[slot]; sv.PackID != "my_pack" || sv.PackVersion != "0.1.0" || sv.PackName != "我的故事" {
		t.Fatalf("slot pack binding: %+v", sv)
	}
	if byID[demoSlot].PackID != "demo" {
		t.Fatalf("demo slot: %+v", byID[demoSlot])
	}
	if err := s.DeletePack(ctx, "demo"); err == nil {
		t.Fatal("builtin pack must not be deletable")
	}
	if err := s.DeletePack(ctx, "my_pack"); err != nil {
		t.Fatal(err)
	}
	saves, _ = s.ListSaves(ctx)
	for _, sv := range saves {
		if sv.ID == slot && sv.PackProblem == "" {
			t.Fatalf("missing pack should be reported: %+v", sv)
		}
	}
	if err := s.LoadGame(ctx, slot); err == nil || !strings.Contains(err.Error(), "故事包") {
		t.Fatalf("loading a save whose pack is gone: %v", err)
	}
	if err := s.LoadGame(ctx, demoSlot); err != nil {
		t.Fatal(err)
	}
}

func zipLighthouseAs(t *testing.T, id, ns string) string {
	t.Helper()
	src := packages.Builtin()[1]
	p := filepath.Join(t.TempDir(), "pack.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	err = fs.WalkDir(src, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(src, name)
		if err != nil {
			return err
		}
		txt := string(b)
		if name == "manifest.yaml" {
			txt = strings.Replace(txt, "id: fog_lighthouse", "id: "+id, 1)
			txt = strings.Replace(txt, "namespace: fog", "namespace: "+ns, 1)
			txt = strings.Replace(txt, `name: "雾港灯塔"`, `name: "我的故事"`, 1)
		}
		txt = strings.ReplaceAll(txt, "fog:", ns+":")
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write([]byte(txt))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = zw.Close()
	_ = f.Close()
	return p
}
