// Package timeline 是开放世界的世界事件时间线（架构 V0.3 第 6 节）：世界不等玩家，
// 事件按世界时间进入窗口、推进阶段、到期结算；玩家可以参与 / 破坏 / 无视。
//
// 本包定义故事包格式（world/timeline.yaml）与运行时状态；调度与结算由引擎完成（需要 CEL 与随机流）。
package timeline

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
)

// 事件状态。
const (
	Scheduled = "scheduled"
	Active    = "active"
	Resolved  = "resolved"
	Cancelled = "cancelled"
)

// StatusLabel 返回状态中文名。
func StatusLabel(s string) string {
	switch s {
	case Active:
		return "进行中"
	case Resolved:
		return "已结算"
	case Cancelled:
		return "已取消"
	}
	return "未开始"
}

// Window 是世界时间窗口（"D3 09:00" 形式）。
type Window struct {
	Start string `yaml:"start" json:"start,omitempty"`
	End   string `yaml:"end" json:"end,omitempty"`
}

// Rumor 是玩家提前得知事件的传闻。
type Rumor struct {
	At    string   `yaml:"at" json:"at,omitempty"`
	Where []string `yaml:"where" json:"where,omitempty"`
	Text  string   `yaml:"text" json:"text"`
	False bool     `yaml:"false" json:"false,omitempty"` // 假传闻
}

// Stage 是事件的阶段。
type Stage struct {
	ID   string `yaml:"id" json:"id"`
	At   string `yaml:"at" json:"at"`
	Text string `yaml:"text" json:"text"`
}

// Hook 是玩家可以参与的切入点。
type Hook struct {
	ID       string         `yaml:"id" json:"id"`
	Label    string         `yaml:"label" json:"label"`
	Tags     []string       `yaml:"tags" json:"tags,omitempty"` // join / disrupt
	Requires string         `yaml:"requires" json:"requires,omitempty"`
	Vars     map[string]int `yaml:"vars" json:"vars,omitempty"` // 完成切入点时对事件变量的增量
	Minutes  int            `yaml:"minutes" json:"minutes,omitempty"`
}

// Outcome 是事件的一个结局。
type Outcome struct {
	ID      string               `yaml:"id" json:"id"`
	Title   string               `yaml:"title" json:"title,omitempty"`
	When    string               `yaml:"when" json:"when,omitempty"`
	Default bool                 `yaml:"default" json:"default,omitempty"`
	Text    string               `yaml:"text" json:"text,omitempty"`
	Effects []definition.Outcome `yaml:"effects" json:"effects,omitempty"`
}

// Event 是一个世界事件定义（故事包或 AI 新增）。
type Event struct {
	ID            string         `yaml:"id" json:"id"`
	Title         string         `yaml:"title" json:"title"`
	Summary       string         `yaml:"summary" json:"summary,omitempty"`
	Importance    int            `yaml:"importance" json:"importance,omitempty"`
	Canon         string         `yaml:"canon" json:"canon,omitempty"`
	Pivotal       bool           `yaml:"pivotal" json:"pivotal,omitempty"`
	Window        Window         `yaml:"window" json:"window"`
	At            string         `yaml:"at" json:"at,omitempty"`
	Location      string         `yaml:"location" json:"location,omitempty"`
	Participants  []string       `yaml:"participants" json:"participants,omitempty"`
	Preconditions string         `yaml:"preconditions" json:"preconditions,omitempty"`
	Rumor         []Rumor        `yaml:"rumor" json:"rumor,omitempty"`
	Stages        []Stage        `yaml:"stages" json:"stages,omitempty"`
	Hooks         []Hook         `yaml:"hooks" json:"hooks,omitempty"`
	Outcomes      []Outcome      `yaml:"outcomes" json:"outcomes,omitempty"`
	Resolve       string         `yaml:"resolve" json:"resolve,omitempty"` // rules / ai
	Vars          map[string]int `yaml:"vars" json:"vars,omitempty"`
	Known         bool           `yaml:"known" json:"known,omitempty"` // 开局就知道（公开日程）
}

// StartMinute 返回事件开始的世界分钟（解析失败返回 -1）。
func (e *Event) StartMinute() int64 {
	s := e.Window.Start
	if s == "" {
		s = e.At
	}
	m, err := ParseTime(s)
	if err != nil {
		return -1
	}
	return m
}

// EndMinute 返回事件结束的世界分钟（没有 end 时等于开始）。
func (e *Event) EndMinute() int64 {
	if e.Window.End != "" {
		if m, err := ParseTime(e.Window.End); err == nil {
			return m
		}
	}
	return e.StartMinute()
}

// DefaultOutcome 返回默认结局（没有声明时取最后一个）。
func (e *Event) DefaultOutcome() *Outcome {
	for i := range e.Outcomes {
		if e.Outcomes[i].Default {
			return &e.Outcomes[i]
		}
	}
	if len(e.Outcomes) > 0 {
		return &e.Outcomes[len(e.Outcomes)-1]
	}
	return nil
}

