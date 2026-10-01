package engine

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/overlay"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
)

func systemCommand(cmd command.Command) bool {
	return cmd.Kind == command.KindWorldChange || cmd.Kind == command.KindDecision
}

// ---------- 开局：角色创建与初始认知 ----------

// Creation 是开局角色创建的选择（第 13 节）。
type Creation struct {
	Name        string         `json:"name,omitempty"`
	Preset      string         `json:"preset,omitempty"` // 预设主角 ID（空 = 故事包玩家模板）
	Custom      bool           `json:"custom,omitempty"`
	Background  string         `json:"background,omitempty"`
	Appearance  string         `json:"appearance,omitempty"`
	Personality string         `json:"personality,omitempty"`
	Story       string         `json:"story,omitempty"`
	Attributes  map[string]int `json:"attributes,omitempty"` // 自建角色的属性分配（增量）
}

// CheckCreation 按 rules/creation.yaml 做确定性检查（审查 Agent 之前的规则层），返回问题列表。
func CheckCreation(p *loader.Package, c Creation) []string {
	var out []string
	cr := &p.Creation
	if c.Custom {
		total := 0
		for k, v := range c.Attributes {
			if _, ok := p.Rules.Attributes[k]; !ok {
				out = append(out, "未知属性："+k)
				continue
			}
			base := p.Player.Attributes[k]
			if v < 0 {
				out = append(out, p.Rules.Attributes[k]+" 不能减少")
			}
			if base+v > cr.AttrMax {
				out = append(out, fmt.Sprintf("%s 超过上限 %d", p.Rules.Attributes[k], cr.AttrMax))
			}
			total += v
		}
		if total > cr.AttributePoints {
			out = append(out, fmt.Sprintf("属性点超出：%d / %d", total, cr.AttributePoints))
		}
		if len(cr.Backgrounds) > 0 && cr.Background(c.Background) == nil {
			out = append(out, "请选择出身")
		}
		text := c.Name + c.Appearance + c.Personality + c.Story
		for _, f := range cr.Forbidden {
			if f != "" && strings.Contains(text, f) {
				out = append(out, "设定里不能出现“"+f+"”")
			}
		}
		if len([]rune(c.Story)) > 400 || len([]rune(c.Appearance)) > 200 || len([]rune(c.Personality)) > 200 {
			out = append(out, "文字太长（背景 ≤ 400 字，外貌 / 性格 ≤ 200 字）")
		}
	} else if c.Preset != "" && c.Preset != loader.PlayerID {
		ch := p.Characters[c.Preset]
		if ch == nil || !ch.Playable {
			out = append(out, "没有这个预设角色")
		}
	}
	return out
}

// PowerScore 返回角色的强度分（审查与卡片生成的上限比较）：属性 ×6 + 技能 ×4 + 装备加成。
func PowerScore(p *loader.Package, attrs map[string]int, skills map[string]int, items map[string]int) int {
	v := 0
	for _, x := range attrs {
		v += x * 6
	}
	for _, x := range skills {
		v += x * 4
	}
	for id, q := range items {
		if it := p.Items[id]; it != nil && q > 0 && (it.Kind == "weapon" || it.Kind == "armor") {
			v += it.Mods["atk"] + it.Mods["def"] + 2
		}
	}
	return v
}

