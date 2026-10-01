package loader

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/combat/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
)

// CodexEntry 是图鉴中的一张介绍卡。物品、技能、敌人、机甲、地点、角色会自动生成介绍卡；
// codex/*.yaml 可以补充势力、科技、传说等条目，或覆盖自动条目的文字。
type CodexEntry struct {
	ID          string            `yaml:"id"`
	Kind        string            `yaml:"kind"` // item / skill / enemy / mech / location / character / faction / lore / tech
	Name        string            `yaml:"name"`
	Icon        string            `yaml:"icon"`
	Rarity      string            `yaml:"rarity"`
	Description string            `yaml:"description"`
	Lore        string            `yaml:"lore"`
	Stats       map[string]string `yaml:"stats"`
	Tags        []string          `yaml:"tags"`
	// Unlock 是解锁条件（CEL）；为空时 faction / lore / tech 开局即解锁。
	Unlock   string `yaml:"unlock"`
	Envelope `yaml:",inline"`
}

// Dimension 是关系维度（信任 / 好感 / 敬畏 / 恩情 / 敌意……可由故事包扩展）。
type Dimension struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
	Min  int    `yaml:"min"`
	Max  int    `yaml:"max"`
	// Negative 为 true 表示该维度越高关系越坏（例如敌意）。
	Negative bool `yaml:"negative"`
}

// EdgeDef 是初始关系边（有向：From 对 To 的看法）。
type EdgeDef struct {
	From   string         `yaml:"from"`
	To     string         `yaml:"to"`
	Values map[string]int `yaml:"values"`
	// Public 为 true 表示玩家开局就知道这段关系（例如众所周知的兄妹）。
	Public bool   `yaml:"public"`
	Note   string `yaml:"note"`
}

// Relations 是关系网配置。
type Relations struct {
	Dimensions []Dimension `yaml:"dimensions"`
	Edges      []EdgeDef   `yaml:"edges"`
}

// DefaultDimensions 是默认关系维度。trust / fear 与旧版 NPC 关系数值共用存储。
var DefaultDimensions = []Dimension{
	{ID: "trust", Name: "信任", Min: -100, Max: 100},
	{ID: "affection", Name: "好感", Min: -100, Max: 100},
	{ID: "awe", Name: "敬畏", Min: 0, Max: 100},
	{ID: "debt", Name: "恩情", Min: 0, Max: 100},
	{ID: "hostility", Name: "敌意", Min: 0, Max: 100, Negative: true},
	{ID: "fear", Name: "畏惧", Min: 0, Max: 100, Negative: true},
}

// Dimension 查找关系维度。
func (r *Relations) Dimension(id string) (Dimension, bool) {
	for _, d := range r.Dimensions {
		if d.ID == id {
			return d, true
		}
	}
	return Dimension{}, false
}

// HasCombat 报告故事包是否带战斗内容。
func (p *Package) HasCombat() bool {
	return p.Combat != nil && (len(p.Combat.EnemyIDs) > 0 || len(p.Combat.SkillIDs) > 0)
}

type combatFile struct {
	Rules      *combat.Config      `yaml:"rules"`
	Statuses   []combat.StatusDef  `yaml:"statuses"`
	Skills     []combat.SkillDef   `yaml:"skills"`
	Enemies    []combat.EnemyDef   `yaml:"enemies"`
	Mechs      []combat.MechDef    `yaml:"mechs"`
	Encounters []combat.Encounter  `yaml:"encounters"`
	Maneuvers  []freeform.Maneuver `yaml:"maneuvers"`
	Tags       []freeform.Tag      `yaml:"tags"`
}

