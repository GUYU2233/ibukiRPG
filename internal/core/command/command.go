package command

// 核心 Command 类型（第 5.2 节）：大多数玩法都经过 ExecuteAction。
const (
	KindAction   = "action"   // ExecuteActionCommand(action_id, args)
	KindFreeform = "freeform" // FreeformAction（第 6 节）
	KindMove     = "move"     // MoveCommand
	// KindCombat 是战斗指令：Action = start / attack / skill / item / defend / flee / mech / eject。
	KindCombat = "combat"
	// KindManage 是成长与装备管理：Action = equip / unequip / allocate / learn / thresholds。
	KindManage = "manage"
	// KindMainline 是主线偏离提示的选择：Action = return / free（Target = free / sandbox）。
	KindMainline = "mainline"
	// KindDirector 是 Director 提交的主线节点提案（自由推演），由 Core 校验后正典化。
	KindDirector = "director"
)

// NodeProposal 是 AI 提议的新主线节点（CanonProposal）。它只是提案：Core 校验 ID 引用、长度与奖励上限后才会正典化。
type NodeProposal struct {
	Title      string   `json:"title"`
	Objective  string   `json:"objective"`
	Goal       string   `json:"goal"`
	Ref        string   `json:"ref"`
	Location   string   `json:"location,omitempty"`
	Enemies    []string `json:"enemies,omitempty"`
	RewardXP   int      `json:"reward_xp,omitempty"`
	RewardGold int      `json:"reward_gold,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	// NewCharacter 非空时同时提议一名新登场的重要角色（只生成角色卡，不可交谈）。
	NewCharacter *CharacterProposal `json:"new_character,omitempty"`
}

// CharacterProposal 是 AI 提议的新角色。
type CharacterProposal struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description"`
}

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
	ID          string        `json:"id"`
	Kind        string        `json:"kind"`
	Action      string        `json:"action,omitempty"`
	Target      string        `json:"target,omitempty"`
	Item        string        `json:"item,omitempty"`
	Destination string        `json:"destination,omitempty"`
	Freeform    *Freeform     `json:"freeform,omitempty"`
	Skill       string        `json:"skill,omitempty"`
	Proposal    *NodeProposal `json:"proposal,omitempty"`
	Input       string        `json:"input,omitempty"`  // 玩家原始输入（审计用）
	Source      string        `json:"source,omitempty"` // ui / resolver:offline / resolver:ai
}