// Setup 在新存档的初始状态上应用角色创建与开局认知（一切从未知开始，第 8.1 节）。
func Setup(p *loader.Package, s *state.State, c Creation) error {
	if probs := CheckCreation(p, c); len(probs) > 0 {
		return fmt.Errorf("%s", strings.Join(probs, "；"))
	}
	pr := &state.Profile{Preset: c.Preset, Background: c.Background, Appearance: c.Appearance, Personality: c.Personality, Story: c.Story, Custom: c.Custom}
	known := slices.Clone(p.Manifest.Start.Known)
	if c.Preset != "" && c.Preset != loader.PlayerID {
		ch := p.Characters[c.Preset]
		if ch == nil || !ch.Playable {
			return fmt.Errorf("没有这个预设角色")
		}
		if strings.TrimSpace(c.Name) == "" {
			s.Player.Name = ch.Name()
		}
		for k, v := range ch.Attributes {
			s.Player.Attributes[k] = v
		}
		for k, v := range ch.Skills {
			s.Player.Skills[k] = v
		}
		if ch.Location != "" {
			s.Player.Location = ch.Location
		}
		if pr.Appearance == "" {
			pr.Appearance = ch.Description
		}
		if pr.Story == "" {
			pr.Story = ch.Lore
		}
		// 预设主角本人从 NPC 列表中移除（不会和自己相遇）
		delete(s.NPCs, c.Preset)
		for _, sk := range ch.StartKnowledge {
			for _, f := range sk.Fields {
				known = append(known, sk.Entity+"#"+f)
			}
			if len(sk.Fields) == 0 {
				known = append(known, sk.Entity)
			}
		}
	}
	if c.Custom {
		for k, v := range c.Attributes {
			s.Player.Attributes[k] += v
		}
	}
	if bg := p.Creation.Background(c.Background); bg != nil {
		for k, v := range bg.Attributes {
			s.Player.Attributes[k] += v
		}
		for k, v := range bg.Items {
			s.Player.Inventory[k] += v
		}
		s.Player.Gold += bg.Gold
		for _, sk := range bg.Skills {
			s.Player.Skills[sk]++
		}
		if bg.Location != "" {
			s.Player.Location = bg.Location
		}
		known = append(known, bg.Known...)
	}
	if n := strings.TrimSpace(c.Name); n != "" {
		s.Player.Name = n
	}
	s.Player.Profile = pr
	// 初始认知：玩家本人全部已知；开局地点公开字段；start.known 与预设主角 / 出身的条目
	k := s.K()
	k.Apply(knowledge.Reveal{Entity: loader.PlayerID, Fields: allFields(s.Doc(p, loader.PlayerID)), Channel: knowledge.ChannelSystem}, 0)
	revealPublic(p, s, s.Player.Location, 0)
	for _, x := range known {
		id, field, ok := strings.Cut(x, "#")
		d := s.Doc(p, id)
		if d == nil {
			continue
		}
		if ok {
			k.Apply(knowledge.Reveal{Entity: id, Fields: []string{field}, Channel: knowledge.ChannelSystem}, 0)
		} else {
			fs := append(slices.Clone(d.Public), "name")
			k.Apply(knowledge.Reveal{Entity: id, Fields: fs, Channel: knowledge.ChannelSystem}, 0)
		}
	}
	for _, id := range s.EventIDs(p) {
		if ev := s.EventDef(p, id); ev != nil && ev.Known {
			k.Apply(knowledge.Reveal{Entity: id, Fields: []string{"existence", "time", "location"}, Channel: knowledge.ChannelSystem}, 0)
		}
	}
	return nil
}

func allFields(d *overlay.EntityDoc) []string {
	if d == nil {
		return nil
	}
	var out []string
	for k := range d.Fields {
		if knowledge.ValidField(d.Kind, k) {
			out = append(out, k)
		}
	}
	out = append(out, "existence", "name", "status")
	slices.Sort(out)
	return slices.Compact(out)
}

func revealPublic(p *loader.Package, s *state.State, id string, turn int) {
	d := s.Doc(p, id)
	if d == nil {
		return
	}
	s.K().Apply(knowledge.Reveal{Entity: id, Fields: d.Public, Channel: knowledge.ChannelWitness, Rev: s.Rev(id)}, turn)
}

// ---------- 回合末：目击揭示 ----------

// runKnowledge 写入本回合玩家亲眼看到的公开字段：所在地点、在场人物、到手的物品、交手的敌人。
func (w *work) runKnowledge() error {
	p, s := w.pkg(), w.s
	var ids []string
	ids = append(ids, s.Player.Location)
	env := &validate.Env{Pkg: p, State: s}
	ids = append(ids, validate.Present(env)...)
	for _, id := range p.ItemIDs {
		if s.Player.Inventory[id] > 0 {
			ids = append(ids, id)
		}
	}
	if s.RPG != nil && s.RPG.Combat != nil {
		for _, u := range s.RPG.Combat.Units {
			if u.Side == "enemy" && p.Doc(u.Ref) != nil {
				ids = append(ids, u.Ref)
			}
		}
	}
	for _, id := range ids {
		d := s.Doc(p, id)
		if d == nil {
			continue
		}
		var fields []string
		for _, f := range append([]string{"existence"}, d.Public...) {
			if s.Knowledge.LevelOf(id, f) < knowledge.Known && knowledge.ValidField(d.Kind, f) || f == "existence" && !s.Knowledge.Aware(id) {
				fields = append(fields, f)
			}
		}
		if len(fields) == 0 {
			continue
		}
		if err := w.emit(event.KnowledgeRevealed, event.Data{Reveal: &knowledge.Reveal{Entity: id, Fields: fields, Channel: knowledge.ChannelWitness, Rev: s.Rev(id)}}); err != nil {
			return err
		}
	}
	return nil
}

