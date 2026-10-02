package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/power"
)

// ---------- 写入工具（v0.2.0-rc1：游戏内函数调用） ----------
//
// 写入工具不直接改存档：它们把提案交给 Writer.Dry（与 WORLD 段、MCP 写入完全相同的校验器做一次演练），
// 通过的提案收集在 Writer 里，由编排器在回合提交时统一写入事件日志（可撤销）。只有 Scope.Write 为真
// 且 Env.Writer 不为空时才列出这些工具（游戏内的叙述者 / 卡片编辑器）；MCP 永远不会拿到它们
// （MCP 写入走自己的 preview → apply 两步）。

// DryResult 是一次演练的结果。
type DryResult struct {
	Accepted []change.Change `json:"-"`
	Summary  []string        `json:"accepted"`
	Rejected []string        `json:"rejected,omitempty"`
	Impact   int             `json:"impact"`
}

// Writer 收集写入工具的提案。并发安全。
type Writer struct {
	// Dry 用同一校验器演练一组变更（在已收集的提案之上），不提交。
	Dry func(cs []change.Change) (DryResult, error)
	// Namespace 是新建实体的命名空间；PowerRef 是强度预算的参考强度。
	Namespace string
	PowerRef  int
	// Max 是一次最多收集的变更数（0 = 6）；Source 写进变更的原因前缀（可空）。
	Max int

	mu       sync.Mutex
	proposed []change.Change
	calls    int
}

// Proposed 返回收集到的提案（副本）。
func (w *Writer) Proposed() []change.Change {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.proposed)
}

// Calls 返回写入工具被调用的次数。
func (w *Writer) Calls() int {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls
}

func (w *Writer) limit() int {
	if w.Max <= 0 {
		return 6
	}
	return w.Max
}

