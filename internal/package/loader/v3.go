package loader

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
	"github.com/GUYU2233/ibukiRPG/internal/world/overlay"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
)

// ---------- 故事包 format 3：实体通用信封（架构 V0.3 第 4.3 节）----------

// KnowledgeSpec 是实体字段的可见性：public 见到就知道；discoverable 需要剧情揭示；hidden 是隐藏真相（仅 AI 可见）。
type KnowledgeSpec struct {
	Public       []string         `yaml:"public"`
	Discoverable []string         `yaml:"discoverable"`
	Hidden       []overlay.Hidden `yaml:"hidden"`
	// KnownBy 列出知道这个实体全部可揭示字段的人物（对话渠道的“说话者自己知道”）。
	KnownBy []string `yaml:"known_by"`
}

// Envelope 是所有实体共用的信封字段（内联在各实体定义里）。
type Envelope struct {
	Importance   int            `yaml:"importance"`
	Canon        string         `yaml:"canon"`
	Locked       []string       `yaml:"locked"`
	Fields       map[string]any `yaml:"fields"`
	Knowledge    KnowledgeSpec  `yaml:"knowledge"`
	AliasUnknown string         `yaml:"alias_unknown"`
}

// Faction 是势力（world/factions.yaml）。
type Faction struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Icon        string   `yaml:"icon"`
	Description string   `yaml:"description"`
	Leader      string   `yaml:"leader"`
	Goals       []string `yaml:"goals"`
	Members     []string `yaml:"members"`
	Territory   []string `yaml:"territory"`
	Envelope    `yaml:",inline"`
}

// PowerTier 是强度档位（角色审查与 AI 生成卡片的上限）。
type PowerTier struct {
	ID    string `yaml:"id" json:"id"`
	Name  string `yaml:"name" json:"name"`
	Power [2]int `yaml:"power" json:"power"`
}

// Balance 是随机性与平衡（rules/balance.yaml，第 4.5 / 9.5 节）。
type Balance struct {
	// Randomness 0 = 纯叙事，100 = 硬核。玩家不能覆盖（第 19 节决定 3）。
	Randomness      int         `yaml:"randomness"`
	PowerTiers      []PowerTier `yaml:"power_tiers"`
	PlayerStartTier string      `yaml:"player_start_tier"`
	AbilityLimits   []string    `yaml:"ability_limits"`
	// AbsurdWords 是能力边界关键词：自由战斗里出现即判为荒谬（无魔法世界里的“火球”）。
	AbsurdWords []string `yaml:"absurd_words"`
	SizeClasses []string `yaml:"size_classes"`
	// Caps 是玩家资源单回合增量上限（金钱 / 经验 / 物品数量）。
	Caps struct {
		GoldPct  int `yaml:"gold_pct"`
		GoldFlat int `yaml:"gold_flat"`
		XPPct    int `yaml:"xp_pct"`
		ItemQty  int `yaml:"item_qty"`
	} `yaml:"caps"`
	// MortalVsMech 是凡人武器对机甲的伤害倍率（百分比，默认 20）。
	MortalVsMech int `yaml:"mortal_vs_mech"`
	// DeathImportance：重要角色死亡提示的默认重要度（由设置覆盖）。
	AuditEvery int `yaml:"audit_every"`
}

// Tier 返回强度档位。
func (b *Balance) Tier(id string) *PowerTier {
	for i := range b.PowerTiers {
		if b.PowerTiers[i].ID == id {
			return &b.PowerTiers[i]
		}
	}
	return nil
}

// TierFor 返回强度分所在的档位。
func (b *Balance) TierFor(power int) *PowerTier {
	for i := range b.PowerTiers {
		t := &b.PowerTiers[i]
		if power >= t.Power[0] && power < t.Power[1] {
			return t
		}
	}
	if len(b.PowerTiers) > 0 {
		return &b.PowerTiers[len(b.PowerTiers)-1]
	}
	return nil
}