// ---------- 世界事件时间线（第 6 节）----------

func (w *work) eventVars(ev *timeline.Event, rt *timeline.Runtime) expression.Vars {
	v := w.vars("", "")
	vars := map[string]any{}
	for k, x := range ev.Vars {
		vars[k] = int64(x)
	}
	if rt != nil {
		for k, x := range rt.Vars {
			vars[k] = int64(x)
		}
	}
	v["event"] = map[string]any{"id": ev.ID, "vars": vars}
	return v
}

func (w *work) evalEvent(expr string, ev *timeline.Event, rt *timeline.Runtime) bool {
	if expr == "" {
		return true
	}
	ok, err := w.eng.Eval.EvalBool(expr, w.eventVars(ev, rt))
	return err == nil && ok
}

// playerInvolved 报告玩家是否“在场”：事件地点 = 玩家位置，或玩家是参与者。
func (w *work) playerInvolved(ev *timeline.Event) bool {
	return ev.Location == w.s.Player.Location || slices.Contains(ev.Participants, loader.PlayerID)
}

// runTimeline 处理到期的世界事件：开始、阶段推进、结算、取消、传闻扩散。interrupt 非空时，
// 玩家所在地有事件开始会写入打断原因（时间跳跃用）。
func (w *work) runTimeline(interrupt *string) error {
	p, s := w.pkg(), w.s
	now := s.Minute
	for _, id := range s.EventIDs(p) {
		ev := s.EventDef(p, id)
		if ev == nil {
			continue
		}
		rt := eventRT(s, id)
		status := s.Timeline.Status(id)
		start, end := ev.StartMinute(), ev.EndMinute()
		// 传闻
		if err := w.spreadRumors(ev, rt); err != nil {
			return err
		}
		switch status {
		case timeline.Scheduled:
			if now < start {
				continue
			}
			if !w.evalEvent(ev.Preconditions, ev, rt) {
				if now >= end {
					if err := w.emit(event.WorldEventCancelled, event.Data{Story: id, Reason: "前提不再成立"}); err != nil {
						return err
					}
				}
				continue
			}
			stage := ""
			for _, st := range ev.Stages {
				if m, err := timeline.ParseTime(st.At); err == nil && m <= now {
					stage = st.ID
				}
			}
			if err := w.emit(event.WorldEventStarted, event.Data{Story: id, Step: stage, Values: ev.Vars, Location: ev.Location}); err != nil {
				return err
			}
			if w.playerInvolved(ev) {
				w.storyTouched = true
				if err := w.emit(event.KnowledgeRevealed, event.Data{Reveal: &knowledge.Reveal{Entity: id, Fields: []string{"existence", "time", "location", "participants"}, Channel: knowledge.ChannelWitness}}); err != nil {
					return err
				}
				if interrupt != nil && *interrupt == "" {
					*interrupt = ev.Title
				}
			}
			rt = eventRT(s, id)
			fallthrough
		case timeline.Active:
			stage := rt.Stage
			for _, st := range ev.Stages {
				if m, err := timeline.ParseTime(st.At); err == nil && m <= now {
					stage = st.ID
				}
			}
			if stage != rt.Stage {
				if err := w.emit(event.WorldEventStage, event.Data{Story: id, Step: stage}); err != nil {
					return err
				}
			}
			if now >= end && (end > start || now > start) {
				if err := w.resolveEvent(ev, "world"); err != nil {
					return err
				}
			}
		case timeline.Resolved:
			// 错过的事件：玩家到达事件地点或 6 小时后听说结果
			if rt != nil && !s.Knowledge.Knows(id, "outcome") && (s.Player.Location == ev.Location || now >= rt.Ended+360) && s.Knowledge.Aware(id) {
				o := ev.OutcomeByID(rt.Outcome)
				claim := ev.Title
				if o != nil && o.Title != "" {
					claim = ev.Title + "：" + o.Title
				}
				if err := w.emit(event.KnowledgeRevealed, event.Data{Reveal: &knowledge.Reveal{Entity: id, Fields: []string{"outcome"}, Channel: knowledge.ChannelRumor, Level: knowledge.Known, Claim: claim}}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *work) spreadRumors(ev *timeline.Event, rt *timeline.Runtime) error {
	s := w.s
	for i, r := range ev.Rumor {
		if rt != nil && slices.Contains(rt.Rumors, i) {
			continue
		}
		at := ev.StartMinute() - 1440
		if r.At != "" {
			if m, err := timeline.ParseTime(r.At); err == nil {
				at = m
			}
		}
		if s.Minute < at || (len(r.Where) > 0 && !slices.Contains(r.Where, s.Player.Location)) {
			continue
		}
		if s.Timeline.Status(ev.ID) == timeline.Resolved || s.Timeline.Status(ev.ID) == timeline.Cancelled {
			continue
		}
		if err := w.emit(event.RumorHeard, event.Data{Story: ev.ID, Delta: i, Text: r.Text, Reveal: &knowledge.Reveal{
			Entity: ev.ID, Fields: []string{"existence", "time", "location"}, Level: knowledge.Rumored, Channel: knowledge.ChannelRumor, Claim: r.Text,
		}}); err != nil {
			return err
		}
		rt = eventRT(s, ev.ID)
	}
	return nil
}

// resolveEvent 按 outcomes 条件选择结局（AI 指定的结局优先），应用结局效果。
func (w *work) resolveEvent(ev *timeline.Event, by string) error {
	rt := eventRT(w.s, ev.ID)
	if rt == nil {
		rt = &timeline.Runtime{Vars: ev.Vars}
	}
	var out *timeline.Outcome
	if rt.Forced != "" {
		out = ev.OutcomeByID(rt.Forced)
		by = "ai"
	}
	if out == nil {
		for i := range ev.Outcomes {
			o := &ev.Outcomes[i]
			if o.When != "" && w.evalEvent(o.When, ev, rt) {
				out = o
				break
			}
		}
	}
	if out == nil {
		out = ev.DefaultOutcome()
	}
	if len(rt.Joined) > 0 && by == "world" {
		by = "player"
	}
	oid := ""
	if out != nil {
		oid = out.ID
	}
	if err := w.emit(event.WorldEventResolved, event.Data{Story: ev.ID, Outcome: oid, Source: by, Title: ev.Title, Text: outcomeText(out)}); err != nil {
		return err
	}
	if out != nil {
		for _, eff := range out.Effects {
			if err := w.applyOutcome(eff, "", ""); err != nil {
				return err
			}
		}
	}
	if w.playerInvolved(ev) {
		w.storyTouched = true
		return w.emit(event.KnowledgeRevealed, event.Data{Reveal: &knowledge.Reveal{Entity: ev.ID, Fields: []string{"existence", "outcome"}, Channel: knowledge.ChannelWitness}})
	}
	return nil
}

func outcomeText(o *timeline.Outcome) string {
	if o == nil {
		return ""
	}
	if o.Text != "" {
		return o.Text
	}
	return o.Title
}

// ---------- 等待 / 时间跳跃（第 6.6 节）----------

// MaxWaitMinutes 是单次时间跳跃上限（7 天）。
const MaxWaitMinutes = 7 * 1440

// WaitMinutes 解析等待目标为分钟数。
func WaitMinutes(p *loader.Package, s *state.State, target string) (int64, error) {
	switch target {
	case "10m":
		return 10, nil
	case "1h", "":
		return 60, nil
	case "dawn":
		m := s.Minute % 1440
		if m < 6*60 {
			return 6*60 - m, nil
		}
		return 1440 - m + 6*60, nil
	}
	if id, ok := strings.CutPrefix(target, "event:"); ok {
		ev := s.EventDef(p, id)
		if ev == nil || !s.Knowledge.Aware(id) {
			return 0, fmt.Errorf("你不知道这件事")
		}
		d := ev.StartMinute() - s.Minute
		if d <= 0 {
			return 0, fmt.Errorf("那件事已经开始了")
		}
		return d, nil
	}
	if strings.HasSuffix(target, "m") {
		var n int64
		if _, err := fmt.Sscanf(target, "%dm", &n); err == nil && n > 0 {
			return n, nil
		}
	}
	if strings.HasSuffix(target, "h") {
		var n int64
		if _, err := fmt.Sscanf(target, "%dh", &n); err == nil && n > 0 {
			return n * 60, nil
		}
	}
	return 0, fmt.Errorf("不知道要等多久")
}

func (w *work) execWait(res *Result) error {
	total, err := WaitMinutes(w.pkg(), w.s, w.cmd.Target)
	if err != nil {
		return reject("%s", err.Error())
	}
	if total > MaxWaitMinutes {
		total = MaxWaitMinutes
	}
	var interrupt string
	for done := int64(0); done < total; {
		seg := min(total-done, 360)
		if err := w.emit(event.TimeAdvanced, event.Data{Minutes: seg, Reason: "wait"}); err != nil {
			return err
		}
		done += seg
		if err := w.runTimeline(&interrupt); err != nil {
			return err
		}
		if interrupt != "" && done < total {
			res.Interrupted = interrupt
			break
		}
	}
	w.actx = map[string]any{"kind": "wait", "id": "wait", "target": w.cmd.Target, "item": "", "success": true, "outcome": "success"}
	return nil
}

// ---------- 参与 / 破坏世界事件 ----------

func (w *work) execHook() error {
	p, s := w.pkg(), w.s
	ev := s.EventDef(p, w.cmd.Action)
	if ev == nil {
		return reject("没有这件事。")
	}
	if s.Timeline.Status(ev.ID) != timeline.Active {
		return reject("%s现在没有在进行。", ev.Title)
	}
	if !w.playerInvolved(ev) {
		return reject("你得先赶到%s。", w.name(ev.Location))
	}
	h := ev.HookByID(w.cmd.Target)
	if h == nil {
		return reject("这件事里没有这个切入点。")
	}
	rt := eventRT(s, ev.ID)
	if rt != nil && slices.Contains(rt.Joined, h.ID) {
		return reject("你已经这样做过了。")
	}
	if !w.evalEvent(h.Requires, ev, rt) {
		return reject("你还缺少条件，做不到“%s”。", h.Label)
	}
	typ := event.WorldEventJoined
	if slices.Contains(h.Tags, "disrupt") {
		typ = event.WorldEventDisrupted
	}
	if h.Minutes > 0 {
		if err := w.emit(event.TimeAdvanced, event.Data{Minutes: int64(h.Minutes), Reason: "hook"}); err != nil {
			return err
		}
	}
	if err := w.emit(typ, event.Data{Story: ev.ID, Key: h.ID, Values: h.Vars, Text: h.Label}); err != nil {
		return err
	}
	w.storyTouched = true
	if slices.Contains(h.Tags, "resolve") {
		if err := w.resolveEvent(ev, "player"); err != nil {
			return err
		}
	}
	w.actx = map[string]any{"kind": "hook", "id": h.ID, "target": ev.ID, "item": "", "success": true, "outcome": "success"}
	return nil
}

// ---------- 统一写入网关：世界变更与揭示（第 14.1 节）----------

// ValidationEnv 返回校验环境（CEL 带知识层变量）。
func (e *Engine) ValidationEnv(s *state.State, source, tier string) *validate.Env {
	return &validate.Env{Pkg: e.Pkg, State: s, Source: source, Tier: tier, Eval: func(expr string) (bool, error) {
		return e.Eval.EvalBool(expr, Vars(e.Pkg, s, "", ""))
	}}
}

func (w *work) execWorldChange(res *Result) error {
	if w.cmd.Action == "revert" {
		return w.revertChange(w.cmd.Target)
	}
	source := w.cmd.Source
	if source == "" {
		source = change.SourceNarrate
	}
	env := w.eng.ValidationEnv(w.s, source, w.cmd.Tier)
	changes, rejects := validate.Changes(env, w.cmd.Changes)
	rv := validate.Reveals(env, w.cmd.Reveals)
	res.Rejects = append(rejects, rv.Rejects...)
	res.Hidden = rv.Hidden
	if len(changes) == 0 && len(rv.Reveals) == 0 {
		if len(res.Rejects) > 0 {
			return w.emit(event.WorldUpdateFailed, event.Data{Reason: "validator_all_rejected", Delta: len(res.Rejects)})
		}
		return nil
	}
	var scores []int
	impact := &change.TurnImpact{HiddenReveals: rv.FreeHidden}
	for i, c := range changes {
		c.ID = fmt.Sprintf("wc-%d-%d", w.turn, w.s.WorldSeq()+1)
		if err := w.applyNative(c, false); err != nil {
			return err
		}
		if err := w.emit(event.WorldChangeApplied, event.Data{Change: &c}); err != nil {
			return err
		}
		changes[i] = c
		scores = append(scores, c.Impact.Score)
		impact.Types = append(impact.Types, c.Impact.Types...)
		impact.Changes = append(impact.Changes, c.ID)
		if slices.Contains(c.Impact.Types, change.TypeMajorDeath) {
			impact.DeathImportance = max(impact.DeathImportance, env.Meta(c).Importance)
		}
	}
	for _, r := range rv.Reveals {
		rr := r
		if err := w.emit(event.KnowledgeRevealed, event.Data{Reveal: &rr, Source: source}); err != nil {
			return err
		}
	}
	if rv.FreeHidden > 0 {
		impact.Types = append(impact.Types, change.TypeStoryImpact)
	}
	slices.Sort(impact.Types)
	impact.Types = slices.Compact(impact.Types)
	impact.Score = change.Aggregate(scores, w.cmd.SelfLevel, rv.FreeHidden)
	if impact.Score > 0 || len(impact.Types) > 0 {
		res.Impact = impact
		return w.emit(event.ImpactAssessed, event.Data{Impact: impact, Tags: impact.Changes})
	}
	return nil
}

// applyNative 为玩家机械状态 / 关系维度的变更发出原生事件（保证升级、背包等规则照常生效）。
func (w *work) applyNative(c change.Change, _ bool) error {
	if !change.IsDeltaPath(c.Target, c.Path) {
		return nil
	}
	var d int
	if err := json.Unmarshal(c.Value, &d); err != nil {
		return fmt.Errorf("native delta: %w", err)
	}
	if a, b, ok := strings.Cut(c.Target, ">"); ok {
		return w.emit(event.RelationEdgeChanged, event.Data{Actor: a, Target: b, Values: map[string]int{strings.TrimPrefix(c.Path, "dims."): d}, Reason: c.Reason})
	}
	switch {
	case c.Path == "gold":
		return w.emit(event.GoldChanged, event.Data{Delta: d, Reason: c.Reason})
	case c.Path == "xp":
		if d > 0 {
			return w.grantXP(d, c.Reason)
		}
		return w.emit(event.XPGained, event.Data{Delta: d, Reason: c.Reason})
	case c.Path == "hp":
		_, mx := PlayerHP(w.pkg(), w.s)
		hp, _ := PlayerHP(w.pkg(), w.s)
		d = max(min(d, mx-hp), 1-hp)
		return w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"hp": d}, Reason: c.Reason})
	case strings.HasPrefix(c.Path, "inventory."):
		it := strings.TrimPrefix(c.Path, "inventory.")
		if d > 0 {
			return w.emit(event.ItemAdded, event.Data{Item: it, Qty: d})
		}
		have := w.s.Player.Inventory[it]
		if q := min(-d, have); q > 0 {
			return w.emit(event.ItemRemoved, event.Data{Item: it, Qty: q})
		}
	}
	return nil
}

// revertChange 单项撤销（第 12.5 节）：写反向操作，不删除原事件；后续依赖它的变更需要先撤销。
func (w *work) revertChange(id string) error {
	o := w.s.World
	e := o.Find(id)
	if e == nil {
		return reject("没有这条世界变更。")
	}
	if e.RevertedBy != "" || e.Reverts != "" {
		return reject("这条变更已经撤销过了。")
	}
	if deps := o.Dependents(id); len(deps) > 0 {
		return reject("之后还有依赖它的变更（%s），请先撤销它们。", strings.Join(deps, "、"))
	}
	inv, err := e.Inverse()
	if err != nil {
		return reject("这条变更无法撤销：%v", err)
	}
	inv.ID = fmt.Sprintf("wc-%d-%d", w.turn, o.Seq+1)
	inv.Source = w.cmd.Source
	if inv.Source == "" {
		inv.Source = change.SourceUserRequest
	}
	inv.Summary = "撤销：" + e.Summary
	if err := w.applyNative(inv, true); err != nil {
		return err
	}
	return w.emit(event.WorldChangeReverted, event.Data{Change: &inv, Key: id})
}

// ---------- 偏离提示 ----------

func (w *work) execDecision() error {
	switch w.cmd.Action {
	case "request":
		if w.cmd.Decision == nil {
			return reject("缺少提示内容。")
		}
		return w.emit(event.DecisionRequested, event.Data{Decision: w.cmd.Decision})
	case "accept", "rollback", "dismiss":
		id := w.cmd.Target
		if w.s.Pending == nil && w.s.Notice == nil {
			return reject("没有待决的选择。")
		}
		outcome := map[string]string{"accept": "accepted", "rollback": "rolled_back", "dismiss": "dismissed"}[w.cmd.Action]
		return w.emit(event.DecisionResolved, event.Data{Key: id, Outcome: outcome})
	}
	return reject("未知的选择。")
}

func eventRT(s *state.State, id string) *timeline.Runtime {
	if s.Timeline == nil {
		return nil
	}
	return s.Timeline.Events[id]
}
