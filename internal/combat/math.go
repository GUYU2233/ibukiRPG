package combat

// 本文件是纯整数的战斗公式（第 28 节：不使用浮点数），没有任何随机性——
// 随机数由调用方从命名 RNG 流取得后作为参数传入。

// HitChance 返回命中率（百分比，夹紧到 5..95）。
func HitChance(acc, eva, bonus int) int { return clamp(acc-eva+bonus, 5, 95) }

// CritChance 返回暴击率（百分比，夹紧到 0..75）。
func CritChance(crit, bonus int) int { return clamp(crit+bonus, 0, 75) }

// Breakdown 是一次伤害计算的分解，供战斗记录展示。
type Breakdown struct {
	Raw       int // 攻击 × 威力%
	Mitigated int // 经防御折算后的伤害
	Variance  int // 浮动百分比（90..110）
	Crit      bool
	Guarded   bool
	Final     int
}

// Damage 计算伤害：raw = atk × power%；防御按 raw² / (raw + def') 平滑折算（永远 ≥ 1，
// 防御越高收益递减）；再乘浮动（90..110%）、暴击（×1.5）、防御姿态（×0.5）。
func Damage(atk, def, power, pierce, variance int, crit, guarded bool) Breakdown {
	if power <= 0 {
		power = 100
	}
	raw := max(atk*power/100, 1)
	effDef := max(def*(100-clamp(pierce, 0, 100))/100, 0)
	m := max(raw*raw/(raw+effDef), 1)
	b := Breakdown{Raw: raw, Mitigated: m, Variance: variance, Crit: crit, Guarded: guarded}
	d := m * clamp(variance, 50, 150) / 100
	if crit {
		d = d * 3 / 2
	}
	if guarded {
		d /= 2
	}
	b.Final = max(d, 1)
	return b
}

// FleeChance 返回逃跑成功率（百分比）。
func FleeChance(partySpd, enemySpd int) int { return clamp(50+(partySpd-enemySpd)*3, 10, 90) }

// Pct 返回 v 占 total 的百分比（total ≤ 0 时为 0）。
func Pct(v, total int) int {
	if total <= 0 {
		return 0
	}
	return v * 100 / total
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Clamp 导出的夹紧函数。
func Clamp(v, lo, hi int) int { return clamp(v, lo, hi) }
