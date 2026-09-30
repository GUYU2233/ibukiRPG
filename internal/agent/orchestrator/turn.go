package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/action/resolver"
	"github.com/GUYU2233/ibukiRPG/internal/agent/canon"
	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
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
	case command.KindMainline:
		c = command.Command{Kind: command.KindMainline, Action: qa.Action, Target: qa.Target}
		if qa.Action == "free" {
			c.Target = "sandbox"
			if s.online() {
				c.Target = "free"
			}
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
	s.emit(dto.StreamEventV1{Type: "turn_started", CommandID: cmdID, Text: input})
	res, nar, online := s.components()

	var cmd command.Command
	resolverName := "ui"
	switch {
	case quick != nil:
		cmd = *quick
	case st.RPG != nil && st.RPG.Combat != nil:
		// 战斗中的自然语言输入：确定性规则解析为战斗动作
		c, ok, reason, opts := resolver.ResolveCombat(input, g.pkg, st)
		resolverName = "combat"
		if !ok {
			return s.commitRejection(ctx, slot, cmdID, input, st.Turn+1, reason, opts, resolverName)
		}
		cmd = c
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
	if _, ai := nar.(*narrator.LLM); ai {
		s.prepareBrief(ctx, slot, g, after, &brief)
	}
	nctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.opts.NarratorTimeout)
	out := nar.Narrate(nctx, brief, func(d string) {
		s.emit(dto.StreamEventV1{Type: "narration_delta", CommandID: cmdID, Text: d})
	})
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
	notices := g.q.Notices(after, result.Events)
	// 自由推演 / 沙盒：当前没有主线节点时，由导演生成下一个节点（AI 提案 → Core 校验 → 正史）
	extra, dnotices, derr := s.directorTurn(ctx, slot, cmdID, g, after)
	if derr != nil {
		return dto.TurnV1{}, derr
	}
	notices = append(notices, dnotices...)
	s.scheduleMemory(slot, g, s.stateOr(after))
	v, err := s.view(ctx, slot, cmdID, true)
	v.Entries = append(v.Entries, extra...)
	v.Notices = notices
	v.Resolver = resolverName
	s.emit(dto.StreamEventV1{Type: "turn_done", CommandID: cmdID})
	return v, err
}

func templateOnly(cmd command.Command, res *engine.Result) bool {
	switch cmd.Kind {
	case command.KindManage, command.KindMainline, command.KindDirector:
		return true
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

// directorTurn 在自由推演模式下补全下一个主线节点，作为独立的被动回合提交。
func (s *Session) directorTurn(ctx context.Context, slot, cmdID string, g *game, st *state.State) ([]dto.EntryV1, []dto.NoticeV1, error) {
	if st.RPG == nil || st.RPG.Combat != nil || st.RPG.Main.CurrentMode() != state.ModeFree || st.RPG.Main.ActiveNode() != nil {
		return nil, nil, nil
	}
	cmd := command.Command{Kind: command.KindDirector, Input: "（导演）", Source: "director", ID: cmdID + ":director"}
	if s.online() {
		cfg := func() provider.Config { s.mu.RLock(); defer s.mu.RUnlock(); return s.ai }()
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.opts.NarratorTimeout)
		pr, err := (&canon.Proposer{Provider: provider.NewOpenAICompatible(cfg, s.opts.Transport), Env: s.toolEnv(pctx, slot, g, st), Retrieval: directorRetrieval(g)}).Propose(pctx, g.pkg, st, recentFacts(g, st))
		cancel()
		if err == nil {
			if _, verr := engine.ValidateProposal(g.pkg, st, pr); verr == nil {
				cmd.Proposal, cmd.Source = pr, "director:ai"
			} else {
				err = verr
			}
		}
		if err != nil {
			s.mu.Lock()
			s.aiErr = "自由推演降级为模板节点：" + err.Error()
			s.mu.Unlock()
		}
	}
	result, after, err := g.eng.Execute(st, cmd)
	if err != nil || !result.Accepted {
		return nil, nil, err
	}
	entries := g.q.TurnEntries(st, after, result, "")
	var ses []eventstore.Entry
	var kept []dto.EntryV1
	for _, e := range entries {
		if e.Kind == "player" {
			continue
		}
		kept = append(kept, e)
		ses = append(ses, fromEntry(e))
	}
	empty := ""
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{
		CommandID: cmd.ID, Accepted: true, Command: cmd, Result: result, Events: result.Events, After: after,
		Entries: ses, Summary: g.summary(after), Narration: &empty,
	}); err != nil && !errors.Is(err, eventstore.ErrDuplicateCommand) {
		return nil, nil, err
	}
	s.mu.Lock()
	if s.slot == slot {
		s.st = after
	}
	s.mu.Unlock()
	return kept, g.q.Notices(after, result.Events), nil
}

// recentFacts 给导演 AI 的最近局势摘要（只含玩家可见信息）。
func recentFacts(g *game, st *state.State) []string {
	var out []string
	if st.RPG != nil {
		for _, n := range st.RPG.Main.Nodes {
			if n.Summary != "" {
				out = append(out, n.Title+"："+n.Summary)
			}
		}
		for id, r := range st.RPG.Encounters {
			if r.Wins > 0 {
				out = append(out, "已击败遭遇："+g.pkg.EntityName(id))
			}
		}
	}
	if len(out) > 12 {
		out = out[len(out)-12:]
	}
	return out
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
