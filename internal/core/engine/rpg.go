package engine

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/combat"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/rules/rng"
	"github.com/GUYU2233/ibukiRPG/internal/story/director"
)

// ---------- 成长与装备 ----------

// ThresholdPresets 是主线敏感度预设（设置项）：mild / heavy。
var ThresholdPresets = map[string][2]int{"relaxed": {50, 85}, "strict": {25, 55}}

func (w *work) execManage() error {
	p := w.pkg()
	r := w.s.R()
	w.passive = true
	switch w.cmd.Action {
	case "equip":
		it, ok := p.Items[w.cmd.Item]
		if !ok || it.Slot == "" {
			return reject("这件东西不能装备。")
		}
		if r.Combat != nil {
			return reject("战斗中无法更换装备。")
		}
		if w.s.Player.Inventory[it.ID] <= 0 {
			return reject("你身上没有%s。", it.Name)
		}
		if it.Level > w.s.PlayerLevel(p) {
			return reject("需要等级 %d 才能装备%s。", it.Level, it.Name)
		}
		if r.Equipment[it.Slot] == it.ID {
			return reject("%s已经装备着了。", it.Name)
		}
		return w.withHPKept(func() error {
			return w.emit(event.ItemEquipped, event.Data{Item: it.ID, Key: it.Slot})
		})
	case "unequip":
		slot := w.cmd.Target
		if slot == "" {
			if it, ok := p.Items[w.cmd.Item]; ok {
				slot = it.Slot
			}
		}
		if r.Combat != nil {
			return reject("战斗中无法更换装备。")
		}
		if _, ok := r.Equipment[slot]; !ok {
			return reject("这个槽位没有装备。")
		}
		return w.withHPKept(func() error {
			return w.emit(event.ItemUnequipped, event.Data{Key: slot, Item: r.Equipment[slot]})
		})
	case "allocate":
		attr := w.cmd.Target
		if _, ok := p.Rules.Attributes[attr]; !ok {
			return reject("没有这项属性。")
		}
		if r.AttrPoints <= 0 {
			return reject("没有可分配的属性点。")
		}
		return w.emit(event.AttributeAllocated, event.Data{Key: attr, Delta: 1})
	case "learn":
		sk, ok := p.Combat.Skills[w.cmd.Skill]
		if !ok || sk.Learn == nil {
			return reject("这个技能无法通过修习获得。")
		}
		if slices.Contains(r.Skills, sk.ID) {
			return reject("你已经掌握了%s。", sk.Name)
		}
		if why := LearnBlocked(p, w.s, sk); why != "" {
			return reject("%s", why)
		}
		return w.emit(event.SkillLearned, event.Data{Skill: sk.ID, Qty: sk.Learn.Cost})
	case "use":
		it, ok := p.Items[w.cmd.Item]
		if !ok || it.Combat == nil || !it.Combat.Field {
			return reject("这件东西现在用不上。")
		}
		if w.s.Player.Inventory[it.ID] <= 0 {
			return reject("你身上没有%s。", it.Name)
		}
		if r.Combat != nil {
			return reject("战斗中请使用战斗面板里的“物品”。")
		}
		hp, mx := PlayerHP(p, w.s)
		heal := min(it.Combat.Heal+mx*it.Combat.HealPct/100, mx-hp)
		merc := min(it.Combat.Mercury, p.Combat.Config.MercuryMax-r.Mercury)
		if heal <= 0 && merc <= 0 {
			return reject("现在用%s没有效果。", it.Name)
		}
		if err := w.emit(event.ItemRemoved, event.Data{Item: it.ID, Qty: 1}); err != nil {
			return err
		}
		return w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"hp": max(heal, 0), "mercury": max(merc, 0)}, Reason: it.ID})
	case "mech_install", "mech_remove":
		w.passive = true
		return w.execMechManage()
	case "thresholds":
		v := map[string]int{"mild": 0, "heavy": 0}
		if pr, ok := ThresholdPresets[w.cmd.Target]; ok {
			v["mild"], v["heavy"] = pr[0], pr[1]
		} else if w.cmd.Target != "standard" {
			return reject("未知的主线敏感度。")
		}
		return w.emit(event.MainlineThresholds, event.Data{Values: v})
	}
	return reject("未知的操作。")
}

