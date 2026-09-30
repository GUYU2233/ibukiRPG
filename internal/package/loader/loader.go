package loader

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
	"github.com/GUYU2233/ibukiRPG/internal/package/manifest"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
)

// Load 从文件系统（os.DirFS 或内嵌 FS）加载并校验一个内容包。
// 如果提供 ev，会预编译包内全部 CEL 表达式，尽早暴露语法错误。
func Load(fsys fs.FS, ev *expression.Evaluator) (*Package, error) {
	data, err := fs.ReadFile(fsys, manifest.FileName)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return nil, err
	}
	p := &Package{
		Manifest:   m,
		Locations:  map[string]*Location{},
		Characters: map[string]*Character{},
		Items:      map[string]*Item{},
		Actions:    map[string]*definition.Definition{},
		Stories:    map[string]*Story{},
	}
	for _, f := range m.Content["rules"] {
		var raw struct {
			Rules              `yaml:",inline"`
			Profile            string    `yaml:"profile"`
			QuietTurnsForNudge int       `yaml:"quiet_turns_for_nudge"`
			Ambient            []Ambient `yaml:"ambient"`
		}
		if err := readYAML(fsys, f, &raw); err != nil {
			return nil, err
		}
		if len(raw.Skills) > 0 {
			p.Rules.Skills = append(p.Rules.Skills, raw.Skills...)
		}
		if raw.Attributes != nil {
			p.Rules.Attributes = raw.Attributes
		}
		if raw.Conditions != nil {
			p.Rules.Conditions = raw.Conditions
		}
		if raw.ConditionModifiers != nil {
			p.Rules.ConditionModifiers = raw.ConditionModifiers
		}
		if raw.Profile != "" {
			p.Pacing = Pacing{Profile: raw.Profile, QuietTurnsForNudge: raw.QuietTurnsForNudge, Ambient: raw.Ambient}
		}
	}
	for _, f := range m.Content["locations"] {
		var l Location
		if err := readYAML(fsys, f, &l); err != nil {
			return nil, err
		}
		if err := checkID(m, l.ID, f); err != nil {
			return nil, err
		}
		if l.Properties == nil {
			l.Properties = map[string]any{}
		}
		p.Locations[l.ID] = &l
		p.LocationIDs = append(p.LocationIDs, l.ID)
	}
	for _, f := range m.Content["characters"] {
		var c Character
		if err := readYAML(fsys, f, &c); err != nil {
			return nil, err
		}
		if err := checkID(m, c.ID, f); err != nil {
			return nil, err
		}
		if c.Type == "player" {
			p.Player = &c
			continue
		}
		p.Characters[c.ID] = &c
		p.NPCIDs = append(p.NPCIDs, c.ID)
	}
	if p.Player == nil {
		return nil, errors.New("package: no player character (type: player)")
	}
	for _, f := range m.Content["items"] {
		var raw struct {
			Items []Item `yaml:"items"`
		}
		if err := readYAML(fsys, f, &raw); err != nil {
			return nil, err
		}
		for i := range raw.Items {
			it := raw.Items[i]
			if err := checkID(m, it.ID, f); err != nil {
				return nil, err
			}
			p.Items[it.ID] = &it
			p.ItemIDs = append(p.ItemIDs, it.ID)
		}
	}
	for _, f := range m.Content["actions"] {
		b, err := fs.ReadFile(fsys, f)
		if err != nil {
			return nil, err
		}
		d, err := definition.Parse(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if err := checkID(m, d.ID, f); err != nil {
			return nil, err
		}
		p.Actions[d.ID] = d
		p.ActionIDs = append(p.ActionIDs, d.ID)
	}
	for _, f := range m.Content["stories"] {
		var s Story
		if err := readYAML(fsys, f, &s); err != nil {
			return nil, err
		}
		if err := checkID(m, s.ID, f); err != nil {
			return nil, err
		}
		p.Stories[s.ID] = &s
		p.StoryIDs = append(p.StoryIDs, s.ID)
	}
	p.tidyText()
	if err := p.validate(ev); err != nil {
		return nil, err
	}
	return p, nil
}

