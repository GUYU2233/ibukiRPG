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
		return nil, fmt.Errorf("读取 manifest.yaml 失败：%w", err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return nil, err
	}
	p := &Package{
		FS:         fsys,
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
		return nil, errors.New("故事包里没有玩家角色（需要一个 type: player 的角色文件）")
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
	for _, f := range m.Content["hud"] {
		var raw struct {
			HUD        []HUDField  `yaml:"hud"`
			Objectives []Objective `yaml:"objectives"`
		}
		if err := readYAML(fsys, f, &raw); err != nil {
			return nil, err
		}
		p.HUD = append(p.HUD, raw.HUD...)
		p.Objectives = append(p.Objectives, raw.Objectives...)
	}
	p.Variables = map[string]int{}
	for k, v := range m.Start.Variables {
		p.Variables[k] = v
	}
	if err := p.loadRPG(fsys, m); err != nil {
		return nil, err
	}
	p.tidyText()
	p.normalizeDialogue()
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

// normalizeDialogue 把旧格式台词（greet / friendly / wary / story / after_story）转换为带条件的台词池。
// 旧格式的每一条都变成一条可重复的台词，条件与旧版叙事器的选择顺序一致。
func (p *Package) normalizeDialogue() {
	for _, id := range p.NPCIDs {
		d := &p.Characters[id].Dialogue
		if len(d.Lines) > 0 {
			continue
		}
		add := func(prefix, topic, when string, prio int, lines []string) {
			for i, t := range lines {
				d.Lines = append(d.Lines, DialogueLine{ID: fmt.Sprintf("%s_%d", prefix, i+1), Topic: topic, When: when, Priority: prio, Text: t})
			}
		}
		for _, sid := range p.StoryIDs {
			k := Key(sid)
			add("story_"+k, k, fmt.Sprintf(`stories.%s.status == "active"`, k), 50, d.Story[sid])
			for _, out := range p.Stories[sid].Outcomes {
				add("after_"+k+"_"+out.ID, k, fmt.Sprintf(`stories.%s.outcome == %q`, k, out.ID), 40, d.AfterStory[sid+"/"+out.ID])
			}
		}
		add("friendly", "smalltalk", `npc.attitude in ["友好", "亲近"]`, 20, d.Friendly)
		add("wary", "smalltalk", `npc.attitude in ["敌视", "戒备", "畏惧"]`, 20, d.Wary)
		add("greet", "greeting", "", 0, d.Greet)
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
		return fmt.Errorf("%s 格式错误：%w", name, err)
	}
	return nil
}

func checkID(m *manifest.Manifest, id, file string) error {
	if !manifest.ValidID(id) {
		return fmt.Errorf("%s：ID %q 不合法（应形如 命名空间:类别/名字）", file, id)
	}
	if manifest.NamespaceOf(id) != m.Namespace && manifest.NamespaceOf(id) != "core" {
		return fmt.Errorf("%s：ID %q 不在本包的命名空间 %q 内", file, id, m.Namespace)
	}
	return nil
}

func validTone(t string) bool {
	switch t {
	case "normal", "warning", "danger", "success":
		return true
	}
	return false
}

// validBind 报告 HUD 内置绑定是否合法。
func validBind(b string) bool {
	switch b {
	case "location", "time", "clock", "day", "period", "gold", "turn", "story", "objective", "conditions",
		"level", "xp", "hp", "sp", "mercury", "deviation", "mode", "anchor", "combat":
		return true
	}
	return strings.HasPrefix(b, "var:") || strings.HasPrefix(b, "flag:")
}

// validate 做引用完整性校验，并预编译 CEL。
func (p *Package) validate(ev *expression.Evaluator) error {
	var errs []error
	if _, ok := p.Locations[p.Manifest.Start.Location]; !ok {
		errs = append(errs, fmt.Errorf("起始地点 %q 不存在（manifest start.location）", p.Manifest.Start.Location))
	}
	compile := func(where, expr string) {
		if ev == nil || expr == "" {
			return
		}
		if _, err := ev.Compile(expr); err != nil {
			errs = append(errs, fmt.Errorf("%s：表达式错误：%w", where, err))
		}
	}
	for _, id := range p.LocationIDs {
		l := p.Locations[id]
		for _, e := range l.Exits {
			if _, ok := p.Locations[e.To]; !ok {
				errs = append(errs, fmt.Errorf("%s：出口指向不存在的地点 %q", id, e.To))
			}
			compile(id+" exit", e.Requires)
		}
		if l.Shop != nil {
			for _, it := range l.Shop.Items {
				if _, ok := p.Items[it]; !ok {
					errs = append(errs, fmt.Errorf("%s：商店物品 %q 不存在", id, it))
				}
			}
		}
	}
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		if _, ok := p.Locations[c.Location]; !ok {
			errs = append(errs, fmt.Errorf("%s：所在地点 %q 不存在", id, c.Location))
		}
		seen := map[string]bool{}
		for _, l := range c.Dialogue.Lines {
			switch {
			case l.ID == "":
				errs = append(errs, fmt.Errorf("%s：有台词缺少 id", id))
			case seen[l.ID]:
				errs = append(errs, fmt.Errorf("%s：台词 id %q 重复", id, l.ID))
			case strings.TrimSpace(l.Text) == "":
				errs = append(errs, fmt.Errorf("%s/%s：台词内容为空", id, l.ID))
			}
			seen[l.ID] = true
			compile(id+"/"+l.ID, l.When)
			for _, rv := range l.Reveals {
				from, to, ok := strings.Cut(rv, ">")
				_, fok := p.Characters[from]
				_, tok := p.Characters[to]
				if !ok || !fok || (!tok && to != PlayerID) {
					errs = append(errs, fmt.Errorf("%s/%s：reveals %q 应形如 角色ID>角色ID", id, l.ID, rv))
				}
			}
		}
	}
	for _, h := range p.HUD {
		if h.ID == "" || h.Label == "" {
			errs = append(errs, fmt.Errorf("HUD：每一项都需要 id 和 label（%s）", h.ID))
		}
		if (h.Bind == "") == (h.Value == "") {
			errs = append(errs, fmt.Errorf("HUD %s：bind 和 value 必须且只能填一个", h.ID))
		}
		if h.Bind != "" && !validBind(h.Bind) {
			errs = append(errs, fmt.Errorf("HUD %s：未知的 bind %q", h.ID, h.Bind))
		}
		compile("hud "+h.ID, h.Value)
		compile("hud "+h.ID+" visible", h.Visible)
		for tone, expr := range h.Tones {
			if !validTone(tone) {
				errs = append(errs, fmt.Errorf("HUD %s：未知的色调 %q", h.ID, tone))
			}
			compile("hud "+h.ID+" tone", expr)
		}
		if h.Tone != "" && !validTone(h.Tone) {
			errs = append(errs, fmt.Errorf("HUD %s：未知的色调 %q", h.ID, h.Tone))
		}
	}
	for i, o := range p.Objectives {
		if strings.TrimSpace(o.Text) == "" {
			errs = append(errs, fmt.Errorf("第 %d 个主线目标的 text 为空", i+1))
		}
		compile(fmt.Sprintf("objective #%d", i+1), o.When)
	}
	for _, id := range p.ActionIDs {
		d := p.Actions[id]
		for _, r := range d.Requirements {
			compile(id, r.Expr)
		}
		for _, c := range d.Checks {
			compile(id, c.Difficulty)
			if _, ok := p.SkillByID(c.Skill); !ok {
				errs = append(errs, fmt.Errorf("%s：未知技能 %q", id, c.Skill))
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
	errs = append(errs, p.validateRPG(compile)...)
	return errors.Join(errs...)
}
