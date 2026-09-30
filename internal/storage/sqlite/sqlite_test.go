package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

const schema = `CREATE TABLE events (
	seq        INTEGER PRIMARY KEY AUTOINCREMENT,
	command_id TEXT NOT NULL,
	type       TEXT NOT NULL,
	payload    TEXT NOT NULL
)`

func count(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInMemoryTransactions(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, MemoryDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	v, err := Version(ctx, db)
	if err != nil || v == "" {
		t.Fatalf("version: %q %v", v, err)
	}
	t.Logf("sqlite version %s", v)

	if _, err := db.ExecContext(ctx, schema); err != nil {
		t.Fatal(err)
	}

	// 交易三事件：全部成功。
	err = WithTx(ctx, db, func(tx *sql.Tx) error {
		for _, typ := range []string{"GoldRemoved", "ItemRemovedFromMerchant", "ItemAddedToPlayer"} {
			if _, err := tx.ExecContext(ctx, "INSERT INTO events(command_id, type, payload) VALUES (?, ?, '{}')", "cmd-1", typ); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, db); n != 3 {
		t.Fatalf("want 3 events, got %d", n)
	}

	// 中途失败：全部回滚。
	boom := errors.New("boom")
	err = WithTx(ctx, db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "INSERT INTO events(command_id, type, payload) VALUES ('cmd-2', 'GoldRemoved', '{}')"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if n := count(t, db); n != 3 {
		t.Fatalf("rollback failed: want 3 events, got %d", n)
	}
}

func TestFileDB(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, t.TempDir()+"/save.db")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %s", mode)
	}
}
