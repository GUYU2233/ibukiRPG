package resolver

import (
	"context"
	"sort"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// 解析结果种类。
const (
	KindAction   = "action"
	KindMove     = "move"
	KindFreeform = "freeform"
	KindReject   = "reject"
	KindClarify  = "clarify"
)

// 合理性（第 8 节）：只判断“能不能尝试”。
const (
	Allow                = "ALLOW"
	AllowWithCheck       = "ALLOW_WITH_CHECK"
	AllowWithConsequence = "ALLOW_WITH_CONSEQUENCE"
	Reject               = "REJECT"
)

// Option 是澄清时给玩家的候选（UI 显示为按钮）。
type Option struct {
	Label   string          `json:"label"`
	Command command.Command `json:"command"`
}

// Resolution 是 Action Resolver 的输出：Intent + Reasonability + Action Matching（第 7 节）。
type Resolution struct {
	Kind          string            `json:"kind"`
	Intent        string            `json:"intent,omitempty"`
	Action        string            `json:"matched_action,omitempty"`
	Target        string            `json:"target,omitempty"`
	Item          string            `json:"item,omitempty"`
	Destination   string            `json:"destination,omitempty"`
	Freeform      *command.Freeform `json:"freeform,omitempty"`
	Reasonability string            `json:"reasonability"`
	Reason        string            `json:"reason,omitempty"`
	Options       []Option          `json:"options,omitempty"`
	Confidence    int               `json:"confidence"` // 千分比
	Source        string            `json:"source"`
}

// Input 是解析输入：玩家文本 + PlayerScope 可见的世界（不含 NPC 秘密）。
type Input struct {
	Text  string
	Pkg   *loader.Package
	State *state.State
}

// Resolver 是 Action Resolver 接口。
type Resolver interface {
	Name() string
	Resolve(ctx context.Context, in Input) (Resolution, error)
}

// Command 把可执行的解析结果转换成 Command（不含 ID）。
func (r Resolution) Command(input, source string) (command.Command, bool) {
	c := command.Command{Input: input, Source: source}
	switch r.Kind {
	case KindAction:
		c.Kind, c.Action, c.Target, c.Item = command.KindAction, r.Action, r.Target, r.Item
	case KindMove:
		c.Kind, c.Destination = command.KindMove, r.Destination
	case KindFreeform:
		if r.Freeform == nil {
			return c, false
		}
		c.Kind, c.Freeform = command.KindFreeform, r.Freeform
	default:
		return c, false
	}
	return c, true
}

// ---------- 共享的实体匹配工具 ----------

type aliasHit struct {
	id  string
	pos int
	n   int
}

// findAliases 在文本中查找别名，按首次出现位置排序（同位置取较长别名）。
func findAliases(text string, ids []string, aliases func(id string) []string) []aliasHit {
	var hits []aliasHit
	for _, id := range ids {
		best := aliasHit{id: id, pos: -1}
		for _, a := range aliases(id) {
			if a == "" {
				continue
			}
			if i := strings.Index(text, a); i >= 0 && (best.pos < 0 || i < best.pos || (i == best.pos && len(a) > best.n)) {
				best = aliasHit{id: id, pos: i, n: len(a)}
			}
		}
		if best.pos >= 0 {
			hits = append(hits, best)
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].pos != hits[j].pos {
			return hits[i].pos < hits[j].pos
		}
		return hits[i].n > hits[j].n
	})
	return hits
}

func npcAliases(p *loader.Package) func(string) []string {
	return func(id string) []string {
		c := p.Characters[id]
		return append([]string{c.Name()}, c.Aliases...)
	}
}

func itemAliases(p *loader.Package) func(string) []string {
	return func(id string) []string {
		it := p.Items[id]
		return append([]string{it.Name}, it.Aliases...)
	}
}

func locationAliases(p *loader.Package) func(string) []string {
	return func(id string) []string {
		l := p.Locations[id]
		return append([]string{l.Name}, l.Aliases...)
	}
}

// LookupID 把 AI 可能返回的 ID 或名称映射为内容包 ID。
func LookupID(p *loader.Package, kind, v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	var ids []string
	var al func(string) []string
	switch kind {
	case "npc":
		ids, al = p.NPCIDs, npcAliases(p)
	case "item":
		ids, al = p.ItemIDs, itemAliases(p)
	case "location":
		ids, al = p.LocationIDs, locationAliases(p)
	case "action":
		for _, id := range p.ActionIDs {
			if id == v || p.Actions[id].Name == v || strings.TrimPrefix(id, "demo:action/") == v {
				return id
			}
		}
		return ""
	}
	for _, id := range ids {
		if id == v {
			return id
		}
		for _, a := range al(id) {
			if a == v {
				return id
			}
		}
	}
	// 容忍 "lena" 这种短 ID
	for _, id := range ids {
		if strings.HasSuffix(id, "/"+v) {
			return id
		}
	}
	return ""
}
