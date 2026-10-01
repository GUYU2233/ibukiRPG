package eventstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/storage/sqlite"
)

// SnapshotInterval：每追加这么多事件写一次快照。恢复 = 最近快照 + 之后的事件（第 43 节）。
const SnapshotInterval = 25

// schemaVersion 是存档数据库结构版本。v2（0.2.0）：分支、检查点、AI 调用记录。
const schemaVersion = 2

// MainBranch 是每个存档的根分支 ID（“主干”）。
const MainBranch = "main"

// LegacyMessage 是 0.1.x 存档被拒绝时给玩家看的说明（架构 V0.3 第 10.7 节）。
const LegacyMessage = "这些存档来自 v0.1.x。0.2.0 改成了开放世界，存档结构完全不同，无法继续。如需继续旧存档，请安装 v0.1.3-rc1；也可以删除它们释放空间。"

// ErrLegacyDatabase 表示打开的是 0.1.x 的存档数据库（schema v1）。
var ErrLegacyDatabase = errors.New(LegacyMessage)

// ErrDuplicateCommand 表示 command_id 已执行过（第 52 节）。
var ErrDuplicateCommand = errors.New("duplicate command_id")

// ErrNotFound 表示存档不存在。
var ErrNotFound = errors.New("save slot not found")

// Store 是基于 SQLite 的 Event Store + Snapshot + Save 元数据。
// 事件表只追加；唯一的删除是删除整个存档。
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// PackageRef 记录存档依赖的内容包版本（第 44 节）。
type PackageRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// Summary 是存档列表上显示的摘要。
type Summary struct {
	Location string `json:"location"`
	Time     string `json:"time"`
	Turn     int    `json:"turn"`
	Gold     int    `json:"gold"`
	Story    string `json:"story,omitempty"`
}

// Slot 是存档元数据。
type Slot struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	PlayerName    string       `json:"player_name"`
	CreatedAt     int64        `json:"created_at"`
	UpdatedAt     int64        `json:"updated_at"`
	EngineVersion string       `json:"engine_version"`
	Packages      []PackageRef `json:"packages"`
	Seed          uint64       `json:"seed"`
	Summary       Summary      `json:"summary"`
	LastSeq       int64        `json:"last_seq"`
	// Branch 是当前分支；PendingFork 非空表示玩家选择了“回到这里”但还没有行动（第 10.3 节）。
	Branch      string  `json:"branch"`
	PendingFork ForkRef `json:"pending_fork"`
	Flags       string  `json:"flags,omitempty"`
}

// ForkRef 指向某分支某个位置（回合末尾）。
type ForkRef struct {
	Branch string `json:"branch,omitempty"`
	Seq    int64  `json:"seq,omitempty"`
	Turn   int    `json:"turn,omitempty"`
}

// Entry 是对话记录（叙事日志）中的一条。叙事文本不是游戏事实，单独存放。
type Entry struct {
	ID        int64           `json:"id"`
	CommandID string          `json:"command_id,omitempty"`
	Turn      int             `json:"turn"`
	Kind      string          `json:"kind"` // player / narration / system / check / story / error
	Text      string          `json:"text"`
	Meta      json.RawMessage `json:"meta,omitempty"`
}

// CommandRecord 是已执行命令的记录（幂等与恢复用）。
type CommandRecord struct {
	CommandID string
	Accepted  bool
	Result    json.RawMessage
	Narration string
	HasNarr   bool
}

