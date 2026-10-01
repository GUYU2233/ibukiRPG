// Package freeform 是自由战斗裁定（架构 V0.3 第 9 节）：自然语言 → AI 解析为 CombatIntent →
// 引擎确定性地做合理性检查、修正、掷骰与程度判定。AI 不能写任何数值。
//
// 叶子包：只含意图结构、合理性检查表、修正表、随机性档位与程度判定的纯函数；掷骰由调用方注入（命名 RNG 流）。
package freeform

import (
	"fmt"
	"slices"
	"strings"
)

// 动作种类白名单（第 9.2 节）。
const (
	KindAttack   = "attack"
	KindSkill    = "skill"
	KindManeuver = "maneuver"
	KindMove     = "move"
	KindDefend   = "defend"
	KindItem     = "item"
	KindMech     = "mech"
	KindTalk     = "talk"
	KindFlee     = "flee"
	KindEnv      = "env"
)

// Kinds 是合法的动作种类。
var Kinds = []string{KindAttack, KindSkill, KindManeuver, KindMove, KindDefend, KindItem, KindMech, KindTalk, KindFlee, KindEnv}

// Action 是意图中的一个动作。
type Action struct {
	Kind     string `json:"kind"`
	To       string `json:"to,omitempty"`
	Target   string `json:"target,omitempty"`
	Part     string `json:"part,omitempty"`
	Means    string `json:"means,omitempty"`
	Skill    string `json:"skill,omitempty"`
	Style    string `json:"style,omitempty"`
	Maneuver string `json:"maneuver,omitempty"`
	Env      string `json:"env,omitempty"`
	Item     string `json:"item,omitempty"`
}

// Circumstance 是 AI 认为成立的情境修正标签（只能从修正标签表里选，不能写数值）。
type Circumstance struct {
	Tag string `json:"tag"`
	Why string `json:"why,omitempty"`
}

// Intent 是 combat_adjudicate 的结构化输出。
type Intent struct {
	Actions       []Action       `json:"actions"`
	Circumstances []Circumstance `json:"circumstances,omitempty"`
	Absurd        string         `json:"absurd,omitempty"`
	SelfCheck     string         `json:"self_check,omitempty"`
	// Raw 是玩家原文；Source 是解析来源（ai / local / rules）。
	Raw    string `json:"raw,omitempty"`
	Source string `json:"source,omitempty"`
}

// Main 返回意图中的主动作（非 move 的第一个）；没有则返回 move 或空动作。
func (in Intent) Main() Action {
	for _, a := range in.Actions {
		if a.Kind != KindMove {
			return a
		}
	}
	if len(in.Actions) > 0 {
		return in.Actions[0]
	}
	return Action{}
}

// ---------- 随机性档位（第 9.5 节）----------

// Profile 是由故事包 balance.randomness 决定的随机性档位。玩家不能覆盖（第 19 节决定 3）。
type Profile struct {
	Randomness int    `json:"randomness"`
	Band       string `json:"band"` // narrative / balanced / hardcore
	DiceN      int    `json:"dice_n"`
	DiceSides  int    `json:"dice_sides"`
	// CritFail 是大失败的差值阈值（0 = 关闭）；NatOne 表示掷出 1 也是大失败（硬核 d20）。
	CritFail        int    `json:"crit_fail"`
	NatOne          bool   `json:"nat_one"`
	VariancePct     int    `json:"variance_pct"`
	StrainedPenalty int    `json:"strained_penalty"`
	Consequence     string `json:"consequence"` // light / medium / heavy
	Defeat          string `json:"defeat"`      // captured / injured / death
}

// ProfileFor 返回随机性档位。
func ProfileFor(r int) Profile {
	r = max(0, min(100, r))
	switch {
	case r <= 30:
		return Profile{Randomness: r, Band: "narrative", DiceN: 3, DiceSides: 6, VariancePct: 10, StrainedPenalty: 0, Consequence: "light", Defeat: "captured"}
	case r <= 69:
		return Profile{Randomness: r, Band: "balanced", DiceN: 2, DiceSides: 10, CritFail: -8, VariancePct: 20, StrainedPenalty: 2, Consequence: "medium", Defeat: "injured"}
	}
	return Profile{Randomness: r, Band: "hardcore", DiceN: 1, DiceSides: 20, CritFail: -6, NatOne: true, VariancePct: 35, StrainedPenalty: 3, Consequence: "heavy", Defeat: "death"}
}