// withHPKept 在更换装备时保持当前生命不超过新的上限。
func (w *work) withHPKept(f func() error) error {
	if err := f(); err != nil {
		return err
	}
	hp, mx := PlayerHP(w.pkg(), w.s)
	if hp <= 0 && mx > 0 {
		return w.emit(event.PlayerVitals, event.Data{Values: map[string]int{"hp": 1}})
	}
	return nil
}

// LearnBlocked 返回技能不能学习的原因（空串表示可以学习）。
func LearnBlocked(p *loader.Package, s *state.State, sk *combat.SkillDef) string {
	r := s.R()
	if sk.Learn == nil {
		return "无法修习"
	}
	if lv := s.PlayerLevel(p); sk.Learn.Level > lv {
		return fmt.Sprintf("需要等级 %d", sk.Learn.Level)
	}
	for _, need := range sk.Learn.Requires {
		if !slices.Contains(r.Skills, need) {
			return "需要先掌握" + p.Combat.Skills[need].Name
		}
	}
	if r.SkillPoints < sk.Learn.Cost {
		return fmt.Sprintf("需要 %d 技能点", sk.Learn.Cost)
	}
	return ""
}

// ---------- 主线：偏离提示的选择 ----------

// Thresholds 返回当前生效的偏离阈值。
func Thresholds(p *loader.Package, s *state.State) (int, int) {
	mild, heavy := p.Mainline.Thresholds.Mild, p.Mainline.Thresholds.Heavy
	if s.RPG != nil && s.RPG.Main.Mild > 0 && s.RPG.Main.Heavy > 0 {
		mild, heavy = s.RPG.Main.Mild, s.RPG.Main.Heavy
	}
	return mild, heavy
}

// CurrentAnchor 返回当前主线锚点（全部完成或未声明时 nil）。
func CurrentAnchor(p *loader.Package, s *state.State) *loader.Anchor {
	idx := 0
	if s.RPG != nil {
		idx = s.RPG.Main.Anchor
	}
	if idx < len(p.Mainline.Anchors) {
		return &p.Mainline.Anchors[idx]
	}
	return nil
}

func (w *work) execMainline() error {
	p := w.pkg()
	m := &w.s.R().Main
	w.passive = true
	if !m.Pending {
		return reject("现在没有需要做的选择。")
	}
	mild, _ := Thresholds(p, w.s)
	switch w.cmd.Action {
	case "return":
		if err := w.emit(event.MainlineModeChanged, event.Data{From: state.ModeMain, To: state.ModeMain, Total: max(mild-10, 0), Reason: "return"}); err != nil {
			return err
		}
		a := CurrentAnchor(p, w.s)
		if a == nil {
			return nil
		}
		text := a.ReturnText
		if text == "" {
			text = "你收回心神，把注意力放回眼前的主线：" + a.Objective
		}
		if err := w.emit(event.MainlineNudged, event.Data{Story: a.ID, Text: text, Reason: "return"}); err != nil {
			return err
		}
		for _, o := range a.Return {
			if err := w.applyOutcome(o, "", ""); err != nil {
				return err
			}
		}
		return nil
	case "free":
		to := w.cmd.Target
		if to != state.ModeFree {
			to = state.ModeSandbox
		}
		return w.emit(event.MainlineModeChanged, event.Data{From: state.ModeMain, To: to, Total: m.Deviation, Reason: "player_choice"})
	}
	return reject("未知的选择。")
}

// ---------- Director：AI 主线节点提案的校验与正典化 ----------