// Open 打开（或创建）存档数据库。
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sqlite.Open(ctx, dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	// 先检查旧版本：0.1.x 的数据库没有 branches 表且 schema_version = 1。
	var hasMeta int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&hasMeta); err != nil {
		return err
	}
	if hasMeta > 0 {
		var v int
		err := s.db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM meta WHERE key='schema_version'`).Scan(&v)
		if err == nil && v < schemaVersion {
			return ErrLegacyDatabase
		}
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS saves (
			slot_id TEXT PRIMARY KEY, name TEXT NOT NULL, player_name TEXT NOT NULL,
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, engine_version TEXT NOT NULL,
			packages TEXT NOT NULL, seed INTEGER NOT NULL, summary TEXT NOT NULL, last_seq INTEGER NOT NULL DEFAULT 0,
			current_branch TEXT NOT NULL DEFAULT 'main', pending_fork_branch TEXT NOT NULL DEFAULT '',
			pending_fork_seq INTEGER NOT NULL DEFAULT 0, pending_fork_turn INTEGER NOT NULL DEFAULT 0,
			flags TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS branches (
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE,
			branch_id TEXT NOT NULL, name TEXT NOT NULL, parent_branch TEXT NOT NULL DEFAULT '',
			fork_seq INTEGER NOT NULL DEFAULT 0, fork_turn INTEGER NOT NULL DEFAULT 0, rng_salt INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active', created_at INTEGER NOT NULL, head_seq INTEGER NOT NULL DEFAULT 0,
			head_turn INTEGER NOT NULL DEFAULT 0, PRIMARY KEY (slot_id, branch_id))`,
		`CREATE TABLE IF NOT EXISTS events (
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE, branch_id TEXT NOT NULL,
			seq INTEGER NOT NULL, command_id TEXT NOT NULL, turn INTEGER NOT NULL, minute INTEGER NOT NULL,
			type TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY (slot_id, branch_id, seq))`,
		`CREATE TABLE IF NOT EXISTS commands (
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE, branch_id TEXT NOT NULL DEFAULT 'main',
			command_id TEXT NOT NULL, accepted INTEGER NOT NULL, command TEXT NOT NULL, result TEXT NOT NULL,
			narration TEXT, created_at INTEGER NOT NULL, PRIMARY KEY (slot_id, command_id))`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE, branch_id TEXT NOT NULL,
			seq INTEGER NOT NULL, state TEXT NOT NULL, PRIMARY KEY (slot_id, branch_id, seq))`,
		`CREATE TABLE IF NOT EXISTS transcript (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE, branch_id TEXT NOT NULL DEFAULT 'main',
			command_id TEXT NOT NULL DEFAULT '', turn INTEGER NOT NULL, kind TEXT NOT NULL, text TEXT NOT NULL, meta TEXT)`,
		`CREATE INDEX IF NOT EXISTS transcript_slot ON transcript(slot_id, branch_id, id)`,
		`CREATE TABLE IF NOT EXISTS memory (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE, branch_id TEXT NOT NULL DEFAULT 'main',
			kind TEXT NOT NULL, subject TEXT NOT NULL DEFAULT '', from_turn INTEGER NOT NULL, to_turn INTEGER NOT NULL,
			text TEXT NOT NULL, compressed TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS memory_slot ON memory(slot_id, branch_id, kind, to_turn)`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE,
			id TEXT NOT NULL, branch_id TEXT NOT NULL, seq INTEGER NOT NULL, turn INTEGER NOT NULL, name TEXT NOT NULL,
			kind TEXT NOT NULL, reason TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, PRIMARY KEY (slot_id, id))`,
		`CREATE TABLE IF NOT EXISTS ai_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE, branch_id TEXT NOT NULL,
			command_id TEXT NOT NULL, turn INTEGER NOT NULL, task TEXT NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL, completion_tokens INTEGER NOT NULL, cached_tokens INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL, ok INTEGER NOT NULL, error TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS ai_calls_slot ON ai_calls(slot_id, branch_id, turn)`,
		`CREATE TABLE IF NOT EXISTS world_change_rejects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE, branch_id TEXT NOT NULL,
			command_id TEXT NOT NULL, turn INTEGER NOT NULL, kind TEXT NOT NULL, proposal TEXT NOT NULL, reason TEXT NOT NULL,
			created_at INTEGER NOT NULL)`,
		`INSERT INTO meta(key, value) VALUES ('schema_version', '2') ON CONFLICT(key) DO NOTHING`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	var v int
	if err := s.db.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM meta WHERE key='schema_version'`).Scan(&v); err != nil {
		return err
	}
	if v > schemaVersion {
		return fmt.Errorf("save database schema %d is newer than this app (%d)", v, schemaVersion)
	}
	return nil
}

// NewSlotID 生成存档 ID（元数据用途，非游戏随机数）。
func NewSlotID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("slot-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

// CreateSlot 创建存档、主干分支，并写入初始快照（seq 0）。
func (s *Store) CreateSlot(ctx context.Context, slot Slot, initial *state.State) error {
	pk, _ := json.Marshal(slot.Packages)
	sum, _ := json.Marshal(slot.Summary)
	st, err := initial.Marshal()
	if err != nil {
		return err
	}
	now := s.now().UnixMilli()
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO saves(slot_id,name,player_name,created_at,updated_at,engine_version,packages,seed,summary,last_seq,current_branch)
			VALUES (?,?,?,?,?,?,?,?,?,0,?)`, slot.ID, slot.Name, slot.PlayerName, now, now, slot.EngineVersion, string(pk), int64(slot.Seed), string(sum), MainBranch); err != nil { //nolint:gosec // seed 以 int64 位模式存储
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO branches(slot_id,branch_id,name,created_at) VALUES (?,?,?,?)`, slot.ID, MainBranch, "主干", now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO snapshots(slot_id, branch_id, seq, state) VALUES (?, ?, 0, ?)`, slot.ID, MainBranch, string(st))
		return err
	})
}