// OutcomeByID 返回结局。
func (e *Event) OutcomeByID(id string) *Outcome {
	for i := range e.Outcomes {
		if e.Outcomes[i].ID == id {
			return &e.Outcomes[i]
		}
	}
	return nil
}

// HookByID 返回切入点。
func (e *Event) HookByID(id string) *Hook {
	for i := range e.Hooks {
		if e.Hooks[i].ID == id {
			return &e.Hooks[i]
		}
	}
	return nil
}

// ParseTime 解析 "D3 09:00" / "D3" / "3 09:00" 为自第 1 天 00:00 起的分钟数。
func ParseTime(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty time")
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "D"), "d")
	var day, h, m int
	if n, _ := fmt.Sscanf(s, "%d %d:%d", &day, &h, &m); n == 3 {
		return int64(day-1)*1440 + int64(h*60+m), nil
	}
	if n, _ := fmt.Sscanf(s, "%d", &day); n == 1 && !strings.Contains(s, ":") {
		return int64(day-1) * 1440, nil
	}
	return 0, fmt.Errorf("bad world time %q (want \"D3 09:00\")", s)
}

// FormatTime 把世界分钟格式化为 "D3 09:00"。
func FormatTime(min int64) string {
	return fmt.Sprintf("D%d %02d:%02d", min/1440+1, (min%1440)/60, min%60)
}

// Runtime 是一个事件的运行时状态。
type Runtime struct {
	Status   string                     `json:"status"`
	Stage    string                     `json:"stage,omitempty"`
	Vars     map[string]int             `json:"vars,omitempty"`
	Outcome  string                     `json:"outcome,omitempty"`
	By       string                     `json:"by,omitempty"` // world / player / ai
	Started  int64                      `json:"started,omitempty"`
	Ended    int64                      `json:"ended,omitempty"`
	Forced   string                     `json:"forced,omitempty"` // AI 通过 timeline_patch 指定的结局
	Joined   []string                   `json:"joined,omitempty"` // 玩家完成的切入点
	Patches  map[string]json.RawMessage `json:"patches,omitempty"`
	Reason   string                     `json:"reason,omitempty"`
	Rumors   []int                      `json:"rumors,omitempty"` // 已扩散的传闻下标
	Location string                     `json:"location,omitempty"`
}

// State 是时间线的运行时：故事包事件 + AI 新增事件。
type State struct {
	Events map[string]*Runtime `json:"events,omitempty"`
	Added  map[string]*Event   `json:"added,omitempty"`
	// DayAdds 记录每个世界日 AI 新增的事件数（每日最多 3 个，第 6.8 节）。
	DayAdds map[int]int `json:"day_adds,omitempty"`
}

// New 返回空时间线状态。
func New() *State {
	return &State{Events: map[string]*Runtime{}, Added: map[string]*Event{}, DayAdds: map[int]int{}}
}

// Ensure 补齐 map。
func (s *State) Ensure() {
	if s.Events == nil {
		s.Events = map[string]*Runtime{}
	}
	if s.Added == nil {
		s.Added = map[string]*Event{}
	}
	if s.DayAdds == nil {
		s.DayAdds = map[int]int{}
	}
}

// Get 返回（必要时创建）事件的运行时状态。
func (s *State) Get(id string, def *Event) *Runtime {
	s.Ensure()
	r := s.Events[id]
	if r == nil {
		r = &Runtime{Status: Scheduled, Vars: map[string]int{}}
		if def != nil {
			for k, v := range def.Vars {
				r.Vars[k] = v
			}
		}
		s.Events[id] = r
	}
	if r.Vars == nil {
		r.Vars = map[string]int{}
	}
	return r
}

// Status 返回事件状态（没有运行时记录视为 scheduled）。
func (s *State) Status(id string) string {
	if s == nil || s.Events[id] == nil {
		return Scheduled
	}
	return s.Events[id].Status
}

// SortedIDs 按开始时间、ID 排序（确定性调度）。
func SortedIDs(defs map[string]*Event) []string {
	ids := make([]string, 0, len(defs))
	for id := range defs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := defs[ids[i]].StartMinute(), defs[ids[j]].StartMinute()
		if a != b {
			return a < b
		}
		return ids[i] < ids[j]
	})
	return ids
}

// Countdown 返回到 target 的模糊倒计时（“约 1 天后”“3 小时后”）。
func Countdown(now, target int64) string {
	d := target - now
	switch {
	case d <= 0:
		return "现在"
	case d < 60:
		return fmt.Sprintf("%d 分钟后", d)
	case d < 1440:
		return fmt.Sprintf("%d 小时后", (d+30)/60)
	}
	days, hours := d/1440, (d%1440+30)/60
	if hours == 0 || hours == 24 {
		return fmt.Sprintf("%d 天后", days+hours/24)
	}
	return fmt.Sprintf("%d 天 %d 时后", days, hours)
}