// loadRPG 读取 combat / codex / relations / mainline 内容。
func (p *Package) loadRPG(fsys fs.FS, m *manifest.Manifest) error {
	c := combat.NewContent()
	for _, f := range m.Content["combat"] {
		var raw combatFile
		if err := readYAML(fsys, f, &raw); err != nil {
			return err
		}
		if raw.Rules != nil {
			c.Config = *raw.Rules
		}
		for i := range raw.Statuses {
			x := raw.Statuses[i]
			if err := checkID(m, x.ID, f); err != nil {
				return err
			}
			c.Statuses[x.ID] = &x
			c.StatusIDs = append(c.StatusIDs, x.ID)
		}
		for i := range raw.Skills {
			x := raw.Skills[i]
			if err := checkID(m, x.ID, f); err != nil {
				return err
			}
			if x.Target == "" {
				x.Target = combat.TargetEnemy
			}
			if x.Hits == 0 {
				x.Hits = 1
			}
			x.Description, x.Lore = JoinCJK(x.Description), JoinCJK(x.Lore)
			c.Skills[x.ID] = &x
			c.SkillIDs = append(c.SkillIDs, x.ID)
		}
		for i := range raw.Enemies {
			x := raw.Enemies[i]
			if err := checkID(m, x.ID, f); err != nil {
				return err
			}
			if x.Level == 0 {
				x.Level = 1
			}
			x.Description, x.Lore = JoinCJK(x.Description), JoinCJK(x.Lore)
			c.Enemies[x.ID] = &x
			c.EnemyIDs = append(c.EnemyIDs, x.ID)
		}
		for i := range raw.Mechs {
			x := raw.Mechs[i]
			if err := checkID(m, x.ID, f); err != nil {
				return err
			}
			if x.HeatMax == 0 {
				x.HeatMax = 100
			}
			x.Description, x.Lore = JoinCJK(x.Description), JoinCJK(x.Lore)
			c.Mechs[x.ID] = &x
			c.MechIDs = append(c.MechIDs, x.ID)
		}
		c.Maneuvers = append(c.Maneuvers, raw.Maneuvers...)
		c.Tags = append(c.Tags, raw.Tags...)
		for i := range raw.Encounters {
			x := raw.Encounters[i]
			if err := checkID(m, x.ID, f); err != nil {
				return err
			}
			x.Intro = JoinCJK(x.Intro)
			c.Encounters[x.ID] = &x
			c.EncIDs = append(c.EncIDs, x.ID)
		}
	}
	c.Config.Normalize()
	p.Combat = c
	for _, f := range m.Content["codex"] {
		var raw struct {
			Entries []CodexEntry `yaml:"entries"`
		}
		if err := readYAML(fsys, f, &raw); err != nil {
			return err
		}
		for _, e := range raw.Entries {
			if err := checkID(m, e.ID, f); err != nil {
				return err
			}
			e.Description, e.Lore = JoinCJK(e.Description), JoinCJK(e.Lore)
			p.Codex = append(p.Codex, e)
		}
	}
	for _, f := range m.Content["prompts"] {
		var raw struct {
			Sections []PromptPatch `yaml:"sections"`
		}
		if err := readYAML(fsys, f, &raw); err != nil {
			return err
		}
		for _, x := range raw.Sections {
			if x.Name == "" || strings.TrimSpace(x.Text) == "" {
				return fmt.Errorf("%s: prompt section needs name and text", f)
			}
			if x.Mode == "" {
				x.Mode = "append"
			}
			if x.Mode != "append" && x.Mode != "replace" {
				return fmt.Errorf("%s: prompt section %s: mode must be append or replace", f, x.Name)
			}
			x.Text = strings.TrimSpace(x.Text)
			p.Prompts = append(p.Prompts, x)
		}
	}
	for _, f := range m.Content["relations"] {
		var raw Relations
		if err := readYAML(fsys, f, &raw); err != nil {
			return err
		}
		p.Relations.Dimensions = append(p.Relations.Dimensions, raw.Dimensions...)
		p.Relations.Edges = append(p.Relations.Edges, raw.Edges...)
	}
	if len(p.Relations.Dimensions) == 0 {
		p.Relations.Dimensions = slices.Clone(DefaultDimensions)
	}
	for _, need := range []string{"trust", "fear"} {
		if _, ok := p.Relations.Dimension(need); !ok {
			for _, d := range DefaultDimensions {
				if d.ID == need {
					p.Relations.Dimensions = append(p.Relations.Dimensions, d)
				}
			}
		}
	}
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		it.Description, it.Lore = JoinCJK(it.Description), JoinCJK(it.Lore)
	}
	return nil
}

