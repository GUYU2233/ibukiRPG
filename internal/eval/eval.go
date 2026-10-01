// Package eval 实现 LLM Eval Runner（架构文档第 39-40 节）。
//
// 用例位于 tests/eval/<suite>/*.yaml，分两类：
//   - 解析用例（intent / reasonability / action_matching / freeform_action）：
//     同一输入分别交给离线解析器与 LLM 解析器（经 Recorded Transport 回放录音）评估；
//   - 0.2.0 确定性用例（merged_output / sensitivity / routing）：合并输出解析与修复、提示灵敏度、按任务路由；
//   - 叙事用例（narrative_consistency / secret_leakage）：把候选叙事交给 Narrative Guard，
//     检查是否按预期放行或拦截。
//
// 录音按请求哈希存放在 tests/eval/recordings/。用例里的 llm_output 是“模型应当返回的内容”，
// 用 -synthesize 可据此生成（合成）录音；用 -record 并配置真实 API Key 可录制真实模型回复。
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GUYU2233/ibukiRPG/internal/action/resolver"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/transport"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/narrative/guard"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// Case 是一个 Eval 用例。
type Case struct {
	ID          string   `yaml:"id"`
	Description string   `yaml:"description"`
	Suite       string   `yaml:"-"`
	File        string   `yaml:"-"`
	Setup       Setup    `yaml:"setup"`
	Input       string   `yaml:"input"`
	Expected    Expected `yaml:"expected"`
	// Modes 限定评估方式：offline / llm。为空时两者都评估（有 llm_output 才评估 llm）。
	Modes     []string       `yaml:"modes"`
	LLMOutput map[string]any `yaml:"llm_output"`
	Guard     *GuardCase     `yaml:"guard"`
	Lookup    *LookupCase    `yaml:"lookup"`
	// 0.2.0：合并输出 / 提示灵敏度 / 按任务路由（见 v02.go）。
	Merged      *MergedCase      `yaml:"merged"`
	Sensitivity *SensitivityCase `yaml:"sensitivity"`
	Routing     *RoutingCase     `yaml:"routing"`
	Audit       *AuditCase       `yaml:"audit"`
}

// LookupCase 是检索用例（retrieval 套件）：正确答案不在默认上下文里，必须用只读工具查到；
// 同时检查范围过滤（NPC / 叙述者不能查到秘密）。
//   - tools 模式：脚本化的模型（进程内假 Provider，确定性）依次调用 calls 里的工具，检查工具结果；
//   - prefetch 模式：模拟不支持函数调用的本地模型，用关键词预检索（[RETRIEVED] 段）。
type LookupCase struct {
	Scope    string       `yaml:"scope"`
	Question string       `yaml:"question"`
	Calls    []LookupCall `yaml:"calls"`
	Prefetch bool         `yaml:"prefetch"`
	// NeedsLookup 要求 expect_contains 不出现在默认上下文（导演提示词的公开资料）里。
	NeedsLookup    bool     `yaml:"needs_lookup"`
	ExpectContains []string `yaml:"expect_contains"`
	ExpectAbsent   []string `yaml:"expect_absent"`
}

// LookupCall 是脚本化模型的一次工具调用。
type LookupCall struct {
	Tool string         `yaml:"tool"`
	Args map[string]any `yaml:"args"`
}

// Setup 在初始状态上做最小修改。
type Setup struct {
	Location  string            `yaml:"location"`
	Flags     []string          `yaml:"flags"`
	NPCs      map[string]string `yaml:"npc_locations"`
	Inventory map[string]int    `yaml:"inventory"`
	Gold      *int              `yaml:"gold"`
}

// Expected 是解析期望。字段为空表示不检查。
type Expected struct {
	Kind        string `yaml:"kind"`
	Action      string `yaml:"action"`
	Target      string `yaml:"target"`
	Item        string `yaml:"item"`
	Destination string `yaml:"destination"`
	Reject      *bool  `yaml:"reject"`
	// FreeformTag 要求 freeform 带有该标签或效果类型。
	FreeformTag string `yaml:"freeform_tag"`
}

// GuardCase 是叙事守卫用例。
type GuardCase struct {
	Text     string        `yaml:"text"`
	Facts    guard.Facts   `yaml:"facts"`
	Context  guard.Context `yaml:"context"`
	ExpectOK bool          `yaml:"expect_ok"`
	Rules    []string      `yaml:"expect_rules"`
}

