package checks

import "fmt"

// 难度范围：Core 对 AI 建议的难度一律夹到这个区间（第 6 节“Core 必须校验”）。
const (
	MinDC = 5
	MaxDC = 25
	Die   = 20
)

// Result 是一次 d20 技能检定的结果。
type Result struct {
	Skill     string `json:"skill"`
	SkillName string `json:"skill_name"`
	Roll      int    `json:"roll"`
	Modifier  int    `json:"modifier"`
	DC        int    `json:"dc"`
	Total     int    `json:"total"`
	Success   bool   `json:"success"`
	Critical  bool   `json:"critical"` // 天然 20
	Fumble    bool   `json:"fumble"`   // 天然 1
}

// floorDiv 向下取整除法，避免 Go 整数除法向零截断导致负属性修正偏差。
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// Modifier 计算检定修正：技能值 + (属性 - 10) / 2（向下取整）+ 额外修正。
func Modifier(skill, attribute, extra int) int {
	return skill + floorDiv(attribute-10, 2) + extra
}

// ClampDC 把难度限制在 [MinDC, MaxDC]。
func ClampDC(dc int) int {
	if dc < MinDC {
		return MinDC
	}
	if dc > MaxDC {
		return MaxDC
	}
	return dc
}

// Resolve 根据骰值判定：天然 20 必定成功，天然 1 必定失败，否则总值 ≥ 难度即成功。
func Resolve(skill, skillName string, roll, modifier, dc int) Result {
	r := Result{Skill: skill, SkillName: skillName, Roll: roll, Modifier: modifier, DC: dc, Total: roll + modifier}
	switch roll {
	case Die:
		r.Success, r.Critical = true, true
	case 1:
		r.Success, r.Fumble = false, true
	default:
		r.Success = r.Total >= dc
	}
	return r
}

// Explain 用通俗中文解释检定结果，例如：
// “掷出 14，加上说服修正 +4，共 18，对抗难度 15 → 成功”。
func Explain(r Result) string {
	verdict := "失败"
	if r.Success {
		verdict = "成功"
	}
	switch {
	case r.Critical:
		return fmt.Sprintf("掷出天然 20！%s检定大成功（难度 %d）", r.SkillName, r.DC)
	case r.Fumble:
		return fmt.Sprintf("掷出天然 1……%s检定大失败（难度 %d）", r.SkillName, r.DC)
	}
	cmp := "≥"
	if !r.Success {
		cmp = "<"
	}
	return fmt.Sprintf("掷出 %d，%s修正 %+d，合计 %d %s 难度 %d → %s", r.Roll, r.SkillName, r.Modifier, r.Total, cmp, r.DC, verdict)
}

// Chance 返回成功概率（百分比，0-100），供 UI 预估难度。
func Chance(modifier, dc int) int {
	wins := 0
	for roll := 1; roll <= Die; roll++ {
		if Resolve("", "", roll, modifier, dc).Success {
			wins++
		}
	}
	return wins * 100 / Die
}
