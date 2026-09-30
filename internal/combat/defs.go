package combat

import (
	"slices"

	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
)

// 数值键：角色、敌人、机甲共用同一套数值模型。
const (
	HP   = "hp"
	SP   = "sp"
	ATK  = "atk"
	DEF  = "def"
	SPD  = "spd"
	ACC  = "acc"
	EVA  = "eva"
	CRIT = "crit"
)

// StatKeys 是数值的展示顺序。
var StatKeys = []string{HP, SP, ATK, DEF, SPD, ACC, EVA, CRIT}

// StatNames 是数值中文名。
var StatNames = map[string]string{HP: "生命", SP: "体力", ATK: "攻击", DEF: "防御", SPD: "速度", ACC: "命中", EVA: "闪避", CRIT: "暴击"}

// Stats 是一组整数数值。
type Stats map[string]int

// Add 返回 a + b（不修改输入）。
func (a Stats) Add(b Stats) Stats {
	out := Stats{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] += v
	}
	return out
}

// Scale 返回 a × n（逐项）。
func (a Stats) Scale(n int) Stats {
	out := Stats{}
	for k, v := range a {
		out[k] = v * n
	}
	return out
}

// Slot 是装备槽位。
type Slot struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// DefaultSlots 是故事包未声明时的装备槽位。
var DefaultSlots = []Slot{{"weapon", "武器"}, {"armor", "护甲"}, {"accessory", "饰品"}}

// Rarity 是稀有度。
type Rarity struct {
	ID    string `yaml:"id"`
	Name  string `yaml:"name"`
	Color string `yaml:"color"`
}

// DefaultRarities 是默认稀有度。
var DefaultRarities = []Rarity{
	{"common", "普通", "#8A8A8A"}, {"uncommon", "精良", "#3B8B4F"}, {"rare", "稀有", "#2F6DB5"},
	{"epic", "史诗", "#8446B0"}, {"legendary", "传说", "#C8791E"},
}

// Config 是 combat/rules.yaml 的全局规则。
type Config struct {
	// AttributeBase：属性超过该值的部分才产生数值加成（默认 10）。
	AttributeBase int `yaml:"attribute_base"`
	// AttributeEffects：属性 → 数值 → 每点加成。
	AttributeEffects map[string]Stats `yaml:"attribute_effects"`
	// XPBase：升到 L+1 级需要 XPBase × L 经验（默认 100）。
	XPBase              int      `yaml:"xp_base"`
	MaxLevel            int      `yaml:"max_level"`
	AttrPointsPerLevel  int      `yaml:"attr_points_per_level"`
	SkillPointsPerLevel int      `yaml:"skill_points_per_level"`
	Slots               []Slot   `yaml:"slots"`
	Rarities            []Rarity `yaml:"rarities"`
	// ResourceName 是机甲能源的名字（默认“红水银”），MercuryMax 为上限。
	ResourceName string `yaml:"resource_name"`
	MercuryMax   int    `yaml:"mercury_max"`
	// SPRegenPct：每个己方回合开始恢复的体力百分比（默认 10）。
	SPRegenPct int `yaml:"sp_regen_pct"`
	MaxRounds  int `yaml:"max_rounds"`
	// DefeatConditions：战败分支 → 附加的状态（例如 injured → 重伤）。
	DefeatConditions map[string]string `yaml:"defeat_conditions"`
	// MechStatuses / MechSpecs：机甲状态与规格项（为空使用默认；故事包可扩展）。
	MechStatuses []MechStatusDef `yaml:"mech_statuses"`
	MechSpecs    []MechSpecDef   `yaml:"mech_specs"`
	// MechDestroyedStatus 是机甲在战斗中被击毁后的状态（默认 damaged）。
	MechDestroyedStatus string `yaml:"mech_destroyed_status"`
}

