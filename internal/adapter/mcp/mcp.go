// Package mcp 是 MCP 适配器（架构文档第 48 节）：把只读检索工具（internal/agent/tools）
// 以 Model Context Protocol 服务器的形式暴露给外部 MCP 客户端（桌面 / CLI，stdio 传输）。
//
// 默认只暴露读取工具。0.2 起可选的世界写入工具（预览 → 确认 / 撤销 / 检查点）只有在
// `--allow-write` 且范围为 director / author 时才会列出，并与游戏内修改走同一个校验网关与事件日志（第 14.5 节）。
// 每次调用都会重新载入存档，因此一边玩一边查也能看到最新状态。
// 移动端不走 MCP，而是通过进程内工具网关（mobile 适配器的 list_tools / call_tool）。
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
)

// ProtocolVersion 是本服务器实现的 MCP 协议版本（客户端请求其它版本时按客户端的版本回应）。
const ProtocolVersion = "2025-06-18"

// EnvFunc 为每次工具调用提供检索环境。
type EnvFunc func(ctx context.Context) (*tools.Env, error)

// Server 是 MCP 服务器。
type Server struct {
	Env     EnvFunc
	Scope   tools.Scope
	Name    string
	Version string
	// World 提供 0.2 世界层工具（可为空）；AllowWrite 打开写入工具。
	World      World
	AllowWrite bool

	mu sync.Mutex // 串行写出
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// JSON-RPC 错误码。
const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Serve 在 r / w 上运行 stdio 传输（每行一条 JSON-RPC 消息），直到输入结束或 ctx 取消。
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") { // 批量请求
			var batch []json.RawMessage
			if err := json.Unmarshal([]byte(line), &batch); err != nil {
				s.write(w, response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "parse error"}})
				continue
			}
			var out []response
			for _, m := range batch {
				if resp, ok := s.handle(ctx, m); ok {
					out = append(out, resp)
				}
			}
			if len(out) > 0 {
				s.write(w, out)
			}
			continue
		}
		if resp, ok := s.handle(ctx, json.RawMessage(line)); ok {
			s.write(w, resp)
		}
	}
	return sc.Err()
}

func (s *Server) write(w io.Writer, v any) {
	b, _ := json.Marshal(v)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = w.Write(append(b, '\n'))
}

// Handle 处理一条消息；通知（没有 id）不需要回应，返回 ok=false。
func (s *Server) Handle(ctx context.Context, msg []byte) (any, bool) {
	return s.handle(ctx, msg)
}

func (s *Server) handle(ctx context.Context, msg json.RawMessage) (response, bool) {
	var req request
	if err := json.Unmarshal(msg, &req); err != nil {
		return response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParse, "parse error"}}, true
	}
	notification := len(req.ID) == 0 || string(req.ID) == "null"
	if req.Method == "" {
		if notification {
			return response{}, false // 客户端发来的响应（本服务器不发请求），忽略
		}
		return response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{codeInvalidRequest, "missing method"}}, true
	}
	result, rerr := s.dispatch(ctx, req)
	if notification {
		return response{}, false
	}
	resp := response{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	return resp, true
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := ProtocolVersion
		if p.ProtocolVersion != "" {
			v = p.ProtocolVersion
		}
		name := s.Name
		if name == "" {
			name = "ibukirpg"
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": name, "version": s.Version},
			"instructions":    s.instructions(),
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "notifications/initialized", "notifications/cancelled", "initialized":
		return nil, nil
	case "tools/list":
		var list []map[string]any
		for _, t := range s.worldToolsVisible() {
			list = append(list, map[string]any{
				"name": t.name, "title": t.title, "description": t.desc, "inputSchema": t.schema,
				"annotations": map[string]any{"readOnlyHint": !t.write, "destructiveHint": t.write, "idempotentHint": !t.write, "openWorldHint": false},
			})
		}
		for _, t := range tools.All() {
			if !t.Allowed(s.Scope) {
				continue
			}
			list = append(list, map[string]any{
				"name":        t.FuncName(),
				"title":       t.Name,
				"description": t.Description,
				"inputSchema": t.Params,
				"annotations": map[string]any{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false},
			})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{codeInvalidParams, "tools/call 需要 name"}
		}
		if wt, ok := s.findWorldTool(p.Name); ok {
			out, err := wt.call(ctx, s.World, p.Arguments)
			if err != nil {
				return toolResult(errJSON(err.Error()), true), nil
			}
			b, _ := json.Marshal(out)
			return toolResult(string(b), false), nil
		}
		if _, ok := tools.Find(p.Name); !ok {
			return nil, &rpcError{codeInvalidParams, "未知工具：" + p.Name}
		}
		env, err := s.env(ctx)
		if err != nil {
			return toolResult(errJSON(err.Error()), true), nil
		}
		args := string(p.Arguments)
		if args == "null" {
			args = ""
		}
		out, cerr := tools.Call(env, s.Scope, p.Name, args)
		return toolResult(out, cerr != nil), nil
	case "resources/list":
		return map[string]any{"resources": []any{}}, nil
	case "prompts/list":
		return map[string]any{"prompts": []any{}}, nil
	}
	return nil, &rpcError{codeMethodNotFound, fmt.Sprintf("method not found: %s", req.Method)}
}

func (s *Server) env(ctx context.Context) (*tools.Env, error) {
	if s.Env == nil {
		return nil, errors.New("没有载入存档")
	}
	return s.Env(ctx)
}

func toolResult(text string, isErr bool) map[string]any {
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": isErr}
}

func errJSON(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}

func (s *Server) instructions() string {
	t := "ibukiRPG 故事包与存档的检索工具（范围：" + s.Scope.String() + "）。" +
		"先用 pack_search 找到实体 ID，再用 pack_get_entity / character_get_card / mech_get_card 查看详情；" +
		"memory_search 检索记忆与对话摘要；story_get_state 查看当前进度。"
	if s.World != nil {
		t += "world_change_log / timeline_get / knowledge_get 查看世界变更、时间线与玩家知识。"
	}
	if s.writable() {
		return t + "写入已开启：先 world_preview_change 预览，再用返回的 preview_token 调 world_apply_change 提交；所有写入都记录为“外部工具”来源，可撤销。"
	}
	return t + "所有工具都不会修改存档。"
}