// BandLabel 返回档位中文名。
func (p Profile) BandLabel() string {
	switch p.Band {
	case "narrative":
		return "叙事"
	case "hardcore":
		return "硬核"
	}
	return "平衡"
}

// DiceSpec 返回骰子规格（3d6 / 2d10 / d20）。
func (p Profile) DiceSpec() string {
	if p.DiceN == 1 {
		return fmt.Sprintf("d%d", p.DiceSides)
	}
	return fmt.Sprintf("%dd%d", p.DiceN, p.DiceSides)
}

// Roll 用注入的掷骰函数掷一次。
func (p Profile) Roll(roll func(sides int) int) []int {
	out := make([]int, p.DiceN)
	for i := range out {
		out[i] = roll(p.DiceSides)
	}
	return out
}

// 程度。
const (
	CritFail = "critical_failure"
	Fail     = "failure"
	Partial  = "partial"
	Success  = "success"
	Crit     = "critical_success"
)

// DegreeLabel 返回程度中文名。
func DegreeLabel(d string) string {
	switch d {
	case CritFail:
		return "大失败"
	case Fail:
		return "失败"
	case Partial:
		return "勉强成功"
	case Success:
		return "成功"
	case Crit:
		return "大成功"
	}
	return d
}

// Degree 由差值判定程度（第 9.4 节）。
func Degree(margin int, rolls []int, p Profile) string {
	if p.NatOne && len(rolls) == 1 && rolls[0] == 1 {
		return CritFail
	}
	switch {
	case margin >= 8:
		return Crit
	case margin >= 2:
		return Success
	case margin >= 0:
		return Partial
	case p.CritFail != 0 && margin <= p.CritFail:
		return CritFail
	}
	return Fail
}

// Succeeded 报告程度是否算成功。
func Succeeded(d string) bool { return d == Partial || d == Success || d == Crit }

// Effect 是程度对应的效果：伤害倍率（百分比）、是否附加状态、状态额外回合。
func Effect(d string) (dmgPct int, applyStatus bool, extraTurns int) {
	switch d {
	case Partial:
		return 50, false, 0
	case Success:
		return 100, true, 0
	case Crit:
		return 150, true, 1
	}
	return 0, false, 0
}

// Consequences 返回失败后果（按档位从轻到重累积）：off_balance 失衡、counter 被反击、stamina 额外体力、jam 武器卡住、wound 受伤。
func Consequences(p Profile, degree string) []string {
	if Succeeded(degree) {
		return nil
	}
	out := []string{"off_balance"}
	switch p.Consequence {
	case "light":
		out = append(out, "stamina")
	case "medium":
		out = append(out, "counter")
	case "heavy":
		out = append(out, "counter")
		if degree == CritFail {
			out = append(out, "jam", "wound")
		}
	}
	if degree == CritFail && p.Consequence != "heavy" {
		out = append(out, "stamina")
	}
	return out
}

// ConsequenceLabel 返回后果中文名。
func ConsequenceLabel(c string) string {
	switch c {
	case "off_balance":
		return "失衡（下回合 −2）"
	case "counter":
		return "被反击"
	case "stamina":
		return "额外体力消耗"
	case "jam":
		return "武器卡住"
	case "wound":
		return "受伤"
	}
	return c
}

// ---------- 合理性检查（第 9.3 节）----------

// 合理性等级。
const (
	Plausible  = "ok"
	Strained   = "strained"
	Absurd     = "absurd"
	Impossible = "impossible"
)

// PlausibilityLabel 返回中文名。
func PlausibilityLabel(p string) string {
	switch p {
	case Plausible:
		return "合理"
	case Strained:
		return "勉强"
	case Absurd:
		return "荒谬 → 降级"
	case Impossible:
		return "不可能"
	}
	return p
}

// Part 是目标的部位。
type Part struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Mult  int    `json:"mult"` // 伤害倍率（百分比，默认 100；弱点通常 150）
	DC    int    `json:"dc"`   // 额外难度
	Weak  bool   `json:"weak,omitempty"`
	Known bool   `json:"known,omitempty"` // 玩家知识层已知该部位
}

