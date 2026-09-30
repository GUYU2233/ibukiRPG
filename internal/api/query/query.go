package query

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/checks"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// Q 是只读查询层（Query API，第 47 节）。它只读取 State，不写任何东西。
type Q struct {
	Pkg  *loader.Package
	Eval *expression.Evaluator
}

func (q *Q) exitLocked(s *state.State, e loader.Exit) bool {
	if e.Requires == "" {
		return false
	}
	ok, err := q.Eval.EvalBool(e.Requires, engine.Vars(q.Pkg, s, "", ""))
	return err != nil || !ok
}

// ActiveStory 返回进行中的事件（在当前地点的优先）。
func (q *Q) ActiveStory(s *state.State) *loader.Story {
	for _, id := range q.Pkg.StoryIDs {
		if s.Stories[id].Status == state.StoryActive {
			return q.Pkg.Stories[id]
		}
	}
	return nil
}

// Scene 构造场景视图。
func (q *Q) Scene(s *state.State) dto.SceneV1 {
	p := q.Pkg
	loc := p.Locations[s.Player.Location]
	v := dto.SceneV1{
		LocationID: loc.ID, LocationName: loc.Name, Description: strings.TrimSpace(loc.Short),
		TimeText: worldtime.Format(s.Minute), Clock: worldtime.Clock(s.Minute), Day: worldtime.Day(s.Minute),
		Period: worldtime.Period(s.Minute), Turn: s.Turn, Gold: s.Player.Gold, PlayerName: s.Player.Name,
		Present: []dto.NPCBriefV1{}, Exits: []dto.ExitV1{}, Conditions: []string{},
		PackID: p.Manifest.ID, PackName: p.Manifest.Name,
	}
	v.Hud = q.Hud(s)
	for _, id := range s.NPCsAt(p, loc.ID) {
		c := p.Characters[id]
		v.Present = append(v.Present, dto.NPCBriefV1{ID: id, Name: c.Name(), Role: c.Identity.Role, Attitude: narrator.Attitude(s.NPCs[id])})
	}
	for _, e := range loc.Exits {
		v.Exits = append(v.Exits, dto.ExitV1{ID: e.To, Label: e.Label, Locked: q.exitLocked(s, e)})
	}
	v.Facts = append(append([]string{}, s.SceneFacts[loc.ID]...), s.Canon[loc.ID]...)
	for _, c := range s.Player.Conditions {
		v.Conditions = append(v.Conditions, p.ConditionName(c))
	}
	if st := q.ActiveStory(s); st != nil {
		b := &dto.StoryBriefV1{ID: st.ID, Title: st.Title, Hints: []string{}}
		for _, h := range st.Hints {
			b.Hints = append(b.Hints, h.Label)
		}
		v.Story = b
	}
	return v
}