// Background 是角色创建的出身（rules/creation.yaml）。
type Background struct {
	ID          string         `yaml:"id" json:"id"`
	Name        string         `yaml:"name" json:"name"`
	Description string         `yaml:"description" json:"description"`
	Attributes  map[string]int `yaml:"attributes" json:"attributes,omitempty"` // 属性加成
	Items       map[string]int `yaml:"items" json:"items,omitempty"`
	Gold        int            `yaml:"gold" json:"gold,omitempty"`
	Skills      []string       `yaml:"skills" json:"skills,omitempty"`
	Location    string         `yaml:"location" json:"location,omitempty"` // 开局地点（为空用 manifest.start）
	Known       []string       `yaml:"known" json:"known,omitempty"`       // 开局知道的条目
}

// Creation 是角色创建规则（第 13.2 节）。
type Creation struct {
	// AttributePoints 是自建角色可分配的属性点；AttrMin / AttrMax 是单项范围。
	AttributePoints int          `yaml:"attribute_points"`
	AttrMin         int          `yaml:"attr_min"`
	AttrMax         int          `yaml:"attr_max"`
	Backgrounds     []Background `yaml:"backgrounds"`
	// Rules 是给审查 Agent 的创建约束（自然语言），例如“不能是贵族出身”。
	Rules []string `yaml:"rules"`
	// MaxPower 是自建角色的强度分上限（为空时取 balance.player_start_tier 的上限）。
	MaxPower int `yaml:"max_power"`
	// Forbidden 是自建角色设定里不允许出现的词（例如原作主角名）。
	Forbidden []string `yaml:"forbidden"`
	// Rewrites 是禁用词的模板改写（例如 魔法: 一手修理齿轮的手艺）；没有改写的禁用词整句删去。
	Rewrites map[string]string `yaml:"rewrites"`
}

// Background 按 ID 查找出身。
func (c *Creation) Background(id string) *Background {
	for i := range c.Backgrounds {
		if c.Backgrounds[i].ID == id {
			return &c.Backgrounds[i]
		}
	}
	return nil
}

// StartKnow 是预设主角开局就知道的条目（例如“认识自己的妹妹”）。
type StartKnow struct {
	Entity string   `yaml:"entity"`
	Fields []string `yaml:"fields"`
}

// ---------- 加载 ----------

func (p *Package) loadV3(fsys fs.FS, m *manifest.Manifest) error {
	p.Factions = map[string]*Faction{}
	for _, f := range m.Content["factions"] {
		var raw struct {
			Factions []Faction `yaml:"factions"`
		}
		if err := readYAML(fsys, f, &raw); err != nil {
			return err
		}
		for i := range raw.Factions {
			x := raw.Factions[i]
			if err := checkID(m, x.ID, f); err != nil {
				return err
			}
			x.Description = JoinCJK(x.Description)
			p.Factions[x.ID] = &x
			p.FactionIDs = append(p.FactionIDs, x.ID)
		}
	}
	p.Timeline = map[string]*timeline.Event{}
	for _, f := range m.Content["timeline"] {
		var raw struct {
			Events []timeline.Event `yaml:"events"`
		}
		if err := readYAML(fsys, f, &raw); err != nil {
			return err
		}
		for i := range raw.Events {
			x := raw.Events[i]
			if err := checkID(m, x.ID, f); err != nil {
				return err
			}
			if x.StartMinute() < 0 {
				return fmt.Errorf("%s：世界事件 %s 的时间不合法（应形如 \"D3 09:00\"）", f, x.ID)
			}
			if x.Resolve == "" {
				x.Resolve = "rules"
			}
			x.Summary = JoinCJK(x.Summary)
			p.Timeline[x.ID] = &x
		}
	}
	for _, f := range m.Content["balance"] {
		if err := readYAML(fsys, f, &p.Balance); err != nil {
			return err
		}
	}
	b := &p.Balance
	if b.Randomness == 0 && len(m.Content["balance"]) == 0 {
		b.Randomness = 45
	}
	if len(b.SizeClasses) == 0 {
		b.SizeClasses = []string{"tiny", "small", "human", "large", "huge", "colossal"}
	}
	if len(b.PowerTiers) == 0 {
		b.PowerTiers = []PowerTier{{"commoner", "普通人", [2]int{0, 40}}, {"trained", "受训者", [2]int{40, 90}}, {"elite", "精英", [2]int{90, 160}}, {"legend", "传奇", [2]int{160, 300}}}
	}
	if b.PlayerStartTier == "" {
		b.PlayerStartTier = "trained"
	}
	if b.Caps.GoldPct == 0 {
		b.Caps.GoldPct = 50
	}
	if b.Caps.GoldFlat == 0 {
		b.Caps.GoldFlat = 50
	}
	if b.Caps.XPPct == 0 {
		b.Caps.XPPct = 50
	}
	if b.Caps.ItemQty == 0 {
		b.Caps.ItemQty = 3
	}
	if b.MortalVsMech == 0 {
		b.MortalVsMech = 20
	}
	if b.AuditEvery == 0 {
		b.AuditEvery = 15
	}
	for _, f := range m.Content["creation"] {
		if err := readYAML(fsys, f, &p.Creation); err != nil {
			return err
		}
	}
	if p.Creation.AttributePoints == 0 {
		p.Creation.AttributePoints = 6
	}
	if p.Creation.AttrMax == 0 {
		p.Creation.AttrMax = 5
	}
	p.buildDocs()
	return nil
}

