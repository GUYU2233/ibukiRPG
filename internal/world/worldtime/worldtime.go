package worldtime

import (
	"fmt"
	"strconv"
	"strings"
)

// MinutesPerDay 每天的分钟数。
const MinutesPerDay = 24 * 60

// Day 返回第几天（从 1 开始）。
func Day(minute int64) int { return int(minute/MinutesPerDay) + 1 }

// Clock 返回 "19:05"。
func Clock(minute int64) string {
	m := minute % MinutesPerDay
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

// Hour 返回小时 0-23。
func Hour(minute int64) int { return int(minute%MinutesPerDay) / 60 }

// Period 返回时段中文名。
func Period(minute int64) string {
	h := Hour(minute)
	switch {
	case h < 5:
		return "深夜"
	case h < 8:
		return "清晨"
	case h < 11:
		return "上午"
	case h < 13:
		return "正午"
	case h < 17:
		return "下午"
	case h < 19:
		return "傍晚"
	case h < 23:
		return "夜晚"
	default:
		return "深夜"
	}
}

// Format 返回 "第1天 19:05 · 夜晚"。
func Format(minute int64) string {
	return fmt.Sprintf("第%d天 %s · %s", Day(minute), Clock(minute), Period(minute))
}

// ParseDuration 把 "30s" / "5m" / "2h" / "15" 解析为分钟（向上取整，至少 0）。
func ParseDuration(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	unit := s[len(s)-1]
	num := s
	mult := int64(1)
	div := int64(1)
	switch unit {
	case 's':
		num, div = s[:len(s)-1], 60
	case 'm':
		num = s[:len(s)-1]
	case 'h':
		num, mult = s[:len(s)-1], 60
	}
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return (n*mult + div - 1) / div, nil
}
