package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func server(t *testing.T, h http.HandlerFunc) Config {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return Config{Kind: KindCustom, BaseURL: srv.URL + "/v1", Model: "m", APIKey: "k"}
}

func TestGenerateJSON(t *testing.T) {
	cfg := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("bad request %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if rf, _ := body["response_format"].(map[string]any); rf["type"] != "json_object" {
			t.Errorf("response_format missing: %v", body)
		}
		_, _ = fmt.Fprint(w, `{"model":"m","choices":[{"message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`)
	})
	resp, err := NewOpenAICompatible(cfg, nil).Generate(context.Background(), Request{Messages: []Message{{Role: "user", Content: "hi"}}, JSON: true})
	if err != nil || resp.Text != `{"ok":true}` || resp.PromptTokens != 3 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
}

func TestStreamSSE(t *testing.T) {
	cfg := server(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, d := range []string{"雨", "还在", "下。"} {
			_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", d)
		}
		_, _ = fmt.Fprint(w, ": keep-alive\n\ndata: [DONE]\n\n")
	})
	var got []string
	resp, err := NewOpenAICompatible(cfg, nil).Stream(context.Background(), Request{}, func(s string) { got = append(got, s) })
	if err != nil || resp.Text != "雨还在下。" || len(got) != 3 {
		t.Fatalf("resp=%+v deltas=%v err=%v", resp, got, err)
	}
}

func TestHTTPErrors(t *testing.T) {
	cfg := server(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, `{"error":{"message":"invalid api key"}}`)
	})
	_, err := NewOpenAICompatible(cfg, nil).Generate(context.Background(), Request{})
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 401 || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("err=%v", err)
	}
}

func TestTimeoutAndOffline(t *testing.T) {
	cfg := server(t, func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := NewOpenAICompatible(cfg, nil).Generate(ctx, Request{}); err == nil {
		t.Fatal("expected timeout error")
	}
	if _, err := NewOpenAICompatible(Config{Kind: KindOffline}, nil).Generate(context.Background(), Request{}); err == nil {
		t.Fatal("offline config must not call network")
	}
}

func TestPresetsNormalize(t *testing.T) {
	c := Config{Kind: KindQwen, APIKey: "x"}.Normalize()
	if !strings.Contains(c.BaseURL, "dashscope") || c.Model == "" || !c.Online() {
		t.Fatalf("qwen preset not applied: %+v", c)
	}
	d := Config{Kind: KindDeepSeek, APIKey: "x"}.Normalize()
	if d.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("deepseek base url %q", d.BaseURL)
	}
}

func TestLocalKindsSkipTools(t *testing.T) {
	for _, k := range []string{KindLocal, KindLlamaCpp, KindMediaPipe} {
		if !IsLocalKind(k) || NewOpenAICompatible(Config{Kind: k, BaseURL: "http://127.0.0.1:1", Model: "m"}, nil).SupportsTools() {
			t.Fatalf("%s should be local without tools", k)
		}
	}
	if IsLocalKind(KindDeepSeek) || !NewOpenAICompatible(Config{Kind: KindDeepSeek, BaseURL: "https://x", Model: "m"}, nil).SupportsTools() {
		t.Fatal("deepseek should try tools")
	}
}
