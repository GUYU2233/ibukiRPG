package query

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// DefaultHUD 是故事包没有声明 hud 时使用的状态栏：位置、时间、铜币、进行中的事件。
var DefaultHUD = []loader.HUDField{
	{ID: "story", Label: "事件", Icon: "auto_stories", Bind: "story", Visible: "", Order: 0, Wide: true},
	{ID: "location", Label: "位置", Icon: "place", Bind: "location", Order: 10},
	{ID: "time", Label: "时间", Icon: "schedule", Bind: "time", Order: 20},
	{ID: "gold", Label: "铜币", Icon: "payments", Bind: "gold", Order: 30},
}

var toneOrder = []string{"danger", "warning", "success", "normal"}

// Objective 返回当前主线目标（故事包 objectives 中第一个成立的）。
func (q *Q) Objective(s *state.State) string {
	vars := engine.Vars(q.Pkg, s, "", "")
	for _, o := range q.Pkg.Objectives {
		if o.When == "" {
			return o.Text
		}
		if ok, err := q.Eval.EvalBool(o.When, vars); err == nil && ok {
			return o.Text
		}
	}
	return ""
}

// Hud 求值故事包声明的 HUD（没有声明时用 DefaultHUD）。求值失败或值为空的字段不显示——HUD 永远不会让回合失败。
func (q *Q) Hud(s *state.State) []dto.HudFieldV1 {
	p := q.Pkg
	fields := p.HUD
	if len(fields) == 0 {
		fields = DefaultHUD
	}
	fields = slices.Clone(fields)
	slices.SortStableFunc(fields, func(a, b loader.HUDField) int { return a.Order - b.Order })
	vars := engine.Vars(p, s, "", "")
	out := []dto.HudFieldV1{}
	for _, f := range fields {
		if f.Visible != "" {
			ok, err := q.Eval.EvalBool(f.Visible, vars)
			if err != nil || !ok {
				continue
			}
		}
		raw, num, isNum := q.hudValue(s, f, vars)
		if raw == "" {
			continue
		}
		v := dto.HudFieldV1{ID: f.ID, Label: f.Label, Icon: f.Icon, Value: raw, Compact: f.IsCompact(), Wide: f.Wide, Tone: f.Tone, Progress: -1}
		if f.Format != "" {
			v.Value = strings.ReplaceAll(f.Format, "{value}", raw)
		}
		if f.Max > 0 && isNum {
			v.Progress = int(min(max(num*1000/int64(f.Max), 0), 1000))
		}
		for _, t := range toneOrder {
			expr, ok := f.Tones[t]
			if !ok {
				continue
			}
			if hit, err := q.Eval.EvalBool(expr, vars); err == nil && hit {
				v.Tone = t
				break
			}
		}
		out = append(out, v)
	}
	return out
}

func (q *Q) hudValue(s *state.State, f loader.HUDField, vars map[string]any) (string, int64, bool) {
	p := q.Pkg
	if f.Value != "" {
		x, err := q.Eval.Eval(f.Value, vars)
		if err != nil {
			return "", 0, false
		}
		switch n := x.(type) {
		case int64:
			return fmt.Sprint(n), n, true
		case uint64:
			return fmt.Sprint(n), int64(n), true //nolint:gosec // 游戏数值很小
		case float64:
			return fmt.Sprint(n), int64(n), true
		case bool:
			return map[bool]string{true: "是", false: "否"}[n], 0, false
		}
		return fmt.Sprint(x), 0, false
	}
	switch {
	case f.Bind == "location":
		return p.EntityName(s.Player.Location), 0, false
	case f.Bind == "time":
		return worldtime.Format(s.Minute), 0, false
	case f.Bind == "clock":
		return worldtime.Clock(s.Minute), 0, false
	case f.Bind == "day":
		return fmt.Sprintf("第 %d 天", worldtime.Day(s.Minute)), int64(worldtime.Day(s.Minute)), true
	case f.Bind == "period":
		return worldtime.Period(s.Minute), 0, false
	case f.Bind == "gold":
		return fmt.Sprint(s.Player.Gold), int64(s.Player.Gold), true
	case f.Bind == "turn":
		return fmt.Sprint(s.Turn), int64(s.Turn), true
	case f.Bind == "story":
		if st := q.ActiveStory(s); st != nil {
			return st.Title, 0, false
		}
		return "", 0, false
	case f.Bind == "objective":
		return q.Objective(s), 0, false
	case f.Bind == "conditions":
		var names []string
		for _, c := range s.Player.Conditions {
			names = append(names, p.ConditionName(c))
		}
		return strings.Join(names, "、"), 0, false
	case strings.HasPrefix(f.Bind, "var:"):
		k := strings.TrimPrefix(f.Bind, "var:")
		n, ok := s.Vars[k]
		if !ok {
			n = p.Variables[k]
		}
		return fmt.Sprint(n), int64(n), true
	case strings.HasPrefix(f.Bind, "flag:"):
		return map[bool]string{true: "是", false: "否"}[s.Flags[strings.TrimPrefix(f.Bind, "flag:")]], 0, false
	}
	return "", 0, false
}
