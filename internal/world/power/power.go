// Package power 是卡片强度预算（v0.2.0-rc1）：按卡片种类、稀有度与等级计算数值预算，
// 校验器、角色审查与 AI 卡片生成共用同一个公式，保证 AI 不会给出远超同类的数值。
//
// 预算 = 参考强度 × 种类系数 × 稀有度系数 × (1 + 0.1 × (等级 − 1))
//
// 参考强度是故事包玩家起始档位（balance.power_tiers 中 player_start_tier）的上限，默认 90。
// 强度分 = Σ 数值 × 权重（hp 0.25、atk / def 2、spd 1.5……，未列出的数值权重 1）。
package power

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

// DefaultRef 是故事包没有强度档位时的参考强度。
const DefaultRef = 90

// MaxLevel 是预算公式接受的最高等级。
const MaxLevel = 20

// Rarities 是稀有度（从低到高）。
var Rarities = []string{"common", "uncommon", "rare", "epic", "legendary"}

var rarityMul = map[string]float64{"common": 1.0, "uncommon": 1.25, "rare": 1.6, "epic": 2.0, "legendary": 2.6}

var rarityLabel = map[string]string{"common": "普通", "uncommon": "精良", "rare": "稀有", "epic": "史诗", "legendary": "传说"}

// kindShare 是各种类卡片占参考强度的比例。
var kindShare = map[string]float64{
	"character": 1.0, "enemy": 1.0, "mech": 2.0,
	"weapon": 0.35, "armor": 0.3, "item": 0.2, "skill": 0.25, "tech": 0.25,
}

// Kinds 返回有数值预算的种类（排序）。
func Kinds() []string { return slices.Sorted(maps.Keys(kindShare)) }

// weights 是强度分里各数值的权重。
var weights = map[string]float64{
	"hp": 0.25, "max_hp": 0.25, "atk": 2, "def": 2, "dmg": 2, "armor": 2, "spd": 1.5, "speed": 1.5,
	"energy": 0.2, "heat": 0, "crit": 1.5,
}

// ignored 是不计入强度分的键（等级、稀有度编码、尺寸等描述性数值）。
var ignored = map[string]bool{"level": true, "lv": true, "rarity": true, "size": true, "price": true, "value": true}

// Params 是预算的输入。
type Params struct {
	Kind   string `json:"kind"`
	Rarity string `json:"rarity"`
	Level  int    `json:"level"`
}

// Norm 规范化参数：未知稀有度按 common，等级限制在 1..MaxLevel。
func (p Params) Norm() Params {
	p.Rarity = strings.ToLower(strings.TrimSpace(p.Rarity))
	if _, ok := rarityMul[p.Rarity]; !ok {
		p.Rarity = "common"
	}
	p.Level = min(max(p.Level, 1), MaxLevel)
	return p
}

// RarityLabel 返回稀有度中文名。
func RarityLabel(r string) string {
	if l, ok := rarityLabel[r]; ok {
		return l
	}
	return r
}

// Has 报告该种类是否有数值预算。
func Has(kind string) bool { _, ok := kindShare[kind]; return ok }

// Budget 返回预算；ref<=0 时使用 DefaultRef；没有预算的种类返回 0。
func Budget(ref int, p Params) int {
	share, ok := kindShare[p.Kind]
	if !ok {
		return 0
	}
	if ref <= 0 {
		ref = DefaultRef
	}
	p = p.Norm()
	v := float64(ref) * share * rarityMul[p.Rarity] * (1 + 0.1*float64(p.Level-1))
	return int(math.Round(v))
}

// Weight 返回数值的权重。
func Weight(stat string) float64 {
	if ignored[stat] {
		return 0
	}
	if w, ok := weights[stat]; ok {
		return w
	}
	return 1
}

// Score 返回一组数值的强度分（负数按 0）。
func Score(stats map[string]int) int {
	v := 0.0
	for k, x := range stats {
		v += float64(max(x, 0)) * Weight(k)
	}
	return int(math.Round(v))
}

