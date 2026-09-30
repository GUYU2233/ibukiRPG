package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/agent/canon"
	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/transport"
)

// scriptedModel 是按脚本调用工具的假模型（OpenAI 兼容响应），最后给出固定回答。
func scriptedModel(calls []LookupCall) http.RoundTripper {
	return transport.Func(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(b, &req)
		done := 0
		for _, m := range req.Messages {
			if m.Role == "tool" {
				done++
			}
		}
		msg := map[string]any{"role": "assistant", "content": "（根据检索结果作答）"}
		if done < len(calls) {
			c := calls[done]
			args, _ := json.Marshal(c.Args)
			name := strings.ReplaceAll(c.Tool, ".", "_")
			msg = map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{
				"id": fmt.Sprintf("call_%d", done), "type": "function", "function": map[string]any{"name": name, "arguments": string(args)}}}}
		}
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": msg}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})
}

func (r *Runner) runLookup(ctx context.Context, c Case) []Result {
	lc := c.Lookup
	sc, err := tools.ParseScope(lc.Scope)
	if err != nil {
		return []Result{{Case: c.ID, Suite: c.Suite, Mode: "lookup", Detail: err.Error()}}
	}
	st := r.state(c)
	env := &tools.Env{Pkg: r.Pkg, State: st}
	judge := func(mode, got string, extra ...string) Result {
		rs := Result{Case: c.ID, Suite: c.Suite, Mode: mode, Pass: true}
		fail := func(f string, a ...any) {
			rs.Pass = false
			if rs.Detail != "" {
				rs.Detail += "; "
			}
			rs.Detail += fmt.Sprintf(f, a...)
		}
		for _, w := range lc.ExpectContains {
			if !strings.Contains(got, w) {
				fail("missing %q", w)
			}
		}
		for _, w := range lc.ExpectAbsent {
			if strings.Contains(got, w) {
				fail("leaked %q", w)
			}
		}
		for _, e := range extra {
			fail("%s", e)
		}
		return rs
	}
	var out []Result
	var pre []string
	if lc.NeedsLookup {
		var base strings.Builder
		for _, m := range canon.BuildMessages(r.Pkg, st, nil) {
			base.WriteString(m.Content)
		}
		for _, w := range lc.ExpectContains {
			if strings.Contains(base.String(), w) {
				pre = append(pre, fmt.Sprintf("%q is already in the default context (case does not need a lookup)", w))
			}
		}
	}
	if len(lc.Calls) > 0 {
		loop := &tools.Loop{Provider: provider.NewOpenAICompatible(EvalConfig, scriptedModel(lc.Calls)), Env: env, Scope: sc,
			Budget: tools.Budget{MaxIters: len(lc.Calls) + 1, MaxCalls: len(lc.Calls)}}
		o := loop.Run(ctx, []provider.Message{{Role: "user", Content: lc.Question}}, lc.Question)
		var got strings.Builder
		extra := append([]string(nil), pre...)
		if o.Mode != "tools" || len(o.Steps) != len(lc.Calls) {
			extra = append(extra, fmt.Sprintf("mode=%s steps=%d", o.Mode, len(o.Steps)))
		}
		for _, s := range o.Steps {
			got.WriteString(s.Result)
		}
		out = append(out, judge("tools", got.String(), extra...))
	}
	if lc.Prefetch {
		out = append(out, judge("prefetch", tools.PrefetchSection(env, sc, lc.Question, 4000), pre...))
	}
	return out
}
