package eventstore_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/packages"
)

func TestCommitLoadSnapshotIdempotency(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "saves.db")
	st, err := eventstore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	ev, _ := expression.New()
	p, err := loader.Load(packages.Demo(), ev)
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.New(p, ev)
	s := state.New(p, 7, "测试者")
	slot := eventstore.Slot{ID: eventstore.NewSlotID(), Name: "存档一", PlayerName: "测试者", EngineVersion: "test", Seed: 7}
	if err := st.CreateSlot(ctx, slot, s); err != nil {
		t.Fatal(err)
	}
	// 执行足够多的回合以触发快照
	for i := 0; i < 30; i++ {
		c := command.Command{ID: fmt.Sprintf("c%d", i), Kind: command.KindAction, Action: "demo:action/look"}
		if i%3 == 1 {
			c = command.Command{ID: fmt.Sprintf("c%d", i), Kind: command.KindAction, Action: "demo:action/persuade", Target: "demo:character/lena"}
		}
		res, ns, err := eng.Execute(s, c)
		if err != nil || !res.Accepted {
			t.Fatalf("%d: %v %+v", i, err, res)
		}
		if err := st.CommitTurn(ctx, slot.ID, eventstore.Commit{CommandID: c.ID, Accepted: true, Command: c, Result: res, Events: res.Events, After: ns,
			Entries: []eventstore.Entry{{Kind: "player", Text: "环顾四周", Turn: res.Turn}}}); err != nil {
			t.Fatal(err)
		}
		s = ns
		// 同一 command_id 再次提交必须被拒绝且不写入
		if err := st.CommitTurn(ctx, slot.ID, eventstore.Commit{CommandID: c.ID, Accepted: true, Command: c, Result: res, Events: res.Events, After: ns}); !errors.Is(err, eventstore.ErrDuplicateCommand) {
			t.Fatalf("duplicate commit: %v", err)
		}
	}
	_ = st.Close()

	// 重新打开（模拟退出后恢复）
	st, err = eventstore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	loaded, err := st.Load(ctx, slot.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := loaded.Marshal()
	b, _ := s.Marshal()
	if !bytes.Equal(a, b) {
		t.Fatal("loaded state differs from live state")
	}
	// 完整 Replay（初始快照 + 全部事件）同样一致
	init, err := st.Initial(ctx, slot.ID)
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := st.Events(ctx, slot.ID, 0, 0)
	for _, e := range evs {
		if err := state.Apply(init, e); err != nil {
			t.Fatal(err)
		}
	}
	c, _ := init.Marshal()
	if !bytes.Equal(c, b) {
		t.Fatal("full replay differs")
	}
	tr, _ := st.Transcript(ctx, slot.ID, 0, 0)
	if len(tr) != 30 {
		t.Fatalf("transcript = %d", len(tr))
	}
	pend, _ := st.PendingNarrations(ctx, slot.ID)
	if len(pend) != 30 {
		t.Fatalf("pending = %d", len(pend))
	}
	if err := st.SetNarration(ctx, slot.ID, "c0", "你环顾四周。", eventstore.Entry{Kind: "narration"}); err != nil {
		t.Fatal(err)
	}
	_ = st.SetNarration(ctx, slot.ID, "c0", "重复", eventstore.Entry{Kind: "narration"})
	tr, _ = st.Transcript(ctx, slot.ID, 0, 0)
	if len(tr) != 31 {
		t.Fatalf("narration should be written exactly once, transcript = %d", len(tr))
	}
	// 复制与删除
	cp, err := st.CopySlot(ctx, slot.ID, "副本")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(ctx, cp); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteSlot(ctx, slot.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Load(ctx, slot.ID); !errors.Is(err, eventstore.ErrNotFound) {
		t.Fatalf("deleted slot load: %v", err)
	}
	slots, _ := st.ListSlots(ctx)
	if len(slots) != 1 || slots[0].ID != cp || slots[0].LastSeq == 0 {
		t.Fatalf("slots after delete: %+v", slots)
	}
}
