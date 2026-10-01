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
	Kind        string `json:"kind"` // action / move / text / combat / manage / wait
	Action      string `json:"action,omitempty"`
	Target      string `json:"target,omitempty"`
	Item        string `json:"item,omitempty"`
	Destination string `json:"destination,omitempty"`
	Skill       string `json:"skill,omitempty"` // kind=combat / manage 时的技能
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
	// Combat 是战斗记录（kind=combat）：一次攻击 / 技能的掷骰与伤害分解。
	Combat *CombatLogV1 `json:"combat,omitempty"`
	// World 是世界更新（kind=world）：知识解锁 chip、世界变更 chip、失败提示。
	World *WorldLogV1 `json:"world,omitempty"`
	// Adjudication 是行动裁定卡（kind=adjudication）。
	Adjudication *AdjudicationV1 `json:"adjudication,omitempty"`
	// Usage 是本回合 token 用量（kind=usage）。
	Usage *UsageV1 `json:"usage,omitempty"`
}

// CombatLogV1 是一条战斗记录。
type CombatLogV1 struct {
	Actor    string   `json:"actor"`
	Target   string   `json:"target,omitempty"`
	Action   string   `json:"action"`
	SkillID  string   `json:"skill_id,omitempty"`
	ItemID   string   `json:"item_id,omitempty"`
	Hit      bool     `json:"hit"`
	Roll     int      `json:"roll,omitempty"`
	Chance   int      `json:"chance,omitempty"`
	Damage   int      `json:"damage,omitempty"`
	Critical bool     `json:"critical,omitempty"`
	Side     string   `json:"side,omitempty"` // party / enemy
	Chips    []string `json:"chips,omitempty"`
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
	// Portrait 为 true 表示故事包提供了立绘（通过 get_portrait 取图），否则 UI 显示占位头像。
	Portrait bool `json:"portrait,omitempty"`
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
	// Hud 是故事包声明的实时状态（每回合重新求值）。
	Hud []HudFieldV1 `json:"hud"`
	// PackID / PackName 是当前存档绑定的故事包。
	PackID   string `json:"pack_id,omitempty"`
	PackName string `json:"pack_name,omitempty"`
	// Combat 非空表示正在战斗（界面切换为战斗面板）。
	Combat *CombatV1 `json:"combat,omitempty"`
	// HasRPG 表示故事包带数值 RPG 内容（显示图鉴 / 关系网 / 成长入口）。
	HasRPG bool `json:"has_rpg,omitempty"`
	// Decision 是待决（或仅通知）的偏离提示；待决时输入被禁用。
	Decision *DecisionV1 `json:"decision,omitempty"`
	// Upcoming 是已知的世界事件倒计时（状态条）。
	Upcoming []WorldEventChipV1 `json:"upcoming,omitempty"`
	// Branch 是当前分支名；PendingTurn > 0 表示已“回到回合 N”，继续行动将创建新分支。
	Branch      string `json:"branch,omitempty"`
	PendingTurn int    `json:"pending_turn,omitempty"`
	// AISuggestions 是 AI 给出的行动建议（✨，来自 WORLD 段）。
	AISuggestions []string `json:"ai_suggestions,omitempty"`
}

// StatusChipV1 是单位身上的状态。
type StatusChipV1 struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Icon   string `json:"icon,omitempty"`
	Turns  int    `json:"turns"`
	Debuff bool   `json:"debuff,omitempty"`
	Note   string `json:"note,omitempty"`
}

