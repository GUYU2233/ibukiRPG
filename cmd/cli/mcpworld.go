package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/GUYU2233/ibukiRPG/internal/agent/orchestrator"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// mcpWorld 用 orchestrator.Session 实现 MCP 的世界层工具。每次调用都重新载入存档；
// 写入前检查存档占用锁（游戏正在运行同一存档时拒绝）。
type mcpWorld struct {
	s    *orchestrator.Session
	slot string
}

func (w *mcpWorld) read(ctx context.Context) error {
	_, err := w.s.OpenForRead(ctx, w.slot)
	return err
}

func (w *mcpWorld) write(ctx context.Context) error {
	_, err := w.s.OpenForWrite(ctx, w.slot)
	return err
}

func (w *mcpWorld) ChangeLog(ctx context.Context, query string, sinceTurn int) (any, error) {
	if err := w.read(ctx); err != nil {
		return nil, err
	}
	chs, err := w.s.WorldChanges(query, "", 100)
	if err != nil {
		return nil, err
	}
	out := chs[:0]
	for _, c := range chs {
		if c.Turn > sinceTurn {
			out = append(out, c)
		}
	}
	return map[string]any{"changes": out}, nil
}

func (w *mcpWorld) Timeline(ctx context.Context) (any, error) {
	if err := w.read(ctx); err != nil {
		return nil, err
	}
	return w.s.WorldPanel("timeline")
}

func (w *mcpWorld) Knowledge(ctx context.Context, entity string) (any, error) {
	if err := w.read(ctx); err != nil {
		return nil, err
	}
	v, ok, err := w.s.Entity(entity)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("玩家还不知道这个实体，或 ID 不存在")
	}
	return v, nil
}

func (w *mcpWorld) Preview(ctx context.Context, raw json.RawMessage) (any, error) {
	if err := w.write(ctx); err != nil {
		return nil, err
	}
	var cs []change.Change
	if err := json.Unmarshal(raw, &cs); err != nil {
		return nil, errors.New("changes 必须是变更对象数组：" + err.Error())
	}
	return w.s.PreviewChanges(ctx, change.SourceMCP+":director", cs)
}

func (w *mcpWorld) Apply(ctx context.Context, token string, accept bool) (any, error) {
	// 重新载入最新状态：预览之后存档被别处修改过时，令牌的头部指纹对不上，提交失败
	if err := w.write(ctx); err != nil {
		return nil, err
	}
	limit := orchestrator.MCPImpactLimit
	if accept {
		limit = 0
	}
	return w.s.ApplyPreview(ctx, token, limit)
}

func (w *mcpWorld) Revert(ctx context.Context, id, mode string) (any, error) {
	if err := w.write(ctx); err != nil {
		return nil, err
	}
	return w.s.RevertChangeMode(ctx, id, mode)
}

func (w *mcpWorld) Checkpoint(ctx context.Context, name string) (any, error) {
	if err := w.write(ctx); err != nil {
		return nil, err
	}
	if name == "" {
		name = "外部工具检查点"
	}
	return w.s.CreateCheckpoint(ctx, name, 0)
}

func (w *mcpWorld) RevertPlan(ctx context.Context, id string) (any, error) {
	if err := w.read(ctx); err != nil {
		return nil, err
	}
	return w.s.RevertPlan(id)
}
