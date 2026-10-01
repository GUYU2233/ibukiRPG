package validate

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/overlay"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
)

// RevealResult 是揭示校验的结果。
type RevealResult struct {
	Reveals []knowledge.Reveal
	Rejects []Reject
	// FreeHidden 是本回合没有 reveal_when 而被揭示的隐藏真相数（每条计入 story_impact +15，第 19 节决定 1）。
	FreeHidden int
	// Hidden 是本回合被揭示的隐藏真相（实体#真相 ID），Guard 用来放行对应的泄露关键词。
	Hidden []string
}

// Present 返回玩家所在地点的人物（故事包 NPC + 新建人物）。
func Present(e *Env) []string {
	s, p := e.State, e.Pkg
	var out []string
	for _, id := range s.NPCsAt(p, s.Player.Location) {
		if !s.Retired(id) {
			out = append(out, id)
		}
	}
	var gen []string
	if s.World != nil {
		for id, d := range s.World.Created {
			if d.Kind == "character" && !s.Retired(id) && s.World.ApplyDoc(d).Field("location") == s.Player.Location {
				gen = append(gen, id)
			}
		}
	}
	slices.Sort(gen)
	return append(out, gen...)
}

func (e *Env) inCombat(id string) bool {
	c := e.State.RPG
	if c == nil || c.Combat == nil {
		return false
	}
	for _, u := range c.Combat.Units {
		if u.Ref == id || u.ID == id {
			return true
		}
	}
	return false
}

// visible 报告实体对玩家当前是否“在场 / 可见”（witness 渠道）。
func (e *Env) visible(id string, present []string) bool {
	s := e.State
	if id == s.Player.Location || id == loader.PlayerID || slices.Contains(present, id) || e.inCombat(id) {
		return true
	}
	if a, b, ok := strings.Cut(id, ">"); ok {
		return (slices.Contains(present, a) || a == loader.PlayerID) && (slices.Contains(present, b) || b == loader.PlayerID)
	}
	if s.Player.Inventory[id] > 0 {
		return true
	}
	if ev := s.EventDef(e.Pkg, id); ev != nil {
		st := s.Timeline.Status(id)
		return ev.Location == s.Player.Location && (st == timeline.Active || st == timeline.Resolved)
	}
	if d := e.doc(id); d != nil && d.Kind == "character" {
		return d.Field("location") == s.Player.Location
	}
	return false
}

// speakerKnows 报告人物 src 是否知道 entity 的 field（对话渠道的“说话者自己知道”，第 8.4 节）。
func (e *Env) speakerKnows(src, entity, field string) bool {
	if src == entity {
		return true
	}
	if slices.Contains(e.Pkg.KnownBy(entity), src) {
		return true
	}
	if a, b, ok := strings.Cut(entity, ">"); ok && (a == src || b == src) {
		return true
	}
	sd, ed := e.doc(src), e.doc(entity)
	if sd == nil || ed == nil {
		return false
	}
	srcFaction := sd.Field("faction")
	switch ed.Kind {
	case "location":
		return !knowledge.IsHidden(field) // 本地人知道地方的公开情况
	case "faction":
		return entity == srcFaction || slices.Contains([]string{"name", "leader", "goals", "territory"}, field)
	case "character":
		if knowledge.IsHidden(field) {
			return false
		}
		if srcFaction != "" && ed.Field("faction") == srcFaction {
			return true
		}
		return slices.Contains([]string{"name", "identity", "faction", "location", "status"}, field)
	case "world_event":
		return true
	case "item", "weapon":
		return !knowledge.IsHidden(field)
	}
	return false
}

// Reveals 校验一组揭示提案。
func Reveals(e *Env, in []knowledge.Reveal) RevealResult {
	var res RevealResult
	s := e.State
	present := Present(e)
	limit := MaxReveals(e.Tier)
	count, inferences := 0, 0
	for _, r := range in {
		bad := func(reason string) {
			res.Rejects = append(res.Rejects, rej("reveal", r.Entity, strings.Join(r.Fields, ","), r.Channel, reason, r))
		}
		kind := e.kindOf(r.Entity)
		if kind == "" && r.Entity != loader.PlayerID {
			bad("实体不存在")
			continue
		}
		if !slices.Contains(knowledge.Channels, r.Channel) || r.Channel == knowledge.ChannelSystem {
			bad("渠道不合法：" + r.Channel)
			continue
		}
		if r.Channel == knowledge.ChannelInference || r.Channel == knowledge.ChannelRumor {
			r.Level = knowledge.Rumored
		}
		if r.Level == knowledge.Unknown {
			r.Level = knowledge.Known
		}
		var doc *overlay.EntityDoc
		if kind != "relation" && kind != "world_event" {
			doc = e.doc(r.Entity)
		}
		var fields []string
		for _, f := range r.Fields {
			if f == "existence" && !s.Knowledge.Aware(r.Entity) {
				fields = append(fields, f)
				continue
			}
			if !knowledge.ValidField(kind, f) {
				bad("字段不在目录中：" + f)
				continue
			}
			if s.Knowledge.LevelOf(r.Entity, f) >= r.Level {
				continue // 已经知道，不算错误
			}
			var reason string
			switch r.Channel {
			case knowledge.ChannelWitness:
				if !e.visible(r.Entity, present) {
					reason = "目击渠道：实体不在场"
				}
			case knowledge.ChannelDialogue:
				switch {
				case !slices.Contains(present, r.Source):
					reason = "对话渠道：说话者不在场"
				case !e.speakerKnows(r.Source, r.Entity, f):
					reason = "对话渠道：说话者自己并不知道这件事"
				}
			case knowledge.ChannelDocument:
				d := e.doc(r.Source)
				if d == nil || (s.Player.Inventory[r.Source] == 0 && !e.visible(r.Source, present)) {
					reason = "文件渠道：文件不在手边"
				}
			case knowledge.ChannelInference:
				if knowledge.IsHidden(f) {
					reason = "推断不能揭示隐藏真相"
				} else if inferences >= 1 {
					reason = "推断每回合只能揭示 1 项"
				}
			}
			if reason == "" && knowledge.IsHidden(f) {
				h := (*overlay.Hidden)(nil)
				if doc != nil {
					h = doc.HiddenByID(strings.TrimPrefix(f, "hidden:"))
				}
				switch {
				case h == nil:
					reason = "没有这条隐藏真相"
				case h.RevealWhen != "":
					ok := false
					if e.Eval != nil {
						ok, _ = e.Eval(h.RevealWhen)
					}
					if !ok {
						reason = "隐藏真相的揭示前提尚未满足"
					}
				default:
					res.FreeHidden++
				}
				if reason == "" {
					res.Hidden = append(res.Hidden, r.Entity+"#"+h.ID)
				}
			}
			if reason == "" && count >= limit {
				reason = fmt.Sprintf("超过单回合揭示上限（%d 项）", limit)
			}
			if reason != "" {
				bad(f + "：" + reason)
				continue
			}
			if r.Channel == knowledge.ChannelInference {
				inferences++
			}
			count++
			fields = append(fields, f)
		}
		if len(fields) == 0 {
			continue
		}
		r.Fields = fields
		r.Rev = s.Rev(r.Entity)
		r.Values = snapshot(e, r.Entity, fields)
		res.Reveals = append(res.Reveals, r)
	}
	return res
}