// Normalize 填充默认值。
func (c *Config) Normalize() {
	if c.AttributeBase == 0 {
		c.AttributeBase = 10
	}
	if c.XPBase == 0 {
		c.XPBase = 100
	}
	if c.MaxLevel == 0 {
		c.MaxLevel = 20
	}
	if c.AttrPointsPerLevel == 0 {
		c.AttrPointsPerLevel = 2
	}
	if c.SkillPointsPerLevel == 0 {
		c.SkillPointsPerLevel = 1
	}
	if len(c.Slots) == 0 {
		c.Slots = DefaultSlots
	}
	if len(c.Rarities) == 0 {
		c.Rarities = DefaultRarities
	}
	if c.ResourceName == "" {
		c.ResourceName = "红水银"
	}
	if c.MercuryMax == 0 {
		c.MercuryMax = 100
	}
	if c.SPRegenPct == 0 {
		c.SPRegenPct = 10
	}
	if c.MaxRounds == 0 {
		c.MaxRounds = 30
	}
	if len(c.MechStatuses) == 0 {
		c.MechStatuses = DefaultMechStatuses
	}
	if len(c.MechSpecs) == 0 {
		c.MechSpecs = DefaultMechSpecs
	}
	if c.MechDestroyedStatus == "" {
		c.MechDestroyedStatus = "damaged"
	}
}

// XPToNext 返回从 level 升到 level+1 需要的经验。
func (c *Config) XPToNext(level int) int { return c.XPBase * max(level, 1) }

// SlotName 返回槽位中文名。
func (c *Config) SlotName(id string) string {
	for _, s := range c.Slots {
		if s.ID == id {
			return s.Name
		}
	}
	return id
}

// RarityOf 返回稀有度（未知时为 common）。
func (c *Config) RarityOf(id string) Rarity {
	for _, r := range c.Rarities {
		if r.ID == id {
			return r
		}
	}
	if id == "" && len(c.Rarities) > 0 {
		return c.Rarities[0]
	}
	return Rarity{ID: id, Name: id, Color: "#8A8A8A"}
}

// RarityRank 返回稀有度序号（越大越稀有）。
func (c *Config) RarityRank(id string) int {
	return max(slices.IndexFunc(c.Rarities, func(r Rarity) bool { return r.ID == id }), 0)
}

// StatusDef 是状态效果（流血、灼烧、眩晕、护盾……）。
type StatusDef struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Icon        string `yaml:"icon"`
	Description string `yaml:"description"`
	Turns       int    `yaml:"turns"`
	// DOT 是每回合伤害（负数为治疗）；DotPct 为最大生命的百分比伤害。
	DOT    int   `yaml:"dot"`
	DotPct int   `yaml:"dot_pct"`
	Mods   Stats `yaml:"mods"`
	Stun   bool  `yaml:"stun"`
	// Debuff 为 true 表示负面状态（会被净化）。
	Debuff bool `yaml:"debuff"`
}

// StatusApply 描述技能附加状态。
type StatusApply struct {
	ID     string `yaml:"id"`
	Chance int    `yaml:"chance"` // 百分比，0 视为 100
	Turns  int    `yaml:"turns"`
}

// Cost 是技能消耗。
type Cost struct {
	SP      int `yaml:"sp"`
	Mercury int `yaml:"mercury"`
	Heat    int `yaml:"heat"`
}

// Learn 是技能学习条件。
type Learn struct {
	Level    int      `yaml:"level"`
	Cost     int      `yaml:"cost"`
	Requires []string `yaml:"requires"`
}

// 技能目标。
const (
	TargetEnemy      = "enemy"
	TargetAllEnemies = "all_enemies"
	TargetSelf       = "self"
	TargetAlly       = "ally"
	TargetAllAllies  = "all_allies"
)

// SkillDef 是战斗技能。
type SkillDef struct {
	ID          string       `yaml:"id"`
	Name        string       `yaml:"name"`
	Icon        string       `yaml:"icon"`
	Rarity      string       `yaml:"rarity"`
	Description string       `yaml:"description"`
	Lore        string       `yaml:"lore"`
	Aliases     []string     `yaml:"aliases"`
	Target      string       `yaml:"target"`
	Power       int          `yaml:"power"` // 攻击力百分比；0 表示不造成伤害
	Hits        int          `yaml:"hits"`
	AccBonus    int          `yaml:"acc_bonus"`
	CritBonus   int          `yaml:"crit_bonus"`
	Pierce      int          `yaml:"pierce"` // 无视防御百分比
	Heal        int          `yaml:"heal"`
	HealPct     int          `yaml:"heal_pct"`
	Restore     Cost         `yaml:"restore"` // 恢复体力 / 红水银 / 降低热量（heat 为降温量）
	Cost        Cost         `yaml:"cost"`
	Status      *StatusApply `yaml:"status"`
	Cleanse     bool         `yaml:"cleanse"`
	MechOnly    bool         `yaml:"mech_only"`
	HumanOnly   bool         `yaml:"human_only"`
	Learn       *Learn       `yaml:"learn"`
	Verb        string       `yaml:"verb"` // 战斗记录动词，例如“挥出”
}

