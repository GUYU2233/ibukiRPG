// Package router 是多服务商 / 按任务的模型路由（架构 V0.3 第 11 节）：统一模型或按任务分配，
// 每个任务有降级链（主模型 → 备用 → 本地 → 离线），每次调用记账（token、耗时、成败）。
//
// 密钥只在内存中：Android 端用 Keystore 加密保存，调用 configure_ai 时解密传入；本包从不持久化密钥。
package router

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
)

// 任务 ID（第 11.2 节）。
const (
	TaskNarrate    = "narrate_world"
	TaskParse      = "parse_action"
	TaskCombat     = "combat_adjudicate"
	TaskMemory     = "memory"
	TaskWorldSim   = "world_sim"
	TaskAudit      = "audit"
	TaskCardGen    = "card_gen"
	TaskCharReview = "char_review"
)

// 路由模式。
const (
	ModeUnified = "unified"
	ModePerTask = "per_task"
)

// 能力档位（第 11.7 节）。
const (
	TierFull    = "full"
	TierLimited = "limited"
	TierMinimal = "minimal"
)

// 本地模型适合度。
const (
	FitGood           = "good"
	FitCaution        = "caution"
	FitNotRecommended = "not_recommended"
)

// TaskInfo 描述一个任务（生成设置页）。
type TaskInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Output   string `json:"output"`
	Suggest  string `json:"suggest"`
	LocalFit string `json:"local_fit"`
	LocalWhy string `json:"local_why"`
}

// Tasks 返回全部任务及其本地模型适合度（生成设置右上角提示图标的列表）。
func Tasks() []TaskInfo {
	return []TaskInfo{
		{TaskNarrate, "叙事 + 世界更新", "叙事正文 + WORLD 段", "云端主力模型", FitNotRecommended, "需要长上下文和稳定的结构化输出，小模型容易写错设定"},
		{TaskParse, "意图解析", "命令结构", "本地 3B 或云端快速模型", FitGood, "输入短、输出小，本地模型速度快"},
		{TaskCombat, "战斗裁定", "CombatIntent", "云端", FitCaution, "可以使用，但荒谬动作更容易被降级、修正标签更保守"},
		{TaskMemory, "检索 / 摘要 / 记忆压缩", "文本 / 关键词", "本地", FitGood, "后台运行，不占关键路径"},
		{TaskWorldSim, "世界模拟", "WorldChange + 文本", "云端", FitCaution, "本地模型只做小改动（传闻、环境描写）"},
		{TaskAudit, "一致性审查", "AuditReport", "云端推理模型", FitNotRecommended, "需要通读大量叙事与设定"},
		{TaskCardGen, "卡片生成", "EntityDoc 提案", "云端", FitNotRecommended, "需要了解世界观与强度体系"},
		{TaskCharReview, "角色审查", "CharacterReview", "云端", FitNotRecommended, "需要判断设定契合度与强度"},
	}
}

// TaskName 返回任务中文名。
func TaskName(id string) string {
	for _, t := range Tasks() {
		if t.ID == id {
			return t.Name
		}
	}
	return id
}

// LocalFit 返回任务对本地模型的适合度。
func LocalFit(task string) string {
	for _, t := range Tasks() {
		if t.ID == task {
			return t.LocalFit
		}
	}
	return FitCaution
}

// Provider 是一个已登记的服务商（含内存中的密钥）。
type Provider struct {
	ID      string   `json:"id"`
	Kind    string   `json:"kind"`
	Label   string   `json:"label,omitempty"`
	BaseURL string   `json:"base_url,omitempty"`
	Model   string   `json:"model,omitempty"` // 默认模型
	Models  []string `json:"models,omitempty"`
	APIKey  string   `json:"api_key,omitempty"`
	NoTools bool     `json:"no_tools,omitempty"`
	// SizeB 是本地模型参数量（十亿），用于判定能力档位（< 3 → minimal）。
	SizeB float64 `json:"size_b,omitempty"`
}

