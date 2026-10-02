package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// 结果大小上限（确定性截断）。
const (
	MaxResults     = 8    // 列表类结果最多条数
	MaxFieldRunes  = 280  // 单个文本字段最多字符数
	MaxResultBytes = 6000 // 单次工具结果的 JSON 字节上限
)

// ErrNotVisible 表示实体不存在或当前范围不可见（两者故意不区分，避免泄露“存在一个秘密”）。
var ErrNotVisible = errors.New("没有找到（或当前身份无权查看）")

// Tool 是一个只读工具。
type Tool struct {
	Name        string         // 规范名（带点，例如 pack.search）
	Description string         // 中文说明
	Params      map[string]any // JSON Schema
	Scopes      []ScopeKind    // 允许的范围（为空表示全部）
	Write       bool           // 写入工具：只有 Scope.Write 时可用（v0.2.0-rc1）
	run         func(e *Env, sc Scope, a args) (any, error)
}

// FuncName 是函数调用 / MCP 使用的名字（点替换为下划线，兼容 OpenAI 的函数名规则）。
func (t Tool) FuncName() string { return strings.ReplaceAll(t.Name, ".", "_") }

// Allowed 报告工具在该范围是否可用。
func (t Tool) Allowed(sc Scope) bool {
	if t.Write != sc.Write && t.Write {
		return false
	}
	return len(t.Scopes) == 0 || slices.Contains(t.Scopes, sc.Kind)
}

type args map[string]any

