package orchestrator_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/transport"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func open(t *testing.T, path string, rt http.RoundTripper) *orchestrator.Session {
	t.Helper()
	s, err := orchestrator.Open(context.Background(), orchestrator.Options{DBPath: path, Package: packages.Demo(), Transport: rt})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func kinds(v dto.TurnV1) string {
	var ks []string
	for _, e := range v.Entries {
		ks = append(ks, e.Kind)
	}
	return strings.Join(ks, ",")
}

func narration(v dto.TurnV1) string {
	for _, e := range v.Entries {
		if e.Kind == "narration" {
			return e.Text
		}
	}
	return ""
}

// 离线模式端到端：输入 → 解析 → 判定 → 事件 → 叙事 → 保存 → 退出 → 恢复。
func TestOfflinePlaythroughAndResume(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ibuki.db")
	s := open(t, path, nil)
	var deltas atomic.Int32
	s.SetSink(func(e dto.StreamEventV1) {
		if e.Type == "narration_delta" {
			deltas.Add(1)
		}
	})
	slot, err := s.NewGame(ctx, "", "阿澈", 20260930)
	if err != nil {
		t.Fatal(err)
	}
	inputs := []string{"环顾四周", "和老板打个招呼", "来一杯麦酒", "喝掉麦酒", "劝说奥托冷静下来", "我在酒馆里唱一首北方的歌", "我要飞起来", "说服"}
	var last dto.TurnV1
	for i, in := range inputs {
		v, err := s.Submit(ctx, "", in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		t.Logf("[%d] %s → %s | %s", i, in, kinds(v), strings.ReplaceAll(narration(v), "\n", " "))
		for _, e := range v.Entries {
			if e.Kind == "system" && len(e.Options) > 0 {
				t.Logf("    options: %d (%s)", len(e.Options), e.Options[0].Label)
			}
		}
		last = v
	}
	if deltas.Load() == 0 {
		t.Fatal("expected streaming deltas")
	}
	// “我要飞起来”必须被拒绝；“说服”（无目标）必须请求澄清
	if last.Accepted || len(last.Entries) < 2 || len(last.Entries[1].Options) < 2 {
		t.Fatalf("clarification expected: %+v", last)
	}
	// 点选澄清选项（快速通道）
	opt := last.Entries[1].Options[0]
	v, err := s.Quick(ctx, "fixed-id-1", opt.Action)
	if err != nil || !v.Accepted {
		t.Fatalf("quick option: %v %+v", err, v)
	}
	// 重试同一 command_id：不重复执行
	st1, _ := s.State()
	v2, err := s.Quick(ctx, "fixed-id-1", opt.Action)
	if err != nil || !v2.Duplicate {
		t.Fatalf("duplicate should be detected: %v %+v", err, v2)
	}
	st2, _ := s.State()
	if st1.LastSeq != st2.LastSeq || st1.Player.Gold != st2.Player.Gold {
		t.Fatal("duplicate command must not change state")
	}
	// 移动 + 快捷建议
	sug, _ := s.Suggestions()
	if len(sug) < 4 {
		t.Fatalf("suggestions: %+v", sug)
	}
	if _, err := s.Submit(ctx, "", "出去看看"); err != nil {
		t.Fatal(err)
	}
	scene, _ := s.Scene(ctx)
	if scene.LocationID != "demo:location/town_square" {
		t.Fatalf("should be in the square: %s", scene.LocationID)
	}
	before, _ := s.State()
	tr, _ := s.Transcript(ctx, 0, 0)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// 恢复
	s = open(t, path, nil)
	defer func() { _ = s.Close() }()
	saves, _ := s.ListSaves(ctx)
	if len(saves) != 1 || saves[0].ID != slot || saves[0].Location != "小镇广场" {
		t.Fatalf("saves: %+v", saves)
	}
	if err := s.LoadGame(ctx, slot); err != nil {
		t.Fatal(err)
	}
	after, _ := s.State()
	a, _ := before.Marshal()
	b, _ := after.Marshal()
	if !bytes.Equal(a, b) {
		t.Fatal("resumed state differs")
	}
	tr2, _ := s.Transcript(ctx, 0, 0)
	if len(tr2) != len(tr) || tr2[0].Kind != "intro" {
		t.Fatalf("transcript after resume: %d vs %d", len(tr2), len(tr))
	}
	if _, err := s.Submit(ctx, "", "回酒馆"); err != nil {
		t.Fatal(err)
	}
	j, _ := s.Journal(ctx)
	if len(j) == 0 {
		t.Fatal("journal empty")
	}
	npcs, _ := s.NPCs()
	if len(npcs) < 3 {
		t.Fatalf("npcs: %d", len(npcs))
	}
	inv, _ := s.Inventory()
	if len(inv.Shop) == 0 {
		t.Fatal("shop should be visible in tavern")
	}
	ch, _ := s.Character()
	if len(ch.Skills) != 8 {
		t.Fatal("skills")
	}
}

// AI 失败（超时 / 500）时：解析降级为离线规则、叙事降级为模板；事件只提交一次，结果与离线一致。
func TestAIFailureFallsBackWithoutRerolling(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	failing := transport.Func(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("simulated timeout")
	})
	run := func(rt http.RoundTripper, online bool) *state.State {
		s := open(t, filepath.Join(t.TempDir(), "x.db"), rt)
		defer func() { _ = s.Close() }()
		if online {
			s.ConfigureAI(provider.Config{Kind: provider.KindDeepSeek, APIKey: "sk-test"})
		}
		if _, err := s.NewGame(ctx, "", "测试", 99); err != nil {
			t.Fatal(err)
		}
		for i, in := range []string{"环顾四周", "说服伯林", "威胁奥托", "调查"} {
			v, err := s.Submit(ctx, "c"+string(rune('a'+i)), in)
			if err != nil {
				t.Fatal(err)
			}
			if online {
				for _, e := range v.Entries {
					if e.Kind == "narration" && !e.Corrected {
						t.Fatalf("narration should be marked as fallback: %+v", e)
					}
				}
			}
		}
		st, _ := s.State()
		if online && s.AIStatus().LastError == "" {
			t.Fatal("last error should be recorded")
		}
		return st
	}
	offline := run(nil, false)
	degraded := run(failing, true)
	if calls.Load() == 0 {
		t.Fatal("AI should have been attempted")
	}
	a, _ := offline.Marshal()
	b, _ := degraded.Marshal()
	if !bytes.Equal(a, b) {
		t.Fatal("degraded AI run must produce exactly the offline game facts")
	}
}

