// Package overlay 是生效世界的覆盖层（架构 V0.3 第 5.1 节）：生效世界 = 故事包静态设定 + 已接受的 WorldChange。
//
// 叶子包：只保存补丁、新建实体、退场记录与变更日志，并实现非玩家路径的 WorldChange 应用。
// 玩家机械状态（金钱 / 经验 / 物品 / 生命）与 NPC 位置由 state 包应用。
package overlay

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// Hidden 是一条隐藏真相（仅 Director 范围可见）。
type Hidden struct {
	ID          string   `json:"id" yaml:"id"`
	Text        string   `json:"text" yaml:"text"`
	RevealWhen  string   `json:"reveal_when,omitempty" yaml:"reveal_when"`
	LeakMarkers []string `json:"leak_markers,omitempty" yaml:"leak_markers"`
}

// EntityDoc 是一个实体的通用文档（第 4.3 节的信封）。故事包实体由加载器转换，新建实体整份保存在覆盖层。
type EntityDoc struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Importance   int               `json:"importance,omitempty"`
	Canon        string            `json:"canon,omitempty"`
	Locked       []string          `json:"locked,omitempty"`
	Fields       map[string]any    `json:"fields"`
	Public       []string          `json:"public,omitempty"`
	Discoverable []string          `json:"discoverable,omitempty"`
	Hidden       []Hidden          `json:"hidden,omitempty"`
	AliasUnknown string            `json:"alias_unknown,omitempty"`
	Stats        map[string]int    `json:"stats,omitempty"`
	Tier         string            `json:"tier,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Retired      *Retirement       `json:"retired,omitempty"`
	Rev          int               `json:"rev,omitempty"`
	Changed      []string          `json:"changed,omitempty"` // 被覆盖层修改过的路径
	Meta         map[string]string `json:"meta,omitempty"`
}

// Name 返回实体名（fields.name 或 fields.title）。
func (d *EntityDoc) Name() string {
	if d == nil {
		return ""
	}
	for _, k := range []string{"name", "title"} {
		if v, ok := d.Fields[k].(string); ok && v != "" {
			return v
		}
	}
	return d.ID
}

// Field 返回字段的文本形式。
func (d *EntityDoc) Field(k string) string {
	if d == nil {
		return ""
	}
	return Text(d.Fields[k])
}

// Text 把字段值格式化为文本。
func Text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []any:
		var parts []string
		for _, e := range x {
			parts = append(parts, Text(e))
		}
		return strings.Join(parts, "、")
	case []string:
		return strings.Join(x, "、")
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, k+" "+Text(x[k]))
		}
		return strings.Join(parts, "，")
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// HiddenByID 返回隐藏真相。
func (d *EntityDoc) HiddenByID(id string) *Hidden {
	for i := range d.Hidden {
		if d.Hidden[i].ID == id {
			return &d.Hidden[i]
		}
	}
	return nil
}

// Clone 深拷贝文档。
func (d *EntityDoc) Clone() *EntityDoc {
	b, _ := json.Marshal(d)
	var out EntityDoc
	_ = json.Unmarshal(b, &out)
	if out.Fields == nil {
		out.Fields = map[string]any{}
	}
	return &out
}

// Retirement 是实体的退场记录（死亡、毁灭、解散、遗失）。
type Retirement struct {
	Reason string `json:"reason"`
	Text   string `json:"text,omitempty"`
	Turn   int    `json:"turn,omitempty"`
	// Location 是退场前 NPC 所在的位置（复活时恢复）。
	Location string `json:"location,omitempty"`
}

// LogEntry 是世界变更日志中的一条（变更日志存于状态，随分支隔离；第 12.1 节）。
type LogEntry struct {
	change.Change
	Turn       int    `json:"turn"`
	Minute     int64  `json:"minute"`
	RevertedBy string `json:"reverted_by,omitempty"`
	Reverts    string `json:"reverts,omitempty"`
}

// Overlay 是覆盖层。
type Overlay struct {
	Patches map[string]map[string]json.RawMessage `json:"patches,omitempty"`
	Created map[string]*EntityDoc                 `json:"created,omitempty"`
	Retired map[string]Retirement                 `json:"retired,omitempty"`
	Links   map[string]json.RawMessage            `json:"links,omitempty"`
	Rev     map[string]int                        `json:"rev,omitempty"`
	Log     []LogEntry                            `json:"log,omitempty"`
	Seq     int                                   `json:"seq,omitempty"`
}

// New 返回空覆盖层。
func New() *Overlay {
	return &Overlay{Patches: map[string]map[string]json.RawMessage{}, Created: map[string]*EntityDoc{}, Retired: map[string]Retirement{}, Links: map[string]json.RawMessage{}, Rev: map[string]int{}}
}

func (o *Overlay) ensure() {
	if o.Patches == nil {
		o.Patches = map[string]map[string]json.RawMessage{}
	}
	if o.Created == nil {
		o.Created = map[string]*EntityDoc{}
	}
	if o.Retired == nil {
		o.Retired = map[string]Retirement{}
	}
	if o.Links == nil {
		o.Links = map[string]json.RawMessage{}
	}
	if o.Rev == nil {
		o.Rev = map[string]int{}
	}
}

// IsRetired 报告实体是否已退场。
func (o *Overlay) IsRetired(id string) bool {
	if o == nil {
		return false
	}
	_, ok := o.Retired[id]
	return ok
}

// Patch 返回覆盖层里某路径的补丁（没有时 nil）。
func (o *Overlay) Patch(id, path string) json.RawMessage {
	if o == nil {
		return nil
	}
	return o.Patches[id][path]
}

// ApplyDoc 把补丁与退场记录应用到文档副本上（静态或新建实体）。
func (o *Overlay) ApplyDoc(d *EntityDoc) *EntityDoc {
	out := d.Clone()
	if o == nil {
		return out
	}
	paths := make([]string, 0, len(o.Patches[d.ID]))
	for p := range o.Patches[d.ID] {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		applyPath(out, p, o.Patches[d.ID][p])
		out.Changed = append(out.Changed, p)
	}
	if r, ok := o.Retired[d.ID]; ok {
		rr := r
		out.Retired = &rr
	}
	out.Rev = o.Rev[d.ID]
	return out
}

func applyPath(d *EntityDoc, path string, raw json.RawMessage) {
	var v any
	_ = json.Unmarshal(raw, &v)
	switch {
	case strings.HasPrefix(path, "fields."):
		d.Fields[strings.TrimPrefix(path, "fields.")] = v
	case path == "importance":
		if f, ok := v.(float64); ok {
			d.Importance = int(f)
		}
	case path == "canon":
		d.Canon, _ = v.(string)
	case path == "alias_unknown":
		d.AliasUnknown, _ = v.(string)
	case path == "tier":
		d.Tier, _ = v.(string)
	case strings.HasPrefix(path, "combat.stats."):
		if d.Stats == nil {
			d.Stats = map[string]int{}
		}
		if f, ok := v.(float64); ok {
			d.Stats[strings.TrimPrefix(path, "combat.stats.")] = int(f)
		}
	case strings.HasPrefix(path, "knowledge.hidden[") && strings.HasSuffix(path, "]"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "knowledge.hidden["), "]")
		text, _ := v.(string)
		if h := d.HiddenByID(id); h != nil {
			h.Text = text
		} else {
			d.Hidden = append(d.Hidden, Hidden{ID: id, Text: text})
		}
	}
}

// PatchablePath 报告路径是否可以存为覆盖层补丁。
func PatchablePath(path string) bool {
	switch {
	case strings.HasPrefix(path, "fields.") && len(path) > len("fields."):
		return true
	case path == "importance", path == "canon", path == "alias_unknown", path == "tier":
		return true
	case strings.HasPrefix(path, "combat.stats.") && len(path) > len("combat.stats."):
		return true
	case strings.HasPrefix(path, "knowledge.hidden[") && strings.HasSuffix(path, "]"):
		return true
	}
	return false
}

// Apply 应用一项非玩家路径的世界修改。timeline_* 与玩家 / NPC 位置路径由调用方处理。
func (o *Overlay) Apply(c change.Change) error {
	o.ensure()
	switch c.Op {
	case change.OpPatch:
		if !PatchablePath(c.Path) {
			return fmt.Errorf("overlay: unsupported path %q", c.Path)
		}
		m := o.Patches[c.Target]
		if string(c.Value) == "null" || len(c.Value) == 0 {
			if m != nil {
				delete(m, c.Path)
				if len(m) == 0 {
					delete(o.Patches, c.Target)
				}
			}
		} else {
			if m == nil {
				m = map[string]json.RawMessage{}
				o.Patches[c.Target] = m
			}
			m[c.Path] = slices.Clone(c.Value)
		}
	case change.OpCreate:
		var d EntityDoc
		if err := json.Unmarshal(c.Value, &d); err != nil {
			return fmt.Errorf("overlay create: %w", err)
		}
		if _, ok := o.Created[c.Target]; ok {
			return fmt.Errorf("overlay create: %s exists", c.Target)
		}
		d.ID = c.Target
		if d.Kind == "" {
			d.Kind = c.Kind
		}
		if d.Fields == nil {
			d.Fields = map[string]any{}
		}
		o.Created[c.Target] = &d
	case change.OpUncreate:
		delete(o.Created, c.Target)
		delete(o.Patches, c.Target)
	case change.OpRetire:
		var r Retirement
		if len(c.Value) > 0 {
			if err := json.Unmarshal(c.Value, &r); err != nil {
				return fmt.Errorf("overlay retire: %w", err)
			}
		}
		if r.Reason == "" {
			r.Reason = "retired"
		}
		o.Retired[c.Target] = r
	case change.OpRestore:
		delete(o.Retired, c.Target)
	case change.OpLink:
		o.Links[c.Target] = slices.Clone(c.Value)
	case change.OpUnlink:
		delete(o.Links, c.Target)
	default:
		return fmt.Errorf("overlay: op %q not handled here", c.Op)
	}
	o.Rev[c.Target]++
	return nil
}

// NextID 分配变更 ID（wc-<回合>-<序号>）。
func (o *Overlay) NextID(turn, n int) string { return fmt.Sprintf("wc-%d-%d", turn, n) }

// Find 返回日志中的变更（没有时 nil）。
func (o *Overlay) Find(id string) *LogEntry {
	if o == nil {
		return nil
	}
	for i := range o.Log {
		if o.Log[i].ID == id {
			return &o.Log[i]
		}
	}
	return nil
}

// Dependents 返回在 id 之后、修改了同一目标（或依赖其新建实体）的未撤销变更（单项撤销前提检查，第 12.5 节）。
func (o *Overlay) Dependents(id string) []string {
	idx := -1
	for i := range o.Log {
		if o.Log[i].ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return nil
	}
	base := o.Log[idx]
	var out []string
	for _, e := range o.Log[idx+1:] {
		if e.RevertedBy != "" || e.Reverts != "" {
			continue
		}
		same := e.Target == base.Target && (base.Op != change.OpPatch || e.Path == base.Path || e.Op != change.OpPatch)
		if same || (base.Op == change.OpCreate && strings.Contains(string(e.Value), base.Target)) {
			out = append(out, e.ID)
		}
	}
	return out
}
