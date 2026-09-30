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

// schemaVersion 是存档数据库结构版本。
const schemaVersion = 1

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
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS saves (
			slot_id TEXT PRIMARY KEY, name TEXT NOT NULL, player_name TEXT NOT NULL,
			created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL, engine_version TEXT NOT NULL,
			packages TEXT NOT NULL, seed INTEGER NOT NULL, summary TEXT NOT NULL, last_seq INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS events (
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE,
			seq INTEGER NOT NULL, command_id TEXT NOT NULL, turn INTEGER NOT NULL, minute INTEGER NOT NULL,
			type TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY (slot_id, seq))`,
		`CREATE TABLE IF NOT EXISTS commands (
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE,
			command_id TEXT NOT NULL, accepted INTEGER NOT NULL, command TEXT NOT NULL, result TEXT NOT NULL,
			narration TEXT, created_at INTEGER NOT NULL, PRIMARY KEY (slot_id, command_id))`,
		`CREATE TABLE IF NOT EXISTS snapshots (
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE,
			seq INTEGER NOT NULL, state TEXT NOT NULL, PRIMARY KEY (slot_id, seq))`,
		`CREATE TABLE IF NOT EXISTS transcript (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE,
			command_id TEXT NOT NULL DEFAULT '', turn INTEGER NOT NULL, kind TEXT NOT NULL, text TEXT NOT NULL, meta TEXT)`,
		`CREATE INDEX IF NOT EXISTS transcript_slot ON transcript(slot_id, id)`,
		`CREATE TABLE IF NOT EXISTS memory (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slot_id TEXT NOT NULL REFERENCES saves(slot_id) ON DELETE CASCADE,
			kind TEXT NOT NULL, subject TEXT NOT NULL DEFAULT '', from_turn INTEGER NOT NULL, to_turn INTEGER NOT NULL,
			text TEXT NOT NULL, compressed TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS memory_slot ON memory(slot_id, kind, to_turn)`,
		`INSERT INTO meta(key, value) VALUES ('schema_version', '1') ON CONFLICT(key) DO NOTHING`,
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

// CreateSlot 创建存档并写入初始快照（seq 0）。
func (s *Store) CreateSlot(ctx context.Context, slot Slot, initial *state.State) error {
	pk, _ := json.Marshal(slot.Packages)
	sum, _ := json.Marshal(slot.Summary)
	st, err := initial.Marshal()
	if err != nil {
		return err
	}
	now := s.now().UnixMilli()
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO saves(slot_id,name,player_name,created_at,updated_at,engine_version,packages,seed,summary,last_seq)
			VALUES (?,?,?,?,?,?,?,?,?,0)`, slot.ID, slot.Name, slot.PlayerName, now, now, slot.EngineVersion, string(pk), int64(slot.Seed), string(sum)); err != nil { //nolint:gosec // seed 以 int64 位模式存储
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO snapshots(slot_id, seq, state) VALUES (?, 0, ?)`, slot.ID, string(st))
		return err
	})
}

func scanSlot(sc interface{ Scan(...any) error }) (Slot, error) {
	var sl Slot
	var pk, sum string
	var seed int64
	if err := sc.Scan(&sl.ID, &sl.Name, &sl.PlayerName, &sl.CreatedAt, &sl.UpdatedAt, &sl.EngineVersion, &pk, &seed, &sum, &sl.LastSeq); err != nil {
		return sl, err
	}
	sl.Seed = uint64(seed) //nolint:gosec // 位模式还原
	_ = json.Unmarshal([]byte(pk), &sl.Packages)
	_ = json.Unmarshal([]byte(sum), &sl.Summary)
	return sl, nil
}

const slotCols = `slot_id,name,player_name,created_at,updated_at,engine_version,packages,seed,summary,last_seq`

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

// CopySlot 复制存档（事件、快照、命令、对话记录全部复制），返回新存档 ID。
func (s *Store) CopySlot(ctx context.Context, src, name string) (string, error) {
	if _, err := s.GetSlot(ctx, src); err != nil {
		return "", err
	}
	dst := NewSlotID()
	now := s.now().UnixMilli()
	err := sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		qs := []string{
			`INSERT INTO saves(slot_id,name,player_name,created_at,updated_at,engine_version,packages,seed,summary,last_seq)
			 SELECT ?, ?, player_name, ?, ?, engine_version, packages, seed, summary, last_seq FROM saves WHERE slot_id=?`,
			`INSERT INTO events(slot_id,seq,command_id,turn,minute,type,data) SELECT ?,seq,command_id,turn,minute,type,data FROM events WHERE slot_id=?`,
			`INSERT INTO commands(slot_id,command_id,accepted,command,result,narration,created_at) SELECT ?,command_id,accepted,command,result,narration,created_at FROM commands WHERE slot_id=?`,
			`INSERT INTO snapshots(slot_id,seq,state) SELECT ?,seq,state FROM snapshots WHERE slot_id=?`,
			`INSERT INTO transcript(slot_id,command_id,turn,kind,text,meta) SELECT ?,command_id,turn,kind,text,meta FROM transcript WHERE slot_id=? ORDER BY id`,
			`INSERT INTO memory(slot_id,kind,subject,from_turn,to_turn,text,compressed) SELECT ?,kind,subject,from_turn,to_turn,text,compressed FROM memory WHERE slot_id=? ORDER BY id`,
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
}

// CommitTurn 在单个事务里追加事件、记录命令、写对话记录，并按需写快照。
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
		var lastSeq int64
		if err := tx.QueryRowContext(ctx, `SELECT last_seq FROM saves WHERE slot_id=?`, slotID).Scan(&lastSeq); err != nil {
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
			if _, err := tx.ExecContext(ctx, `INSERT INTO events(slot_id,seq,command_id,turn,minute,type,data) VALUES (?,?,?,?,?,?,?)`,
				slotID, e.Seq, e.CommandID, e.Turn, e.Minute, e.Type, string(data)); err != nil {
				return err
			}
			lastSeq = e.Seq
		}
		var narr any
		if c.Narration != nil {
			narr = *c.Narration
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO commands(slot_id,command_id,accepted,command,result,narration,created_at) VALUES (?,?,?,?,?,?,?)`,
			slotID, c.CommandID, boolInt(c.Accepted), string(cmdJSON), string(resJSON), narr, now); err != nil {
			return err
		}
		for _, en := range c.Entries {
			if err := insertEntry(ctx, tx, slotID, c.CommandID, en); err != nil {
				return err
			}
		}
		if len(c.Events) > 0 && c.After != nil {
			var snapSeq int64
			if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq),0) FROM snapshots WHERE slot_id=?`, slotID).Scan(&snapSeq); err != nil {
				return err
			}
			if lastSeq-snapSeq >= SnapshotInterval {
				st, err := c.After.Marshal()
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `INSERT INTO snapshots(slot_id,seq,state) VALUES (?,?,?)`, slotID, lastSeq, string(st)); err != nil {
					return err
				}
			}
		}
		_, err := tx.ExecContext(ctx, `UPDATE saves SET updated_at=?, summary=?, last_seq=? WHERE slot_id=?`, now, string(sum), lastSeq, slotID)
		return err
	})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func insertEntry(ctx context.Context, tx *sql.Tx, slotID, cmdID string, en Entry) error {
	var meta any
	if len(en.Meta) > 0 {
		meta = string(en.Meta)
	}
	if en.CommandID == "" {
		en.CommandID = cmdID
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO transcript(slot_id,command_id,turn,kind,text,meta) VALUES (?,?,?,?,?,?)`,
		slotID, en.CommandID, en.Turn, en.Kind, en.Text, meta)
	return err
}