// submit 演练并收集提案。
func (w *Writer) submit(cs []change.Change) (any, error) {
	if w == nil || w.Dry == nil {
		return nil, errors.New("当前不能修改世界")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	room := w.limit() - len(w.proposed)
	if room <= 0 {
		return nil, fmt.Errorf("本回合最多提交 %d 项修改，已经用完", w.limit())
	}
	if len(cs) > room {
		cs = cs[:room]
	}
	res, err := w.Dry(append(slices.Clone(w.proposed), cs...))
	if err != nil {
		return nil, err
	}
	// Dry 返回的是全部已接受的变更（含之前收集的）；只保留新增部分的结果给模型
	prev := len(w.proposed)
	w.proposed = res.Accepted
	out := map[string]any{"accepted": res.Summary[min(prev, len(res.Summary)):], "impact": res.Impact}
	if len(res.Rejected) > 0 {
		out["rejected"] = res.Rejected
	}
	if len(res.Accepted) == prev {
		out["note"] = "没有修改被接受；请根据拒绝原因调整，或不再修改世界"
	}
	return out, nil
}

var slugClean = regexp.MustCompile(`[^a-z0-9_]+`)

// WriteTools 返回写入工具（只有 Scope.Write 时可用）。
func WriteTools() []Tool {
	changeItem := map[string]any{"type": "object", "properties": map[string]any{
		"op":     map[string]any{"type": "string", "enum": []string{"patch", "create", "retire", "restore", "link", "unlink", "timeline_add", "timeline_patch", "timeline_cancel"}},
		"target": strProp("实体 ID（新建用 <命名空间>:<种类>/<英文短名>）；关系用 A>B"),
		"path":   strProp("字段路径，例如 fields.description / location / combat.stats.atk / dims.trust"),
		"value":  map[string]any{"description": "新值（patch）或实体内容（create）"},
		"reason": strProp("原因（叙事里发生了什么）"),
	}, "required": []string{"op", "target", "reason"}}
	return []Tool{
		{Name: "world.propose_change", Write: true,
			Description: "提交世界设定的修改（与 WORLD 段的 changes 相同的格式）。修改会先经过引擎校验：不合规的被拒绝并告诉你原因，数值超出强度预算会被缩小。通过的修改在本回合结束时写入世界变更日志，玩家可以撤销。",
			Params:      obj(map[string]any{"changes": map[string]any{"type": "array", "items": changeItem, "maxItems": 6}}, "changes"),
			run: func(e *Env, sc Scope, a args) (any, error) {
				raw, _ := json.Marshal(a["changes"])
				var cs []change.Change
				if err := json.Unmarshal(raw, &cs); err != nil || len(cs) == 0 {
					return nil, errors.New("changes 必须是非空的变更数组")
				}
				return e.Writer.submit(cs)
			}},
		{Name: "entity.generate", Write: true,
			Description: "生成一张新卡片（人物 / 敌人 / 武器 / 护甲 / 物品 / 技能 / 机甲 / 势力 / 地点 / 设定条目）。数值会按“种类 × 稀有度 × 等级”的强度预算自动缩放；先用 rules.power_budget 查看预算。",
			Params: obj(map[string]any{
				"kind":        map[string]any{"type": "string", "enum": []string{"character", "enemy", "weapon", "armor", "item", "skill", "mech", "faction", "location", "lore"}},
				"slug":        strProp("英文短名（小写字母 / 数字 / 下划线）"),
				"name":        strProp("名字"),
				"description": strProp("一两句描述，必须符合世界观"),
				"rarity":      map[string]any{"type": "string", "enum": []string{"common", "uncommon", "rare", "epic", "legendary"}},
				"level":       map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
				"stats":       map[string]any{"type": "object", "description": "数值，例如 {\"atk\":12,\"def\":4}"},
				"fields":      map[string]any{"type": "object", "description": "其它文字字段（appearance / personality / origin ……）"},
				"reason":      strProp("为什么要生成（玩家要求 / 叙事里出现）"),
			}, "kind", "name", "description", "reason"),
			run: func(e *Env, sc Scope, a args) (any, error) {
				if e.Writer == nil {
					return nil, errors.New("当前不能修改世界")
				}
				c, info := generateChange(e.Writer, a)
				res, err := e.Writer.submit([]change.Change{c})
				if err != nil {
					return nil, err
				}
				m, _ := res.(map[string]any)
				m["id"], m["power"] = c.Target, info
				return m, nil
			}},
		{Name: "rules.power_budget", Write: true,
			Description: "查询强度预算：某种类 / 稀有度 / 等级的卡片数值上限（强度分 = Σ 数值 × 权重）。",
			Params: obj(map[string]any{"kind": map[string]any{"type": "string", "enum": power.Kinds()}, "rarity": strProp("common / uncommon / rare / epic / legendary"),
				"level": map[string]any{"type": "integer", "minimum": 1, "maximum": 20}}, "kind"),
			run: func(e *Env, sc Scope, a args) (any, error) {
				ref := 0
				if e.Writer != nil {
					ref = e.Writer.PowerRef
				}
				p := power.Params{Kind: a.str("kind"), Rarity: a.str("rarity"), Level: a.num("level", 1, 1, 20)}.Norm()
				return map[string]any{"budget": power.Budget(ref, p), "describe": power.Describe(ref, p), "table": power.Table(ref)}, nil
			}},
	}
}

// generateChange 把 entity.generate 的参数转成 create 变更（数值按预算缩放）。
func generateChange(w *Writer, a args) (change.Change, string) {
	kind := a.str("kind")
	entKind, subtype := kind, ""
	switch kind {
	case "weapon", "armor":
		entKind, subtype = "item", kind
	case "enemy":
		entKind, subtype = "character", "enemy"
	case "skill":
		entKind, subtype = "lore", "skill"
	}
	slug := slugClean.ReplaceAllString(strings.ToLower(a.str("slug")), "_")
	slug = strings.Trim(slug, "_")
	if slug == "" {
		slug = fmt.Sprintf("card_%d", w.Calls()+1)
	}
	rarity := a.str("rarity")
	if rarity == "" {
		rarity = "common"
	}
	level := a.num("level", 1, 1, 20)
	stats := map[string]int{}
	if m, ok := a["stats"].(map[string]any); ok {
		for k, v := range m {
			if f, ok := v.(float64); ok && k != "level" {
				stats[k] = int(f)
			}
		}
	}
	info := ""
	if power.Has(kind) && len(stats) > 0 {
		p := power.Params{Kind: kind, Rarity: rarity, Level: level}.Norm()
		budget := power.Budget(w.PowerRef, p)
		fitted, _ := power.Fit(stats, budget)
		stats = fitted
		info = fmt.Sprintf("%s；强度分 %d", power.Describe(w.PowerRef, p), power.Score(stats))
	}
	stats["level"] = level
	fields := map[string]any{"name": a.str("name"), "description": a.str("description")}
	if m, ok := a["fields"].(map[string]any); ok {
		for k, v := range m {
			if _, dup := fields[k]; !dup {
				fields[k] = v
			}
		}
	}
	tags := []string{rarity}
	if subtype != "" {
		tags = append([]string{subtype}, tags...)
	}
	value := map[string]any{"importance": 2, "fields": fields, "public": []string{"name", "description"}, "tags": tags, "stats": stats}
	v, _ := json.Marshal(value)
	return change.Change{Op: change.OpCreate, Target: w.Namespace + ":" + entKind + "/" + slug, Kind: entKind, Value: v, Reason: a.str("reason")}, info
}