// Result 是单个用例在某种方式下的结果。
type Result struct {
	Case   string `json:"case"`
	Suite  string `json:"suite"`
	Mode   string `json:"mode"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

// EvalConfig 是 LLM 模式使用的固定 Provider 配置（影响请求哈希）。
var EvalConfig = provider.Config{Kind: provider.KindDeepSeek, BaseURL: "https://api.deepseek.com", Model: "deepseek-chat", APIKey: "eval"}

// LoadCases 读取 dir 下所有用例。
func LoadCases(dir string) ([]Case, error) {
	var cases []Case
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		b, err := os.ReadFile(path) //nolint:gosec // 用例目录由调用方指定
		if err != nil {
			return err
		}
		var c Case
		if err := yaml.Unmarshal(b, &c); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		rel, _ := filepath.Rel(dir, path)
		c.File = rel
		c.Suite = filepath.Dir(rel)
		if c.ID == "" {
			c.ID = strings.TrimSuffix(rel, ".yaml")
		}
		cases = append(cases, c)
		return nil
	})
	sort.Slice(cases, func(i, j int) bool { return cases[i].ID < cases[j].ID })
	return cases, err
}

// Runner 执行用例。
type Runner struct {
	Pkg           *loader.Package
	RecordingsDir string
	// Upstream 非 nil 时为真实录制模式。
	Upstream http.RoundTripper
}

func (r *Runner) state(c Case) *state.State {
	s := state.New(r.Pkg, 1, "测试者")
	if c.Setup.Location != "" {
		s.Player.Location = resolver.LookupID(r.Pkg, "location", c.Setup.Location)
	}
	for _, f := range c.Setup.Flags {
		s.Flags[f] = true
	}
	for npc, loc := range c.Setup.NPCs {
		if id := resolver.LookupID(r.Pkg, "npc", npc); id != "" {
			s.NPCs[id].Location = resolver.LookupID(r.Pkg, "location", loc)
		}
	}
	for it, n := range c.Setup.Inventory {
		if id := resolver.LookupID(r.Pkg, "item", it); id != "" {
			s.Player.Inventory[id] = n
		}
	}
	if c.Setup.Gold != nil {
		s.Player.Gold = *c.Setup.Gold
	}
	return s
}

func (c Case) wants(mode string) bool {
	if len(c.Modes) == 0 {
		return mode == "offline" || c.LLMOutput != nil
	}
	for _, m := range c.Modes {
		if m == mode {
			return true
		}
	}
	return false
}

// Run 执行全部用例。
func (r *Runner) Run(ctx context.Context, cases []Case) []Result {
	var out []Result
	for _, c := range cases {
		if c.Guard != nil {
			out = append(out, r.runGuard(c))
			continue
		}
		if c.Lookup != nil {
			out = append(out, r.runLookup(ctx, c)...)
			continue
		}
		switch {
		case c.Merged != nil:
			out = append(out, runMerged(c))
			continue
		case c.Sensitivity != nil:
			out = append(out, runSensitivity(c))
			continue
		case c.Routing != nil:
			out = append(out, runRouting(c))
			continue
		case c.Audit != nil:
			out = append(out, runAudit(c))
			continue
		}
		in := resolver.Input{Text: c.Input, Pkg: r.Pkg, State: r.state(c)}
		if c.wants("offline") {
			res, err := resolver.Offline{}.Resolve(ctx, in)
			out = append(out, r.judge(c, "offline", res, err))
		}
		if c.wants("llm") {
			tr := &transport.Recorded{Dir: r.RecordingsDir, Upstream: r.Upstream}
			l := &resolver.LLM{Provider: provider.NewOpenAICompatible(EvalConfig, tr)}
			res, err := l.Resolve(ctx, in)
			out = append(out, r.judge(c, "llm", res, err))
		}
	}
	return out
}

func short(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func (r *Runner) judge(c Case, mode string, res resolver.Resolution, err error) Result {
	rs := Result{Case: c.ID, Suite: c.Suite, Mode: mode, Pass: true}
	fail := func(format string, a ...any) {
		rs.Pass = false
		if rs.Detail != "" {
			rs.Detail += "; "
		}
		rs.Detail += fmt.Sprintf(format, a...)
	}
	if err != nil {
		fail("error: %v", err)
		return rs
	}
	e := c.Expected
	if e.Kind != "" && res.Kind != e.Kind {
		fail("kind=%s want %s", res.Kind, e.Kind)
	}
	if e.Reject != nil && (res.Kind == resolver.KindReject) != *e.Reject {
		fail("reject=%v want %v (%s)", res.Kind == resolver.KindReject, *e.Reject, res.Reason)
	}
	if e.Action != "" && short(res.Action) != e.Action {
		fail("action=%s want %s", short(res.Action), e.Action)
	}
	if e.Target != "" {
		got := short(res.Target)
		if res.Freeform != nil && got == "" && len(res.Freeform.Targets) > 0 {
			got = short(res.Freeform.Targets[0])
		}
		if got != e.Target {
			fail("target=%s want %s", got, e.Target)
		}
	}
	if e.Item != "" && short(res.Item) != e.Item {
		fail("item=%s want %s", short(res.Item), e.Item)
	}
	if e.Destination != "" && short(res.Destination) != e.Destination {
		fail("destination=%s want %s", short(res.Destination), e.Destination)
	}
	if e.FreeformTag != "" {
		ok := false
		if res.Freeform != nil {
			for _, t := range res.Freeform.Tags {
				ok = ok || t == e.FreeformTag
			}
			for _, pe := range res.Freeform.Effects {
				ok = ok || pe.Type == e.FreeformTag
			}
		}
		if !ok {
			fail("freeform tag/effect %q missing", e.FreeformTag)
		}
	}
	return rs
}

func (r *Runner) runGuard(c Case) Result {
	g := c.Guard
	rep := guard.Check(g.Text, g.Facts, g.Context)
	rs := Result{Case: c.ID, Suite: c.Suite, Mode: "guard", Pass: rep.OK == g.ExpectOK}
	if !rs.Pass {
		rs.Detail = fmt.Sprintf("ok=%v want %v %v", rep.OK, g.ExpectOK, rep.Violations)
	}
	for _, want := range g.Rules {
		found := false
		for _, v := range rep.Violations {
			found = found || v.Rule == want
		}
		if !found {
			rs.Pass = false
			rs.Detail += fmt.Sprintf(" missing rule %s", want)
		}
	}
	return rs
}

// Synthesize 根据用例中的 llm_output 写入合成录音（模拟模型回复）。
// 返回写入的录音数。录音文件含 comment 字段标注“synthetic”。
func (r *Runner) Synthesize(ctx context.Context, cases []Case) (int, error) {
	n := 0
	for _, c := range cases {
		if c.Guard != nil || c.Lookup != nil || c.Merged != nil || c.Sensitivity != nil || c.Routing != nil || c.LLMOutput == nil || !c.wants("llm") {
			continue
		}
		content, err := json.Marshal(c.LLMOutput)
		if err != nil {
			return n, err
		}
		body, _ := json.Marshal(map[string]any{
			"id": "synthetic-" + c.ID, "object": "chat.completion", "model": EvalConfig.Model,
			"choices": []any{map[string]any{"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": string(content)}}},
			"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0},
		})
		var captured []byte
		capture := transport.Func(func(req *http.Request) (*http.Response, error) {
			key := transport.Hash(req.Method, trimPath(req.URL.Path), readAll(req))
			captured = []byte(key)
			return &http.Response{StatusCode: 599, Body: http.NoBody, Request: req}, nil
		})
		in := resolver.Input{Text: c.Input, Pkg: r.Pkg, State: r.state(c)}
		l := &resolver.LLM{Provider: provider.NewOpenAICompatible(EvalConfig, capture)}
		_, _ = l.Resolve(ctx, in)
		if captured == nil {
			return n, fmt.Errorf("%s: request not captured", c.ID)
		}
		rec := transport.Recording{Status: 200, Body: string(body), Comment: "synthetic: " + c.ID}
		if err := transport.Save(r.RecordingsDir, string(captured), rec); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Summary 汇总结果。
func Summary(results []Result) (pass, total int, bySuite map[string][2]int) {
	bySuite = map[string][2]int{}
	for _, r := range results {
		k := r.Suite + "/" + r.Mode
		v := bySuite[k]
		v[1]++
		total++
		if r.Pass {
			v[0]++
			pass++
		}
		bySuite[k] = v
	}
	return pass, total, bySuite
}

func trimPath(p string) string {
	if i := strings.Index(p, "/chat/completions"); i >= 0 {
		return p[i:]
	}
	return p
}

func readAll(req *http.Request) []byte {
	if req.Body == nil {
		return nil
	}
	b, _ := io.ReadAll(req.Body)
	return b
}
