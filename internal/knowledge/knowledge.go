// Package knowledge 是玩家知识层（架构 V0.3 第 8 节）：一切从“未知”开始，按实体的字段逐项解锁。
//
// 这是一个叶子包：只定义数据结构、字段目录与纯函数。揭示的渠道校验需要游戏状态，由 internal/knowledge/reveal 完成。
package knowledge

import (
	"encoding/json"
	"slices"
	"sort"
	"strings"
)

// Level 是玩家对一个字段的认知程度。
type Level int

// 认知程度。
const (
	Unknown Level = 0
	Rumored Level = 1
	Known   Level = 2
)

// String 返回中文名。
func (l Level) String() string {
	switch l {
	case Rumored:
		return "传闻"
	case Known:
		return "已知"
	}
	return "未知"
}

// 揭示渠道。
const (
	ChannelWitness   = "witness"
	ChannelDialogue  = "dialogue"
	ChannelDocument  = "document"
	ChannelInference = "inference"
	ChannelRumor     = "rumor"
	ChannelSystem    = "system"
)

// Channels 是合法的渠道。
var Channels = []string{ChannelWitness, ChannelDialogue, ChannelDocument, ChannelInference, ChannelRumor, ChannelSystem}

// Fact 是玩家对一个字段的认知。
type Fact struct {
	Level   Level           `json:"l"`
	Turn    int             `json:"t"`
	Channel string          `json:"c"`
	Source  string          `json:"s,omitempty"`
	Rev     int             `json:"r"`
	Claim   string          `json:"claim,omitempty"`
	Value   json.RawMessage `json:"v,omitempty"`
}

// Store 是玩家知识：实体 ID → 字段 → 认知。
type Store struct {
	Facts map[string]map[string]*Fact `json:"facts"`
}

// New 返回空知识库。
func New() *Store { return &Store{Facts: map[string]map[string]*Fact{}} }

// Get 返回字段认知（没有时为 nil）。
func (s *Store) Get(entity, field string) *Fact {
	if s == nil || s.Facts == nil {
		return nil
	}
	return s.Facts[entity][field]
}

// LevelOf 返回字段的认知程度。
func (s *Store) LevelOf(entity, field string) Level {
	if f := s.Get(entity, field); f != nil {
		return f.Level
	}
	return Unknown
}

// Knows 报告字段是否已知（传闻不算）。
func (s *Store) Knows(entity, field string) bool { return s.LevelOf(entity, field) >= Known }

// Aware 报告玩家是否知道这个实体存在（含传闻）。
func (s *Store) Aware(entity string) bool {
	if s == nil || s.Facts == nil {
		return false
	}
	return len(s.Facts[entity]) > 0
}

