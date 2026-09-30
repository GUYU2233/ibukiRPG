package dto

// 以下是 V1 视图类型。所有字段都是 JSON 友好的基本类型，Kotlin 端用 kotlinx.serialization 解码。

// CheckV1 是一次检定的展示信息。
type CheckV1 struct {
	Label       string `json:"label"` // 例如 “说服 · 伯林”
	SkillName   string `json:"skill_name"`
	Roll        int    `json:"roll"`
	Modifier    int    `json:"modifier"`
	DC          int    `json:"dc"`
	Total       int    `json:"total"`
	Success     bool   `json:"success"`
	Critical    bool   `json:"critical"`
	Fumble      bool   `json:"fumble"`
	Explanation string `json:"explanation"`
}

// QuickActionV1 是快速通道请求（第 9 节）：明确的 UI 操作不经过 Resolver。
type QuickActionV1 struct {
	Kind        string `json:"kind"` // action / move / text
	Action      string `json:"action,omitempty"`
	Target      string `json:"target,omitempty"`
	Item        string `json:"item,omitempty"`
	Destination string `json:"destination,omitempty"`
	Text        string `json:"text,omitempty"`  // kind=text 时作为玩家输入提交
	Label       string `json:"label,omitempty"` // 显示在对话记录中的玩家行为
}

// OptionV1 是系统给出的可点选项（澄清、替代方案）。
type OptionV1 struct {
	Label  string        `json:"label"`
	Action QuickActionV1 `json:"action"`
}

// EntryV1 是对话记录中的一条。
type EntryV1 struct {
	ID        int64      `json:"id"`
	CommandID string     `json:"command_id,omitempty"`
	Turn      int        `json:"turn"`
	Kind      string     `json:"kind"` // player / narration / check / system / story / intro
	Text      string     `json:"text"`
	Check     *CheckV1   `json:"check,omitempty"`
	Chips     []string   `json:"chips,omitempty"`
	Options   []OptionV1 `json:"options,omitempty"`
	Corrected bool       `json:"corrected,omitempty"`
	Source    string     `json:"source,omitempty"`
}

// SuggestionV1 是上下文相关的快捷建议。
type SuggestionV1 struct {
	Label    string        `json:"label"`
	Icon     string        `json:"icon"`
	Hint     string        `json:"hint,omitempty"`
	Disabled bool          `json:"disabled,omitempty"`
	Action   QuickActionV1 `json:"action"`
}

// NPCBriefV1 是场景中的人物。
type NPCBriefV1 struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Attitude string `json:"attitude"`
}

// ExitV1 是出口。
type ExitV1 struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Locked bool   `json:"locked"`
}

// StoryBriefV1 是进行中的事件。
type StoryBriefV1 struct {
	ID    string   `json:"id"`
	Title string   `json:"title"`
	Hints []string `json:"hints"`
}

// SceneV1 是场景头部与概要。
type SceneV1 struct {
	SlotID       string        `json:"slot_id"`
	SaveName     string        `json:"save_name"`
	LocationID   string        `json:"location_id"`
	LocationName string        `json:"location_name"`
	Description  string        `json:"description"`
	TimeText     string        `json:"time_text"`
	Clock        string        `json:"clock"`
	Day          int           `json:"day"`
	Period       string        `json:"period"`
	Turn         int           `json:"turn"`
	Gold         int           `json:"gold"`
	PlayerName   string        `json:"player_name"`
	Present      []NPCBriefV1  `json:"present"`
	Exits        []ExitV1      `json:"exits"`
	Facts        []string      `json:"facts"`
	Conditions   []string      `json:"conditions"`
	Story        *StoryBriefV1 `json:"story,omitempty"`
}