func (a args) str(k string) string {
	if v, ok := a[k].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (a args) num(k string, def, lo, hi int) int {
	v, ok := a[k].(float64)
	if !ok {
		return def
	}
	return min(max(int(v), lo), hi)
}

func obj(props map[string]any, required ...string) map[string]any {
	o := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		o["required"] = required
	}
	return o
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

var entityTypes = []string{"character", "location", "item", "skill", "enemy", "mech", "faction", "tech", "lore", "story", "action"}

// All 返回全部工具（按名字排序，确定）。
func All() []Tool {
	ts := []Tool{
		{Name: "pack.search", Description: "在整个故事包（人物 / 地点 / 物品 / 技能 / 敌人 / 机甲 / 势力 / 图鉴 / 剧情节点）中全文检索。缺少设定信息、遇到陌生名词时先查这里，再下笔。",
			Params: obj(map[string]any{"query": strProp("关键词（中文名、别名或描述片段，可用空格分隔多个词）"), "type": map[string]any{"type": "string", "enum": entityTypes, "description": "可选：只搜某一类"}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": MaxResults}}, "query"),
			run:    search},
		{Name: "pack.get_entity", Description: "按 ID 或名字读取一个实体的完整（当前身份可见的）资料。",
			Params: obj(map[string]any{"id": strProp("实体 ID 或名字")}, "id"), run: getEntity},
		{Name: "pack.list", Description: "列出某一类实体（ID + 名字），用于确认有哪些可用的地点 / 人物 / 敌人等。",
			Params: obj(map[string]any{"type": map[string]any{"type": "string", "enum": entityTypes}}, "type"), run: list},
		{Name: "character.get_card", Description: "读取人物卡：身份、状态（活跃 / 归档 / 已故）、所在地、与玩家的关系、公开描述。",
			Params: obj(map[string]any{"id": strProp("人物 ID 或名字")}, "id"), run: characterCard},
		{Name: "mech.get_card", Description: "读取机械甲胄卡：型号、驾驶者、状态、规格、挂载与改装。未掌握的字段显示“未知”。",
			Params: obj(map[string]any{"id": strProp("机甲 ID 或名字")}, "id"), run: mechCard},
		{Name: "relationship.get", Description: "查询人物之间的关系（信任 / 好感 / 敌意……）。只填 who 时返回与此人有关的全部可见关系。",
			Params: obj(map[string]any{"who": strProp("人物 ID 或名字"), "other": strProp("可选：另一方")}, "who"), run: relationship},
		{Name: "memory.search", Description: "检索记忆：NPC 的交谈 / 情节 / 观察记忆、事件日志与对话摘要。上下文被压缩后，用它找回早先的细节。",
			Params: obj(map[string]any{"query": strProp("关键词"), "who": strProp("可选：只看某个 NPC 的记忆"), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": MaxResults}}, "query"),
			Scopes: []ScopeKind{ScopeNPC, ScopePlayer, ScopeDirector}, run: memorySearch},
		{Name: "story.get_state", Description: "当前剧情状态：回合、所在地、当前目标、进行中的剧情事件。",
			Params: obj(map[string]any{}), Scopes: []ScopeKind{ScopePlayer, ScopeDirector}, run: storyState},
		{Name: "world.get_location", Description: "读取地点：描述、场景事实、出口、在场人物。省略 id 表示玩家当前位置。",
			Params: obj(map[string]any{"id": strProp("可选：地点 ID 或名字")}), run: location},
		{Name: "rules.get_action", Description: "读取一个动作规则：说明、关键词、检定技能与耗时。",
			Params: obj(map[string]any{"id": strProp("动作 ID 或名字")}, "id"), run: action},
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })
	return ts
}

// Find 按规范名或函数名查找工具（含写入工具；是否可用由 Allowed 判断）。
func Find(name string) (Tool, bool) {
	for _, t := range append(All(), WriteTools()...) {
		if t.Name == name || t.FuncName() == name {
			return t, true
		}
	}
	return Tool{}, false
}

// Specs 返回某范围可用工具的函数声明（OpenAI tools 格式）。
func Specs(sc Scope) []provider.ToolSpec {
	var out []provider.ToolSpec
	ts := All()
	if sc.Write {
		ts = append(ts, WriteTools()...)
	}
	for _, t := range ts {
		if t.Allowed(sc) {
			out = append(out, provider.ToolSpec{Type: "function", Function: provider.FunctionSpec{Name: t.FuncName(), Description: t.Description, Parameters: t.Params}})
		}
	}
	return out
}

// Call 执行一次工具调用，返回有上限的 JSON 文本。错误也以 JSON 返回（{"error": …}），便于模型继续。
func Call(e *Env, sc Scope, name, rawArgs string) (string, error) {
	t, ok := Find(name)
	if !ok {
		return errJSON("未知工具：" + name), fmt.Errorf("unknown tool %q", name)
	}
	if !t.Allowed(sc) {
		return errJSON("当前身份不能使用 " + t.Name), fmt.Errorf("tool %s not allowed for %s", t.Name, sc)
	}
	a := args{}
	if strings.TrimSpace(rawArgs) != "" {
		if err := json.Unmarshal([]byte(rawArgs), &a); err != nil {
			return errJSON("参数不是合法 JSON"), err
		}
	}
	if e == nil || e.Pkg == nil || e.State == nil {
		return errJSON("没有载入存档"), errors.New("no game loaded")
	}
	v, err := t.run(e, sc, a)
	if err != nil {
		return errJSON(err.Error()), err
	}
	return bound(v), nil
}

func errJSON(msg string) string {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b)
}

// bound 把结果编码为 JSON 并确定性地截断到 MaxResultBytes。
func bound(v any) string {
	b, _ := json.Marshal(v)
	if len(b) <= MaxResultBytes {
		return string(b)
	}
	// 逐步截断：对 results 列表砍尾巴，直到放得下
	if m, ok := v.(map[string]any); ok {
		if rs, ok := m["results"].([]map[string]any); ok {
			for len(rs) > 1 {
				rs = rs[:len(rs)-1]
				m["results"], m["truncated"] = rs, true
				if b, _ = json.Marshal(m); len(b) <= MaxResultBytes {
					return string(b)
				}
			}
		}
	}
	r := []rune(string(b))
	for len(string(r)) > MaxResultBytes-40 {
		r = r[:len(r)*9/10]
	}
	out, _ := json.Marshal(map[string]any{"truncated": true, "partial": string(r)})
	return string(out)
}

func clip(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) > MaxFieldRunes {
		return string(r[:MaxFieldRunes]) + "…"
	}
	return string(r)
}

// ---------- 检索 ----------

// terms 把查询拆成词：空白 / 标点分隔；长中文词额外拆成二元组用于模糊匹配。
func terms(q string) (words, grams []string) {
	f := strings.FieldsFunc(q, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || strings.ContainsRune("，。、；：？！“”‘’（）《》·", r)
	})
	for _, w := range f {
		if w == "" {
			continue
		}
		words = append(words, strings.ToLower(w))
		r := []rune(w)
		if len(r) >= 4 {
			for i := 0; i+2 <= len(r); i++ {
				grams = append(grams, string(r[i:i+2]))
			}
		}
	}
	return
}

func score(d doc, text string, words, grams []string) int {
	sc := 0
	name := strings.ToLower(d.Name)
	low := strings.ToLower(text)
	for _, w := range words {
		switch {
		case name == w || d.ID == w:
			sc += 20
		case strings.Contains(name, w):
			sc += 10
		}
		for _, a := range d.Aliases {
			if strings.EqualFold(a, w) {
				sc += 8
				break
			}
		}
		sc += min(strings.Count(low, w), 3) * 2
	}
	for _, g := range grams {
		if strings.Contains(name, g) || strings.Contains(low, g) {
			sc++
		}
	}
	return sc
}

func snippet(text string, words []string) string {
	r := []rune(text)
	low := strings.ToLower(text)
	for _, w := range words {
		if i := strings.Index(low, w); i >= 0 {
			start := len([]rune(low[:i]))
			a, b := max(start-40, 0), min(start+120, len(r))
			return strings.ReplaceAll(string(r[a:b]), "\n", " ")
		}
	}
	return clip(strings.ReplaceAll(string(r[:min(len(r), 160)]), "\n", " "))
}

func search(e *Env, sc Scope, a args) (any, error) {
	q := a.str("query")
	if q == "" {
		return nil, errors.New("query 不能为空")
	}
	typ := a.str("type")
	limit := a.num("limit", MaxResults, 1, MaxResults)
	words, grams := terms(q)
	type hit struct {
		d     doc
		text  string
		score int
	}
	var hits []hit
	for _, d := range e.docs() {
		if (typ != "" && d.Type != typ) || !e.docVisible(sc, d) {
			continue
		}
		text := d.text(sc, e)
		if s := score(d, text, words, grams); s > 0 {
			hits = append(hits, hit{d, text, s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].d.ID < hits[j].d.ID
	})
	res := []map[string]any{}
	for i, h := range hits {
		if i >= limit {
			break
		}
		res = append(res, map[string]any{"id": h.d.ID, "type": h.d.Type, "name": displayName(e, sc, h.d), "snippet": snippet(h.text, words), "score": h.score})
	}
	return map[string]any{"query": q, "scope": sc.String(), "total": len(hits), "results": res}, nil
}

func displayName(e *Env, sc Scope, d doc) string {
	if d.Type == "mech" && sc.Kind == ScopePlayer && e.Q != nil {
		if m, ok := e.Q.Mech(e.State, d.ID); ok {
			return m.Name
		}
	}
	return d.Name
}

func list(e *Env, sc Scope, a args) (any, error) {
	typ := a.str("type")
	if !slices.Contains(entityTypes, typ) {
		return nil, fmt.Errorf("type 必须是 %s 之一", strings.Join(entityTypes, " / "))
	}
	res := []map[string]any{}
	total := 0
	for _, d := range e.docs() {
		if d.Type != typ || !e.docVisible(sc, d) {
			continue
		}
		total++
		if len(res) < 40 {
			res = append(res, map[string]any{"id": d.ID, "name": displayName(e, sc, d)})
		}
	}
	return map[string]any{"type": typ, "total": total, "results": res}, nil
}

func getEntity(e *Env, sc Scope, a args) (any, error) {
	id := e.resolveID(sc, a.str("id"))
	d, ok := e.findDoc(id)
	if !ok || !e.docVisible(sc, d) {
		return nil, ErrNotVisible
	}
	out := map[string]any{"id": d.ID, "type": d.Type, "name": displayName(e, sc, d), "text": clip(d.text(sc, e))}
	if len(d.Aliases) > 0 && d.Type != "action" {
		out["aliases"] = d.Aliases
	}
	switch d.Type {
	case "character":
		if c, ok := characterCard(e, sc, args{"id": id}); ok == nil {
			out["card"] = c
		}
	case "mech":
		if c, err := mechCard(e, sc, args{"id": id}); err == nil {
			out["card"] = c
		}
	case "location":
		if l, err := location(e, sc, args{"id": id}); err == nil {
			out["location"] = l
		}
	}
	if sc.Kind == ScopePlayer && e.Q != nil {
		if c, ok := e.Q.Card(e.State, id); ok && len(c.Stats) > 0 {
			out["stats"] = c.Stats
		}
	}
	return out, nil
}

// ---------- 人物 / 机甲 / 关系 ----------

func characterCard(e *Env, sc Scope, a args) (any, error) {
	id := e.resolveID(sc, a.str("id"))
	p, s := e.Pkg, e.State
	if id == loader.PlayerID {
		return map[string]any{"id": id, "name": p.Player.Name(), "role": p.Player.Identity.Role, "level": s.PlayerLevel(p), "location": p.EntityName(s.Player.Location)}, nil
	}
	c, ok := p.Characters[id]
	if !ok || !e.visible(sc, id) {
		// 自由推演中动态登场的角色（只有卡片）
		if s.RPG != nil {
			if cd, ok := s.RPG.Cards[id]; ok && cd.Dynamic && sc.Kind != ScopeNPC {
				return map[string]any{"id": id, "name": cd.Name, "role": cd.Role, "status": cd.Status, "dynamic": true}, nil
			}
		}
		return nil, ErrNotVisible
	}
	out := map[string]any{"id": id, "name": c.Name(), "role": c.Identity.Role, "description": clip(c.Description)}
	if c.Faction != "" {
		out["faction"] = p.EntityName(c.Faction)
	}
	if n := s.NPCs[id]; n != nil {
		if n.Location != "" {
			out["location"] = p.EntityName(n.Location)
		}
		if sc.Kind != ScopeNPC || sc.NPC == id {
			out["attitude_to_player"] = n.Attitude()
			out["talks_with_player"] = n.Talks
		}
	}
	if s.RPG != nil {
		if cd, ok := s.RPG.Cards[id]; ok {
			out["status"] = cd.Status
		}
	}
	switch {
	case sc.Kind == ScopeDirector:
		out["lore"] = clip(c.Lore)
		out["personality"] = clip(c.Personality.Description)
		var sec []string
		for _, x := range c.Secrets {
			sec = append(sec, clip(x.Text))
		}
		out["secrets"] = sec
		out["goals"] = c.Goals
	case sc.Kind == ScopeNPC && sc.NPC == id:
		out["lore"] = clip(c.Lore)
		out["personality"] = clip(c.Personality.Description)
		var sec []string
		for _, x := range c.Secrets {
			sec = append(sec, clip(x.Text))
		}
		out["own_secrets"] = sec // 只有本人能看到自己的秘密；台词里不要直接说出来
		out["goals"] = c.Goals
	case sc.Kind == ScopePlayer:
		if e.State.RPG != nil {
			if _, ok := e.State.RPG.Codex[id]; ok && c.Lore != "" {
				out["lore"] = clip(c.Lore)
			}
		}
	}
	if e.Q != nil && sc.Kind != ScopeNPC {
		var ms []string
		for _, mid := range mechIDs(p) {
			m := p.Combat.Mechs[mid]
			if m.Pilot == id && (sc.Kind == ScopeDirector || s.MechFieldKnown(p, mid, "pilot")) {
				ms = append(ms, mid)
			}
		}
		if len(ms) > 0 {
			out["mechs"] = ms
		}
	}
	return out, nil
}

func mechIDs(p *loader.Package) []string {
	if p.Combat == nil {
		return nil
	}
	return p.Combat.MechIDs
}

func mechCard(e *Env, sc Scope, a args) (any, error) {
	p := e.Pkg
	id := e.resolveID(sc, a.str("id"))
	if p.Combat == nil {
		return nil, ErrNotVisible
	}
	m, ok := p.Combat.Mechs[id]
	if !ok {
		return nil, ErrNotVisible
	}
	switch sc.Kind {
	case ScopePlayer:
		if e.Q == nil {
			return nil, ErrNotVisible
		}
		c, ok := e.Q.Mech(e.State, id)
		if !ok || !c.Known {
			return nil, ErrNotVisible
		}
		fields := map[string]string{}
		for _, f := range c.Fields {
			fields[f.Label] = f.Value
		}
		specs := map[string]string{}
		for _, x := range c.Specs {
			if x.Known {
				specs[x.Name] = fmt.Sprint(x.Value)
			} else {
				specs[x.Name] = "未知"
			}
		}
		slots := append(toSlots(c.Hardpoints), toSlots(c.Slots)...)
		out := map[string]any{"id": id, "name": c.Name, "status": c.Status.Name, "fields": fields, "specs": specs, "slots": slots, "unknown_fields": c.Unknown}
		if c.LoreKnown {
			out["lore"] = clip(c.Lore)
		}
		return out, nil
	case ScopeNPC:
		if !e.npcKnows(sc.NPC, id) {
			return nil, ErrNotVisible
		}
		if m.Pilot != sc.NPC {
			return map[string]any{"id": id, "name": m.Name, "class": m.Class, "description": clip(m.Description)}, nil
		}
	}
	v := e.State.MechOf(p, id)
	out := map[string]any{"id": id, "name": m.Name, "model": m.Model, "maker": m.Maker, "class": m.Class, "pilot": p.EntityName(v.Pilot),
		"status": v.Status, "specs": m.Specs, "installed": v.Installed, "mounted": v.Mounted, "description": clip(m.Description)}
	if sc.Kind == ScopeDirector {
		out["lore"] = clip(m.Lore)
		out["hidden_fields"] = m.Hidden
	}
	return out, nil
}

func toSlots[T interface{ ~[]E }, E any](xs T) []string {
	var out []string
	for _, x := range xs {
		b, _ := json.Marshal(x)
		var s struct {
			Name  string `json:"name"`
			Known bool   `json:"known"`
			Part  *struct {
				Name string `json:"name"`
			} `json:"part"`
		}
		_ = json.Unmarshal(b, &s)
		switch {
		case !s.Known:
			out = append(out, s.Name+"：未知")
		case s.Part == nil:
			out = append(out, s.Name+"：空")
		default:
			out = append(out, s.Name+"："+s.Part.Name)
		}
	}
	return out
}

func relationship(e *Env, sc Scope, a args) (any, error) {
	p, s := e.Pkg, e.State
	who := e.resolveID(sc, a.str("who"))
	other := ""
	if a.str("other") != "" {
		other = e.resolveID(sc, a.str("other"))
	}
	if who == "" {
		return nil, errors.New("who 不能为空")
	}
	if !e.visible(sc, who) || (other != "" && !e.visible(sc, other)) {
		return nil, ErrNotVisible
	}
	res := []map[string]any{}
	add := func(from, to string, vals map[string]int, src string) {
		if (from != who && to != who) || (other != "" && from != other && to != other) {
			return
		}
		if sc.Kind == ScopeNPC && from != sc.NPC && to != sc.NPC {
			return // NPC 只知道与自己有关的关系
		}
		keys := make([]string, 0, len(vals))
		for k := range vals {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s %d", dimName(p, k), vals[k]))
		}
		if len(parts) == 0 {
			return
		}
		res = append(res, map[string]any{"from": p.EntityName(from), "to": p.EntityName(to), "values": strings.Join(parts, "，"), "source": src})
	}
	switch sc.Kind {
	case ScopePlayer:
		if e.Q == nil {
			return nil, ErrNotVisible
		}
		rv := e.Q.Relations(s)
		for _, ed := range rv.Edges {
			vals := map[string]int{}
			for _, d := range ed.Values {
				vals[d.ID] = d.Value
			}
			add(ed.From, ed.To, vals, ed.Source)
		}
	default:
		// NPC→玩家（NPC 结构里的 trust / fear 与关系表）
		for _, id := range p.NPCIDs {
			if n := s.NPCs[id]; n != nil {
				add(id, loader.PlayerID, map[string]int{"trust": n.Trust, "fear": n.Fear}, "state")
			}
		}
		keys := []string{}
		if s.RPG != nil {
			for k := range s.RPG.Relations {
				keys = append(keys, k)
			}
		}
		slices.Sort(keys)
		for _, k := range keys {
			from, to, _ := strings.Cut(k, ">")
			add(from, to, s.RPG.Relations[k], "state")
		}
		if len(keys) == 0 {
			for _, ed := range p.Relations.Edges {
				add(ed.From, ed.To, ed.Values, "pack")
			}
		}
	}
	if len(res) > MaxResults*2 {
		res = res[:MaxResults*2]
	}
	return map[string]any{"who": p.EntityName(who), "results": res}, nil
}

func dimName(p *loader.Package, id string) string {
	for _, d := range p.Relations.Dimensions {
		if d.ID == id {
			return d.Name
		}
	}
	switch id {
	case "trust":
		return "信任"
	case "fear":
		return "畏惧"
	}
	return id
}

// ---------- 记忆 ----------

func memorySearch(e *Env, sc Scope, a args) (any, error) {
	p, s := e.Pkg, e.State
	q := a.str("query")
	limit := a.num("limit", MaxResults, 1, MaxResults)
	words, grams := terms(q)
	who := ""
	if a.str("who") != "" {
		who = e.resolveID(sc, a.str("who"))
	}
	if sc.Kind == ScopeNPC {
		if who != "" && who != sc.NPC {
			return nil, ErrNotVisible // NPC 读不到别人的脑子
		}
		who = sc.NPC
	}
	type item struct {
		src   string
		text  string
		turn  int
		score int
	}
	var items []item
	consider := func(src, text string, turn int) {
		d := doc{Name: ""}
		sc := score(d, text, words, grams)
		if q == "" {
			sc = 1
		}
		if sc > 0 {
			items = append(items, item{src, clip(text), turn, sc})
		}
	}
	npcs := p.NPCIDs
	if who != "" {
		npcs = []string{who}
	}
	if sc.Kind != ScopePlayer {
		for _, id := range npcs {
			n := s.NPCs[id]
			if n == nil {
				continue
			}
			name := p.EntityName(id)
			for _, x := range n.Exchanges {
				consider(name+"·交谈", x.Text, x.Turn)
			}
			for _, x := range n.Episodes {
				consider(name+"·情节", x.Text, x.Turn)
			}
			for _, x := range n.Memories {
				consider(name+"·观察", x.Text, x.Turn)
			}
			for _, x := range n.Beliefs {
				consider(name+"·认知", x.Text, x.Turn)
			}
		}
	} else {
		// 叙述者：玩家亲历的日志 + 与玩家交谈过的内容（玩家当时在场）
		for _, id := range npcs {
			if n := s.NPCs[id]; n != nil {
				for _, x := range n.Exchanges {
					consider(p.EntityName(id)+"·交谈", x.Text, x.Turn)
				}
			}
		}
	}
	if sc.Kind != ScopeNPC {
		for _, j := range e.journal() {
			consider("日志·"+j.Time, j.Text, 0)
		}
		for _, sm := range e.Summaries {
			if sm.Kind == "npc" && sc.Kind == ScopePlayer {
				continue
			}
			consider(fmt.Sprintf("摘要·第%d-%d回合", sm.FromTurn, sm.ToTurn), summaryText(sm), sm.ToTurn)
		}
	} else {
		for _, sm := range e.Summaries {
			if sm.Kind == "npc" && sm.Subject == sc.NPC {
				consider(fmt.Sprintf("摘要·第%d-%d回合", sm.FromTurn, sm.ToTurn), summaryText(sm), sm.ToTurn)
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		if items[i].turn != items[j].turn {
			return items[i].turn > items[j].turn
		}
		return items[i].text < items[j].text
	})
	res := []map[string]any{}
	for i, it := range items {
		if i >= limit {
			break
		}
		r := map[string]any{"source": it.src, "text": it.text}
		if it.turn > 0 {
			r["turn"] = it.turn
		}
		res = append(res, r)
	}
	return map[string]any{"query": q, "scope": sc.String(), "total": len(items), "results": res}, nil
}

// summaryText 在摘要后注明被压缩掉的内容，提醒 Agent 细节需另行查询。
func summaryText(sm Summary) string {
	if len(sm.Compressed) == 0 {
		return sm.Text
	}
	return sm.Text + "（已压缩省略：" + strings.Join(sm.Compressed, "、") + "）"
}

// ---------- 剧情 / 世界 / 规则 ----------

func storyState(e *Env, sc Scope, _ args) (any, error) {
	p, s := e.Pkg, e.State
	out := map[string]any{"turn": s.Turn, "location": p.EntityName(s.Player.Location)}
	if e.Q != nil {
		out["objective"] = e.Q.Objective(s)
	}
	var stories []map[string]any
	for _, id := range p.StoryIDs {
		st := s.Stories[id]
		if st == nil || (st.Status == state.StoryInactive && sc.Kind != ScopeDirector) {
			continue
		}
		x := map[string]any{"id": id, "title": p.Stories[id].Title, "status": st.Status}
		if sc.Kind == ScopeDirector {
			x["steps_done"] = st.Steps
		}
		stories = append(stories, x)
	}
	out["stories"] = stories
	return out, nil
}

func location(e *Env, sc Scope, a args) (any, error) {
	p, s := e.Pkg, e.State
	id := s.Player.Location
	if a.str("id") != "" {
		id = e.resolveID(sc, a.str("id"))
	}
	l, ok := p.Locations[id]
	if !ok || !e.visible(sc, id) {
		return nil, ErrNotVisible
	}
	var exits []string
	for _, x := range l.Exits {
		exits = append(exits, fmt.Sprintf("%s（%d 分钟）", p.EntityName(x.To), x.Minutes))
	}
	var present []string
	for _, nid := range p.NPCIDs {
		if n := s.NPCs[nid]; n != nil && n.Location == id {
			present = append(present, p.EntityName(nid))
		}
	}
	out := map[string]any{"id": id, "name": l.Name, "description": clip(l.Description), "facts": l.SceneFacts, "exits": exits, "present": present}
	if sc.Kind == ScopeDirector && l.Search.Success != "" {
		out["search_clue"] = clip(l.Search.Success)
	}
	return out, nil
}

func action(e *Env, sc Scope, a args) (any, error) {
	p := e.Pkg
	id := e.resolveID(sc, a.str("id"))
	d, ok := p.Actions[id]
	if !ok {
		for _, aid := range p.ActionIDs {
			if p.Actions[aid].Name == a.str("id") {
				d, ok = p.Actions[aid], true
			}
		}
	}
	if !ok {
		return nil, ErrNotVisible
	}
	var checks []string
	for _, c := range d.Checks {
		checks = append(checks, c.Skill)
	}
	kw := d.Keywords
	if len(kw) > 12 {
		kw = kw[:12]
	}
	return map[string]any{"id": d.ID, "name": d.Name, "description": clip(d.Description), "keywords": kw, "checks": checks, "time": d.Cost.Time, "target": d.Target}, nil
}
