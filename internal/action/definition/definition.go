package definition

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Check 描述一次技能检定。Difficulty 为 CEL 表达式。
type Check struct {
	Skill      string `yaml:"skill"`
	Difficulty string `yaml:"difficulty"`
}

// Effect 描述一个结果效果。
type Effect struct {
	Type   string         `yaml:"type"`
	Target string         `yaml:"target"`
	Values map[string]any `yaml:"values"`
}

// Outcome 是结果列表中的一项。
type Outcome struct {
	Effect Effect `yaml:"effect"`
}

// Cost 描述动作消耗。
type Cost struct {
	Time string `yaml:"time"`
}

// Definition 是数据驱动的 ActionDefinition（见架构文档第 5.1 节）。
type Definition struct {
	ID           string               `yaml:"id"`
	Category     string               `yaml:"category"`
	Name         string               `yaml:"name"`
	Description  string               `yaml:"description"`
	Requirements []string             `yaml:"requirements"`
	Checks       []Check              `yaml:"checks"`
	Cost         Cost                 `yaml:"cost"`
	Outcomes     map[string][]Outcome `yaml:"outcomes"`
}

// Parse 解析 ActionDefinition YAML。
func Parse(data []byte) (*Definition, error) {
	var d Definition
	if err := yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("parse action: %w", err)
	}
	if d.ID == "" {
		return nil, fmt.Errorf("action: id is required")
	}
	return &d, nil
}

// Load 从文件加载 ActionDefinition。
func Load(path string) (*Definition, error) {
	data, err := os.ReadFile(path) //nolint:gosec // 路径由 Package Loader 控制
	if err != nil {
		return nil, err
	}
	return Parse(data)
}
