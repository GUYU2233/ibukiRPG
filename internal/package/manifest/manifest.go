package manifest

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileName 是 Package 根目录下的清单文件名。
const FileName = "manifest.yaml"

// Type 是 Package 类型（见架构文档第 33 节）。
type Type string

// 支持的 Package 类型。
const (
	TypeCore      Type = "core"
	TypeWorld     Type = "world"
	TypeStory     Type = "story"
	TypeMod       Type = "mod"
	TypeExpansion Type = "expansion"
	TypeRuleset   Type = "ruleset"
	TypeContent   Type = "content"
)

var validTypes = map[Type]bool{
	TypeCore: true, TypeWorld: true, TypeStory: true, TypeMod: true,
	TypeExpansion: true, TypeRuleset: true, TypeContent: true,
}

// Dependency 声明对其他 Package 的依赖。
type Dependency struct {
	ID       string `yaml:"id"`
	Version  string `yaml:"version"`
	Optional bool   `yaml:"optional,omitempty"`
}

// Manifest 对应 manifest.yaml。
type Manifest struct {
	ID           string              `yaml:"id"`
	Namespace    string              `yaml:"namespace"`
	Name         string              `yaml:"name"`
	Version      string              `yaml:"version"`
	Type         Type                `yaml:"type"`
	Engine       string              `yaml:"engine"`
	Description  string              `yaml:"description"`
	Authors      []string            `yaml:"authors"`
	Dependencies []Dependency        `yaml:"dependencies"`
	Content      map[string][]string `yaml:"content"`
	Start        Start               `yaml:"start"`
	// 以下为故事包卡片信息（故事包选择界面）。
	Author  string   `yaml:"author"`
	Tagline string   `yaml:"tagline"`
	Cover   string   `yaml:"cover"`  // 包内图片路径（png/jpg/webp），可选
	Icon    string   `yaml:"icon"`   // 封面图标提示（Material Symbols 名称），可选
	Accent  string   `yaml:"accent"` // 封面主色，例如 "#8D5A2B"
	Tags    []string `yaml:"tags"`
	// SaveCompat 声明本版本能继续读取哪些版本的存档（版本约束，例如 ">=0.1.0"）；为空时只读取同版本存档。
	SaveCompat string `yaml:"save_compat"`
}

// AuthorText 返回作者显示文本。
func (m *Manifest) AuthorText() string {
	if m.Author != "" {
		return m.Author
	}
	return strings.Join(m.Authors, "、")
}

// Start 描述世界包的新游戏起点。
type Start struct {
	Location string `yaml:"location"`
	Day      int    `yaml:"day"`
	Time     string `yaml:"time"`
	Intro    string `yaml:"intro"`
	// Variables 是故事变量的初始值（整数）。
	Variables map[string]int `yaml:"variables"`
}

var packIDRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

var namespaceRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

// idRe 匹配命名空间化 ID，例如 demo:character/lena、magic.mod:effect/black_blood。
var idRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*:[a-z][a-z0-9_]*/[a-z0-9_./-]+$`)

// ValidID 判断是否为合法的命名空间化 ID。
func ValidID(id string) bool { return idRe.MatchString(id) }

// NamespaceOf 返回 ID 的命名空间部分。
func NamespaceOf(id string) string {
	ns, _, _ := strings.Cut(id, ":")
	return ns
}

// Parse 解析并校验 manifest 内容。
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Load 从文件加载 manifest。
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path) //nolint:gosec // 路径由调用方（Package Loader）控制
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Validate 做基础结构校验。
func (m *Manifest) Validate() error {
	switch {
	case m.ID == "":
		return fmt.Errorf("manifest.yaml 缺少 id")
	case !packIDRe.MatchString(m.ID):
		return fmt.Errorf("故事包 id %q 不合法：只能使用小写字母、数字、下划线和点，并以字母开头", m.ID)
	case !namespaceRe.MatchString(m.Namespace):
		return fmt.Errorf("故事包 %s 的命名空间 %q 不合法：只能使用小写字母、数字、下划线和点", m.ID, m.Namespace)
	case m.Version == "":
		return fmt.Errorf("故事包 %s 缺少 version", m.ID)
	case !validTypes[m.Type]:
		return fmt.Errorf("故事包 %s 的类型 %q 未知", m.ID, m.Type)
	}
	if _, err := ParseVersion(m.Version); err != nil {
		return fmt.Errorf("故事包 %s 的版本号 %q 不合法（应形如 1.2.0）", m.ID, m.Version)
	}
	for _, c := range []struct{ name, expr string }{{"engine", m.Engine}, {"save_compat", m.SaveCompat}} {
		if c.expr == "" {
			continue
		}
		if _, err := ParseConstraint(c.expr); err != nil {
			return fmt.Errorf("故事包 %s 的 %s 约束 %q 不合法", m.ID, c.name, c.expr)
		}
	}
	for _, d := range m.Dependencies {
		if d.ID == "" {
			return fmt.Errorf("故事包 %s 的依赖缺少 id", m.ID)
		}
		if d.Version != "" {
			if _, err := ParseConstraint(d.Version); err != nil {
				return fmt.Errorf("故事包 %s 对 %s 的版本约束 %q 不合法", m.ID, d.ID, d.Version)
			}
		}
	}
	return nil
}