// SetNarration 为已提交的命令补写叙事（叙事失败/降级不会影响已提交事件）。
// 已有叙事时不覆盖，保证重试不会产生重复记录。
func (s *Store) SetNarration(ctx context.Context, slotID, cmdID, text string, entry Entry) error {
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE commands SET narration=? WHERE slot_id=? AND command_id=? AND narration IS NULL`, text, slotID, cmdID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil
		}
		entry.Text = text
		return insertEntry(ctx, tx, slotID, cmdID, entry)
	})
}

// AppendEntries 追加对话记录（例如新游戏开场白）。
func (s *Store) AppendEntries(ctx context.Context, slotID string, entries []Entry) error {
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, en := range entries {
			if err := insertEntry(ctx, tx, slotID, en.CommandID, en); err != nil {
				return err
			}
		}
		return nil
	})
}

// LookupCommand 查询已执行的命令。
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

// PendingNarrations 返回已提交但缺少叙事的命令（例如叙事期间应用被杀）。
func (s *Store) PendingNarrations(ctx context.Context, slotID string) ([]CommandRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT command_id, accepted, result FROM commands WHERE slot_id=? AND narration IS NULL ORDER BY created_at, rowid`, slotID)
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

// Load 恢复存档状态：最近快照 + 之后的全部事件。
func (s *Store) Load(ctx context.Context, slotID string) (*state.State, error) {
	var seq int64
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT seq, state FROM snapshots WHERE slot_id=? ORDER BY seq DESC LIMIT 1`, slotID).Scan(&seq, &raw)
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
	evs, err := s.Events(ctx, slotID, seq, 0)
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

// Initial 返回存档的初始状态（seq 0 快照），用于完整 Replay 校验。
func (s *Store) Initial(ctx context.Context, slotID string) (*state.State, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT state FROM snapshots WHERE slot_id=? AND seq=0`, slotID).Scan(&raw); err != nil {
		return nil, err
	}
	return state.Unmarshal([]byte(raw))
}

