package dto

import "encoding/json"

// ---------- 0.2.0：开放世界 DTO（架构 V0.3 第 16 节）----------

// WorldLogV1 是叙事流中一回合的世界更新（知识解锁 chip + 世界变更 chip + 失败提示）。
type WorldLogV1 struct {
	Changes  []WorldChangeV1   `json:"changes,omitempty"`
	Reveals  []KnowledgeChipV1 `json:"reveals,omitempty"`
	Rejected int               `json:"rejected,omitempty"`
	// Failed：parse / refused / validator_all_rejected；空 = 成功。
	Failed string `json:"failed,omitempty"`
	Impact int    `json:"impact,omitempty"`
}

// WorldChangeV1 是一条世界变更（日志页 / chip）。隐藏真相相关的变更只给脱敏摘要。
type WorldChangeV1 struct {
	ID          string   `json:"id"`
	Op          string   `json:"op"`
	Target      string   `json:"target"`
	TargetName  string   `json:"target_name"`
	Path        string   `json:"path,omitempty"`
	Summary     string   `json:"summary"`
	Reason      string   `json:"reason,omitempty"`
	Before      string   `json:"before,omitempty"`
	After       string   `json:"after,omitempty"`
	Source      string   `json:"source"`
	SourceLabel string   `json:"source_label"`
	Turn        int      `json:"turn"`
	Time        string   `json:"time,omitempty"`
	Impact      int      `json:"impact"`
	Types       []string `json:"types,omitempty"`
	RevertedBy  string   `json:"reverted_by,omitempty"`
	Reverts     string   `json:"reverts,omitempty"`
	Mechanical  bool     `json:"mechanical,omitempty"`
	Hidden      bool     `json:"hidden,omitempty"`
	CanRevert   bool     `json:"can_revert,omitempty"`
}

// KnowledgeChipV1 是一次知识解锁（叙事流 chip）。
type KnowledgeChipV1 struct {
	Entity     string `json:"entity"`
	Name       string `json:"name"`
	Field      string `json:"field"`
	FieldLabel string `json:"field_label"`
	Level      string `json:"level"` // known / rumored
	Channel    string `json:"channel,omitempty"`
	Kind       string `json:"kind,omitempty"` // field / relation / rumor
}

// DecisionV1 是偏离提示（底部面板）。
type DecisionV1 struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	TypeLabel    string   `json:"type_label"`
	Sensitivity  string   `json:"sensitivity"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Score        int      `json:"score"`
	Lines        []string `json:"lines,omitempty"`
	More         int      `json:"more,omitempty"`
	Notify       bool     `json:"notify,omitempty"`
	RollbackTurn int      `json:"rollback_turn"`
	RollbackText string   `json:"rollback_text,omitempty"`
	Checkpoint   string   `json:"checkpoint,omitempty"`
}

// UsageV1 是 token 用量（回合或存档累计）。
type UsageV1 struct {
	Turn             int           `json:"turn,omitempty"`
	Calls            int           `json:"calls"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	Failed           int           `json:"failed,omitempty"`
	Tasks            []UsageTaskV1 `json:"tasks,omitempty"`
}

// UsageTaskV1 是某任务的用量明细。
type UsageTaskV1 struct {
	Task             string `json:"task"`
	Name             string `json:"name"`
	Model            string `json:"model"`
	Calls            int    `json:"calls"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	LatencyMS        int64  `json:"latency_ms"`
	Failed           int    `json:"failed,omitempty"`
}

// AdjudicationV1 是行动裁定卡（自由战斗）。
type AdjudicationV1 struct {
	Plausibility      string   `json:"plausibility"`
	PlausibilityLabel string   `json:"plausibility_label"`
	Parse             string   `json:"parse"`
	Mods              []string `json:"mods,omitempty"`
	ModTotal          int      `json:"mod_total"`
	Dice              string   `json:"dice,omitempty"`
	Rolls             []int    `json:"rolls,omitempty"`
	Total             int      `json:"total,omitempty"`
	DC                int      `json:"dc"`
	Degree            string   `json:"degree,omitempty"`
	DegreeLabel       string   `json:"degree_label,omitempty"`
	Margin            int      `json:"margin,omitempty"`
	Result            string   `json:"result,omitempty"`
	Downgrade         string   `json:"downgrade,omitempty"`
	Band              string   `json:"band,omitempty"`
}

// BranchV1 是一条分支。
type BranchV1 struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Parent   string `json:"parent,omitempty"`
	ForkTurn int    `json:"fork_turn"`
	HeadTurn int    `json:"head_turn"`
	Status   string `json:"status"`
	Current  bool   `json:"current,omitempty"`
}

// TimelineTurnV1 是时间线中的一个回合。
type TimelineTurnV1 struct {
	Turn    int    `json:"turn"`
	Time    string `json:"time,omitempty"`
	Summary string `json:"summary"`
	Current bool   `json:"current,omitempty"`
	Fork    bool   `json:"fork,omitempty"` // 有其他分支从这里分出
}

// CheckpointV1 是检查点。
type CheckpointV1 struct {
	ID     string `json:"id"`
	Branch string `json:"branch"`
	Turn   int    `json:"turn"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Reason string `json:"reason,omitempty"`
}

