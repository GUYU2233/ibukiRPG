// Package change 定义 WorldChange（架构 V0.3 第 5 节）：结构化、可校验、可反向、带影响评分的世界修改。
//
// 叶子包：只含数据结构与纯函数。校验器在 internal/world/validate（需要游戏状态）。
package change

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
)

// 操作类型。uncreate / timeline_remove / timeline_uncancel 只由引擎生成（反向操作）。
const (
	OpCreate           = "create"
	OpPatch            = "patch"
	OpRetire           = "retire"
	OpRestore          = "restore"
	OpLink             = "link"
	OpUnlink           = "unlink"
	OpTimelineAdd      = "timeline_add"
	OpTimelinePatch    = "timeline_patch"
	OpTimelineCancel   = "timeline_cancel"
	OpUncreate         = "uncreate"
	OpTimelineRemove   = "timeline_remove"
	OpTimelineUncancel = "timeline_uncancel"
)

// PublicOps 是 AI / 玩家 / MCP 可以提议的操作。
var PublicOps = []string{OpCreate, OpPatch, OpRetire, OpRestore, OpLink, OpUnlink, OpTimelineAdd, OpTimelinePatch, OpTimelineCancel}

// 来源。
const (
	SourceNarrate     = "narrate"
	SourceCombat      = "combat"
	SourceWorldSim    = "world_sim"
	SourceAudit       = "audit"
	SourceUserRequest = "user_request"
	SourceMCP         = "mcp"
	SourceCreation    = "creation"
	SourcePack        = "pack"
)

// SourceLabel 返回来源的中文名（日志页筛选）。
func SourceLabel(src string) string {
	switch {
	case src == SourceNarrate:
		return "AI 叙事"
	case src == SourceCombat:
		return "战斗"
	case src == SourceWorldSim:
		return "世界模拟"
	case src == SourceAudit:
		return "审查修复"
	case src == SourceUserRequest:
		return "你的指令"
	case src == SourceCreation:
		return "角色创建"
	case strings.HasPrefix(src, SourceMCP):
		return "外部工具"
	case src == SourcePack:
		return "故事包"
	}
	return src
}

// Change 是一项世界修改。
type Change struct {
	ID     string          `json:"id,omitempty"`
	Op     string          `json:"op"`
	Target string          `json:"target"`
	Kind   string          `json:"kind,omitempty"`
	Path   string          `json:"path,omitempty"`
	Value  json.RawMessage `json:"value,omitempty"`
	Before json.RawMessage `json:"before,omitempty"`
	// Prev 是修改前覆盖层里该路径的原始补丁（空 = 没有补丁，使用故事包原值）；反向操作用它精确复原。
	Prev     json.RawMessage `json:"prev,omitempty"`
	Reason   string          `json:"reason"`
	Source   string          `json:"source,omitempty"`
	Impact   Impact          `json:"impact,omitempty"`
	Evidence string          `json:"evidence,omitempty"`
	// Hidden 表示这项修改涉及隐藏真相：玩家只看到脱敏摘要。
	Hidden bool `json:"hidden,omitempty"`
	// Summary 是引擎生成的玩家可见摘要。
	Summary string `json:"summary,omitempty"`
}

// Impact 是单项或整回合的影响评估（引擎计算，0–100）。
type Impact struct {
	Score int      `json:"score,omitempty"`
	Level string   `json:"level,omitempty"` // AI 自评：none / minor / major / break
	Types []string `json:"types,omitempty"` // lore_deviation / major_death / story_impact
}

// 提示类型（第 7.1 节）。
const (
	TypeLoreDeviation = "lore_deviation"
	TypeMajorDeath    = "major_death"
	TypeStoryImpact   = "story_impact"
)

// TypePriority 是同一回合多种提示同时满足时的显示优先级。
var TypePriority = []string{TypeMajorDeath, TypeLoreDeviation, TypeStoryImpact}

// TypeLabel 返回提示类型中文名。
func TypeLabel(t string) string {
	switch t {
	case TypeLoreDeviation:
		return "彻底偏离设定"
	case TypeMajorDeath:
		return "重要角色死亡"
	case TypeStoryImpact:
		return "严重影响故事"
	}
	return t
}

// SelfLevelScore 把 AI 自评等级换算为分数（只能抬高引擎分数）。
func SelfLevelScore(level string) int {
	switch level {
	case "minor":
		return 30
	case "major":
		return 60
	case "break":
		return 85
	}
	return 0
}

