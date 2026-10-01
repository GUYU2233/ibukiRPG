package router

import (
	"context"
	"errors"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
)

type fake struct {
	name string
	err  error
}

func (f *fake) Name() string { return f.name }
func (f *fake) Generate(context.Context, provider.Request) (provider.Response, error) {
	if f.err != nil {
		return provider.Response{}, f.err
	}
	return provider.Response{Text: f.name, Model: f.name, PromptTokens: 100, CompletionTokens: 20}, nil
}
func (f *fake) Stream(ctx context.Context, r provider.Request, on func(string)) (provider.Response, error) {
	return f.Generate(ctx, r)
}

func newRouter(fail map[string]error) *Router {
	r := New(func(cfg provider.Config) provider.Provider {
		return &fake{name: cfg.Kind + ":" + cfg.Model, err: fail[cfg.Model]}
	})
	r.SetProviders([]Provider{
		{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "k1"},
		{ID: "qw", Kind: provider.KindQwen, APIKey: "k2"},
		{ID: "loc", Kind: provider.KindLlamaCpp, BaseURL: "http://127.0.0.1:8080", Model: "qwen2.5-3b", SizeB: 3},
		{ID: "nokey", Kind: provider.KindCustom, BaseURL: "https://x", Model: "m"},
	})
	return r
}

func TestPerTaskRouting(t *testing.T) {
	r := newRouter(nil)
	r.SetSettings(Settings{Mode: ModePerTask, Unified: Route{Provider: "qw"}, Tasks: map[string]Route{
		TaskNarrate: {Provider: "ds", Fallback: []Route{{Provider: "qw"}}},
		TaskMemory:  {Provider: "loc"},
		TaskAudit:   {Provider: "nokey"}, // 没有密钥：不可用，回退到统一模型
	}})
	if tg, _ := r.Primary(TaskNarrate); tg.Entry.ID != "ds" || tg.Model != "deepseek-chat" {
		t.Fatalf("narrate → %+v", tg.Entry)
	}
	if tg, _ := r.Primary(TaskMemory); tg.Entry.ID != "loc" || tg.Tier != TierLimited {
		t.Fatalf("memory → %+v", tg)
	}
	if tg, _ := r.Primary(TaskAudit); tg.Entry.ID != "qw" {
		t.Fatalf("audit should fall back to unified, got %s", tg.Entry.ID)
	}
	// 未配置的任务回退到 narrate_world 的模型
	if tg, _ := r.Primary(TaskCardGen); tg.Entry.ID != "ds" {
		t.Fatalf("card_gen → %s", tg.Entry.ID)
	}
	if len(r.Chain(TaskNarrate)) != 2 {
		t.Fatalf("narrate chain %d", len(r.Chain(TaskNarrate)))
	}
	r.SetSettings(Settings{Mode: ModePerTask, Tasks: map[string]Route{TaskNarrate: {Provider: "loc"}}})
	if w := r.LocalWarnings(); len(w) == 0 {
		t.Fatal("narrate on local model should warn")
	}
}

func TestUnifiedAndOffline(t *testing.T) {
	r := newRouter(nil)
	r.SetSettings(Settings{Mode: ModeUnified, Unified: Route{Provider: "qw", Model: "qwen-max"}})
	for _, task := range SortedTaskIDs() {
		if tg, ok := r.Primary(task); !ok || tg.Model != "qwen-max" {
			t.Fatalf("%s → %+v", task, tg)
		}
	}
	empty := New(nil)
	if empty.Online() {
		t.Fatal("no providers must be offline")
	}
	if ps := r.Providers(); ps[0].APIKey != "set" {
		t.Fatal("keys must be masked")
	}
}

func TestFallbackAndUsage(t *testing.T) {
	r := newRouter(map[string]error{"deepseek-chat": errors.New("http 503")})
	r.SetSettings(Settings{Mode: ModePerTask, Tasks: map[string]Route{TaskNarrate: {Provider: "ds", Fallback: []Route{{Provider: "qw"}}}}})
	var calls []Usage
	p, _, ok := r.For(TaskNarrate, func(u Usage) { calls = append(calls, u) }, nil)
	if !ok {
		t.Fatal("offline")
	}
	resp, err := p.Generate(context.Background(), provider.Request{})
	if err != nil || resp.Text != "qwen:qwen-plus" {
		t.Fatalf("fallback: %v %q", err, resp.Text)
	}
	if len(calls) != 2 || calls[0].OK || !calls[1].OK || calls[1].PromptTokens != 100 || calls[1].Task != TaskNarrate {
		t.Fatalf("usage %+v", calls)
	}
	// 内容审核拒绝不切换备用（StopOn）
	calls = nil
	r = newRouter(map[string]error{"deepseek-chat": errors.New("content_filter")})
	r.SetSettings(Settings{Mode: ModePerTask, Tasks: map[string]Route{TaskNarrate: {Provider: "ds", Fallback: []Route{{Provider: "qw"}}}}})
	p, _, _ = r.For(TaskNarrate, func(u Usage) { calls = append(calls, u) }, func(err error) bool { return err.Error() == "content_filter" })
	if _, err := p.Generate(context.Background(), provider.Request{}); err == nil || len(calls) != 1 {
		t.Fatalf("refusal must not fall through: %v %d", err, len(calls))
	}
}

func TestLocalFitHints(t *testing.T) {
	if LocalFit(TaskMemory) != FitGood || LocalFit(TaskNarrate) != FitNotRecommended || LocalFit(TaskCombat) != FitCaution {
		t.Fatal("fit table")
	}
	if (Provider{Kind: provider.KindLlamaCpp, SizeB: 1.5}).Tier() != TierMinimal {
		t.Fatal("small local model must be minimal tier")
	}
}
