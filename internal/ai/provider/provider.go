package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Message 是一条对话消息。ToolCalls / ToolCallID 用于 OpenAI 兼容的函数调用（没有时不会序列化，
// 因此不影响旧录音的请求哈希）。
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall 是模型发起的一次函数调用。
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // 固定为 function
	Function FunctionCall `json:"function"`
}

// FunctionCall 是函数名与 JSON 参数。
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolSpec 是提供给模型的函数声明（OpenAI tools 格式）。
type ToolSpec struct {
	Type     string       `json:"type"` // function
	Function FunctionSpec `json:"function"`
}

// FunctionSpec 是函数名、说明与 JSON Schema 参数。
type FunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Request 是一次模型调用。
type Request struct {
	Messages    []Message
	Temperature float64
	MaxTokens   int
	JSON        bool // 要求 JSON 对象输出（response_format=json_object）
	// Tools 非空时启用函数调用（只读检索工具）。
	Tools []ToolSpec
}

// Response 是模型输出与统计。
type Response struct {
	Text             string
	Model            string
	PromptTokens     int
	CompletionTokens int
	Latency          time.Duration
	FirstToken       time.Duration
	// ToolCalls 是模型要求执行的函数调用（为空表示给出了最终回答）。
	ToolCalls []ToolCall
}

// ToolCaller 由支持函数调用的 Provider 实现。本地小模型 / 离线模式不实现，调用方改用关键词预检索。
type ToolCaller interface {
	SupportsTools() bool
}

// ErrToolsUnsupported 表示服务端拒绝了 tools 参数（调用方应降级为预检索）。
var ErrToolsUnsupported = errors.New("provider does not support tool calling")

// Provider 是 AI Provider 接口（第 36 节）。实现必须可取消、可超时，失败不影响 Game State。
type Provider interface {
	Name() string
	Generate(ctx context.Context, req Request) (Response, error)
	Stream(ctx context.Context, req Request, onDelta func(string)) (Response, error)
}

// Provider 种类。
const (
	KindOffline  = "offline"
	KindDeepSeek = "deepseek"
	KindQwen     = "qwen"
	KindCustom   = "custom"
	// KindLocal 是设备本地模型（Android 端通过本地 HTTP 服务暴露），不支持函数调用。
	KindLocal = "local"
	// KindLlamaCpp 是 Android 端 llama.cpp 本地 GGUF 模型（同样不支持函数调用，改用关键词预检索）。
	KindLlamaCpp = "llamacpp"
	// KindMediaPipe 是 0.1.1 的 MediaPipe 本地模型（已移除；保留常量以兼容旧配置）。
	KindMediaPipe = "mediapipe"
)

// IsLocalKind 报告 kind 是否为设备本地模型（不支持函数调用）。
func IsLocalKind(kind string) bool {
	return kind == KindLocal || kind == KindLlamaCpp || kind == KindMediaPipe
}

// Preset 是内置的 OpenAI 兼容服务预设。
type Preset struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// Presets 返回内置预设（UI 用）。
func Presets() []Preset {
	return []Preset{
		{Kind: KindOffline, Label: "离线模式（无需联网）"},
		{Kind: KindDeepSeek, Label: "DeepSeek", BaseURL: "https://api.deepseek.com", Model: "deepseek-chat"},
		{Kind: KindQwen, Label: "通义千问（DashScope 兼容模式）", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Model: "qwen-plus"},
		{Kind: KindCustom, Label: "自定义 OpenAI 兼容服务"},
	}
}

// Config 是运行时 Provider 配置。密钥只保存在内存中（Android 端用 Keystore 加密持久化）。
type Config struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key,omitempty"`
	// NoTools 关闭函数调用（服务端不支持时由用户或自动探测设置）。
	NoTools bool `json:"no_tools,omitempty"`
}