func scanSlot(sc interface{ Scan(...any) error }) (Slot, error) {
	var sl Slot
	var pk, sum string
	var seed int64
	if err := sc.Scan(&sl.ID, &sl.Name, &sl.PlayerName, &sl.CreatedAt, &sl.UpdatedAt, &sl.EngineVersion, &pk, &seed, &sum, &sl.LastSeq,
		&sl.Branch, &sl.PendingFork.Branch, &sl.PendingFork.Seq, &sl.PendingFork.Turn, &sl.Flags); err != nil {
		return sl, err
	}
	sl.Seed = uint64(seed) //nolint:gosec // 位模式还原
	_ = json.Unmarshal([]byte(pk), &sl.Packages)
	_ = json.Unmarshal([]byte(sum), &sl.Summary)
	return sl, nil
}

const slotCols = `slot_id,name,player_name,created_at,updated_at,engine_version,packages,seed,summary,last_seq,current_branch,pending_fork_branch,pending_fork_seq,pending_fork_turn,flags`

// ListSlots 按最近游玩时间倒序列出存档。
func (s *Store) ListSlots(ctx context.Context) ([]Slot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+slotCols+` FROM saves ORDER BY updated_at DESC, slot_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Slot{}
	for rows.Next() {
		sl, err := scanSlot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sl)
	}
	return out, rows.Err()
}

// GetSlot 读取单个存档元数据。
func (s *Store) GetSlot(ctx context.Context, id string) (Slot, error) {
	sl, err := scanSlot(s.db.QueryRowContext(ctx, `SELECT `+slotCols+` FROM saves WHERE slot_id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return sl, ErrNotFound
	}
	return sl, err
}

