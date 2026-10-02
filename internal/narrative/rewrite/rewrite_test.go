package rewrite

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTemplateDropsWholeClause(t *testing.T) {
	in := "在下水道长大，据说会一点魔法，能让齿轮自己转起来。"
	got := Template(in, []string{"魔法"}, nil)
	if got != "在下水道长大，能让齿轮自己转起来。" {
		t.Fatalf("got %q", got)
	}
	for _, bad := range []string{"会一点，", "，，", "，。"} {
		if strings.Contains(got, bad) {
			t.Fatalf("dangling fragment %q in %q", bad, got)
		}
	}
}

func TestTemplateRewriteWord(t *testing.T) {
	got := Template("据说会一点魔法。", []string{"魔法"}, map[string]string{"魔法": "修齿轮的手艺"})
	if got != "据说会一点修齿轮的手艺。" {
		t.Fatalf("got %q", got)
	}
}

func TestTemplateWholeSentence(t *testing.T) {
	got := Template("会魔法。喜欢喝茶，讨厌下雨。", []string{"魔法"}, nil)
	if got != "喜欢喝茶，讨厌下雨。" {
		t.Fatalf("got %q", got)
	}
	if got := Template("魔法", []string{"魔法"}, nil); got != "" {
		t.Fatalf("all-forbidden text should become empty, got %q", got)
	}
}

func TestRewriteAIThenFallback(t *testing.T) {
	ok := func(context.Context, string, []string) (string, error) {
		return "从小摆弄齿轮，手很巧。", nil
	}
	got, src := Rewrite(context.Background(), ok, "据说会一点魔法，能让齿轮自己转起来。", []string{"魔法"}, nil, 100)
	if src != "ai" || got != "从小摆弄齿轮，手很巧。" {
		t.Fatalf("%s %q", src, got)
	}
	bad := func(context.Context, string, []string) (string, error) { return "还是会魔法", nil }
	if _, src := Rewrite(context.Background(), bad, "会魔法，爱笑。", []string{"魔法"}, nil, 100); src != "template" {
		t.Fatalf("AI output still forbidden must fall back, got %s", src)
	}
	fail := func(context.Context, string, []string) (string, error) { return "", errors.New("offline") }
	got, src = Rewrite(context.Background(), fail, "会魔法，爱笑。", []string{"魔法"}, nil, 100)
	if src != "template" || got != "爱笑。" {
		t.Fatalf("%s %q", src, got)
	}
	if _, src := Rewrite(context.Background(), nil, "爱笑。", []string{"魔法"}, nil, 100); src != "none" {
		t.Fatal("clean text untouched")
	}
}
