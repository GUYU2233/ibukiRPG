package loader

import (
	"io/fs"

	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
)

// Skill 是技能定义。
type Skill struct {
	ID        string `yaml:"id"`
	Name      string `yaml:"name"`
	Attribute string `yaml:"attribute"`
}

// Rules 是 rules/skills.yaml 的内容。
type Rules struct {
	Skills     []Skill           `yaml:"skills"`
	Attributes map[string]string `yaml:"attributes"`
	Conditions map[string]string `yaml:"conditions"`
	// ConditionModifiers: condition → skill → 修正值。
	ConditionModifiers map[string]map[string]int `yaml:"condition_modifiers"`
}

// Ambient 是一条环境事件候选。
type Ambient struct {
	Location string `yaml:"location"`
	Text     string `yaml:"text"`
	Fact     string `yaml:"fact"`
}

// Pacing 是 PacingProfile。
type Pacing struct {
	Profile            string    `yaml:"profile"`
	QuietTurnsForNudge int       `yaml:"quiet_turns_for_nudge"`
	Ambient            []Ambient `yaml:"ambient"`
}

// Exit 是地点出口。
type Exit struct {
	To            string `yaml:"to"`
	Label         string `yaml:"label"`
	Minutes       int    `yaml:"minutes"`
	Requires      string `yaml:"requires"`
	LockedMessage string `yaml:"locked_message"`
}

// Shop 描述地点里的商店。
type Shop struct {
	Seller string   `yaml:"seller"`
	Items  []string `yaml:"items"`
}

// Search 是调查动作的地点文本。
type Search struct {
	Success string `yaml:"success"`
	Failure string `yaml:"failure"`
}

// Location 是地点定义。
type Location struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Aliases     []string       `yaml:"aliases"`
	Tags        []string       `yaml:"tags"`
	Description string         `yaml:"description"`
	Short       string         `yaml:"short"`
	Properties  map[string]any `yaml:"properties"`
	Exits       []Exit         `yaml:"exits"`
	SceneFacts  []string       `yaml:"scene_facts"`
	Shop        *Shop          `yaml:"shop"`
	Search      Search         `yaml:"search"`
}

// Identity 是角色身份。
type Identity struct {
	Name string `yaml:"name"`
	Age  int    `yaml:"age"`
	Role string `yaml:"role"`
}

// Secret 是角色秘密；Keywords 供 Narrative Guard 检测泄露。
type Secret struct {
	ID       string   `yaml:"id"`
	Text     string   `yaml:"text"`
	Keywords []string `yaml:"keywords"`
}

// DialogueLine 是台词池中的一条台词：由 CEL 条件（when）决定能否说，
// 引擎按优先级与“说过没有”挑选，并以 DialogueOccurred 事件记录（对话记忆）。
type DialogueLine struct {
	ID       string `yaml:"id"`
	Topic    string `yaml:"topic"`
	When     string `yaml:"when"`
	Priority int    `yaml:"priority"`
	// Once 表示只说一次；否则说过之后仍可重复（但未说过的台词总是优先）。
	Once bool   `yaml:"once"`
	Text string `yaml:"text"`
	// Memory 是这次交谈的摘要，写入 NPC 的对话记忆（以及 AI 上下文）。为空时使用话题名。
	Memory string `yaml:"memory"`
	// Remember 非空时，同时写入一条 NPC 的情节记忆（例如“把储藏室的怪事告诉了玩家”）。
	Remember string `yaml:"remember"`
	// Reveals 是这句台词透露的关系（"from>to"，角色 ID），玩家由此“听说”这段关系（关系网只显示玩家知道的）。
	Reveals []string `yaml:"reveals"`
	// Relation 是这句台词带来的说话者→玩家关系变化（例如 {affection: 2}），只在第一次说时生效。
	Relation map[string]int `yaml:"relation"`
}

