package guard

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Facts 是本回合不可变事实（第 11 节）。叙事必须与之一致。
type Facts struct {
	PlayerLocation string   `json:"player_location" yaml:"player_location"`
	Time           string   `json:"time" yaml:"time"`
	Outcome        string   `json:"outcome" yaml:"outcome"` // success / failure / none
	Checks         []string `json:"checks,omitempty" yaml:"checks"`
	GoldDelta      int      `json:"gold_delta" yaml:"gold_delta"`
	Gold           int      `json:"gold" yaml:"gold"`
	ItemsAdded     []string `json:"items_added,omitempty" yaml:"items_added"`
	ItemsRemoved   []string `json:"items_removed,omitempty" yaml:"items_removed"`
	Relationships  []string `json:"relationship_changes,omitempty" yaml:"relationship_changes"`
	StoryEvents    []string `json:"story_events,omitempty" yaml:"story_events"`
	PresentNPCs    []string `json:"present_npcs" yaml:"present_npcs"`
	Deaths         []string `json:"deaths" yaml:"deaths"` // Phase 0 永远为空
}

// Context 是 Guard 需要的额外信息（不会发给 Narrator）。
type Context struct {
	AllowedNames   []string `yaml:"allowed_names"`   // 本回合可以出现的人物名（在场者 + 事件中涉及者）
	AbsentNames    []string `yaml:"absent_names"`    // 不在场、不应“出现”的人物名
	OtherLocations []string `yaml:"other_locations"` // 其他地点名（不应“来到”）
	OtherItems     []string `yaml:"other_items"`     // 本回合没有获得的物品名
	SecretKeywords []string `yaml:"secret_keywords"` // 玩家不知道的秘密关键词
	// Base 是模板事实稿：其中出现的数字、人名视为已授权。
	Base string `yaml:"base"`
}

// Violation 是一条违规。
type Violation struct {
	Rule   string `json:"rule" yaml:"rule"`
	Detail string `json:"detail" yaml:"detail"`
}

// Report 是检查结果。
type Report struct {
	OK         bool        `json:"ok" yaml:"ok"`
	Violations []Violation `json:"violations,omitempty" yaml:"violations"`
}

// MaxRunes 叙事长度上限。
const MaxRunes = 700

var (
	deathWords   = []string{"死了", "死去", "身亡", "断气", "咽气", "尸体", "被杀", "丧命", "毙命"}
	successWords = []string{"成功说服", "终于说服", "被你说服", "答应了你的请求", "完全相信了你", "被你吓住", "吓得屁滚尿流"}
	failureWords = []string{"没能说服", "不为所动", "失败了", "没有成功", "毫不在意你的威胁", "识破了你"}
	arriveVerbs  = []string{"来到", "走进", "进入", "到了", "回到"}
	gainVerbs    = []string{"得到", "获得", "拿到", "捡到", "收下", "塞给你"}
	moneyRe      = regexp.MustCompile(`([0-9]+|[零一二两三四五六七八九十百]+)\s*(?:枚|个|块)?\s*(铜币|金币|银币|铜板)`)
)

// Check 执行规则检查（Rule Check，第 11 节的第一层）。
func Check(text string, f Facts, c Context) Report {
	var vs []Violation
	add := func(rule, format string, a ...any) {
		vs = append(vs, Violation{Rule: rule, Detail: fmt.Sprintf(format, a...)})
	}
	t := strings.TrimSpace(text)
	if t == "" {
		add("empty", "叙事为空")
	}
	if n := utf8.RuneCountInString(t); n > MaxRunes {
		add("length", "叙事过长（%d 字）", n)
	}
	for _, k := range c.SecretKeywords {
		if k != "" && strings.Contains(t, k) && !strings.Contains(c.Base, k) {
			add("secret_leak", "泄露了玩家不知道的秘密：%s", k)
		}
	}
	if len(f.Deaths) == 0 {
		for _, w := range deathWords {
			if strings.Contains(t, w) && !strings.Contains(c.Base, w) {
				add("death", "出现了未发生的死亡：%s", w)
			}
		}
	}
	for _, n := range c.AbsentNames {
		if n != "" && strings.Contains(t, n) && !contains(c.AllowedNames, n) && !strings.Contains(c.Base, n) {
			add("absent_npc", "不在场的人物出现在叙事中：%s", n)
		}
	}
	switch f.Outcome {
	case "failure":
		for _, w := range successWords {
			if strings.Contains(t, w) {
				add("outcome", "检定失败但叙事写成了成功：%s", w)
			}
		}
	case "success":
		for _, w := range failureWords {
			if strings.Contains(t, w) && !strings.Contains(c.Base, w) {
				add("outcome", "检定成功但叙事写成了失败：%s", w)
			}
		}
	}
	for _, loc := range c.OtherLocations {
		for _, v := range arriveVerbs {
			if strings.Contains(t, v+loc) || strings.Contains(t, v+"了"+loc) {
				add("location", "叙事中玩家到了别的地点：%s", loc)
			}
		}
	}
	for _, it := range c.OtherItems {
		for _, v := range gainVerbs {
			if strings.Contains(t, v+it) || strings.Contains(t, v+"了"+it) || strings.Contains(t, v+"一"+it) {
				add("item", "叙事中出现了未发生的物品获得：%s", it)
			}
		}
	}
	for _, m := range moneyRe.FindAllStringSubmatch(t, -1) {
		n, ok := parseNum(m[1])
		if !ok || strings.Contains(c.Base, m[0]) {
			continue
		}
		if m[2] == "铜币" || m[2] == "铜板" || m[2] == "金币" {
			d := f.GoldDelta
			if d < 0 {
				d = -d
			}
			if n != d && n != f.Gold {
				add("gold", "叙事中的钱数（%s）与事实不符（变化 %d，现有 %d）", m[0], f.GoldDelta, f.Gold)
			}
		}
	}
	return Report{OK: len(vs) == 0, Violations: vs}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// parseNum 解析阿拉伯数字或简单中文数字（最多到百）。
func parseNum(s string) (int, bool) {
	if n, err := strconv.Atoi(s); err == nil {
		return n, true
	}
	digits := map[rune]int{'零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	total, cur := 0, 0
	for _, r := range s {
		switch r {
		case '十':
			if cur == 0 {
				cur = 1
			}
			total += cur * 10
			cur = 0
		case '百':
			if cur == 0 {
				cur = 1
			}
			total += cur * 100
			cur = 0
		default:
			d, ok := digits[r]
			if !ok {
				return 0, false
			}
			cur = d
		}
	}
	return total + cur, true
}
