package combat

// ---------- 机械甲胄卡（v0.1.2-rc2） ----------
//
// 机甲是独立于普通装备的卡片类型：有自己的立绘、规格（装甲 / 出力 / 机动 / 能源容量 / 过热阈值 / 同步率 …）、
// 改装槽（安装改装件）、武器挂点（挂载武器）与状态（已整备 / 战损 / 未激活 / 丢失 / 封印 / 维修中 …）。
// 玩家没有在故事中得知的字段一律显示“未知”，由事件揭示。

// MechStatusDef 是机甲状态（故事包可扩展）。
type MechStatusDef struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	// Tone：positive / warning / danger / neutral（UI 颜色）。
	Tone string `yaml:"tone"`
	// Engage 为 true 时可以在战斗中启动。
	Engage bool `yaml:"engage"`
	// StatPct 是该状态下机甲战斗数值的百分比（0 视为 100）。
	StatPct int `yaml:"stat_pct"`
}

// DefaultMechStatuses 是默认状态集合。
var DefaultMechStatuses = []MechStatusDef{
	{ID: "ready", Name: "已整备", Tone: "positive", Engage: true},
	{ID: "damaged", Name: "战损", Tone: "warning", Engage: true, StatPct: 80},
	{ID: "inactive", Name: "未激活", Tone: "neutral"},
	{ID: "lost", Name: "丢失", Tone: "danger"},
	{ID: "sealed", Name: "封印", Tone: "danger"},
	{ID: "repair", Name: "维修中", Tone: "warning"},
}

// MechSpecDef 是机甲规格项（仅展示与改装计算用；战斗数值见 MechDef.Stats）。
type MechSpecDef struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	Max  int    `yaml:"max"`
	Unit string `yaml:"unit"`
}

// DefaultMechSpecs 是默认规格项。
var DefaultMechSpecs = []MechSpecDef{
	{ID: "armor", Name: "装甲", Max: 100},
	{ID: "output", Name: "出力", Max: 100},
	{ID: "mobility", Name: "机动", Max: 100},
	{ID: "capacity", Name: "能源容量", Max: 200},
	{ID: "heat_threshold", Name: "过热阈值", Max: 200},
	{ID: "sync", Name: "同步率", Max: 100, Unit: "%"},
}

// MechSlotDef 是改装槽或武器挂点。Kind 用于匹配改装件 / 武器的 mech.slot（为空表示通用）。
type MechSlotDef struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	Kind string `yaml:"kind"`
}

// MechItem 是改装件 / 挂载武器的机甲属性（写在物品的 mech 字段）。
type MechItem struct {
	// Slot 是可安装的槽位类型（匹配 MechSlotDef.Kind；为空表示任意）。
	Slot string `yaml:"slot"`
	// Hardpoint 为 true 表示这是挂载武器（装在武器挂点），否则是改装件（装在改装槽）。
	Hardpoint bool `yaml:"hardpoint"`
	// Stats 是加到机甲战斗数值上的修正；Specs 是加到规格上的修正。
	Stats Stats          `yaml:"stats"`
	Specs map[string]int `yaml:"specs"`
	// Skills 是安装后机甲获得的技能（挂载武器的攻击方式）。
	Skills []string `yaml:"skills"`
	// HeatMax / Capacity 修正过热上限与每回合能源消耗。
	HeatMax int `yaml:"heat_max"`
	PerTurn int `yaml:"mercury_per_turn"`
	// Mechs 限定只能装在哪些机甲上（为空表示不限）。
	Mechs []string `yaml:"mechs"`
}

// 可揭示的机甲字段（MechDef.Hidden 中使用）。
const (
	MechFieldPilot  = "pilot"
	MechFieldModel  = "model"
	MechFieldMaker  = "maker"
	MechFieldLore   = "lore"
	MechFieldStats  = "stats"
	MechFieldStatus = "status"
	MechFieldImage  = "image"
	// 前缀字段：spec:<id> / slot:<id> / hardpoint:<id> / skill:<id>
	MechFieldSpec      = "spec:"
	MechFieldSlot      = "slot:"
	MechFieldHardpoint = "hardpoint:"
	MechFieldSkill     = "skill:"
)

// MechStatus 返回状态定义（未知状态按“已整备”处理）。
func (c *Config) MechStatus(id string) MechStatusDef {
	for _, s := range c.MechStatuses {
		if s.ID == id {
			return s
		}
	}
	return MechStatusDef{ID: id, Name: id, Tone: "neutral", Engage: true}
}

// MechSpec 返回规格定义。
func (c *Config) MechSpec(id string) MechSpecDef {
	for _, s := range c.MechSpecs {
		if s.ID == id {
			return s
		}
	}
	return MechSpecDef{ID: id, Name: id, Max: 100}
}

// HasMechStatus 报告状态是否已声明。
func (c *Config) HasMechStatus(id string) bool {
	for _, s := range c.MechStatuses {
		if s.ID == id {
			return true
		}
	}
	return false
}

// HasMechSpec 报告规格是否已声明。
func (c *Config) HasMechSpec(id string) bool {
	for _, s := range c.MechSpecs {
		if s.ID == id {
			return true
		}
	}
	return false
}
