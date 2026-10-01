package orchestrator_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/packages"
)

// auditLLM：审查请求返回脚本化的审查报告；其余请求返回不改世界的叙事，并记录是否带了 [CORRECTION]。
type auditLLM struct {
	mu          sync.Mutex
	report      string
	audits      int
	corrections int
}

func (f *auditLLM) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	if strings.Contains(string(body), "一致性审查员") {
		f.audits++
		return chat(f.report), nil
	}
	if strings.Contains(string(body), "[CORRECTION]") {
		f.corrections++
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(sse("你环顾四周。\n<<<WORLD>>>\n{\"v\":1}\n<<<END>>>"))), Header: http.Header{}}, nil
}

const auditReport = `{"findings":[
 {"kind":"omission","turn":1,"text":"叙事里工坊的炉子熄了，描述没变","fix":[{"op":"patch","target":"brass:location/workshop","path":"fields.description","value":"炉子已经熄了，工坊里只剩机油味。","reason":"补记：炉子熄了"}]},
 {"kind":"omission","turn":1,"text":"欧琳递给你一把扳手，背包里没有","fix":[{"op":"patch","target":"player","path":"inventory.brass:item/wrench","value":1,"reason":"补记：欧琳给的扳手"}]},
 {"kind":"hallucination","turn":1,"text":"叙事称欧琳是议会成员","fix":[],"note":"欧琳不是议会成员，她是工匠行会长"}
]}`

// TestAuditAutoFixAndSuggest：设定类修复自动提交（审查来源，可撤销）；机械状态修复只给建议，玩家确认后才提交；
// 幻觉类发现在下一次叙事里以 [CORRECTION] 注入。
func TestAuditAutoFixAndSuggest(t *testing.T) {
	ctx := context.Background()
	f := &auditLLM{report: auditReport}
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	look(t, s)
	before, _ := s.State()
	rep, err := s.RunAudit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 3 || rep.Fixed == nil || len(rep.Fixed.Changes) != 1 || rep.Fixed.Changes[0].Source != change.SourceAudit {
		t.Fatalf("report %+v fixed %+v", rep, rep.Fixed)
	}
	if len(rep.Suggestions) != 1 || rep.Suggestions[0].Status != "pending" {
		t.Fatalf("suggestions %+v", rep.Suggestions)
	}
	after, _ := s.State()
	if after.Player.Inventory["brass:item/wrench"] != before.Player.Inventory["brass:item/wrench"] {
		t.Fatal("mechanical fix must not be applied without confirmation")
	}
	// 叙事流里有审查卡片
	tr, _ := s.Transcript(ctx, 50, 0)
	if !hasKind(tr, "audit") {
		t.Fatalf("no audit entry: %s", kindsOf(tr))
	}
	// 应用建议 → 背包 +1，状态变为 applied；重复应用被拒绝
	if _, err := s.ResolveAuditSuggestion(ctx, rep.Suggestions[0].ID, "apply"); err != nil {
		t.Fatal(err)
	}
	after, _ = s.State()
	if after.Player.Inventory["brass:item/wrench"] != before.Player.Inventory["brass:item/wrench"]+1 {
		t.Fatalf("suggestion not applied: %v", after.Player.Inventory)
	}
	tr, _ = s.Transcript(ctx, 50, 0)
	for _, e := range tr {
		if e.Audit != nil && e.Audit.Suggestions[0].Status != "applied" {
			t.Fatalf("status %q", e.Audit.Suggestions[0].Status)
		}
	}
	if _, err := s.ResolveAuditSuggestion(ctx, rep.Suggestions[0].ID, "apply"); err == nil {
		t.Fatal("applying twice should fail")
	}
	// 自动修复可以单项撤销
	if _, err := s.RevertChange(ctx, rep.Fixed.Changes[0].ID); err != nil {
		t.Fatal(err)
	}
	// 下一回合的叙事提示词带 [CORRECTION]，之后不再重复
	look(t, s)
	look(t, s)
	if f.corrections != 1 {
		t.Fatalf("corrections injected %d times", f.corrections)
	}
}

