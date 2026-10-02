package tools

import (
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/api/query"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// Summary 是记忆 Agent 生成的一条摘要（滚动对话摘要 / 长期记忆），供 memory.search 检索。
type Summary struct {
	Kind       string   `json:"kind"`              // rolling / longterm / npc
	Subject    string   `json:"subject,omitempty"` // NPC ID（kind=npc）
	FromTurn   int      `json:"from_turn"`
	ToTurn     int      `json:"to_turn"`
	Text       string   `json:"text"`
	Compressed []string `json:"compressed,omitempty"` // 被压缩掉的内容类别（提示 Agent 需要时去查）
}

// Env 是工具运行所需的只读环境：故事包 + 存档状态 +（可选）事件日志与记忆摘要。
type Env struct {
	Pkg       *loader.Package
	State     *state.State
	Q         *query.Q
	Events    []event.Event
	Summaries []Summary
	// Writer 收集写入工具的提案（nil 表示只读）。
	Writer *Writer
}

func (e *Env) journal() []dto.JournalEntryV1 {
	if e.Q == nil || len(e.Events) == 0 {
		return nil
	}
	return e.Q.Journal(e.Events, e.Pkg.Player.Name())
}

// ---------- 可见性 ----------

// playerKnows 报告玩家是否已经知道某个实体（PlayerScope）。
func (e *Env) playerKnows(id string) bool {
	s, p := e.State, e.Pkg
	if s == nil {
		return false
	}
	if id == loader.PlayerID || id == p.Player.ID {
		return true
	}
	if s.RPG != nil {
		if _, ok := s.RPG.Codex[id]; ok {
			return true
		}
		if _, ok := s.RPG.Cards[id]; ok {
			return true
		}
	}
	if n, ok := s.NPCs[id]; ok {
		return n.Talks > 0 || (n.Location != "" && n.Location == s.Player.Location)
	}
	if id == s.Player.Location {
		return true
	}
	if l, ok := p.Locations[s.Player.Location]; ok {
		for _, x := range l.Exits {
			if x.To == id {
				return true
			}
		}
	}
	if s.Player.Inventory[id] > 0 {
		return true
	}
	if st, ok := s.Stories[id]; ok && st.Status != state.StoryInactive {
		return true
	}
	return false
}

// npcKnows 报告某个 NPC 是否认识 / 知道某个实体（NPCScope）：自己、同处一地的人、关系网里有边的人、
// 地点（公开）、自己驾驶的机甲、公开的势力 / 物品 / 技能。
func (e *Env) npcKnows(npc, id string) bool {
	p, s := e.Pkg, e.State
	if id == npc || id == loader.PlayerID {
		return true
	}
	if _, ok := p.Locations[id]; ok {
		return true
	}
	if _, ok := p.Items[id]; ok {
		return true
	}
	if _, ok := p.Characters[id]; ok {
		for _, ed := range p.Relations.Edges {
			if (ed.From == npc && ed.To == id) || (ed.To == npc && ed.From == id) {
				return true
			}
		}
		if s != nil {
			a, b := s.NPCs[npc], s.NPCs[id]
			if a != nil && b != nil && a.Location != "" && a.Location == b.Location {
				return true
			}
			if s.RPG != nil {
				if _, ok := s.RPG.Relations[npc+">"+id]; ok {
					return true
				}
			}
		}
		return false
	}
	if p.Combat != nil {
		if m, ok := p.Combat.Mechs[id]; ok {
			return m.Pilot == npc || !m.Enemy
		}
		if _, ok := p.Combat.Skills[id]; ok {
			return true
		}
		if _, ok := p.Combat.Enemies[id]; ok {
			return true
		}
	}
	for _, c := range p.Codex {
		if c.ID == id {
			// 显式解锁条件的纯图鉴不是 NPC 的公共知识；玩家解锁也不代表 NPC 知道。
			// 实际地点 / 物品等实体仍遵循上面的实体可见性规则。
			if strings.TrimSpace(c.Unlock) != "" {
				return false
			}
			return c.Kind == "faction" || c.Kind == "location" || c.Kind == "item" || c.Kind == "tech"
		}
	}
	return false
}

func (e *Env) visible(sc Scope, id string) bool {
	switch sc.Kind {
	case ScopeDirector:
		return true
	case ScopeNPC:
		return e.npcKnows(sc.NPC, id)
	default:
		return e.playerKnows(id)
	}
}

// ---------- 文档（全文检索用） ----------

type doc struct {
	ID      string
	Type    string
	Name    string
	Aliases []string
	Public  string // 所有范围可见的文字（名称、公开描述）
	Known   string // 玩家已解锁 / NPC 知道时可见（例如图鉴来历）
	Secret  string // 仅 director 可见（秘密、隐藏字段、剧本节点、调查线索）
}

func (d doc) text(sc Scope, e *Env) string {
	switch sc.Kind {
	case ScopeDirector:
		return join(d.Public, d.Known, d.Secret)
	case ScopeNPC:
		if d.ID == sc.NPC {
			return join(d.Public, d.Known) // 自己的来历自己当然知道（秘密另行列出）
		}
		return d.Public
	default:
		if e.State != nil && e.State.RPG != nil {
			if _, ok := e.State.RPG.Codex[d.ID]; ok {
				return join(d.Public, d.Known)
			}
		}
		return d.Public
	}
}

func join(parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return strings.Join(out, "\n")
}

// docs 按确定顺序列出故事包的全部实体文档。
func (e *Env) docs() []doc {
	p := e.Pkg
	var out []doc
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		var sec []string
		for _, x := range c.Secrets {
			sec = append(sec, "秘密："+x.Text)
		}
		if len(c.Goals) > 0 {
			sec = append(sec, "目标："+strings.Join(c.Goals, "；"))
		}
		out = append(out, doc{ID: id, Type: "character", Name: c.Name(), Aliases: c.Aliases,
			Public: join(c.Identity.Role, c.Description), Known: join(c.Lore, c.Personality.Description), Secret: strings.Join(sec, "\n")})
	}
	for _, id := range p.LocationIDs {
		l := p.Locations[id]
		out = append(out, doc{ID: id, Type: "location", Name: l.Name, Aliases: l.Aliases,
			Public: join(l.Description, strings.Join(l.SceneFacts, "；")), Secret: join("调查线索：" + l.Search.Success)})
	}
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		out = append(out, doc{ID: id, Type: "item", Name: it.Name, Aliases: it.Aliases, Public: it.Description, Known: it.Lore})
	}
	if c := p.Combat; c != nil {
		for _, id := range c.SkillIDs {
			sk := c.Skills[id]
			out = append(out, doc{ID: id, Type: "skill", Name: sk.Name, Public: sk.Description, Known: sk.Lore})
		}
		for _, id := range c.EnemyIDs {
			en := c.Enemies[id]
			out = append(out, doc{ID: id, Type: "enemy", Name: en.Name, Aliases: en.Aliases, Public: en.Description, Known: en.Lore})
		}
		for _, id := range c.MechIDs {
			m := c.Mechs[id]
			pub := m.Class
			if !m.Enemy {
				pub = join(m.Class, m.Description)
			}
			out = append(out, doc{ID: id, Type: "mech", Name: m.Name, Public: pub, Known: join(m.Model, m.Maker, m.Description), Secret: m.Lore})
		}
	}
	seen := map[string]bool{}
	for _, d := range out {
		seen[d.ID] = true
	}
	for _, c := range p.Codex {
		if seen[c.ID] {
			continue
		}
		seen[c.ID] = true
		out = append(out, doc{ID: c.ID, Type: codexType(c.Kind), Name: c.Name, Public: c.Description, Known: c.Lore})
	}
	for _, id := range p.StoryIDs {
		st := p.Stories[id]
		var steps []string
		for _, x := range st.Steps {
			steps = append(steps, x.Title)
		}
		for _, x := range st.Outcomes {
			steps = append(steps, "结局："+x.Title)
		}
		out = append(out, doc{ID: id, Type: "story", Name: st.Title, Known: st.Intro, Secret: strings.Join(steps, "；")})
	}
	for _, id := range p.ActionIDs {
		a := p.Actions[id]
		out = append(out, doc{ID: id, Type: "action", Name: a.Name, Aliases: a.Keywords, Public: a.Description})
	}
	return out
}