// Target 是一个可选目标。
type Target struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Aliases  []string `json:"aliases,omitempty"`
	Size     string   `json:"size,omitempty"`
	Parts    []Part   `json:"parts,omitempty"`
	Statuses []string `json:"statuses,omitempty"` // 状态标签（cooling / stagger / prone / guarded ...）
	Unaware  bool     `json:"unaware,omitempty"`
	Defense  int      `json:"defense"` // DC 的目标防御修正
	Mech     bool     `json:"mech,omitempty"`
}

// Means 是可用的武器 / 工具。
type Means struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Aliases  []string `json:"aliases,omitempty"`
	Equipped bool     `json:"equipped,omitempty"`
	Quick    bool     `json:"quick,omitempty"`
	Owned    bool     `json:"owned,omitempty"`
	Pierce   bool     `json:"pierce,omitempty"` // 破甲：凡人武器对机甲有效
}

// SkillInfo 是角色掌握的技能。
type SkillInfo struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	Usable  bool     `json:"usable"`
	Why     string   `json:"why,omitempty"`
	Bonus   int      `json:"bonus,omitempty"` // 技能修正
}

// Maneuver 是故事包定义的战技（推撞 / 绊倒 / 缴械 / 擒抱 / 致盲……）。
type Maneuver struct {
	ID            string   `json:"id" yaml:"id"`
	Name          string   `json:"name" yaml:"name"`
	Keywords      []string `json:"keywords,omitempty" yaml:"keywords"`
	SizeSensitive bool     `json:"size_sensitive,omitempty" yaml:"size_sensitive"`
	DC            int      `json:"dc,omitempty" yaml:"dc"`
	Status        string   `json:"status,omitempty" yaml:"status"`
	Turns         int      `json:"turns,omitempty" yaml:"turns"`
	Power         int      `json:"power,omitempty" yaml:"power"` // 附带伤害（攻击力百分比，0 = 无伤害）
}

// Tag 是情境修正标签（第 9.3 节“修正标签前提”）。
type Tag struct {
	ID    string `json:"id" yaml:"id"`
	Label string `json:"label" yaml:"label"`
	Value int    `json:"value" yaml:"value"`
	// Requires：target_status:<a>|<b>（目标带任一状态标签）/ fact:<关键词>（场景事实）/ unaware（目标未察觉）/ 空（总成立）。
	Requires string `json:"requires,omitempty" yaml:"requires"`
	// Breach 表示破甲类标签：凡人武器对机甲有效的前提。
	Breach bool `json:"breach,omitempty" yaml:"breach"`
}

// Context 是裁定所需的局面信息（由引擎从状态构造）。
type Context struct {
	ActorSize   string      `json:"actor_size,omitempty"`
	SizeClasses []string    `json:"size_classes,omitempty"`
	Targets     []Target    `json:"targets"`
	Means       []Means     `json:"means,omitempty"`
	Skills      []SkillInfo `json:"skills,omitempty"`
	Maneuvers   []Maneuver  `json:"maneuvers,omitempty"`
	Tags        []Tag       `json:"tags,omitempty"`
	SceneFacts  []string    `json:"scene_facts,omitempty"`
	// AbsurdWords 是故事包能力边界的关键词（无魔法世界里的“火球”“咒语”）+ 引擎内置的物理不可能词。
	AbsurdWords []string `json:"absurd_words,omitempty"`
	SPLow       bool     `json:"sp_low,omitempty"`
	InMech      bool     `json:"in_mech,omitempty"`
	// AttrMod 是属性修正（引擎由数值计算）。
	AttrMod int `json:"attr_mod"`
	// Local 表示意图来自本地模型：只接受修正标签表内的标签，合理性按“正常”档。
	Local bool `json:"local,omitempty"`
}

// BuiltinAbsurd 是任何世界都荒谬的说法。
var BuiltinAbsurd = []string{"太阳", "月亮", "星球", "一拳打爆", "瞬移", "时间停止", "时间倒流", "无敌", "秒杀", "毁灭世界", "飞上天", "召唤神龙"}

// Mod 是一项修正。
type Mod struct {
	Label string `json:"label"`
	Value int    `json:"value"`
	Tag   string `json:"tag,omitempty"`
}