// TestPeriodicAudit：每 N 回合在回合结束后自动审查；关闭后不再运行。
func TestPeriodicAudit(t *testing.T) {
	ctx := context.Background()
	f := &auditLLM{report: `{"findings":[]}`}
	s, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: filepath.Join(t.TempDir(), "a.db"), Packs: packages.Builtin(), Transport: f, AuditSync: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}, AuditEvery: 5})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		look(t, s)
	}
	if f.audits != 2 {
		t.Fatalf("audits = %d, want 2 (every 5 turns)", f.audits)
	}
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}, AuditEvery: -1})
	for range 6 {
		look(t, s)
	}
	if f.audits != 2 {
		t.Fatalf("audit ran while disabled: %d", f.audits)
	}
}

// TestPreviewApply：玩家明确要求的修改先预览再确认；头部变化后令牌失效；MCP 写入受影响分上限约束。
func TestPreviewApply(t *testing.T) {
	ctx := context.Background()
	s := openPacks(t, nil)
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	patch := []change.Change{{Op: change.OpPatch, Target: "brass:location/workshop", Path: "fields.description", Value: []byte(`"墙上新刷了一层绿漆。"`), Reason: "玩家要求"}}
	pv, err := s.PreviewChanges(ctx, change.SourceUserRequest, patch)
	if err != nil {
		t.Fatal(err)
	}
	if pv.Token == "" || len(pv.Changes) != 1 || pv.Changes[0].After == "" {
		t.Fatalf("preview %+v", pv)
	}
	// 预览不留任何记录
	if chs, _ := s.WorldChanges("", "", 0); len(chs) != 0 {
		t.Fatalf("preview committed: %+v", chs)
	}
	if _, err := s.ApplyPreview(ctx, pv.Token, 0); err != nil {
		t.Fatal(err)
	}
	chs, _ := s.WorldChanges("", "", 0)
	if len(chs) != 1 || chs[0].Source != change.SourceUserRequest {
		t.Fatalf("log %+v", chs)
	}
	if _, err := s.ApplyPreview(ctx, pv.Token, 0); err == nil {
		t.Fatal("token reuse should fail")
	}
	// 头部变化 → 需要重新预览
	pv, _ = s.PreviewChanges(ctx, change.SourceUserRequest, []change.Change{{Op: change.OpPatch, Target: "brass:location/workshop", Path: "fields.description", Value: []byte(`"又刷回了白色。"`), Reason: "玩家要求"}})
	look(t, s)
	if _, err := s.ApplyPreview(ctx, pv.Token, 0); !errors.Is(err, orchestrator.ErrPreviewStale) {
		t.Fatalf("stale apply err = %v", err)
	}
	// 重要人物死亡：影响分高，MCP 不带 accept_impact 被拒绝
	pv, err = s.PreviewChanges(ctx, "mcp:test", []change.Change{{Op: change.OpRetire, Target: "brass:character/orin", Value: []byte(`"death"`), Reason: "测试"}})
	if err != nil {
		t.Fatal(err)
	}
	if pv.Token == "" {
		t.Fatalf("retire preview %+v", pv)
	}
	if pv.Impact <= orchestrator.MCPImpactLimit {
		t.Fatalf("impact %d should exceed limit", pv.Impact)
	}
	if _, err := s.ApplyPreview(ctx, pv.Token, orchestrator.MCPImpactLimit); err == nil {
		t.Fatal("high-impact MCP write should be rejected without accept_impact")
	}
	if _, err := s.ApplyPreview(ctx, pv.Token, 0); err != nil {
		t.Fatal(err)
	}
	// 校验失败的提案只返回原因
	pv, _ = s.PreviewChanges(ctx, change.SourceUserRequest, []change.Change{{Op: change.OpPatch, Target: "brass:location/nowhere", Path: "fields.description", Value: []byte(`"x"`), Reason: "r"}})
	if pv.Token != "" || len(pv.Rejected) == 0 {
		t.Fatalf("invalid preview %+v", pv)
	}
}

