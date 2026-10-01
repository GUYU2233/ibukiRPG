package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/action/resolver"
	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/combat/freeform"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
)

// MaxInputRunes 是玩家单次输入的最大长度。
const MaxInputRunes = 200

// Submit 提交自然语言输入（经过 Action Resolver）。
func (s *Session) Submit(ctx context.Context, cmdID, text string) (dto.TurnV1, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return dto.TurnV1{}, errors.New("请输入你想做的事")
	}
	if utf8.RuneCountInString(text) > MaxInputRunes {
		text = string([]rune(text)[:MaxInputRunes])
	}
	return s.turn(ctx, cmdID, text, nil)
}

// Quick 执行快速通道动作（明确的 UI 操作，不经过 Resolver，第 9 节）。
func (s *Session) Quick(ctx context.Context, cmdID string, qa dto.QuickActionV1) (dto.TurnV1, error) {
	if qa.Kind == "text" {
		return s.Submit(ctx, cmdID, qa.Text)
	}
	var c command.Command
	switch qa.Kind {
	case "action":
		c = command.Command{Kind: command.KindAction, Action: qa.Action, Target: qa.Target, Item: qa.Item}
	case "move":
		c = command.Command{Kind: command.KindMove, Destination: qa.Destination}
	case command.KindCombat, command.KindManage:
		c = command.Command{Kind: qa.Kind, Action: qa.Action, Target: qa.Target, Item: qa.Item, Skill: qa.Skill}
	case command.KindWait:
		// 时间跳跃：Target = 10m / 1h / dawn / event:<id>
		c = command.Command{Kind: command.KindWait, Target: qa.Target}
		if qa.Label == "" {
			qa.Label = waitLabel(qa.Target)
		}
	default:
		return dto.TurnV1{}, errors.New("未知的快捷动作")
	}
	label := qa.Label
	if label == "" {
		label = qa.Action + qa.Destination
	}
	c.Input, c.Source = label, "ui"
	return s.turn(ctx, cmdID, label, &c)
}

