// Package validate 是统一写入网关的校验器（架构 V0.3 第 5.3 / 8.4 节）：
// 所有来源（AI 叙事、战斗、世界模拟、审查、玩家指令、MCP、角色创建）提出的世界变更与知识揭示
// 都经过同一套确定性检查；被丢弃的提案带原因返回，由调用方写入诊断表。
package validate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/overlay"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
)

// 模型能力档位（第 11.7 节）：本地小模型只能做有限的修改。
const (
	TierFull    = "full"
	TierLimited = "limited"
	TierMinimal = "minimal"
)

// MaxChanges 返回单回合最多接受的变更数。
func MaxChanges(tier string) int {
	switch tier {
	case TierLimited:
		return 3
	case TierMinimal:
		return 2
	}
	return 12
}

// MaxReveals 返回单回合最多揭示的字段数。
func MaxReveals(tier string) int {
	if tier == TierFull || tier == "" {
		return 8
	}
	return 3
}

// MaxText 是单个文本字段的最大长度（字）。
const MaxText = 400

// MaxTimelineAddsPerDay 是每个世界日 AI 最多新增的时间线事件数。
const MaxTimelineAddsPerDay = 3

// Env 是校验所需的环境。
type Env struct {
	Pkg    *loader.Package
	State  *state.State
	Source string
	Tier   string
	// Eval 求值 CEL（揭示前提 reveal_when）；为 nil 时有前提的真相一律不可揭示。
	Eval func(expr string) (bool, error)
}

// Reject 是一条被丢弃的提案。
type Reject struct {
	Kind   string          `json:"kind"` // change / reveal
	Target string          `json:"target"`
	Path   string          `json:"path,omitempty"`
	Op     string          `json:"op,omitempty"`
	Reason string          `json:"reason"`
	Raw    json.RawMessage `json:"raw,omitempty"`
}

func rej(kind, target, path, op, reason string, raw any) Reject {
	b, _ := json.Marshal(raw)
	return Reject{Kind: kind, Target: target, Path: path, Op: op, Reason: reason, Raw: b}
}

// directorSources 可以修改隐藏真相的来源（它们本来就能看到相关真相）。
var directorSources = []string{change.SourceNarrate, change.SourceWorldSim, change.SourceAudit, change.SourceUserRequest, "mcp:director"}

// retireReasons 是合法的退场原因。
var retireReasons = []string{"death", "destroyed", "disbanded", "lost", "left", "retired"}

var slugRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// GenNamespace 返回新建实体的命名空间（<包命名空间>.gen）。
func GenNamespace(p *loader.Package) string { return p.Manifest.Namespace + ".gen" }

func (e *Env) doc(id string) *overlay.EntityDoc { return e.State.Doc(e.Pkg, id) }

func (e *Env) kindOf(id string) string {
	if strings.Contains(id, ">") {
		return "relation"
	}
	if d := e.doc(id); d != nil {
		return d.Kind
	}
	if e.State.EventDef(e.Pkg, id) != nil {
		return "world_event"
	}
	return ""
}

// Meta 返回评分元数据。
func (e *Env) Meta(c change.Change) change.Meta {
	m := change.Meta{Kind: e.kindOf(c.Target), Importance: 2, Canon: "minor"}
	if c.Op == change.OpCreate {
		m.Kind = c.Kind
		var d overlay.EntityDoc
		_ = json.Unmarshal(c.Value, &d)
		m.Importance = max(d.Importance, 1)
		m.Canon = "flavor"
		return m
	}
	if strings.HasPrefix(c.Op, "timeline") {
		m.Kind = "world_event"
		ev := e.State.EventDef(e.Pkg, c.Target)
		if c.Op == change.OpTimelineAdd {
			var x timeline.Event
			_ = json.Unmarshal(c.Value, &x)
			ev = &x
			m.Kind = "timeline"
		}
		if ev != nil {
			m.Importance, m.Canon, m.Pivotal = max(ev.Importance, 1), ev.Canon, ev.Pivotal
			if m.Canon == "" {
				m.Canon = "minor"
			}
		}
		if c.Op == change.OpTimelinePatch && c.Path == "outcome" {
			m.OutcomeChange = true
		}
		return m
	}
	if strings.Contains(c.Target, ">") {
		a, b, _ := strings.Cut(c.Target, ">")
		m.Importance = 1
		for _, x := range []string{a, b} {
			if d := e.doc(x); d != nil {
				m.Importance = max(m.Importance, d.Importance-1)
			}
		}
		m.Canon = "flavor"
		return m
	}
	if d := e.doc(c.Target); d != nil {
		m.Importance, m.Canon, m.Locked = max(d.Importance, 1), d.Canon, d.Locked
		if m.Canon == "" {
			m.Canon = "minor"
		}
	}
	if c.Target == loader.PlayerID {
		m.Importance, m.Canon = 1, "flavor"
	}
	if c.Op == change.OpRetire {
		var r overlay.Retirement
		_ = json.Unmarshal(c.Value, &r)
		m.RetireReason = r.Reason
	}
	return m
}