// branchOf 返回存档的当前分支。
func (s *Store) branchOf(ctx context.Context, q querier, slotID string) (string, error) {
	var b string
	err := q.QueryRowContext(ctx, `SELECT current_branch FROM saves WHERE slot_id=?`, slotID).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return b, err
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DeleteSlot 删除整个存档（事件、快照、记录级联删除）。
func (s *Store) DeleteSlot(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM saves WHERE slot_id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RenameSlot 修改存档名。
func (s *Store) RenameSlot(ctx context.Context, id, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE saves SET name=? WHERE slot_id=?`, name, id)
	return err
}

// SetSlotFlags 写入存档标记（例如“外部工具修改了设定”的待通知计数）。
func (s *Store) SetSlotFlags(ctx context.Context, id, flags string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE saves SET flags=? WHERE slot_id=?`, flags, id)
	return err
}

// CopySlot 复制存档（全部分支、事件、快照、命令、对话记录、检查点），返回新存档 ID。
func (s *Store) CopySlot(ctx context.Context, src, name string) (string, error) {
	if _, err := s.GetSlot(ctx, src); err != nil {
		return "", err
	}
	dst := NewSlotID()
	now := s.now().UnixMilli()
	err := sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		qs := []string{
			`INSERT INTO saves(slot_id,name,player_name,created_at,updated_at,engine_version,packages,seed,summary,last_seq,current_branch,pending_fork_branch,pending_fork_seq,pending_fork_turn)
			 SELECT ?, ?, player_name, ?, ?, engine_version, packages, seed, summary, last_seq, current_branch, pending_fork_branch, pending_fork_seq, pending_fork_turn FROM saves WHERE slot_id=?`,
			`INSERT INTO branches(slot_id,branch_id,name,parent_branch,fork_seq,fork_turn,rng_salt,status,created_at,head_seq,head_turn)
			 SELECT ?,branch_id,name,parent_branch,fork_seq,fork_turn,rng_salt,status,created_at,head_seq,head_turn FROM branches WHERE slot_id=?`,
			`INSERT INTO events(slot_id,branch_id,seq,command_id,turn,minute,type,data) SELECT ?,branch_id,seq,command_id,turn,minute,type,data FROM events WHERE slot_id=?`,
			`INSERT INTO commands(slot_id,branch_id,command_id,accepted,command,result,narration,created_at) SELECT ?,branch_id,command_id,accepted,command,result,narration,created_at FROM commands WHERE slot_id=?`,
			`INSERT INTO snapshots(slot_id,branch_id,seq,state) SELECT ?,branch_id,seq,state FROM snapshots WHERE slot_id=?`,
			`INSERT INTO transcript(slot_id,branch_id,command_id,turn,kind,text,meta) SELECT ?,branch_id,command_id,turn,kind,text,meta FROM transcript WHERE slot_id=? ORDER BY id`,
			`INSERT INTO memory(slot_id,branch_id,kind,subject,from_turn,to_turn,text,compressed) SELECT ?,branch_id,kind,subject,from_turn,to_turn,text,compressed FROM memory WHERE slot_id=? ORDER BY id`,
			`INSERT INTO checkpoints(slot_id,id,branch_id,seq,turn,name,kind,reason,created_at) SELECT ?,id,branch_id,seq,turn,name,kind,reason,created_at FROM checkpoints WHERE slot_id=?`,
		}
		if _, err := tx.ExecContext(ctx, qs[0], dst, name, now, now, src); err != nil {
			return err
		}
		for _, q := range qs[1:] {
			if _, err := tx.ExecContext(ctx, q, dst, src); err != nil {
				return err
			}
		}
		return nil
	})
	return dst, err
}

// Commit 是一次回合提交的全部内容：要么全部写入，要么全部不写（第 51 节）。
type Commit struct {
	CommandID string
	Accepted  bool
	Command   any
	Result    any
	Events    []event.Event
	After     *state.State // 执行后的状态（用于快照）
	Entries   []Entry      // 玩家输入、检定、系统消息等（叙事稍后追加）
	Summary   Summary
	// Narration 非 nil 时同时写入叙事（例如被拒绝的命令直接给出系统回复）。
	Narration *string
	// ForceSnapshot 让本次提交无论间隔都写快照（新分支头部、导入）。
	ForceSnapshot bool
}

