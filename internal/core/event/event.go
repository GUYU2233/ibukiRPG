package event

import (
	"github.com/GUYU2233/ibukiRPG/internal/combat/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
)

// 事件类型。所有永久状态变化都必须通过这些事件（第 3.2 节）。
const (
	GameStarted           = "GameStarted"
	ActionPerformed       = "ActionPerformed"
	FreeformPerformed     = "FreeformActionPerformed"
	SkillCheckResolved    = "SkillCheckResolved"
	TimeAdvanced          = "TimeAdvanced"
	LocationChanged       = "LocationChanged"
	GoldChanged           = "GoldChanged"
	ItemAdded             = "ItemAdded"
	ItemRemoved           = "ItemRemoved"
	RelationshipChanged   = "RelationshipChanged"
	RelationshipNudge     = "RelationshipNudge"
	SceneFactChanged      = "SceneFactChanged"
	SceneFactsCleared     = "SceneFactsCleared"
	SceneFactPromoted     = "SceneFactPromoted"
	ConditionApplied      = "ConditionApplied"
	MinorConditionApplied = "MinorConditionApplied"
	ConditionRemoved      = "ConditionRemoved"
	NoiseGenerated        = "NoiseGenerated"
	AttentionChanged      = "AttentionChanged"
	FlagSet               = "FlagSet"
	NPCMoved              = "NPCMoved"
	ObservedEventCreated  = "ObservedEventCreated"
	BeliefUpdated         = "BeliefUpdated"
	StoryStarted          = "StoryStarted"
	StoryStepReached      = "StoryStepReached"
	StoryResolved         = "StoryResolved"
	PacingUpdated         = "PacingUpdated"
	AmbientEvent          = "AmbientEvent"
	NoMechanicalEffect    = "NoMechanicalEffect"
	TurnCompleted         = "TurnCompleted"
	// DialogueOccurred 记录玩家与 NPC 的一次交谈：NPC 说了哪条台词、谈到什么话题（对话记忆）。
	DialogueOccurred = "DialogueOccurred"
	// MemoryRecorded 记录 NPC 的一条情节记忆（Episodic Memory），只写给亲历 / 目击者。
	MemoryRecorded = "MemoryRecorded"
	// VarChanged 修改故事变量（整数，Delta 形式，保证可重放）。
	VarChanged = "VarChanged"

	// ---- 战斗（v0.1.1-rc2）：回合制、先攻、HP/体力、机甲形态（红水银 / 过热）。----
	CombatStarted      = "CombatStarted"      // Story=遭遇 ID，Units=参战单位快照
	CombatRoundStarted = "CombatRoundStarted" // Delta=回合数，Tags=先攻顺序，Values=先攻掷骰
	CombatTurnStarted  = "CombatTurnStarted"  // Actor=行动单位，Delta=顺序游标
	CombatActed        = "CombatActed"        // Actor/Target/Action/Skill/Item + 命中掷骰（Roll/DC）与伤害分解（Values）
	UnitHPChanged      = "UnitHPChanged"      // Target，Delta（已按上下限裁剪）
	UnitResource       = "UnitResourceChanged"
	UnitStatusApplied  = "UnitStatusApplied" // Target，Condition=状态 ID，Delta=持续回合
	UnitStatusTicked   = "UnitStatusTicked"  // Target，Condition；持续回合 -1，归零移除
	UnitStatusRemoved  = "UnitStatusRemoved"
	UnitDefending      = "UnitDefending"
	UnitDefeated       = "UnitDefeated"
	MechEngaged        = "MechEngaged"    // Target，Units[0]=机甲形态快照
	MechDisengaged     = "MechDisengaged" // Target，Reason
	UnitOverheated     = "UnitOverheated"
	CombatEnded        = "CombatEnded" // Story，Outcome=victory/defeat/fled，Reason=战败分支

	// ---- 成长 ----
	XPGained           = "XPGained"
	LevelUp            = "LevelUp" // Delta=新等级，Values=attr_points/skill_points
	AttributeAllocated = "AttributeAllocated"
	SkillLearned       = "SkillLearned"
	ItemEquipped       = "ItemEquipped"   // Item，Key=槽位
	ItemUnequipped     = "ItemUnequipped" // Key=槽位
	PlayerVitals       = "PlayerVitalsChanged"
	CodexUnlocked      = "CodexUnlocked"

	// ---- 关系网 ----
	RelationEdgeChanged = "RelationEdgeChanged" // Actor→Target，Values=各维度增量，Notable=玩家知情
	RelationRevealed    = "RelationRevealed"    // 玩家得知 Actor→Target 的当前关系（Values=绝对值）

	// ---- 角色卡 ----
	CharacterCardCreated  = "CharacterCardCreated"
	CharacterCardArchived = "CharacterCardArchived"
	CharacterCardRestored = "CharacterCardRestored"
	CharacterDied         = "CharacterDied"

	// ---- 开放世界（v0.2.0）：世界变更、知识层、时间线、偏离提示、自由战斗、分支 ----
	WorldChangeApplied  = "WorldChangeApplied"  // Change
	WorldChangeReverted = "WorldChangeReverted" // Change=反向操作，Key=被撤销的变更 ID
	ImpactAssessed      = "ImpactAssessed"      // Impact，Tags=变更 ID
	WorldUpdateFailed   = "WorldUpdateFailed"   // Reason=parse/refused/timeout/interrupted/validator_all_rejected
	KnowledgeRevealed   = "KnowledgeRevealed"   // Reveal
	RumorHeard          = "RumorHeard"          // Reveal（level=rumored），Story=世界事件 ID
	WorldEventScheduled = "WorldEventScheduled" // WorldEvent（AI / 玩家新增；故事包事件不需要）
	WorldEventStarted   = "WorldEventStarted"   // Story=事件 ID，Step=阶段
	WorldEventStage     = "WorldEventStageAdvanced"
	WorldEventJoined    = "WorldEventJoined"    // Story，Key=hook，Values=vars 增量
	WorldEventDisrupted = "WorldEventDisrupted" // 同上
	WorldEventResolved  = "WorldEventResolved"  // Story，Outcome，Source=world/player/ai
	WorldEventCancelled = "WorldEventCancelled" // Story，Reason
	DecisionRequested   = "DecisionRequested"   // Decision
	DecisionResolved    = "DecisionResolved"    // Key=决定 ID，Outcome=accepted/rolled_back
	CombatIntentParsed  = "CombatIntentParsed"  // Intent，Text=原文，Source
	ActionAdjudicated   = "ActionAdjudicated"   // Adjudication
	PartHit             = "PartHit"             // Target，Key=部位，Delta=倍率，Remove=破坏
	BranchCreated       = "BranchCreated"       // Key=分支 ID，Seed=随机盐
	CharacterCreated    = "CharacterCreated"    // Profile（开局角色创建）
	// ---- 机械甲胄卡（v0.1.2-rc2）----
	MechStatusChanged = "MechStatusChanged" // Target=机甲 Key=状态 Reason
	MechPartChanged   = "MechPartChanged"   // Target=机甲 Key=槽位 Item=部件 Remove=卸下
	MechRevealed      = "MechRevealed"      // Target=机甲 Tags=揭示的字段（"*" 为全部）Source
	MechPilotChanged  = "MechPilotChanged"  // Target=机甲 Actor=驾驶者（空为无人）
)

