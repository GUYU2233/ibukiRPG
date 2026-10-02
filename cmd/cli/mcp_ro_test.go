package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/adapter/mcp"
	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/storage/sqlite"
	"github.com/GUYU2233/ibukiRPG/packages"
)

// dirDigest 返回存档目录里全部数据库文件（含 -wal / -shm 之外的主库与 WAL）的摘要。
func dirDigest(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	for _, suf := range []string{"", "-wal"} {
		b, err := os.ReadFile(filepath.Join(dir, adapter.DBFileName+suf)) // #nosec G304 -- 测试临时目录
		if err != nil || len(b) == 0 {
			continue // 只读连接可能创建空的 -wal；只比较内容
		}
		h.Write([]byte(suf))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// MCP 读工具不写存档：缺失的叙事只在内存里用模板补上；只读模式下写入会失败。
func TestMCPReadToolsDoNotWrite(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { _ = adapter.Close() })
	var out bytes.Buffer
	if err := run(strings.NewReader("环顾四周\n/quit\n"), &out, options{dataDir: dir, provider: "offline", newGame: true, player: "阿澈", seed: 7}); err != nil {
		t.Fatal(err)
	}
	_ = adapter.Close()
	ctx := context.Background()
	db := filepath.Join(dir, adapter.DBFileName)
	// 模拟“事件已提交、叙事未写入”的回合
	raw, err := sqlite.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	var cmdID string
	if err := raw.QueryRowContext(ctx, `SELECT command_id FROM commands WHERE narration IS NOT NULL ORDER BY rowid DESC LIMIT 1`).Scan(&cmdID); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE commands SET narration=NULL WHERE command_id=?`, `DELETE FROM transcript WHERE command_id=? AND kind='narration'`} {
		if _, err := raw.ExecContext(ctx, q, cmdID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := raw.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()
	before := dirDigest(t, dir)

	msgs := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"world_change_log","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"timeline_get","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"world_get_location","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"knowledge_get","arguments":{"entity":"demo:character/mira"}}}`,
	}
	for _, scope := range []string{"player", "director"} {
		var stdout, stderr bytes.Buffer
		if err := runMCP([]string{"--save", dir, "--scope", scope}, strings.NewReader(strings.Join(msgs, "\n")+"\n"), &stdout, &stderr); err != nil {
			t.Fatalf("%v\n%s", err, stderr.String())
		}
		if strings.Count(stdout.String(), `"isError":true`) > 0 {
			t.Fatalf("%s read tool error: %s", scope, stdout.String())
		}
	}
	if after := dirDigest(t, dir); after != before {
		es, _ := os.ReadDir(dir)
		for _, e := range es {
			fi, _ := e.Info()
			t.Logf("%s %d", e.Name(), fi.Size())
		}
		t.Fatal("MCP read tools modified the save database")
	}

	// 只读会话：叙事在内存里补上，存档不变；写入被拒绝
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: db, Packs: packages.Builtin(), NoSlotLock: true, ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.OpenForRead(ctx, ""); err != nil {
		t.Fatal(err)
	}
	tr, err := s.Transcript(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range tr {
		if e.Kind == "narration" && e.Source == "template(recovered)" {
			found = true
		}
	}
	if !found {
		t.Fatalf("recovered narration not merged in memory: %+v", tr)
	}
	srv := &mcp.Server{Scope: tools.Director(), World: &mcpWorld{s: s}, AllowWrite: true}
	r, _ := srv.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"checkpoint_create","arguments":{"name":"x"}}}`))
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), "只读") {
		t.Fatalf("write in read-only session should fail: %s", b)
	}
	_ = s.Close()
	if after := dirDigest(t, dir); after != before {
		t.Fatal("read-only session modified the save database")
	}

	// 正常载入（游戏）时才补写
	g, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: db, Packs: packages.Builtin(), NoSlotLock: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.OpenForWrite(ctx, ""); err != nil {
		t.Fatal(err)
	}
	_ = g.Close()
	if after := dirDigest(t, dir); after == before {
		t.Fatal("game load should repair the missing narration")
	}
}

// 外部工具写入后，下次打开游戏提示“外部工具修改了 N 项设定”，确认后不再提示。
func TestExternalChangeNotice(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { _ = adapter.Close() })
	var out bytes.Buffer
	if err := run(strings.NewReader("环顾四周\n/quit\n"), &out, options{dataDir: dir, provider: "offline", newGame: true, player: "阿澈", seed: 7}); err != nil {
		t.Fatal(err)
	}
	_ = adapter.Close()
	ctx := context.Background()
	db := filepath.Join(dir, adapter.DBFileName)
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: db, Packs: packages.Builtin(), NoSlotLock: true})
	if err != nil {
		t.Fatal(err)
	}
	srv := &mcp.Server{Scope: tools.Director(), World: &mcpWorld{s: s}, AllowWrite: true}
	call := func(name, args string) string {
		r, _ := srv.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"`+name+`","arguments":`+args+`}}`))
		b, _ := json.Marshal(r)
		var m struct {
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"result"`
		}
		_ = json.Unmarshal(b, &m)
		return m.Result.Content[0].Text
	}
	var pv struct {
		Token string `json:"preview_token"`
	}
	_ = json.Unmarshal([]byte(call("world_preview_change", `{"changes":[{"op":"patch","target":"demo:location/rusty_tankard","path":"fields.description","value":"酒馆墙上多了一幅画。","reason":"作者调试"},{"op":"patch","target":"demo:character/mira","path":"fields.appearance","value":"戴着一顶新草帽。","reason":"作者调试"}]}`)), &pv)
	if pv.Token == "" {
		t.Fatal("no preview token")
	}
	call("world_apply_change", `{"preview_token":"`+pv.Token+`","accept_impact":true}`)
	_ = s.Close()

	out.Reset()
	if err := run(strings.NewReader("/quit\n"), &out, options{dataDir: dir, provider: "offline"}); err != nil {
		t.Fatal(err)
	}
	_ = adapter.Close()
	if !strings.Contains(out.String(), "外部工具修改了 2 项设定") {
		t.Fatalf("missing external change notice:\n%s", out.String())
	}
	out.Reset()
	if err := run(strings.NewReader("/quit\n"), &out, options{dataDir: dir, provider: "offline"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "外部工具修改了") {
		t.Fatalf("notice should be acknowledged:\n%s", out.String())
	}
}
