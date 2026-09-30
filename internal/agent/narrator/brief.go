package narrator

import (
	"context"

	"fmt"
	"hash/fnv"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"

	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/narrative/guard"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/checks"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// NPCBrief 是 Narrator 可见的在场人物信息（NarratorScope：不含秘密）。
type NPCBrief struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description"`
	Attitude    string `json:"attitude"`
}

// Brief 是叙事输入：只包含玩家可观察的事实与本回合已结算的事件（第 17 节）。
type Brief struct {
	CommandID  string          `json:"command_id"`
	PlayerName string          `json:"player_name"`
	Input      string          `json:"input"`
	Location   string          `json:"location"`
	Time       string          `json:"time"`
	Present    []NPCBrief      `json:"present"`
	SceneFacts []string        `json:"scene_facts"`
	Facts      guard.Facts     `json:"immutable_facts"`
	Base       string          `json:"base"` // 模板事实稿，也是降级时的最终叙事
	Guard      guard.Context   `json:"-"`
	Checks     []checks.Result `json:"-"`
	// Memories 是本回合涉及的 NPC 的记忆（NPCScope：只含该 NPC 自己经历、听到、看到的事，不含秘密）。
	Memories []NPCMemory `json:"npc_memory,omitempty"`

	// StorySoFar 是记忆 Agent 生成的 [STORY_SO_FAR] 段（早期回合的摘要）；为空时不出现在提示词里。
	StorySoFar string `json:"-"`
	// Lookup 是需要先检索再叙述的原因（自由推演 / 未知名词 / 回忆被压缩的内容）；为空时不检索。
	Lookup []string `json:"-"`
	// Retrieval 是 [TOOLS] + [RETRIEVAL_POLICY] 段（故事包可修补）。
	Retrieval string `json:"-"`
	// Research 执行只读检索（PlayerScope，由编排器注入）：返回模型直接给出的回答，或带检索结果的新消息。
	Research Research `json:"-"`
}

// Known 返回本回合上下文里已经出现的文字（用于判断玩家提到的名词是否“未知”）。
func (b Brief) Known() []string {
	out := []string{b.Location, b.Base, strings.Join(b.SceneFacts, "；"), b.StorySoFar}
	for _, p := range b.Present {
		out = append(out, p.Name+p.Role+p.Description)
	}
	for _, m := range b.Memories {
		out = append(out, m.Name+strings.Join(m.Exchanges, "")+strings.Join(m.Episodes, ""))
	}
	return out
}

// Research 是检索回调：answer 非空表示模型检索后已直接写好叙事；否则用 msgs 再生成一次。
type Research func(ctx context.Context, p provider.Provider, msgs []provider.Message, hint string) (answer string, out []provider.Message)

// NPCMemory 是 AI 上下文中某个 NPC 的记忆摘要。
type NPCMemory struct {
	Name      string   `json:"name"`
	Attitude  string   `json:"attitude"`
	Talks     int      `json:"talks"`
	SinceLast string   `json:"since_last,omitempty"` // 距上次交谈，例如“12 分钟前”
	Exchanges []string `json:"exchanges,omitempty"`  // 最近的交谈摘要（旧 → 新）
	Topics    []string `json:"topics,omitempty"`
	Episodes  []string `json:"episodes,omitempty"` // 关键情节记忆（重要的在前）
	Seen      []string `json:"seen,omitempty"`     // 自上次交谈后看到的玩家行为
	Intent    string   `json:"intent,omitempty"`   // 本回合台词要表达的意思（引擎挑选的台词摘要）
	Line      string   `json:"line,omitempty"`     // 模板台词原文（供 AI 改写）
	Repeat    bool     `json:"repeat,omitempty"`
}

// MaxMemoryItems 限制每类记忆写入 AI 上下文的条数（本地小模型上下文有限）。
const MaxMemoryItems = 4