// validateRPG 校验战斗 / 图鉴 / 关系 / 主线内容的引用完整性。
func (p *Package) validateRPG(compile func(where, expr string)) []error {
	var errs []error
	c := p.Combat
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }
	for _, m := range p.validatePortraits() {
		bad("%s", m)
	}
	skill := func(where, id string) {
		if _, ok := c.Skills[id]; !ok {
			bad("%s：未知的战斗技能 %q", where, id)
		}
	}
	status := func(where string, sa *combat.StatusApply) {
		if sa != nil {
			if _, ok := c.Statuses[sa.ID]; !ok {
				bad("%s：未知的状态效果 %q", where, sa.ID)
			}
		}
	}
	outcomes := func(where string, os []definition.Outcome) {
		for _, o := range os {
			compile(where, o.When)
		}
	}
	for _, id := range c.SkillIDs {
		s := c.Skills[id]
		switch s.Target {
		case combat.TargetEnemy, combat.TargetAllEnemies, combat.TargetSelf, combat.TargetAlly, combat.TargetAllAllies:
		default:
			bad("%s：未知的技能目标 %q", id, s.Target)
		}
		status(id, s.Status)
		if s.Learn != nil {
			for _, r := range s.Learn.Requires {
				skill(id, r)
			}
		}
	}
	for _, id := range c.EnemyIDs {
		e := c.Enemies[id]
		if e.Stats[combat.HP] <= 0 {
			bad("%s：敌人需要 stats.hp > 0", id)
		}
		for _, s := range e.Skills {
			skill(id, s)
		}
		for _, r := range e.AI {
			compile(id+" ai", r.When)
			if r.Skill != "" && r.Skill != "defend" {
				skill(id+" ai", r.Skill)
			}
		}
		for _, d := range e.Drops {
			if _, ok := p.Items[d.Item]; !ok {
				bad("%s：掉落物品 %q 不存在", id, d.Item)
			}
		}
	}
	for _, id := range c.MechIDs {
		for _, s := range c.Mechs[id].Skills {
			skill(id, s)
		}
		compile(id+" requires", c.Mechs[id].Requires)
		p.validateMech(c.Mechs[id], bad, skill)
	}
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		if (it.Kind == "mech_part" || it.Kind == "mech_weapon") && it.Mech == nil {
			bad("%s：机甲改装件 / 武器需要 mech 块", id)
		}
		if it.Mech == nil {
			continue
		}
		for _, sk := range it.Mech.Skills {
			skill(id, sk)
		}
		for _, m := range it.Mech.Mechs {
			if _, ok := c.Mechs[m]; !ok {
				bad("%s：未知的机甲 %q", id, m)
			}
		}
		for k := range it.Mech.Specs {
			if !c.Config.HasMechSpec(k) {
				bad("%s：未知的机甲规格 %q", id, k)
			}
		}
	}
	for _, id := range c.EnemyIDs {
		if m := c.Enemies[id].MechCard; m != "" {
			if _, ok := c.Mechs[m]; !ok {
				bad("%s：未知的机甲 %q", id, m)
			}
		}
	}
	for _, id := range c.EncIDs {
		e := c.Encounters[id]
		if len(e.Enemies) == 0 {
			bad("%s：遭遇战没有敌人", id)
		}
		for _, en := range e.Enemies {
			if _, ok := c.Enemies[en]; !ok {
				bad("%s：未知的敌人 %q", id, en)
			}
		}
		for _, a := range e.Allies {
			ch, ok := p.Characters[a]
			if !ok || ch.Combat == nil {
				bad("%s：同伴 %q 不存在或没有 combat 块", id, a)
			}
		}
		if e.Location != "" {
			if _, ok := p.Locations[e.Location]; !ok {
				bad("%s：地点 %q 不存在", id, e.Location)
			}
		}
		if e.Defeat.MoveTo != "" {
			if _, ok := p.Locations[e.Defeat.MoveTo]; !ok {
				bad("%s：战败后前往的地点 %q 不存在", id, e.Defeat.MoveTo)
			}
		}
		switch e.Defeat.Branch {
		case "", "captured", "injured", "rescued":
		default:
			bad("%s：未知的战败分支 %q（captured / injured / rescued）", id, e.Defeat.Branch)
		}
		compile(id+" available", e.Available)
		outcomes(id+" victory", e.Victory)
		outcomes(id+" defeat", e.Defeat.Effects)
		outcomes(id+" fled", e.Fled)
	}
	chars := append([]*Character{p.Player}, func() []*Character {
		out := []*Character{}
		for _, id := range p.NPCIDs {
			out = append(out, p.Characters[id])
		}
		return out
	}()...)
	for _, ch := range chars {
		switch ch.Card {
		case "", "major", "minor", "none":
		default:
			bad("%s：未知的角色卡等级 %q", ch.ID, ch.Card)
		}
		if ch.Promote != nil {
			compile(ch.ID+" promote", ch.Promote.When)
		}
		compile(ch.ID+" archive_when", ch.ArchiveWhen)
		if ch.Combat == nil {
			continue
		}
		for _, s := range ch.Combat.Skills {
			skill(ch.ID, s)
		}
		for _, r := range ch.Combat.AI {
			compile(ch.ID+" ai", r.When)
		}
		if ch.Combat.Mech != "" {
			if _, ok := c.Mechs[ch.Combat.Mech]; !ok {
				bad("%s：未知的机甲 %q", ch.ID, ch.Combat.Mech)
			}
		}
		for slot, it := range ch.Combat.Equipment {
			item, ok := p.Items[it]
			if !ok {
				bad("%s：初始装备 %q 不存在", ch.ID, it)
			} else if item.Slot != slot {
				bad("%s：%s 不能装备在 %s 槽位", ch.ID, it, slot)
			}
		}
	}
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		if it.Slot != "" && !slices.ContainsFunc(c.Config.Slots, func(s combat.Slot) bool { return s.ID == it.Slot }) {
			bad("%s：未知的装备槽位 %q", id, it.Slot)
		}
		if it.Combat != nil {
			status(id, it.Combat.Status)
		}
	}
	for _, e := range p.Codex {
		if strings.TrimSpace(e.Name) == "" {
			bad("图鉴 %s：缺少 name", e.ID)
		}
		compile("codex "+e.ID, e.Unlock)
	}
	known := func(id string) bool { _, ok := p.Characters[id]; return ok || id == PlayerID }
	for _, e := range p.Relations.Edges {
		if !known(e.From) || !known(e.To) || e.From == e.To {
			bad("关系 %s → %s：角色不存在", e.From, e.To)
		}
		for k := range e.Values {
			if _, ok := p.Relations.Dimension(k); !ok {
				bad("关系 %s → %s：未知维度 %q", e.From, e.To, k)
			}
		}
	}
	return errs
}