// CombatUnitV1 是战斗单位。
type CombatUnitV1 struct {
	ID        string         `json:"id"`
	Ref       string         `json:"ref"`
	Name      string         `json:"name"`
	Side      string         `json:"side"`
	Level     int            `json:"level"`
	HP        int            `json:"hp"`
	MaxHP     int            `json:"max_hp"`
	SP        int            `json:"sp"`
	MaxSP     int            `json:"max_sp"`
	Mech      bool           `json:"mech,omitempty"`
	MechName  string         `json:"mech_name,omitempty"`
	PilotHP   int            `json:"pilot_hp,omitempty"`
	Heat      int            `json:"heat,omitempty"`
	HeatMax   int            `json:"heat_max,omitempty"`
	Statuses  []StatusChipV1 `json:"statuses"`
	Down      bool           `json:"down,omitempty"`
	Current   bool           `json:"current,omitempty"`
	Defending bool           `json:"defending,omitempty"`
	Icon      string         `json:"icon,omitempty"`
	Tier      string         `json:"tier,omitempty"`
	// Portrait 为 true 表示故事包提供了立绘（通过 get_portrait 取图），否则 UI 显示占位头像。
	Portrait bool `json:"portrait,omitempty"`
	// Parts 是自由战斗的部位（0.2.0）：未知部位只显示名字，弱点要先发现。
	Parts []CombatPartV1 `json:"parts,omitempty"`
	Size  string         `json:"size,omitempty"`
}

// CombatPartV1 是敌人的一个部位。
type CombatPartV1 struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Known  bool   `json:"known"`
	Weak   bool   `json:"weak,omitempty"` // 仅 Known 时填写
	Broken bool   `json:"broken,omitempty"`
}

// CombatActionV1 是战斗面板上的一个行动按钮。
type CombatActionV1 struct {
	Kind     string `json:"kind"` // attack / skill / item / defend / flee / mech / eject
	ID       string `json:"id,omitempty"`
	Label    string `json:"label"`
	Icon     string `json:"icon,omitempty"`
	Hint     string `json:"hint,omitempty"`
	Target   string `json:"target"` // enemy / ally / self / all_enemies / all_allies / none
	Disabled bool   `json:"disabled,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Qty      int    `json:"qty,omitempty"`
}

// CombatV1 是战斗面板。
type CombatV1 struct {
	Encounter    string           `json:"encounter"`
	Title        string           `json:"title"`
	Round        int              `json:"round"`
	YourTurn     bool             `json:"your_turn"`
	Party        []CombatUnitV1   `json:"party"`
	Enemies      []CombatUnitV1   `json:"enemies"`
	Actions      []CombatActionV1 `json:"actions"`
	Skills       []CombatActionV1 `json:"skills"`
	Items        []CombatActionV1 `json:"items"`
	Order        []string         `json:"order"`
	ResourceName string           `json:"resource_name"`
	Mercury      int              `json:"mercury"`
	MercuryMax   int              `json:"mercury_max"`
}

// NodeV1 是动态主线节点。
type NodeV1 struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Objective string `json:"objective"`
	Goal      string `json:"goal"`
	Source    string `json:"source"`
	Reward    string `json:"reward,omitempty"`
	Location  string `json:"location,omitempty"`
}

// NoticeV1 是本回合的通知（新角色卡、升级、图鉴解锁……），界面以 Snackbar 呈现。
type NoticeV1 struct {
	Kind string `json:"kind"` // card / card_archived / card_restored / death / level / codex / mainline / node
	Text string `json:"text"`
	Ref  string `json:"ref,omitempty"`
}

// KVV1 是一行“标签：值”。
type KVV1 struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// CardV1 是图鉴 / 介绍卡：物品、装备、技能、敌人、机甲、势力、地点、角色共用。
type CardV1 struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	KindName    string   `json:"kind_name"`
	Name        string   `json:"name"`
	Icon        string   `json:"icon,omitempty"`
	Rarity      string   `json:"rarity,omitempty"`
	RarityName  string   `json:"rarity_name,omitempty"`
	RarityColor string   `json:"rarity_color,omitempty"`
	Description string   `json:"description,omitempty"`
	Lore        string   `json:"lore,omitempty"`
	Stats       []KVV1   `json:"stats,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Known       bool     `json:"known"`
}

// CodexCategoryV1 是图鉴分类。
type CodexCategoryV1 struct {
	Kind     string   `json:"kind"`
	Name     string   `json:"name"`
	Unlocked int      `json:"unlocked"`
	Entries  []CardV1 `json:"entries"`
}