// snapshot 记录数值类字段的快照（关系维度、生死），UI 显示快照值而不是当前真值。
func snapshot(e *Env, id string, fields []string) json.RawMessage {
	vals := map[string]any{}
	for _, f := range fields {
		switch f {
		case "dims":
			if a, b, ok := strings.Cut(id, ">"); ok {
				vals["dims"] = e.State.Edge(a, b)
			}
		case "status":
			if e.State.Retired(id) {
				vals["status"] = "dead"
			} else {
				vals["status"] = "alive"
			}
		}
	}
	if len(vals) == 0 {
		return nil
	}
	b, _ := json.Marshal(vals)
	return b
}

// Option 是可揭示集合中的一项（[REVEALABLE] 段）。
type Option struct {
	Entity  string `json:"entity"`
	Name    string `json:"name"`
	Field   string `json:"field"`
	Channel string `json:"channel"`
	Source  string `json:"source,omitempty"`
	// Text 是该字段的真值（仅给叙事 AI；隐藏真相给原文）。
	Text string `json:"text,omitempty"`
}

// Revealable 预先计算本回合“渠道已满足”的字段（第 8.5 节），最多 max 项。
func Revealable(e *Env, maxN int) []Option {
	s := e.State
	present := Present(e)
	var out []Option
	add := func(o Option) bool {
		if len(out) >= maxN {
			return false
		}
		if s.Knowledge.LevelOf(o.Entity, o.Field) >= knowledge.Known {
			return true
		}
		out = append(out, o)
		return true
	}
	fieldsOf := func(d *overlay.EntityDoc) []string {
		fs := append(append([]string{}, d.Public...), d.Discoverable...)
		for _, h := range d.Hidden {
			fs = append(fs, "hidden:"+h.ID)
		}
		return fs
	}
	hiddenOK := func(d *overlay.EntityDoc, f string) (string, bool) {
		h := d.HiddenByID(strings.TrimPrefix(f, "hidden:"))
		if h == nil {
			return "", false
		}
		if h.RevealWhen != "" {
			if e.Eval == nil {
				return "", false
			}
			if ok, _ := e.Eval(h.RevealWhen); !ok {
				return "", false
			}
		}
		return h.Text, true
	}
	for _, id := range present {
		d := e.doc(id)
		if d == nil {
			continue
		}
		for _, f := range fieldsOf(d) {
			text := d.Field(f)
			if knowledge.IsHidden(f) {
				t, ok := hiddenOK(d, f)
				if !ok {
					continue
				}
				text = t
			}
			ch := knowledge.ChannelDialogue
			if slices.Contains(d.Public, f) {
				ch = knowledge.ChannelWitness
			}
			if !add(Option{Entity: id, Name: d.Name(), Field: f, Channel: ch, Source: id, Text: clip(text, 120)}) {
				return out
			}
		}
	}
	// 在场人物知道的其他实体（known_by / 同势力）
	for _, id := range e.Pkg.DocIDs() {
		d := e.doc(id)
		if d == nil || d.Retired != nil || slices.Contains(present, id) || id == loader.PlayerID {
			continue
		}
		for _, src := range present {
			if !slices.Contains(e.Pkg.KnownBy(id), src) {
				continue
			}
			for _, f := range fieldsOf(d) {
				text := d.Field(f)
				if knowledge.IsHidden(f) {
					t, ok := hiddenOK(d, f)
					if !ok {
						continue
					}
					text = t
				}
				if !add(Option{Entity: id, Name: d.Name(), Field: f, Channel: knowledge.ChannelDialogue, Source: src, Text: clip(text, 120)}) {
					return out
				}
			}
			break
		}
	}
	return out
}