// PlayerCombat 返回玩家的战斗块（没有时返回一个基础块）。
func (p *Package) PlayerCombat() *combat.CharacterCombat {
	if p.Player.Combat != nil {
		return p.Player.Combat
	}
	return &combat.CharacterCombat{Level: 1, Stats: combat.Stats{combat.HP: 40, combat.SP: 20, combat.ATK: 8, combat.DEF: 4, combat.SPD: 8, combat.ACC: 80, combat.EVA: 8, combat.CRIT: 5}}
}

// StatsFor 计算角色数值：基础 + 每级成长 ×(等级-1) + 属性加成 ×(属性-基准) + 装备加成。
// 纯整数、与 map 遍历顺序无关（逐项求和）。
func (p *Package) StatsFor(cc *combat.CharacterCombat, level int, attrs map[string]int, equip map[string]string) combat.Stats {
	cfg := &p.Combat.Config
	st := cc.Stats.Add(cc.Growth.Scale(max(level, 1) - 1))
	for a, eff := range cfg.AttributeEffects {
		if v, ok := attrs[a]; ok {
			st = st.Add(eff.Scale(v - cfg.AttributeBase))
		}
	}
	for _, id := range equip {
		if it, ok := p.Items[id]; ok {
			st = st.Add(it.Mods)
		}
	}
	for _, k := range combat.StatKeys {
		st[k] = max(st[k], 0)
	}
	st[combat.HP] = max(st[combat.HP], 1)
	return st
}

// NPCStats 计算同伴 NPC 的战斗数值。
func (p *Package) NPCStats(id string) (combat.Stats, int) {
	c := p.Characters[id]
	if c == nil || c.Combat == nil {
		return nil, 0
	}
	lvl := max(c.Combat.Level, 1)
	return p.StatsFor(c.Combat, lvl, c.Attributes, c.Combat.Equipment), lvl
}

// EnemyStats 返回敌人数值（补齐缺省项）。
func (p *Package) EnemyStats(e *combat.EnemyDef) combat.Stats {
	st := combat.Stats{combat.ACC: 80, combat.EVA: 5, combat.CRIT: 5, combat.SP: 20, combat.SPD: 8}.Add(nil)
	for k, v := range e.Stats {
		st[k] = v
	}
	return st
}

