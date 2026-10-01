package query

import (
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

var retireLabels = map[string]string{"death": "已死亡", "destroyed": "已毁灭", "disbanded": "已解散", "lost": "已遗失", "left": "已离开", "retired": "已退场"}

// EntityView 返回知识层过滤后的实体视图（一切从未知开始：未解锁字段只显示“未知”）。
// 玩家对该实体一无所知时返回 false。
func (q *Q) EntityView(s *state.State, id string) (dto.EntityViewV1, bool) {
	d := s.Doc(q.Pkg, id)
	if d == nil {
		return dto.EntityViewV1{}, false
	}
	player := id == loader.PlayerID
	if !player && !s.Knowledge.Aware(id) && !q.knowsField(s, d, "name") {
		return dto.EntityViewV1{}, false
	}
	v := dto.EntityViewV1{ID: id, Kind: d.Kind, Name: q.KnownName(s, id), Icon: q.icon(id), Fields: []dto.EntityFieldV1{}}
	for _, f := range knowledge.DisplayFields(d.Kind) {
		if f == "name" || f == "title" {
			continue
		}
		val := d.Field(f)
		if val == "" && f != "status" {
			continue
		}
		lv := knowledge.Known
		if !player {
			lv = q.FieldLevel(s, d, f)
		}
		fv := dto.EntityFieldV1{Key: f, Label: knowledge.FieldLabel(f), Level: levelCode(lv), Changed: slices.Contains(d.Changed, f)}
		switch {
		case f == "status":
			fv.Value = "存活"
			if d.Retired != nil {
				fv.Value = retireLabels[d.Retired.Reason]
			}
			if !player && lv < knowledge.Known && d.Retired == nil {
				fv.Value = ""
			}
		case lv >= knowledge.Rumored:
			fv.Value = q.namesIn(s, val, lv >= knowledge.Known)
		}
		if fv.Value == "" {
			fv.Level = "unknown"
		}
		v.Fields = append(v.Fields, fv)
	}
	for _, h := range d.Hidden {
		if s.Knowledge.Knows(id, "hidden:"+h.ID) {
			v.Secrets = append(v.Secrets, h.Text)
		}
	}
	if d.Retired != nil {
		v.Retired = retireLabels[d.Retired.Reason]
	}
	switch d.Kind {
	case "location":
		v.Here = s.Player.Location == id
	case "character":
		if n, ok := s.NPCs[id]; ok {
			v.Here = n.Location == s.Player.Location && d.Retired == nil
		}
	}
	known := 0
	for _, f := range v.Fields {
		if f.Level != "unknown" {
			known++
		}
	}
	if len(v.Fields) > 0 && !player {
		v.Progress = strings.Repeat("●", known) + strings.Repeat("○", len(v.Fields)-known)
	}
	return v, true
}

// Entities 返回玩家知道的某类实体（故事包 + AI 新建），玩家与在场者在前。
func (q *Q) Entities(s *state.State, kind string) []dto.EntityViewV1 {
	ids := []string{}
	seen := map[string]bool{}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if kind == "character" {
		add(loader.PlayerID)
	}
	for _, id := range slices.Sorted(func(yield func(string) bool) {
		for id := range q.Pkg.Docs {
			if !yield(id) {
				return
			}
		}
	}) {
		add(id)
	}
	if s.World != nil {
		for _, id := range slices.Sorted(func(yield func(string) bool) {
			for id := range s.World.Created {
				if !yield(id) {
					return
				}
			}
		}) {
			add(id)
		}
	}
	out := []dto.EntityViewV1{}
	for _, id := range ids {
		d := s.Doc(q.Pkg, id)
		if d == nil || d.Kind != kind {
			continue
		}
		if v, ok := q.EntityView(s, id); ok {
			out = append(out, v)
		}
	}
	slices.SortStableFunc(out, func(a, b dto.EntityViewV1) int {
		rank := func(v dto.EntityViewV1) int {
			switch {
			case v.ID == loader.PlayerID:
				return 0
			case v.Here:
				return 1
			case v.Retired != "":
				return 3
			}
			return 2
		}
		return rank(a) - rank(b)
	})
	return out
}

// TimelineEvents 返回世界面板“时间线”页：玩家知道的全部事件（含已结束的结果）。
func (q *Q) TimelineEvents(s *state.State) []dto.WorldEventChipV1 {
	out := []dto.WorldEventChipV1{}
	for _, id := range s.EventIDs(q.Pkg) {
		ev := s.EventDef(q.Pkg, id)
		if ev == nil {
			continue
		}
		known := ev.Known || s.Knowledge.LevelOf(id, "existence") > 0
		if !known {
			continue
		}
		st := s.Timeline.Status(id)
		c := dto.WorldEventChipV1{ID: id, Title: ev.Title, Status: st, StatusLabel: timeline.StatusLabel(st), Location: q.KnownName(s, ev.Location),
			Rumored: !ev.Known && !s.Knowledge.Knows(id, "existence"), Summary: ev.Summary, Pivotal: ev.Pivotal}
		if c.Rumored {
			// 只听过传闻：不给出真实摘要与地点，只给传闻原文
			c.Summary, c.Location = "", Unknown
			if len(ev.Rumor) > 0 {
				c.Summary = "传闻：" + ev.Rumor[0].Text
			}
		}
		if m := ev.StartMinute(); m >= 0 {
			c.Time = worldtime.Format(m)
			if st != timeline.Resolved && st != timeline.Cancelled && st != timeline.Active {
				c.Countdown = timeline.Countdown(s.Minute, m)
				c.StartsIn = m - s.Minute
			}
		}
		if s.Timeline != nil && s.Timeline.Events[id] != nil {
			r := s.Timeline.Events[id]
			for _, o := range ev.Outcomes {
				if o.ID == r.Outcome {
					c.Outcome = o.Title
					if c.Outcome == "" {
						c.Outcome = o.Text
					}
				}
			}
			if c.Outcome == "" && r.Reason != "" {
				c.Outcome = r.Reason
			}
		}
		out = append(out, c)
	}
	return out
}

func levelCode(l knowledge.Level) string {
	switch l {
	case knowledge.Known:
		return "known"
	case knowledge.Rumored:
		return "rumored"
	}
	return "unknown"
}

func (q *Q) icon(id string) string {
	if c := q.Pkg.Characters[id]; c != nil {
		return c.Icon
	}
	if id == loader.PlayerID && q.Pkg.Player != nil {
		return q.Pkg.Player.Icon
	}
	if f := q.Pkg.Factions[id]; f != nil {
		return f.Icon
	}
	return ""
}

// namesIn 把字段值里的实体 ID（单个或逗号 / 顿号分隔的列表）换成玩家知道的名字。
// known=true 时，被引用实体的名字随字段一起得知（知道“托克属于工匠行会”就知道了行会的名字）。
func (q *Q) namesIn(s *state.State, val string, known bool) string {
	if !strings.Contains(val, ":") || !strings.Contains(val, "/") {
		return val
	}
	parts := strings.FieldsFunc(val, func(r rune) bool { return r == ',' || r == '，' || r == '、' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.Contains(p, ":") && strings.Contains(p, "/") && !strings.ContainsAny(p, " 　") {
			n := q.KnownName(s, p)
			if d := s.Doc(q.Pkg, p); known && n == Unknown && d != nil && d.Name() != "" {
				n = d.Name()
			}
			p = n
		}
		out = append(out, p)
	}
	return strings.Join(out, "、")
}