// BuildMemory 为 NPC 构造记忆摘要（使用本回合开始前的状态：NPC 在开口前记得什么）。
func BuildMemory(p *loader.Package, before *state.State, id string, dlg *event.Data) NPCMemory {
	c := p.Characters[id]
	n := before.NPCs[id]
	m := NPCMemory{Name: c.Name(), Attitude: n.Attitude(), Talks: n.Talks}
	if n.Talks > 0 {
		m.SinceLast = fmt.Sprintf("%d 分钟前", before.Minute-n.LastTalkMinute)
	}
	ex := n.Exchanges
	if len(ex) > MaxMemoryItems {
		ex = ex[len(ex)-MaxMemoryItems:]
	}
	for _, e := range ex {
		m.Exchanges = append(m.Exchanges, fmt.Sprintf("%s（%s）", e.Text, worldtime.Clock(e.Minute)))
	}
	var topics []string
	for t := range n.Topics {
		topics = append(topics, c.Dialogue.TopicName(t))
	}
	slices.Sort(topics)
	m.Topics = topics
	eps := slices.Clone(n.Episodes)
	slices.SortStableFunc(eps, func(a, b state.Episode) int {
		if a.Importance != b.Importance {
			return b.Importance - a.Importance
		}
		return b.Turn - a.Turn
	})
	for i, e := range eps {
		if i >= MaxMemoryItems {
			break
		}
		m.Episodes = append(m.Episodes, e.Text)
	}
	if what, ok := recentDeed(p, before, id); ok {
		m.Seen = append(m.Seen, "你"+what)
	}
	if dlg != nil && dlg.Target == id {
		m.Intent, m.Repeat = dlg.Text, dlg.Repeat
		if l := c.Dialogue.Line(dlg.Step); l != nil {
			m.Line = engine.FillLine(l.Text, c, before.Player.Name)
		}
	}
	return m
}

// Attitude 把关系数值翻译成文字。
func Attitude(n *state.NPC) string { return n.Attitude() }

func pick(seed string, salt string, options []string) string {
	if len(options) == 0 {
		return ""
	}
	h := fnv.New32a()
	h.Write([]byte(seed + "|" + salt))
	return options[int(h.Sum32()%uint32(len(options)))] //nolint:gosec // 小整数取模
}

