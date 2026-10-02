package orchestrator_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
)

const forbiddenStory = "在下水道长大，据说会一点魔法，能让齿轮自己转起来。"

// TestCreationForbiddenTemplateRewrite：离线时禁用词用模板改写（故事包 rewrites 或删去整个分句），不会留下残句。
func TestCreationForbiddenTemplateRewrite(t *testing.T) {
	s := openPacks(t, nil)
	rv, err := s.ReviewCreation(context.Background(), "brass_trial", engine.Creation{Name: "白鸦", Custom: true, Background: "scavenger",
		Story: forbiddenStory, Personality: "无敌的乐天派，喜欢龙，爱笑"})
	if err != nil {
		t.Fatal(err)
	}
	var rec engine.Creation
	if err := json.Unmarshal(rv.Recommended, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Story != "在下水道长大，据说会一点修理机械的诀窍，能让齿轮自己转起来。" {
		t.Fatalf("story %q", rec.Story)
	}
	// 无敌 → 很能扛（改写词）；“喜欢龙”没有改写词 → 删去整个分句
	if rec.Personality != "很能扛的乐天派，爱笑" {
		t.Fatalf("personality %q", rec.Personality)
	}
	for _, bad := range []string{"会一点，", "，，", "魔法", "龙"} {
		if strings.Contains(rec.Story+rec.Personality, bad) {
			t.Fatalf("leftover %q", bad)
		}
	}
}

// rewriteLLM：角色审查返回仍含禁用词的推荐卡；改写请求返回通顺的改写。
type rewriteLLM struct{ rewrites int }

func (f *rewriteLLM) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	switch {
	case strings.Contains(string(body), "设定编辑"):
		f.rewrites++
		return chat(`{"text":"在下水道长大，从小摆弄齿轮，修好过半座镇子的管道阀门。"}`), nil
	case strings.Contains(string(body), "角色审查员"):
		return chat(`{"lore_fit":"minor","conflicts":["这个世界没有魔法"],"suggestions":["把魔法换成机械天赋"],"recommended":{"story":"会魔法的下水道孩子。"}}`), nil
	}
	return chat(`{}`), nil
}

// TestCreationForbiddenAIRewrite：有角色审查模型时由 AI 改写整句；AI 的推荐卡仍违规时再次改写，而不是删词。
func TestCreationForbiddenAIRewrite(t *testing.T) {
	f := &rewriteLLM{}
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}})
	rv, err := s.ReviewCreation(context.Background(), "brass_trial", engine.Creation{Name: "白鸦", Custom: true, Background: "scavenger", Story: forbiddenStory})
	if err != nil {
		t.Fatal(err)
	}
	var rec engine.Creation
	_ = json.Unmarshal(rv.Recommended, &rec)
	if rv.Source != "ai" || rec.Story != "在下水道长大，从小摆弄齿轮，修好过半座镇子的管道阀门。" || f.rewrites == 0 {
		t.Fatalf("source %s story %q rewrites %d", rv.Source, rec.Story, f.rewrites)
	}
}