func codexType(kind string) string {
	switch kind {
	case "":
		return "lore"
	default:
		return kind
	}
}

// docVisible：story / anchor 类文档对 NPC 永远不可见；对叙述者只有已触发的故事与当前锚点可见。
func (e *Env) docVisible(sc Scope, d doc) bool {
	switch d.Type {
	case "story":
		if sc.Kind == ScopeNPC {
			return false
		}
		if sc.Kind == ScopePlayer {
			st, ok := e.State.Stories[d.ID]
			return ok && st.Status != state.StoryInactive
		}
		return true
	case "anchor":
		if sc.Kind == ScopeNPC {
			return false
		}
		return sc.Kind == ScopeDirector || d.Known != ""
	case "action":
		return true
	}
	return e.visible(sc, d.ID)
}

func (e *Env) findDoc(id string) (doc, bool) {
	for _, d := range e.docs() {
		if d.ID == id {
			return d, true
		}
	}
	return doc{}, false
}

// resolveID 把名字 / 别名 / ID 解析为实体 ID（确定性：精确 ID > 名字 > 别名，同级按文档顺序）。
func (e *Env) resolveID(sc Scope, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if ref == "player" || ref == "你" || ref == "玩家" {
		return loader.PlayerID
	}
	ds := e.docs()
	for _, d := range ds {
		if d.ID == ref {
			return d.ID
		}
	}
	for _, d := range ds {
		if d.Name == ref && e.docVisible(sc, d) {
			return d.ID
		}
	}
	for _, d := range ds {
		if slices.Contains(d.Aliases, ref) && e.docVisible(sc, d) {
			return d.ID
		}
	}
	return ref
}