// Fit 把数值等比缩小到预算以内（返回新 map 与是否调整过）；budget<=0 表示不限。
func Fit(stats map[string]int, budget int) (map[string]int, bool) {
	out := maps.Clone(stats)
	if out == nil {
		out = map[string]int{}
	}
	changed := false
	for k, v := range out {
		if v < 0 {
			out[k] = 0
			changed = true
		}
	}
	if budget <= 0 {
		return out, changed
	}
	score := Score(out)
	if score <= budget {
		return out, changed
	}
	ratio := float64(budget) / float64(score)
	for _, k := range slices.Sorted(maps.Keys(out)) {
		if Weight(k) == 0 {
			continue
		}
		out[k] = int(math.Floor(float64(out[k]) * ratio))
	}
	// 取整后仍可能略超：从权重最大的数值开始逐个减 1
	keys := slices.Sorted(maps.Keys(out))
	slices.SortStableFunc(keys, func(a, b string) int {
		switch {
		case Weight(a) > Weight(b):
			return -1
		case Weight(a) < Weight(b):
			return 1
		}
		return 0
	})
	for Score(out) > budget {
		reduced := false
		for _, k := range keys {
			if Weight(k) > 0 && out[k] > 0 {
				out[k]--
				reduced = true
				if Score(out) <= budget {
					break
				}
			}
		}
		if !reduced {
			break
		}
	}
	return out, true
}

// MaxFor 返回在其它数值不变时，stat 最多能取到的值（预算内）。
func MaxFor(stats map[string]int, stat string, budget int) int {
	w := Weight(stat)
	if w == 0 || budget <= 0 {
		return math.MaxInt32
	}
	rest := maps.Clone(stats)
	delete(rest, stat)
	left := float64(budget - Score(rest))
	return max(int(math.Floor(left/w)), 0)
}

// ParamsFrom 从卡片的种类、标签、元数据与字段推断预算参数。
// 物品的武器 / 护甲子类来自 meta.type / fields.type / tags；稀有度来自 meta.rarity / fields.rarity / tags；
// 等级来自 stats.level / meta.level / fields.level。
func ParamsFrom(kind string, tags []string, meta map[string]string, fields map[string]any, stats map[string]int) Params {
	p := Params{Kind: kind, Level: 1}
	str := func(k string) string {
		if v := meta[k]; v != "" {
			return v
		}
		if v, ok := fields[k].(string); ok {
			return v
		}
		return ""
	}
	if kind == "item" {
		sub := str("type")
		for _, t := range tags {
			if t == "weapon" || t == "armor" {
				sub = t
			}
		}
		if sub == "weapon" || sub == "armor" {
			p.Kind = sub
		}
	}
	p.Rarity = str("rarity")
	if p.Rarity == "" {
		for _, t := range tags {
			if _, ok := rarityMul[t]; ok {
				p.Rarity = t
			}
		}
	}
	switch {
	case stats["level"] > 0:
		p.Level = stats["level"]
	case str("level") != "":
		if n, err := strconv.Atoi(str("level")); err == nil {
			p.Level = n
		}
	default:
		if f, ok := fields["level"].(float64); ok {
			p.Level = int(f)
		}
	}
	return p.Norm()
}

// Describe 返回预算说明（提示词 / 审查用），例如“weapon · 稀有 · Lv3：预算 61”。
func Describe(ref int, p Params) string {
	p = p.Norm()
	return fmt.Sprintf("%s · %s · Lv%d：预算 %d", p.Kind, RarityLabel(p.Rarity), p.Level, Budget(ref, p))
}

// Table 返回提示词用的预算表（各种类 common/rare/legendary 的 1 级预算与权重说明）。
func Table(ref int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "强度预算（参考强度 %d；每升 1 级 +10%%；稀有度 普通×1 精良×1.25 稀有×1.6 史诗×2 传说×2.6）：\n", refOr(ref))
	for _, k := range Kinds() {
		fmt.Fprintf(&sb, "- %s：普通 %d / 稀有 %d / 传说 %d\n", k, Budget(ref, Params{Kind: k}), Budget(ref, Params{Kind: k, Rarity: "rare"}), Budget(ref, Params{Kind: k, Rarity: "legendary"}))
	}
	sb.WriteString("强度分 = Σ 数值 × 权重（hp 0.25，atk / def / dmg / armor 2，spd 1.5，crit 1.5，energy 0.2，其它 1；level 不计）。超出预算的数值会被等比缩小。\n")
	return sb.String()
}

func refOr(ref int) int {
	if ref <= 0 {
		return DefaultRef
	}
	return ref
}
