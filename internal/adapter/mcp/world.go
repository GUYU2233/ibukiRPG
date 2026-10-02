package mcp

import (
	"context"
	"encoding/json"
)

// World 是 0.2 世界层工具的后端（CLI 用 orchestrator.Session 实现）。
// 读工具（world_change_log / timeline_get / knowledge_get）在所有范围可用；
// 写工具只有在 `--allow-write` 且范围为 director / author 时才会列出（第 14.5 节）。
type World interface {
	ChangeLog(ctx context.Context, query string, sinceTurn int) (any, error)
	Timeline(ctx context.Context) (any, error)
	Knowledge(ctx context.Context, entity string) (any, error)
	Preview(ctx context.Context, changes json.RawMessage) (any, error)
	Apply(ctx context.Context, token string, acceptImpact bool) (any, error)
	Revert(ctx context.Context, changeID, mode string) (any, error)
	RevertPlan(ctx context.Context, changeID string) (any, error)
	Checkpoint(ctx context.Context, name string) (any, error)
}

type worldTool struct {
	name, title, desc string
	write             bool
	schema            map[string]any
	call              func(ctx context.Context, w World, args json.RawMessage) (any, error)
}

func obj(props map[string]any, required ...string) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }

var worldTools = []worldTool{
	{name: "world_change_log", title: "世界变更日志", desc: "检索世界变更日志（谁、何时、为什么改了什么）。只显示玩家可见的摘要。",
		schema: obj(map[string]any{"query": str("关键词（实体名 / 字段 / 原因）"), "since_turn": map[string]any{"type": "integer", "description": "只看这一回合之后的变更"}}),
		call: func(ctx context.Context, w World, a json.RawMessage) (any, error) {
			var p struct {
				Query     string `json:"query"`
				SinceTurn int    `json:"since_turn"`
			}
			_ = json.Unmarshal(a, &p)
			return w.ChangeLog(ctx, p.Query, p.SinceTurn)
		}},
	{name: "timeline_get", title: "世界时间线", desc: "读取开放世界的事件时间线（已发生 / 即将发生 / 传闻）。玩家不知道的事件不会出现。",
		schema: obj(map[string]any{}),
		call:   func(ctx context.Context, w World, _ json.RawMessage) (any, error) { return w.Timeline(ctx) }},
	{name: "knowledge_get", title: "玩家知识", desc: "读取一个实体对玩家可见的字段（未解锁的字段显示“未知”）。",
		schema: obj(map[string]any{"entity": str("实体 ID")}, "entity"),
		call: func(ctx context.Context, w World, a json.RawMessage) (any, error) {
			var p struct {
				Entity string `json:"entity"`
			}
			_ = json.Unmarshal(a, &p)
			return w.Knowledge(ctx, p.Entity)
		}},
	{name: "world_preview_change", title: "预览世界修改", write: true,
		desc:   "预览一组世界修改：返回差异、影响分、被拒绝项与原因，以及 preview_token。不会修改存档。变更格式同 WORLD 段：{op,target,path,value,reason}。",
		schema: obj(map[string]any{"changes": map[string]any{"type": "array", "items": map[string]any{"type": "object"}, "description": "变更列表"}}, "changes"),
		call: func(ctx context.Context, w World, a json.RawMessage) (any, error) {
			var p struct {
				Changes json.RawMessage `json:"changes"`
			}
			_ = json.Unmarshal(a, &p)
			return w.Preview(ctx, p.Changes)
		}},
	{name: "world_apply_change", title: "提交世界修改", write: true,
		desc:   "提交一份预览过的修改（只接受 preview_token）。存档在预览后变化则失败；影响分超过 50 时需要 accept_impact=true。以“外部工具”来源记录在日志里，可撤销。",
		schema: obj(map[string]any{"preview_token": str("world_preview_change 返回的令牌"), "accept_impact": map[string]any{"type": "boolean"}}, "preview_token"),
		call: func(ctx context.Context, w World, a json.RawMessage) (any, error) {
			var p struct {
				Token  string `json:"preview_token"`
				Accept bool   `json:"accept_impact"`
			}
			_ = json.Unmarshal(a, &p)
			return w.Apply(ctx, p.Token, p.Accept)
		}},
	{name: "world_revert_plan", title: "撤销前检查", desc: "列出之后依赖这条变更的全部变更（撤销顺序），以及能否只撤销这一条。不会修改存档。",
		schema: obj(map[string]any{"change_id": str("变更 ID，例如 wc-12-1")}, "change_id"),
		call: func(ctx context.Context, w World, a json.RawMessage) (any, error) {
			var p struct {
				ID string `json:"change_id"`
			}
			_ = json.Unmarshal(a, &p)
			return w.RevertPlan(ctx, p.ID)
		}},
	{name: "world_revert_change", title: "撤销世界修改", write: true,
		desc:   "撤销一条世界变更。之后有依赖它的变更时需要指定 mode：chain = 连同依赖一起撤销；single = 只撤销这一条（新建实体等不能单独撤销）。先用 world_revert_plan 查看依赖链。",
		schema: obj(map[string]any{"change_id": str("变更 ID，例如 wc-12-1"), "mode": map[string]any{"type": "string", "enum": []string{"chain", "single"}, "description": "有依赖时的撤销方式"}}, "change_id"),
		call: func(ctx context.Context, w World, a json.RawMessage) (any, error) {
			var p struct {
				ID   string `json:"change_id"`
				Mode string `json:"mode"`
			}
			_ = json.Unmarshal(a, &p)
			return w.Revert(ctx, p.ID, p.Mode)
		}},
	{name: "checkpoint_create", title: "新建检查点", write: true, desc: "在当前回合新建一个手动检查点。",
		schema: obj(map[string]any{"name": str("检查点名字")}),
		call: func(ctx context.Context, w World, a json.RawMessage) (any, error) {
			var p struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal(a, &p)
			return w.Checkpoint(ctx, p.Name)
		}},
}

func (s *Server) worldToolsVisible() []worldTool {
	if s.World == nil {
		return nil
	}
	var out []worldTool
	for _, t := range worldTools {
		if t.write && !s.writable() {
			continue
		}
		out = append(out, t)
	}
	return out
}

// writable 报告写入工具是否可用：--allow-write 且范围为 director（author 同 director）。player / npc 范围永远只读。
func (s *Server) writable() bool {
	return s.AllowWrite && s.Scope.Kind == "director"
}

func (s *Server) findWorldTool(name string) (worldTool, bool) {
	for _, t := range s.worldToolsVisible() {
		if t.name == name {
			return t, true
		}
	}
	return worldTool{}, false
}