// PromptPatch 是故事包对 AI 提示词段落的补丁（例如 [TOOLS] / [RETRIEVAL_POLICY] / [STYLE]）。
// Agent 为空表示对所有 Agent 生效；mode=append 追加在默认文字之后，replace 整段替换。
type PromptPatch struct {
	Name  string `yaml:"name"`
	Agent string `yaml:"agent"` // narrator / director / npc / 空
	Mode  string `yaml:"mode"`
	Text  string `yaml:"text"`
}

// CodexName 返回图鉴条目 / 实体的显示名。
func (p *Package) CodexName(id string) string { return p.EntityName(id) }

// 立绘资源限制。
const (
	maxPortraitBytes = 2 << 20
)

var portraitExts = []string{".png", ".jpg", ".jpeg", ".webp"}

// validPortraitPath 检查立绘路径（包内相对路径、位于 assets/、扩展名受支持）。
func validPortraitPath(path string) bool {
	if !strings.HasPrefix(path, "assets/") || strings.Contains(path, "..") || !fs.ValidPath(path) {
		return false
	}
	low := strings.ToLower(path)
	for _, e := range portraitExts {
		if strings.HasSuffix(low, e) {
			return true
		}
	}
	return false
}

// validatePortraits 校验角色立绘文件存在且大小合理。
func (p *Package) validatePortraits() []string {
	var errs []string
	for _, id := range append([]string{PlayerID}, p.NPCIDs...) {
		c := p.Character(id)
		if c == nil || c.Portrait == "" {
			continue
		}
		if !validPortraitPath(c.Portrait) {
			errs = append(errs, fmt.Sprintf("角色 %s 的 portrait %q 必须是 assets/ 下的 png/jpg/webp", id, c.Portrait))
			continue
		}
		if p.FS == nil {
			continue
		}
		st, err := fs.Stat(p.FS, c.Portrait)
		if err != nil {
			errs = append(errs, fmt.Sprintf("角色 %s 的立绘 %q 不存在", id, c.Portrait))
		} else if st.Size() > maxPortraitBytes {
			errs = append(errs, fmt.Sprintf("角色 %s 的立绘 %q 超过 2 MB", id, c.Portrait))
		}
	}
	return errs
}

// Portrait 读取角色立绘（找不到时返回 nil，由 UI 显示占位头像）。
func (p *Package) Portrait(id string) ([]byte, string) {
	path := p.PortraitPath(id)
	if path == "" || p.FS == nil || !validPortraitPath(path) {
		return nil, ""
	}
	b, err := fs.ReadFile(p.FS, path)
	if err != nil || len(b) > maxPortraitBytes {
		return nil, ""
	}
	mime := "image/png"
	switch low := strings.ToLower(path); {
	case strings.HasSuffix(low, ".jpg"), strings.HasSuffix(low, ".jpeg"):
		mime = "image/jpeg"
	case strings.HasSuffix(low, ".webp"):
		mime = "image/webp"
	}
	return b, mime
}

// Character 返回角色定义（player 返回玩家角色）。
func (p *Package) Character(id string) *Character {
	if id == PlayerID {
		return p.Player
	}
	return p.Characters[id]
}