// FreeformWhitelist 是 FreeformAction 允许产生的轻量事件（第 6.1 节）。
var FreeformWhitelist = map[string]bool{
	SkillCheckResolved:    true,
	TimeAdvanced:          true,
	NoiseGenerated:        true,
	AttentionChanged:      true,
	SceneFactChanged:      true,
	RelationshipNudge:     true,
	MinorConditionApplied: true,
	ObservedEventCreated:  true,
	BeliefUpdated:         true,
	NoMechanicalEffect:    true,
	FreeformPerformed:     true,
	PacingUpdated:         true,
	AmbientEvent:          true,
	TurnCompleted:         true,
	MemoryRecorded:        true,
}

// Event 是一条不可变的领域事件。Seq 由 Event Store 分配（单存档内单调递增）。
type Event struct {
	Seq       int64  `json:"seq"`
	CommandID string `json:"command_id,omitempty"`
	Turn      int    `json:"turn"`
	Minute    int64  `json:"minute"`
	Type      string `json:"type"`
	Data      Data   `json:"data"`
}

// Data 是事件载荷。为了让存档格式简单、可审计，所有事件共享一个带 omitempty 的结构。
// 数值全部为整数（第 28 节）；map 在 JSON 编码时按键排序，保证字节级确定。
type Data struct {
	Actor      string         `json:"actor,omitempty"`
	Target     string         `json:"target,omitempty"`
	Item       string         `json:"item,omitempty"`
	Action     string         `json:"action,omitempty"`
	Outcome    string         `json:"outcome,omitempty"`
	Location   string         `json:"location,omitempty"`
	From       string         `json:"from,omitempty"`
	To         string         `json:"to,omitempty"`
	Skill      string         `json:"skill,omitempty"`
	Text       string         `json:"text,omitempty"`
	Flag       string         `json:"flag,omitempty"`
	Condition  string         `json:"condition,omitempty"`
	Story      string         `json:"story,omitempty"`
	Step       string         `json:"step,omitempty"`
	Title      string         `json:"title,omitempty"`
	Stream     string         `json:"stream,omitempty"`
	Source     string         `json:"source,omitempty"`
	Witness    string         `json:"witness,omitempty"`
	Key        string         `json:"key,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Tags       []string       `json:"tags,omitempty"`
	Values     map[string]int `json:"values,omitempty"`
	Minutes    int64          `json:"minutes,omitempty"`
	Delta      int            `json:"delta,omitempty"`
	Qty        int            `json:"qty,omitempty"`
	Roll       int            `json:"roll,omitempty"`
	Modifier   int            `json:"modifier,omitempty"`
	DC         int            `json:"dc,omitempty"`
	Total      int            `json:"total,omitempty"`
	Counter    uint64         `json:"counter,omitempty"`
	Seed       uint64         `json:"seed,omitempty"`
	Confidence int            `json:"confidence,omitempty"` // 千分比，1000 = 1.0
	Tension    int            `json:"tension,omitempty"`
	QuietTurns int            `json:"quiet_turns,omitempty"`
	Success    bool           `json:"success,omitempty"`
	Critical   bool           `json:"critical,omitempty"`
	Fumble     bool           `json:"fumble,omitempty"`
	Persist    bool           `json:"persist,omitempty"`
	Remove     bool           `json:"remove,omitempty"`
	Notable    bool           `json:"notable,omitempty"`
	Nudge      bool           `json:"nudge,omitempty"`
	Repeat     bool           `json:"repeat,omitempty"`
	// ---- v0.2.0 ----
	Change       *change.Change         `json:"change,omitempty"`
	Reveal       *knowledge.Reveal      `json:"reveal,omitempty"`
	WorldEvent   *timeline.Event        `json:"world_event,omitempty"`
	Decision     *change.Decision       `json:"decision,omitempty"`
	Impact       *change.TurnImpact     `json:"impact,omitempty"`
	Intent       *freeform.Intent       `json:"intent,omitempty"`
	Adjudication *freeform.Adjudication `json:"adjudication,omitempty"`
	Profile      map[string]string      `json:"profile,omitempty"`
	// Units 是战斗单位快照（CombatStarted / MechEngaged）。
	Units []Unit `json:"units,omitempty"`
	// Node 是被正典化的主线节点（MainlineNodeCanonized）或动态角色（CharacterCardCreated）。
	Node *Node `json:"node,omitempty"`
}

// Unit 是战斗单位快照：角色与敌人共用同一套数值模型。
type Unit struct {
	ID     string         `json:"id"`
	Ref    string         `json:"ref"`  // player / NPC ID / 敌人定义 ID / 机甲定义 ID
	Side   string         `json:"side"` // party / enemy
	Name   string         `json:"name"`
	Level  int            `json:"level,omitempty"`
	HP     int            `json:"hp"`
	MaxHP  int            `json:"max_hp"`
	SP     int            `json:"sp"`
	MaxSP  int            `json:"max_sp"`
	Stats  map[string]int `json:"stats,omitempty"`
	Skills []string       `json:"skills,omitempty"`
	Tags   []string       `json:"tags,omitempty"`
	// 机甲形态参数（MechEngaged）。
	HeatMax int `json:"heat_max,omitempty"`
}

// Node 是一个动态主线节点（自由推演 AI 生成 / 沙盒模板生成）或动态角色卡。
type Node struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Objective string         `json:"objective,omitempty"`
	Goal      string         `json:"goal,omitempty"` // reach / talk / defeat / obtain / level
	Ref       string         `json:"ref,omitempty"`
	Location  string         `json:"location,omitempty"`
	Enemies   []string       `json:"enemies,omitempty"`
	Reward    map[string]int `json:"reward,omitempty"` // xp / gold
	Item      string         `json:"item,omitempty"`   // 奖励物品
	Summary   string         `json:"summary,omitempty"`
	Source    string         `json:"source,omitempty"` // ai / template
	Role      string         `json:"role,omitempty"`   // 动态角色身份
}
