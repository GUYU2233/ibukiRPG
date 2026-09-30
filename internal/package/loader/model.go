package loader

import (
	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
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

// Dialogue 是模板叙事器使用的台词库（叙事内容，不是游戏事实）。
type Dialogue struct {
	Greet      []string            `yaml:"greet"`
	Friendly   []string            `yaml:"friendly"`
	Wary       []string            `yaml:"wary"`
	Story      map[string][]string `yaml:"story"`
	AfterStory map[string][]string `yaml:"after_story"`
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
	return id
}

// PlayerID 是玩家角色的固定 ID。
const PlayerID = "player"