// validateV3 校验 format 3 新增内容的引用完整性。
func (p *Package) validateV3(compile func(where, expr string)) []error {
	var errs []error
	bad := func(f string, a ...any) { errs = append(errs, fmt.Errorf(f, a...)) }
	exists := func(id string) bool { return p.Docs[id] != nil || id == PlayerID }
	for _, id := range p.TimelineIDs() {
		e := p.Timeline[id]
		if e.Location != "" && p.Locations[e.Location] == nil {
			bad("世界事件 %s：地点 %s 不存在", id, e.Location)
		}
		for _, x := range e.Participants {
			if !exists(x) {
				bad("世界事件 %s：参与者 %s 不存在", id, x)
			}
		}
		compile("timeline "+id+" preconditions", e.Preconditions)
		for _, h := range e.Hooks {
			compile("timeline "+id+" hook "+h.ID, h.Requires)
		}
		for _, o := range e.Outcomes {
			compile("timeline "+id+" outcome "+o.ID, o.When)
		}
		if len(e.Outcomes) == 0 {
			bad("世界事件 %s：至少需要一个结局（outcomes）", id)
		}
		for _, r := range e.Rumor {
			if _, err := timeline.ParseTime(r.At); r.At != "" && err != nil {
				bad("世界事件 %s：传闻时间 %q 不合法", id, r.At)
			}
		}
	}
	for _, id := range p.FactionIDs {
		f := p.Factions[id]
		if f.Leader != "" && !exists(f.Leader) {
			bad("势力 %s：首领 %s 不存在", id, f.Leader)
		}
	}
	for _, id := range p.Manifest.Start.Known {
		if !exists(strings.SplitN(id, "#", 2)[0]) {
			bad("manifest start.known：%s 不存在", id)
		}
	}
	for _, d := range p.Docs {
		for _, h := range d.Hidden {
			compile("hidden "+d.ID+"#"+h.ID, h.RevealWhen)
		}
		switch d.Canon {
		case "", "core", "major", "minor", "flavor":
		default:
			bad("%s：canon 只能是 core / major / minor / flavor", d.ID)
		}
		if d.Importance < 0 || d.Importance > 5 {
			bad("%s：importance 应在 1–5", d.ID)
		}
	}
	for _, bg := range p.Creation.Backgrounds {
		for it := range bg.Items {
			if p.Items[it] == nil {
				bad("出身 %s：物品 %s 不存在", bg.ID, it)
			}
		}
		if bg.Location != "" && p.Locations[bg.Location] == nil {
			bad("出身 %s：地点 %s 不存在", bg.ID, bg.Location)
		}
	}
	if b := p.Balance; b.Randomness < 0 || b.Randomness > 100 {
		bad("balance.randomness 应在 0–100")
	}
	return errs
}