// CommitTurn 在单个事务里向当前分支追加事件、记录命令、写对话记录，并按需写快照。
// 同一 command_id 第二次提交返回 ErrDuplicateCommand，且不写入任何内容。
func (s *Store) CommitTurn(ctx context.Context, slotID string, c Commit) error {
	cmdJSON, err := json.Marshal(c.Command)
	if err != nil {
		return err
	}
	resJSON, err := json.Marshal(c.Result)
	if err != nil {
		return err
	}
	sum, _ := json.Marshal(c.Summary)
	now := s.now().UnixMilli()
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM commands WHERE slot_id=? AND command_id=?`, slotID, c.CommandID).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return ErrDuplicateCommand
		}
		branch, err := s.branchOf(ctx, tx, slotID)
		if err != nil {
			return err
		}
		var lastSeq int64
		var headTurn int
		if err := tx.QueryRowContext(ctx, `SELECT head_seq, head_turn FROM branches WHERE slot_id=? AND branch_id=?`, slotID, branch).Scan(&lastSeq, &headTurn); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		for _, e := range c.Events {
			if e.Seq != lastSeq+1 {
				return fmt.Errorf("event seq %d is not contiguous with %d", e.Seq, lastSeq)
			}
			data, err := json.Marshal(e.Data)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO events(slot_id,branch_id,seq,command_id,turn,minute,type,data) VALUES (?,?,?,?,?,?,?,?)`,
				slotID, branch, e.Seq, e.CommandID, e.Turn, e.Minute, e.Type, string(data)); err != nil {
				return err
			}
			lastSeq = e.Seq
			headTurn = max(headTurn, e.Turn)
		}
		var narr any
		if c.Narration != nil {
			narr = *c.Narration
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO commands(slot_id,branch_id,command_id,accepted,command,result,narration,created_at) VALUES (?,?,?,?,?,?,?,?)`,
			slotID, branch, c.CommandID, boolInt(c.Accepted), string(cmdJSON), string(resJSON), narr, now); err != nil {
			return err
		}
		for _, en := range c.Entries {
			if err := insertEntry(ctx, tx, slotID, branch, c.CommandID, en); err != nil {
				return err
			}
		}
		if len(c.Events) > 0 && c.After != nil {
			var snapSeq int64
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM snapshots WHERE slot_id=? AND branch_id=?`, slotID, branch).Scan(&snapSeq); err != nil {
				return err
			}
			if c.ForceSnapshot || lastSeq-snapSeq >= SnapshotInterval {
				st, err := c.After.Marshal()
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO snapshots(slot_id,branch_id,seq,state) VALUES (?,?,?,?)`, slotID, branch, lastSeq, string(st)); err != nil {
					return err
				}
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE branches SET head_seq=?, head_turn=? WHERE slot_id=? AND branch_id=?`, lastSeq, headTurn, slotID, branch); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE saves SET updated_at=?, summary=?, last_seq=? WHERE slot_id=?`, now, string(sum), lastSeq, slotID)
		return err
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func insertEntry(ctx context.Context, tx *sql.Tx, slotID, branch, cmdID string, en Entry) error {
	var meta any
	if len(en.Meta) > 0 {
		meta = string(en.Meta)
	}
	if en.CommandID == "" {
		en.CommandID = cmdID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO transcript(slot_id,branch_id,command_id,turn,kind,text,meta) VALUES (?,?,?,?,?,?,?)`,
		slotID, branch, en.CommandID, en.Turn, en.Kind, en.Text, meta)
	return err
}

// SetNarration 为已提交的命令补写叙事（叙事失败/降级不会影响已提交事件）。
// 已有叙事时不覆盖，保证重试不会产生重复记录。叙事写到命令所在的分支。
func (s *Store) SetNarration(ctx context.Context, slotID, cmdID, text string, entry Entry) error {
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var branch string
		if err := tx.QueryRowContext(ctx, `SELECT branch_id FROM commands WHERE slot_id=? AND command_id=?`, slotID, cmdID).Scan(&branch); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		}
		res, err := tx.ExecContext(ctx, `UPDATE commands SET narration=? WHERE slot_id=? AND command_id=? AND narration IS NULL`, text, slotID, cmdID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		entry.Text = text
		return insertEntry(ctx, tx, slotID, branch, cmdID, entry)
	})
}

// AppendEntries 向当前分支追加对话记录（例如新游戏开场白、系统提示）。
func (s *Store) AppendEntries(ctx context.Context, slotID string, entries []Entry) error {
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		branch, err := s.branchOf(ctx, tx, slotID)
		if err != nil {
			return err
		}
		for _, en := range entries {
			if err := insertEntry(ctx, tx, slotID, branch, en.CommandID, en); err != nil {
				return err
			}
		}
		return nil
	})
}