// CodexV1 是图鉴。
type CodexV1 struct {
	Categories []CodexCategoryV1 `json:"categories"`
	Unlocked   int               `json:"unlocked"`
	Total      int               `json:"total"`
}

// DimValueV1 是关系的一个维度。
type DimValueV1 struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Value    int    `json:"value"`
	Negative bool   `json:"negative,omitempty"`
}

// EdgeV1 是关系网中的一条（有向）关系：只包含玩家知道的。
type EdgeV1 struct {
	From     string       `json:"from"`
	To       string       `json:"to"`
	FromName string       `json:"from_name"`
	ToName   string       `json:"to_name"`
	Values   []DimValueV1 `json:"values"`
	Label    string       `json:"label"`
	Tone     string       `json:"tone"` // positive / negative / neutral / mixed
	Source   string       `json:"source"`
	Turn     int          `json:"turn,omitempty"`
	Note     string       `json:"note,omitempty"`
}

// PersonV1 是关系网中的一个人。
type PersonV1 struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role,omitempty"`
	Icon   string `json:"icon,omitempty"`
	Card   string `json:"card,omitempty"` // active / archived / dead / 空
	Player bool   `json:"player,omitempty"`
	// Portrait 为 true 表示故事包提供了立绘（通过 get_portrait 取图），否则 UI 显示占位头像。
	Portrait bool `json:"portrait,omitempty"`
}

// RelChangeV1 是关系变化历史。
type RelChangeV1 struct {
	Turn int    `json:"turn"`
	Time string `json:"time"`
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text"`
}

// RelationsV1 是关系网（玩家视角）。
type RelationsV1 struct {
	Dimensions []DimValueV1  `json:"dimensions"`
	People     []PersonV1    `json:"people"`
	Edges      []EdgeV1      `json:"edges"`
	History    []RelChangeV1 `json:"history"`
}

// CharacterCardV1 是角色卡。
type CharacterCardV1 struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Role        string       `json:"role,omitempty"`
	Icon        string       `json:"icon,omitempty"`
	Description string       `json:"description,omitempty"`
	Lore        string       `json:"lore,omitempty"`
	Status      string       `json:"status"`
	Reason      string       `json:"reason,omitempty"`
	Faction     string       `json:"faction,omitempty"`
	Level       int          `json:"level,omitempty"`
	Stats       []StatV1     `json:"stats,omitempty"`
	Relation    []DimValueV1 `json:"relation,omitempty"`
	Memories    []string     `json:"memories,omitempty"`
	Location    string       `json:"location,omitempty"`
	Dynamic     bool         `json:"dynamic,omitempty"`
	Turn        int          `json:"turn"`
	// Portrait 为 true 表示故事包提供了立绘（通过 get_portrait 取图），否则 UI 显示占位头像。
	Portrait bool `json:"portrait,omitempty"`
	// Mechs 是玩家已知由该角色驾驶的机甲卡 ID。
	Mechs []string `json:"mechs,omitempty"`
}

// CardsV1 是角色卡列表。
type CardsV1 struct {
	Active   []CharacterCardV1 `json:"active"`
	Archived []CharacterCardV1 `json:"archived"`
	Dead     []CharacterCardV1 `json:"dead"`
}

// EquipSlotV1 是装备槽位。
type EquipSlotV1 struct {
	Slot    string         `json:"slot"`
	Name    string         `json:"name"`
	Item    *CardV1        `json:"item,omitempty"`
	Unequip *QuickActionV1 `json:"unequip,omitempty"`
}

// SkillCardV1 是技能面板中的一项。
type SkillCardV1 struct {
	Card    CardV1         `json:"card"`
	Learned bool           `json:"learned"`
	Learn   *QuickActionV1 `json:"learn,omitempty"`
	Blocked string         `json:"blocked,omitempty"`
	Cost    int            `json:"cost,omitempty"`
}