// validateMech 校验机甲卡。
func (p *Package) validateMech(m *combat.MechDef, bad func(string, ...any), skill func(string, string)) {
	cfg := &p.Combat.Config
	if m.Status != "" && !cfg.HasMechStatus(m.Status) {
		bad("%s：未知的机甲状态 %q", m.ID, m.Status)
	}
	for k := range m.Specs {
		if !cfg.HasMechSpec(k) {
			bad("%s：未知的机甲规格 %q", m.ID, k)
		}
	}
	if m.Pilot != "" && p.Character(m.Pilot) == nil {
		bad("%s：驾驶者 %q 不存在", m.ID, m.Pilot)
	}
	if m.Portrait != "" {
		if !validPortraitPath(m.Portrait) {
			bad("%s：portrait %q 必须是 assets/ 下的 png/jpg/webp", m.ID, m.Portrait)
		} else if p.FS != nil {
			if st, err := fs.Stat(p.FS, m.Portrait); err != nil {
				bad("%s：立绘 %q 不存在", m.ID, m.Portrait)
			} else if st.Size() > maxPortraitBytes {
				bad("%s：立绘 %q 超过 2 MB", m.ID, m.Portrait)
			}
		}
	}
	slots := map[string]combat.MechSlotDef{}
	hps := map[string]combat.MechSlotDef{}
	for _, sl := range m.ModSlots {
		if _, dup := slots[sl.ID]; dup || sl.ID == "" {
			bad("%s：改装槽 ID %q 为空或重复", m.ID, sl.ID)
		}
		slots[sl.ID] = sl
	}
	for _, sl := range m.Hardpoints {
		if _, dup := hps[sl.ID]; dup || sl.ID == "" {
			bad("%s：武器挂点 ID %q 为空或重复", m.ID, sl.ID)
		}
		hps[sl.ID] = sl
	}
	check := func(kind string, defs map[string]combat.MechSlotDef, inst map[string]string, hardpoint bool) {
		for sl, it := range inst {
			d, ok := defs[sl]
			if !ok {
				bad("%s：没有%s %q", m.ID, kind, sl)
				continue
			}
			item := p.Items[it]
			if item == nil || item.Mech == nil {
				bad("%s：%s %s 安装的 %q 不是机甲改装件 / 武器", m.ID, kind, sl, it)
				continue
			}
			if why := MechFits(m, d, item, hardpoint); why != "" {
				bad("%s：%s", m.ID, why)
			}
		}
	}
	check("改装槽", slots, m.Installed, false)
	check("武器挂点", hps, m.Mounted, true)
	for _, h := range m.Hidden {
		switch {
		case h == combat.MechFieldPilot, h == combat.MechFieldModel, h == combat.MechFieldMaker, h == combat.MechFieldLore,
			h == combat.MechFieldStats, h == combat.MechFieldStatus, h == combat.MechFieldImage:
		case strings.HasPrefix(h, combat.MechFieldSpec):
			if !cfg.HasMechSpec(strings.TrimPrefix(h, combat.MechFieldSpec)) {
				bad("%s：hidden 引用了未知规格 %q", m.ID, h)
			}
		case strings.HasPrefix(h, combat.MechFieldSlot):
			if _, ok := slots[strings.TrimPrefix(h, combat.MechFieldSlot)]; !ok {
				bad("%s：hidden 引用了未知改装槽 %q", m.ID, h)
			}
		case strings.HasPrefix(h, combat.MechFieldHardpoint):
			if _, ok := hps[strings.TrimPrefix(h, combat.MechFieldHardpoint)]; !ok {
				bad("%s：hidden 引用了未知武器挂点 %q", m.ID, h)
			}
		case strings.HasPrefix(h, combat.MechFieldSkill):
			skill(m.ID+" hidden", strings.TrimPrefix(h, combat.MechFieldSkill))
		default:
			bad("%s：未知的 hidden 字段 %q", m.ID, h)
		}
	}
}

// MechFits 检查物品能否装进机甲的某个槽位；返回空串表示可以。
func MechFits(m *combat.MechDef, slot combat.MechSlotDef, item *Item, hardpoint bool) string {
	if item.Mech == nil {
		return item.Name + "不是机甲部件"
	}
	if item.IsMechWeapon() != hardpoint {
		if hardpoint {
			return item.Name + "不是挂载武器，不能装在武器挂点"
		}
		return item.Name + "是挂载武器，只能装在武器挂点"
	}
	if item.Mech.Slot != "" && slot.Kind != "" && item.Mech.Slot != slot.Kind {
		return fmt.Sprintf("%s不能装在%s（需要 %s 槽）", item.Name, slot.Name, item.Mech.Slot)
	}
	if len(item.Mech.Mechs) > 0 && !slices.Contains(item.Mech.Mechs, m.ID) {
		return item.Name + "与" + m.Name + "不兼容"
	}
	return ""
}

// MechSlot 查找改装槽 / 武器挂点定义。
func MechSlot(m *combat.MechDef, id string) (combat.MechSlotDef, bool, bool) {
	for _, s := range m.ModSlots {
		if s.ID == id {
			return s, false, true
		}
	}
	for _, s := range m.Hardpoints {
		if s.ID == id {
			return s, true, true
		}
	}
	return combat.MechSlotDef{}, false, false
}

// PortraitPath 返回角色或机甲的立绘路径（没有时为空）。
func (p *Package) PortraitPath(id string) string {
	if c := p.Character(id); c != nil {
		return c.Portrait
	}
	if p.Combat != nil {
		if m := p.Combat.Mechs[id]; m != nil {
			return m.Portrait
		}
	}
	return ""
}

// IsMechWeapon 报告物品是否是机甲挂载武器。
func (it *Item) IsMechWeapon() bool {
	return it.Mech != nil && (it.Mech.Hardpoint || it.Kind == "mech_weapon")
}
