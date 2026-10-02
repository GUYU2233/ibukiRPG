package eval

import (
	"context"
	"errors"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/agent/simulator"
	"github.com/GUYU2233/ibukiRPG/internal/narrative/rewrite"
	"github.com/GUYU2233/ibukiRPG/internal/world/power"
)

// ---------- 0.2.0-rc1 用例：世界模拟 / 强度预算 / 禁用词改写（确定性，不需要录音）----------

// SimCase 是世界模拟输出用例（world_sim 套件）：模型原始回复 → 清洗后的传闻与场外变更。
type SimCase struct {
	Raw           string   `yaml:"raw"`
	Compact       bool     `yaml:"compact"`
	ExpectError   bool     `yaml:"expect_error"`
	ExpectNews    int      `yaml:"expect_news"`
	ExpectChanges int      `yaml:"expect_changes"`
	ForbidTargets []string `yaml:"forbid_targets"`
}

// PowerCase 是强度预算用例（power_budget 套件）：种类 × 稀有度 × 等级 → 预算；数值缩放后不超过预算。
type PowerCase struct {
	Ref          int            `yaml:"ref"`
	Kind         string         `yaml:"kind"`
	Rarity       string         `yaml:"rarity"`
	Level        int            `yaml:"level"`
	Stats        map[string]int `yaml:"stats"`
	ExpectBudget int            `yaml:"expect_budget"`
	ExpectFitted bool           `yaml:"expect_fitted"`
	// HigherThan：同一参考强度下，这个参数组合的预算应当更高。
	HigherThan *power.Params `yaml:"higher_than"`
}

// RewriteCase 是禁用词改写用例（rewrite 套件）：AI 改写（可空 / 失败 / 不合格）→ 模板兜底。
type RewriteCase struct {
	Text      string            `yaml:"text"`
	Forbidden []string          `yaml:"forbidden"`
	Rewrites  map[string]string `yaml:"rewrites"`
	// AI 是假 AI 的改写结果；AIError 为真时假 AI 返回错误；都为空时没有 AI。
	AI           string   `yaml:"ai"`
	AIError      bool     `yaml:"ai_error"`
	ExpectSource string   `yaml:"expect_source"` // ai / template / none
	ExpectHas    []string `yaml:"expect_contains"`
	ExpectLacks  []string `yaml:"expect_lacks"`
}

func runSim(c Case) Result {
	sc := c.Sim
	rs := newResult(c, "structured")
	o, err := simulator.Parse(sc.Raw, sc.Compact)
	if sc.ExpectError {
		if err == nil {
			rs.fail("expected parse error")
		}
		return rs.Result
	}
	if err != nil {
		rs.fail("parse: %v", err)
		return rs.Result
	}
	if len(o.News) != sc.ExpectNews {
		rs.fail("news=%d want %d", len(o.News), sc.ExpectNews)
	}
	if len(o.Changes) != sc.ExpectChanges {
		rs.fail("changes=%d want %d", len(o.Changes), sc.ExpectChanges)
	}
	for _, ch := range o.Changes {
		if !strings.HasPrefix(ch.Reason, "场外") {
			rs.fail("reason %q not prefixed 场外", ch.Reason)
		}
		for _, t := range sc.ForbidTargets {
			if ch.Target == t || strings.HasPrefix(ch.Target, t+">") {
				rs.fail("forbidden target %s kept", t)
			}
		}
	}
	return rs.Result
}

func runPower(c Case) Result {
	pc := c.Power
	rs := newResult(c, "rules")
	p := power.Params{Kind: pc.Kind, Rarity: pc.Rarity, Level: pc.Level}
	b := power.Budget(pc.Ref, p)
	if pc.ExpectBudget > 0 && b != pc.ExpectBudget {
		rs.fail("budget=%d want %d", b, pc.ExpectBudget)
	}
	if pc.HigherThan != nil {
		if o := power.Budget(pc.Ref, *pc.HigherThan); b <= o {
			rs.fail("budget %d not higher than %+v (%d)", b, *pc.HigherThan, o)
		}
	}
	if len(pc.Stats) > 0 {
		out, fitted := power.Fit(pc.Stats, b)
		if fitted != pc.ExpectFitted {
			rs.fail("fitted=%v want %v", fitted, pc.ExpectFitted)
		}
		if s := power.Score(out); b > 0 && s > b {
			rs.fail("score %d over budget %d after fit", s, b)
		}
	}
	return rs.Result
}

func runRewrite(c Case) Result {
	rc := c.Rewrite
	rs := newResult(c, "rules")
	var ai rewrite.AI
	switch {
	case rc.AIError:
		ai = func(context.Context, string, []string) (string, error) { return "", errors.New("offline") }
	case rc.AI != "":
		ai = func(context.Context, string, []string) (string, error) { return rc.AI, nil }
	}
	out, src := rewrite.Rewrite(context.Background(), ai, rc.Text, rc.Forbidden, rc.Rewrites, 0)
	if rc.ExpectSource != "" && src != rc.ExpectSource {
		rs.fail("source=%s want %s (%q)", src, rc.ExpectSource, out)
	}
	if rewrite.Contains(out, rc.Forbidden) {
		rs.fail("forbidden word kept: %q", out)
	}
	for _, s := range rc.ExpectHas {
		if !strings.Contains(out, s) {
			rs.fail("missing %q in %q", s, out)
		}
	}
	for _, s := range rc.ExpectLacks {
		if strings.Contains(out, s) {
			rs.fail("unexpected %q in %q", s, out)
		}
	}
	return rs.Result
}