// GrowthV1 是成长面板（等级、经验、属性点、数值、装备、技能）。
type GrowthV1 struct {
	Level        int           `json:"level"`
	XP           int           `json:"xp"`
	XPNext       int           `json:"xp_next"`
	AttrPoints   int           `json:"attr_points"`
	SkillPoints  int           `json:"skill_points"`
	HP           int           `json:"hp"`
	MaxHP        int           `json:"max_hp"`
	Mercury      int           `json:"mercury"`
	MercuryMax   int           `json:"mercury_max"`
	ResourceName string        `json:"resource_name"`
	HasMech      bool          `json:"has_mech,omitempty"`
	MechName     string        `json:"mech_name,omitempty"`
	Stats        []StatV1      `json:"stats"`
	Equipment    []EquipSlotV1 `json:"equipment"`
	Skills       []SkillCardV1 `json:"skills"`
	// Attributes 是可分配属性点的基础属性（Note 为每点加成说明）。
	Attributes []AttributeV1 `json:"attributes"`
}

// AttributeV1 是一项可加点的基础属性。
type AttributeV1 struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Value    int            `json:"value"`
	Effect   string         `json:"effect,omitempty"`
	Allocate *QuickActionV1 `json:"allocate,omitempty"`
}

// HudFieldV1 是 HUD 的一项。
type HudFieldV1 struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Icon    string `json:"icon,omitempty"`
	Value   string `json:"value"`
	Compact bool   `json:"compact"`
	Wide    bool   `json:"wide,omitempty"`
	Tone    string `json:"tone,omitempty"`
	// Progress 为 0-1000 的千分比；-1 表示不是进度条。
	Progress int `json:"progress"`
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
	Notices     []NoticeV1     `json:"notices,omitempty"`
	Usage       *UsageV1       `json:"usage,omitempty"`
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
	// Growth 是数值 RPG 成长信息（故事包带战斗内容时）。
	Growth *GrowthV1 `json:"growth,omitempty"`
	// Portrait 为 true 表示故事包提供了立绘（通过 get_portrait 取图），否则 UI 显示占位头像。
	Portrait bool `json:"portrait,omitempty"`
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
	// 装备 / 介绍卡信息（v0.1.1-rc2）。
	Card     *CardV1        `json:"card,omitempty"`
	Equipped bool           `json:"equipped,omitempty"`
	Equip    *QuickActionV1 `json:"equip,omitempty"`
	// Mech 非空表示这是机甲改装件 / 挂载武器，点开时跳到该机甲卡进行安装。
	Mech string `json:"mech,omitempty"`
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
	// Talks 是和玩家交谈过的次数；Memories 是 NPC 对玩家的记忆摘要（最近的交谈 + 重要经历，新的在前）。
	Talks    int      `json:"talks"`
	Memories []string `json:"memories"`
	// Portrait 为 true 表示故事包提供了立绘（通过 get_portrait 取图），否则 UI 显示占位头像。
	Portrait bool `json:"portrait,omitempty"`
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
	// 存档绑定的故事包；PackProblem 非空表示故事包缺失或不兼容（此时无法读取）。
	PackID      string `json:"pack_id"`
	PackName    string `json:"pack_name"`
	PackVersion string `json:"pack_version"`
	PackProblem string `json:"pack_problem,omitempty"`
}

