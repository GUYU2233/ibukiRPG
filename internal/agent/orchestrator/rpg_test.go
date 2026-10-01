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

// TestOfflineCombatText：离线选择“自由推演”进入沙盒模式；战斗中的自然语言输入被解析为战斗动作。
func TestOfflineCombatText(t *testing.T) {
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
