package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	// 纯 Go SQLite 驱动（无 CGO），便于 Android / Windows 交叉编译。
	_ "modernc.org/sqlite"
)

// DriverName 是 modernc.org/sqlite 注册的驱动名。
const DriverName = "sqlite"

// MemoryDSN 返回独立的内存数据库 DSN。
const MemoryDSN = ":memory:"

// Open 打开数据库并设置推荐的 PRAGMA。
//
// 游戏是单用户场景：统一限制为单连接，避免 SQLITE_BUSY，也保证内存库（每个连接独立）可用。
// 文件库的 PRAGMA 同时写进 DSN（modernc 的 _pragma 参数），连接重建后依然生效。
func Open(ctx context.Context, dsn string) (*sql.DB, error) {
	open := dsn
	if dsn != MemoryDSN && !strings.Contains(dsn, "?") {
		open = dsn + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open(DriverName, open)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	if dsn != MemoryDSN {
		pragmas = append(pragmas, "PRAGMA journal_mode = WAL")
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	return db, nil
}

// Version 返回 SQLite 引擎版本。
func Version(ctx context.Context, db *sql.DB) (string, error) {
	var v string
	err := db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&v)
	return v, err
}

// WithTx 在事务中执行 fn：fn 返回错误则回滚，否则提交。
func WithTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()
	return fn(tx)
}