// turn 执行一个完整回合。任何失败都不会产生半个回合：事件、命令记录与对话记录在同一事务提交；
// 叙事在提交之后生成，失败时降级为模板，永不重新执行命令或重新掷骰。
func (s *Session) turn(ctx context.Context, cmdID, input string, quick *command.Command) (dto.TurnV1, error) {
	if !s.turnMu.TryLock() {
		return dto.TurnV1{}, ErrBusy
	}
	defer s.turnMu.Unlock()
	slot, st, g, err := s.current()
	if err != nil {
		return dto.TurnV1{}, err
	}
	if cmdID == "" {
		cmdID = NewCommandID()
	}
	// 幂等：同一 command_id 已执行过，直接返回之前的结果（第 52 节）
	if rec, ok, err := s.store.LookupCommand(ctx, slot, cmdID); err != nil {
		return dto.TurnV1{}, err
	} else if ok {
		if !rec.HasNarr {
			if err := s.repairNarrations(ctx, slot, g); err != nil {
				return dto.TurnV1{}, err
			}
		}
		v, err := s.view(ctx, slot, cmdID, rec.Accepted)
		v.Duplicate = true
		return v, err
	}
	// 已“回到这里”：先分出新分支（原分支保留）
	if st, err = s.forkIfPending(ctx, slot, g, st, cmdID); err != nil {
		return dto.TurnV1{}, err
	}
	sl, err := s.store.GetSlot(ctx, slot)
	if err != nil {
		return dto.TurnV1{}, err
	}
	branch := sl.Branch
	s.setRec(aiRec{slot: slot, branch: branch, cmdID: cmdID, turn: st.Turn + 1})
	defer s.setRec(aiRec{})
	s.emit(dto.StreamEventV1{Type: "turn_started", CommandID: cmdID, Text: input})
	res, nar, online := s.components()

	var cmd command.Command
	resolverName := "ui"
	switch {
	case quick != nil:
		cmd = *quick
	case st.RPG != nil && st.RPG.Combat != nil:
		// 战斗中的自然语言输入：短指令走确定性解析；自由描述走自由战斗裁定（AI 解析意图，引擎裁定数值）
		c, ok, reason, opts := resolver.ResolveCombat(input, g.pkg, st)
		resolverName = "combat"
		switch {
		case ok && utf8.RuneCountInString(input) <= 8:
			cmd = c
		case st.RPG.Combat.Current() != nil && st.RPG.Combat.Current().ID == "player":
			in, src := s.combatIntent(ctx, g, st, input)
			resolverName = "freeform:" + src
			if src == "rules" && in.SelfCheck == freeform.Unmatched {
				if reason == "" {
					reason = "战斗中做不到这件事。"
				}
				return s.commitRejection(ctx, slot, cmdID, input, st.Turn+1, reason, opts, resolverName)
			}
			cmd = command.Command{Kind: command.KindCombat, Action: "freeform", Intent: &in, Input: input, Source: "resolver:" + src}
		case ok:
			cmd = c
		default:
			return s.commitRejection(ctx, slot, cmdID, input, st.Turn+1, reason, opts, resolverName)
		}
	default:
		rctx, cancel := context.WithTimeout(ctx, s.opts.ResolverTimeout)
		r, err := res.Resolve(rctx, resolver.Input{Text: input, Pkg: g.pkg, State: st})
		cancel()
		if err != nil {
			// 离线解析器不会失败；这里只是防御
			r, _ = resolver.Offline{}.Resolve(ctx, resolver.Input{Text: input, Pkg: g.pkg, State: st})
		}
		resolverName = r.Source
		c, ok := r.Command(input, "resolver:"+r.Source)
		if !ok {
			return s.commitRejection(ctx, slot, cmdID, input, st.Turn+1, r.Reason, r.Options, resolverName)
		}
		cmd = c
	}
	cmd.ID = cmdID
	result, after, err := g.eng.Execute(st, cmd)
	if err != nil {
		return dto.TurnV1{}, err
	}
	if !result.Accepted {
		return s.commitRejection(ctx, slot, cmdID, input, st.Turn+1, result.Reason, nil, resolverName)
	}
	entries := g.q.TurnEntries(st, after, result, input)
	var ses []eventstore.Entry
	for _, e := range entries {
		ses = append(ses, fromEntry(e))
	}
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{
		CommandID: cmdID, Accepted: true, Command: cmd, Result: result, Events: result.Events, After: after,
		Entries: ses, Summary: g.summary(after),
	}); err != nil {
		if errors.Is(err, eventstore.ErrDuplicateCommand) {
			v, verr := s.view(ctx, slot, cmdID, true)
			v.Duplicate = true
			return v, verr
		}
		return dto.TurnV1{}, err
	}
	s.mu.Lock()
	if s.slot == slot {
		s.st = after
	}
	s.mu.Unlock()

	// 叙事：流式输出 → Guard → 必要时模板兜底。战斗回合 / 管理操作使用模板（快速、确定），
	// 只有开战与战斗结束时才请 AI 润色。
	if templateOnly(cmd, result) {
		nar = narrator.Template{}
	}
	brief := narrator.Build(g.pkg, st, after, cmd, result)
	llm, ai := nar.(*narrator.LLM)
	if ai {
		s.prepareBrief(ctx, slot, g, after, &brief)
	}
	nctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.opts.NarratorTimeout)
	onDelta := func(d string) {
		s.emit(dto.StreamEventV1{Type: "narration_delta", CommandID: cmdID, Text: d})
	}
	var out narrator.Output
	var wo narrator.WorldOutput
	merged := ai && cmd.Kind != command.KindCombat
	if merged {
		// 叙事 + 世界更新合并输出（第 11.3 节）
		tgt, _ := s.router.Primary(router.TaskNarrate)
		tier := tgt.Tier
		wo = llm.NarrateWorld(nctx, brief, s.worldSections(g, after, input, tier), tier, onDelta)
		out = wo.Output
		if wo.Refused {
			out.Text = brief.Base + "\n\n（AI 服务商拒绝生成本回合内容（内容审核）。规则结果已保存；你可以换种说法、回到上一回合，或在生成设置里为叙事换一个模型。）"
		} else if wo.WorldErr == "parse" {
			out.Text += "\n\n（本回合的世界变化没能记录（AI 输出格式错误），叙事已保留。审查时会补上。）"
		}
	} else {
		out = nar.Narrate(nctx, brief, onDelta)
	}
	cancel()
	if online && out.Err != "" {
		s.mu.Lock()
		s.aiErr = "叙事降级为模板：" + out.Err
		s.mu.Unlock()
	}
	meta, _ := json.Marshal(entryMeta{Corrected: out.Corrected, Source: out.Source})
	if err := s.store.SetNarration(context.WithoutCancel(ctx), slot, cmdID, out.Text, eventstore.Entry{Kind: "narration", Turn: result.Turn, Meta: meta}); err != nil {
		return dto.TurnV1{}, err
	}
	s.emit(dto.StreamEventV1{Type: "narration_done", CommandID: cmdID, Text: out.Text, Corrected: out.Corrected, Source: out.Source})
	final := after
	if merged {
		wr, err := s.commitWorld(context.WithoutCancel(ctx), slot, branch, g, cmdID, st, after, wo, tierOf(s.router))
		if err != nil {
			return dto.TurnV1{}, err
		}
		final = wr.after
		if wo.World != nil {
			s.setAISuggestions(slot, wo.World.Suggestions)
		}
		s.emit(dto.StreamEventV1{Type: "world_update_done", CommandID: cmdID})
	} else {
		s.setAISuggestions(slot, nil)
	}
	s.mu.Lock()
	if s.slot == slot {
		s.st = final
	}
	s.mu.Unlock()
	s.dailyCheckpoint(context.WithoutCancel(ctx), slot, branch, st, final)
	notices := g.q.Notices(final, result.Events)
	s.scheduleMemory(slot, g, s.stateOr(final))
	var usage *dto.UsageV1
	if s.router.Settings().ShowUsage && online {
		if u, uerr := s.Usage(ctx, result.Turn); uerr == nil && u.Calls > 0 {
			usage = &u
			// 用量行作为条目落盘：重新打开存档后叙事流里仍能看到“回合 N · ↑… ↓…”。
			_ = s.store.AppendEntries(ctx, slot, []eventstore.Entry{fromEntry(dto.EntryV1{CommandID: cmdID + ":usage", Kind: "usage", Turn: result.Turn, Usage: usage})})
		}
	}
	v, err := s.view(ctx, slot, cmdID, true)
	v.Notices = notices
	v.Resolver = resolverName
	v.Usage = usage
	s.emit(dto.StreamEventV1{Type: "turn_done", CommandID: cmdID})
	return v, err
}