// ValidateProposal 校验 AI 提议的主线节点；返回可正典化的节点或拒绝原因。奖励会被夹紧到上限。
func ValidateProposal(p *loader.Package, s *state.State, pr *command.NodeProposal) (*event.Node, error) {
	if pr == nil {
		return nil, fmt.Errorf("没有提案")
	}
	text := func(name, v string, lo, hi int) error {
		n := utf8.RuneCountInString(strings.TrimSpace(v))
		if n < lo || n > hi {
			return fmt.Errorf("%s长度应在 %d–%d 字之间", name, lo, hi)
		}
		if strings.ContainsAny(v, "<>{}\x00") || strings.Contains(v, "http") {
			return fmt.Errorf("%s含有不允许的字符", name)
		}
		return nil
	}
	if err := text("标题", pr.Title, 2, 24); err != nil {
		return nil, err
	}
	if err := text("目标", pr.Objective, 4, 80); err != nil {
		return nil, err
	}
	if pr.Summary != "" {
		if err := text("概要", pr.Summary, 0, 240); err != nil {
			return nil, err
		}
	}
	n := &event.Node{Title: strings.TrimSpace(pr.Title), Objective: strings.TrimSpace(pr.Objective), Goal: pr.Goal, Ref: pr.Ref, Location: pr.Location, Summary: strings.TrimSpace(pr.Summary), Source: "ai", Reward: map[string]int{}}
	if n.Location != "" {
		if _, ok := p.Locations[n.Location]; !ok {
			return nil, fmt.Errorf("地点 %q 不存在", n.Location)
		}
	}
	switch pr.Goal {
	case "reach":
		if _, ok := p.Locations[pr.Ref]; !ok {
			return nil, fmt.Errorf("目的地 %q 不存在", pr.Ref)
		}
		if pr.Ref == s.Player.Location {
			return nil, fmt.Errorf("目的地就是当前所在地")
		}
		n.Location = pr.Ref
	case "talk":
		np, ok := s.NPCs[pr.Ref]
		if !ok || np.Location == "" {
			return nil, fmt.Errorf("角色 %q 不存在或已不在人世", pr.Ref)
		}
		n.Location = np.Location
	case "defeat":
		if len(pr.Enemies) == 0 || len(pr.Enemies) > 4 {
			return nil, fmt.Errorf("敌人数量应为 1–4")
		}
		budget := s.PlayerLevel(p)*3 + 3
		sum := 0
		for _, e := range pr.Enemies {
			d, ok := p.Combat.Enemies[e]
			if !ok {
				return nil, fmt.Errorf("敌人 %q 不存在", e)
			}
			if d.Tier == "boss" {
				return nil, fmt.Errorf("自由推演节点不能直接安排首领战")
			}
			sum += d.Level
		}
		if sum > budget {
			return nil, fmt.Errorf("敌人太强（等级合计 %d，上限 %d）", sum, budget)
		}
		n.Enemies = slices.Clone(pr.Enemies)
		if n.Location == "" {
			n.Location = s.Player.Location
		}
		n.Ref = ""
	case "obtain":
		it, ok := p.Items[pr.Ref]
		if !ok {
			return nil, fmt.Errorf("物品 %q 不存在", pr.Ref)
		}
		if it.Kind == "key" || p.Combat.Config.RarityRank(it.Rarity) > p.Combat.Config.RarityRank("rare") {
			return nil, fmt.Errorf("不能把关键道具或史诗以上物品设为目标")
		}
	default:
		return nil, fmt.Errorf("未知目标类型 %q", pr.Goal)
	}
	n.Reward["xp"] = combat.Clamp(pr.RewardXP, 0, p.Mainline.MaxRewardXP)
	n.Reward["gold"] = combat.Clamp(pr.RewardGold, 0, 200)
	return n, nil
}

func (w *work) execDirector() error {
	p := w.pkg()
	m := &w.s.R().Main
	w.passive = true
	if m.CurrentMode() != state.ModeFree {
		return reject("不在自由推演模式。")
	}
	if m.ActiveNode() != nil {
		return reject("当前主线节点还没有完成。")
	}
	if w.cmd.Proposal == nil {
		return w.templateNode("template")
	}
	n, err := ValidateProposal(p, w.s, w.cmd.Proposal)
	if err != nil {
		return reject("提案未通过校验：%v", err)
	}
	n.ID = fmt.Sprintf("node%d", len(m.Nodes)+1)
	if err := w.emit(event.MainlineNodeCanonized, event.Data{Node: n, Source: "ai", Title: n.Title}); err != nil {
		return err
	}
	if c := w.cmd.Proposal.NewCharacter; c != nil {
		name := strings.TrimSpace(c.Name)
		nr := utf8.RuneCountInString(name)
		clash := false
		for _, id := range p.NPCIDs {
			clash = clash || p.Characters[id].Name() == name
		}
		for _, card := range w.s.R().Cards {
			clash = clash || card.Name == name
		}
		if nr >= 1 && nr <= 12 && !clash && utf8.RuneCountInString(c.Role) <= 16 && utf8.RuneCountInString(c.Description) <= 160 && !strings.ContainsAny(name+c.Role+c.Description, "<>{}") {
			id := fmt.Sprintf("dyn:character/%s", n.ID)
			if err := w.emit(event.CharacterCardCreated, event.Data{Target: id, Reason: "自由推演中登场", Node: &event.Node{ID: id, Title: name, Role: strings.TrimSpace(c.Role), Summary: strings.TrimSpace(c.Description), Source: "ai"}}); err != nil {
				return err
			}
		}
	}
	return nil
}

