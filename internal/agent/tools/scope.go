// Package tools 是 AI Agent 的只读检索工具网关（Command/Query API 的 READ 权限部分）。
//
// 所有工具都是确定性的只读查询：输入相同的故事包 + 存档状态 + 参数，输出完全相同；结果大小有上限；
// 不产生任何命令或事件。每次调用都带一个 Scope，按 Agent 身份过滤可见数据：
//
//   - npc:<id>  —— NPC Agent：只能看到该 NPC 自己知道的事（NPCScope），读不到别人的秘密 / 隐藏数值 / 剧本节点；
//   - player    —— 叙述者（Narrator）：只能看到玩家已知的信息（PlayerScope）+ 当前锚点允许的伏笔；
//   - director  —— 导演 / 自由推演规划器：可以读取完整故事包（含秘密、隐藏字段、全部锚点），但同样只读。
//
// 同一套工具既通过进程内网关提供给 AI 管线（移动端 / CLI），也通过 MCP 适配器（stdio）暴露给外部客户端。
package tools

import (
	"fmt"
	"strings"
)

// ScopeKind 是 Agent 的数据可见范围。
type ScopeKind string

// 可见范围。
const (
	ScopeNPC      ScopeKind = "npc"
	ScopePlayer   ScopeKind = "player"
	ScopeDirector ScopeKind = "director"
)

// Scope 是一次工具调用的可见范围。
type Scope struct {
	Kind ScopeKind
	NPC  string // Kind == npc 时为 NPC 的 ID
}

// Player 是叙述者使用的 PlayerScope。
func Player() Scope { return Scope{Kind: ScopePlayer} }

// Director 是导演 / 规划器使用的全局只读范围。
func Director() Scope { return Scope{Kind: ScopeDirector} }

// NPC 返回某个 NPC 的 NPCScope。
func NPC(id string) Scope { return Scope{Kind: ScopeNPC, NPC: id} }

// String 返回 "npc:<id>" / "player" / "director"。
func (s Scope) String() string {
	if s.Kind == ScopeNPC {
		return "npc:" + s.NPC
	}
	return string(s.Kind)
}

// ParseScope 解析 "npc:<id>" / "player"（或 "narrator"）/ "director"（"author" 是 director 的别名：作者调试用，读权限相同，
// MCP `--allow-write` 时可写）。
func ParseScope(v string) (Scope, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "" || v == "player" || v == "narrator":
		return Player(), nil
	case v == "director" || v == "planner" || v == "author":
		return Director(), nil
	case strings.HasPrefix(v, "npc:") && len(v) > 4:
		return NPC(strings.TrimPrefix(v, "npc:")), nil
	}
	return Scope{}, fmt.Errorf("未知的范围 %q（可用：player / director / author / npc:<角色ID>）", v)
}
