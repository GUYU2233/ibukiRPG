package eventstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/GUYU2233/ibukiRPG/internal/storage/sqlite"
)

// 分支状态。
const (
	BranchActive     = "active"
	BranchRolledBack = "rolled_back"
	BranchArchived   = "archived"
)

// Branch 是一条时间线的元数据（架构 V0.3 第 10.1 节）。
type Branch struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Parent   string `json:"parent,omitempty"`
	ForkSeq  int64  `json:"fork_seq"`
	ForkTurn int    `json:"fork_turn"`
	Salt     uint64 `json:"rng_salt"`
	Status   string `json:"status"`
	Created  int64  `json:"created_at"`
	HeadSeq  int64  `json:"head_seq"`
	HeadTurn int    `json:"head_turn"`
}

// ErrBranch 表示分支操作不合法（例如删除当前分支）。
var ErrBranch = errors.New("branch operation not allowed")

// NewSalt 生成新分支的随机盐（元数据随机，记录在分支第一个事件里，重放仍确定）。
func NewSalt() uint64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	v := binary.LittleEndian.Uint64(b[:])
	if v == 0 {
		v = 1
	}
	return v
}

func newBranchID() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return "b-" + hex.EncodeToString(b[:])
}

// Branches 列出存档的全部分支（按创建时间）。
func (s *Store) Branches(ctx context.Context, slotID string) ([]Branch, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT branch_id,name,parent_branch,fork_seq,fork_turn,rng_salt,status,created_at,head_seq,head_turn
		FROM branches WHERE slot_id=? ORDER BY created_at, rowid`, slotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Branch
	for rows.Next() {
		var b Branch
		var salt int64
		if err := rows.Scan(&b.ID, &b.Name, &b.Parent, &b.ForkSeq, &b.ForkTurn, &salt, &b.Status, &b.Created, &b.HeadSeq, &b.HeadTurn); err != nil {
			return nil, err
		}
		b.Salt = uint64(salt) //nolint:gosec // 位模式还原
		out = append(out, b)
	}
	return out, rows.Err()
}

// GetBranch 读取单个分支。
func (s *Store) GetBranch(ctx context.Context, slotID, branch string) (Branch, error) {
	bs, err := s.Branches(ctx, slotID)
	if err != nil {
		return Branch{}, err
	}
	for _, b := range bs {
		if b.ID == branch {
			return b, nil
		}
	}
	return Branch{}, ErrNotFound
}

// SeqAtTurnEnd 返回分支上回合 turn 的最后一个事件序号（回合 0 = 开局快照之后的事件为空时返回 0）。
func (s *Store) SeqAtTurnEnd(ctx context.Context, slotID, branch string, turn int) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM events WHERE slot_id=? AND branch_id=? AND turn<=?`, slotID, branch, turn).Scan(&seq)
	return seq, err
}

// SetPendingFork 记录“回到这里”的目标；下一次提交时才真正分叉。
func (s *Store) SetPendingFork(ctx context.Context, slotID string, f ForkRef) error {
	_, err := s.db.ExecContext(ctx, `UPDATE saves SET pending_fork_branch=?, pending_fork_seq=?, pending_fork_turn=? WHERE slot_id=?`, f.Branch, f.Seq, f.Turn, slotID)
	return err
}

// ClearPendingFork 取消待定回滚。
func (s *Store) ClearPendingFork(ctx context.Context, slotID string) error {
	return s.SetPendingFork(ctx, slotID, ForkRef{})
}