// Build 根据执行前后的状态与本回合事件构造 Brief（含模板事实稿 Base）。
func Build(p *loader.Package, before, after *state.State, cmd command.Command, res *engine.Result) Brief {
	b := Brief{
		CommandID:  cmd.ID,
		PlayerName: after.Player.Name,
		Input:      cmd.Input,
		Location:   p.EntityName(after.Player.Location),
		Time:       worldtime.Format(after.Minute),
		SceneFacts: append([]string{}, after.SceneFacts[after.Player.Location]...),
	}
	b.SceneFacts = append(b.SceneFacts, after.Canon[after.Player.Location]...)
	allowed := map[string]bool{}
	for _, id := range after.NPCsAt(p, after.Player.Location) {
		c := p.Characters[id]
		b.Present = append(b.Present, NPCBrief{Name: c.Name(), Role: c.Identity.Role, Description: c.Description, Attitude: Attitude(after.NPCs[id])})
		allowed[c.Name()] = true
	}
	for _, id := range before.NPCsAt(p, before.Player.Location) {
		allowed[p.Characters[id].Name()] = true
	}
	f := guard.Facts{PlayerLocation: b.Location, Time: b.Time, Outcome: "none", Gold: after.Player.Gold, Deaths: []string{}}
	for _, e := range res.Events {
		d := e.Data
		switch e.Type {
		case event.SkillCheckResolved:
			r := checks.Resolve(d.Skill, p.SkillName(d.Skill), d.Roll, d.Modifier, d.DC)
			b.Checks = append(b.Checks, r)
			f.Checks = append(f.Checks, checks.Explain(r))
		case event.ActionPerformed, event.FreeformPerformed:
			if len(b.Checks) > 0 {
				f.Outcome = map[bool]string{true: "success", false: "failure"}[d.Success]
			}
			if d.Target != "" {
				allowed[p.EntityName(d.Target)] = true
			}
		case event.GoldChanged:
			f.GoldDelta += d.Delta
		case event.ItemAdded:
			f.ItemsAdded = append(f.ItemsAdded, p.EntityName(d.Item))
		case event.ItemRemoved:
			f.ItemsRemoved = append(f.ItemsRemoved, p.EntityName(d.Item))
		case event.RelationshipChanged, event.RelationshipNudge:
			for _, k := range []string{"trust", "fear"} {
				if v := d.Values[k]; v != 0 {
					f.Relationships = append(f.Relationships, fmt.Sprintf("%s %s %+d", p.EntityName(d.Target), map[string]string{"trust": "信任", "fear": "畏惧"}[k], v))
				}
			}
			allowed[p.EntityName(d.Target)] = true
		case event.NPCMoved:
			allowed[p.EntityName(d.Target)] = true
		case event.StoryStarted:
			f.StoryEvents = append(f.StoryEvents, "事件开始："+d.Title)
		case event.StoryResolved:
			f.StoryEvents = append(f.StoryEvents, "事件结局："+d.Title)
		case event.CombatActed:
			if t := ActText(p, func(id string) string { return UnitName(before, after, id) }, d); t != "" {
				f.StoryEvents = append(f.StoryEvents, "战斗："+t)
			}
		case event.CombatEnded:
			f.StoryEvents = append(f.StoryEvents, "战斗结束："+map[string]string{"victory": "胜利", "defeat": "战败", "fled": "撤退"}[d.Outcome])
		case event.UnitDefeated:
			f.StoryEvents = append(f.StoryEvents, UnitName(before, after, d.Target)+"倒下（未死亡）")
		}
	}
	for _, pr := range b.Present {
		f.PresentNPCs = append(f.PresentNPCs, pr.Name)
	}
	b.Facts = f
	b.Base = compose(p, before, after, cmd, res, b)
	// NPC 记忆：本回合交谈 / 被针对的 NPC（只在其在场时）。
	var dlg *event.Data
	var involved []string
	for i := range res.Events {
		e := res.Events[i]
		switch e.Type {
		case event.DialogueOccurred:
			d := e.Data
			dlg = &d
			involved = append(involved, d.Target)
		case event.ActionPerformed, event.FreeformPerformed:
			if _, ok := p.Characters[e.Data.Target]; ok {
				involved = append(involved, e.Data.Target)
			}
		}
	}
	for _, id := range involved {
		if slices.ContainsFunc(b.Memories, func(m NPCMemory) bool { return m.Name == p.Characters[id].Name() }) {
			continue
		}
		if before.NPCs[id] != nil {
			b.Memories = append(b.Memories, BuildMemory(p, before, id, dlg))
		}
	}
	// Guard 上下文
	gc := guard.Context{Base: b.Base}
	for n := range allowed {
		gc.AllowedNames = append(gc.AllowedNames, n)
	}
	slices.Sort(gc.AllowedNames)
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		if !allowed[c.Name()] {
			gc.AbsentNames = append(gc.AbsentNames, c.Name())
		}
		for _, s := range c.Secrets {
			gc.SecretKeywords = append(gc.SecretKeywords, s.Keywords...)
		}
	}
	for _, id := range p.LocationIDs {
		if id != after.Player.Location && id != before.Player.Location {
			gc.OtherLocations = append(gc.OtherLocations, p.Locations[id].Name)
		}
	}
	for _, id := range p.ItemIDs {
		if !slices.Contains(f.ItemsAdded, p.Items[id].Name) {
			gc.OtherItems = append(gc.OtherItems, p.Items[id].Name)
		}
	}
	b.Guard = gc
	return b
}

// ---------- 模板叙事 ----------