// tidyText 去掉 YAML 折叠块在中文之间引入的空格与换行。
func (p *Package) tidyText() {
	for _, id := range p.LocationIDs {
		l := p.Locations[id]
		l.Description, l.Short = JoinCJK(l.Description), JoinCJK(l.Short)
	}
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		c.Description = JoinCJK(c.Description)
		c.Personality.Description = JoinCJK(c.Personality.Description)
	}
}

// JoinCJK 删除两个非 ASCII 字符之间的空白，并去掉首尾空白。
func JoinCJK(s string) string {
	r := []rune(strings.TrimSpace(s))
	out := make([]rune, 0, len(r))
	for i := 0; i < len(r); i++ {
		if unicode.IsSpace(r[i]) {
			j := i
			for j < len(r) && unicode.IsSpace(r[j]) {
				j++
			}
			if len(out) > 0 && j < len(r) && out[len(out)-1] > 127 && r[j] > 127 {
				i = j - 1
				continue
			}
			out = append(out, ' ')
			i = j - 1
			continue
		}
		out = append(out, r[i])
	}
	return string(out)
}

func readYAML(fsys fs.FS, name string, v any) error {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func checkID(m *manifest.Manifest, id, file string) error {
	if !manifest.ValidID(id) {
		return fmt.Errorf("%s: invalid id %q", file, id)
	}
	if manifest.NamespaceOf(id) != m.Namespace && manifest.NamespaceOf(id) != "core" {
		return fmt.Errorf("%s: id %q outside namespace %q", file, id, m.Namespace)
	}
	return nil
}

// validate 做引用完整性校验，并预编译 CEL。
func (p *Package) validate(ev *expression.Evaluator) error {
	var errs []error
	if _, ok := p.Locations[p.Manifest.Start.Location]; !ok {
		errs = append(errs, fmt.Errorf("start location %q not found", p.Manifest.Start.Location))
	}
	compile := func(where, expr string) {
		if ev == nil || expr == "" {
			return
		}
		if _, err := ev.Compile(expr); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
		}
	}
	for _, id := range p.LocationIDs {
		l := p.Locations[id]
		for _, e := range l.Exits {
			if _, ok := p.Locations[e.To]; !ok {
				errs = append(errs, fmt.Errorf("%s: exit to unknown location %q", id, e.To))
			}
			compile(id+" exit", e.Requires)
		}
		if l.Shop != nil {
			for _, it := range l.Shop.Items {
				if _, ok := p.Items[it]; !ok {
					errs = append(errs, fmt.Errorf("%s: shop item %q not found", id, it))
				}
			}
		}
	}
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		if _, ok := p.Locations[c.Location]; !ok {
			errs = append(errs, fmt.Errorf("%s: unknown location %q", id, c.Location))
		}
	}
	for _, id := range p.ActionIDs {
		d := p.Actions[id]
		for _, r := range d.Requirements {
			compile(id, r.Expr)
		}
		for _, c := range d.Checks {
			compile(id, c.Difficulty)
			if _, ok := p.SkillByID(c.Skill); !ok {
				errs = append(errs, fmt.Errorf("%s: unknown skill %q", id, c.Skill))
			}
		}
		for _, outs := range d.Outcomes {
			for _, o := range outs {
				compile(id, o.When)
			}
		}
	}
	for _, id := range p.StoryIDs {
		s := p.Stories[id]
		compile(id, s.Trigger)
		compile(id, s.PacingTrigger)
		for _, st := range append(append([]StoryStep{}, s.Steps...), s.Outcomes...) {
			compile(id+"/"+st.ID, st.When)
		}
	}
	return errors.Join(errs...)
}