// Fork 从 (from, seq/turn) 分出新分支并设为当前分支：复制分叉点之前的事件、快照、对话记录与记忆，
// 使每个分支自成一体（读取只看本分支，等价于沿谱系重放）。返回新分支。
func (s *Store) Fork(ctx context.Context, slotID string, at ForkRef, name string, salt uint64) (Branch, error) {
	b := Branch{ID: newBranchID(), Name: name, Parent: at.Branch, ForkSeq: at.Seq, ForkTurn: at.Turn, Salt: salt, Status: BranchActive, Created: s.now().UnixMilli(), HeadSeq: at.Seq, HeadTurn: at.Turn}
	err := sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM branches WHERE slot_id=?`, slotID).Scan(&n); err != nil {
			return err
		}
		if b.Name == "" {
			b.Name = "分支 " + string("ABCDEFGHIJKLMNOPQRSTUVWXYZ"[min(n-1, 25)])
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO branches(slot_id,branch_id,name,parent_branch,fork_seq,fork_turn,rng_salt,status,created_at,head_seq,head_turn)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`, slotID, b.ID, b.Name, b.Parent, b.ForkSeq, b.ForkTurn, int64(b.Salt), b.Status, b.Created, b.HeadSeq, b.HeadTurn); err != nil { //nolint:gosec // 位模式存储
			return err
		}
		qs := []struct {
			q    string
			args []any
		}{
			{`INSERT INTO events(slot_id,branch_id,seq,command_id,turn,minute,type,data) SELECT slot_id,?,seq,command_id,turn,minute,type,data FROM events WHERE slot_id=? AND branch_id=? AND seq<=?`, []any{b.ID, slotID, at.Branch, at.Seq}},
			{`INSERT INTO snapshots(slot_id,branch_id,seq,state) SELECT slot_id,?,seq,state FROM snapshots WHERE slot_id=? AND branch_id=? AND seq<=?`, []any{b.ID, slotID, at.Branch, at.Seq}},
			{`INSERT INTO transcript(slot_id,branch_id,command_id,turn,kind,text,meta) SELECT slot_id,?,command_id,turn,kind,text,meta FROM transcript WHERE slot_id=? AND branch_id=? AND turn<=? ORDER BY id`, []any{b.ID, slotID, at.Branch, at.Turn}},
			{`INSERT INTO memory(slot_id,branch_id,kind,subject,from_turn,to_turn,text,compressed) SELECT slot_id,?,kind,subject,from_turn,to_turn,text,compressed FROM memory WHERE slot_id=? AND branch_id=? AND to_turn<=? ORDER BY id`, []any{b.ID, slotID, at.Branch, at.Turn}},
			{`UPDATE saves SET current_branch=?, pending_fork_branch='', pending_fork_seq=0, pending_fork_turn=0, last_seq=? WHERE slot_id=?`, []any{b.ID, at.Seq, slotID}},
		}
		for _, q := range qs {
			if _, err := tx.ExecContext(ctx, q.q, q.args...); err != nil {
				return err
			}
		}
		return nil
	})
	return b, err
}

// SwitchBranch 切换当前分支（清除待定回滚）。
func (s *Store) SwitchBranch(ctx context.Context, slotID, branch string) error {
	b, err := s.GetBranch(ctx, slotID, branch)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE saves SET current_branch=?, pending_fork_branch='', pending_fork_seq=0, pending_fork_turn=0, last_seq=? WHERE slot_id=?`, branch, b.HeadSeq, slotID)
	return err
}

// RenameBranch 重命名分支。
func (s *Store) RenameBranch(ctx context.Context, slotID, branch, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE branches SET name=? WHERE slot_id=? AND branch_id=?`, name, slotID, branch)
	return err
}

