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

// Message 是一条对话消息。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request 是一次模型调用。
type Request struct {
	Messages    []Message
	Temperature float64
	MaxTokens   int
	JSON        bool // 要求 JSON 对象输出（response_format=json_object）
}

// Response 是模型输出与统计。
type Response struct {
	Text             string
	Model            string
	PromptTokens     int
	CompletionTokens int
	Latency          time.Duration
	FirstToken       time.Duration
}

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
)

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

type chatRequest struct {
	Model          string            `json:"model"`
	Messages       []Message         `json:"messages"`
	Temperature    float64           `json:"temperature"`
	MaxTokens      int               `json:"max_tokens,omitempty"`
	Stream         bool              `json:"stream"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
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
		return Response{}, readError(resp)
	}
	var cr chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&cr); err != nil {
		return Response{}, fmt.Errorf("decode response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return Response{}, errors.New("empty choices")
	}
	out := Response{Text: cr.Choices[0].Message.Content, Model: cr.Model, Latency: time.Since(start)}
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