// Events 返回 seq > after 的事件；limit<=0 表示不限。
func (s *Store) Events(ctx context.Context, slotID string, after int64, limit int) ([]event.Event, error) {
	q := `SELECT seq, command_id, turn, minute, type, data FROM events WHERE slot_id=? AND seq>? ORDER BY seq`
	args := []any{slotID, after}
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

// Transcript 返回最近 limit 条对话记录（按时间正序）；beforeID>0 时用于向前翻页。
func (s *Store) Transcript(ctx context.Context, slotID string, limit int, beforeID int64) ([]Entry, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `SELECT id, command_id, turn, kind, text, meta FROM transcript WHERE slot_id=?`
	args := []any{slotID}
	if beforeID > 0 {
		q += ` AND id<?`
		args = append(args, beforeID)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// StateAt 恢复到 seq（含）时的状态：不晚于 seq 的最近快照 + 之后到 seq 的事件。
func (s *Store) StateAt(ctx context.Context, slotID string, seq int64) (*state.State, error) {
	var snap int64
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT seq, state FROM snapshots WHERE slot_id=? AND seq<=? ORDER BY seq DESC LIMIT 1`, slotID, seq).Scan(&snap, &raw)
	if err != nil {
		return nil, err
	}
	st, err := state.Unmarshal([]byte(raw))
	if err != nil {
		return nil, err
	}
	evs, err := s.Events(ctx, slotID, snap, int(seq-snap))
	if err != nil {
		return nil, err
	}
	for _, e := range evs {
		if err := state.Apply(st, e); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// EntriesByCommand 返回某条命令产生的对话记录。
func (s *Store) EntriesByCommand(ctx context.Context, slotID, cmdID string) ([]Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, command_id, turn, kind, text, meta FROM transcript WHERE slot_id=? AND command_id=? ORDER BY id`, slotID, cmdID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

// Memories 按写入顺序返回存档的全部摘要。
func (s *Store) Memories(ctx context.Context, slotID string) ([]Memory, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, subject, from_turn, to_turn, text, compressed FROM memory WHERE slot_id=? ORDER BY id`, slotID)
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

// ReplaceMemories 在一个事务里删除 ids 指定的摘要并写入 add（长期压缩：旧的滚动摘要合并为一条）。
func (s *Store) ReplaceMemories(ctx context.Context, slotID string, remove []int64, add []Memory) error {
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, id := range remove {
			if _, err := tx.ExecContext(ctx, `DELETE FROM memory WHERE slot_id=? AND id=?`, slotID, id); err != nil {
				return err
			}
		}
		for _, m := range add {
			if _, err := tx.ExecContext(ctx, `INSERT INTO memory(slot_id,kind,subject,from_turn,to_turn,text,compressed) VALUES (?,?,?,?,?,?,?)`,
				slotID, m.Kind, m.Subject, m.FromTurn, m.ToTurn, m.Text, strings.Join(m.Compressed, "\x1f")); err != nil {
				return err
			}
		}
		return nil
	})
}