// Result 是合理性检查与修正的结果（掷骰前）。
type Result struct {
	Plausibility    string     `json:"plausibility"`
	Reason          string     `json:"reason,omitempty"` // impossible 时给玩家看的原因
	Options         []string   `json:"options,omitempty"`
	Final           Action     `json:"final"`
	Original        Action     `json:"original"`
	Moved           bool       `json:"moved,omitempty"`
	Downgraded      bool       `json:"downgraded,omitempty"`
	DowngradeReason string     `json:"downgrade_reason,omitempty"`
	Notes           []string   `json:"notes,omitempty"`
	Mods            []Mod      `json:"mods,omitempty"`
	ModTotal        int        `json:"mod_total"`
	DCParts         []Mod      `json:"dc_parts,omitempty"`
	DC              int        `json:"dc"`
	Target          *Target    `json:"target,omitempty"`
	Part            *Part      `json:"part,omitempty"`
	Gamble          bool       `json:"gamble,omitempty"` // 攻击未知部位：成功则揭示
	Maneuver        *Maneuver  `json:"maneuver,omitempty"`
	Means           *Means     `json:"means,omitempty"`
	Skill           *SkillInfo `json:"skill,omitempty"`
	NeedsRoll       bool       `json:"needs_roll"`
}

// MaxActions 是每回合最多的动作数（1 次移动 + 1 次主动作）。
const MaxActions = 2

func matchName(want, id, name string, aliases []string) bool {
	if want == "" {
		return false
	}
	if want == id || want == name || strings.HasSuffix(id, "/"+want) {
		return true
	}
	for _, a := range aliases {
		if want == a {
			return true
		}
	}
	return name != "" && (strings.Contains(want, name) || strings.Contains(name, want))
}

func (c Context) target(want string) *Target {
	for i := range c.Targets {
		t := &c.Targets[i]
		if matchName(want, t.ID, t.Name, t.Aliases) {
			return t
		}
	}
	return nil
}

func sizeIndex(classes []string, s string) int {
	if s == "" {
		s = "human"
	}
	return slices.Index(classes, s)
}

func (c Context) tagByID(id string) *Tag {
	for i := range c.Tags {
		if c.Tags[i].ID == id {
			return &c.Tags[i]
		}
	}
	return nil
}

func (c Context) hasFact(kw string) bool {
	for _, f := range c.SceneFacts {
		if strings.Contains(f, kw) {
			return true
		}
	}
	return false
}

// tagHolds 检查修正标签的前提。
func (c Context) tagHolds(t *Tag, tgt *Target) bool {
	req := t.Requires
	switch {
	case req == "":
		return true
	case req == "unaware":
		return tgt != nil && tgt.Unaware
	case strings.HasPrefix(req, "fact:"):
		for _, kw := range strings.Split(strings.TrimPrefix(req, "fact:"), "|") {
			if c.hasFact(kw) {
				return true
			}
		}
		return false
	case strings.HasPrefix(req, "target_status:"):
		if tgt == nil {
			return false
		}
		for _, st := range strings.Split(strings.TrimPrefix(req, "target_status:"), "|") {
			if slices.Contains(tgt.Statuses, st) {
				return true
			}
		}
		return false
	}
	return false
}

