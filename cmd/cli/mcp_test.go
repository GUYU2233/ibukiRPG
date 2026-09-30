package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
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
