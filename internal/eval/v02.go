package eval

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/agent/audit"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// ---------- 0.2.0 用例：合并输出 / 提示灵敏度 / 按任务路由（确定性，不需要录音）----------

// MergedCase 是合并结构化输出用例（merged_output 套件）：模型原始回复 → 叙事 + 世界更新。
type MergedCase struct {
	Raw string `yaml:"raw"`
	// ExpectWorld：应解析出世界更新；ExpectRepaired：需要修复才能解析；ExpectBroken：修复后仍失败（回退为只有叙事）。
	ExpectWorld     bool     `yaml:"expect_world"`
	ExpectRepaired  bool     `yaml:"expect_repaired"`
	ExpectBroken    bool     `yaml:"expect_broken"`
	ExpectRefusal   bool     `yaml:"expect_refusal"`
	ExpectChanges   int      `yaml:"expect_changes"`
	NarrationHas    []string `yaml:"narration_contains"`
	NarrationLacks  []string `yaml:"narration_lacks"`
	ExpectRevealCnt int      `yaml:"expect_reveals"`
}

// SensitivityCase 是提示灵敏度用例：本回合影响 + 设置 → 是否提示 / 仅通知。
type SensitivityCase struct {
	Impact   map[string]any    `yaml:"impact"`   // change.TurnImpact（JSON 字段名）
	Settings map[string]string `yaml:"settings"` // type → "level[/notify]"
	Expect   string            `yaml:"expect"`   // none | <type> | <type>/notify
}

// RoutingCase 是按任务路由用例：服务商 + 生成设置 → 某任务实际使用的服务商 / 档位。
type RoutingCase struct {
	Providers      []map[string]any `yaml:"providers"` // router.Provider（JSON 字段名）
	Settings       map[string]any   `yaml:"settings"`  // router.Settings（JSON 字段名）
	Task           string           `yaml:"task"`
	ExpectProvider string           `yaml:"expect_provider"` // 空 = 应当没有可用服务商（离线）
	ExpectTier     string           `yaml:"expect_tier"`
	ExpectChain    int              `yaml:"expect_chain"`
	ExpectWarning  bool             `yaml:"expect_local_warning"`
}

// AuditCase 是一致性审查用例（audit 套件）：审查 Agent 的原始输出 → 自动修复 / 建议 / 更正提示的分类。
type AuditCase struct {
	Raw string `yaml:"raw"`
	// Kinds 是实体类型表（实体 ID → character / location / item …），判断“生死”这类机械状态。
	Kinds             map[string]string `yaml:"kinds"`
	ExpectAutos       int               `yaml:"expect_autos"`
	ExpectSuggestions int               `yaml:"expect_suggestions"`
	ExpectCorrections int               `yaml:"expect_corrections"`
	ExpectError       bool              `yaml:"expect_error"`
}

type resultBuilder struct{ Result }

func newResult(c Case, mode string) *resultBuilder {
	return &resultBuilder{Result{Case: c.ID, Suite: c.Suite, Mode: mode, Pass: true}}
}

func (r *resultBuilder) fail(format string, a ...any) {
	r.Pass = false
	if r.Detail != "" {
		r.Detail += "; "
	}
	r.Detail += fmt.Sprintf(format, a...)
}

func runMerged(c Case) Result {
	m := c.Merged
	rs := newResult(c, "structured")
	if m.ExpectRefusal != structured.IsRefusal(m.Raw, nil) {
		rs.fail("refusal=%v want %v", !m.ExpectRefusal, m.ExpectRefusal)
	}
	narr, raw, found := structured.Split(m.Raw)
	for _, s := range m.NarrationHas {
		if !strings.Contains(narr, s) {
			rs.fail("narration missing %q", s)
		}
	}
	for _, s := range m.NarrationLacks {
		if strings.Contains(narr, s) {
			rs.fail("narration leaks %q", s)
		}
	}
	if !found {
		if m.ExpectWorld {
			rs.fail("no <<<WORLD>>> section")
		}
		return rs.Result
	}
	w, repaired, err := structured.Parse(raw)
	switch {
	case m.ExpectBroken:
		if err == nil {
			rs.fail("expected unrecoverable JSON, parsed ok")
		}
		return rs.Result
	case err != nil:
		rs.fail("parse: %v", err)
		return rs.Result
	}
	if repaired != m.ExpectRepaired {
		rs.fail("repaired=%v want %v", repaired, m.ExpectRepaired)
	}
	if m.ExpectWorld && w.Empty() {
		rs.fail("world update empty")
	}
	if m.ExpectChanges > 0 && len(w.AllChanges()) != m.ExpectChanges {
		rs.fail("changes=%d want %d", len(w.AllChanges()), m.ExpectChanges)
	}
	if m.ExpectRevealCnt > 0 && len(w.Reveals) != m.ExpectRevealCnt {
		rs.fail("reveals=%d want %d", len(w.Reveals), m.ExpectRevealCnt)
	}
	return rs.Result
}