// Check 做合理性检查并计算修正与难度（第 9.3、9.4 节）。完全确定性。
func Check(ctx Context, in Intent, p Profile) Result {
	r := Result{Plausibility: Plausible, NeedsRoll: true}
	// 动作数量与白名单
	var acts []Action
	moves := 0
	for _, a := range in.Actions {
		if !slices.Contains(Kinds, a.Kind) {
			r.Notes = append(r.Notes, "无法识别的动作「"+a.Kind+"」已忽略")
			continue
		}
		if a.Kind == KindMove {
			if moves > 0 {
				continue
			}
			moves++
		}
		acts = append(acts, a)
	}
	if len(acts) > MaxActions {
		r.Notes = append(r.Notes, "你只来得及完成前两个动作")
		acts = acts[:MaxActions]
	}
	main := Action{Kind: KindDefend}
	found := false
	for _, a := range acts {
		if a.Kind == KindMove {
			r.Moved = true
			continue
		}
		if !found {
			main, found = a, true
		}
	}
	if !found && r.Moved {
		main = Action{Kind: KindMove}
	}
	r.Original, r.Final = main, main
	raw := in.Raw + " " + main.Means + " " + main.Env + " " + main.Style
	// 能力边界 / 物理不可能
	absurdWord := ""
	for _, w := range append(slices.Clone(ctx.AbsurdWords), BuiltinAbsurd...) {
		if w != "" && strings.Contains(raw, w) {
			absurdWord = w
			break
		}
	}
	// 目标
	var tgt *Target
	needsTarget := main.Kind == KindAttack || main.Kind == KindSkill || main.Kind == KindManeuver || main.Kind == KindTalk || main.Kind == KindEnv
	if needsTarget {
		if main.Target == "" && len(ctx.Targets) > 0 {
			tgt = &ctx.Targets[0]
		} else {
			tgt = ctx.target(main.Target)
		}
		if tgt == nil {
			if absurdWord != "" || in.Absurd != "" {
				r.Plausibility = Impossible
				r.Reason = "这个动作在当前世界里不可能做到（" + firstNonEmpty(absurdWord, in.Absurd) + "）。"
			} else {
				r.Plausibility = Impossible
				r.Reason = "战场上没有「" + main.Target + "」。"
			}
			for _, t := range ctx.Targets {
				r.Options = append(r.Options, "攻击"+t.Name)
			}
			r.Options = append(r.Options, "防御", "撤退")
			r.NeedsRoll = false
			return r
		}
		r.Target = tgt
		r.Final.Target = tgt.ID
	}
	downgrade := func(to Action, why string) {
		r.Final = to
		r.Downgraded = true
		if r.DowngradeReason == "" {
			r.DowngradeReason = why
		} else {
			r.DowngradeReason += "；" + why
		}
	}
	if absurdWord != "" || in.Absurd != "" {
		r.Plausibility = Absurd
		downgrade(Action{Kind: KindAttack, Target: r.Final.Target}, "「"+firstNonEmpty(absurdWord, in.Absurd)+"」超出了这个世界的能力边界，你只能做到最接近的一击")
	}
	// 技能
	if r.Final.Kind == KindSkill {
		var sk *SkillInfo
		for i := range ctx.Skills {
			if matchName(r.Final.Skill, ctx.Skills[i].ID, ctx.Skills[i].Name, ctx.Skills[i].Aliases) {
				sk = &ctx.Skills[i]
			}
		}
		switch {
		case sk == nil:
			downgrade(Action{Kind: KindAttack, Target: r.Final.Target, Part: r.Final.Part, Means: r.Final.Means}, "你还不会「"+r.Final.Skill+"」，改为普通攻击")
		case !sk.Usable:
			downgrade(Action{Kind: KindAttack, Target: r.Final.Target, Part: r.Final.Part, Means: r.Final.Means}, sk.Name+"："+sk.Why+"，改为普通攻击")
		default:
			r.Skill = sk
			r.Final.Skill = sk.ID
		}
	}
	// 战技与物理尺度
	if r.Final.Kind == KindManeuver {
		var mv *Maneuver
		for i := range ctx.Maneuvers {
			m := &ctx.Maneuvers[i]
			if matchName(r.Final.Maneuver, m.ID, m.Name, m.Keywords) {
				mv = m
			}
		}
		if mv == nil {
			for i := range ctx.Maneuvers {
				m := &ctx.Maneuvers[i]
				for _, kw := range m.Keywords {
					if kw != "" && strings.Contains(in.Raw, kw) {
						mv = m
					}
				}
			}
		}
		if mv == nil {
			downgrade(Action{Kind: KindAttack, Target: r.Final.Target}, "没有「"+r.Final.Maneuver+"」这种战技，改为普通攻击")
		} else {
			if mv.SizeSensitive && tgt != nil && len(ctx.SizeClasses) > 0 {
				a, b := sizeIndex(ctx.SizeClasses, ctx.ActorSize), sizeIndex(ctx.SizeClasses, tgt.Size)
				if a >= 0 && b >= 0 && b-a >= 2 {
					r.Plausibility = Absurd
					shove := ctx.maneuver("shove")
					why := "对方体型远大于你，「" + mv.Name + "」不可能做到"
					if shove != nil && shove.ID != mv.ID && !shove.SizeSensitive {
						downgrade(Action{Kind: KindManeuver, Target: r.Final.Target, Maneuver: shove.ID}, why+"，改为"+shove.Name)
						mv = shove
					} else {
						downgrade(Action{Kind: KindAttack, Target: r.Final.Target}, why+"，改为普通攻击")
						mv = nil
					}
				}
			}
			if mv != nil {
				r.Maneuver = mv
				r.Final.Maneuver = mv.ID
			}
		}
	}
	// 武器 / 工具
	if r.Final.Kind == KindAttack || r.Final.Kind == KindSkill || r.Final.Kind == KindManeuver {
		if r.Final.Means != "" {
			var m *Means
			for i := range ctx.Means {
				if matchName(r.Final.Means, ctx.Means[i].ID, ctx.Means[i].Name, ctx.Means[i].Aliases) {
					m = &ctx.Means[i]
				}
			}
			switch {
			case m == nil:
				r.Notes = append(r.Notes, "你手边没有「"+r.Final.Means+"」，改用现有的武器")
				r.Final.Means = equippedID(ctx)
			case m.Equipped:
				r.Means = m
			case m.Owned && m.Quick:
				if r.Moved {
					r.Notes = append(r.Notes, "取出"+m.Name+"占用了移动的时间")
					r.Moved = false
				}
				r.Means = m
			default:
				r.Notes = append(r.Notes, m.Name+"不在手边，来不及取出，改用现有的武器")
				r.Final.Means = equippedID(ctx)
			}
		} else {
			r.Final.Means = equippedID(ctx)
		}
		if r.Means == nil {
			for i := range ctx.Means {
				if ctx.Means[i].ID == r.Final.Means {
					r.Means = &ctx.Means[i]
				}
			}
		}
	}
	// 凡人对机甲：需要破甲类修正或破甲武器
	if tgt != nil && tgt.Mech && !ctx.InMech && (r.Final.Kind == KindAttack || r.Final.Kind == KindSkill) {
		breach := r.Means != nil && r.Means.Pierce
		for _, c := range in.Circumstances {
			if t := ctx.tagByID(c.Tag); t != nil && t.Breach && ctx.tagHolds(t, tgt) {
				breach = true
			}
		}
		if !breach {
			r.Notes = append(r.Notes, "凡人的武器很难伤到机甲（需要破甲手段）")
			r.DCParts = append(r.DCParts, Mod{Label: "以凡人之躯对抗机甲", Value: 4})
		}
	}
	// 修正
	r.Mods = append(r.Mods, Mod{Label: "属性", Value: ctx.AttrMod})
	if r.Skill != nil && r.Skill.Bonus != 0 {
		r.Mods = append(r.Mods, Mod{Label: "技能：" + r.Skill.Name, Value: r.Skill.Bonus})
	}
	situ := 0
	var situMods []Mod
	addSitu := func(m Mod) {
		situMods = append(situMods, m)
		situ += m.Value
	}
	// 部位 / 弱点
	if r.Final.Part != "" && tgt != nil && (r.Final.Kind == KindAttack || r.Final.Kind == KindSkill || r.Final.Kind == KindManeuver) {
		var part *Part
		for i := range tgt.Parts {
			if matchName(r.Final.Part, tgt.Parts[i].ID, tgt.Parts[i].Name, nil) {
				part = &tgt.Parts[i]
			}
		}
		if part == nil {
			r.Notes = append(r.Notes, tgt.Name+"身上没有「"+r.Final.Part+"」这个部位")
			r.Final.Part = ""
		} else {
			r.Part = part
			r.Final.Part = part.ID
			if part.DC != 0 {
				r.DCParts = append(r.DCParts, Mod{Label: "部位：" + part.Name, Value: part.DC})
			}
			switch {
			case part.Known && part.Weak:
				addSitu(Mod{Label: "知晓弱点", Value: 2, Tag: "known_weakness"})
			case !part.Known:
				r.Gamble = true
				addSitu(Mod{Label: "碰运气（部位未知）", Value: -1, Tag: "gamble"})
			}
		}
	}
	for _, c := range in.Circumstances {
		t := ctx.tagByID(c.Tag)
		if t == nil {
			r.Notes = append(r.Notes, "无法识别的情境「"+c.Tag+"」不计入")
			continue
		}
		if !ctx.tagHolds(t, tgt) {
			r.Notes = append(r.Notes, "「"+t.Label+"」的前提不成立，不计入")
			continue
		}
		if t.Value != 0 {
			addSitu(Mod{Label: t.Label, Value: t.Value, Tag: t.ID})
		}
	}
	if r.Moved && (r.Final.Kind == KindAttack || r.Final.Kind == KindSkill || r.Final.Kind == KindManeuver) {
		addSitu(Mod{Label: "移动后攻击", Value: -1, Tag: "moved"})
	}
	if ctx.SPLow {
		addSitu(Mod{Label: "体力不足", Value: -2, Tag: "sp_low"})
	}
	strained := !ctx.Local && (in.SelfCheck == "勉强" || in.SelfCheck == "strained")
	if strained && r.Plausibility == Plausible {
		r.Plausibility = Strained
		if p.StrainedPenalty > 0 {
			addSitu(Mod{Label: "勉强", Value: -p.StrainedPenalty, Tag: "strained"})
		}
	}
	clamped := max(-6, min(6, situ))
	if clamped != situ {
		situMods = append(situMods, Mod{Label: "情境修正上限 ±6", Value: clamped - situ})
	}
	r.Mods = append(r.Mods, situMods...)
	for _, m := range r.Mods {
		r.ModTotal += m.Value
	}
	// 难度
	r.DC = 10
	if tgt != nil {
		r.DCParts = append([]Mod{{Label: "目标防御", Value: tgt.Defense}}, r.DCParts...)
		if slices.Contains(tgt.Statuses, "guarded") {
			r.DCParts = append(r.DCParts, Mod{Label: "目标在防御", Value: 2})
		}
		if slices.Contains(tgt.Statuses, "stagger") || slices.Contains(tgt.Statuses, "prone") {
			r.DCParts = append(r.DCParts, Mod{Label: "目标失衡", Value: -2})
		}
	}
	if r.Maneuver != nil && r.Maneuver.DC != 0 {
		r.DCParts = append(r.DCParts, Mod{Label: "战技：" + r.Maneuver.Name, Value: r.Maneuver.DC})
	}
	for _, m := range r.DCParts {
		r.DC += m.Value
	}
	switch r.Final.Kind {
	case KindDefend, KindMove, KindItem, KindMech, KindFlee:
		r.NeedsRoll = false
	}
	return r
}

