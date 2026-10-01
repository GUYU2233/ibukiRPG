package state

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/overlay"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
)

// Profile 是开局创建的玩家角色（预设或自建）。
type Profile struct {
	Preset      string `json:"preset,omitempty"`
	Background  string `json:"background,omitempty"`
	Appearance  string `json:"appearance,omitempty"`
	Personality string `json:"personality,omitempty"`
	Story       string `json:"story,omitempty"`
	Custom      bool   `json:"custom,omitempty"`
}

// W 返回覆盖层（惰性初始化）。
func (s *State) W() *overlay.Overlay {
	if s.World == nil {
		s.World = overlay.New()
	}
	return s.World
}

// K 返回玩家知识库（惰性初始化）。
func (s *State) K() *knowledge.Store {
	if s.Knowledge == nil {
		s.Knowledge = knowledge.New()
	}
	if s.Knowledge.Facts == nil {
		s.Knowledge.Facts = map[string]map[string]*knowledge.Fact{}
	}
	return s.Knowledge
}

// T 返回时间线运行时（惰性初始化）。
func (s *State) T() *timeline.State {
	if s.Timeline == nil {
		s.Timeline = timeline.New()
	}
	s.Timeline.Ensure()
	return s.Timeline
}

// Doc 返回实体的生效文档：静态设定（或新建实体）+ 覆盖层补丁 + 退场标记。没有时返回 nil。
func (s *State) Doc(p *loader.Package, id string) *overlay.EntityDoc {
	base := p.Doc(id)
	if base == nil && s.World != nil {
		base = s.World.Created[id]
	}
	if base == nil {
		return nil
	}
	d := s.World.ApplyDoc(base)
	if id == loader.PlayerID {
		d.Fields["name"] = s.Player.Name
		if s.Player.Profile != nil {
			if s.Player.Profile.Appearance != "" {
				d.Fields["appearance"] = s.Player.Profile.Appearance
			}
			if s.Player.Profile.Personality != "" {
				d.Fields["personality"] = s.Player.Profile.Personality
			}
			if s.Player.Profile.Story != "" {
				d.Fields["background"] = s.Player.Profile.Story
			}
		}
	}
	if n, ok := s.NPCs[id]; ok && d.Kind == "character" {
		d.Fields["location"] = n.Location
	}
	return d
}

// EventDef 返回世界事件定义（故事包或 AI 新增），应用 timeline_patch 补丁。
func (s *State) EventDef(p *loader.Package, id string) *timeline.Event {
	var e *timeline.Event
	if x, ok := p.Timeline[id]; ok {
		cp := *x
		e = &cp
	} else if s.Timeline != nil && s.Timeline.Added[id] != nil {
		cp := *s.Timeline.Added[id]
		e = &cp
	}
	if e == nil {
		return nil
	}
	if s.Timeline != nil {
		if r := s.Timeline.Events[id]; r != nil {
			for path, raw := range r.Patches {
				applyEventPatch(e, path, raw)
			}
		}
	}
	return e
}

func applyEventPatch(e *timeline.Event, path string, raw json.RawMessage) {
	var str string
	_ = json.Unmarshal(raw, &str)
	switch path {
	case "window.start":
		e.Window.Start = str
	case "window.end":
		e.Window.End = str
	case "location":
		e.Location = str
	case "summary":
		e.Summary = str
	case "title":
		e.Title = str
	}
}

// EventIDs 返回全部世界事件（故事包 + 新增），按开始时间排序。
func (s *State) EventIDs(p *loader.Package) []string {
	all := map[string]*timeline.Event{}
	for id, e := range p.Timeline {
		all[id] = e
	}
	if s.Timeline != nil {
		for id, e := range s.Timeline.Added {
			all[id] = e
		}
	}
	return timeline.SortedIDs(all)
}

// Retired 报告实体是否已退场（死亡 / 毁灭 / 遗失）。
func (s *State) Retired(id string) bool { return s.World.IsRetired(id) }

// KnowledgeVars 返回 CEL 的 known / gone 变量。
func (s *State) KnowledgeVars() (map[string]any, map[string]any) {
	known := map[string]any{}
	if s.Knowledge != nil {
		for ent, fs := range s.Knowledge.Facts {
			m := map[string]any{}
			for f, fact := range fs {
				m[f] = int64(fact.Level)
			}
			known[ent] = m
		}
	}
	gone := map[string]any{}
	if s.World != nil {
		for id := range s.World.Retired {
			gone[id] = true
		}
	}
	return known, gone
}

