// Command uifixtures 用真实引擎（离线模式）跑一段试玩，导出 Android 截图测试所需的 JSON 夹具。
//
//	go run ./cmd/uifixtures -out android/app/src/test/resources/fixtures
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
)

func call(typ, cmdID string, payload any) json.RawMessage {
	req := map[string]any{"version": "v1", "type": typ}
	if cmdID != "" {
		req["command_id"] = cmdID
	}
	if payload != nil {
		req["payload"] = payload
	}
	b, _ := json.Marshal(req)
	var resp struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(adapter.Handle(context.Background(), string(b))), &resp); err != nil || !resp.OK {
		fmt.Fprintf(os.Stderr, "%s failed: %v %s\n", typ, err, resp.Error)
		os.Exit(1)
	}
	return resp.Data
}

func main() {
	out := flag.String("out", "android/app/src/test/resources/fixtures", "输出目录")
	seed := flag.Uint64("seed", 3, "世界种子")
	flag.Parse()
	dir, err := os.MkdirTemp("", "ibuki-fixtures")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	call("init", "", map[string]string{"data_dir": dir})
	// 另一个故事包的存档（存档列表 / 故事包卡片上的存档数）
	call("new_game", "", map[string]any{"player_name": "小雾", "seed": *seed, "pack_id": "fog_lighthouse"})
	call("submit_text", "fixture-fog", map[string]string{"text": "和姑娘打个招呼"})
	call("new_game", "", map[string]any{"player_name": "阿澈", "seed": *seed})
	inputs := []string{"和老板打个招呼", "来一杯麦酒", "和伯林聊聊", "仔细搜查奥托的座位附近", "说服伯林让我去储藏室看看"}
	for i, in := range inputs {
		call("submit_text", fmt.Sprintf("fixture-%d", i+1), map[string]string{"text": in})
	}
	files := map[string]json.RawMessage{
		"game.json":      call("get_bundle", "", nil),
		"character.json": call("get_character", "", nil),
		"inventory.json": call("get_inventory", "", nil),
		"npcs.json":      call("get_npcs", "", nil),
		"journal.json":   call("get_journal", "", nil),
		"saves.json":     call("list_saves", "", nil),
		"presets.json":   call("presets", "", nil),
		"packs.json":     call("list_packs", "", nil),
	}
	if err := os.MkdirAll(*out, 0o750); err != nil {
		panic(err)
	}
	for name, data := range files {
		var v any
		_ = json.Unmarshal(data, &v)
		pretty, _ := json.MarshalIndent(v, "", "  ")
		if err := os.WriteFile(filepath.Join(*out, name), pretty, 0o600); err != nil {
			panic(err)
		}
	}
	fmt.Println("fixtures written to", *out)
}
