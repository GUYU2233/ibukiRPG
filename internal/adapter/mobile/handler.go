package mobile

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
	"github.com/GUYU2233/ibukiRPG/internal/rules/rng"
	"github.com/GUYU2233/ibukiRPG/internal/storage/sqlite"
)

// Handle 处理一条 JSON 请求并返回 JSON 响应。永不 panic，错误写入响应体。
func Handle(ctx context.Context, requestJSON string) string {
	var req dto.RequestV1
	if err := json.Unmarshal([]byte(requestJSON), &req); err != nil {
		return encode(dto.ResponseV1{Version: dto.V1, Error: "invalid json: " + err.Error()})
	}
	resp := dto.ResponseV1{Version: dto.V1, CommandID: req.CommandID}
	if req.Version != dto.V1 {
		resp.Error = fmt.Sprintf("unsupported version %q", req.Version)
		return encode(resp)
	}
	data, err := dispatch(ctx, req)
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.OK = true
		resp.Data = data
	}
	return encode(resp)
}

func dispatch(ctx context.Context, req dto.RequestV1) (any, error) {
	switch req.Type {
	case "ping":
		return map[string]string{"pong": buildinfo.Version}, nil
	case "sqlite_smoke":
		return sqliteSmoke(ctx)
	case "roll":
		var p struct {
			Seed    uint64 `json:"seed"`
			Entity  string `json:"entity"`
			Purpose string `json:"purpose"`
			Sides   int    `json:"sides"`
		}
		if len(req.Payload) > 0 {
			if err := json.Unmarshal(req.Payload, &p); err != nil {
				return nil, err
			}
		}
		if p.Sides <= 0 {
			p.Sides = 20
		}
		s := rng.New(p.Seed, rng.Key{Namespace: "demo", Entity: p.Entity, Purpose: p.Purpose})
		return map[string]int{"roll": s.Roll(p.Sides)}, nil
	default:
		return nil, fmt.Errorf("unknown request type %q", req.Type)
	}
}

// sqliteSmoke 在内存 SQLite 中建表、事务写入并读回，验证移动端存储链路。
func sqliteSmoke(ctx context.Context) (any, error) {
	db, err := sqlite.Open(ctx, sqlite.MemoryDSN)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, `CREATE TABLE events (seq INTEGER PRIMARY KEY, type TEXT NOT NULL)`); err != nil {
		return nil, err
	}
	err = sqlite.WithTx(ctx, db, func(tx *sql.Tx) error {
		for _, t := range []string{"TimeAdvanced", "SceneFactChanged"} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO events(type) VALUES (?)`, t); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&n); err != nil {
		return nil, err
	}
	v, err := sqlite.Version(ctx, db)
	if err != nil {
		return nil, err
	}
	return map[string]any{"sqlite_version": v, "events": n}, nil
}

func encode(r dto.ResponseV1) string {
	b, err := json.Marshal(r)
	if err != nil {
		return `{"version":"v1","ok":false,"error":"encode failure"}`
	}
	return string(b)
}