func templateOnly(cmd command.Command, res *engine.Result) bool {
	switch cmd.Kind {
	case command.KindManage, command.KindDecision, command.KindWorldChange, command.KindWait:
		return res.Interrupted == ""
	case command.KindCombat:
		for _, e := range res.Events {
			if e.Type == event.CombatEnded || e.Type == event.CombatStarted {
				return false
			}
		}
		return true
	}
	return false
}

func (s *Session) commitRejection(ctx context.Context, slot, cmdID, input string, turn int, reason string, opts []resolver.Option, resolverName string) (dto.TurnV1, error) {
	if reason == "" {
		reason = "这件事现在做不到。"
	}
	sys := dto.EntryV1{Kind: "system", Turn: turn, Text: reason}
	for _, o := range opts {
		qa := dto.QuickActionV1{Kind: o.Command.Kind, Action: o.Command.Action, Target: o.Command.Target, Item: o.Command.Item, Skill: o.Command.Skill, Destination: o.Command.Destination, Label: o.Label}
		if qa.Kind == command.KindFreeform {
			continue
		}
		sys.Options = append(sys.Options, dto.OptionV1{Label: o.Label, Action: qa})
	}
	empty := ""
	err := s.store.CommitTurn(ctx, slot, eventstore.Commit{
		CommandID: cmdID, Accepted: false, Command: map[string]string{"input": input}, Result: &engine.Result{CommandID: cmdID, Reason: reason},
		Entries: []eventstore.Entry{fromEntry(dto.EntryV1{Kind: "player", Turn: turn, Text: input}), fromEntry(sys)},
		Summary: s.currentSummary(), Narration: &empty,
	})
	if err != nil && !errors.Is(err, eventstore.ErrDuplicateCommand) {
		return dto.TurnV1{}, err
	}
	s.emit(dto.StreamEventV1{Type: "turn_done", CommandID: cmdID})
	v, err := s.view(ctx, slot, cmdID, false)
	v.Resolver = resolverName
	return v, err
}

func (s *Session) currentSummary() eventstore.Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.st == nil || s.g == nil {
		return eventstore.Summary{}
	}
	return s.g.summary(s.st)
}

func (s *Session) view(ctx context.Context, slot, cmdID string, accepted bool) (dto.TurnV1, error) {
	// 同时取回 <id>:fork / <id>:world / <id>:decision 的条目（前缀匹配）
	es, err := s.store.EntriesByCommand(ctx, slot, cmdID)
	if err != nil {
		return dto.TurnV1{}, err
	}
	v := dto.TurnV1{CommandID: cmdID, Accepted: accepted, Entries: toEntries(es)}
	if v.Scene, err = s.Scene(ctx); err != nil {
		return v, err
	}
	v.Suggestions, err = s.Suggestions()
	return v, err
}

// stateOr 返回当前存档的最新状态（导演回合可能又推进了一步），没有时返回 fallback。
func (s *Session) stateOr(fallback *state.State) *state.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.st != nil {
		return s.st
	}
	return fallback
}

// tierOf 返回叙事任务首选模型的能力档位。
func tierOf(r *router.Router) string {
	if t, ok := r.Primary(router.TaskNarrate); ok {
		return t.Tier
	}
	return router.TierFull
}

func (s *Session) setAISuggestions(slot string, sg []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.aiSugg == nil {
		s.aiSugg = map[string][]string{}
	}
	if len(sg) > 3 {
		sg = sg[:3]
	}
	s.aiSugg[slot] = sg
}

func (s *Session) aiSuggestions(slot string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aiSugg[slot]
}

func waitLabel(t string) string {
	switch {
	case t == "dawn":
		return "等到天亮"
	case strings.HasPrefix(t, "event:"):
		return "等待事件发生"
	case t == "":
		return "稍等片刻"
	}
	return "等待 " + t
}
