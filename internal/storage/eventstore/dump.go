package eventstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/storage/sqlite"
)

// Dump 是一个存档的完整导出内容（存档导出 / 导入使用，第 10.6 节）。不含任何密钥或服务商配置。
type Dump struct {
	Slot        Slot             `json:"slot"`
	Branches    []Branch         `json:"branches"`
	Events      []BranchEvent    `json:"-"`
	Snapshots   []BranchSnapshot `json:"-"`
	Transcript  []BranchEntry    `json:"-"`
	Memory      []BranchMemory   `json:"-"`
	Commands    []BranchCommand  `json:"-"`
	Checkpoints []Checkpoint     `json:"checkpoints"`
	AICalls     []AICall         `json:"-"`
}

// BranchEvent 是带分支的事件行。
type BranchEvent struct {
	Branch string `json:"branch"`
	event.Event
}

// BranchSnapshot 是带分支的快照行。
type BranchSnapshot struct {
	Branch string          `json:"branch"`
	Seq    int64           `json:"seq"`
	State  json.RawMessage `json:"state"`
}

// BranchEntry 是带分支的对话记录。
type BranchEntry struct {
	Branch string `json:"branch"`
	Entry
}

// BranchMemory 是带分支的记忆摘要。
type BranchMemory struct {
	Branch string `json:"branch"`
	Memory
}

// BranchCommand 是带分支的命令记录。
type BranchCommand struct {
	Branch    string          `json:"branch"`
	CommandID string          `json:"command_id"`
	Accepted  bool            `json:"accepted"`
	Command   json.RawMessage `json:"command"`
	Result    json.RawMessage `json:"result"`
	Narration *string         `json:"narration,omitempty"`
	Created   int64           `json:"created_at"`
}

// DumpSlot 导出存档。onlyBranch 非空时只导出该分支；withCalls 附带 AI 调用记录（调试包）。
// 快照只导出每个分支的 seq 0 与最新一个（导入后重放校验）。
func (s *Store) DumpSlot(ctx context.Context, slotID, onlyBranch string, withCalls bool) (*Dump, error) {
	sl, err := s.GetSlot(ctx, slotID)
	if err != nil {
		return nil, err
	}
	d := &Dump{Slot: sl}
	bs, err := s.Branches(ctx, slotID)
	if err != nil {
		return nil, err
	}
	for _, b := range bs {
		if onlyBranch != "" && b.ID != onlyBranch {
			continue
		}
		d.Branches = append(d.Branches, b)
	}
	keep := map[string]bool{}
	for _, b := range d.Branches {
		keep[b.ID] = true
	}
	for _, b := range d.Branches {
		evs, err := s.BranchEvents(ctx, slotID, b.ID, 0, 0)
		if err != nil {
			return nil, err
		}
		for _, e := range evs {
			d.Events = append(d.Events, BranchEvent{Branch: b.ID, Event: e})
		}
		rows, err := s.db.QueryContext(ctx, `SELECT seq, state FROM snapshots WHERE slot_id=? AND branch_id=? AND (seq=0 OR seq=(SELECT MAX(seq) FROM snapshots WHERE slot_id=? AND branch_id=?)) ORDER BY seq`, slotID, b.ID, slotID, b.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var sn BranchSnapshot
			var raw string
			if err := rows.Scan(&sn.Seq, &raw); err != nil {
				_ = rows.Close()
				return nil, err
			}
			sn.Branch, sn.State = b.ID, json.RawMessage(raw)
			d.Snapshots = append(d.Snapshots, sn)
		}
		_ = rows.Close()
		tr, err := s.BranchTranscript(ctx, slotID, b.ID, -1, 1<<30, 0)
		if err != nil {
			return nil, err
		}
		for _, e := range tr {
			d.Transcript = append(d.Transcript, BranchEntry{Branch: b.ID, Entry: e})
		}
	}
	mrows, err := s.db.QueryContext(ctx, `SELECT branch_id, id, kind, subject, from_turn, to_turn, text, compressed FROM memory WHERE slot_id=? ORDER BY id`, slotID)
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		var m BranchMemory
		var comp string
		if err := mrows.Scan(&m.Branch, &m.ID, &m.Kind, &m.Subject, &m.FromTurn, &m.ToTurn, &m.Text, &comp); err != nil {
			_ = mrows.Close()
			return nil, err
		}
		if comp != "" {
			m.Compressed = strings.Split(comp, "\x1f")
		}
		if keep[m.Branch] {
			d.Memory = append(d.Memory, m)
		}
	}
	_ = mrows.Close()
	crows, err := s.db.QueryContext(ctx, `SELECT branch_id, command_id, accepted, command, result, narration, created_at FROM commands WHERE slot_id=? ORDER BY created_at, rowid`, slotID)
	if err != nil {
		return nil, err
	}
	for crows.Next() {
		var c BranchCommand
		var acc int
		var cmd, res string
		var narr sql.NullString
		if err := crows.Scan(&c.Branch, &c.CommandID, &acc, &cmd, &res, &narr, &c.Created); err != nil {
			_ = crows.Close()
			return nil, err
		}
		c.Accepted, c.Command, c.Result = acc == 1, json.RawMessage(cmd), json.RawMessage(res)
		if narr.Valid {
			v := narr.String
			c.Narration = &v
		}
		if keep[c.Branch] {
			d.Commands = append(d.Commands, c)
		}
	}
	_ = crows.Close()
	cps, err := s.Checkpoints(ctx, slotID)
	if err != nil {
		return nil, err
	}
	for _, c := range cps {
		if keep[c.Branch] {
			d.Checkpoints = append(d.Checkpoints, c)
		}
	}
	if withCalls {
		calls, err := s.AICalls(ctx, slotID, onlyBranch, 0)
		if err != nil {
			return nil, err
		}
		d.AICalls = calls
	}
	if onlyBranch != "" {
		d.Slot.Branch = onlyBranch
		d.Slot.PendingFork = ForkRef{}
	}
	return d, nil
}

