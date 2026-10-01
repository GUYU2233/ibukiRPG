package command

import (
	"github.com/GUYU2233/ibukiRPG/internal/combat/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// 核心 Command 类型（第 5.2 节）：大多数玩法都经过 ExecuteAction。
const (
	KindAction   = "action"   // ExecuteActionCommand(action_id, args)
	KindFreeform = "freeform" // FreeformAction（第 6 节）
	KindMove     = "move"     // MoveCommand
	// KindCombat 是战斗指令：Action = start / attack / skill / item / defend / flee / mech / eject。
	KindCombat = "combat"
	// KindManage 是成长与装备管理：Action = equip / unequip / allocate / learn。
	KindManage = "manage"
	// KindWorldChange 是统一写入网关提交的世界变更（架构 V0.3 第 14.1 节）。
	KindWorldChange = "world_change"
	// KindDecision 是接受 / 回滚偏离提示（Action = accept / rollback）。
	KindDecision = "decision"
	// KindWait 是时间跳跃（Target = 10m / 1h / dawn / event:<id>）。
	KindWait = "wait"
)

// SuggestedCheck 是 AI / 规则解析器建议的检定。仅是建议，Core 会校验并夹紧。
type SuggestedCheck struct {
	Skill      string `json:"skill"`
	Difficulty int    `json:"difficulty"`
}

// ProposedEffect 是 FreeformAction 建议的轻量效果（白名单见 event.FreeformWhitelist）。
// Type: noise / attention / scene_fact / relationship_nudge / minor_condition / none
type ProposedEffect struct {
	Type   string `json:"type"`
	Target string `json:"target,omitempty"`
	Text   string `json:"text,omitempty"`
	Value  int    `json:"value,omitempty"`
	// When: success / failure / always；为空时按类型取默认值（见 engine）。
	When string `json:"when,omitempty"`
}

// Freeform 是 FreeformAction 的结构（第 6 节）。
type Freeform struct {
	Description      string           `json:"description"`
	Targets          []string         `json:"targets,omitempty"`
	Check            *SuggestedCheck  `json:"suggested_check,omitempty"`
	EstimatedMinutes int              `json:"estimated_time,omitempty"`
	Tags             []string         `json:"tags,omitempty"`
	Effects          []ProposedEffect `json:"proposed_effects,omitempty"`
	Reasonability    string           `json:"reasonability,omitempty"`
}

// Command 是进入 Game Core 的唯一执行协议。ID 即 command_id，用于幂等（第 52 节）。
type Command struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Action      string    `json:"action,omitempty"`
	Target      string    `json:"target,omitempty"`
	Item        string    `json:"item,omitempty"`
	Destination string    `json:"destination,omitempty"`
	Freeform    *Freeform `json:"freeform,omitempty"`
	Skill       string    `json:"skill,omitempty"`
	// Changes 是世界变更提案（KindWorldChange），已经过校验器。
	Changes []change.Change `json:"changes,omitempty"`
	// Intent 是自由战斗意图（KindCombat，Action = freeform）。
	Intent *freeform.Intent `json:"intent,omitempty"`
	Input  string           `json:"input,omitempty"`  // 玩家原始输入（审计用）
	Source string           `json:"source,omitempty"` // ui / resolver:offline / resolver:ai
}