// TurnV1 是一次提交的返回。
type TurnV1 struct {
	CommandID   string         `json:"command_id"`
	Accepted    bool           `json:"accepted"`
	Duplicate   bool           `json:"duplicate,omitempty"`
	Entries     []EntryV1      `json:"entries"`
	Scene       SceneV1        `json:"scene"`
	Suggestions []SuggestionV1 `json:"suggestions"`
	Resolver    string         `json:"resolver,omitempty"`
}

// StatV1 是属性 / 技能。
type StatV1 struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Value    int    `json:"value"`
	Modifier int    `json:"modifier"`
	Note     string `json:"note,omitempty"`
}

// CharacterV1 是角色面板。
type CharacterV1 struct {
	Name       string   `json:"name"`
	Role       string   `json:"role"`
	Gold       int      `json:"gold"`
	Attributes []StatV1 `json:"attributes"`
	Skills     []StatV1 `json:"skills"`
	Conditions []StatV1 `json:"conditions"`
}

// ItemV1 是物品。
type ItemV1 struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Qty         int            `json:"qty"`
	Price       int            `json:"price"`
	Description string         `json:"description"`
	Affordable  bool           `json:"affordable"`
	Use         *QuickActionV1 `json:"use,omitempty"`
	UseLabel    string         `json:"use_label,omitempty"`
	Buy         *QuickActionV1 `json:"buy,omitempty"`
}

// InventoryV1 是背包面板。
type InventoryV1 struct {
	Gold       int      `json:"gold"`
	Items      []ItemV1 `json:"items"`
	Shop       []ItemV1 `json:"shop"`
	ShopSeller string   `json:"shop_seller,omitempty"`
}

// BeliefV1 是 NPC 知道的事（可追溯来源）。
type BeliefV1 struct {
	Text       string `json:"text"`
	Source     string `json:"source"`
	When       string `json:"when"`
	Confidence int    `json:"confidence"`
}

// NPCActionV1 是对 NPC 的可执行动作（带成功率预估）。
type NPCActionV1 struct {
	Label  string        `json:"label"`
	Chance int           `json:"chance"` // -1 表示无需检定
	Action QuickActionV1 `json:"action"`
}

// NPCV1 是人物关系面板中的一项。
type NPCV1 struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Role         string        `json:"role"`
	Description  string        `json:"description"`
	LocationName string        `json:"location_name"`
	Present      bool          `json:"present"`
	Trust        int           `json:"trust"`
	Fear         int           `json:"fear"`
	Attitude     string        `json:"attitude"`
	Beliefs      []BeliefV1    `json:"beliefs"`
	Actions      []NPCActionV1 `json:"actions"`
}

// JournalEntryV1 是日志条目（来自事件流）。
type JournalEntryV1 struct {
	Seq  int64  `json:"seq"`
	Turn int    `json:"turn"`
	Time string `json:"time"`
	Kind string `json:"kind"` // check / gold / item / relation / move / story / condition / world
	Text string `json:"text"`
}

// SlotV1 是存档列表项。
type SlotV1 struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PlayerName string `json:"player_name"`
	Location   string `json:"location"`
	Time       string `json:"time"`
	Turn       int    `json:"turn"`
	Gold       int    `json:"gold"`
	Story      string `json:"story,omitempty"`
	UpdatedAt  int64  `json:"updated_at"`
	CreatedAt  int64  `json:"created_at"`
	Current    bool   `json:"current"`
}

// AIStatusV1 是 AI 设置状态（永不返回密钥）。
type AIStatusV1 struct {
	Kind      string `json:"kind"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	HasKey    bool   `json:"has_key"`
	Online    bool   `json:"online"`
	LastError string `json:"last_error,omitempty"`
}

// StreamEventV1 是推送给 UI 的流式事件。
type StreamEventV1 struct {
	Version   string `json:"version"`
	Type      string `json:"type"` // turn_started / narration_delta / narration_done / turn_done
	CommandID string `json:"command_id"`
	Text      string `json:"text,omitempty"`
	Corrected bool   `json:"corrected,omitempty"`
	Source    string `json:"source,omitempty"`
}