// templateNode 用故事包的沙盒模板确定性地生成一个委托（离线沙盒模式 / AI 失败时的兜底）。
func (w *work) templateNode(source string) error {
	p := w.pkg()
	m := &w.s.R().Main
	tpls := p.Mainline.Sandbox.Templates
	key := rng.Key{Namespace: "mainline", Entity: "sandbox", Purpose: "pick"}
	counter := w.s.RNG[key.String()]
	g := rng.NewAt(w.s.Seed, key, counter)
	n := &event.Node{ID: fmt.Sprintf("node%d", len(m.Nodes)+1), Source: source, Reward: map[string]int{}}
	pickS := func(list []string) string {
		if len(list) == 0 {
			return ""
		}
		return list[g.IntN(len(list))]
	}
	if len(tpls) == 0 {
		// 没有模板：在故事包的地点之间做一次“巡游”委托。
		var locs []string
		for _, id := range p.LocationIDs {
			if id != w.s.Player.Location {
				locs = append(locs, id)
			}
		}
		if len(locs) == 0 {
			return nil
		}
		to := pickS(locs)
		n.Title, n.Goal, n.Ref, n.Location = "四处走走", "reach", to, to
		n.Objective = "去" + p.EntityName(to) + "看看有什么新鲜事。"
		n.Reward["xp"] = 30
	} else {
		t := tpls[g.IntN(len(tpls))]
		loc, en, npc, item := pickS(t.Locations), pickS(t.Enemies), pickS(t.NPCs), pickS(t.Items)
		if t.Goal == "reach" && loc == w.s.Player.Location && len(t.Locations) > 1 {
			for _, l := range t.Locations {
				if l != loc {
					loc = l
					break
				}
			}
		}
		if t.Goal == "talk" && npc != "" && (w.s.NPCs[npc] == nil || w.s.NPCs[npc].Location == "") {
			npc = ""
		}
		repl := strings.NewReplacer("{location}", p.EntityName(loc), "{npc}", p.EntityName(npc), "{item}", p.EntityName(item), "{enemy}", enemyName(p, en))
		n.Title, n.Objective, n.Goal, n.Location = repl.Replace(t.Title), repl.Replace(t.Objective), t.Goal, loc
		switch t.Goal {
		case "defeat":
			n.Enemies = []string{en}
			if len(t.Enemies) > 1 && g.IntN(2) == 1 {
				n.Enemies = append(n.Enemies, en)
			}
			if n.Location == "" {
				n.Location = w.s.Player.Location
			}
		case "reach":
			n.Ref = loc
		case "talk":
			if npc == "" {
				return nil
			}
			n.Ref, n.Location = npc, w.s.NPCs[npc].Location
		case "obtain":
			n.Ref = item
		}
		n.Reward["xp"], n.Reward["gold"] = t.RewardXP, t.RewardGold
	}
	return w.emit(event.MainlineNodeCanonized, event.Data{Node: n, Source: source, Title: n.Title, Stream: key.String(), Counter: counter})
}

func enemyName(p *loader.Package, id string) string {
	if e, ok := p.Combat.Enemies[id]; ok {
		return e.Name
	}
	return id
}

// ---------- 回合末：主线 / 角色卡 / 图鉴 ----------

