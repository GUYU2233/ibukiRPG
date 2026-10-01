package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// ---------- 统一写入网关：预览 → 确认（第 14.1 / 14.3 / 14.5 节） ----------

// ErrPreviewStale 表示预览之后存档头部已经变化（例如后台审查提交了修复），需要重新预览。
var ErrPreviewStale = errors.New("存档在预览之后发生了变化，请重新预览")

// ErrSlotBusy 表示存档正被另一个游戏进程使用（slot_lock）。
var ErrSlotBusy = errors.New("存档正在被游戏使用，请先退出游戏或只读查询")

// MCPImpactLimit 是 MCP 写入不带 accept_impact 时允许的最大影响分（超过即拒绝）。
const MCPImpactLimit = 50

type preview struct {
	slot    string
	source  string
	changes []change.Change
	head    string
	impact  int
}

// headHash 是存档当前状态的指纹（预览令牌的一部分）：任何提交都会改变它。
func headHash(slot string, st *state.State) string {
	b, _ := json.Marshal(st)
	h := sha256.Sum256(append([]byte(slot+"\x00"), b...))
	return hex.EncodeToString(h[:8])
}

// PreviewChanges 对一组变更提案做“演练”：走与游戏内完全相同的校验与影响评估，但不提交任何事件。
// 返回字段级差异（玩家不知道的字段只计数，不剧透）、被拒绝项与原因，以及确认时使用的 preview_token。
func (s *Session) PreviewChanges(ctx context.Context, source string, changes []change.Change) (dto.ChangePreviewV1, error) {
	if len(changes) == 0 {
		return dto.ChangePreviewV1{}, errors.New("没有要预览的修改")
	}
	slot, st, g, err := s.current()
	if err != nil {
		return dto.ChangePreviewV1{}, err
	}
	cmd := command.Command{ID: "preview", Kind: command.KindWorldChange, Changes: changes, Source: source}
	res, after, err := g.eng.Execute(st, cmd)
	if err != nil {
		return dto.ChangePreviewV1{}, err
	}
	out := dto.ChangePreviewV1{}
	for _, r := range res.Rejects {
		out.Rejected = append(out.Rejected, dto.RejectV1{Target: r.Target, Path: r.Path, Op: r.Op, Reason: r.Reason})
	}
	if !res.Accepted {
		if out.Rejected == nil && res.Reason != "" {
			out.Rejected = append(out.Rejected, dto.RejectV1{Reason: res.Reason})
		}
		return out, nil
	}
	if log := g.q.WorldLog(after, res.Events); log != nil {
		for _, c := range log.Changes {
			if c.Hidden {
				out.HiddenCount++
				continue
			}
			out.Changes = append(out.Changes, c)
		}
		out.Impact = log.Impact
	}
	if res.Impact != nil {
		out.Impact = res.Impact.Score
		out.Types = res.Impact.Types
	}
	if len(out.Changes) == 0 && out.HiddenCount == 0 {
		return out, nil
	}
	head := headHash(slot, st)
	b, _ := json.Marshal(changes)
	h := sha256.Sum256(append([]byte(source+"\x00"+head+"\x00"), b...))
	out.Token = "pv-" + hex.EncodeToString(h[:8])
	s.prevMu.Lock()
	if s.previews == nil {
		s.previews = map[string]preview{}
	}
	s.previews[out.Token] = preview{slot: slot, source: source, changes: changes, head: head, impact: out.Impact}
	s.prevMu.Unlock()
	return out, nil
}

// ApplyPreview 提交一份预览过的提案（同一套校验器重新执行，结果作为“系统回合”写入事件日志）。
// maxImpact>0 时影响分超过它的提案被拒绝（MCP 写入未带 accept_impact）。
func (s *Session) ApplyPreview(ctx context.Context, token string, maxImpact int) (dto.WorldLogV1, error) {
	s.prevMu.Lock()
	p, ok := s.previews[token]
	s.prevMu.Unlock()
	if !ok {
		return dto.WorldLogV1{}, errors.New("预览令牌无效或已使用，请重新预览")
	}
	if maxImpact > 0 && p.impact > maxImpact {
		return dto.WorldLogV1{}, fmt.Errorf("影响分 %d 超过上限 %d：确认要这样改时带 accept_impact=true", p.impact, maxImpact)
	}
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	slot, st, g, err := s.current()
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	if slot != p.slot || headHash(slot, st) != p.head {
		return dto.WorldLogV1{}, ErrPreviewStale
	}
	log, err := s.commitSystemChanges(ctx, slot, g, st, "apply-"+strings.TrimPrefix(token, "pv-"), p.source, p.changes)
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	s.prevMu.Lock()
	delete(s.previews, token)
	s.prevMu.Unlock()
	return log, nil
}