// Suggestions 生成上下文相关的快捷建议（快速通道：点击直接构造 Command，不经过 Resolver）。
func (q *Q) Suggestions(s *state.State) []dto.SuggestionV1 {
	p := q.Pkg
	loc := p.Locations[s.Player.Location]
	present := s.NPCsAt(p, loc.ID)
	var out []dto.SuggestionV1
	add := func(sg dto.SuggestionV1) { out = append(out, sg) }
	story := q.ActiveStory(s)
	if story != nil && story.Location == loc.ID {
		for _, h := range story.Hints {
			add(dto.SuggestionV1{Label: h.Label, Icon: "auto_stories", Action: dto.QuickActionV1{Kind: "text", Text: h.Text, Label: h.Text}})
		}
	}
	if look := p.ActionID("look"); look != "" {
		add(dto.SuggestionV1{Label: "环顾四周", Icon: "visibility", Action: dto.QuickActionV1{Kind: "action", Action: look, Label: "环顾四周"}})
	}
	// 事件相关人物排在前面
	ordered := append([]string{}, present...)
	if story != nil {
		slices.SortStableFunc(ordered, func(a, b string) int {
			ra, rb := len(p.Characters[a].Dialogue.Story[story.ID]) > 0, len(p.Characters[b].Dialogue.Story[story.ID]) > 0
			switch {
			case ra && !rb:
				return -1
			case rb && !ra:
				return 1
			}
			return 0
		})
	}
	talk := p.DialogueAction()
	for i, id := range ordered {
		if i >= 3 || talk == "" {
			break
		}
		name := p.Characters[id].Name()
		add(dto.SuggestionV1{Label: "与" + name + "交谈", Icon: "chat", Action: dto.QuickActionV1{Kind: "action", Action: talk, Target: id, Label: "和" + name + "聊聊"}})
	}
	drink, keeper := p.DrinkItem(), p.ShopKeeper(loc.ID)
	if bartender, _ := loc.Properties["has_bartender"].(bool); bartender && drink != "" && s.Player.Inventory[drink] == 0 && keeper != "" && s.NPCs[keeper].Location == loc.ID {
		price := p.Items[drink].Price
		sg := dto.SuggestionV1{Label: "点酒", Icon: "local_bar", Hint: fmt.Sprintf("%d 铜币", price), Action: dto.QuickActionV1{Kind: "action", Action: p.ActionID("order_drink"), Label: "点一杯" + p.Items[drink].Name}}
		if s.Player.Gold < price {
			sg.Disabled, sg.Hint = true, "铜币不足"
		}
		add(sg)
	}
	for _, id := range p.ItemIDs {
		it := p.Items[id]
		if use := p.ActionID("use_item"); use != "" && s.Player.Inventory[id] > 0 && it.Use != nil && it.Use.Condition != "" && !s.HasCondition(it.Use.Condition) {
			add(dto.SuggestionV1{Label: it.Use.Verb + it.Name, Icon: "backpack", Action: dto.QuickActionV1{Kind: "action", Action: use, Item: id, Label: it.Use.Verb + it.Name}})
			break
		}
	}
	storyHere := story != nil && story.Location == loc.ID && len(story.Hints) > 0
	if invID := p.ActionID("investigate"); invID != "" && !storyHere {
		inv := dto.SuggestionV1{Label: "调查", Icon: "search", Action: dto.QuickActionV1{Kind: "action", Action: invID, Label: "仔细调查周围"}}
		if def := p.Actions[invID]; len(def.Checks) > 0 {
			if dc, err := q.Eval.EvalInt(def.Checks[0].Difficulty, engine.Vars(p, s, "", "")); err == nil {
				inv.Hint = fmt.Sprintf("成功率 %d%%", checks.Chance(q.modifier(s, def.Checks[0].Skill), checks.ClampDC(int(dc))))
			}
		}
		add(inv)
	}
	for _, e := range loc.Exits {
		sg := dto.SuggestionV1{Label: "前往" + e.Label, Icon: "directions_walk", Action: dto.QuickActionV1{Kind: "move", Destination: e.To, Label: "前往" + e.Label}}
		if q.exitLocked(s, e) {
			sg.Hint = "未获允许"
			sg.Icon = "lock"
		}
		add(sg)
	}
	if rest := p.ActionID("rest"); rest != "" {
		add(dto.SuggestionV1{Label: "休息片刻", Icon: "hourglass", Hint: "30 分钟", Action: dto.QuickActionV1{Kind: "action", Action: rest, Label: "找个地方歇一会儿"}})
	}
	return out
}

func (q *Q) modifier(s *state.State, skill string) int {
	sk, _ := q.Pkg.SkillByID(skill)
	extra := 0
	for _, c := range s.Player.Conditions {
		extra += q.Pkg.Rules.ConditionModifiers[c][skill]
	}
	return checks.Modifier(s.Player.Skills[skill], s.Player.Attributes[sk.Attribute], extra)
}