func (w *work) turnEvents(typ string) []event.Event {
	var out []event.Event
	for _, e := range w.events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

func (w *work) runRPG() error {
	if err := w.runMainline(); err != nil {
		return err
	}
	if err := w.runCards(); err != nil {
		return err
	}
	return w.runCodex()
}

func (w *work) runMainline() error {
	p := w.pkg()
	r := w.s.R()
	m := &r.Main
	mode := m.CurrentMode()
	if mode != state.ModeMain {
		if err := w.checkNode(); err != nil {
			return err
		}
		if mode == state.ModeSandbox && m.ActiveNode() == nil && r.Combat == nil {
			return w.templateNode("template")
		}
		return nil
	}
	if !p.Mainline.Enabled() {
		return nil
	}
	a := CurrentAnchor(p, w.s)
	if a == nil {
		return nil
	}
	progress := false
	for _, e := range w.events {
		switch e.Type {
		case event.StoryStarted, event.StoryStepReached, event.StoryResolved:
			if e.Data.Story == a.Story {
				progress = true
			}
		}
	}
	done, err := w.anchorDone(a)
	if err != nil {
		return err
	}
	if done {
		if err := w.emit(event.MainlineAnchorReached, event.Data{Story: a.ID, Title: a.Title, Delta: m.Anchor + 1}); err != nil {
			return err
		}
		progress = true
	}
	if r.Combat != nil && !done {
		return nil // 战斗中不评分
	}
	var hits []director.Hit
	tags := map[string]bool{}
	if w.cmd.Freeform != nil {
		for _, t := range w.cmd.Freeform.Tags {
			tags[t] = true
		}
	}
	if w.combatOutcome != "" {
		tags[w.combatOutcome] = true
	}
	action := ""
	if w.actx != nil {
		action, _ = w.actx["id"].(string)
	}
	for _, d := range p.Mainline.Deviant {
		if (d.Action != "" && d.Action == action) || (d.Tag != "" && tags[d.Tag]) {
			hits = append(hits, director.Hit{Weight: d.Weight, Reason: d.Reason})
		}
	}
	hits = append(hits, w.deviation...)
	budget := a.BudgetTurns
	res := director.Score(director.Input{
		AnchorLocations: a.Locations, PlayerLocation: w.s.Player.Location,
		TurnsSinceProgress: w.turn - m.LastProgressTurn, Budget: budget,
		Progress: progress, Deviant: hits, Passive: w.passive || w.combatTouched,
	})
	total := combat.Clamp(m.Deviation+res.Delta, 0, 100)
	if total != m.Deviation || progress {
		if err := w.emit(event.DeviationChanged, event.Data{Delta: total - m.Deviation, Total: total, Tags: res.Reasons, Success: progress}); err != nil {
			return err
		}
	}
	mild, heavy := Thresholds(p, w.s)
	a = CurrentAnchor(p, w.s)
	if a == nil {
		return nil
	}
	switch director.Level(m.Deviation, mild, heavy) {
	case 2:
		if !m.Pending {
			return w.emit(event.MainlinePrompted, event.Data{Total: m.Deviation, Story: a.ID})
		}
	case 1:
		if w.turn-m.LastNudgeTurn >= 3 && !w.passive {
			text := "有什么东西在提醒你：" + a.Objective
			if len(a.Nudges) > 0 {
				text = a.Nudges[m.Nudges%len(a.Nudges)]
			}
			if err := w.emit(event.MainlineNudged, event.Data{Story: a.ID, Text: text}); err != nil {
				return err
			}
			for _, o := range a.NudgeEffect {
				if err := w.applyOutcome(o, "", ""); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *work) anchorDone(a *loader.Anchor) (bool, error) {
	if a.Complete != "" {
		return w.eng.Eval.EvalBool(a.Complete, w.vars("", ""))
	}
	st := w.s.Stories[a.Story]
	return st != nil && st.Status == state.StoryResolved, nil
}

// checkNode 检查动态主线节点是否完成，完成则发放奖励。
func (w *work) checkNode() error {
	m := &w.s.R().Main
	n := m.ActiveNode()
	if n == nil {
		return nil
	}
	done := false
	switch n.Goal {
	case "reach":
		done = w.s.Player.Location == n.Ref
	case "talk":
		for _, e := range w.turnEvents(event.DialogueOccurred) {
			done = done || e.Data.Target == n.Ref
		}
	case "defeat":
		for _, e := range w.turnEvents(event.CombatEnded) {
			done = done || (e.Data.Key == n.ID && e.Data.Outcome == "victory")
		}
	case "obtain":
		done = w.s.Player.Inventory[n.Ref] > 0
	}
	if !done {
		return nil
	}
	if err := w.emit(event.MainlineNodeCompleted, event.Data{Story: n.ID, Title: n.Title}); err != nil {
		return err
	}
	if g := n.Reward["gold"]; g > 0 {
		if err := w.emit(event.GoldChanged, event.Data{Delta: g, Reason: "node"}); err != nil {
			return err
		}
	}
	if n.Item != "" {
		if _, ok := w.pkg().Items[n.Item]; ok {
			if err := w.emit(event.ItemAdded, event.Data{Item: n.Item, Qty: 1}); err != nil {
				return err
			}
		}
	}
	return w.grantXP(n.Reward["xp"], "完成："+n.Title)
}

// InteractionScore 是 NPC 与玩家的互动分：交谈 ×2 + 情节记忆 + 与玩家相关的关系变化次数。
func InteractionScore(s *state.State, id string) int {
	n := s.NPCs[id]
	if n == nil {
		return 0
	}
	score := n.Talks*2 + len(n.Episodes)
	if s.RPG != nil {
		for _, c := range s.RPG.RelLog {
			if (c.From == id && c.To == loader.PlayerID) || (c.To == id && c.From == loader.PlayerID) {
				score++
			}
		}
	}
	return score
}

func (w *work) runCards() error {
	p := w.pkg()
	r := w.s.R()
	talked := map[string]bool{}
	for _, e := range w.turnEvents(event.DialogueOccurred) {
		talked[e.Data.Target] = true
	}
	for _, id := range p.NPCIDs {
		c := p.Characters[id]
		card := r.Cards[id]
		n := w.s.NPCs[id]
		if n == nil || n.Location == "" {
			continue
		}
		switch {
		case card == nil && c.CardTier() == "major":
			if err := w.emit(event.CharacterCardCreated, event.Data{Target: id, Reason: "主要角色"}); err != nil {
				return err
			}
		case card == nil && c.CardTier() == "minor" && c.Promote != nil:
			hit, reason := false, c.Promote.Reason
			if c.Promote.Score > 0 && InteractionScore(w.s, id) >= c.Promote.Score {
				hit = true
				if reason == "" {
					reason = "与你往来频繁"
				}
			}
			if !hit && c.Promote.When != "" {
				ok, err := w.eng.Eval.EvalBool(c.Promote.When, w.vars(id, ""))
				if err != nil {
					return fmt.Errorf("%s promote: %w", id, err)
				}
				hit = ok
				if reason == "" {
					reason = "在故事中变得重要"
				}
			}
			if hit {
				if err := w.emit(event.CharacterCardCreated, event.Data{Target: id, Reason: reason}); err != nil {
					return err
				}
			}
		case card != nil && card.Status == state.CardArchived && talked[id]:
			if err := w.emit(event.CharacterCardRestored, event.Data{Target: id, Reason: "再次相遇"}); err != nil {
				return err
			}
		case card != nil && card.Status == state.CardActive && card.Changed == 0 && c.ArchiveWhen != "":
			ok, err := w.eng.Eval.EvalBool(c.ArchiveWhen, w.vars(id, ""))
			if err != nil {
				return fmt.Errorf("%s archive_when: %w", id, err)
			}
			if ok {
				if err := w.emit(event.CharacterCardArchived, event.Data{Target: id, Reason: "淡出了故事"}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *work) runCodex() error {
	p := w.pkg()
	r := w.s.R()
	unlock := func(id, reason string) error {
		if _, ok := r.Codex[id]; ok {
			return nil
		}
		return w.emit(event.CodexUnlocked, event.Data{Key: id, Reason: reason})
	}
	for _, id := range p.ItemIDs {
		if w.s.Player.Inventory[id] > 0 {
			if err := unlock(id, "item"); err != nil {
				return err
			}
		}
	}
	for _, id := range slices.Clone(r.Skills) {
		if err := unlock(id, "skill"); err != nil {
			return err
		}
	}
	for _, e := range w.turnEvents(event.CombatActed) {
		if e.Data.Skill != "" {
			if err := unlock(e.Data.Skill, "seen"); err != nil {
				return err
			}
		}
	}
	if err := unlock(w.s.Player.Location, "visit"); err != nil {
		return err
	}
	for _, id := range p.NPCIDs {
		if c := r.Cards[id]; (c != nil && c.Status != "") || w.s.NPCs[id].Talks > 0 {
			if err := unlock(id, "met"); err != nil {
				return err
			}
		}
	}
	for _, e := range p.Codex {
		if _, ok := r.Codex[e.ID]; ok {
			continue
		}
		ok := false
		if e.Unlock == "" {
			ok = e.Kind == "faction" || e.Kind == "lore" || e.Kind == "tech"
		} else {
			v, err := w.eng.Eval.EvalBool(e.Unlock, w.vars("", ""))
			if err != nil {
				return fmt.Errorf("codex %s: %w", e.ID, err)
			}
			ok = v
		}
		if ok {
			if err := unlock(e.ID, "codex"); err != nil {
				return err
			}
		}
	}
	return nil
}