// commitSystemChanges 以“系统回合”提交一组变更（不推进世界时间、不产生叙事）。调用方持有 turnMu。
func (s *Session) commitSystemChanges(ctx context.Context, slot string, g *game, st *state.State, cmdID, source string, changes []change.Change) (dto.WorldLogV1, error) {
	st, err := s.forkIfPending(ctx, slot, g, st, cmdID)
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	cmd := command.Command{ID: cmdID, Kind: command.KindWorldChange, Changes: changes, Source: source}
	res, after, err := g.eng.Execute(st, cmd)
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	if !res.Accepted || len(res.Events) == 0 {
		reason := res.Reason
		if reason == "" && len(res.Rejects) > 0 {
			reason = res.Rejects[0].Reason
		}
		if reason == "" {
			reason = "没有可提交的修改"
		}
		return dto.WorldLogV1{}, errors.New(reason)
	}
	log := g.q.WorldLog(after, res.Events)
	var entries []eventstore.Entry
	if log != nil {
		entries = append(entries, fromEntry(dto.EntryV1{CommandID: cmdID, Kind: "world", Turn: res.Turn, World: log, Source: source}))
	}
	empty := ""
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{CommandID: cmd.ID, Accepted: true, Command: cmd, Result: res, Events: res.Events, After: after,
		Entries: entries, Summary: g.summary(after), Narration: &empty}); err != nil {
		if errors.Is(err, eventstore.ErrDuplicateCommand) {
			return dto.WorldLogV1{}, errors.New("这项修改已经提交过了")
		}
		return dto.WorldLogV1{}, err
	}
	s.mu.Lock()
	if s.slot == slot {
		s.st = after
	}
	s.mu.Unlock()
	if log == nil {
		return dto.WorldLogV1{}, nil
	}
	return *log, nil
}

// ---------- slot_lock：存档占用锁（第 14.5 节） ----------

// slotLockTTL 是锁的过期时间：持有者每 10 秒刷新一次。
const slotLockTTL = 30 * time.Second

func (s *Session) lockPath(slot string) string {
	db := s.opts.DBPath
	if db == "" || strings.Contains(db, ":memory:") || strings.Contains(db, "mode=memory") || slot == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(db), "locks", slot+".lock")
}

// holdSlot 让本进程持有存档锁（载入 / 新建存档时调用），并在后台刷新心跳。
func (s *Session) holdSlot(slot string) {
	if s.opts.NoSlotLock {
		return
	}
	p := s.lockPath(slot)
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if s.lockStop != nil {
		close(s.lockStop)
		if s.lockFile != "" && s.lockFile != p {
			_ = os.Remove(s.lockFile)
		}
		s.lockStop, s.lockFile = nil, ""
	}
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return
	}
	write := func() { _ = os.WriteFile(p, []byte(s.lockOwner()), 0o600) }
	write()
	stop := make(chan struct{})
	s.lockStop, s.lockFile = stop, p
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				write()
			}
		}
	}()
}

func (s *Session) releaseSlot() {
	s.lockMu.Lock()
	defer s.lockMu.Unlock()
	if s.lockStop != nil {
		close(s.lockStop)
		_ = os.Remove(s.lockFile)
		s.lockStop, s.lockFile = nil, ""
	}
}

// SlotBusy 报告存档是否正被**另一个**会话持有（锁存在、未过期、持有者不是本会话）。
func (s *Session) SlotBusy(slot string) bool {
	p := s.lockPath(slot)
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	if err != nil || time.Since(fi.ModTime()) > slotLockTTL {
		return false
	}
	b, err := os.ReadFile(filepath.Clean(p)) // #nosec G304 -- 路径由存档目录与存档 ID 拼成
	if err != nil {
		return false
	}
	owner := strings.TrimSpace(string(b))
	return owner != "" && owner != s.lockOwner()
}

// lockOwner 是本会话的锁持有者标识（进程号 + 会话随机串）。
func (s *Session) lockOwner() string {
	s.ownerOnce.Do(func() { s.owner = strconv.Itoa(os.Getpid()) + "-" + NewCommandID() })
	return s.owner
}

// OpenForWrite 为外部写入（MCP）载入存档：被游戏占用时返回 ErrSlotBusy。slotID 为空时取最近更新的存档。
// 不持有存档锁（MCP 写入是一次性的短操作）。
func (s *Session) OpenForWrite(ctx context.Context, slotID string) (string, error) {
	return s.openExternal(ctx, slotID, true)
}

// OpenForRead 为外部只读查询（MCP 世界工具）载入存档的最新状态，不检查占用锁。
func (s *Session) OpenForRead(ctx context.Context, slotID string) (string, error) {
	return s.openExternal(ctx, slotID, false)
}

func (s *Session) openExternal(ctx context.Context, slotID string, write bool) (string, error) {
	if slotID == "" {
		slots, err := s.store.ListSlots(ctx)
		if err != nil {
			return "", err
		}
		if len(slots) == 0 {
			return "", ErrNoGame
		}
		slotID = slots[0].ID
	}
	if write && s.SlotBusy(slotID) {
		return "", ErrSlotBusy
	}
	s.mu.RLock()
	cur, st := s.slot, s.st
	s.mu.RUnlock()
	if cur == slotID && st != nil {
		// 重新载入，拿到其他进程提交后的最新状态
		if fresh, err := s.store.Load(ctx, slotID); err == nil {
			s.mu.Lock()
			s.st = fresh
			s.mu.Unlock()
			return slotID, nil
		}
	}
	return slotID, s.LoadGame(ctx, slotID)
}
