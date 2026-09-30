package manifest

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version 是简化的语义化版本：主.次.修订 + 可选预发布标记（0.1.2rc1、0.1.2-rc1 都能解析）。
// 预发布版本小于同号正式版本；预发布之间按字符串比较。
type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

var versionRe = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:[-.]?([0-9A-Za-z.]+))?$`)

// ParseVersion 解析版本号。
func ParseVersion(s string) (Version, error) {
	m := versionRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Version{}, fmt.Errorf("invalid version %q", s)
	}
	atoi := func(x string) int {
		n, _ := strconv.Atoi(x)
		return n
	}
	return Version{Major: atoi(m[1]), Minor: atoi(m[2]), Patch: atoi(m[3]), Pre: m[4]}, nil
}

// Compare 返回 -1 / 0 / 1。
func (v Version) Compare(o Version) int {
	for _, d := range []int{v.Major - o.Major, v.Minor - o.Minor, v.Patch - o.Patch} {
		if d < 0 {
			return -1
		}
		if d > 0 {
			return 1
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	case v.Pre < o.Pre:
		return -1
	}
	return 1
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

type clause struct {
	op string
	v  Version
}

// Constraint 是以空格或逗号分隔的约束（全部满足才算满足），例如 ">=0.1.0 <0.3.0"。
type Constraint []clause

var clauseRe = regexp.MustCompile(`^(>=|<=|>|<|=|==|\^|~)?\s*(.+)$`)

// ParseConstraint 解析版本约束。空串表示任意版本。
func ParseConstraint(s string) (Constraint, error) {
	var c Constraint
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		m := clauseRe.FindStringSubmatch(part)
		if m == nil {
			return nil, fmt.Errorf("invalid constraint %q", part)
		}
		v, err := ParseVersion(m[2])
		if err != nil {
			return nil, err
		}
		op := m[1]
		if op == "" || op == "==" {
			op = "="
		}
		c = append(c, clause{op: op, v: v})
	}
	return c, nil
}

// Allows 报告版本是否满足约束。
func (c Constraint) Allows(v Version) bool {
	for _, cl := range c {
		d := v.Compare(cl.v)
		ok := false
		switch cl.op {
		case "=":
			ok = d == 0
		case ">":
			ok = d > 0
		case ">=":
			ok = d >= 0
		case "<":
			ok = d < 0
		case "<=":
			ok = d <= 0
		case "^":
			ok = d >= 0 && v.Major == cl.v.Major && (cl.v.Major != 0 || v.Minor == cl.v.Minor)
		case "~":
			ok = d >= 0 && v.Major == cl.v.Major && v.Minor == cl.v.Minor
		}
		if !ok {
			return false
		}
	}
	return true
}

// Satisfies 是便捷函数：version 是否满足 constraint。无法解析的引擎版本（例如 dev / ci 构建）视为满足。
func Satisfies(version, constraint string) (bool, error) {
	if strings.TrimSpace(constraint) == "" {
		return true, nil
	}
	c, err := ParseConstraint(constraint)
	if err != nil {
		return false, err
	}
	v, err := ParseVersion(version)
	if err != nil {
		return true, nil //nolint:nilerr // 开发构建版本号不规范时不阻止加载
	}
	// 引擎的预发布版本（0.1.2rc1）应满足 ">=0.1.0"；比较时忽略预发布标记更符合直觉。
	v.Pre = ""
	return c.Allows(v), nil
}