// SetBranchStatus 标记分支状态（例如偏离提示里回滚后标记为 rolled_back，分支仍保留）。
func (s *Store) SetBranchStatus(ctx context.Context, slotID, branch, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE branches SET status=? WHERE slot_id=? AND branch_id=?`, status, slotID, branch)
	return err
}

// DeleteBranch 删除一个非当前分支及其数据。主干不能删除。
func (s *Store) DeleteBranch(ctx context.Context, slotID, branch string) error {
	cur, err := s.branchOf(ctx, s.db, slotID)
	if err != nil {
		return err
	}
	if branch == cur || branch == MainBranch {
		return ErrBranch
	}
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, t := range []string{"events", "snapshots", "transcript", "memory", "checkpoints", "branches"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+t+` WHERE slot_id=? AND branch_id=?`, slotID, branch); err != nil { //nolint:gosec // 表名来自常量
				return err
			}
		}
		return nil
	})
}

// ---------- 检查点 ----------

// Checkpoint 是指向某分支某位置的命名书签（第 10.5 节）。
type Checkpoint struct {
	ID      string `json:"id"`
	Branch  string `json:"branch"`
	Seq     int64  `json:"seq"`
	Turn    int    `json:"turn"`
	Name    string `json:"name"`
	Kind    string `json:"kind"` // auto / manual
	Reason  string `json:"reason,omitempty"`
	Created int64  `json:"created_at"`
}

// CreateCheckpoint 新建检查点；kind=auto 时只保留最近 keepAuto 个自动检查点。
func (s *Store) CreateCheckpoint(ctx context.Context, slotID string, cp Checkpoint, keepAuto int) (Checkpoint, error) {
	if cp.ID == "" {
		var b [4]byte
		_, _ = rand.Read(b[:])
		cp.ID = "cp-" + hex.EncodeToString(b[:])
	}
	cp.Created = s.now().UnixMilli()
	err := sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO checkpoints(slot_id,id,branch_id,seq,turn,name,kind,reason,created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			slotID, cp.ID, cp.Branch, cp.Seq, cp.Turn, cp.Name, cp.Kind, cp.Reason, cp.Created); err != nil {
			return err
		}
		if cp.Kind == "auto" && keepAuto > 0 {
			_, err := tx.ExecContext(ctx, `DELETE FROM checkpoints WHERE slot_id=? AND kind='auto' AND id NOT IN
				(SELECT id FROM checkpoints WHERE slot_id=? AND kind='auto' ORDER BY created_at DESC, rowid DESC LIMIT ?)`, slotID, slotID, keepAuto)
			return err
		}
		return nil
	})
	return cp, err
}

// Checkpoints 列出检查点（新的在前）。
func (s *Store) Checkpoints(ctx context.Context, slotID string) ([]Checkpoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,branch_id,seq,turn,name,kind,reason,created_at FROM checkpoints WHERE slot_id=? ORDER BY created_at DESC, rowid DESC`, slotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Checkpoint
	for rows.Next() {
		var c Checkpoint
		if err := rows.Scan(&c.ID, &c.Branch, &c.Seq, &c.Turn, &c.Name, &c.Kind, &c.Reason, &c.Created); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RenameCheckpoint 重命名检查点。
func (s *Store) RenameCheckpoint(ctx context.Context, slotID, id, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE checkpoints SET name=? WHERE slot_id=? AND id=?`, name, slotID, id)
	return err
}

// DeleteCheckpoint 删除检查点。
func (s *Store) DeleteCheckpoint(ctx context.Context, slotID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM checkpoints WHERE slot_id=? AND id=?`, slotID, id)
	return err
}

// ---------- AI 调用记账（非游戏事实）----------

// AICall 是一次模型调用的记账（第 11.6 节）。不含请求正文与密钥。
type AICall struct {
	Branch           string `json:"branch"`
	CommandID        string `json:"command_id"`
	Turn             int    `json:"turn"`
	Task             string `json:"task"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	CachedTokens     int    `json:"cached_tokens"`
	LatencyMS        int64  `json:"latency_ms"`
	OK               bool   `json:"ok"`
	Error            string `json:"error,omitempty"`
}

// RecordAICall 写入一次 AI 调用记录。
func (s *Store) RecordAICall(ctx context.Context, slotID string, c AICall) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO ai_calls(slot_id,branch_id,command_id,turn,task,provider,model,prompt_tokens,completion_tokens,cached_tokens,latency_ms,ok,error,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, slotID, c.Branch, c.CommandID, c.Turn, c.Task, c.Provider, c.Model, c.PromptTokens, c.CompletionTokens, c.CachedTokens, c.LatencyMS, boolInt(c.OK), c.Error, s.now().UnixMilli())
	return err
}

// AICalls 返回 AI 调用记录；turn>0 时只返回该回合（当前分支）。
func (s *Store) AICalls(ctx context.Context, slotID, branch string, turn int) ([]AICall, error) {
	q := `SELECT branch_id,command_id,turn,task,provider,model,prompt_tokens,completion_tokens,cached_tokens,latency_ms,ok,error FROM ai_calls WHERE slot_id=?`
	args := []any{slotID}
	if branch != "" {
		q += ` AND branch_id=?`
		args = append(args, branch)
	}
	if turn > 0 {
		q += ` AND turn=?`
		args = append(args, turn)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AICall
	for rows.Next() {
		var c AICall
		var ok int
		if err := rows.Scan(&c.Branch, &c.CommandID, &c.Turn, &c.Task, &c.Provider, &c.Model, &c.PromptTokens, &c.CompletionTokens, &c.CachedTokens, &c.LatencyMS, &ok, &c.Error); err != nil {
			return nil, err
		}
		c.OK = ok == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- 被拒绝的世界修改（诊断）----------

// Reject 是一条被校验器拒绝的提案。
type Reject struct {
	Branch    string `json:"branch"`
	CommandID string `json:"command_id"`
	Turn      int    `json:"turn"`
	Kind      string `json:"kind"` // change / reveal
	Proposal  string `json:"proposal"`
	Reason    string `json:"reason"`
}

// RecordRejects 写入被拒绝的提案。
func (s *Store) RecordRejects(ctx context.Context, slotID string, rs []Reject) error {
	if len(rs) == 0 {
		return nil
	}
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, r := range rs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO world_change_rejects(slot_id,branch_id,command_id,turn,kind,proposal,reason,created_at) VALUES (?,?,?,?,?,?,?,?)`,
				slotID, r.Branch, r.CommandID, r.Turn, r.Kind, r.Proposal, r.Reason, s.now().UnixMilli()); err != nil {
				return err
			}
		}
		return nil
	})
}

// Rejects 返回分支上的被拒绝提案（新的在后）。
func (s *Store) Rejects(ctx context.Context, slotID, branch string) ([]Reject, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT branch_id,command_id,turn,kind,proposal,reason FROM world_change_rejects WHERE slot_id=? AND branch_id=? ORDER BY id`, slotID, branch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Reject
	for rows.Next() {
		var r Reject
		if err := rows.Scan(&r.Branch, &r.CommandID, &r.Turn, &r.Kind, &r.Proposal, &r.Reason); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