// Config 返回 provider.Config。
func (p Provider) Config(model string) provider.Config {
	if model == "" {
		model = p.Model
	}
	return provider.Config{Kind: p.Kind, BaseURL: p.BaseURL, Model: model, APIKey: p.APIKey, NoTools: p.NoTools}.Normalize()
}

// Usable 报告服务商是否可以调用（本地模型不需要密钥）。
func (p Provider) Usable(model string) bool {
	c := p.Config(model)
	if provider.IsLocalKind(p.Kind) {
		return c.BaseURL != ""
	}
	return c.Online()
}

// Tier 返回服务商的能力档位。
func (p Provider) Tier() string {
	if !provider.IsLocalKind(p.Kind) {
		return TierFull
	}
	if p.SizeB > 0 && p.SizeB < 3 {
		return TierMinimal
	}
	return TierLimited
}

// Route 是一个任务的模型分配。
type Route struct {
	Provider    string  `json:"provider"`
	Model       string  `json:"model,omitempty"`
	Fallback    []Route `json:"fallback,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
}

// Settings 是生成设置。
type Settings struct {
	Mode    string           `json:"mode"`
	Unified Route            `json:"unified"`
	Tasks   map[string]Route `json:"tasks,omitempty"`
	// AuditEvery：0 = 使用故事包默认；-1 = 关闭。
	AuditEvery int `json:"audit_every_turns,omitempty"`
	// SimEvery：场外世界模拟间隔（游戏内分钟）；0 = 使用故事包默认；-1 = 关闭。
	SimEvery int `json:"sim_every_minutes,omitempty"`
	// SimAICalls：每个游戏日最多调用 AI 世界模拟的次数（成本上限）；0 = 使用故事包默认；-1 = 只用离线规则。
	SimAICalls int `json:"sim_ai_calls_per_day,omitempty"`
	// NoWorldTools 关闭游戏内写入工具（函数调用）：世界修改只走 WORLD 段 JSON。
	NoWorldTools   bool `json:"world_tools_off,omitempty"`
	ShowUsage      bool `json:"show_token_usage"`
	RetryOnRefusal bool `json:"retry_on_refusal,omitempty"`
}

// Usage 是一次调用的记账（不含正文与密钥）。
type Usage struct {
	Task             string `json:"task"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	LatencyMS        int64  `json:"latency_ms"`
	OK               bool   `json:"ok"`
	Error            string `json:"error,omitempty"`
}

// Target 是路由解析出的一个可调用模型。
type Target struct {
	Provider provider.Provider
	Entry    Provider
	Model    string
	Tier     string
	Route    Route
}

// Router 持有服务商与生成设置。并发安全。
type Router struct {
	mu        sync.RWMutex
	providers map[string]Provider
	order     []string
	settings  Settings
	newP      func(cfg provider.Config) provider.Provider
}

// New 创建路由器；newP 为 nil 时使用 OpenAI 兼容实现。
func New(newP func(cfg provider.Config) provider.Provider) *Router {
	if newP == nil {
		newP = func(cfg provider.Config) provider.Provider { return provider.NewOpenAICompatible(cfg, nil) }
	}
	return &Router{providers: map[string]Provider{}, newP: newP, settings: Settings{Mode: ModeUnified, ShowUsage: true}}
}

// SetProviders 替换服务商列表（密钥只保存在内存）。
func (r *Router) SetProviders(ps []Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers = map[string]Provider{}
	r.order = nil
	for _, p := range ps {
		if strings.TrimSpace(p.ID) == "" {
			p.ID = p.Kind
		}
		r.providers[p.ID] = p
		r.order = append(r.order, p.ID)
	}
}

// Providers 返回服务商列表（密钥打码）。
func (r *Router) Providers() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, 0, len(r.order))
	for _, id := range r.order {
		p := r.providers[id]
		if p.APIKey != "" {
			p.APIKey = "set"
		}
		out = append(out, p)
	}
	return out
}

// SetSettings 设置生成设置。
func (r *Router) SetSettings(s Settings) {
	if s.Mode == "" {
		s.Mode = ModeUnified
	}
	r.mu.Lock()
	r.settings = s
	r.mu.Unlock()
}