// TimelineV1 是时间线与检查点视图。
type TimelineV1 struct {
	Branches    []BranchV1       `json:"branches"`
	Turns       []TimelineTurnV1 `json:"turns"`
	Checkpoints []CheckpointV1   `json:"checkpoints"`
	PendingTurn int              `json:"pending_turn,omitempty"` // >0：已选择“回到这里”，继续行动将创建新分支
	Branch      string           `json:"branch"`
}

// WorldEventChipV1 是一个已知的世界事件（状态条倒计时 / 时间线页签）。
type WorldEventChipV1 struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	StatusLabel string `json:"status_label"`
	Location    string `json:"location,omitempty"`
	Countdown   string `json:"countdown,omitempty"`
	StartsIn    int64  `json:"starts_in_min,omitempty"`
	Rumored     bool   `json:"rumored,omitempty"`
	// 以下仅世界面板“时间线”页填写。
	Summary string `json:"summary,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Time    string `json:"time,omitempty"`
	Pivotal bool   `json:"pivotal,omitempty"`
}

// CreationPresetV1 是可选的预设主角。
type CreationPresetV1 struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Attributes  map[string]int `json:"attributes,omitempty"`
}

// CreationBackgroundV1 是自建角色可选的出身。
type CreationBackgroundV1 struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Attributes  map[string]int `json:"attributes,omitempty"`
	Gold        int            `json:"gold,omitempty"`
}

// CreationOptionsV1 是角色创建界面需要的选项（第 13 节）。
type CreationOptionsV1 struct {
	PackID          string                 `json:"pack_id"`
	Presets         []CreationPresetV1     `json:"presets"`
	Backgrounds     []CreationBackgroundV1 `json:"backgrounds,omitempty"`
	AttributeNames  map[string]string      `json:"attribute_names,omitempty"`
	BaseAttributes  map[string]int         `json:"base_attributes,omitempty"`
	AttributePoints int                    `json:"attribute_points"`
	AttrMax         int                    `json:"attr_max"`
	Rules           []string               `json:"rules,omitempty"`
	MaxPower        int                    `json:"max_power,omitempty"`
}

// CreationReviewV1 是角色审查结果（规则层 + 审查 Agent）。
type CreationReviewV1 struct {
	OK          bool            `json:"ok"`
	Source      string          `json:"source"` // rules | ai
	Problems    []string        `json:"problems,omitempty"`
	LoreFit     string          `json:"lore_fit,omitempty"`
	Power       int             `json:"power"`
	MaxPower    int             `json:"max_power,omitempty"`
	Conflicts   []string        `json:"conflicts,omitempty"`
	Suggestions []string        `json:"suggestions,omitempty"`
	Recommended json.RawMessage `json:"recommended,omitempty"` // 推荐的角色卡（engine.Creation 结构）
}

// EntityFieldV1 是实体卡片上的一个字段（按玩家认知等级展示，第 8 节）。
type EntityFieldV1 struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value,omitempty"` // unknown 时为空
	Level string `json:"level"`           // known | rumored | unknown
	// Changed：该字段被世界变更改写过。
	Changed bool `json:"changed,omitempty"`
}

// EntityViewV1 是知识层过滤后的实体视图（世界面板各页通用）。
type EntityViewV1 struct {
	ID       string          `json:"id"`
	Kind     string          `json:"kind"`
	Name     string          `json:"name"`
	Icon     string          `json:"icon,omitempty"`
	Fields   []EntityFieldV1 `json:"fields"`
	Secrets  []string        `json:"secrets,omitempty"` // 已揭示的隐藏真相
	Retired  string          `json:"retired,omitempty"` // 退场原因（死亡 / 遗失…）
	Here     bool            `json:"here,omitempty"`    // 当前所在地 / 在场
	Progress string          `json:"progress,omitempty"`
}

