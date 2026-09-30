package memory_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/memory"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func setup(t *testing.T, turns int) (*loader.Package, *state.State, []eventstore.Entry) {
	t.Helper()
	ev, _ := expression.New()
	p, err := loader.Load(packages.Demo(), ev)
	if err != nil {
		t.Fatal(err)
	}
	s := state.New(p, 1, "测试者")
	s.Turn = turns
	var tr []eventstore.Entry
	for i := 1; i <= turns; i++ {
		tr = append(tr, eventstore.Entry{Turn: i, Kind: "player", Text: fmt.Sprintf("第%d件事", i)},
			eventstore.Entry{Turn: i, Kind: "narration", Text: fmt.Sprintf("你做了第%d件事。之后的描写很长很长。", i)})
	}
	return p, s, tr
}

type failing struct{}

func (failing) Summarize(context.Context, string, []string, int) (string, error) {
	return "", errors.New("offline")
}

func TestPlanDeterministicWithCompressionNotes(t *testing.T) {
	p, s, tr := setup(t, 16)
	a := &memory.Agent{}
	r1, add1 := a.Plan(context.Background(), p, s, tr, nil)
	r2, add2 := a.Plan(context.Background(), p, s, tr, nil)
	if !reflect.DeepEqual(add1, add2) || !reflect.DeepEqual(r1, r2) {
		t.Fatal("plan is not deterministic")
	}
	// 16 回合，保留最近 4 个：1-12 回合压缩为两条滚动摘要（每条 6 回合）
	if len(add1) != 2 || add1[0].FromTurn != 1 || add1[0].ToTurn != 6 || add1[1].ToTurn != 12 {
		t.Fatalf("rolling: %+v", add1)
	}
	if !strings.Contains(add1[0].Text, "第1件事") || strings.Contains(add1[0].Text, "描写很长") {
		t.Fatalf("summary should keep the gist, drop description: %s", add1[0].Text)
	}
	if len(add1[0].Compressed) == 0 {
		t.Fatal("summary must record what was compressed")
	}
	// LLM 摘要器失败时回退离线结果
	b := &memory.Agent{Summarizer: failing{}}
	_, add3 := b.Plan(context.Background(), p, s, tr, nil)
	if !reflect.DeepEqual(add1, add3) {
		t.Fatal("failed summarizer should fall back to offline summaries")
	}
	sums := memory.ToSummaries(add1, s.Turn)
	sec := memory.StorySoFar(sums, 2000)
	if !strings.HasPrefix(sec, "[STORY_SO_FAR]") || !strings.Contains(sec, "逐字对话") || !strings.Contains(sec, "先检索") {
		t.Fatalf("story so far:\n%s", sec)
	}
	if memory.StorySoFar(nil, 100) != "" {
		t.Fatal("no summaries → no section")
	}
}

func TestLongTermAndNPC(t *testing.T) {
	p, s, tr := setup(t, 40)
	borin := "demo:character/borin"
	for i := 0; i < 10; i++ {
		s.NPCs[borin].Memories = append(s.NPCs[borin].Memories, state.Memory{Text: fmt.Sprintf("看到你做第%d件事", i), Turn: i + 1, Notable: i%3 == 0})
	}
	a := &memory.Agent{}
	remove, add := a.Plan(context.Background(), p, s, tr, nil)
	kinds := map[string]int{}
	for _, m := range add {
		kinds[m.Kind]++
	}
	if len(remove) != 0 || kinds["longterm"] != 1 || kinds["rolling"] != 2 || kinds["npc"] != 1 {
		t.Fatalf("kinds=%v remove=%v", kinds, remove)
	}
	for _, m := range add {
		if m.Kind == "npc" && (m.Subject != borin || !strings.Contains(m.Text, "伯林")) {
			t.Fatalf("npc summary: %+v", m)
		}
	}
}