// TestSlotLock：游戏会话持有存档锁时，外部写入（MCP）被拒绝；游戏退出后可以写入。
func TestSlotLock(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "lock.db")
	game, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: db, Packs: packages.Builtin()})
	if err != nil {
		t.Fatal(err)
	}
	slot, err := game.NewGameIn(ctx, "brass_trial", "", "阿砾", 7)
	if err != nil {
		t.Fatal(err)
	}
	ext, err := orchestrator.Open(ctx, orchestrator.Options{DBPath: db, Packs: packages.Builtin(), NoSlotLock: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ext.Close() }()
	if _, err := ext.OpenForWrite(ctx, slot); !errors.Is(err, orchestrator.ErrSlotBusy) {
		t.Fatalf("err = %v, want ErrSlotBusy", err)
	}
	_ = game.Close()
	if _, err := ext.OpenForWrite(ctx, slot); err != nil {
		t.Fatal(err)
	}
}

func hasKind(es []dto.EntryV1, k string) bool {
	for _, e := range es {
		if e.Kind == k {
			return true
		}
	}
	return false
}

func kindsOf(es []dto.EntryV1) string {
	var ks []string
	for _, e := range es {
		ks = append(ks, e.Kind)
	}
	return strings.Join(ks, ",")
}

// cardLLM：卡片编辑请求返回脚本化的修改提案。
type cardLLM struct{ reply string }

func (f *cardLLM) RoundTrip(r *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(r.Body)
	if strings.Contains(string(body), "卡片编辑器") {
		return chat(f.reply), nil
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(sse("你环顾四周。"))), Header: http.Header{}}, nil
}

// TestRequestEdit：玩家要求修改卡片 → card_gen 提案 → 预览（不提交）→ 确认后才写入，来源为“你的指令”。
func TestRequestEdit(t *testing.T) {
	ctx := context.Background()
	f := &cardLLM{reply: `{"changes":[{"op":"patch","target":"brass:item/wrench","path":"fields.description","value":"一把改装过的扳手，握柄里藏着一块小电池，敲上去会冒蓝色电火花。"}],"note":"加了电击效果"}`}
	s := openPacks(t, f)
	s.ConfigureProviders([]router.Provider{{ID: "ds", Kind: provider.KindDeepSeek, APIKey: "sk-test"}}, router.Settings{Mode: router.ModeUnified, Unified: router.Route{Provider: "ds"}})
	if _, err := s.NewGameIn(ctx, "brass_trial", "", "阿砾", 7); err != nil {
		t.Fatal(err)
	}
	pv, err := s.RequestEdit(ctx, "brass:item/wrench", "把我的扳手改成带电击的")
	if err != nil {
		t.Fatal(err)
	}
	if pv.Token == "" || len(pv.Changes) != 1 || pv.Note == "" || !strings.HasPrefix(pv.Changes[0].Reason, "你的要求") {
		t.Fatalf("preview %+v", pv)
	}
	if chs, _ := s.WorldChanges("", "", 0); len(chs) != 0 {
		t.Fatal("request_edit must not commit before confirmation")
	}
	if _, err := s.ApplyPreview(ctx, pv.Token, 0); err != nil {
		t.Fatal(err)
	}
	chs, _ := s.WorldChanges("", "", 0)
	if len(chs) != 1 || chs[0].Source != change.SourceUserRequest {
		t.Fatalf("log %+v", chs)
	}
	if _, err := s.RequestEdit(ctx, "brass:item/nope", "随便改改"); err == nil {
		t.Fatal("unknown target should fail")
	}
}