// LookupCommand 查询已执行的命令（任意分支）。
func (s *Store) LookupCommand(ctx context.Context, slotID, cmdID string) (CommandRecord, bool, error) {
	var r CommandRecord
	var acc int
	var res string
	var narr sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT command_id, accepted, result, narration FROM commands WHERE slot_id=? AND command_id=?`, slotID, cmdID).
		Scan(&r.CommandID, &acc, &res, &narr)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	r.Accepted = acc == 1
	r.Result = json.RawMessage(res)
	r.Narration, r.HasNarr = narr.String, narr.Valid
	return r, true, nil
}

// PendingNarrations 返回当前分支上已提交但缺少叙事的命令（例如叙事期间应用被杀）。
func (s *Store) PendingNarrations(ctx context.Context, slotID string) ([]CommandRecord, error) {
	branch, err := s.branchOf(ctx, s.db, slotID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT command_id, accepted, result FROM commands WHERE slot_id=? AND branch_id=? AND narration IS NULL ORDER BY created_at, rowid`, slotID, branch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []CommandRecord
	for rows.Next() {
		var r CommandRecord
		var acc int
		var res string
		if err := rows.Scan(&r.CommandID, &acc, &res); err != nil {
			return nil, err
		}
		r.Accepted, r.Result = acc == 1, json.RawMessage(res)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Load 恢复存档当前分支的头部状态：最近快照 + 之后的全部事件。
func (s *Store) Load(ctx context.Context, slotID string) (*state.State, error) {
	branch, err := s.branchOf(ctx, s.db, slotID)
	if err != nil {
		return nil, err
	}
	return s.BranchStateAt(ctx, slotID, branch, -1)
}

// Initial 返回存档的初始状态（seq 0 快照），用于完整 Replay 校验。
func (s *Store) Initial(ctx context.Context, slotID string) (*state.State, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT state FROM snapshots WHERE slot_id=? AND branch_id=? AND seq=0`, slotID, MainBranch).Scan(&raw); err != nil {
		return nil, err
	}
	return state.Unmarshal([]byte(raw))
}

// Events 返回当前分支上 seq > after 的事件；limit<=0 表示不限。
func (s *Store) Events(ctx context.Context, slotID string, after int64, limit int) ([]event.Event, error) {
	branch, err := s.branchOf(ctx, s.db, slotID)
	if err != nil {
		return nil, err
	}
	return s.BranchEvents(ctx, slotID, branch, after, limit)
}

// BranchEvents 返回指定分支上 seq > after 的事件。
func (s *Store) BranchEvents(ctx context.Context, slotID, branch string, after int64, limit int) ([]event.Event, error) {
	q := `SELECT seq, command_id, turn, minute, type, data FROM events WHERE slot_id=? AND branch_id=? AND seq>? ORDER BY seq`
	args := []any{slotID, branch, after}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []event.Event
	for rows.Next() {
		var e event.Event
		var data string
		if err := rows.Scan(&e.Seq, &e.CommandID, &e.Turn, &e.Minute, &e.Type, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Transcript 返回当前分支最近 limit 条对话记录（按时间正序）；beforeID>0 时用于向前翻页。
// 存在待定回滚（pending fork）时只返回分叉点（回合）之前的记录。
func (s *Store) Transcript(ctx context.Context, slotID string, limit int, beforeID int64) ([]Entry, error) {
	sl, err := s.GetSlot(ctx, slotID)
	if err != nil {
		return nil, err
	}
	maxTurn := -1
	if sl.PendingFork.Branch != "" {
		maxTurn = sl.PendingFork.Turn
	}
	return s.BranchTranscript(ctx, slotID, sl.Branch, maxTurn, limit, beforeID)
}

// BranchTranscript 返回指定分支的对话记录；maxTurn>=0 时只返回 turn<=maxTurn 的记录。
func (s *Store) BranchTranscript(ctx context.Context, slotID, branch string, maxTurn, limit int, beforeID int64) ([]Entry, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `SELECT id, command_id, turn, kind, text, meta FROM transcript WHERE slot_id=? AND branch_id=?`
	args := []any{slotID, branch}
	if beforeID > 0 {
		q += ` AND id<?`
		args = append(args, beforeID)
	}
	if maxTurn >= 0 {
		q += ` AND turn<=?`
		args = append(args, maxTurn)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out, err := scanEntries(rows)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var e Entry
		var meta sql.NullString
		if err := rows.Scan(&e.ID, &e.CommandID, &e.Turn, &e.Kind, &e.Text, &meta); err != nil {
			return nil, err
		}
		if meta.Valid {
			e.Meta = json.RawMessage(meta.String)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// StateAt 恢复当前分支到 seq（含）时的状态。
func (s *Store) StateAt(ctx context.Context, slotID string, seq int64) (*state.State, error) {
	branch, err := s.branchOf(ctx, s.db, slotID)
	if err != nil {
		return nil, err
	}
	return s.BranchStateAt(ctx, slotID, branch, seq)
}

// BranchStateAt 恢复指定分支到 seq（含）时的状态；seq<0 表示分支头部。
// 每个分支在分叉时复制了祖先的事件与快照，因此只需读本分支（等价于沿谱系重放）。
func (s *Store) BranchStateAt(ctx context.Context, slotID, branch string, seq int64) (*state.State, error) {
	q := `SELECT seq, state FROM snapshots WHERE slot_id=? AND branch_id=? ORDER BY seq DESC LIMIT 1`
	args := []any{slotID, branch}
	if seq >= 0 {
		q = `SELECT seq, state FROM snapshots WHERE slot_id=? AND branch_id=? AND seq<=? ORDER BY seq DESC LIMIT 1`
		args = append(args, seq)
	}
	var snap int64
	var raw string
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&snap, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	st, err := state.Unmarshal([]byte(raw))
	if err != nil {
		return nil, err
	}
	limit := 0
	if seq >= 0 {
		limit = int(seq - snap)
		if limit == 0 {
			return st, nil
		}
	}
	evs, err := s.BranchEvents(ctx, slotID, branch, snap, limit)
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		if err := state.Apply(st, e); err != nil {
			return nil, fmt.Errorf("replay seq %d: %w", e.Seq, err)
		}
	}
	return st, nil
}

// EntriesByCommand 返回某条命令产生的对话记录。
func (s *Store) EntriesByCommand(ctx context.Context, slotID, cmdID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, command_id, turn, kind, text, meta FROM transcript WHERE slot_id=? AND (command_id=? OR command_id LIKE ?) AND branch_id=(SELECT current_branch FROM saves WHERE slot_id=?) ORDER BY id`, slotID, cmdID, cmdID+":%", slotID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanEntries(rows)
}

// Memory 是记忆 Agent 写入的一条摘要（滚动对话摘要 / 长期记忆 / NPC 记忆摘要）。
// 摘要不是游戏事实：它只帮助 AI 在上下文被压缩后回忆早期内容，删除它不影响重放。
type Memory struct {
	ID         int64    `json:"id"`
	Kind       string   `json:"kind"` // rolling / longterm / npc
	Subject    string   `json:"subject,omitempty"`
	FromTurn   int      `json:"from_turn"`
	ToTurn     int      `json:"to_turn"`
	Text       string   `json:"text"`
	Compressed []string `json:"compressed,omitempty"`
}

// Memories 按写入顺序返回存档当前分支的全部摘要。
func (s *Store) Memories(ctx context.Context, slotID string) ([]Memory, error) {
	branch, err := s.branchOf(ctx, s.db, slotID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, subject, from_turn, to_turn, text, compressed FROM memory WHERE slot_id=? AND branch_id=? ORDER BY id`, slotID, branch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Memory
	for rows.Next() {
		var m Memory
		var comp string
		if err := rows.Scan(&m.ID, &m.Kind, &m.Subject, &m.FromTurn, &m.ToTurn, &m.Text, &comp); err != nil {
			return nil, err
		}
		if comp != "" {
			m.Compressed = strings.Split(comp, "\x1f")
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceMemories 在一个事务里删除 ids 指定的摘要并向当前分支写入 add（长期压缩：旧的滚动摘要合并为一条）。
func (s *Store) ReplaceMemories(ctx context.Context, slotID string, remove []int64, add []Memory) error {
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		branch, err := s.branchOf(ctx, tx, slotID)
		if err != nil {
			return err
		}
		for _, id := range remove {
			if _, err := tx.ExecContext(ctx, `DELETE FROM memory WHERE slot_id=? AND branch_id=? AND id=?`, slotID, branch, id); err != nil {
				return err
			}
		}
		for _, m := range add {
			if _, err := tx.ExecContext(ctx, `INSERT INTO memory(slot_id,branch_id,kind,subject,from_turn,to_turn,text,compressed) VALUES (?,?,?,?,?,?,?,?)`,
				slotID, branch, m.Kind, m.Subject, m.FromTurn, m.ToTurn, m.Text, strings.Join(m.Compressed, "\x1f")); err != nil {
				return err
			}
		}
		return nil
	})
}
