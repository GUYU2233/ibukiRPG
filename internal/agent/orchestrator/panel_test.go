package orchestrator_test

import (
	"context"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
)

// TestWorldPanelTabs：8 个页签都能取到；一切从未知开始（未见过的角色不出现，已知角色未解锁字段为 unknown）。
func TestWorldPanelTabs(t *testing.T) {
	ctx := context.Background()
	s := onlineBrass(t, &fakeLLM{})
	for _, tab := range dto.WorldTabs {
		p, err := s.WorldPanel(tab)
		if err != nil || p.Tab != tab {
			t.Fatalf("tab %s: %v", tab, err)
		}
	}
	if _, err := s.WorldPanel("nope"); err == nil {
		t.Fatal("unknown tab must fail")
	}
	p, _ := s.WorldPanel("characters")
	if len(p.Entities) == 0 || p.Entities[0].ID != "player" {
		t.Fatalf("player first: %+v", p.Entities)
	}
	for _, e := range p.Entities[1:] {
		if e.ID == "rivet" {
			for _, f := range e.Fields {
				if f.Key == "background" && f.Level != "unknown" {
					t.Fatalf("rivet background should start unknown: %+v", f)
				}
			}
		}
	}
	if v, ok, _ := s.Entity("steam_golem"); ok && v.Name == "蒸汽魔像" {
		t.Fatalf("unseen enemy must not be named: %+v", v)
	}
	tl, _ := s.WorldPanel("timeline")
	if len(tl.Events) == 0 {
		t.Fatal("public events (trial_day) should be listed")
	}
	_ = ctx
	_ = engine.Creation{}
}

// TestCreationReview：规则层拦住超额属性 / 禁用词；推荐卡合法；预设列表包含故事包模板。
func TestCreationReview(t *testing.T) {
	ctx := context.Background()
	s := onlineBrass(t, &fakeLLM{})
	opts, err := s.CreationOptions("brass_trial")
	if err != nil || len(opts.Presets) == 0 || len(opts.Backgrounds) != 3 {
		t.Fatalf("options %+v %v", opts, err)
	}
	bad := engine.Creation{Custom: true, Name: "测试", Background: "nope", Attributes: map[string]int{}}
	for k := range opts.AttributeNames {
		bad.Attributes[k] = 99
	}
	rv, err := s.ReviewCreation(ctx, "brass_trial", bad)
	if err != nil || rv.OK || len(rv.Problems) == 0 {
		t.Fatalf("over-budget creation must be flagged: %+v %v", rv, err)
	}
	if len(rv.Recommended) == 0 {
		t.Fatal("recommended card missing")
	}
	good := engine.Creation{Custom: true, Name: "小锤", Background: opts.Backgrounds[0].ID}
	rv, _ = s.ReviewCreation(ctx, "brass_trial", good)
	if !rv.OK {
		t.Fatalf("valid creation flagged: %+v", rv)
	}
}