// PackV1 是故事包选择界面的一张卡片。
type PackV1 struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Type        string   `json:"type"`
	Author      string   `json:"author"`
	Tagline     string   `json:"tagline,omitempty"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Icon        string   `json:"icon,omitempty"`
	Accent      string   `json:"accent,omitempty"`
	Cover       string   `json:"cover,omitempty"` // base64 编码的封面图片
	Engine      string   `json:"engine,omitempty"`
	Builtin     bool     `json:"builtin"`
	Playable    bool     `json:"playable"`
	Error       string   `json:"error,omitempty"`
	SaveCount   int      `json:"save_count"`
	IsDefault   bool     `json:"is_default,omitempty"`
}

// AIStatusV1 是 AI 设置状态（永不返回密钥）。
type AIStatusV1 struct {
	Kind      string `json:"kind"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	HasKey    bool   `json:"has_key"`
	Online    bool   `json:"online"`
	LastError string `json:"last_error,omitempty"`
	// Provider / Mode：0.2.0 多服务商（叙事任务的首选服务商与路由模式）。
	Provider      string   `json:"provider,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	LocalWarnings []string `json:"local_warnings,omitempty"`
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

// PortraitV1 是角色立绘（base64）。
type PortraitV1 struct {
	ID     string `json:"id"`
	Mime   string `json:"mime"`
	Base64 string `json:"base64"`
}

// ---------- 机械甲胄卡（v0.1.2-rc2） ----------

// MechFieldV1 是机甲卡上的一个文本字段；Known=false 时 Value 为“未知”。
type MechFieldV1 struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
	Known bool   `json:"known"`
}

// MechStatV1 是规格 / 战斗数值条；Known=false 时 Value 无意义（UI 显示“未知”）。
type MechStatV1 struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value int    `json:"value"`
	Max   int    `json:"max"`
	Unit  string `json:"unit,omitempty"`
	Bonus int    `json:"bonus,omitempty"` // 改装件带来的加成
	Known bool   `json:"known"`
}

// MechStatusV1 是机甲状态。
type MechStatusV1 struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Tone   string `json:"tone"`
	Engage bool   `json:"engage"`
	Known  bool   `json:"known"`
}

// MechSlotV1 是改装槽或武器挂点。
type MechSlotV1 struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Kind      string          `json:"kind,omitempty"`
	Hardpoint bool            `json:"hardpoint,omitempty"`
	Known     bool            `json:"known"`
	Part      *CardV1         `json:"part,omitempty"`
	Remove    *QuickActionV1  `json:"remove,omitempty"`
	Install   []QuickActionV1 `json:"install,omitempty"` // 背包里可以装进这个槽位的部件
}

// MechLogV1 是机甲履历。
type MechLogV1 struct {
	Turn int    `json:"turn"`
	Time string `json:"time,omitempty"`
	Text string `json:"text"`
}

// MechCardV1 是机械甲胄卡（独立于普通装备卡）。
type MechCardV1 struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Icon        string        `json:"icon,omitempty"`
	Rarity      string        `json:"rarity,omitempty"`
	RarityName  string        `json:"rarity_name,omitempty"`
	RarityColor string        `json:"rarity_color,omitempty"`
	Known       bool          `json:"known"` // 卡片是否已解锁
	Owned       bool          `json:"owned,omitempty"`
	Enemy       bool          `json:"enemy,omitempty"`
	Portrait    bool          `json:"portrait,omitempty"` // 有立绘且玩家已知
	Status      MechStatusV1  `json:"status"`
	Fields      []MechFieldV1 `json:"fields"` // 型号 / 制造方 / 分类 / 驾驶者
	PilotID     string        `json:"pilot_id,omitempty"`
	Specs       []MechStatV1  `json:"specs"`
	Stats       []MechStatV1  `json:"stats"`
	StatsKnown  bool          `json:"stats_known"`
	Energy      []MechFieldV1 `json:"energy"` // 启动消耗 / 每回合消耗 / 过热上限
	Slots       []MechSlotV1  `json:"slots"`
	Hardpoints  []MechSlotV1  `json:"hardpoints"`
	Skills      []CardV1      `json:"skills"`
	Description string        `json:"description,omitempty"`
	Lore        string        `json:"lore,omitempty"`
	LoreKnown   bool          `json:"lore_known"`
	History     []MechLogV1   `json:"history"`
	Unknown     int           `json:"unknown"` // 仍未知的字段数
}

// MechsV1 是机甲卡列表。
type MechsV1 struct {
	Mechs        []MechCardV1 `json:"mechs"`
	ResourceName string       `json:"resource_name"`
}