// IsPlayerPath 报告 patch 路径是否指向玩家的机械状态（金钱 / 经验 / 物品 / 生命）。这类修改的值是整数增量。
func IsPlayerPath(target, path string) bool {
	return target == "player" && (path == "gold" || path == "xp" || path == "hp" || strings.HasPrefix(path, "inventory."))
}

// IsDeltaPath 报告 patch 的值是否是整数增量（玩家机械状态、关系维度 dims.<维度>）。
func IsDeltaPath(target, path string) bool {
	return IsPlayerPath(target, path) || (strings.Contains(target, ">") && strings.HasPrefix(path, "dims."))
}

// IsMechanical 报告修改是否涉及机械状态（物品、金钱、经验、生死）。审查修复对这类修改只给建议（第 19 节决定 4）。
func (c Change) IsMechanical(kindOf func(string) string) bool {
	if IsPlayerPath(c.Target, c.Path) {
		return true
	}
	if (c.Op == OpRetire || c.Op == OpRestore) && kindOf(c.Target) == "character" {
		return true
	}
	return false
}

// Inverse 返回撤销这项修改的反向操作（引擎在提交 WorldChangeReverted 时使用）。
func (c Change) Inverse() (Change, error) {
	inv := Change{Target: c.Target, Kind: c.Kind, Path: c.Path, Reason: "撤销：" + c.Reason, Hidden: c.Hidden}
	switch c.Op {
	case OpPatch, OpTimelinePatch:
		inv.Op = c.Op
		if IsDeltaPath(c.Target, c.Path) {
			var d int
			if err := json.Unmarshal(c.Value, &d); err != nil {
				return inv, fmt.Errorf("inverse: %w", err)
			}
			inv.Value, _ = json.Marshal(-d)
			return inv, nil
		}
		if c.Path == "location" || c.Op == OpTimelinePatch {
			inv.Value, inv.Before = c.Before, c.Value
			return inv, nil
		}
		inv.Value = c.Prev
		if len(inv.Value) == 0 {
			inv.Value = json.RawMessage("null")
		}
		inv.Prev, inv.Before = c.Value, c.Value
	case OpCreate:
		inv.Op, inv.Before = OpUncreate, c.Value
	case OpUncreate:
		inv.Op, inv.Value = OpCreate, c.Before
	case OpRetire:
		inv.Op, inv.Value = OpRestore, c.Value
	case OpRestore:
		inv.Op, inv.Value = OpRetire, c.Value
	case OpLink:
		if len(c.Before) == 0 || string(c.Before) == "null" {
			inv.Op, inv.Before = OpUnlink, c.Value
		} else {
			inv.Op, inv.Value, inv.Before = OpLink, c.Before, c.Value
		}
	case OpUnlink:
		inv.Op, inv.Value = OpLink, c.Before
	case OpTimelineAdd:
		inv.Op, inv.Before = OpTimelineRemove, c.Value
	case OpTimelineRemove:
		inv.Op, inv.Value = OpTimelineAdd, c.Before
	case OpTimelineCancel:
		inv.Op, inv.Value = OpTimelineUncancel, c.Before
	case OpTimelineUncancel:
		inv.Op, inv.Before = OpTimelineCancel, c.Value
	default:
		return inv, fmt.Errorf("inverse: unknown op %q", c.Op)
	}
	return inv, nil
}

// ---------- 影响评分（第 7.2 节）----------

// Meta 是评分需要的实体元数据（由校验器从生效世界取得）。
type Meta struct {
	Kind       string
	Importance int
	Canon      string
	Locked     []string
	Pivotal    bool
	// OutcomeChange 表示 timeline_patch 改变了 pivotal 事件的结局。
	OutcomeChange bool
	// RetireReason 是 retire 的原因（death / destroyed / disbanded / lost）。
	RetireReason string
}

var importanceMul = []float64{1.0, 0.4, 0.7, 1.0, 1.4, 2.0}

// Score 计算单项修改的影响分。
func Score(c Change, m Meta) int {
	base := 0.0
	switch c.Op {
	case OpCreate, OpUncreate:
		base = 5
	case OpPatch:
		base = 10
	case OpLink, OpUnlink:
		base = 5
	case OpRetire, OpRestore:
		base = 30
	case OpTimelineCancel, OpTimelineUncancel:
		base = 25
	case OpTimelineAdd, OpTimelineRemove:
		base = 5
	case OpTimelinePatch:
		base = 10
		if m.OutcomeChange {
			base = 30
		}
	}
	imp := m.Importance
	if imp < 1 || imp > 5 {
		imp = 3
	}
	v := base * importanceMul[imp]
	switch m.Canon {
	case "minor":
		v += 5
	case "major":
		v += 15
	case "core":
		v += 35
	}
	if c.Op == OpPatch && lockedPath(m.Locked, c.Path) {
		v += 20
	}
	if m.Pivotal {
		v += 25
	}
	if c.Op == OpCreate && m.Importance >= 4 && strings.HasPrefix(c.Kind, "timeline") {
		v = math.Max(v, 60)
	}
	return min(int(math.Round(v)), 100)
}

