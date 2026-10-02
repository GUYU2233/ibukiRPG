package query

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/combat/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/overlay"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// KnownName 返回玩家视角的名字：知道名字 → 真名；只知道存在 → 外号（alias_unknown）；否则“未知”。
func (q *Q) KnownName(s *state.State, id string) string {
	if id == loader.PlayerID {
		return s.Player.Name
	}
	d := s.Doc(q.Pkg, id)
	if d == nil {
		return q.Pkg.EntityName(id)
	}
	if q.knowsField(s, d, "name") || q.knowsField(s, d, "title") {
		return d.Name()
	}
	if d.AliasUnknown != "" {
		return d.AliasUnknown
	}
	switch d.Kind {
	case "character":
		return "陌生人"
	case "location":
		return "未知的地方"
	}
	return Unknown
}

// knowsField：公开字段在玩家“见过”该实体（知道其存在）时视为已知。
func (q *Q) knowsField(s *state.State, d *overlay.EntityDoc, f string) bool {
	if s.Knowledge.Knows(d.ID, f) {
		return true
	}
	return slices.Contains(d.Public, f) && s.Knowledge.Aware(d.ID) && s.Knowledge.Knows(d.ID, "existence")
}

// FieldLevel 返回玩家对字段的认知等级。
func (q *Q) FieldLevel(s *state.State, d *overlay.EntityDoc, f string) knowledge.Level {
	if q.knowsField(s, d, f) {
		return knowledge.Known
	}
	return s.Knowledge.LevelOf(d.ID, f)
}

// ChangeView 把一条世界变更日志转成玩家视图（隐藏真相脱敏）。
func (q *Q) ChangeView(s *state.State, e overlay.LogEntry) dto.WorldChangeV1 {
	v := dto.WorldChangeV1{ID: e.ID, Op: e.Op, Target: e.Target, TargetName: q.KnownName(s, e.Target), Path: e.Path, Summary: e.Summary,
		Reason: e.Reason, Source: e.Source, SourceLabel: change.SourceLabel(e.Source), Turn: e.Turn, Time: worldtime.Format(e.Minute),
		Impact: e.Impact.Score, Types: e.Impact.Types, RevertedBy: e.RevertedBy, Reverts: e.Reverts, Hidden: e.Hidden}
	v.Mechanical = e.IsMechanical(q.Pkg.KindOf)
	if e.Hidden {
		v.Summary = fmt.Sprintf("与%s有关的一项隐藏设定发生了变化", v.TargetName)
		v.Reason = ""
	} else {
		v.Before, v.After = rawText(e.Before), rawText(e.Value)
	}
	v.CanRevert = e.RevertedBy == "" && e.Reverts == ""
	return v
}

func rawText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return string(raw)
	}
	t := overlay.Text(v)
	if r := []rune(t); len(r) > 80 {
		t = string(r[:80]) + "…"
	}
	return t
}