// applyWorld 处理 v0.2.0 的开放世界事件。返回 handled=false 表示不是本组事件。
func applyWorld(s *State, e event.Event) (bool, error) {
	d := e.Data
	switch e.Type {
	case event.WorldChangeApplied:
		if d.Change == nil {
			return true, fmt.Errorf("WorldChangeApplied without change")
		}
		if err := applyChange(s, *d.Change, e.Turn); err != nil {
			return true, err
		}
		s.W().Log = append(s.W().Log, overlay.LogEntry{Change: *d.Change, Turn: e.Turn, Minute: e.Minute})
		s.W().Seq++
	case event.WorldChangeReverted:
		if d.Change == nil {
			return true, fmt.Errorf("WorldChangeReverted without change")
		}
		if err := applyChange(s, *d.Change, e.Turn); err != nil {
			return true, err
		}
		w := s.W()
		if orig := w.Find(d.Key); orig != nil {
			orig.RevertedBy = d.Change.ID
		}
		w.Log = append(w.Log, overlay.LogEntry{Change: *d.Change, Turn: e.Turn, Minute: e.Minute, Reverts: d.Key})
		w.Seq++
	case event.ImpactAssessed, event.WorldUpdateFailed, event.CombatIntentParsed, event.ActionAdjudicated:
		// 记录事件：不改变结构化状态（日志 / 审查使用）。
	case event.KnowledgeRevealed, event.RumorHeard:
		if d.Reveal == nil {
			return true, fmt.Errorf("%s without reveal", e.Type)
		}
		s.K().Apply(*d.Reveal, e.Turn)
		if e.Type == event.RumorHeard && d.Story != "" {
			r := s.T().Get(d.Story, nil)
			r.Rumors = append(r.Rumors, d.Delta)
		}
	case event.WorldEventScheduled:
		if d.WorldEvent == nil {
			return true, fmt.Errorf("WorldEventScheduled without event")
		}
		ev := *d.WorldEvent
		t := s.T()
		t.Added[ev.ID] = &ev
		t.DayAdds[int(e.Minute/1440)+1]++
	case event.WorldEventStarted:
		r := s.T().Get(d.Story, nil)
		r.Status, r.Started, r.Stage = timeline.Active, e.Minute, d.Step
		for k, v := range d.Values {
			if _, ok := r.Vars[k]; !ok {
				r.Vars[k] = v
			}
		}
	case event.WorldEventStage:
		s.T().Get(d.Story, nil).Stage = d.Step
	case event.WorldEventJoined, event.WorldEventDisrupted:
		r := s.T().Get(d.Story, nil)
		for k, v := range d.Values {
			r.Vars[k] += v
		}
		if d.Key != "" {
			r.Joined = append(r.Joined, d.Key)
		}
	case event.WorldEventResolved:
		r := s.T().Get(d.Story, nil)
		r.Status, r.Outcome, r.By, r.Ended = timeline.Resolved, d.Outcome, d.Source, e.Minute
	case event.WorldEventCancelled:
		r := s.T().Get(d.Story, nil)
		r.Status, r.Reason, r.Ended = timeline.Cancelled, d.Reason, e.Minute
	case event.DecisionRequested:
		if d.Decision == nil {
			return true, fmt.Errorf("DecisionRequested without decision")
		}
		dc := *d.Decision
		if dc.Notify {
			s.Notice = &dc
		} else {
			s.Pending = &dc
		}
	case event.DecisionResolved:
		if s.Pending != nil && (d.Key == "" || s.Pending.ID == d.Key) {
			s.Pending = nil
		}
		if s.Notice != nil && (d.Key == "" || s.Notice.ID == d.Key) {
			s.Notice = nil
		}
	case event.PartHit:
		if s.RPG != nil && s.RPG.Combat != nil {
			if u := s.RPG.Combat.Unit(d.Target); u != nil {
				if u.PartDamage == nil {
					u.PartDamage = map[string]int{}
				}
				u.PartDamage[d.Key] += d.Total
				if d.Remove && !strings.Contains(","+strings.Join(u.Broken, ",")+",", ","+d.Key+",") {
					u.Broken = append(u.Broken, d.Key)
				}
			}
		}
	case event.BranchCreated:
		s.Branch = d.Key
		s.Seed ^= d.Seed
	case event.CharacterCreated:
		pr := &Profile{Preset: d.Profile["preset"], Background: d.Profile["background"], Appearance: d.Profile["appearance"],
			Personality: d.Profile["personality"], Story: d.Profile["story"], Custom: d.Profile["custom"] == "true"}
		s.Player.Profile = pr
		if n := d.Profile["name"]; n != "" {
			s.Player.Name = n
		}
	default:
		return false, nil
	}
	return true, nil
}

