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
}

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
		return fmt.Errorf("manifest: id is required")
	case !namespaceRe.MatchString(m.Namespace):
		return fmt.Errorf("manifest %s: invalid namespace %q", m.ID, m.Namespace)
	case m.Version == "":
		return fmt.Errorf("manifest %s: version is required", m.ID)
	case !validTypes[m.Type]:
		return fmt.Errorf("manifest %s: unknown type %q", m.ID, m.Type)
	}
	return nil
}
