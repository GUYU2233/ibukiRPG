package event

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
}