func (c Context) maneuver(id string) *Maneuver {
	for i := range c.Maneuvers {
		if c.Maneuvers[i].ID == id || strings.HasSuffix(c.Maneuvers[i].ID, "/"+id) {
			return &c.Maneuvers[i]
		}
	}
	return nil
}

func equippedID(ctx Context) string {
	for _, m := range ctx.Means {
		if m.Equipped {
			return m.ID
		}
	}
	return ""
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if s != "" {
			return s
		}
	}
	return ""
}

// Outcome 是掷骰后的结果。
type Outcome struct {
	Dice   string `json:"dice"`
	Rolls  []int  `json:"rolls"`
	Roll   int    `json:"roll"`
	Total  int    `json:"total"`
	Margin int    `json:"margin"`
	Degree string `json:"degree"`
}

// Resolve 掷骰并判定程度。
func Resolve(r Result, p Profile, roll func(sides int) int) Outcome {
	rolls := p.Roll(roll)
	sum := 0
	for _, v := range rolls {
		sum += v
	}
	total := sum + r.ModTotal
	margin := total - r.DC
	return Outcome{Dice: p.DiceSpec(), Rolls: rolls, Roll: sum, Total: total, Margin: margin, Degree: Degree(margin, rolls, p)}
}

// Adjudication 是 ActionAdjudicated 事件的载荷：合理性、修正明细、骰子与程度、后果。
type Adjudication struct {
	Check        Result   `json:"check"`
	Roll         *Outcome `json:"roll,omitempty"`
	Band         string   `json:"band"`
	Consequences []string `json:"consequences,omitempty"`
	Damage       int      `json:"damage,omitempty"`
	Status       string   `json:"status,omitempty"`
	PartRevealed bool     `json:"part_revealed,omitempty"`
	PartBroken   bool     `json:"part_broken,omitempty"`
}

// Degree 返回程度（不需要掷骰的动作视为成功）。
func (a *Adjudication) Degree() string {
	if a.Roll == nil {
		return Success
	}
	return a.Roll.Degree
}