var freeformLines = map[string]map[string][]string{
	"performance": {
		"success": {"你%s。一曲终了，有人吹了声口哨，还有人跟着拍起了桌子。", "你%s。周围渐渐安静下来，连擦杯子的声音都停了，末了响起一阵稀稀拉拉却真诚的掌声。"},
		"failure": {"你%s，可惜跑了调，角落里传来几声压低的窃笑。", "你%s，却在最关键的地方卡了壳，只好尴尬地清了清嗓子。"},
	},
	"noise": {
		"success": {"你%s。“砰”的一声，整个%s都安静了一瞬，所有人都朝你看了过来。"},
	},
	"stealth": {
		"success": {"你%s，动作干净利落，没有人注意到。", "你%s。四下里人声嘈杂，谁也没留意你的小动作。"},
		"failure": {"你%s，却不小心弄出了声响——有人警觉地看了过来。", "你%s，可惜动作太僵硬，立刻引来了狐疑的目光。"},
	},
	"social": {
		"success": {"你%s。对方愣了一下，随即笑了，气氛轻松了不少。", "你%s，对方的神情明显柔和了下来。"},
		"failure": {"你%s，气氛却变得有点尴尬，对方别过了脸。", "你%s，换来的只是一个客气而疏远的点头。"},
	},
	"observe": {
		"success": {"你%s。%s", "你%s，渐渐留意到一些之前忽略的细节。%s"},
		"failure": {"你%s，但什么特别的也没注意到。", "你%s，可周围太嘈杂，你什么也没听清。"},
	},
	"help": {
		"success": {"你%s。旁边有人看了你一眼，点点头：“手脚挺麻利。”", "你%s，忙完时出了一身薄汗，心里倒挺踏实。"},
		"failure": {"你%s，结果笨手笨脚地碰倒了一摞杯子。", "你%s，却越帮越忙，只好讪讪地退到一边。"},
	},
	"misc": {
		"success": {"你%s。", "你%s，片刻之后，一切又恢复如常。"},
	},
}

func compose(p *loader.Package, before, after *state.State, cmd command.Command, res *engine.Result, b Brief) string {
	var parts []string
	seed := cmd.ID
	loc := p.Locations[after.Player.Location]
	var act, dlg *event.Data
	for i := range res.Events {
		e := res.Events[i]
		switch e.Type {
		case event.ActionPerformed, event.FreeformPerformed:
			d := e.Data
			act = &d
		case event.DialogueOccurred:
			d := e.Data
			dlg = &d
		}
	}
	moved := before.Player.Location != after.Player.Location
	switch {
	case cmd.Kind == command.KindMove:
		parts = append(parts, arrival(p, after, loc))
	case act != nil && act.Action != "":
		parts = append(parts, actionText(p, before, after, seed, *act, dlg, p.Locations[before.Player.Location]))
	case act != nil:
		parts = append(parts, freeformText(p, after, seed, *act, cmd, loc))
	}
	for _, e := range res.Events {
		d := e.Data
		switch e.Type {
		case event.StoryStepReached:
			if st := findStep(p, d.Story, d.Step, false); st != nil && st.Narration != "" {
				parts = append(parts, strings.TrimSpace(st.Narration))
			}
		case event.StoryResolved:
			if st := findStep(p, d.Story, d.Outcome, true); st != nil && st.Narration != "" {
				parts = append(parts, strings.TrimSpace(st.Narration))
			}
		case event.StoryStarted:
			parts = append(parts, strings.TrimSpace(p.Stories[d.Story].Intro))
		case event.AmbientEvent:
			parts = append(parts, d.Text)
		}
	}
	parts = append(parts, rpgParts(p, before, after, cmd, res)...)
	if moved && cmd.Kind != command.KindMove {
		parts = append(parts, arrival(p, after, loc))
	}
	out := strings.Join(nonEmpty(parts), "\n\n")
	if out == "" {
		out = "时间悄悄流过。"
	}
	return out
}

