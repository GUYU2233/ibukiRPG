package transport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Recording 是一条录制的 HTTP 响应。
type Recording struct {
	Status  int    `json:"status"`
	Body    string `json:"body"`
	Comment string `json:"comment,omitempty"`
	// Request 仅用于人工审阅（不参与匹配）。
	Request json.RawMessage `json:"request,omitempty"`
}

// Recorded 是 Recorded LLM Transport（第 40 节）：Request Hash → Recorded Response。
// CI 不依赖真实 API；Prompt 变化会改变哈希，从而显式暴露需要重新录制的用例。
type Recorded struct {
	Dir string
	// Upstream 非 nil 时为录制模式：缺失的录音会真实请求并写入 Dir。
	Upstream http.RoundTripper
}

// Hash 计算请求哈希：方法 + 路径 + 请求体（Authorization 等头不参与，密钥不会进入录音）。
func Hash(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method + " " + path + "\n"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))[:24]
}

// RoundTrip 实现 http.RoundTripper。
func (r *Recorded) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		body = b
	}
	key := Hash(req.Method, trimVersion(req.URL.Path), body)
	path := filepath.Join(r.Dir, key+".json")
	if b, err := os.ReadFile(path); err == nil { //nolint:gosec // 测试录音目录
		var rec Recording
		if err := json.Unmarshal(b, &rec); err != nil {
			return nil, fmt.Errorf("recording %s: %w", key, err)
		}
		return response(req, rec), nil
	}
	if r.Upstream == nil {
		return nil, fmt.Errorf("no recording for request hash %s (dir %s)", key, r.Dir)
	}
	req2 := req.Clone(req.Context())
	req2.Body = io.NopCloser(bytes.NewReader(body))
	resp, err := r.Upstream.RoundTrip(req2)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	rec := Recording{Status: resp.StatusCode, Body: string(b), Request: json.RawMessage(body)}
	out, _ := json.MarshalIndent(rec, "", "  ")
	if err := os.MkdirAll(r.Dir, 0o750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return nil, err
	}
	return response(req, rec), nil
}

// 统一去掉 /v1 前缀差异，使 DeepSeek 与兼容模式的录音可以共享。
func trimVersion(p string) string {
	if i := strings.Index(p, "/chat/completions"); i >= 0 {
		return p[i:]
	}
	return p
}

func response(req *http.Request, rec Recording) *http.Response {
	ct := "application/json"
	if strings.HasPrefix(strings.TrimSpace(rec.Body), "data:") {
		ct = "text/event-stream"
	}
	return &http.Response{
		StatusCode: rec.Status,
		Status:     http.StatusText(rec.Status),
		Header:     http.Header{"Content-Type": []string{ct}},
		Body:       io.NopCloser(strings.NewReader(rec.Body)),
		Request:    req,
	}
}

// Save 手工写入一条录音（用于构造测试夹具）。
func Save(dir, key string, rec Recording) error {
	out, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, key+".json"), out, 0o600)
}

// Func 把函数适配为 RoundTripper（测试用，例如模拟超时 / 500）。
type Func func(*http.Request) (*http.Response, error)

// RoundTrip 实现 http.RoundTripper。
func (f Func) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
