package mobile

import (
	"context"
	"encoding/json"
	"testing"
)

func call(t *testing.T, req string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(Handle(context.Background(), req)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestHandle(t *testing.T) {
	if r := call(t, `{"version":"v1","type":"ping","command_id":"c1"}`); r["ok"] != true || r["command_id"] != "c1" {
		t.Fatalf("ping: %v", r)
	}
	r := call(t, `{"version":"v1","type":"sqlite_smoke"}`)
	if r["ok"] != true {
		t.Fatalf("sqlite_smoke: %v", r)
	}
	if data := r["data"].(map[string]any); data["events"] != float64(2) {
		t.Fatalf("events: %v", data)
	}
	a := call(t, `{"version":"v1","type":"roll","payload":{"seed":1,"entity":"e","purpose":"p"}}`)
	b := call(t, `{"version":"v1","type":"roll","payload":{"seed":1,"entity":"e","purpose":"p"}}`)
	if a["data"].(map[string]any)["roll"] != b["data"].(map[string]any)["roll"] {
		t.Fatal("roll should be deterministic")
	}
	for _, bad := range []string{`not json`, `{"version":"v9","type":"ping"}`, `{"version":"v1","type":"nope"}`} {
		if r := call(t, bad); r["ok"] != false || r["error"] == "" {
			t.Errorf("%s: expected error, got %v", bad, r)
		}
	}
}

func TestGameAPI(t *testing.T) {
	dir := t.TempDir()
	var events []string
	SetSink(func(s string) { events = append(events, s) })
	defer SetSink(nil)
	if r := call(t, `{"version":"v1","type":"list_saves"}`); r["ok"] != false {
		t.Fatal("list_saves before init should fail")
	}
	b, _ := json.Marshal(map[string]any{"version": "v1", "type": "init", "payload": map[string]string{"data_dir": dir}})
	if r := call(t, string(b)); r["ok"] != true {
		t.Fatalf("init: %v", r)
	}
	r := call(t, `{"version":"v1","type":"new_game","payload":{"player_name":"小林","seed":5}}`)
	if r["ok"] != true {
		t.Fatalf("new_game: %v", r)
	}
	data := r["data"].(map[string]any)
	if data["scene"].(map[string]any)["location_name"] != "锈酒杯酒馆" || len(data["transcript"].([]any)) != 1 {
		t.Fatalf("bundle: %v", data)
	}
	r = call(t, `{"version":"v1","type":"submit_text","command_id":"k1","payload":{"text":"环顾四周"}}`)
	if r["ok"] != true || r["data"].(map[string]any)["accepted"] != true {
		t.Fatalf("submit: %v", r)
	}
	r = call(t, `{"version":"v1","type":"quick_action","command_id":"k2","payload":{"kind":"action","action":"demo:action/order_drink","label":"点一杯麦酒"}}`)
	if r["ok"] != true {
		t.Fatalf("quick: %v", r)
	}
	for _, typ := range []string{"get_scene", "get_suggestions", "get_character", "get_inventory", "get_npcs", "get_journal", "get_transcript", "ai_status", "presets", "list_saves", "version"} {
		if r := call(t, `{"version":"v1","type":"`+typ+`"}`); r["ok"] != true {
			t.Errorf("%s: %v", typ, r)
		}
	}
	if len(events) == 0 {
		t.Fatal("no stream events")
	}
	r = call(t, `{"version":"v1","type":"test_ai","payload":{"kind":"deepseek"}}`)
	if r["data"].(map[string]any)["ok"] != false {
		t.Fatalf("test_ai without key should fail gracefully: %v", r)
	}
}