// TimelineIDs 返回故事包时间线事件 ID（按开始时间排序）。
func (p *Package) TimelineIDs() []string { return timeline.SortedIDs(p.Timeline) }

// Doc 返回实体的静态文档（没有时 nil）。
func (p *Package) Doc(id string) *overlay.EntityDoc { return p.Docs[id] }

// KindOf 返回实体种类（未知实体返回空串）。
func (p *Package) KindOf(id string) string {
	if id == PlayerID {
		return "character"
	}
	if strings.Contains(id, ">") {
		return "relation"
	}
	if d := p.Docs[id]; d != nil {
		return d.Kind
	}
	return ""
}

// buildDocs 把各类实体转换为通用文档（WorldChange、知识层、影响评估、检索统一使用）。
func (p *Package) buildDocs() {
	p.Docs = map[string]*overlay.EntityDoc{}
	put := func(id, kind string, env *Envelope, fields map[string]any, defImp int, defCanon string) {
		d := &overlay.EntityDoc{ID: id, Kind: kind, Fields: map[string]any{}}
		for k, v := range fields {
			if v == nil || v == "" {
				continue
			}
			if l, ok := v.([]string); ok && len(l) == 0 {
				continue
			}
			d.Fields[k] = v
		}
		d.Importance, d.Canon = defImp, defCanon
		if env != nil {
			for k, v := range env.Fields {
				d.Fields[k] = v
			}
			if env.Importance > 0 {
				d.Importance = env.Importance
			}
			if env.Canon != "" {
				d.Canon = env.Canon
			}
			d.Locked = env.Locked
			d.Public = env.Knowledge.Public
			d.Discoverable = env.Knowledge.Discoverable
			d.Hidden = append(d.Hidden, env.Knowledge.Hidden...)
			d.AliasUnknown = env.AliasUnknown
			if len(env.Knowledge.KnownBy) > 0 {
				d.Meta = map[string]string{"known_by": strings.Join(env.Knowledge.KnownBy, ",")}
			}
		}
		if len(d.Public) == 0 {
			d.Public = defaultPublic(kind)
		}
		p.Docs[id] = d
	}
	for _, id := range p.LocationIDs {
		l := p.Locations[id]
		var exits []string
		for _, x := range l.Exits {
			exits = append(exits, x.To)
		}
		put(id, "location", &l.Envelope, map[string]any{"name": l.Name, "description": l.Description, "exits": exits}, 2, "minor")
	}
	chars := append([]string{}, p.NPCIDs...)
	for _, id := range chars {
		c := p.Characters[id]
		imp := 2
		if c.CardTier() == "major" {
			imp = 3
		}
		f := map[string]any{"name": c.Name(), "appearance": c.Description, "identity": c.Identity.Role, "personality": c.Personality.Description, "goals": c.Goals, "location": c.Location}
		if c.Faction != "" {
			f["faction"] = c.Faction
		}
		if c.Lore != "" {
			f["background"] = c.Lore
		}
		put(id, "character", &c.Envelope, f, imp, "minor")
		d := p.Docs[id]
		for _, s := range c.Secrets {
			if d.HiddenByID(s.ID) == nil {
				d.Hidden = append(d.Hidden, overlay.Hidden{ID: s.ID, Text: s.Text, LeakMarkers: s.Keywords})
			}
		}
	}
	if p.Player != nil {
		put(PlayerID, "character", &p.Player.Envelope, map[string]any{"name": p.Player.Name(), "appearance": p.Player.Description, "identity": p.Player.Identity.Role}, 5, "minor")
	}
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		kind := "item"
		if it.Kind == "weapon" || it.Kind == "mech_weapon" {
			kind = "weapon"
		}
		f := map[string]any{"name": it.Name, "description": it.Description}
		if it.Lore != "" {
			f["origin"] = it.Lore
		}
		put(id, kind, &it.Envelope, f, 1, "flavor")
	}
	for _, id := range p.FactionIDs {
		x := p.Factions[id]
		put(id, "faction", &x.Envelope, map[string]any{"name": x.Name, "description": x.Description, "leader": x.Leader, "goals": x.Goals, "members": x.Members, "territory": x.Territory}, 3, "major")
	}
	if p.Combat != nil {
		for _, id := range p.Combat.EnemyIDs {
			e := p.Combat.Enemies[id]
			imp := max(e.Importance, 1)
			var parts []string
			for _, pt := range e.Parts {
				parts = append(parts, pt.ID)
			}
			put(id, "enemy", nil, map[string]any{"name": e.Name, "description": e.Description, "parts": parts}, imp, "flavor")
		}
		for _, id := range p.Combat.MechIDs {
			m := p.Combat.Mechs[id]
			put(id, "mech", nil, map[string]any{"name": m.Name, "description": m.Description, "lore": m.Lore}, 3, "major")
		}
		for _, id := range p.Combat.SkillIDs {
			s := p.Combat.Skills[id]
			put(id, "skill", nil, map[string]any{"name": s.Name, "description": s.Description}, 1, "flavor")
		}
	}
	for i := range p.Codex {
		c := &p.Codex[i]
		if _, dup := p.Docs[c.ID]; dup {
			// 自动条目的补充：合并文字
			d := p.Docs[c.ID]
			if c.Lore != "" {
				d.Fields["details"] = c.Lore
			}
			mergeEnvelope(d, &c.Envelope)
			continue
		}
		kind := c.Kind
		if kind == "" {
			kind = "lore"
		}
		f := map[string]any{"title": c.Name, "summary": c.Description, "details": c.Lore}
		if kind == "faction" {
			f = map[string]any{"name": c.Name, "description": c.Description, "goals": c.Lore}
		}
		put(c.ID, kind, &c.Envelope, f, 2, "minor")
	}
	for _, id := range p.TimelineIDs() {
		e := p.Timeline[id]
		canon := e.Canon
		if canon == "" {
			canon = "minor"
		}
		put(id, "world_event", nil, map[string]any{"title": e.Title, "summary": e.Summary, "time": e.Window.Start, "location": e.Location, "participants": e.Participants}, max(e.Importance, 1), canon)
	}
}