// Dialogue 是台词库（叙事内容，不是游戏事实）。
//
// 推荐使用 lines（带条件的台词池）；greet / friendly / wary / story / after_story 是旧格式，
// 加载时会自动转换为 lines。
type Dialogue struct {
	Greet      []string            `yaml:"greet"`
	Friendly   []string            `yaml:"friendly"`
	Wary       []string            `yaml:"wary"`
	Story      map[string][]string `yaml:"story"`
	AfterStory map[string][]string `yaml:"after_story"`
	// Topics：话题 ID → 中文名。
	Topics map[string]string `yaml:"topics"`
	Lines  []DialogueLine    `yaml:"lines"`
	// Repeat：没有新话可说时使用的模板，可用 {last_said} {last_topic} {player} {name}。
	Repeat []string `yaml:"repeat"`
	// Callback：NPC 提起自上次交谈后目击到的玩家行为，可用 {what} {player} {name}。
	Callback []string `yaml:"callback"`
}

// Line 按 ID 查找台词。
func (d *Dialogue) Line(id string) *DialogueLine {
	for i := range d.Lines {
		if d.Lines[i].ID == id {
			return &d.Lines[i]
		}
	}
	return nil
}

// TopicName 返回话题中文名（未声明则原样返回）。
func (d *Dialogue) TopicName(id string) string {
	if n, ok := d.Topics[id]; ok {
		return n
	}
	return id
}

// Character 是角色定义（NPC 与玩家模板共用）。
type Character struct {
	ID                   string                       `yaml:"id"`
	Type                 string                       `yaml:"type"`
	Identity             Identity                     `yaml:"identity"`
	Aliases              []string                     `yaml:"aliases"`
	Location             string                       `yaml:"location"`
	Description          string                       `yaml:"description"`
	Attributes           map[string]int               `yaml:"attributes"`
	Skills               map[string]int               `yaml:"skills"`
	Personality          struct{ Description string } `yaml:"personality"`
	Goals                []string                     `yaml:"goals"`
	Traits               []string                     `yaml:"traits"`
	RelationshipToPlayer map[string]int               `yaml:"relationship_to_player"`
	Dialogue             Dialogue                     `yaml:"dialogue"`
	Secrets              []Secret                     `yaml:"secrets"`
	Tags                 []string                     `yaml:"tags"`
	Gold                 int                          `yaml:"gold"`
	Inventory            map[string]int               `yaml:"inventory"`
	// ---- v0.1.2-rc2：战斗、角色卡、图鉴与立绘 ----
	Combat *combat.CharacterCombat `yaml:"combat"`
	// Card 是角色卡等级：major（开局即有角色卡）/ minor（满足 promote 条件后升格）/ none。
	// 为空时视为 major（兼容旧故事包）。
	Card        string   `yaml:"card"`
	Promote     *Promote `yaml:"promote"`
	ArchiveWhen string   `yaml:"archive_when"`
	Faction     string   `yaml:"faction"`
	Icon        string   `yaml:"icon"`
	Lore        string   `yaml:"lore"`
	// Portrait 是立绘图片在包内的相对路径（assets/ 下，png/jpg/webp，≤ 2 MB）；可省略。
	Portrait string `yaml:"portrait"`
}

// Promote 是次要角色升格为主要角色（创建角色卡）的量化条件：满足任意一条即可。
type Promote struct {
	// Score：互动分（交谈 ×2 + 情节记忆 + 并肩作战 ×3 + 与之相关的关系变化）达到该值。
	Score int `yaml:"score"`
	// When：CEL 条件（例如剧情触发）。
	When   string `yaml:"when"`
	Reason string `yaml:"reason"`
}

// CardTier 返回角色卡等级（默认 major）。
func (c *Character) CardTier() string {
	if c.Card == "" {
		return "major"
	}
	return c.Card
}

// Name 返回显示名。
func (c *Character) Name() string { return c.Identity.Name }

// ItemUse 描述物品的使用效果。
type ItemUse struct {
	Verb      string `yaml:"verb"`
	Condition string `yaml:"condition"`
	Text      string `yaml:"text"`
}

