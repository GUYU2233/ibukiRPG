package engine

import (
	"fmt"
	"slices"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/perception/witness"
	"github.com/GUYU2233/ibukiRPG/internal/rules/checks"
	"github.com/GUYU2233/ibukiRPG/internal/rules/expression"
	"github.com/GUYU2233/ibukiRPG/internal/rules/rng"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// Engine 是 Game Core：把 Command 变成 Event。它是确定性的——
// 同样的 State + Command（+ 内容包）永远产生同样的 Event 序列。AI 不参与这里的任何决定。
type Engine struct {
	Pkg  *loader.Package
	Eval *expression.Evaluator
}

// New 创建引擎。
func New(p *loader.Package, ev *expression.Evaluator) *Engine { return &Engine{Pkg: p, Eval: ev} }

// Result 是一条 Command 的执行结果。
type Result struct {
	CommandID string        `json:"command_id"`
	Turn      int           `json:"turn"`
	Accepted  bool          `json:"accepted"`
	Reason    string        `json:"reason,omitempty"` // 被拒绝时给玩家看的原因
	Events    []event.Event `json:"events"`
	// ---- v0.2.0 ----
	// Rejects 是被校验器丢弃的世界变更 / 揭示提案（诊断，不是游戏事件）。
	Rejects []validate.Reject `json:"rejects,omitempty"`
	// Impact 是本回合世界变更的影响汇总（KindWorldChange）。
	Impact *change.TurnImpact `json:"impact,omitempty"`
	// Hidden 是本回合被接受揭示的隐藏真相（实体#ID），Guard 放行其泄露关键词。
	Hidden []string `json:"hidden,omitempty"`
	// Interrupted 是时间跳跃被打断的原因。
	Interrupted string `json:"interrupted,omitempty"`
}

// Rejection 表示命令在校验阶段被拒绝（不产生任何事件，不推进时间）。
type Rejection struct{ Reason string }

func (r *Rejection) Error() string { return r.Reason }

func reject(format string, a ...any) error { return &Rejection{Reason: fmt.Sprintf(format, a...)} }

type work struct {
	eng    *Engine
	s      *state.State
	cmd    command.Command
	turn   int
	events []event.Event
	actx   map[string]any
	quiet  bool
	// storyTouched 表示本回合有剧情推进（不算平静回合）。
	storyTouched bool
	tension      int
	// combatTouched 表示本回合是战斗回合；combatOutcome 是本回合结束的战斗结果。
	combatTouched bool
	combatOutcome string
	// passive 表示本回合是不推动世界的操作（整理装备），不计入节奏。
	passive bool
}

// Execute 在 s 的副本上执行命令，返回结果与新状态。s 本身不会被修改。
// 校验失败返回 Accepted=false 的结果（非 error）；error 只表示内部错误。
func (e *Engine) Execute(s *state.State, cmd command.Command) (*Result, *state.State, error) {
	w := &work{eng: e, s: s.Clone(), cmd: cmd, turn: s.Turn + 1}
	res := &Result{CommandID: cmd.ID, Turn: w.turn}
	preActive := map[string]bool{}
	for _, id := range e.Pkg.StoryIDs {
		if st := w.s.Stories[id]; st != nil && st.Status == state.StoryActive {
			preActive[id] = true
		}
	}
	var err error
	if w.s.Pending != nil && cmd.Kind != command.KindDecision {
		res.Reason = "有一个待决的选择：请先在面板里选择“接受”或“回到之前”。"
		return res, s, nil
	}
	if systemCommand(cmd) {
		// 提交 B / 系统回合与玩家回合共享回合号（第 3.1 节）
		w.turn = s.Turn
		res.Turn = w.turn
		w.passive = true
	}
	if w.s.RPG != nil && w.s.RPG.Combat != nil && cmd.Kind != command.KindCombat && cmd.Kind != command.KindManage && !systemCommand(cmd) {
		res.Reason = "正在战斗中：请选择攻击、技能、物品、防御或逃跑。"
		return res, s, nil
	}
	switch cmd.Kind {
	case command.KindAction:
		err = w.execAction()
	case command.KindFreeform:
		err = w.execFreeform()
	case command.KindMove:
		err = w.execMove()
	case command.KindCombat:
		err = w.execCombat()
	case command.KindManage:
		err = w.execManage()
	case command.KindWorldChange:
		err = w.execWorldChange(res)
	case command.KindDecision:
		err = w.execDecision()
	case command.KindWait:
		err = w.execWait(res)
	case command.KindHook:
		err = w.execHook()
	default:
		err = fmt.Errorf("unknown command kind %q", cmd.Kind)
	}
	if err == nil && !systemCommand(cmd) {
		err = w.runTimeline(nil)
	}
	if err == nil {
		err = w.runStories(preActive)
	}
	if err == nil {
		err = w.runRPG()
	}
	if err == nil && !w.passive && !w.combatTouched {
		err = w.runPacing()
	}
	if err == nil {
		err = w.runKnowledge()
	}
	if err == nil && !systemCommand(cmd) {
		err = w.emit(event.TurnCompleted, event.Data{})
	}
	if err != nil {
		var rj *Rejection
		if asRejection(err, &rj) {
			res.Reason = rj.Reason
			return res, s, nil
		}
		return nil, s, err
	}
	res.Accepted = true
	res.Events = w.events
	return res, w.s, nil
}

func asRejection(err error, out **Rejection) bool {
	r, ok := err.(*Rejection) //nolint:errorlint // Rejection 从不被包装
	if ok {
		*out = r
	}
	return ok
}

func (w *work) emit(typ string, d event.Data) error {
	// 序号由引擎按 LastSeq 连续分配，Event Store 追加时校验连续性。
	e := event.Event{Seq: w.s.LastSeq + 1, CommandID: w.cmd.ID, Turn: w.turn, Minute: w.s.Minute, Type: typ, Data: d}
	if err := state.Apply(w.s, e); err != nil {
		return fmt.Errorf("apply %s: %w", typ, err)
	}
	w.events = append(w.events, e)
	return nil
}

func (w *work) pkg() *loader.Package { return w.eng.Pkg }

func (w *work) name(id string) string {
	if id == loader.PlayerID || id == "actor" {
		return w.s.Player.Name
	}
	return w.pkg().EntityName(id)
}

// ---------- ExecuteAction ----------

func (w *work) execAction() error {
	p := w.pkg()
	def, ok := p.Actions[w.cmd.Action]
	if !ok {
		return reject("未知的动作。")
	}
	loc := w.s.Player.Location
	target, item := w.cmd.Target, w.cmd.Item
	switch def.Target {
	case definition.TargetCharacter, definition.TargetOptionalChar:
		if target == "" && def.Target == definition.TargetCharacter {
			return reject("你想对谁%s？", def.Name)
		}
		if target != "" {
			n, ok := w.s.NPCs[target]
			if !ok {
				return reject("这里没有这个人。")
			}
			if n.Location != loc {
				return reject("%s不在这里。", w.name(target))
			}
		}
	case definition.TargetShopItem, definition.TargetInventoryItem:
		if item == "" {
			item = target
		}
		if _, ok := p.Items[item]; !ok {
			return reject("你想要哪样东西？")
		}
		target = ""
	default:
		target = ""
	}
	vars := w.vars(target, item)
	for _, r := range def.Requirements {
		ok, err := w.eng.Eval.EvalBool(r.Expr, vars)
		if err != nil {
			return fmt.Errorf("%s requirement: %w", def.ID, err)
		}
		if !ok {
			msg := r.Message
			if msg == "" {
				msg = "现在还做不到。"
			}
			return reject("%s", msg)
		}
	}
	minutes, err := worldtime.ParseDuration(def.Cost.Time)
	if err != nil {
		return err
	}
	if minutes > 0 {
		if err := w.emit(event.TimeAdvanced, event.Data{Minutes: minutes, Reason: def.ID}); err != nil {
			return err
		}
	}
	success, critical, fumble := true, false, false
	for _, c := range def.Checks {
		dc, err := w.eng.Eval.EvalInt(c.Difficulty, w.vars(target, item))
		if err != nil {
			return fmt.Errorf("%s difficulty: %w", def.ID, err)
		}
		r, err := w.rollCheck(c.Skill, int(dc), target)
		if err != nil {
			return err
		}
		success = success && r.Success
		critical = critical || r.Critical
		fumble = fumble || r.Fumble
	}
	outcome := "success"
	if !success {
		outcome = "failure"
	}
	if critical && success {
		if _, ok := def.Outcomes["critical_success"]; ok {
			outcome = "critical_success"
		}
	}
	if fumble && !success {
		if _, ok := def.Outcomes["critical_failure"]; ok {
			outcome = "critical_failure"
		}
	}
	summary := w.tmpl(def.Witness.Summary, target, item, outcomeName(success))
	if err := w.emit(event.ActionPerformed, event.Data{
		Actor: loader.PlayerID, Action: def.ID, Target: target, Item: item,
		Outcome: outcome, Success: success, Location: loc, Text: summary, Notable: def.Witness.Notable,
	}); err != nil {
		return err
	}
	// 交谈：先按“交谈前”的记忆挑选台词（DialogueOccurred），再结算动作效果。
	if def.Dialogue && target != "" && success {
		if err := w.converse(target); err != nil {
			return err
		}
	}
	for _, o := range def.Outcomes[outcome] {
		if err := w.applyOutcome(o, target, item); err != nil {
			return err
		}
	}
	if summary != "" {
		if err := w.observe(summary, def.Witness.Notable, event.ActionPerformed, "player:"+def.ID+":"+target, target, def.ID); err != nil {
			return err
		}
		// 被针对的 NPC 会把显眼的遭遇记成情节记忆（交谈本身由对话记忆负责）。
		if target != "" && def.Witness.Notable && !def.Dialogue {
			imp := 2
			if !success {
				imp = 1
			}
			if err := w.remember(target, "player:"+loader.Key(def.ID), summary, "experienced", imp); err != nil {
				return err
			}
		}
	}
	if len(def.Checks) > 0 && !success {
		w.tension += 40
	}
	w.quiet = def.Quiet
	w.actx = map[string]any{"kind": "action", "id": def.ID, "target": target, "item": item, "success": success, "outcome": outcome}
	return nil
}

func outcomeName(success bool) string {
	if success {
		return "成功"
	}
	return "失败"
}

// rollCheck 通过命名 RNG 流 ("check", "player", skill) 掷 d20。
func (w *work) rollCheck(skill string, dc int, target string) (checks.Result, error) {
	p := w.pkg()
	sk, ok := p.SkillByID(skill)
	if !ok {
		return checks.Result{}, fmt.Errorf("unknown skill %q", skill)
	}
	extra := 0
	for _, c := range w.s.Player.Conditions {
		extra += p.Rules.ConditionModifiers[c][skill]
	}
	mod := checks.Modifier(w.s.Player.Skills[skill], w.s.Player.Attributes[sk.Attribute], extra)
	dc = checks.ClampDC(dc)
	key := rng.Key{Namespace: "check", Entity: loader.PlayerID, Purpose: skill}
	counter := w.s.RNG[key.String()]
	roll := rng.NewAt(w.s.Seed, key, counter).Roll(checks.Die)
	r := checks.Resolve(skill, sk.Name, roll, mod, dc)
	err := w.emit(event.SkillCheckResolved, event.Data{
		Actor: loader.PlayerID, Target: target, Skill: skill, Roll: r.Roll, Modifier: r.Modifier, DC: r.DC,
		Total: r.Total, Success: r.Success, Critical: r.Critical, Fumble: r.Fumble,
		Stream: key.String(), Counter: counter,
	})
	return r, err
}

// ---------- Move ----------

func (w *work) execMove() error {
	p := w.pkg()
	from := w.s.Player.Location
	to := w.cmd.Destination
	if to == from {
		return reject("你已经在%s了。", w.name(to))
	}
	var exit *loader.Exit
	for i := range p.Locations[from].Exits {
		if p.Locations[from].Exits[i].To == to {
			exit = &p.Locations[from].Exits[i]
		}
	}
	if exit == nil {
		if _, ok := p.Locations[to]; ok {
			return reject("从这里去不了%s。", w.name(to))
		}
		return reject("没有这个地方。")
	}
	if exit.Requires != "" {
		ok, err := w.eng.Eval.EvalBool(exit.Requires, w.vars("", ""))
		if err != nil {
			return fmt.Errorf("exit requires: %w", err)
		}
		if !ok {
			msg := exit.LockedMessage
			if msg == "" {
				msg = "那里现在进不去。"
			}
			return reject("%s", msg)
		}
	}
	if err := w.observe(fmt.Sprintf("%s离开了，往%s去了", w.s.Player.Name, w.name(to)), false, event.LocationChanged, "", "", ""); err != nil {
		return err
	}
	if exit.Minutes > 0 {
		if err := w.emit(event.TimeAdvanced, event.Data{Minutes: int64(exit.Minutes), Reason: "move"}); err != nil {
			return err
		}
	}
	if err := w.movePlayer(to); err != nil {
		return err
	}
	w.actx = map[string]any{"kind": "move", "id": "move", "target": to, "item": "", "success": true, "outcome": "success"}
	return nil
}

// movePlayer 离开当前场景（丢弃短期 Scene Fact）并进入新场景（初始化 Scene Fact）。
func (w *work) movePlayer(to string) error {
	p := w.pkg()
	from := w.s.Player.Location
	if from == to {
		return nil
	}
	if err := w.emit(event.SceneFactsCleared, event.Data{Location: from}); err != nil {
		return err
	}
	if err := w.emit(event.LocationChanged, event.Data{Actor: loader.PlayerID, From: from, To: to}); err != nil {
		return err
	}
	facts := append([]string{}, p.Locations[to].SceneFacts...)
	for _, id := range p.StoryIDs {
		st := p.Stories[id]
		if w.s.Stories[id].Status == state.StoryActive && st.Location == to {
			for _, o := range st.IntroEffects {
				if o.Effect.Type == "scene_fact" {
					facts = append(facts, fmt.Sprint(o.Effect.Values["fact"]))
				}
			}
		}
	}
	for _, f := range facts {
		if err := w.emit(event.SceneFactChanged, event.Data{Location: to, Text: f}); err != nil {
			return err
		}
	}
	return w.observe(fmt.Sprintf("%s走了进来", w.s.Player.Name), false, event.LocationChanged, "", "", "")
}

// ---------- Observation / Witness ----------

// observe 让玩家所在地点的 NPC 观察一个客观事件：写入观察记忆；
// notable 事件还会更新信念（Event → Observation → Belief，第 15 节）。
func (w *work) observe(text string, notable bool, sourceEvent, key, target, action string) error {
	for _, npc := range witness.Witnesses(w.s, w.pkg(), witness.Visibility{Mode: "local", Location: w.s.Player.Location}) {
		if err := w.emit(event.ObservedEventCreated, event.Data{Witness: npc, Text: text, Reason: sourceEvent, Action: action, Target: target, Notable: notable}); err != nil {
			return err
		}
		if notable && key != "" {
			if err := w.emit(event.BeliefUpdated, event.Data{
				Witness: npc, Key: key, Text: text, Source: "witness", Reason: sourceEvent,
				Confidence: witness.Confidence(npc, target),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// remember 为 NPC 写入一条情节记忆（MemoryRecorded）。
func (w *work) remember(npc, key, text, source string, importance int) error {
	if _, ok := w.s.NPCs[npc]; !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	return w.emit(event.MemoryRecorded, event.Data{Witness: npc, Key: key, Text: text, Source: source, Delta: importance})
}

// ---------- 模板 ----------

func (w *work) tmpl(s, target, item, outcome string) string {
	if s == "" || !strings.Contains(s, "{") {
		return s
	}
	key := target
	if i := strings.LastIndex(target, "/"); i >= 0 {
		key = target[i+1:]
	}
	var cond, verb string
	if it, ok := w.pkg().Items[item]; ok && it.Use != nil {
		cond, verb = it.Use.Condition, it.Use.Verb
	}
	r := strings.NewReplacer(
		"{actor}", w.s.Player.Name,
		"{target.key}", key,
		"{target.id}", target,
		"{target}", w.name(target),
		"{item.id}", item,
		"{item.condition}", cond,
		"{item.verb}", verb,
		"{item}", w.name(item),
		"{location.id}", w.s.Player.Location,
		"{location}", w.name(w.s.Player.Location),
		"{outcome}", outcome,
	)
	return r.Replace(s)
}

func containsStr(list []string, s string) bool { return slices.Contains(list, s) }