// Changes 校验一组变更提案，返回接受的变更（已填好 Before / Prev / Source / Impact / Summary）与被丢弃的提案。
func Changes(e *Env, in []change.Change) ([]change.Change, []Reject) {
	var out []change.Change
	var rejects []Reject
	seenPath := map[string]bool{}
	retired := map[string]bool{}
	limit := MaxChanges(e.Tier)
	adds := 0
	day := int(e.State.Minute/1440) + 1
	if e.State.Timeline != nil {
		adds = e.State.Timeline.DayAdds[day]
	}
	for _, c := range in {
		c.ID, c.Before, c.Prev, c.Impact.Score, c.Impact.Types, c.Summary = "", nil, nil, 0, nil, ""
		c.Source = e.Source
		c.Reason = clip(strings.TrimSpace(c.Reason), 120)
		bad := func(reason string) { rejects = append(rejects, rej("change", c.Target, c.Path, c.Op, reason, c)) }
		if len(out) >= limit {
			bad(fmt.Sprintf("超过单回合上限（%d 项）", limit))
			continue
		}
		if !slices.Contains(change.PublicOps, c.Op) {
			bad("未知操作 " + c.Op)
			continue
		}
		if !opAllowed(e.Tier, c) {
			bad("当前模型能力档位不允许这类修改")
			continue
		}
		if c.Op == change.OpCreate || c.Op == change.OpTimelineAdd {
			c.Target = genID(e, c)
		}
		key := c.Target + "|" + c.Path
		if c.Op == change.OpPatch && seenPath[key] {
			bad("同一路径单回合只能修改一次")
			continue
		}
		if retired[c.Target] && c.Op != change.OpRestore {
			bad("同一提案里先退场再修改同一实体，丢弃后者")
			continue
		}
		var err error
		switch c.Op {
		case change.OpCreate:
			err = e.checkCreate(&c)
		case change.OpPatch:
			err = e.checkPatch(&c)
		case change.OpRetire:
			err = e.checkRetire(&c)
		case change.OpRestore:
			if !e.State.Retired(c.Target) {
				err = fmt.Errorf("%s 没有退场，无需恢复", c.Target)
			}
			if err == nil {
				b, _ := json.Marshal(e.State.World.Retired[c.Target])
				c.Value = b
			}
		case change.OpLink, change.OpUnlink:
			err = e.checkLink(&c)
		case change.OpTimelineAdd:
			if adds >= MaxTimelineAddsPerDay {
				err = fmt.Errorf("每个世界日最多新增 %d 个时间线事件", MaxTimelineAddsPerDay)
			} else {
				err = e.checkTimelineAdd(&c)
				if err == nil {
					adds++
				}
			}
		case change.OpTimelinePatch:
			err = e.checkTimelinePatch(&c)
		case change.OpTimelineCancel:
			err = e.checkTimelineCancel(&c)
		}
		if err != nil {
			bad(err.Error())
			continue
		}
		if e.State.RPG != nil && e.State.RPG.Combat != nil && e.Source != change.SourceCombat &&
			c.Op == change.OpPatch && c.Target == loader.PlayerID && c.Path == "hp" {
			bad("战斗进行中，只有战斗结算能修改参战单位的生命")
			continue
		}
		m := e.Meta(c)
		c.Impact.Score = change.Score(c, m)
		c.Impact.Types = change.Types(c, m)
		c.Summary = Summary(e, c)
		if c.Op == change.OpPatch {
			seenPath[key] = true
		}
		if c.Op == change.OpRetire {
			retired[c.Target] = true
		}
		out = append(out, c)
	}
	return out, rejects
}

