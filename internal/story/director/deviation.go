package director

import "slices"

// Input 是一回合的主线贴合度评分输入（全部来自确定性状态与本回合事件）。
type Input struct {
	// AnchorLocations 是当前锚点的“主线地点”；为空表示不按地点评分。
	AnchorLocations []string
	PlayerLocation  string
	// TurnsSinceProgress 是距离上次主线推进的回合数；Budget 是锚点允许的回合预算。
	TurnsSinceProgress int
	Budget             int
	// Progress 表示本回合推进了主线（锚点故事开始 / 推进 / 结束，或锚点达成）。
	Progress bool
	// Deviant 是本回合命中的偏离行为（权重 + 原因）。
	Deviant []Hit
	// Passive 表示本回合是不推动世界的操作（查看、整理装备），只结算明确的偏离行为。
	Passive bool
}

// Hit 是一条命中的偏离规则。
type Hit struct {
	Weight int
	Reason string
}

// Result 是评分结果：偏离度增量与可读原因。
type Result struct {
	Delta   int
	Reasons []string
}

// 评分参数（整数）。
const (
	ProgressBonus   = -20
	OffTrackPerTurn = 4
	OnTrackDecay    = -2
	StallBase       = 2
	StallMax        = 6
)

// Score 计算本回合的偏离度增量。它是纯函数：同样的输入永远得到同样的结果，
// AI 只可能在上游提供“这是哪一类行为”的分类（最终仍映射为故事包声明的规则）。
func Score(in Input) Result {
	var r Result
	add := func(d int, why string) {
		if d != 0 {
			r.Delta += d
			r.Reasons = append(r.Reasons, why)
		}
	}
	if in.Progress {
		add(ProgressBonus, "推进了主线")
	}
	if !in.Passive && !in.Progress {
		if len(in.AnchorLocations) > 0 {
			if slices.Contains(in.AnchorLocations, in.PlayerLocation) {
				add(OnTrackDecay, "身处主线舞台")
			} else {
				add(OffTrackPerTurn, "远离主线舞台")
			}
		}
		if over := in.TurnsSinceProgress - in.Budget; in.Budget > 0 && over > 0 {
			add(min(StallBase+over/4, StallMax), "主线停滞")
		}
	}
	for _, h := range in.Deviant {
		add(h.Weight, h.Reason)
	}
	return r
}

// Level 返回偏离等级：0 贴合、1 轻微偏离、2 严重偏离。
func Level(deviation, mild, heavy int) int {
	switch {
	case deviation >= heavy:
		return 2
	case deviation >= mild:
		return 1
	}
	return 0
}

// Adherence 把偏离度换算成贴合度百分比（100 - 偏离度）。
func Adherence(deviation int) int { return max(0, min(100, 100-deviation)) }