// Settings 返回生成设置。
func (r *Router) Settings() Settings {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.settings
}

// routeFor 返回任务的路由（第 11.8 节：未配置时统一模式回退到统一模型，按任务模式回退到 narrate_world）。
func (r *Router) routeFor(task string) Route {
	s := r.settings
	if s.Mode == ModePerTask {
		if rt, ok := s.Tasks[task]; ok && rt.Provider != "" {
			return rt
		}
		if rt, ok := s.Tasks[TaskNarrate]; ok && rt.Provider != "" {
			return rt
		}
	}
	if s.Unified.Provider != "" {
		return s.Unified
	}
	// 没有设置：第一个可用的服务商
	for _, id := range r.order {
		if r.providers[id].Usable("") {
			return Route{Provider: id}
		}
	}
	return Route{}
}

// Chain 返回任务的降级链（只含可用的模型；空 = 离线）。
func (r *Router) Chain(task string) []Target {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt := r.routeFor(task)
	var out []Target
	seen := map[string]bool{}
	add := func(x Route) {
		p, ok := r.providers[x.Provider]
		if !ok || !p.Usable(x.Model) {
			return
		}
		cfg := p.Config(x.Model)
		key := p.ID + "|" + cfg.Model
		if seen[key] {
			return
		}
		seen[key] = true
		if x.Temperature == 0 {
			x.Temperature = rt.Temperature
		}
		if x.MaxTokens == 0 {
			x.MaxTokens = rt.MaxTokens
		}
		out = append(out, Target{Provider: r.newP(cfg), Entry: p, Model: cfg.Model, Tier: p.Tier(), Route: x})
	}
	add(rt)
	for _, f := range rt.Fallback {
		add(f)
	}
	// 统一模型作为最后的云端兜底
	if r.settings.Mode == ModePerTask && r.settings.Unified.Provider != "" {
		add(r.settings.Unified)
	}
	return out
}

// Primary 返回任务的首选模型（没有则 ok=false，表示离线）。
func (r *Router) Primary(task string) (Target, bool) {
	c := r.Chain(task)
	if len(c) == 0 {
		return Target{}, false
	}
	return c[0], true
}

// Online 报告是否有任何可用模型。
func (r *Router) Online() bool {
	_, ok := r.Primary(TaskNarrate)
	return ok
}

// UsesLocal 报告某任务的首选模型是否为本地模型。
func (r *Router) UsesLocal(task string) bool {
	t, ok := r.Primary(task)
	return ok && provider.IsLocalKind(t.Entry.Kind)
}

// LocalWarnings 列出分配给本地模型但“不推荐”的任务（生成设置页提示）。
func (r *Router) LocalWarnings() []string {
	var out []string
	for _, t := range Tasks() {
		if t.LocalFit == FitNotRecommended && r.UsesLocal(t.ID) {
			out = append(out, t.Name)
		}
	}
	return out
}

// ---------- 记账包装 ----------

// Metered 包装一个 Provider：每次调用都报告 Usage。
type Metered struct {
	P      provider.Provider
	Task   string
	PID    string
	Record func(Usage)
}

// Name 实现 provider.Provider。
func (m *Metered) Name() string { return m.P.Name() }

// SupportsTools 透传函数调用能力。
func (m *Metered) SupportsTools() bool {
	if tc, ok := m.P.(provider.ToolCaller); ok {
		return tc.SupportsTools()
	}
	return false
}

func (m *Metered) record(resp provider.Response, err error, start time.Time) {
	if m.Record == nil {
		return
	}
	u := Usage{Task: m.Task, Provider: m.PID, Model: resp.Model, PromptTokens: resp.PromptTokens, CompletionTokens: resp.CompletionTokens,
		LatencyMS: time.Since(start).Milliseconds(), OK: err == nil}
	if u.Model == "" {
		u.Model = m.P.Name()
	}
	if err != nil {
		u.Error = err.Error()
		if len(u.Error) > 200 {
			u.Error = u.Error[:200]
		}
	}
	m.Record(u)
}