// Item 是物品定义。
type Item struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Aliases     []string `yaml:"aliases"`
	Price       int      `yaml:"price"`
	Unsellable  bool     `yaml:"unsellable"`
	Description string   `yaml:"description"`
	Use         *ItemUse `yaml:"use"`
	// ---- v0.1.1-rc2：装备、消耗品与介绍卡 ----
	Kind   string             `yaml:"kind"` // consumable / weapon / armor / accessory / mech_part / mech_weapon / key / material
	Rarity string             `yaml:"rarity"`
	Icon   string             `yaml:"icon"`
	Slot   string             `yaml:"slot"`
	Mods   combat.Stats       `yaml:"mods"`
	Lore   string             `yaml:"lore"`
	Combat *combat.ItemCombat `yaml:"combat"`
	// Level 是装备需求等级。
	Level int `yaml:"level"`
	// Mech 是机甲改装件 / 挂载武器的属性（kind: mech_part / mech_weapon）。
	Mech *combat.MechItem `yaml:"mech"`
}

// StoryStep 是故事中的非终结步骤或结局。
type StoryStep struct {
	ID        string               `yaml:"id"`
	Title     string               `yaml:"title"`
	Once      bool                 `yaml:"once"`
	When      string               `yaml:"when"`
	Effects   []definition.Outcome `yaml:"effects"`
	Narration string               `yaml:"narration"`
}

// HUDField 是故事包声明的一项实时状态显示（第 3 项：HUD）。
//
// 值来源二选一：bind（内置绑定：location / time / clock / day / period / gold / turn / story /
// objective / conditions / var:<名字> / flag:<名字>）或 value（CEL 表达式）。
type HUDField struct {
	ID      string `yaml:"id"`
	Label   string `yaml:"label"`
	Icon    string `yaml:"icon"`
	Bind    string `yaml:"bind"`
	Value   string `yaml:"value"`
	Format  string `yaml:"format"`  // 例如 "{value}%"
	Visible string `yaml:"visible"` // CEL；为空总是显示
	Order   int    `yaml:"order"`
	// Compact 为 false 时只在展开的状态卡中显示（默认 true）。
	Compact *bool `yaml:"compact"`
	// Max > 0 时渲染为进度条（值 / Max）。
	Max  int    `yaml:"max"`
	Tone string `yaml:"tone"` // normal / warning / danger / success
	// Tones：色调 → CEL 条件（按 danger、warning、success 的顺序取第一个成立的），覆盖 Tone。
	Tones map[string]string `yaml:"tones"`
	// Wide 为 true 时在紧凑模式下单独占一行（适合“当前目标”等长文本）。
	Wide bool `yaml:"wide"`
}

// IsCompact 报告字段是否出现在紧凑行。
func (h HUDField) IsCompact() bool { return h.Compact == nil || *h.Compact }

// Objective 是主线目标：按顺序取第一个 when 成立的目标。
type Objective struct {
	When string `yaml:"when"`
	Text string `yaml:"text"`
}

// Hint 是故事提示（会作为快捷建议出现）。
type Hint struct {
	Label string `yaml:"label"`
	Text  string `yaml:"text"`
}

// Story 是一个 Story Node（第 23 节）。
type Story struct {
	ID             string               `yaml:"id"`
	Title          string               `yaml:"title"`
	Priority       string               `yaml:"priority"`
	Constraint     string               `yaml:"constraint"`
	Location       string               `yaml:"location"`
	Trigger        string               `yaml:"trigger"`
	PacingTrigger  string               `yaml:"pacing_trigger"`
	Intro          string               `yaml:"intro"`
	IntroEffects   []definition.Outcome `yaml:"intro_effects"`
	Hints          []Hint               `yaml:"hints"`
	TimeoutMinutes int                  `yaml:"timeout_minutes"`
	Steps          []StoryStep          `yaml:"steps"`
	Outcomes       []StoryStep          `yaml:"outcomes"`
}