// Normalize 补全预设的默认 BaseURL / Model。
func (c Config) Normalize() Config {
	if c.Kind == "" {
		c.Kind = KindOffline
	}
	for _, p := range Presets() {
		if p.Kind == c.Kind {
			if strings.TrimSpace(c.BaseURL) == "" {
				c.BaseURL = p.BaseURL
			}
			if strings.TrimSpace(c.Model) == "" {
				c.Model = p.Model
			}
		}
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.Model = strings.TrimSpace(c.Model)
	c.APIKey = strings.TrimSpace(c.APIKey)
	return c
}

// Online 报告配置是否可以联网调用。
func (c Config) Online() bool {
	c = c.Normalize()
	return c.Kind != KindOffline && c.BaseURL != "" && c.Model != "" && c.APIKey != ""
}

// OpenAICompatible 是 OpenAI 兼容 Chat Completions Provider（DeepSeek、DashScope 兼容模式等）。
type OpenAICompatible struct {
	cfg    Config
	client *http.Client
}

// NewOpenAICompatible 创建 Provider。transport 为 nil 时使用默认传输；测试时注入录制传输。
func NewOpenAICompatible(cfg Config, transport http.RoundTripper) *OpenAICompatible {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &OpenAICompatible{cfg: cfg.Normalize(), client: &http.Client{Transport: transport}}
}

// Name 返回 Provider 名。
func (p *OpenAICompatible) Name() string { return p.cfg.Kind + ":" + p.cfg.Model }

// SupportsTools 报告是否尝试函数调用。设备本地模型（local / llamacpp）不支持；其它 OpenAI 兼容服务先尝试，
// 被服务端拒绝时返回 ErrToolsUnsupported。
func (p *OpenAICompatible) SupportsTools() bool {
	return !IsLocalKind(p.cfg.Kind) && !p.cfg.NoTools
}

type chatRequest struct {
	Model          string            `json:"model"`
	Messages       []Message         `json:"messages"`
	Temperature    float64           `json:"temperature"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Stream         bool              `json:"stream"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
	Tools          []ToolSpec        `json:"tools,omitempty"`
	ToolChoice     string            `json:"tool_choice,omitempty"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// HTTPError 是非 2xx 响应。
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	switch e.Status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Sprintf("鉴权失败（HTTP %d），请检查 API Key：%s", e.Status, e.Message)
	case http.StatusTooManyRequests:
		return fmt.Sprintf("请求过于频繁或额度不足（HTTP 429）：%s", e.Message)
	case http.StatusNotFound:
		return fmt.Sprintf("接口或模型不存在（HTTP 404），请检查 Base URL 与模型名：%s", e.Message)
	}
	return fmt.Sprintf("HTTP %d：%s", e.Status, e.Message)
}

func (p *OpenAICompatible) newRequest(ctx context.Context, req Request, stream bool) (*http.Request, error) {
	if !p.cfg.Online() {
		return nil, errors.New("AI 服务未配置（缺少 Base URL、模型或 API Key）")
	}
	body := chatRequest{Model: p.cfg.Model, Messages: req.Messages, Temperature: req.Temperature, MaxTokens: req.MaxTokens, Stream: stream}
	if req.JSON {
		body.ResponseFormat = map[string]string{"type": "json_object"}
	}
	if len(req.Tools) > 0 && !stream {
		body.Tools, body.ToolChoice = req.Tools, "auto"
		body.ResponseFormat = nil // 部分服务不允许 tools 与 json_object 同时出现
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	if stream {
		hr.Header.Set("Accept", "text/event-stream")
	}
	return hr, nil
}

func readError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var cr chatResponse
	msg := strings.TrimSpace(string(b))
	if json.Unmarshal(b, &cr) == nil && cr.Error != nil && cr.Error.Message != "" {
		msg = cr.Error.Message
	}
	if len([]rune(msg)) > 200 {
		msg = string([]rune(msg)[:200])
	}
	return &HTTPError{Status: resp.StatusCode, Message: msg}
}

// toolsRejected 判断错误是否是服务端不支持 tools 参数（400 / 404 / 422 且提到 tool / function）。
func toolsRejected(err error) bool {
	var he *HTTPError
	if !errors.As(err, &he) {
		return false
	}
	if he.Status != http.StatusBadRequest && he.Status != http.StatusNotFound && he.Status != http.StatusUnprocessableEntity {
		return false
	}
	m := strings.ToLower(he.Message)
	return strings.Contains(m, "tool") || strings.Contains(m, "function")
}

// Generate 非流式调用。
func (p *OpenAICompatible) Generate(ctx context.Context, req Request) (Response, error) {
	start := time.Now()
	hr, err := p.newRequest(ctx, req, false)
	if err != nil {
		return Response{}, err
	}
	resp, err := p.client.Do(hr)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		err := readError(resp)
		if len(req.Tools) > 0 && toolsRejected(err) {
			return Response{}, fmt.Errorf("%w: %w", ErrToolsUnsupported, err)
		}
		return Response{}, err
	}
	var cr chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cr); err != nil {
		return Response{}, fmt.Errorf("decode response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return Response{}, errors.New("empty choices")
	}
	out := Response{Text: cr.Choices[0].Message.Content, Model: cr.Model, Latency: time.Since(start), ToolCalls: cr.Choices[0].Message.ToolCalls}
	out.FirstToken = out.Latency
	if cr.Usage != nil {
		out.PromptTokens, out.CompletionTokens = cr.Usage.PromptTokens, cr.Usage.CompletionTokens
	}
	return out, nil
}

// Stream 流式调用（SSE），每个增量回调 onDelta。
func (p *OpenAICompatible) Stream(ctx context.Context, req Request, onDelta func(string)) (Response, error) {
	start := time.Now()
	hr, err := p.newRequest(ctx, req, true)
	if err != nil {
		return Response{}, err
	}
	resp, err := p.client.Do(hr)
	if err != nil {
		return Response{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		return Response{}, readError(resp)
	}
	var out Response
	var sb strings.Builder
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var cr chatResponse
		if err := json.Unmarshal([]byte(data), &cr); err != nil {
			continue
		}
		if cr.Model != "" {
			out.Model = cr.Model
		}
		if cr.Usage != nil {
			out.PromptTokens, out.CompletionTokens = cr.Usage.PromptTokens, cr.Usage.CompletionTokens
		}
		for _, ch := range cr.Choices {
			if d := ch.Delta.Content; d != "" {
				if sb.Len() == 0 {
					out.FirstToken = time.Since(start)
				}
				sb.WriteString(d)
				if onDelta != nil {
					onDelta(d)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Response{}, err
	}
	out.Text = sb.String()
	out.Latency = time.Since(start)
	if out.Text == "" {
		return out, errors.New("empty stream")
	}
	return out, nil
}

// Ping 发一个极小请求验证配置（设置页“测试连接”）。
func Ping(ctx context.Context, p Provider) (Response, error) {
	return p.Generate(ctx, Request{
		Messages:  []Message{{Role: "user", Content: "请只回复“连接成功”四个字。"}},
		MaxTokens: 10,
	})
}