// Generate 实现 provider.Provider。
func (m *Metered) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	start := time.Now()
	resp, err := m.P.Generate(ctx, req)
	m.record(resp, err, start)
	return resp, err
}

// Stream 实现 provider.Provider。
func (m *Metered) Stream(ctx context.Context, req provider.Request, onDelta func(string)) (provider.Response, error) {
	start := time.Now()
	resp, err := m.P.Stream(ctx, req, onDelta)
	m.record(resp, err, start)
	return resp, err
}

// Chained 是带降级链的 Provider：主模型失败（非内容审核拒绝）时依次尝试备用模型。
type Chained struct {
	Targets []provider.Provider
	// StopOn 返回 true 时不再尝试下一个（例如内容审核拒绝：避免把被拒内容发给另一家）。
	StopOn func(error) bool
}

// Name 实现 provider.Provider。
func (c *Chained) Name() string {
	if len(c.Targets) == 0 {
		return "offline"
	}
	return c.Targets[0].Name()
}

// SupportsTools 以主模型为准。
func (c *Chained) SupportsTools() bool {
	if len(c.Targets) == 0 {
		return false
	}
	if tc, ok := c.Targets[0].(provider.ToolCaller); ok {
		return tc.SupportsTools()
	}
	return false
}

// Generate 实现 provider.Provider。
func (c *Chained) Generate(ctx context.Context, req provider.Request) (provider.Response, error) {
	var last error
	for _, p := range c.Targets {
		resp, err := p.Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
		last = err
		if ctx.Err() != nil || (c.StopOn != nil && c.StopOn(err)) {
			break
		}
	}
	if last == nil {
		last = provider.ErrOffline
	}
	return provider.Response{}, last
}

// Stream 实现 provider.Provider。已经输出过分片的模型失败时不再切换（避免叙事重复）。
func (c *Chained) Stream(ctx context.Context, req provider.Request, onDelta func(string)) (provider.Response, error) {
	var last error
	for _, p := range c.Targets {
		sent := false
		resp, err := p.Stream(ctx, req, func(d string) {
			sent = true
			if onDelta != nil {
				onDelta(d)
			}
		})
		if err == nil {
			return resp, nil
		}
		last = err
		if sent || ctx.Err() != nil || (c.StopOn != nil && c.StopOn(err)) {
			break
		}
	}
	if last == nil {
		last = provider.ErrOffline
	}
	return provider.Response{}, last
}

// For 返回任务的可调用 Provider（降级链 + 记账）；ok=false 表示离线。
func (r *Router) For(task string, record func(Usage), stopOn func(error) bool) (provider.Provider, Target, bool) {
	chain := r.Chain(task)
	if len(chain) == 0 {
		return nil, Target{}, false
	}
	var ps []provider.Provider
	for _, t := range chain {
		ps = append(ps, &Metered{P: t.Provider, Task: task, PID: t.Entry.ID, Record: record})
	}
	if len(ps) == 1 {
		return ps[0], chain[0], true
	}
	return &Chained{Targets: ps, StopOn: stopOn}, chain[0], true
}

// FromLegacy 把 0.1.x 的单一 provider.Config 转成一个服务商 + 统一路由。
func FromLegacy(cfg provider.Config) ([]Provider, Settings) {
	cfg = cfg.Normalize()
	if cfg.Kind == provider.KindOffline {
		return nil, Settings{Mode: ModeUnified, ShowUsage: true}
	}
	p := Provider{ID: cfg.Kind, Kind: cfg.Kind, BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey, NoTools: cfg.NoTools}
	return []Provider{p}, Settings{Mode: ModeUnified, Unified: Route{Provider: p.ID, Model: cfg.Model}, ShowUsage: true}
}

// SortedTaskIDs 返回任务 ID 列表。
func SortedTaskIDs() []string {
	var out []string
	for _, t := range Tasks() {
		out = append(out, t.ID)
	}
	slices.Sort(out)
	return out
}
