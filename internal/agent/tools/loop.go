package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
)

// Budget 是一次 Agent 调用的检索预算。
type Budget struct {
	MaxIters       int // 最多几轮“模型 → 工具 → 模型”
	MaxCalls       int // 工具调用总数上限
	MaxResultRunes int // 注入上下文的工具结果总字数上限
	MaxTokens      int // 每轮研究调用的输出 token 上限
}

// DefaultBudget 是叙述者 / 导演的默认预算（保守：检索是为了补足上下文，不是为了聊天）。
func DefaultBudget() Budget {
	return Budget{MaxIters: 3, MaxCalls: 6, MaxResultRunes: 6000, MaxTokens: 600}
}

func (b Budget) norm() Budget {
	d := DefaultBudget()
	if b.MaxIters <= 0 {
		b.MaxIters = d.MaxIters
	}
	if b.MaxCalls <= 0 {
		b.MaxCalls = d.MaxCalls
	}
	if b.MaxResultRunes <= 0 {
		b.MaxResultRunes = d.MaxResultRunes
	}
	if b.MaxTokens <= 0 {
		b.MaxTokens = d.MaxTokens
	}
	return b
}

// Step 是检索过程中的一步（调试 / 测试 / 审计用）。
type Step struct {
	Tool   string `json:"tool"`
	Args   string `json:"args"`
	Result string `json:"result"`
	Error  string `json:"error,omitempty"`
}

// Outcome 是检索循环的结果。
type Outcome struct {
	Messages []provider.Message // 带上检索结果的消息（交给最终生成）
	Answer   *provider.Response // 模型在检索后直接给出的回答（nil 表示需要调用方再生成一次）
	Steps    []Step
	Mode     string // tools / prefetch / none
	Capped   bool   // 触达预算上限
}

// Loop 是工具调用循环：OpenAI 兼容函数调用；Provider 不支持时降级为关键词预检索。
type Loop struct {
	Provider    provider.Provider
	Env         *Env
	Scope       Scope
	Budget      Budget
	Temperature float64
	JSON        bool // 最终回答要求 JSON（导演提案）
}