// AIRule 是敌人 / 同伴的确定性行动规则：按顺序取第一个满足 when 的规则。
// when 是 CEL（变量 self / foes / allies / round，均为 map），target 为
// lowest_hp / highest_hp / random / self / weakest_ally / player / first。
type AIRule struct {
	When   string `yaml:"when"`
	Skill  string `yaml:"skill"` // 空 = 普通攻击；"defend" = 防御
	Target string `yaml:"target"`
	Chance int    `yaml:"chance"` // 百分比，0 视为 100（使用确定性 RNG 流）
}

// Drop 是掉落。
type Drop struct {
	Item   string `yaml:"item"`
	Chance int    `yaml:"chance"`
	Qty    int    `yaml:"qty"`
}

// EnemyDef 是敌人定义（与角色共用数值模型）。
type EnemyDef struct {
	ID      string   `yaml:"id"`
	Name    string   `yaml:"name"`
	Aliases []string `yaml:"aliases"`
	Icon    string   `yaml:"icon"`
	Rarity  string   `yaml:"rarity"`
	Tier    string   `yaml:"tier"` // minion / elite / boss
	// MechCard 是这个敌人驾驶 / 本身就是的机甲卡 ID（遭遇时解锁该机甲卡）。
	MechCard    string   `yaml:"mech_card"`
	Faction     string   `yaml:"faction"`
	Description string   `yaml:"description"`
	Lore        string   `yaml:"lore"`
	Level       int      `yaml:"level"`
	Stats       Stats    `yaml:"stats"`
	Skills      []string `yaml:"skills"`
	AI          []AIRule `yaml:"ai"`
	XP          int      `yaml:"xp"`
	Gold        int      `yaml:"gold"`
	Drops       []Drop   `yaml:"drops"`
	Mech        bool     `yaml:"mech"`
	Tags        []string `yaml:"tags"`
}

// MechDef 是机甲形态（例如炽天使）：独立数值、技能与能源 / 热量规则。
type MechDef struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Icon        string   `yaml:"icon"`
	Rarity      string   `yaml:"rarity"`
	Description string   `yaml:"description"`
	Lore        string   `yaml:"lore"`
	Stats       Stats    `yaml:"stats"`
	Skills      []string `yaml:"skills"`
	EngageCost  int      `yaml:"engage_cost"`      // 启动消耗红水银
	PerTurn     int      `yaml:"mercury_per_turn"` // 每回合消耗
	HeatMax     int      `yaml:"heat_max"`
	AttackHeat  int      `yaml:"attack_heat"` // 普通攻击产生的热量
	CoolPerTurn int      `yaml:"cool_per_turn"`
	// OverheatDamage：过热时驾驶者受到的伤害（生命百分比）。
	OverheatDamage int `yaml:"overheat_damage"`
	// Requires 是可以启动的 CEL 条件（例如 world.flags.dragon_awake）。
	Requires string `yaml:"requires"`

	// ---- 机甲卡（v0.1.2-rc2）----
	Model    string `yaml:"model"`    // 型号
	Maker    string `yaml:"maker"`    // 制造方
	Class    string `yaml:"class"`    // 分类（炽天使 / 量产型 / 蒸汽甲胄 …）
	Pilot    string `yaml:"pilot"`    // 驾驶者角色 ID（可为空）
	Portrait string `yaml:"portrait"` // 立绘（assets/ 下）
	// Status 是初始状态（默认 ready）。
	Status string         `yaml:"status"`
	Specs  map[string]int `yaml:"specs"`
	// ModSlots 改装槽；Hardpoints 武器挂点；Installed / Mounted 是初始安装的改装件 / 武器（槽位 → 物品 ID）。
	ModSlots   []MechSlotDef     `yaml:"mod_slots"`
	Hardpoints []MechSlotDef     `yaml:"hardpoints"`
	Installed  map[string]string `yaml:"installed"`
	Mounted    map[string]string `yaml:"mounted"`
	// Hidden 是玩家起初不知道的字段（pilot / model / maker / lore / stats / status / image / spec:<id> / slot:<id> / hardpoint:<id> / skill:<id>）。
	Hidden []string `yaml:"hidden"`
	// Known 为 true 时开局就解锁这张机甲卡（否则在见到 / 启动 / 被揭示时解锁）。
	Known bool `yaml:"known"`
	// Enemy 为 true 表示这是敌方机甲（不能被玩家改装）。
	Enemy bool `yaml:"enemy"`
}