// Character 构造角色面板。
func (q *Q) Character(s *state.State) dto.CharacterV1 {
	p := q.Pkg
	v := dto.CharacterV1{Name: s.Player.Name, Role: p.Player.Identity.Role, Gold: s.Player.Gold, Attributes: []dto.StatV1{}, Skills: []dto.StatV1{}, Conditions: []dto.StatV1{}}
	for _, a := range []string{"strength", "agility", "intelligence", "charisma", "resolve"} {
		val := s.Player.Attributes[a]
		v.Attributes = append(v.Attributes, dto.StatV1{ID: a, Name: p.Rules.Attributes[a], Value: val, Modifier: checks.Modifier(0, val, 0)})
	}
	for _, sk := range p.Rules.Skills {
		note := ""
		base := checks.Modifier(s.Player.Skills[sk.ID], s.Player.Attributes[sk.Attribute], 0)
		mod := q.modifier(s, sk.ID)
		if mod != base {
			note = fmt.Sprintf("状态修正 %+d", mod-base)
		}
		v.Skills = append(v.Skills, dto.StatV1{ID: sk.ID, Name: sk.Name, Value: s.Player.Skills[sk.ID], Modifier: mod, Note: note})
	}
	for _, c := range s.Player.Conditions {
		var mods []string
		for _, sk := range p.Rules.Skills {
			if m := p.Rules.ConditionModifiers[c][sk.ID]; m != 0 {
				mods = append(mods, fmt.Sprintf("%s %+d", sk.Name, m))
			}
		}
		v.Conditions = append(v.Conditions, dto.StatV1{ID: c, Name: p.ConditionName(c), Note: strings.Join(mods, "，")})
	}
	return v
}

// Inventory 构造背包面板。
func (q *Q) Inventory(s *state.State) dto.InventoryV1 {
	p := q.Pkg
	v := dto.InventoryV1{Gold: s.Player.Gold, Items: []dto.ItemV1{}, Shop: []dto.ItemV1{}}
	for _, id := range p.ItemIDs {
		n := s.Player.Inventory[id]
		if n <= 0 {
			continue
		}
		it := p.Items[id]
		iv := dto.ItemV1{ID: id, Name: it.Name, Qty: n, Price: it.Price, Description: it.Description}
		if use := p.ActionID("use_item"); it.Use != nil && use != "" {
			iv.UseLabel = it.Use.Verb
			iv.Use = &dto.QuickActionV1{Kind: "action", Action: use, Item: id, Label: it.Use.Verb + it.Name}
		}
		v.Items = append(v.Items, iv)
	}
	loc := p.Locations[s.Player.Location]
	if loc.Shop != nil && s.NPCs[loc.Shop.Seller] != nil && s.NPCs[loc.Shop.Seller].Location == loc.ID {
		v.ShopSeller = p.EntityName(loc.Shop.Seller)
		for _, id := range loc.Shop.Items {
			it := p.Items[id]
			var act *dto.QuickActionV1
			if buy := p.ActionID("buy"); buy != "" {
				act = &dto.QuickActionV1{Kind: "action", Action: buy, Item: id, Label: "购买" + it.Name}
			}
			if id == p.DrinkItem() {
				act = &dto.QuickActionV1{Kind: "action", Action: p.ActionID("order_drink"), Label: "点一杯" + it.Name}
			}
			v.Shop = append(v.Shop, dto.ItemV1{ID: id, Name: it.Name, Price: it.Price, Description: it.Description, Affordable: s.Player.Gold >= it.Price, Buy: act})
		}
	}
	return v
}

var sourceNames = map[string]string{"witness": "亲眼所见", "dialogue": "听人说起", "rumor": "传闻"}

// npcMemories 把 NPC 的对话记忆与经历记忆整理成面板文字（新的在前，最多 6 条）。
func npcMemories(n *state.NPC) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(t string) {
		if t != "" && !seen[t] && len(out) < 6 {
			seen[t] = true
			out = append(out, t)
		}
	}
	for i := len(n.Exchanges) - 1; i >= 0 && i >= len(n.Exchanges)-3; i-- {
		e := n.Exchanges[i]
		add(worldtime.Clock(e.Minute) + " 交谈：" + e.Text)
	}
	for i := len(n.Episodes) - 1; i >= 0; i-- {
		add(n.Episodes[i].Text)
	}
	return out
}

