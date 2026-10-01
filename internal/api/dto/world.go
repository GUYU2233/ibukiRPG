package dto

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
}