// WorldChanges 返回世界变更日志（新的在前），可按关键词与来源筛选。
func (q *Q) WorldChanges(s *state.State, query, source string, limit int) []dto.WorldChangeV1 {
	if s.World == nil {
		return []dto.WorldChangeV1{}
	}
	out := []dto.WorldChangeV1{}
	for i := len(s.World.Log) - 1; i >= 0; i-- {
		v := q.ChangeView(s, s.World.Log[i])
		if source != "" && !strings.HasPrefix(v.Source, source) {
			continue
		}
		if query != "" && !strings.Contains(v.Summary+v.TargetName+v.Reason, query) {
			continue
		}
		out = append(out, v)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// RevealChip 把一次揭示转成叙事流 chip。
func (q *Q) RevealChip(s *state.State, r knowledge.Reveal) []dto.KnowledgeChipV1 {
	var out []dto.KnowledgeChipV1
	for _, f := range r.Fields {
		c := dto.KnowledgeChipV1{Entity: r.Entity, Name: q.KnownName(s, r.Entity), Field: f, FieldLabel: knowledge.FieldLabel(f), Level: "known", Channel: r.Channel, Kind: "field"}
		if r.EffectiveLevel() == knowledge.Rumored {
			c.Level, c.Kind = "rumored", "rumor"
			if ev := s.EventDef(q.Pkg, r.Entity); ev != nil && c.Name == Unknown {
				c.Name = ev.Title // 传闻里听到的说法
			}
		}
		if strings.Contains(r.Entity, ">") {
			a, b, _ := strings.Cut(r.Entity, ">")
			c.Name, c.Kind = q.KnownName(s, a)+" → "+q.KnownName(s, b), "relation"
		}
		if f == "existence" {
			c.FieldLabel = "新发现"
		}
		out = append(out, c)
	}
	return out
}

// WorldLog 汇总一回合（含提交 B）的世界更新。没有内容时返回 nil。
func (q *Q) WorldLog(after *state.State, evs []event.Event) *dto.WorldLogV1 {
	w := &dto.WorldLogV1{}
	for _, e := range evs {
		switch e.Type {
		case event.WorldChangeApplied, event.WorldChangeReverted:
			if e.Data.Change != nil {
				le := overlay.LogEntry{Change: *e.Data.Change, Turn: e.Turn, Minute: e.Minute}
				if e.Type == event.WorldChangeReverted {
					le.Reverts = e.Data.Key
				}
				w.Changes = append(w.Changes, q.ChangeView(after, le))
			}
		case event.KnowledgeRevealed:
			if e.Data.Reveal != nil && e.Data.Reveal.Channel != knowledge.ChannelSystem {
				w.Reveals = append(w.Reveals, q.RevealChip(after, *e.Data.Reveal)...)
			}
		case event.WorldUpdateFailed:
			w.Failed = e.Data.Reason
			w.Rejected = e.Data.Delta
		case event.ImpactAssessed:
			if e.Data.Impact != nil {
				w.Impact = e.Data.Impact.Score
			}
		}
	}
	if len(w.Changes) == 0 && len(w.Reveals) == 0 && w.Failed == "" {
		return nil
	}
	return w
}

// DecisionView 返回待决 / 通知中的偏离提示。
func (q *Q) DecisionView(s *state.State, settings change.Settings) *dto.DecisionV1 {
	d := s.Pending
	if d == nil {
		d = s.Notice
	}
	if d == nil {
		return nil
	}
	v := &dto.DecisionV1{ID: d.ID, Type: d.Type, TypeLabel: change.TypeLabel(d.Type), Title: d.Title, Summary: d.Summary, Score: d.Score,
		Notify: d.Notify, RollbackTurn: d.Rollback.Turn, Checkpoint: d.Checkpoint, World: d.World}
	v.Sensitivity = sensLabel(settings.For(d.Type).Level)
	lines := d.Lines
	if len(lines) > 3 {
		v.More = len(lines) - 3
		lines = lines[:3]
	}
	v.Lines = lines
	v.RollbackText = fmt.Sprintf("回到回合 %d", d.Rollback.Turn)
	if d.World {
		v.RollbackText = "撤掉这次场外推进"
	}
	return v
}

func sensLabel(l string) string {
	switch l {
	case change.SensOff:
		return "关"
	case change.SensLow:
		return "低"
	case change.SensHigh:
		return "高"
	}
	return "中"
}

// AdjudicationView 把行动裁定事件转成裁定卡。
func AdjudicationView(a *freeform.Adjudication, targetName string) *dto.AdjudicationV1 {
	if a == nil {
		return nil
	}
	c := a.Check
	v := &dto.AdjudicationV1{Plausibility: c.Plausibility, PlausibilityLabel: freeform.PlausibilityLabel(c.Plausibility), ModTotal: c.ModTotal, DC: c.DC, Band: a.Band}
	var parse []string
	if c.Moved {
		parse = append(parse, "移动")
	}
	f := c.Final
	act := map[string]string{freeform.KindAttack: "攻击", freeform.KindSkill: "技能", freeform.KindManeuver: "战技", freeform.KindEnv: "利用环境",
		freeform.KindTalk: "交涉", freeform.KindDefend: "防御", freeform.KindFlee: "撤退", freeform.KindItem: "使用物品", freeform.KindMech: "机甲"}[f.Kind]
	if act == "" {
		act = f.Kind
	}
	if c.Skill != nil {
		act += "「" + c.Skill.Name + "」"
	}
	if c.Maneuver != nil {
		act += "「" + c.Maneuver.Name + "」"
	}
	parse = append(parse, act)
	if targetName != "" {
		parse = append(parse, targetName)
	}
	if c.Part != nil {
		p := c.Part.Name
		if c.Part.Known && c.Part.Weak {
			p += "（弱点）"
		}
		parse = append(parse, p)
	}
	if c.Means != nil {
		parse = append(parse, c.Means.Name)
	}
	v.Parse = strings.Join(parse, " · ")
	for _, m := range c.Mods {
		v.Mods = append(v.Mods, fmt.Sprintf("%s %+d", m.Label, m.Value))
	}
	if c.Downgraded {
		v.Downgrade = c.DowngradeReason
	}
	if a.Roll != nil {
		r := a.Roll
		v.Rolls, v.Total, v.Degree, v.DegreeLabel, v.Margin, v.Dice = r.Rolls, r.Total, r.Degree, freeform.DegreeLabel(r.Degree), r.Margin, r.Dice
	}
	var res []string
	if a.Damage > 0 {
		t := fmt.Sprintf("伤害 %d", a.Damage)
		if c.Part != nil && c.Part.Mult != 100 && c.Part.Mult > 0 {
			t += fmt.Sprintf("（%s ×%.1f）", c.Part.Name, float64(c.Part.Mult)/100)
		}
		res = append(res, t)
	}
	if a.Status != "" {
		res = append(res, "附加状态")
	}
	if a.PartRevealed {
		res = append(res, "发现了弱点")
	}
	if a.PartBroken {
		res = append(res, "部位被破坏")
	}
	for _, cs := range a.Consequences {
		res = append(res, freeform.ConsequenceLabel(cs))
	}
	v.Result = strings.Join(res, " · ")
	return v
}

// WorldEventChipV1 是状态条上的世界事件倒计时。
func (q *Q) Upcoming(s *state.State, n int) []dto.WorldEventChipV1 {
	var out []dto.WorldEventChipV1
	for _, id := range s.EventIDs(q.Pkg) {
		ev := s.EventDef(q.Pkg, id)
		if ev == nil {
			continue
		}
		st := s.Timeline.Status(id)
		if st == timeline.Resolved || st == timeline.Cancelled {
			continue
		}
		known := ev.Known || s.Knowledge.LevelOf(id, "existence") > 0
		if !known {
			continue
		}
		c := dto.WorldEventChipV1{ID: id, Title: ev.Title, Status: st, StatusLabel: timeline.StatusLabel(st), Location: q.KnownName(s, ev.Location),
			Rumored: !ev.Known && !s.Knowledge.Knows(id, "existence")}
		if st == timeline.Active {
			c.Countdown = "进行中"
		} else {
			c.Countdown = timeline.Countdown(s.Minute, ev.StartMinute())
			c.StartsIn = ev.StartMinute() - s.Minute
		}
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(a, b dto.WorldEventChipV1) int { return int(a.StartsIn - b.StartsIn) })
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// RevertPlan 返回撤销 id 前的级联检查（依赖链 + 能否单独撤销）。
func (q *Q) RevertPlan(s *state.State, id string) (dto.RevertPlanV1, error) {
	e := s.World.Find(id)
	if e == nil {
		return dto.RevertPlanV1{}, fmt.Errorf("没有这条世界变更")
	}
	p := dto.RevertPlanV1{Change: q.ChangeView(s, *e)}
	for _, d := range s.World.Chain(id) {
		if de := s.World.Find(d); de != nil {
			p.Dependents = append(p.Dependents, q.ChangeView(s, *de))
		}
	}
	ok, shadow, why := s.World.SinglePlan(id)
	p.CanSingle = ok && p.Change.CanRevert
	switch {
	case !ok:
		p.SingleNote = why
	case shadow != "":
		who := shadow
		if se := s.World.Find(shadow); se != nil {
			who = fmt.Sprintf("回合 %d 的变更", se.Turn)
		}
		p.SingleNote = fmt.Sprintf("这个值之后被%s改过：只撤销这一条时当前值不变，以后再撤销那条变更会回到这条之前的值", who)
	case len(p.Dependents) > 0:
		p.SingleNote = "之后的变更与它互不覆盖，可以只撤销这一条"
	}
	return p, nil
}