// Fields 返回实体的已知 / 传闻字段（排序）。
func (s *Store) Fields(entity string) []string {
	if s == nil || s.Facts == nil {
		return nil
	}
	out := make([]string, 0, len(s.Facts[entity]))
	for f := range s.Facts[entity] {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// Entities 返回玩家知道（或听说过）的全部实体 ID（排序）。
func (s *Store) Entities() []string {
	if s == nil || s.Facts == nil {
		return nil
	}
	out := make([]string, 0, len(s.Facts))
	for id := range s.Facts {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Reveal 是一次揭示提案 / 结果（KnowledgeRevealed 事件的载荷）。
type Reveal struct {
	Entity   string          `json:"entity"`
	Fields   []string        `json:"fields"`
	Level    Level           `json:"level,omitempty"` // 0 视为 Known
	Channel  string          `json:"channel"`
	Source   string          `json:"source,omitempty"`
	Claim    string          `json:"claim,omitempty"`
	Evidence string          `json:"evidence,omitempty"`
	Rev      int             `json:"rev,omitempty"`
	Values   json.RawMessage `json:"values,omitempty"` // 字段 → 快照值（关系维度、生死等）
}

// EffectiveLevel 返回揭示的级别（缺省为已知）。
func (r Reveal) EffectiveLevel() Level {
	if r.Level == Unknown {
		return Known
	}
	return r.Level
}

// Apply 把一次揭示写入知识库。单调：已知不会降级为传闻；传闻可以升级为已知。
func (s *Store) Apply(r Reveal, turn int) {
	if s.Facts == nil {
		s.Facts = map[string]map[string]*Fact{}
	}
	m := s.Facts[r.Entity]
	if m == nil {
		m = map[string]*Fact{}
		s.Facts[r.Entity] = m
	}
	var vals map[string]json.RawMessage
	if len(r.Values) > 0 {
		_ = json.Unmarshal(r.Values, &vals)
	}
	lvl := r.EffectiveLevel()
	for _, f := range r.Fields {
		old := m[f]
		if old != nil && old.Level > lvl {
			continue
		}
		nf := &Fact{Level: lvl, Turn: turn, Channel: r.Channel, Source: r.Source, Rev: r.Rev, Claim: r.Claim}
		if v, ok := vals[f]; ok {
			nf.Value = v
		}
		if old != nil && old.Level == lvl && lvl == Known && nf.Value == nil {
			// 再次得知（例如重新见到）：刷新修订号与回合，保留快照
			nf.Value = old.Value
		}
		m[f] = nf
	}
	// 知道任何字段都意味着知道它存在
	if _, ok := m["existence"]; !ok {
		m["existence"] = &Fact{Level: lvl, Turn: turn, Channel: r.Channel, Source: r.Source, Rev: r.Rev}
	} else if m["existence"].Level < lvl {
		m["existence"].Level = lvl
	}
}

// Stale 报告已知字段是否可能已过时（实体修订号大于揭示时的修订号）。
func (s *Store) Stale(entity, field string, rev int) bool {
	f := s.Get(entity, field)
	return f != nil && rev > f.Rev
}

// ---------- 字段目录（第 8.2 节）----------

// Catalog 是每种实体可揭示的字段。带冒号前缀的字段（hidden:<id>、part:<id>、spec:*、slot:*）按前缀匹配。
var Catalog = map[string][]string{
	"character":   {"existence", "name", "appearance", "identity", "faction", "background", "personality", "goals", "abilities", "stats", "equipment", "mech", "location", "status", "hidden:"},
	"relation":    {"existence", "label", "dims", "history"},
	"location":    {"existence", "name", "description", "exits", "controller", "dangers", "hidden:"},
	"faction":     {"existence", "name", "leader", "goals", "members", "territory", "stance", "hidden:"},
	"item":        {"existence", "name", "description", "stats", "effects", "origin", "hidden:"},
	"weapon":      {"existence", "name", "description", "stats", "effects", "origin", "hidden:"},
	"mech":        {"existence", "name", "model", "spec:", "slot:", "lore", "pilot", "status", "hidden:"},
	"codex":       {"existence", "title", "summary", "details", "hidden:"},
	"lore":        {"existence", "title", "summary", "details", "hidden:"},
	"tech":        {"existence", "title", "summary", "details", "hidden:"},
	"history":     {"existence", "title", "summary", "details", "hidden:"},
	"world_event": {"existence", "time", "location", "participants", "outcome"},
	"enemy":       {"existence", "name", "stats", "part:", "skills"},
	"skill":       {"existence", "name", "description"},
}

// ValidField 报告 field 是否属于 kind 的字段目录。
func ValidField(kind, field string) bool {
	fs, ok := Catalog[kind]
	if !ok {
		fs = Catalog["codex"]
	}
	for _, f := range fs {
		if strings.HasSuffix(f, ":") {
			if strings.HasPrefix(field, f) && len(field) > len(f) {
				return true
			}
			continue
		}
		if f == field {
			return true
		}
	}
	return false
}

// DisplayFields 返回 kind 在 UI 中逐项展示的字段（不含 existence 与带前缀的动态字段）。
func DisplayFields(kind string) []string {
	var out []string
	for _, f := range Catalog[kind] {
		if f == "existence" || strings.HasSuffix(f, ":") {
			continue
		}
		out = append(out, f)
	}
	return out
}

// FieldLabel 返回字段的中文名。
func FieldLabel(field string) string {
	if strings.HasPrefix(field, "hidden:") {
		return "秘密"
	}
	if strings.HasPrefix(field, "part:") {
		return "部位"
	}
	if l, ok := fieldLabels[field]; ok {
		return l
	}
	return field
}

var fieldLabels = map[string]string{
	"existence": "存在", "name": "名字", "appearance": "外貌", "identity": "身份", "faction": "所属势力",
	"background": "背景", "personality": "性格", "goals": "目标", "abilities": "能力", "stats": "数值",
	"equipment": "装备", "mech": "机甲", "location": "所在地", "status": "生死", "label": "关系",
	"dims": "关系数值", "history": "往事", "description": "描述", "exits": "出口", "controller": "控制者",
	"dangers": "危险", "leader": "首领", "members": "成员", "territory": "领地", "stance": "态度",
	"effects": "效果", "origin": "来历", "model": "型号", "lore": "传说", "pilot": "驾驶者",
	"title": "标题", "summary": "概要", "details": "详情", "time": "时间", "participants": "参与者",
	"outcome": "结果", "skills": "技能",
}

// IsHidden 报告字段是否是隐藏真相字段。
func IsHidden(field string) bool { return strings.HasPrefix(field, "hidden:") }

// SortFields 按字段目录顺序排序（未知字段按字母序排在后面）。
func SortFields(kind string, fields []string) []string {
	order := map[string]int{}
	for i, f := range Catalog[kind] {
		order[f] = i
	}
	out := slices.Clone(fields)
	slices.SortStableFunc(out, func(a, b string) int {
		ia, oka := order[a]
		ib, okb := order[b]
		switch {
		case oka && okb:
			return ia - ib
		case oka:
			return -1
		case okb:
			return 1
		}
		return strings.Compare(a, b)
	})
	return out
}
