package mobile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func req(typ string, payload any) string {
	b, _ := json.Marshal(map[string]any{"version": "v1", "type": typ, "payload": payload})
	return string(b)
}

// TestWorldAPI：0.2.0 新增请求（旧存档提示 / 角色创建 / 回溯 / 检查点 / 存档导出导入 / 世界面板 / 多服务商设置）。
func TestWorldAPI(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { _ = Close() })
	// 0.1.x 旧存档：init 给出提示，删除后消失
	if err := os.WriteFile(filepath.Join(dir, LegacyDBFileName), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := call(t, req("init", map[string]string{"data_dir": dir}))
	if r["ok"] != true || r["data"].(map[string]any)["legacy_notice"] == nil {
		t.Fatalf("init should report the legacy save: %v", r)
	}
	if r = call(t, req("delete_legacy_saves", map[string]string{"data_dir": dir})); r["ok"] != true {
		t.Fatalf("delete legacy: %v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, LegacyDBFileName)); !os.IsNotExist(err) {
		t.Fatal("legacy db not deleted")
	}
	r = call(t, req("get_creation", map[string]string{"pack_id": "brass_trial"}))
	if r["ok"] != true || len(r["data"].(map[string]any)["backgrounds"].([]any)) == 0 {
		t.Fatalf("get_creation: %v", r)
	}
	cr := map[string]any{"custom": true, "name": "小锤", "background": "scavenger"}
	if r = call(t, req("review_creation", map[string]any{"pack_id": "brass_trial", "creation": cr})); r["ok"] != true || r["data"].(map[string]any)["ok"] != true {
		t.Fatalf("review_creation: %v", r)
	}
	if r = call(t, req("new_game", map[string]any{"pack_id": "brass_trial", "seed": 3, "creation": cr})); r["ok"] != true {
		t.Fatalf("new_game with creation: %v", r)
	}
	// 多服务商配置（没有可用 key 时仍应成功并返回状态）
	ai := map[string]any{"providers": []map[string]any{{"id": "ds", "kind": "deepseek", "model": "deepseek-chat"}},
		"settings": map[string]any{"mode": "unified", "unified": map[string]string{"provider": "ds"}, "show_token_usage": true},
		"prompts":  map[string]any{"lore_deviation": map[string]string{"level": "high", "mode": "notify"}}}
	if r = call(t, req("configure_ai", ai)); r["ok"] != true {
		t.Fatalf("configure_ai: %v", r)
	}
	if r = call(t, req("configure_ai", map[string]any{"kind": "offline"})); r["ok"] != true {
		t.Fatalf("legacy configure_ai: %v", r)
	}
	if r = call(t, req("get_tasks", nil)); r["ok"] != true || len(r["data"].(map[string]any)["tasks"].([]any)) < 5 {
		t.Fatalf("get_tasks: %v", r)
	}
	for _, typ := range []string{"get_prompt_settings", "get_pending_decision", "search_world_changes", "get_timeline", "get_usage"} {
		if r := call(t, req(typ, nil)); r["ok"] != true {
			t.Errorf("%s: %v", typ, r)
		}
	}
	for _, tab := range []string{"characters", "relations", "codex", "equipment", "map", "factions", "timeline", "log"} {
		if r := call(t, req("get_world_panel", map[string]string{"tab": tab})); r["ok"] != true {
			t.Errorf("world panel %s: %v", tab, r)
		}
	}
	if r = call(t, req("wait", map[string]string{"target": "1h"})); r["ok"] != true {
		t.Fatalf("wait: %v", r)
	}
	if r = call(t, req("submit_text", map[string]string{"text": "环顾四周"})); r["ok"] != true {
		t.Fatalf("submit: %v", r)
	}
	r = call(t, req("create_checkpoint", map[string]any{"name": "测试点"}))
	if r["ok"] != true {
		t.Fatalf("checkpoint: %v", r)
	}
	cp := r["data"].(map[string]any)["id"].(string)
	if r = call(t, req("rollback_to", map[string]int{"turn": 1})); r["ok"] != true {
		t.Fatalf("rollback_to: %v", r)
	}
	if r = call(t, req("cancel_rollback", nil)); r["ok"] != true {
		t.Fatalf("cancel_rollback: %v", r)
	}
	if r = call(t, req("restore_checkpoint", map[string]string{"id": cp})); r["ok"] != true {
		t.Fatalf("restore_checkpoint: %v", r)
	}
	// 导出 → 检查 → 导入
	r = call(t, req("export_save", map[string]string{"dir": filepath.Join(dir, "exports")}))
	if r["ok"] != true {
		t.Fatalf("export_save: %v", r)
	}
	path := r["data"].(map[string]any)["path"].(string)
	if r = call(t, req("inspect_save", map[string]string{"path": path})); r["ok"] != true || r["data"].(map[string]any)["ok"] != true {
		t.Fatalf("inspect_save: %v", r)
	}
	if r = call(t, req("import_save", map[string]string{"path": path})); r["ok"] != true {
		t.Fatalf("import_save: %v", r)
	}
	if r = call(t, req("load_game", map[string]string{"slot_id": r["data"].(map[string]any)["slot_id"].(string)})); r["ok"] != true {
		t.Fatalf("load imported: %v", r)
	}
	if r = call(t, req("get_entity", map[string]string{"id": "brass:enemy/steam_golem"})); r["ok"] != false {
		t.Fatalf("unknown entity must be hidden: %v", r)
	}
}