// Run 执行检索。hint 是用于预检索的文字（玩家输入 + 需要查证的名词）。
func (l *Loop) Run(ctx context.Context, msgs []provider.Message, hint string) Outcome {
	b := l.Budget.norm()
	tc, ok := l.Provider.(provider.ToolCaller)
	if !ok || !tc.SupportsTools() {
		return Prefetch(l.Env, l.Scope, msgs, hint, b)
	}
	out := Outcome{Messages: append([]provider.Message(nil), msgs...), Mode: "tools"}
	specs := Specs(l.Scope)
	calls, used := 0, 0
	for iter := 0; iter < b.MaxIters; iter++ {
		resp, err := l.Provider.Generate(ctx, provider.Request{Messages: out.Messages, Temperature: l.Temperature, MaxTokens: b.MaxTokens, Tools: specs})
		if err != nil {
			if errors.Is(err, provider.ErrToolsUnsupported) {
				p := Prefetch(l.Env, l.Scope, msgs, hint, b)
				p.Steps = append(out.Steps, p.Steps...)
				return p
			}
			// 其它错误：放弃检索，交给调用方按原上下文生成（调用方自己的降级逻辑会接住）
			out.Mode = "none"
			return out
		}
		if len(resp.ToolCalls) == 0 {
			if strings.TrimSpace(resp.Text) != "" {
				r := resp
				out.Answer = &r
			}
			return out
		}
		out.Messages = append(out.Messages, provider.Message{Role: "assistant", Content: resp.Text, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			var result string
			step := Step{Tool: call.Function.Name, Args: call.Function.Arguments}
			switch {
			case calls >= b.MaxCalls:
				result = errJSON("检索次数已用完，请根据已有信息作答")
				out.Capped = true
			case used >= b.MaxResultRunes:
				result = errJSON("检索结果已达上限，请根据已有信息作答")
				out.Capped = true
			default:
				r, err := Call(l.Env, l.Scope, call.Function.Name, call.Function.Arguments)
				if err != nil {
					step.Error = err.Error()
				}
				if n := utf8.RuneCountInString(r); used+n > b.MaxResultRunes {
					rr := []rune(r)
					r = string(rr[:max(b.MaxResultRunes-used, 0)]) + "…（截断）"
					out.Capped = true
				}
				result = r
				used += utf8.RuneCountInString(r)
			}
			calls++
			step.Result = result
			out.Steps = append(out.Steps, step)
			out.Messages = append(out.Messages, provider.Message{Role: "tool", ToolCallID: call.ID, Name: call.Function.Name, Content: result})
		}
	}
	out.Capped = true
	return out
}

// Prefetch 是不支持函数调用时的降级：用关键词检索把最相关的资料直接放进上下文（[RETRIEVED] 段）。
func Prefetch(e *Env, sc Scope, msgs []provider.Message, hint string, b Budget) Outcome {
	b = b.norm()
	out := Outcome{Messages: append([]provider.Message(nil), msgs...), Mode: "prefetch"}
	sec := PrefetchSection(e, sc, hint, b.MaxResultRunes/2)
	if sec == "" {
		out.Mode = "none"
		return out
	}
	out.Steps = append(out.Steps, Step{Tool: "prefetch", Args: hint, Result: sec})
	// 追加到最后一条 user 消息
	for i := len(out.Messages) - 1; i >= 0; i-- {
		if out.Messages[i].Role == "user" {
			out.Messages[i].Content += "\n" + sec
			return out
		}
	}
	out.Messages = append(out.Messages, provider.Message{Role: "user", Content: sec})
	return out
}

// PrefetchSection 生成 [RETRIEVED] 段：对 hint 做一次 pack.search（当前范围），取前几条的可见资料。
func PrefetchSection(e *Env, sc Scope, hint string, maxRunes int) string {
	if e == nil || strings.TrimSpace(hint) == "" {
		return ""
	}
	v, err := search(e, sc, args{"query": hint, "limit": float64(5)})
	if err != nil {
		return ""
	}
	res, _ := v.(map[string]any)["results"].([]map[string]any)
	if len(res) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[RETRIEVED]（系统根据你的输入从故事包检索到的资料，只可作为背景，不得与 [IMMUTABLE_FACTS] 矛盾）\n")
	for _, r := range res {
		d, ok := e.findDoc(fmt.Sprint(r["id"]))
		if !ok {
			continue
		}
		line := fmt.Sprintf("- %s（%s）：%s\n", r["name"], d.Type, strings.ReplaceAll(clip(d.text(sc, e)), "\n", " "))
		if utf8.RuneCountInString(sb.String())+utf8.RuneCountInString(line) > maxRunes {
			break
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// UnknownRefs 找出文字里提到、但不在已知上下文 known 中的故事包实体名（当前范围可见的才算），用于触发检索。
func UnknownRefs(e *Env, sc Scope, text string, known []string) []string {
	if e == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, d := range e.docs() {
		if d.Type == "action" || d.Type == "anchor" || d.Type == "story" || !e.docVisible(sc, d) {
			continue
		}
		names := append([]string{d.Name}, d.Aliases...)
		for _, n := range names {
			if utf8.RuneCountInString(n) < 2 || !strings.Contains(text, n) || seen[d.Name] {
				continue
			}
			inCtx := false
			for _, k := range known {
				if strings.Contains(k, n) || strings.Contains(k, d.Name) {
					inCtx = true
					break
				}
			}
			if !inCtx {
				seen[d.Name] = true
				out = append(out, d.Name)
			}
			break
		}
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// Flatten 把检索循环的结果折叠回“原始消息 + [RETRIEVED] 段”，交给不带 tools 的最终生成
// （有的服务不接受不带 tools 声明的 tool 消息历史）。预检索模式下消息已经带了 [RETRIEVED]。
func Flatten(orig []provider.Message, o Outcome) []provider.Message {
	if o.Mode != "tools" || len(o.Steps) == 0 {
		if o.Mode == "prefetch" {
			return o.Messages
		}
		return orig
	}
	var sb strings.Builder
	sb.WriteString("[RETRIEVED]（你刚才检索到的资料，只可作为背景，不得与 [IMMUTABLE_FACTS] 矛盾）\n")
	for _, s := range o.Steps {
		fmt.Fprintf(&sb, "- %s(%s)：%s\n", s.Tool, s.Args, s.Result)
	}
	out := append([]provider.Message(nil), orig...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Role == "user" {
			out[i].Content += "\n" + sb.String()
			break
		}
	}
	return out
}
