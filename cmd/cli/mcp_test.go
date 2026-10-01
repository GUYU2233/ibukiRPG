package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/adapter/mcp"
	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/packages"
)

// `ibukirpg mcp`：对真实存档跑一轮 stdio MCP 会话（initialize → tools/list → tools/call），
// 只暴露只读工具，NPC 范围读不到别人的秘密，存档不被修改。
func TestMCPStdio(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { _ = adapter.Close() })
	var out bytes.Buffer
	if err := run(strings.NewReader("环顾四周\n/quit\n"), &out, options{dataDir: dir, provider: "offline", newGame: true, player: "阿澈", seed: 7}); err != nil {
		t.Fatal(err)
	}
	_ = adapter.Close()
	msgs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"pack_get_entity","arguments":{"id":"demo:character/mira"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"world_get_location","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"story_get_state","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"submit_text","params":{"text":"砸烂酒馆"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"submit_text","arguments":{"text":"砸烂酒馆"}}}`,
		`not json`,
	}
	var stdout, stderr bytes.Buffer
	err := runMCP([]string{"--save", dir, "--scope", "npc:demo:character/lena"}, strings.NewReader(strings.Join(msgs, "\n")+"\n"), &stdout, &stderr)
	if err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 8 { // 通知没有回应
		t.Fatalf("got %d responses:\n%s", len(lines), stdout.String())
	}
	resp := make([]map[string]any, len(lines))
	for i, l := range lines {
		if err := json.Unmarshal([]byte(l), &resp[i]); err != nil {
			t.Fatalf("line %d not JSON: %s", i, l)
		}
	}
	init := resp[0]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-03-26" || init["capabilities"].(map[string]any)["tools"] == nil {
		t.Fatalf("initialize: %v", init)
	}
	list := resp[1]["result"].(map[string]any)["tools"].([]any)
	if len(list) < 8 {
		t.Fatalf("tools/list: %d tools", len(list))
	}
	for _, x := range list {
		m := x.(map[string]any)
		name := m["name"].(string)
		if strings.Contains(name, "submit") || strings.Contains(name, "save") || strings.Contains(name, "quick") {
			t.Errorf("write tool exposed: %s", name)
		}
		if m["annotations"].(map[string]any)["readOnlyHint"] != true {
			t.Errorf("%s not marked read-only", name)
		}
	}
	text := resp[2]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "米拉") || strings.Contains(text, "家族的私生女") {
		t.Fatalf("npc-scoped get_entity: %s", text)
	}
	state := resp[3]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(state, "锈酒杯酒馆") {
		t.Fatalf("world_get_location: %s", state)
	}
	// NPC 范围不能看剧情进度（工具返回 isError）
	if resp[4]["result"].(map[string]any)["isError"] != true {
		t.Fatalf("npc story_get_state should be refused: %v", resp[4])
	}
	if resp[5]["error"] == nil || resp[6]["error"] == nil || resp[7]["error"] == nil {
		t.Fatalf("write methods / unknown tools / garbage must error: %v | %v | %v", resp[5], resp[6], resp[7])
	}
}

// MCP 写入门控：只有 --allow-write + director/author 范围才列出写工具；预览 → 提交走同一网关；
// 游戏正在使用存档时写入被拒绝。
func TestMCPWriteGate(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { _ = adapter.Close() })
	var out bytes.Buffer
	if err := run(strings.NewReader("环顾四周\n/quit\n"), &out, options{dataDir: dir, provider: "offline", newGame: true, player: "阿澈", seed: 7}); err != nil {
		t.Fatal(err)
	}
	_ = adapter.Close()
	list := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	names := func(args ...string) string {
		var stdout, stderr bytes.Buffer
		if err := runMCP(append([]string{"--save", dir}, args...), strings.NewReader(list+"\n"), &stdout, &stderr); err != nil {
			t.Fatalf("%v\n%s", err, stderr.String())
		}
		return stdout.String()
	}
	if s := names("--scope", "player", "--allow-write"); strings.Contains(s, "world_apply_change") || !strings.Contains(s, "world_change_log") {
		t.Fatalf("player scope must stay read-only: %s", s)
	}
	if s := names("--scope", "director"); strings.Contains(s, "world_apply_change") {
		t.Fatalf("write tools without --allow-write: %s", s)
	}
	if s := names("--scope", "author", "--allow-write"); !strings.Contains(s, "world_apply_change") || !strings.Contains(s, "world_preview_change") {
		t.Fatalf("author + --allow-write should list write tools: %s", s)
	}

	ctx := context.Background()
	db := filepath.Join(dir, adapter.DBFileName)
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: db, Packs: packages.Builtin(), NoSlotLock: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	srv := &mcp.Server{Scope: tools.Director(), World: &mcpWorld{s: s}, AllowWrite: true}
	call := func(name, args string) (string, bool) {
		r, _ := srv.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`))
		b, _ := json.Marshal(r)
		var m struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
		}
		_ = json.Unmarshal(b, &m)
		return m.Result.Content[0].Text, m.Result.IsError
	}
	text, isErr := call("world_preview_change", `{"changes":[{"op":"patch","target":"demo:location/rusty_tankard","path":"fields.description","value":"酒馆墙上多了一幅画。","reason":"作者调试"}]}`)
	if isErr {
		t.Fatalf("preview: %s", text)
	}
	var pv struct {
		Token string `json:"preview_token"`
	}
	_ = json.Unmarshal([]byte(text), &pv)
	if pv.Token == "" {
		t.Fatalf("no token: %s", text)
	}
	// 游戏会话打开同一存档 → 写入被拒绝
	game, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: db, Packs: packages.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	if err := game.LoadGame(ctx, s.SlotID()); err != nil {
		t.Fatal(err)
	}
	if text, isErr := call("world_apply_change", `{"preview_token":"`+pv.Token+`"}`); !isErr || !strings.Contains(text, "正在被游戏使用") {
		t.Fatalf("apply while game running: %s", text)
	}
	_ = game.Close()
	if text, isErr := call("world_apply_change", `{"preview_token":"`+pv.Token+`"}`); isErr {
		t.Fatalf("apply: %s", text)
	}
	text, _ = call("world_change_log", `{}`)
	if !strings.Contains(text, "外部工具") {
		t.Fatalf("change log should mark external tool source: %s", text)
	}
}