func runSensitivity(c Case) Result {
	sc := c.Sensitivity
	rs := newResult(c, "rules")
	st := change.DefaultSettings()
	for typ, v := range sc.Settings {
		level, mode, _ := strings.Cut(v, "/")
		sens := change.Sensitivity{Level: level, Mode: mode}
		switch typ {
		case change.TypeLoreDeviation:
			st.LoreDeviation = sens
		case change.TypeMajorDeath:
			st.MajorDeath = sens
		case change.TypeStoryImpact:
			st.StoryImpact = sens
		default:
			rs.fail("unknown prompt type %s", typ)
		}
	}
	var imp change.TurnImpact
	if err := viaJSON(sc.Impact, &imp); err != nil {
		rs.fail("impact: %v", err)
	}
	got := "none"
	if p := change.Decide(imp, st); p != nil {
		got = p.Type
		if p.Notify {
			got += "/notify"
		}
	}
	if got != sc.Expect {
		rs.fail("prompt=%s want %s", got, sc.Expect)
	}
	return rs.Result
}

func runRouting(c Case) Result {
	rc := c.Routing
	rs := newResult(c, "rules")
	r := router.New(func(cfg provider.Config) provider.Provider { return provider.NewOpenAICompatible(cfg, nil) })
	var ps []router.Provider
	var st router.Settings
	if err := viaJSON(rc.Providers, &ps); err != nil {
		rs.fail("providers: %v", err)
	}
	if err := viaJSON(rc.Settings, &st); err != nil {
		rs.fail("settings: %v", err)
	}
	r.SetProviders(ps)
	r.SetSettings(st)
	tg, ok := r.Primary(rc.Task)
	switch {
	case rc.ExpectProvider == "" && ok:
		rs.fail("expected offline, got %s", tg.Entry.ID)
	case rc.ExpectProvider != "" && !ok:
		rs.fail("no provider for %s, want %s", rc.Task, rc.ExpectProvider)
	case ok && tg.Entry.ID != rc.ExpectProvider:
		rs.fail("provider=%s want %s", tg.Entry.ID, rc.ExpectProvider)
	}
	if ok && rc.ExpectTier != "" && tg.Tier != rc.ExpectTier {
		rs.fail("tier=%s want %s", tg.Tier, rc.ExpectTier)
	}
	if rc.ExpectChain > 0 && len(r.Chain(rc.Task)) != rc.ExpectChain {
		rs.fail("chain=%d want %d", len(r.Chain(rc.Task)), rc.ExpectChain)
	}
	if rc.ExpectWarning != (len(r.LocalWarnings()) > 0) {
		rs.fail("local warnings=%v", r.LocalWarnings())
	}
	return rs.Result
}

// viaJSON 把 YAML 解出的通用值按 JSON 字段名转成目标结构（运行时类型只有 json 标签）。
func viaJSON(v, out any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func runAudit(c Case) Result {
	a := c.Audit
	rs := newResult(c, "audit")
	fs, err := audit.Parse(a.Raw)
	if a.ExpectError {
		if err == nil {
			rs.fail("expected parse error")
		}
		return rs.Result
	}
	if err != nil {
		rs.fail("parse: %v", err)
		return rs.Result
	}
	p := audit.Classify(fs, func(id string) string { return a.Kinds[id] })
	if len(p.Autos) != a.ExpectAutos {
		rs.fail("autos=%d want %d", len(p.Autos), a.ExpectAutos)
	}
	if len(p.Suggestions) != a.ExpectSuggestions {
		rs.fail("suggestions=%d want %d", len(p.Suggestions), a.ExpectSuggestions)
	}
	if len(p.Corrections) != a.ExpectCorrections {
		rs.fail("corrections=%d want %d", len(p.Corrections), a.ExpectCorrections)
	}
	return rs.Result
}
