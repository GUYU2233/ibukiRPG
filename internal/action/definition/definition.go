package definition

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Requirement 是一条 CEL 前置条件，可附带失败时给玩家看的说明。
// YAML 中既可写成字符串，也可写成 {expr, message}。
type Requirement struct {
	Expr    string `yaml:"expr"`
	Message string `yaml:"message"`
}

// UnmarshalYAML 支持标量写法。
func (r *Requirement) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		r.Expr = n.Value
		return nil
	}
	type raw Requirement
	var v raw
	if err := n.Decode(&v); err != nil {
		return err
	}
	*r = Requirement(v)
	return nil
}

// Check 描述一次技能检定。Difficulty 为 CEL 表达式。
type Check struct {
	Skill      string `yaml:"skill"`
	Difficulty string `yaml:"difficulty"`
}

// Effect 描述一个结果效果。Values 中的字符串值可以是 CEL 表达式（数值字段）
// 或含 {target.key} 等占位符的模板（文本字段）。
type Effect struct {
	Type   string         `yaml:"type"`
	Target string         `yaml:"target"`
	Values map[string]any `yaml:"values"`
}

// Outcome 是结果列表中的一项；When 为可选 CEL 条件。
type Outcome struct {
	When   string `yaml:"when,omitempty"`
	Effect Effect `yaml:"effect"`
}

// Cost 描述动作消耗。
type Cost struct {
	Time string `yaml:"time"`
}

// Witness 描述动作被目击时写入 NPC 信念的摘要。
type Witness struct {
	Summary string `yaml:"summary"`
	Notable bool   `yaml:"notable"`
}

// 目标类型。
const (
	TargetNone          = "none"
	TargetCharacter     = "character"
	TargetOptionalChar  = "optional_character"
	TargetShopItem      = "shop_item"
	TargetInventoryItem = "inventory_item"
)

// Definition 是数据驱动的 ActionDefinition（见架构文档第 5.1 节）。
type Definition struct {
	ID           string               `yaml:"id"`
	Category     string               `yaml:"category"`
	Name         string               `yaml:"name"`
	Label        string               `yaml:"label"`
	Description  string               `yaml:"description"`
	Target       string               `yaml:"target"`
	Keywords     []string             `yaml:"keywords"`
	Quiet        bool                 `yaml:"quiet"`
	Requirements []Requirement        `yaml:"requirements"`
	Checks       []Check              `yaml:"checks"`
	Cost         Cost                 `yaml:"cost"`
	Outcomes     map[string][]Outcome `yaml:"outcomes"`
	Narration    map[string][]string  `yaml:"narration"`
	Witness      Witness              `yaml:"witness"`
	// Dialogue 标记交谈类动作：执行时引擎从目标 NPC 的台词池挑选台词并写入对话记忆。
	Dialogue bool `yaml:"dialogue"`
}

// NeedsTarget 报告动作是否必须指定目标。
func (d *Definition) NeedsTarget() bool {
	switch d.Target {
	case TargetCharacter, TargetShopItem, TargetInventoryItem:
		return true
	}
	return false
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
	if d.Target == "" {
		d.Target = TargetNone
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
