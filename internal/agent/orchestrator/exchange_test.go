package orchestrator_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// rezip 改写 .ibksave 中的某个文件（模拟旧版本 / 被篡改的存档）。
func rezip(t *testing.T, b []byte, name string, edit func([]byte) []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, _ := f.Open()
		raw, _ := io.ReadAll(rc)
		_ = rc.Close()
		if f.Name == name {
			raw = edit(raw)
		}
		w, _ := zw.Create(f.Name)
		_, _ = w.Write(raw)
	}
	_ = zw.Close()
	return buf.Bytes()
}

// TestSaveExportImport：导出 → 导入为新存档 → 状态一致（含分支与检查点）；不含密钥；旧版本 / 篡改被拒绝。
func TestSaveExportImport(t *testing.T) {
	ctx := context.Background()
	f := &fakeLLM{}
	s := onlineBrass(t, f)
	look(t, s)
	if _, err := s.CreateCheckpoint(ctx, "手动", 0); err != nil {
		t.Fatal(err)
	}
	look(t, s)
	before, _ := s.Scene(ctx)
	data, name, err := s.ExportSave(ctx, "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(name, ".ibksave") {
		t.Fatalf("file name %q", name)
	}
	if bytes.Contains(data, []byte("sk-test")) {
		t.Fatal("export must not contain API keys")
	}
	chk, err := s.InspectSave(data)
	if err != nil || !chk.OK || chk.Manifest.Events == 0 || len(chk.Manifest.Branches) != 1 {
		t.Fatalf("inspect %+v %v", chk, err)
	}
	id, _, err := s.ImportSave(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.LoadGame(ctx, id); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Scene(ctx)
	if after.Turn != before.Turn || after.LocationID != before.LocationID || !strings.HasSuffix(after.SaveName, "（导入）") {
		t.Fatalf("imported scene differs: %+v vs %+v", after.Turn, before.Turn)
	}
	tl, _ := s.Timeline(ctx)
	if len(tl.Checkpoints) != 1 {
		t.Fatalf("checkpoints not imported: %+v", tl.Checkpoints)
	}
	look(t, s) // 导入的存档可以继续玩

	// 0.1.x 存档
	old := rezip(t, data, "manifest.json", func(b []byte) []byte {
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		m["engine_version"], m["version"] = "0.1.2", 1
		out, _ := json.Marshal(m)
		return out
	})
	if _, _, err := s.ImportSave(ctx, old); err == nil || !strings.Contains(err.Error(), "v0.1.x") {
		t.Fatalf("legacy save must be rejected: %v", err)
	}
	// 被塞进密钥的存档
	leaked := rezip(t, data, "save.json", func(b []byte) []byte {
		return bytes.Replace(b, []byte(`"slot":{`), []byte(`"slot":{"api_key":"sk-abcdefghijklmnopqrstuvwx",`), 1)
	})
	if chk, _ := s.InspectSave(leaked); chk.OK {
		t.Fatal("save with an API key must be rejected")
	}
	if _, err := s.InspectSave([]byte("not a zip")); err == nil {
		t.Fatal("garbage must be rejected")
	}
}