func lockedPath(locked []string, path string) bool {
	p := strings.TrimPrefix(path, "fields.")
	for _, l := range locked {
		l = strings.TrimPrefix(l, "fields.")
		if l == p || strings.HasPrefix(p, l+".") {
			return true
		}
	}
	return false
}

// Types 判定单项修改触发的提示类型（不含阈值）；deathImportance 是 major_death 的最低重要度。
func Types(c Change, m Meta) []string {
	var out []string
	if c.Op == OpRetire && m.Kind == "character" && m.RetireReason == "death" {
		out = append(out, TypeMajorDeath)
	}
	if m.Canon == "core" && c.Op != OpCreate {
		out = append(out, TypeLoreDeviation)
	}
	if m.Pivotal && (c.Op == OpTimelineCancel || m.OutcomeChange) {
		out = append(out, TypeLoreDeviation)
	}
	if m.Kind == "rule" && c.Op != OpCreate {
		out = append(out, TypeLoreDeviation)
	}
	if c.Op == OpPatch && lockedPath(m.Locked, c.Path) {
		out = append(out, TypeLoreDeviation)
	}
	if c.Op == OpRetire && (m.Kind == "faction" || (m.Kind == "location" && m.Importance >= 3)) {
		out = append(out, TypeStoryImpact)
	}
	if c.Op == OpPatch && m.Kind == "faction" && (c.Path == "fields.leader" || c.Path == "fields.territory") && m.Importance >= 3 {
		out = append(out, TypeStoryImpact)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// TurnImpact 是一个回合的影响汇总。
type TurnImpact struct {
	Score int      `json:"score"`
	Types []string `json:"types,omitempty"`
	// DeathImportance 是本回合死亡人物中的最高重要度（major_death 灵敏度比较用）。
	DeathImportance int      `json:"death_importance,omitempty"`
	Changes         []string `json:"changes,omitempty"`
	HiddenReveals   int      `json:"hidden_reveals,omitempty"`
}

// HiddenRevealScore 是每条没有 reveal_when 的隐藏真相被揭示时计入 story_impact 的分数（第 8.4 节）。
const HiddenRevealScore = 15

// Aggregate 汇总一个回合：最大单项分 + 其余各项分之和 × 0.25（上限 100）；AI 自评只能抬高；
// 自由揭示的隐藏真相每条 +15 并计入 story_impact。
func Aggregate(scores []int, selfLevel string, hiddenReveals int) int {
	if len(scores) == 0 && hiddenReveals == 0 {
		return 0
	}
	s := slices.Clone(scores)
	slices.Sort(s)
	total := 0.0
	if len(s) > 0 {
		total = float64(s[len(s)-1])
		rest := 0
		for _, v := range s[:len(s)-1] {
			rest += v
		}
		total += float64(rest) * 0.25
	}
	total += float64(hiddenReveals * HiddenRevealScore)
	v := min(int(math.Round(total)), 100)
	if sl := SelfLevelScore(selfLevel); sl > v && len(scores) > 0 {
		v = sl
	}
	return v
}

// ---------- 灵敏度（第 7.3 节）----------

// 灵敏度档位。
const (
	SensOff  = "off"
	SensLow  = "low"
	SensMid  = "mid"
	SensHigh = "high"
)

// 提示模式。
const (
	ModeModal  = "modal"
	ModeNotify = "notify"
)

// Sensitivity 是一类提示的灵敏度与模式。
type Sensitivity struct {
	Level string `json:"level"`
	Mode  string `json:"mode"`
}

// Settings 是三类提示的设置（App 设置，不属于存档）。
type Settings struct {
	LoreDeviation Sensitivity `json:"lore_deviation"`
	MajorDeath    Sensitivity `json:"major_death"`
	StoryImpact   Sensitivity `json:"story_impact"`
	// AutoCheckpoint：重要决定前自动创建检查点（默认开）；KeepAuto：自动检查点保留数。
	AutoCheckpoint  *bool `json:"auto_checkpoint,omitempty"`
	KeepAuto        int   `json:"keep_auto,omitempty"`
	DailyCheckpoint *bool `json:"daily_checkpoint,omitempty"`
}

// DefaultSettings 返回默认设置（全部“中”、弹窗确认、自动检查点开、保留 20 个）。
func DefaultSettings() Settings {
	mid := Sensitivity{Level: SensMid, Mode: ModeModal}
	return Settings{LoreDeviation: mid, MajorDeath: mid, StoryImpact: mid, KeepAuto: 20}
}

// AutoCheckpointOn 报告是否开启重要决定前的自动检查点。
func (s Settings) AutoCheckpointOn() bool { return s.AutoCheckpoint == nil || *s.AutoCheckpoint }

// DailyCheckpointOn 报告是否开启每日检查点。
func (s Settings) DailyCheckpointOn() bool { return s.DailyCheckpoint == nil || *s.DailyCheckpoint }

// For 返回某类提示的设置（未设置时为默认“中 / 弹窗”）。
func (s Settings) For(typ string) Sensitivity {
	var v Sensitivity
	switch typ {
	case TypeLoreDeviation:
		v = s.LoreDeviation
	case TypeMajorDeath:
		v = s.MajorDeath
	case TypeStoryImpact:
		v = s.StoryImpact
	}
	if v.Level == "" {
		v.Level = SensMid
	}
	if v.Mode == "" {
		v.Mode = ModeModal
	}
	return v
}

// ScoreThreshold 返回 lore_deviation / story_impact 的分数阈值（关 = 不提示，返回 -1）。
func ScoreThreshold(level string) int {
	switch level {
	case SensOff:
		return -1
	case SensLow:
		return 80
	case SensHigh:
		return 40
	}
	return 60
}

// DeathThreshold 返回 major_death 触发所需的最低重要度（关 = -1）。
func DeathThreshold(level string) int {
	switch level {
	case SensOff:
		return -1
	case SensLow:
		return 5
	case SensHigh:
		return 3
	}
	return 4
}

// ThresholdText 返回灵敏度的说明文字（设置页实时说明）。
func ThresholdText(typ, level string) string {
	if level == SensOff {
		return "不提示（仍写入日志）"
	}
	if typ == TypeMajorDeath {
		return fmt.Sprintf("重要度 ≥ %d 的人物死亡时提示", DeathThreshold(level))
	}
	return fmt.Sprintf("影响分 ≥ %d 时提示", ScoreThreshold(level))
}

// Prompt 是灵敏度判定的结果。
type Prompt struct {
	Type   string `json:"type"`
	Notify bool   `json:"notify"` // true：仅通知，不打断
	Score  int    `json:"score"`
}

// Decide 按灵敏度判定本回合是否需要提示；多种类型同时满足时按优先级只返回一个。
func Decide(t TurnImpact, s Settings) *Prompt {
	for _, typ := range TypePriority {
		sens := s.For(typ)
		hit := false
		switch typ {
		case TypeMajorDeath:
			th := DeathThreshold(sens.Level)
			hit = th > 0 && slices.Contains(t.Types, TypeMajorDeath) && t.DeathImportance >= th
		case TypeLoreDeviation:
			th := ScoreThreshold(sens.Level)
			hit = th > 0 && slices.Contains(t.Types, TypeLoreDeviation) && t.Score >= th
		case TypeStoryImpact:
			th := ScoreThreshold(sens.Level)
			// story_impact：显式触发条件（势力毁灭、真相大面积揭示……）或整回合影响分超过阈值
			hit = th > 0 && t.Score >= th
		}
		if hit {
			return &Prompt{Type: typ, Notify: sens.Mode == ModeNotify, Score: t.Score}
		}
	}
	return nil
}

// ---------- 待决选择（第 7.4 节）----------

// Ref 指向某分支某位置。
type Ref struct {
	Branch string `json:"branch,omitempty"`
	Seq    int64  `json:"seq"`
	Turn   int    `json:"turn"`
}

// Decision 是一次偏离提示（DecisionRequested 的载荷，持久化在状态里）。
type Decision struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Score      int      `json:"score"`
	Changes    []string `json:"changes"`
	Summary    string   `json:"summary"`
	Title      string   `json:"title,omitempty"`
	Lines      []string `json:"lines,omitempty"` // “将会改变”列表（脱敏）
	Rollback   Ref      `json:"rollback"`
	Checkpoint string   `json:"checkpoint,omitempty"`
	Notify     bool     `json:"notify,omitempty"`
	World      bool     `json:"world,omitempty"` // 由场外世界模拟产生
	Outcome    string   `json:"outcome,omitempty"`
}