// AI 叙事篡改结果时，Guard 拦截并回退到模板。
func TestGuardRejectsLyingNarrator(t *testing.T) {
	ctx := context.Background()
	fake := transport.Func(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		var out string
		if strings.Contains(string(body), "动作解析器") {
			out = `{"choices":[{"message":{"content":"{\"kind\":\"action\",\"matched_action\":\"demo:action/look\",\"targets\":[],\"reasonability\":\"ALLOW\",\"confidence\":0.9}"}}]}`
		} else {
			// 叙事凭空让玩家得到了火把，并泄露了米拉的秘密
			out = "data: {\"choices\":[{\"delta\":{\"content\":\"你环顾四周，顺手得到了火把。米拉其实是私生女。\"}}]}\n\ndata: [DONE]\n\n"
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(out)), Header: http.Header{}}, nil
	})
	s := open(t, filepath.Join(t.TempDir(), "g.db"), fake)
	defer func() { _ = s.Close() }()
	s.ConfigureAI(provider.Config{Kind: provider.KindQwen, APIKey: "sk-test"})
	if _, err := s.NewGame(ctx, "", "", 1); err != nil {
		t.Fatal(err)
	}
	var streamed strings.Builder
	var final dto.StreamEventV1
	s.SetSink(func(e dto.StreamEventV1) {
		switch e.Type {
		case "narration_delta":
			streamed.WriteString(e.Text)
		case "narration_done":
			final = e
		}
	})
	v, err := s.Submit(ctx, "", "四处看看")
	if err != nil {
		t.Fatal(err)
	}
	if v.Resolver != "ai" {
		t.Fatalf("resolver = %s", v.Resolver)
	}
	n := narration(v)
	if strings.Contains(n, "私生女") || strings.Contains(n, "得到了火把") || !final.Corrected || final.Source != "template(guard)" {
		t.Fatalf("guard should replace narration, got %q (%+v)", n, final)
	}
	if !strings.Contains(streamed.String(), "火把") {
		t.Fatal("draft should have been streamed before correction")
	}
}