func mergeEnvelope(d *overlay.EntityDoc, env *Envelope) {
	for k, v := range env.Fields {
		d.Fields[k] = v
	}
	if env.Importance > 0 {
		d.Importance = env.Importance
	}
	if env.Canon != "" {
		d.Canon = env.Canon
	}
	if len(env.Knowledge.Public) > 0 {
		d.Public = env.Knowledge.Public
	}
	if len(env.Knowledge.Discoverable) > 0 {
		d.Discoverable = env.Knowledge.Discoverable
	}
	d.Hidden = append(d.Hidden, env.Knowledge.Hidden...)
	if env.AliasUnknown != "" {
		d.AliasUnknown = env.AliasUnknown
	}
}

// defaultPublic 是没有声明 knowledge.public 时“见到就知道”的字段。
func defaultPublic(kind string) []string {
	switch kind {
	case "character":
		return []string{"appearance"}
	case "location":
		return []string{"name", "description", "exits"}
	case "item", "weapon":
		return []string{"name", "description"}
	case "enemy", "mech":
		return []string{"name"}
	case "faction":
		return []string{"name"}
	case "world_event":
		return []string{"time", "location"}
	}
	return []string{"title", "summary"}
}

// DocIDs 返回全部静态文档 ID（排序）。
func (p *Package) DocIDs() []string {
	out := make([]string, 0, len(p.Docs))
	for id := range p.Docs {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// KnownBy 返回知道该实体全部可揭示字段的人物。
func (p *Package) KnownBy(id string) []string {
	d := p.Docs[id]
	if d == nil || d.Meta["known_by"] == "" {
		return nil
	}
	return strings.Split(d.Meta["known_by"], ",")
}

// Playable 返回可选的预设主角（type: player 的玩家模板 + playable: true 的人物）。
func (p *Package) Playable() []*Character {
	out := []*Character{p.Player}
	for _, id := range p.NPCIDs {
		if c := p.Characters[id]; c.Playable {
			out = append(out, c)
		}
	}
	return out
}