func opAllowed(tier string, c change.Change) bool {
	switch tier {
	case TierLimited:
		return c.Op == change.OpPatch || c.Op == change.OpLink || c.Op == change.OpUnlink || (c.Op == change.OpCreate && (c.Kind == "lore" || c.Kind == "item"))
	case TierMinimal:
		return c.Op == change.OpPatch && (strings.HasPrefix(c.Path, "fields.") || change.IsDeltaPath(c.Target, c.Path))
	}
	return true
}

func genID(e *Env, c change.Change) string {
	ns := GenNamespace(e.Pkg)
	kind := c.Kind
	if c.Op == change.OpTimelineAdd {
		kind = "event"
	}
	if strings.HasPrefix(c.Target, ns+":") {
		return c.Target
	}
	slug := c.Target
	if i := strings.LastIndexAny(slug, "/:"); i >= 0 {
		slug = slug[i+1:]
	}
	slug = strings.ToLower(slug)
	if !slugRe.MatchString(slug) {
		slug = fmt.Sprintf("n%d_%d", e.State.Turn, e.State.WorldSeq()+1)
	}
	return fmt.Sprintf("%s:%s/%s", ns, kind, slug)
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func textOK(v any) error {
	switch x := v.(type) {
	case string:
		if utf8.RuneCountInString(x) > MaxText {
			return fmt.Errorf("文本超过 %d 字", MaxText)
		}
	case []any:
		if len(x) > 20 {
			return fmt.Errorf("列表超过 20 项")
		}
		for _, y := range x {
			if err := textOK(y); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, y := range x {
			if err := textOK(y); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Env) nameTaken(name string) bool {
	if name == "" {
		return false
	}
	for _, id := range e.Pkg.DocIDs() {
		if d := e.Pkg.Docs[id]; d.Name() == name {
			return true
		}
	}
	if e.State.World != nil {
		for _, d := range e.State.World.Created {
			if d.Name() == name {
				return true
			}
		}
	}
	return false
}

func (e *Env) checkCreate(c *change.Change) error {
	if _, ok := knowledge.Catalog[c.Kind]; !ok || c.Kind == "relation" || c.Kind == "world_event" {
		return fmt.Errorf("不能新建种类 %q 的实体", c.Kind)
	}
	if e.doc(c.Target) != nil {
		return fmt.Errorf("ID %s 已存在", c.Target)
	}
	var d overlay.EntityDoc
	if err := json.Unmarshal(c.Value, &d); err != nil {
		return fmt.Errorf("新实体文档格式错误：%w", err)
	}
	if d.Fields == nil {
		// 允许直接给字段 map
		var f map[string]any
		if json.Unmarshal(c.Value, &f) == nil {
			d.Fields = f
		}
	}
	d.ID, d.Kind = c.Target, c.Kind
	if d.Name() == c.Target {
		return fmt.Errorf("新实体需要 name 或 title")
	}
	if e.nameTaken(d.Name()) {
		return fmt.Errorf("同名实体已存在：%s", d.Name())
	}
	for k, v := range d.Fields {
		if err := textOK(v); err != nil {
			return fmt.Errorf("字段 %s：%w", k, err)
		}
	}
	if len(d.Hidden) > 0 && !slices.Contains(directorSources, e.Source) {
		return fmt.Errorf("来源 %s 不能写隐藏真相", e.Source)
	}
	if d.Importance < 1 || d.Importance > 5 {
		d.Importance = 1
	}
	if d.Importance > 3 && e.Source != change.SourceUserRequest {
		d.Importance = 3
	}
	if d.Canon == "core" {
		d.Canon = "minor"
	}
	if d.Canon == "" {
		d.Canon = "flavor"
	}
	if len(d.Stats) > 0 {
		e.clampStats(d.Stats, c)
	}
	if len(d.Public) == 0 {
		d.Public = []string{"name"}
	}
	b, _ := json.Marshal(d)
	c.Value = b
	return nil
}

// clampStats 按强度档位夹紧新实体的战斗数值（第 5.3 节“数值上限”）。
func (e *Env) clampStats(st map[string]int, c *change.Change) {
	tier := e.Pkg.Balance.Tier(e.Pkg.Balance.PlayerStartTier)
	if tier == nil {
		return
	}
	limit := tier.Power[1]
	clamped := false
	for k, v := range st {
		if v > limit {
			st[k] = limit
			clamped = true
		}
		if v < 0 {
			st[k] = 0
		}
	}
	if clamped {
		c.Reason += "（已按世界强度上限调整）"
	}
}

func (e *Env) checkPatch(c *change.Change) error {
	s, p := e.State, e.Pkg
	if change.IsPlayerPath(c.Target, c.Path) {
		var d int
		if err := json.Unmarshal(c.Value, &d); err != nil {
			return fmt.Errorf("玩家数值修改必须是整数增量")
		}
		orig := d
		caps := p.Balance.Caps
		switch {
		case c.Path == "gold":
			lim := s.Player.Gold*caps.GoldPct/100 + caps.GoldFlat
			d = max(min(d, lim), -s.Player.Gold)
		case c.Path == "xp":
			lv := s.PlayerLevel(p)
			lim := max(10, p.Combat.Config.XPToNext(lv)*caps.XPPct/100)
			d = max(min(d, lim), 0)
		case c.Path == "hp":
			d = max(min(d, 50), -50)
		case strings.HasPrefix(c.Path, "inventory."):
			it := strings.TrimPrefix(c.Path, "inventory.")
			if p.Items[it] == nil {
				return fmt.Errorf("物品 %s 不存在", it)
			}
			d = max(min(d, caps.ItemQty), -s.Player.Inventory[it])
		}
		if d == 0 {
			return fmt.Errorf("修改量为 0")
		}
		if d != orig {
			c.Reason += "（已按单回合上限调整）"
		}
		c.Value, _ = json.Marshal(d)
		return nil
	}
	if strings.Contains(c.Target, ">") {
		a, b, _ := strings.Cut(c.Target, ">")
		if e.doc(a) == nil || e.doc(b) == nil {
			return fmt.Errorf("关系的人物不存在")
		}
		if !strings.HasPrefix(c.Path, "dims.") {
			return fmt.Errorf("关系只能修改 dims.<维度>")
		}
		if _, ok := p.Relations.Dimension(strings.TrimPrefix(c.Path, "dims.")); !ok {
			return fmt.Errorf("未知的关系维度 %s", c.Path)
		}
		var d int
		if err := json.Unmarshal(c.Value, &d); err != nil {
			return fmt.Errorf("关系维度修改必须是整数增量")
		}
		d = max(min(d, 20), -20)
		if d == 0 {
			return fmt.Errorf("修改量为 0")
		}
		c.Value, _ = json.Marshal(d)
		return nil
	}
	d := e.doc(c.Target)
	if d == nil {
		return fmt.Errorf("实体 %s 不存在", c.Target)
	}
	if d.Retired != nil {
		return fmt.Errorf("%s 已退场", d.Name())
	}
	if c.Target == loader.PlayerID {
		return fmt.Errorf("玩家角色只能修改金钱 / 经验 / 生命 / 物品")
	}
	if c.Path == "location" || c.Path == "fields.location" {
		if d.Kind != "character" {
			return fmt.Errorf("只有人物可以移动")
		}
		var loc string
		if json.Unmarshal(c.Value, &loc) != nil || (loc != "" && p.Locations[loc] == nil) {
			return fmt.Errorf("地点不存在")
		}
		c.Before, _ = json.Marshal(d.Field("location"))
		if _, ok := s.NPCs[c.Target]; ok {
			c.Path = "location"
		} else {
			c.Path = "fields.location"
			c.Prev = s.World.Patch(c.Target, c.Path)
		}
		return nil
	}
	if !overlay.PatchablePath(c.Path) {
		return fmt.Errorf("路径 %s 不可修改", c.Path)
	}
	var v any
	if err := json.Unmarshal(c.Value, &v); err != nil {
		return fmt.Errorf("值格式错误")
	}
	if err := textOK(v); err != nil {
		return err
	}
	if strings.HasPrefix(c.Path, "knowledge.hidden[") {
		if !slices.Contains(directorSources, e.Source) {
			return fmt.Errorf("来源 %s 不能修改隐藏真相", e.Source)
		}
		c.Hidden = true
	}
	if c.Path == "canon" || c.Path == "importance" {
		if e.Source != change.SourceUserRequest && !strings.HasPrefix(e.Source, change.SourceMCP) {
			return fmt.Errorf("只有作者指令能修改设定层级 / 重要度")
		}
	}
	if strings.HasPrefix(c.Path, "combat.stats.") {
		f, ok := v.(float64)
		if !ok {
			return fmt.Errorf("战斗数值必须是数字")
		}
		cur := d.Stats[strings.TrimPrefix(c.Path, "combat.stats.")]
		tier := p.Balance.Tier(p.Balance.PlayerStartTier)
		width := 50
		if tier != nil {
			width = max(tier.Power[1]-tier.Power[0], 10)
		}
		nv := int(f)
		if nv > cur+width/2 {
			nv = cur + width/2
			c.Reason += "（已按世界强度上限调整）"
		}
		c.Value, _ = json.Marshal(max(nv, 0))
	}
	// Before：当前生效值；Prev：覆盖层原始补丁（反向操作用）
	c.Before = currentValue(d, c.Path)
	c.Prev = e.State.World.Patch(c.Target, c.Path)
	return nil
}

func currentValue(d *overlay.EntityDoc, path string) json.RawMessage {
	var v any
	switch {
	case strings.HasPrefix(path, "fields."):
		v = d.Fields[strings.TrimPrefix(path, "fields.")]
	case path == "importance":
		v = d.Importance
	case path == "canon":
		v = d.Canon
	case path == "alias_unknown":
		v = d.AliasUnknown
	case path == "tier":
		v = d.Tier
	case strings.HasPrefix(path, "combat.stats."):
		v = d.Stats[strings.TrimPrefix(path, "combat.stats.")]
	case strings.HasPrefix(path, "knowledge.hidden["):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "knowledge.hidden["), "]")
		if h := d.HiddenByID(id); h != nil {
			v = h.Text
		}
	}
	b, _ := json.Marshal(v)
	return b
}

func (e *Env) checkRetire(c *change.Change) error {
	if c.Target == loader.PlayerID {
		return fmt.Errorf("玩家角色不能通过世界变更退场")
	}
	d := e.doc(c.Target)
	if d == nil {
		return fmt.Errorf("实体 %s 不存在", c.Target)
	}
	if d.Retired != nil {
		return fmt.Errorf("%s 已经退场", d.Name())
	}
	var r overlay.Retirement
	if len(c.Value) > 0 && string(c.Value) != "null" {
		if err := json.Unmarshal(c.Value, &r); err != nil {
			var reason string
			if json.Unmarshal(c.Value, &reason) != nil {
				return fmt.Errorf("退场值格式错误")
			}
			r.Reason = reason
		}
	}
	if r.Reason == "" {
		r.Reason = "retired"
	}
	if !slices.Contains(retireReasons, r.Reason) {
		return fmt.Errorf("退场原因只能是 %s", strings.Join(retireReasons, " / "))
	}
	if r.Text == "" {
		r.Text = c.Reason
	}
	r.Turn = e.State.Turn
	b, _ := json.Marshal(r)
	c.Value = b
	return nil
}

func (e *Env) checkLink(c *change.Change) error {
	a, b, ok := strings.Cut(c.Target, ">")
	if !ok || e.doc(a) == nil || e.doc(b) == nil {
		return fmt.Errorf("link 的目标应为 \"实体A>实体B\"，且两者都存在")
	}
	if e.State.World != nil {
		c.Before = e.State.World.Links[c.Target]
	}
	if c.Op == change.OpUnlink {
		if len(c.Before) == 0 {
			return fmt.Errorf("没有这条关联")
		}
		return nil
	}
	var v any
	if err := json.Unmarshal(c.Value, &v); err != nil {
		return fmt.Errorf("关联值格式错误")
	}
	return textOK(v)
}

func (e *Env) checkTimelineAdd(c *change.Change) error {
	var ev timeline.Event
	if err := json.Unmarshal(c.Value, &ev); err != nil {
		return fmt.Errorf("时间线事件格式错误：%w", err)
	}
	ev.ID = c.Target
	if strings.TrimSpace(ev.Title) == "" {
		return fmt.Errorf("时间线事件需要标题")
	}
	start := ev.StartMinute()
	if start < 0 {
		return fmt.Errorf("时间线事件时间不合法（应形如 \"D3 09:00\"）")
	}
	if start < e.State.Minute {
		return fmt.Errorf("不能新增已经过去的事件")
	}
	if ev.Location != "" && e.Pkg.Locations[ev.Location] == nil {
		return fmt.Errorf("地点 %s 不存在", ev.Location)
	}
	if e.State.EventDef(e.Pkg, ev.ID) != nil {
		return fmt.Errorf("事件 %s 已存在", ev.ID)
	}
	if len(ev.Outcomes) == 0 {
		ev.Outcomes = []timeline.Outcome{{ID: "happens", Title: ev.Title, Default: true}}
	}
	for _, o := range ev.Outcomes {
		if len(o.Effects) > 0 && e.Source != change.SourceUserRequest {
			return fmt.Errorf("AI 新增事件的结局不能带机械效果")
		}
	}
	if ev.Importance > 3 && e.Source != change.SourceUserRequest {
		ev.Importance = 3
	}
	ev.Pivotal = false
	ev.Resolve = "rules"
	if ev.Canon == "" || ev.Canon == "core" {
		ev.Canon = "minor"
	}
	ev.Preconditions = ""
	for i := range ev.Outcomes {
		ev.Outcomes[i].When = ""
	}
	for i := range ev.Hooks {
		ev.Hooks[i].Requires = ""
	}
	b, _ := json.Marshal(ev)
	c.Value = b
	return nil
}

func (e *Env) checkTimelinePatch(c *change.Change) error {
	ev := e.State.EventDef(e.Pkg, c.Target)
	if ev == nil {
		return fmt.Errorf("世界事件 %s 不存在", c.Target)
	}
	st := e.State.Timeline.Status(c.Target)
	if st == timeline.Resolved || st == timeline.Cancelled {
		return fmt.Errorf("事件已%s", timeline.StatusLabel(st))
	}
	var rt *timeline.Runtime
	if e.State.Timeline != nil {
		rt = e.State.Timeline.Events[c.Target]
	}
	switch {
	case c.Path == "outcome":
		var o string
		if json.Unmarshal(c.Value, &o) != nil || ev.OutcomeByID(o) == nil {
			return fmt.Errorf("结局必须是事件声明的 outcomes 之一")
		}
		prev := ""
		if rt != nil {
			prev = rt.Forced
		}
		c.Before, _ = json.Marshal(prev)
	case strings.HasPrefix(c.Path, "vars."):
		var v int
		if json.Unmarshal(c.Value, &v) != nil {
			return fmt.Errorf("事件变量必须是整数")
		}
		k := strings.TrimPrefix(c.Path, "vars.")
		cur := ev.Vars[k]
		if rt != nil {
			if x, ok := rt.Vars[k]; ok {
				cur = x
			}
		}
		c.Before, _ = json.Marshal(cur)
	case slices.Contains([]string{"window.start", "window.end", "location", "summary", "title"}, c.Path):
		var v string
		if json.Unmarshal(c.Value, &v) != nil {
			return fmt.Errorf("值必须是文本")
		}
		if strings.HasPrefix(c.Path, "window.") {
			m, err := timeline.ParseTime(v)
			if err != nil || m < e.State.Minute {
				return fmt.Errorf("时间不合法或已经过去")
			}
		}
		if c.Path == "location" && e.Pkg.Locations[v] == nil {
			return fmt.Errorf("地点不存在")
		}
		var prev json.RawMessage
		if rt != nil {
			prev = rt.Patches[c.Path]
		}
		if len(prev) == 0 {
			prev = json.RawMessage("null")
		}
		c.Before = prev
	default:
		return fmt.Errorf("时间线路径 %s 不可修改", c.Path)
	}
	return nil
}

func (e *Env) checkTimelineCancel(c *change.Change) error {
	if e.State.EventDef(e.Pkg, c.Target) == nil {
		return fmt.Errorf("世界事件 %s 不存在", c.Target)
	}
	st := e.State.Timeline.Status(c.Target)
	if st == timeline.Resolved || st == timeline.Cancelled {
		return fmt.Errorf("事件已%s", timeline.StatusLabel(st))
	}
	c.Before, _ = json.Marshal(st)
	return nil
}

// Summary 返回变更的玩家可见摘要（涉及隐藏真相时脱敏）。
func Summary(e *Env, c change.Change) string {
	name := func(id string) string {
		if id == loader.PlayerID {
			return e.State.Player.Name
		}
		if d := e.doc(id); d != nil {
			return d.Name()
		}
		if ev := e.State.EventDef(e.Pkg, id); ev != nil {
			return ev.Title
		}
		return id
	}
	val := func(raw json.RawMessage) string {
		var v any
		_ = json.Unmarshal(raw, &v)
		if s, ok := v.(string); ok && (e.doc(s) != nil || e.Pkg.Locations[s] != nil) {
			return name(s)
		}
		return clip(overlay.Text(v), 40)
	}
	if c.Hidden {
		return name(c.Target) + "：某个秘密发生了变化"
	}
	switch c.Op {
	case change.OpCreate:
		var d overlay.EntityDoc
		_ = json.Unmarshal(c.Value, &d)
		return "新" + kindName(c.Kind) + "：" + d.Name()
	case change.OpPatch:
		if change.IsPlayerPath(c.Target, c.Path) {
			var d int
			_ = json.Unmarshal(c.Value, &d)
			switch c.Path {
			case "gold":
				return fmt.Sprintf("金钱 %+d", d)
			case "xp":
				return fmt.Sprintf("经验 %+d", d)
			case "hp":
				return fmt.Sprintf("生命 %+d", d)
			}
			return fmt.Sprintf("%s ×%+d", name(strings.TrimPrefix(c.Path, "inventory.")), d)
		}
		if strings.Contains(c.Target, ">") {
			a, b, _ := strings.Cut(c.Target, ">")
			var d int
			_ = json.Unmarshal(c.Value, &d)
			dim, _ := e.Pkg.Relations.Dimension(strings.TrimPrefix(c.Path, "dims."))
			return fmt.Sprintf("%s→%s %s %+d", name(a), name(b), dim.Name, d)
		}
		if c.Path == "location" {
			return fmt.Sprintf("%s 去了 %s", name(c.Target), val(c.Value))
		}
		field := strings.TrimPrefix(strings.TrimPrefix(c.Path, "fields."), "combat.stats.")
		return fmt.Sprintf("%s：%s → %s", name(c.Target), knowledge.FieldLabel(field), val(c.Value))
	case change.OpRetire:
		var r overlay.Retirement
		_ = json.Unmarshal(c.Value, &r)
		return name(c.Target) + " " + RetireLabel(r.Reason)
	case change.OpRestore:
		return name(c.Target) + " 回归"
	case change.OpLink:
		a, b, _ := strings.Cut(c.Target, ">")
		return fmt.Sprintf("%s 与 %s 产生关联：%s", name(a), name(b), val(c.Value))
	case change.OpUnlink:
		a, b, _ := strings.Cut(c.Target, ">")
		return fmt.Sprintf("%s 与 %s 的关联解除", name(a), name(b))
	case change.OpTimelineAdd:
		var ev timeline.Event
		_ = json.Unmarshal(c.Value, &ev)
		return "新的世界事件：" + ev.Title
	case change.OpTimelinePatch:
		if c.Path == "outcome" {
			ev := e.State.EventDef(e.Pkg, c.Target)
			var o string
			_ = json.Unmarshal(c.Value, &o)
			if ev != nil && ev.OutcomeByID(o) != nil && ev.OutcomeByID(o).Title != "" {
				o = ev.OutcomeByID(o).Title
			}
			return name(c.Target) + " 的走向改变：" + o
		}
		return name(c.Target) + " 有了变化"
	case change.OpTimelineCancel:
		return name(c.Target) + " 被取消"
	}
	return name(c.Target)
}

// RetireLabel 返回退场原因的中文。
func RetireLabel(r string) string {
	switch r {
	case "death":
		return "死亡"
	case "destroyed":
		return "被毁"
	case "disbanded":
		return "解散"
	case "lost":
		return "遗失"
	case "left":
		return "离开"
	}
	return "退场"
}

func kindName(k string) string {
	switch k {
	case "character":
		return "人物"
	case "location":
		return "地点"
	case "faction":
		return "势力"
	case "item", "weapon":
		return "物品"
	case "mech":
		return "机甲"
	}
	return "设定"
}