// NPCs 构造人物关系面板：只列出玩家见过的人；包含 NPC 知道的事及其来源（可解释性）。
func (q *Q) NPCs(s *state.State) []dto.NPCV1 {
	p := q.Pkg
	out := []dto.NPCV1{}
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		n := s.NPCs[id]
		present := n.Location == s.Player.Location
		if !present && len(n.Memories) == 0 {
			continue
		}
		v := dto.NPCV1{ID: id, Name: c.Name(), Role: c.Identity.Role, Description: c.Description, LocationName: p.EntityName(n.Location),
			Present: present, Trust: n.Trust, Fear: n.Fear, Attitude: narrator.Attitude(n), Beliefs: []dto.BeliefV1{}, Actions: []dto.NPCActionV1{},
			Talks: n.Talks, Memories: npcMemories(n)}
		for i := len(n.Beliefs) - 1; i >= 0; i-- {
			b := n.Beliefs[i]
			v.Beliefs = append(v.Beliefs, dto.BeliefV1{Text: b.Text, Source: sourceNames[b.Source], When: worldtime.Format(b.Minute), Confidence: b.Confidence})
		}
		if present {
			for _, a := range []string{"talk", "persuade", "intimidate", "deceive"} {
				def := p.Actions[p.ActionID(a)]
				if a == "talk" {
					def = p.Actions[p.DialogueAction()]
				}
				if def == nil {
					continue
				}
				label := strings.ReplaceAll(def.Label, "{target}", "")
				if label == "" {
					label = def.Name
				}
				if a == "talk" {
					label = "交谈"
				}
				na := dto.NPCActionV1{Label: label, Chance: -1, Action: dto.QuickActionV1{Kind: "action", Action: def.ID, Target: id, Label: strings.ReplaceAll(def.Label, "{target}", c.Name())}}
				if a == "talk" {
					na.Action.Label = "和" + c.Name() + "聊聊"
				}
				if len(def.Checks) > 0 {
					if dc, err := q.Eval.EvalInt(def.Checks[0].Difficulty, engine.Vars(p, s, id, "")); err == nil {
						na.Chance = checks.Chance(q.modifier(s, def.Checks[0].Skill), checks.ClampDC(int(dc)))
					}
				}
				v.Actions = append(v.Actions, na)
			}
		}
		out = append(out, v)
	}
	return out
}

// Journal 把事件流翻译成可读日志（第 53 节 Event Timeline 的玩家版）。
func (q *Q) Journal(events []event.Event, playerName string) []dto.JournalEntryV1 {
	p := q.Pkg
	out := []dto.JournalEntryV1{}
	add := func(e event.Event, kind, text string) {
		out = append(out, dto.JournalEntryV1{Seq: e.Seq, Turn: e.Turn, Time: worldtime.Format(e.Minute), Kind: kind, Text: text})
	}
	for _, e := range events {
		d := e.Data
		switch e.Type {
		case event.SkillCheckResolved:
			r := checks.Resolve(d.Skill, p.SkillName(d.Skill), d.Roll, d.Modifier, d.DC)
			label := r.SkillName + "检定"
			if d.Target != "" {
				label += "（" + p.EntityName(d.Target) + "）"
			}
			add(e, "check", label+"："+checks.Explain(r))
		case event.GoldChanged:
			add(e, "gold", fmt.Sprintf("铜币 %+d", d.Delta))
		case event.ItemAdded:
			add(e, "item", fmt.Sprintf("获得 %s ×%d", p.EntityName(d.Item), d.Qty))
		case event.ItemRemoved:
			add(e, "item", fmt.Sprintf("失去 %s ×%d", p.EntityName(d.Item), d.Qty))
		case event.RelationshipChanged, event.RelationshipNudge:
			add(e, "relation", relText(p, d))
		case event.LocationChanged:
			add(e, "move", "来到 "+p.EntityName(d.To))
		case event.StoryStarted:
			add(e, "story", "事件开始：《"+d.Title+"》")
		case event.StoryStepReached:
			add(e, "story", "事件进展："+stepTitle(p, d.Story, d.Step))
		case event.StoryResolved:
			add(e, "story", "事件结局：《"+p.Stories[d.Story].Title+"》· "+d.Title)
		case event.ConditionApplied, event.MinorConditionApplied:
			add(e, "condition", "获得状态："+p.ConditionName(d.Condition))
		case event.ConditionRemoved:
			add(e, "condition", "状态消失："+p.ConditionName(d.Condition))
		case event.NPCMoved:
			add(e, "world", fmt.Sprintf("%s去了%s", p.EntityName(d.Target), p.EntityName(d.To)))
		case event.FlagSet:
			if d.Flag == "backroom_allowed" {
				add(e, "world", "伯林允许你进入储藏室")
			}
		}
	}
	return out
}