// WorldPanelV1 是世界面板一页的数据：角色 / 关系网 / 图鉴 / 装备 / 地图 / 势力 / 时间线 / 日志。
// characters / map / factions 用 Entities；timeline 用 Events；log 用 Changes；
// relations / codex / equipment 分别复用 RelationsV1 / CodexV1 / InventoryV1。
type WorldPanelV1 struct {
	Tab       string             `json:"tab"`
	Entities  []EntityViewV1     `json:"entities,omitempty"`
	Events    []WorldEventChipV1 `json:"events,omitempty"`
	Changes   []WorldChangeV1    `json:"changes,omitempty"`
	Relations *RelationsV1       `json:"relations,omitempty"`
	Codex     *CodexV1           `json:"codex,omitempty"`
	Inventory *InventoryV1       `json:"inventory,omitempty"`
}

// WorldTabs 是世界面板的 8 个页签（顺序即 UI 顺序）。
var WorldTabs = []string{"characters", "relations", "codex", "equipment", "map", "factions", "timeline", "log"}

// ChangePreviewV1 是一组变更提案的预览（第 14.3 节）：字段级差异、影响分、被拒绝项与确认令牌。
type ChangePreviewV1 struct {
	Token   string          `json:"preview_token,omitempty"`
	Changes []WorldChangeV1 `json:"changes,omitempty"`
	// HiddenCount 是会被修改、但玩家还不知道的字段数（不剧透，只计数）。
	HiddenCount int        `json:"hidden_count,omitempty"`
	Impact      int        `json:"impact"`
	Types       []string   `json:"types,omitempty"`
	Rejected    []RejectV1 `json:"rejected,omitempty"`
	// Note 是卡片编辑器的一句话说明（可空）。
	Note string `json:"note,omitempty"`
}

// RejectV1 是一项被校验器拒绝的提案。
type RejectV1 struct {
	Target string `json:"target,omitempty"`
	Path   string `json:"path,omitempty"`
	Op     string `json:"op,omitempty"`
	Reason string `json:"reason"`
}

// AuditV1 是一次一致性审查的结果（叙事流 kind=audit 条目）。
type AuditV1 struct {
	ID       string           `json:"id"`
	Findings []AuditFindingV1 `json:"findings,omitempty"`
	// Fixed 是自动应用的修复（设定 / 文字类，可在日志页单项撤销）。
	Fixed *WorldLogV1 `json:"fixed,omitempty"`
	// Suggestions 是涉及机械状态（物品 / 金钱 / 经验 / 生死）的修复建议，需要玩家确认（第 19 节决定 4）。
	Suggestions []AuditSuggestionV1 `json:"suggestions,omitempty"`
}

// AuditFindingV1 是一条审查发现。
type AuditFindingV1 struct {
	Kind string `json:"kind"` // omission / hallucination / contradiction
	Turn int    `json:"turn,omitempty"`
	Text string `json:"text"`
	Note string `json:"note,omitempty"`
}

// AuditSuggestionV1 是一条待确认的修复建议。
type AuditSuggestionV1 struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Reason  string `json:"reason,omitempty"`
	// Status：pending / applied / ignored。
	Status  string          `json:"status"`
	Changes json.RawMessage `json:"changes,omitempty"`
}

// RevertPlanV1 是撤销前的级联检查（v0.2.0-rc1）：之后依赖这条变更的全部变更（撤销顺序，新的在前），
// 以及能否只撤销这一条。
type RevertPlanV1 struct {
	Change     WorldChangeV1   `json:"change"`
	Dependents []WorldChangeV1 `json:"dependents,omitempty"`
	CanSingle  bool            `json:"can_single"`
	// SingleNote 说明只撤销这一条的效果（例如“当前值被后续变更覆盖，保持不变”）或不能单独撤销的原因。
	SingleNote string `json:"single_note,omitempty"`
}

// ExternalNoticeV1 是“外部工具修改了 N 项设定”的提示（v0.2.0-rc1）：上次确认之后，MCP 等外部工具提交且未撤销的世界变更。
// 打开游戏 / 载入存档时显示，链接到世界变更日志（source=mcp 筛选）；确认后调用 ack_external_changes。
type ExternalNoticeV1 struct {
	Count int      `json:"count"`
	Text  string   `json:"text"`
	IDs   []string `json:"ids,omitempty"`
	// Source 是日志页的来源筛选值。
	Source string `json:"source"`
}