// Defeat 描述战败分支：不是游戏结束，而是被俘 / 重伤 / 获救。
type Defeat struct {
	Branch  string               `yaml:"branch"` // captured / injured / rescued
	Text    string               `yaml:"text"`
	MoveTo  string               `yaml:"move_to"`
	HPPct   int                  `yaml:"hp_pct"`
	Effects []definition.Outcome `yaml:"effects"`
}

// Encounter 是一场遭遇战。
type Encounter struct {
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Location string   `yaml:"location"`
	Intro    string   `yaml:"intro"`
	Enemies  []string `yaml:"enemies"`
	Allies   []string `yaml:"allies"`
	// NoFlee 禁止逃跑；AllowMech 允许启动机甲形态。
	NoFlee    bool `yaml:"no_flee"`
	AllowMech bool `yaml:"allow_mech"`
	// Available 为空时该遭遇只能由剧情效果（combat_start）触发；否则在地点处显示“挑战”按钮。
	Available   string               `yaml:"available"`
	Label       string               `yaml:"label"`
	Repeatable  bool                 `yaml:"repeatable"`
	Victory     []definition.Outcome `yaml:"victory"`
	VictoryText string               `yaml:"victory_text"`
	Defeat      Defeat               `yaml:"defeat"`
	Fled        []definition.Outcome `yaml:"fled"`
	XPBonus     int                  `yaml:"xp_bonus"`
	GoldBonus   int                  `yaml:"gold_bonus"`
}

// CharacterCombat 是角色（玩家 / 同伴 NPC）的战斗块。
type CharacterCombat struct {
	Level  int      `yaml:"level"`
	Stats  Stats    `yaml:"stats"`
	Growth Stats    `yaml:"growth"` // 每级成长
	Skills []string `yaml:"skills"`
	AI     []AIRule `yaml:"ai"`
	Mech   string   `yaml:"mech"`
	// Equipment：槽位 → 物品 ID（玩家初始装备）。
	Equipment map[string]string `yaml:"equipment"`
	Mercury   int               `yaml:"mercury"`
}

// ItemCombat 是物品在战斗中的用法。
type ItemCombat struct {
	Heal    int          `yaml:"heal"`
	HealPct int          `yaml:"heal_pct"`
	SP      int          `yaml:"sp"`
	Mercury int          `yaml:"mercury"`
	Cool    int          `yaml:"cool"`
	Damage  int          `yaml:"damage"`
	Cure    []string     `yaml:"cure"`
	Status  *StatusApply `yaml:"status"`
	Target  string       `yaml:"target"` // self / ally / enemy / all_enemies
	// Field 为 true 时也可以在战斗外使用（治疗、补充红水银）。
	Field bool `yaml:"field"`
}

// Content 是故事包的全部战斗内容。
type Content struct {
	Config     Config
	Statuses   map[string]*StatusDef
	StatusIDs  []string
	Skills     map[string]*SkillDef
	SkillIDs   []string
	Enemies    map[string]*EnemyDef
	EnemyIDs   []string
	Mechs      map[string]*MechDef
	MechIDs    []string
	Encounters map[string]*Encounter
	EncIDs     []string
}

// NewContent 返回空内容。
func NewContent() *Content {
	return &Content{Statuses: map[string]*StatusDef{}, Skills: map[string]*SkillDef{}, Enemies: map[string]*EnemyDef{}, Mechs: map[string]*MechDef{}, Encounters: map[string]*Encounter{}}
}