// applyChange 应用一项世界修改。玩家机械状态与关系维度由引擎同时发出的原生事件改变（这里只记日志），
// NPC 位置与人物死亡同步到 NPC 状态，时间线操作写入时间线运行时，其余写入覆盖层。
func applyChange(s *State, c change.Change, turn int) error {
	if change.IsDeltaPath(c.Target, c.Path) {
		return nil
	}
	switch c.Op {
	case change.OpTimelineAdd:
		var ev timeline.Event
		if err := json.Unmarshal(c.Value, &ev); err != nil {
			return fmt.Errorf("timeline_add: %w", err)
		}
		ev.ID = c.Target
		t := s.T()
		t.Added[ev.ID] = &ev
		return nil
	case change.OpTimelineRemove:
		delete(s.T().Added, c.Target)
		delete(s.T().Events, c.Target)
		return nil
	case change.OpTimelineCancel:
		r := s.T().Get(c.Target, nil)
		r.Status, r.Reason = timeline.Cancelled, c.Reason
		return nil
	case change.OpTimelineUncancel:
		var prev string
		_ = json.Unmarshal(c.Value, &prev)
		if prev == "" {
			prev = timeline.Scheduled
		}
		r := s.T().Get(c.Target, nil)
		r.Status, r.Reason = prev, ""
		return nil
	case change.OpTimelinePatch:
		r := s.T().Get(c.Target, nil)
		switch {
		case c.Path == "outcome":
			var o string
			_ = json.Unmarshal(c.Value, &o)
			r.Forced = o
		case strings.HasPrefix(c.Path, "vars."):
			var v int
			if err := json.Unmarshal(c.Value, &v); err != nil {
				return fmt.Errorf("timeline_patch vars: %w", err)
			}
			r.Vars[strings.TrimPrefix(c.Path, "vars.")] = v
		default:
			if r.Patches == nil {
				r.Patches = map[string]json.RawMessage{}
			}
			if string(c.Value) == "null" || len(c.Value) == 0 {
				delete(r.Patches, c.Path)
			} else {
				r.Patches[c.Path] = c.Value
			}
		}
		return nil
	}
	if c.Op == change.OpPatch && (c.Path == "location" || c.Path == "fields.location") {
		if n, ok := s.NPCs[c.Target]; ok {
			var loc string
			_ = json.Unmarshal(c.Value, &loc)
			n.Location = loc
			s.W().Rev[c.Target]++
			return nil
		}
	}
	w := s.W()
	switch c.Op {
	case change.OpRetire:
		if n, ok := s.NPCs[c.Target]; ok {
			var r overlay.Retirement
			_ = json.Unmarshal(c.Value, &r)
			if r.Location == "" {
				r.Location = n.Location
			}
			b, _ := json.Marshal(r)
			c.Value = b
			n.Location = ""
			if r.Reason == "death" {
				rp := s.R()
				card := rp.Cards[c.Target]
				if card == nil {
					card = &Card{Created: turn}
					rp.Cards[c.Target] = card
				}
				card.Status, card.Changed, card.Reason = CardDead, turn, r.Text
			}
		}
	case change.OpRestore:
		if n, ok := s.NPCs[c.Target]; ok {
			if r, was := w.Retired[c.Target]; was {
				n.Location = r.Location
			}
			if card := s.R().Cards[c.Target]; card != nil && card.Status == CardDead {
				card.Status, card.Changed = CardActive, turn
			}
		}
	}
	return w.Apply(c)
}

// Rev 返回实体的修订号（只读，不初始化覆盖层）。
func (s *State) Rev(id string) int {
	if s.World == nil {
		return 0
	}
	return s.World.Rev[id]
}

// WorldSeq 返回已写入的世界变更数（只读）。
func (s *State) WorldSeq() int {
	if s.World == nil {
		return 0
	}
	return s.World.Seq
}