func nonEmpty(in []string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func findStep(p *loader.Package, story, id string, outcome bool) *loader.StoryStep {
	st, ok := p.Stories[story]
	if !ok {
		return nil
	}
	list := st.Steps
	if outcome {
		list = st.Outcomes
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

func presentLine(p *loader.Package, s *state.State, loc string) string {
	ids := s.NPCsAt(p, loc)
	if len(ids) == 0 {
		return "这里一个人也没有。"
	}
	var ps []string
	for _, id := range ids {
		c := p.Characters[id]
		ps = append(ps, fmt.Sprintf("%s（%s）", c.Name(), c.Identity.Role))
	}
	return "这里有：" + strings.Join(ps, "、") + "。"
}

func arrival(p *loader.Package, s *state.State, loc *loader.Location) string {
	lines := []string{fmt.Sprintf("你来到%s。%s", loc.Name, strings.TrimSpace(loc.Short))}
	lines = append(lines, presentLine(p, s, loc.ID))
	if c := s.Canon[loc.ID]; len(c) > 0 {
		lines = append(lines, strings.Join(c, "；")+"。")
	}
	return strings.Join(lines, "")
}

func exitsLine(p *loader.Package, loc *loader.Location) string {
	var ex []string
	for _, e := range loc.Exits {
		ex = append(ex, e.Label)
	}
	if len(ex) == 0 {
		return ""
	}
	return "从这里可以去：" + strings.Join(ex, "、") + "。"
}

func actionText(p *loader.Package, before, after *state.State, seed string, d event.Data, dlg *event.Data, loc *loader.Location) string {
	def := p.Actions[d.Action]
	target := p.EntityName(d.Target)
	fill := func(s string) string {
		return strings.NewReplacer("{target}", target, "{item}", p.EntityName(d.Item), "{actor}", "你").Replace(s)
	}
	key := loader.Key(d.Action)
	if def != nil && def.Dialogue {
		key = "talk"
	}
	switch key {
	case "look":
		if d.Target != "" {
			c := p.Characters[d.Target]
			return fmt.Sprintf("你打量着%s。%s看起来对你%s。", c.Name(), c.Description, attitudeWord(after.NPCs[d.Target]))
		}
		facts := append(append([]string{}, after.SceneFacts[loc.ID]...), after.Canon[loc.ID]...)
		text := strings.TrimSpace(loc.Description)
		if len(facts) > 0 {
			text += "你注意到：" + strings.Join(facts, "；") + "。"
		}
		return text + presentLine(p, after, loc.ID) + exitsLine(p, loc)
	case "talk":
		return talkLine(p, before, after, seed, d.Target, dlg)
	case "use_item":
		if it, ok := p.Items[d.Item]; ok && it.Use != nil && it.Use.Text != "" {
			return it.Use.Text
		}
		return fmt.Sprintf("你用了%s。", p.EntityName(d.Item))
	case "investigate":
		if d.Success {
			return loc.Search.Success
		}
		return loc.Search.Failure
	}
	lines := def.Narration[d.Outcome]
	if len(lines) == 0 {
		lines = def.Narration["success"]
	}
	if len(lines) == 0 {
		return fmt.Sprintf("你%s%s。", def.Name, target)
	}
	return fill(pick(seed, d.Action, lines))
}

func attitudeWord(n *state.NPC) string {
	switch Attitude(n) {
	case "友好", "亲近":
		return "颇有好感"
	case "畏惧":
		return "有些害怕"
	case "敌视", "戒备":
		return "心存戒备"
	}
	return "还算客气"
}

// talkLine 渲染本回合的交谈：台词由引擎按对话记忆挑选（DialogueOccurred），这里只负责把它讲出来。
// 同一句话再次被挑中（repeat）时，NPC 会提起“刚才说过”；自上次交谈后目击到的玩家行为会被顺带提起。
func talkLine(p *loader.Package, before, after *state.State, seed, id string, dlg *event.Data) string {
	c := p.Characters[id]
	nb := before.NPCs[id]
	if dlg == nil || dlg.Step == "" {
		return fmt.Sprintf("%s和你随便聊了几句。", c.Name())
	}
	line := c.Dialogue.Line(dlg.Step)
	if line == nil {
		return fmt.Sprintf("%s和你随便聊了几句。", c.Name())
	}
	lastSaid, lastTopic := "", "刚才的事"
	if ex := nb.LastExchange(); ex != nil {
		lastSaid = ex.Text
		if ex.Topic != "" {
			lastTopic = c.Dialogue.TopicName(ex.Topic)
		}
	}
	what, hasWhat := recentDeed(p, before, id)
	fill := func(t string) string {
		return strings.NewReplacer(
			"{player}", after.Player.Name, "{name}", c.Name(), "{last_said}", lastSaid, "{last_topic}", lastTopic,
			"{said}", dlg.Text, "{what}", what, "{talks}", fmt.Sprint(nb.Talks), "{topic}", topicOf(c, line),
		).Replace(t)
	}
	var text string
	if dlg.Repeat {
		tpls := c.Dialogue.Repeat
		if len(tpls) == 0 {
			tpls = defaultRepeat
		}
		text = fill(pick(seed, id+"|repeat", tpls))
	} else {
		text = fill(line.Text)
	}
	// 台词本身已经在回应“看到了什么”（引用 {what} 或以 npc.seen 为条件）时，不再追加。
	if hasWhat && !strings.Contains(line.Text, "{what}") && !strings.Contains(line.When, "npc.seen") {
		tpls := c.Dialogue.Callback
		if len(tpls) == 0 {
			tpls = defaultCallback
		}
		text += "\n\n" + fill(pick(seed, id+"|callback", tpls))
	}
	return text
}

func topicOf(c *loader.Character, l *loader.DialogueLine) string {
	if l.Topic == "" {
		return "这件事"
	}
	return c.Dialogue.TopicName(l.Topic)
}

var defaultRepeat = []string{
	"{name}摆摆手：“{player}，{topic}的事我刚才就跟你说过了，别的我也不知道更多了。”",
	"{name}看了你一眼：“还想听一遍{topic}？刚才说的就是全部了。”",
}

var defaultCallback = []string{"{name}又补了一句：“刚才你{what}，我可都看见了。”"}

// recentDeed 返回 NPC 自上次与玩家交谈以来亲眼看到的、玩家做的一件事（最近的一件）。
// 第一次交谈时只提显眼的事。不含针对该 NPC 本人的行为与普通交谈、进出门。
func recentDeed(p *loader.Package, s *state.State, id string) (string, bool) {
	n := s.NPCs[id]
	talk := p.DialogueAction()
	for i := len(n.Memories) - 1; i >= 0; i-- {
		m := n.Memories[i]
		if n.Talks > 0 && m.Turn <= n.LastTalkTurn {
			break
		}
		if m.Action == "" || m.Target == id || m.Action == talk || m.Action == p.ActionID("look") {
			continue
		}
		if n.Talks == 0 && !m.Notable {
			continue
		}
		what := m.Text
		if j := strings.Index(what, "（"); j > 0 {
			what = what[:j]
		}
		return strings.TrimPrefix(what, s.Player.Name), true
	}
	return "", false
}

func freeformText(p *loader.Package, s *state.State, seed string, d event.Data, cmd command.Command, loc *loader.Location) string {
	tag := "misc"
	if len(d.Tags) > 0 {
		if _, ok := freeformLines[d.Tags[0]]; ok {
			tag = d.Tags[0]
		}
	}
	lines := freeformLines[tag][d.Outcome]
	if len(lines) == 0 {
		lines = freeformLines[tag]["success"]
	}
	tpl := pick(seed, tag, lines)
	desc := strings.TrimSuffix(d.Text, "。")
	var text string
	switch strings.Count(tpl, "%s") {
	case 2:
		extra := loc.Name
		if tag == "observe" {
			extra = strings.TrimSpace(loc.Short)
		}
		text = fmt.Sprintf(tpl, desc, extra)
	default:
		text = fmt.Sprintf(tpl, desc)
	}
	if tag == "misc" {
		if ids := s.NPCsAt(p, loc.ID); len(ids) > 0 {
			who := p.Characters[ids[int(fnvN(seed, len(ids)))]].Name()
			text += pick(seed, "react", []string{who + "看了你一眼，没说什么。", who + "挑了挑眉，似乎觉得有点意思。", "没有人特别在意。"})
		}
	}
	return text
}

func fnvN(seed string, n int) uint32 {
	h := fnv.New32a()
	h.Write([]byte(seed))
	return h.Sum32() % uint32(n) //nolint:gosec // 小整数
}