// RestoreSlot 把导出内容写成一个新存档（新 slot ID，不覆盖已有存档）。整个过程在一个事务里，失败不留残留。
func (s *Store) RestoreSlot(ctx context.Context, d *Dump, newID, name string) error {
	pk, _ := json.Marshal(d.Slot.Packages)
	sum, _ := json.Marshal(d.Slot.Summary)
	now := s.now().UnixMilli()
	return sqlite.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		var head int64
		for _, b := range d.Branches {
			if b.ID == d.Slot.Branch {
				head = b.HeadSeq
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO saves(slot_id,name,player_name,created_at,updated_at,engine_version,packages,seed,summary,last_seq,current_branch,flags)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, newID, name, d.Slot.PlayerName, now, now, d.Slot.EngineVersion, string(pk), int64(d.Slot.Seed), string(sum), head, d.Slot.Branch, d.Slot.Flags); err != nil { //nolint:gosec // 位模式
			return err
		}
		for _, b := range d.Branches {
			parent := b.Parent
			if _, err := tx.ExecContext(ctx, `INSERT INTO branches(slot_id,branch_id,name,parent_branch,fork_seq,fork_turn,rng_salt,status,created_at,head_seq,head_turn) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
				newID, b.ID, b.Name, parent, b.ForkSeq, b.ForkTurn, int64(b.Salt), b.Status, b.Created, b.HeadSeq, b.HeadTurn); err != nil { //nolint:gosec // 位模式
				return err
			}
		}
		for _, e := range d.Events {
			data, err := json.Marshal(e.Data)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO events(slot_id,branch_id,seq,command_id,turn,minute,type,data) VALUES (?,?,?,?,?,?,?,?)`,
				newID, e.Branch, e.Seq, e.CommandID, e.Turn, e.Minute, e.Type, string(data)); err != nil {
				return err
			}
		}
		for _, sn := range d.Snapshots {
			if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO snapshots(slot_id,branch_id,seq,state) VALUES (?,?,?,?)`, newID, sn.Branch, sn.Seq, string(sn.State)); err != nil {
				return err
			}
		}
		for _, e := range d.Transcript {
			if err := insertEntry(ctx, tx, newID, e.Branch, e.CommandID, e.Entry); err != nil {
				return err
			}
		}
		for _, m := range d.Memory {
			if _, err := tx.ExecContext(ctx, `INSERT INTO memory(slot_id,branch_id,kind,subject,from_turn,to_turn,text,compressed) VALUES (?,?,?,?,?,?,?,?)`,
				newID, m.Branch, m.Kind, m.Subject, m.FromTurn, m.ToTurn, m.Text, strings.Join(m.Compressed, "\x1f")); err != nil {
				return err
			}
		}
		for _, c := range d.Commands {
			var narr any
			if c.Narration != nil {
				narr = *c.Narration
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO commands(slot_id,branch_id,command_id,accepted,command,result,narration,created_at) VALUES (?,?,?,?,?,?,?,?)`,
				newID, c.Branch, c.CommandID, boolInt(c.Accepted), string(c.Command), string(c.Result), narr, c.Created); err != nil {
				return err
			}
		}
		for _, c := range d.Checkpoints {
			if _, err := tx.ExecContext(ctx, `INSERT INTO checkpoints(slot_id,id,branch_id,seq,turn,name,kind,reason,created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
				newID, c.ID, c.Branch, c.Seq, c.Turn, c.Name, c.Kind, c.Reason, c.Created); err != nil {
				return err
			}
		}
		return nil
	})
}