func stepTitle(p *loader.Package, story, step string) string {
	if st, ok := p.Stories[story]; ok {
		for _, x := range st.Steps {
			if x.ID == step && x.Title != "" {
				return x.Title
			}
		}
	}
	return step
}

func relText(p *loader.Package, d event.Data) string {
	var parts []string
	for _, k := range []string{"trust", "fear"} {
		if v := d.Values[k]; v != 0 {
			parts = append(parts, fmt.Sprintf("%s %+d", map[string]string{"trust": "信任", "fear": "畏惧"}[k], v))
		}
	}
	return p.EntityName(d.Target) + " " + strings.Join(parts, "，")
}

// TurnEntries 把本回合结果翻译成对话记录条目（叙事除外）。
func (q *Q) TurnEntries(before, after *state.State, res *engine.Result, input string) []dto.EntryV1 {
	p := q.Pkg
	entries := []dto.EntryV1{{Kind: "player", Text: input, Turn: res.Turn}}
	var chips []string
	var stories []dto.EntryV1
	timeSpent := after.Minute - before.Minute
	for _, e := range res.Events {
		d := e.Data
		switch e.Type {
		case event.SkillCheckResolved:
			r := checks.Resolve(d.Skill, p.SkillName(d.Skill), d.Roll, d.Modifier, d.DC)
			label := r.SkillName
			if d.Target != "" {
				label += " · " + p.EntityName(d.Target)
			}
			entries = append(entries, dto.EntryV1{Kind: "check", Turn: res.Turn, Text: checks.Explain(r), Check: &dto.CheckV1{
				Label: label, SkillName: r.SkillName, Roll: r.Roll, Modifier: r.Modifier, DC: r.DC, Total: r.Total,
				Success: r.Success, Critical: r.Critical, Fumble: r.Fumble, Explanation: checks.Explain(r)}})
		case event.GoldChanged:
			chips = append(chips, fmt.Sprintf("铜币 %+d", d.Delta))
		case event.ItemAdded:
			chips = append(chips, "获得 "+p.EntityName(d.Item))
		case event.ItemRemoved:
			chips = append(chips, "失去 "+p.EntityName(d.Item))
		case event.RelationshipChanged, event.RelationshipNudge:
			chips = append(chips, relText(p, d))
		case event.ConditionApplied, event.MinorConditionApplied:
			chips = append(chips, "状态："+p.ConditionName(d.Condition))
		case event.ConditionRemoved:
			chips = append(chips, p.ConditionName(d.Condition)+"消退")
		case event.NoiseGenerated:
			chips = append(chips, "引起了骚动")
		case event.StoryStarted:
			stories = append(stories, dto.EntryV1{Kind: "story", Turn: res.Turn, Text: "事件开始：《" + d.Title + "》"})
		case event.StoryStepReached:
			stories = append(stories, dto.EntryV1{Kind: "story", Turn: res.Turn, Text: "事件进展：" + stepTitle(p, d.Story, d.Step)})
		case event.StoryResolved:
			stories = append(stories, dto.EntryV1{Kind: "story", Turn: res.Turn, Text: "事件结局：" + d.Title})
		}
	}
	if timeSpent >= 10 {
		chips = append(chips, fmt.Sprintf("时间 +%d 分钟", timeSpent))
	}
	if len(chips) > 0 {
		entries = append(entries, dto.EntryV1{Kind: "system", Turn: res.Turn, Text: strings.Join(chips, " · "), Chips: chips})
	}
	return append(entries, stories...)
}

// KindRank 决定同一命令内条目的展示顺序。
func KindRank(kind string) int {
	switch kind {
	case "intro":
		return 0
	case "player":
		return 1
	case "check":
		return 2
	case "narration":
		return 3
	case "system":
		return 4
	case "story":
		return 5
	}
	return 6
}