// Package 是加载完成的内容包。所有有序切片保持 manifest 中的声明顺序，
// 引擎遍历时只用有序切片，不依赖 map 顺序（第 30 节）。
type Package struct {
	// FS 是包的文件系统（用于读取立绘等资源），不参与序列化。
	FS          fs.FS `json:"-" yaml:"-"`
	Manifest    *manifest.Manifest
	Rules       Rules
	Pacing      Pacing
	Locations   map[string]*Location
	LocationIDs []string
	Characters  map[string]*Character
	NPCIDs      []string
	Player      *Character
	Items       map[string]*Item
	ItemIDs     []string
	Actions     map[string]*definition.Definition
	ActionIDs   []string
	Stories     map[string]*Story
	StoryIDs    []string
	// HUD 是故事包声明的状态栏；为空时查询层使用默认 HUD。
	HUD        []HUDField
	Objectives []Objective
	// Variables 是故事变量及其初始值（manifest start.variables）。
	Variables map[string]int
	// ---- v0.1.1-rc2 ----
	Combat *combat.Content
	Codex  []CodexEntry
	// Prompts 是提示词段落补丁（manifest content.prompts）。
	Prompts   []PromptPatch
	Relations Relations
}

// Key 返回命名空间化 ID 的最后一段，例如 demo:action/talk → talk。
func Key(id string) string {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '/' || id[i] == ':' {
			return id[i+1:]
		}
	}
	return id
}

// ActionID 返回本包中某类动作的完整 ID（例如 talk → demo:action/talk）；不存在时返回空串。
func (p *Package) ActionID(key string) string {
	id := p.Manifest.Namespace + ":action/" + key
	if _, ok := p.Actions[id]; ok {
		return id
	}
	for _, aid := range p.ActionIDs {
		if Key(aid) == key {
			return aid
		}
	}
	return ""
}

// DialogueAction 返回标记为 dialogue 的交谈动作 ID（没有则回退到 key 为 talk 的动作）。
func (p *Package) DialogueAction() string {
	for _, id := range p.ActionIDs {
		if p.Actions[id].Dialogue {
			return id
		}
	}
	return p.ActionID("talk")
}

// SkillByID 查找技能。
func (p *Package) SkillByID(id string) (Skill, bool) {
	for _, s := range p.Rules.Skills {
		if s.ID == id {
			return s, true
		}
	}
	return Skill{}, false
}

// SkillName 返回技能中文名（未知则原样返回）。
func (p *Package) SkillName(id string) string {
	if s, ok := p.SkillByID(id); ok {
		return s.Name
	}
	return id
}

// ConditionName 返回状态中文名。
func (p *Package) ConditionName(id string) string {
	if n, ok := p.Rules.Conditions[id]; ok {
		return n
	}
	return id
}

// EntityName 返回任意 ID（角色、地点、物品）的显示名。
func (p *Package) EntityName(id string) string {
	if id == PlayerID {
		return p.Player.Name()
	}
	if c, ok := p.Characters[id]; ok {
		return c.Name()
	}
	if l, ok := p.Locations[id]; ok {
		return l.Name
	}
	if it, ok := p.Items[id]; ok {
		return it.Name
	}
	if p.Combat != nil {
		if e, ok := p.Combat.Enemies[id]; ok {
			return e.Name
		}
		if s, ok := p.Combat.Skills[id]; ok {
			return s.Name
		}
		if m, ok := p.Combat.Mechs[id]; ok {
			return m.Name
		}
		if e, ok := p.Combat.Encounters[id]; ok {
			return e.Title
		}
	}
	for _, e := range p.Codex {
		if e.ID == id {
			return e.Name
		}
	}
	return id
}

// PlayerID 是玩家角色的固定 ID。
const PlayerID = "player"

// DrinkItem 返回 order_drink 动作提供的饮品物品 ID（没有则为空）。
func (p *Package) DrinkItem() string {
	id := p.ActionID("order_drink")
	if id == "" {
		return ""
	}
	for _, o := range p.Actions[id].Outcomes["success"] {
		if o.Effect.Type == "item_add" {
			if it, ok := o.Effect.Values["item"].(string); ok {
				return it
			}
		}
	}
	return ""
}

// ShopKeeper 返回地点的店主（没有商店为空）。
func (p *Package) ShopKeeper(loc string) string {
	if l, ok := p.Locations[loc]; ok && l.Shop != nil {
		return l.Shop.Seller
	}
	return ""
}
