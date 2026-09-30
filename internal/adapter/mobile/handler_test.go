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
