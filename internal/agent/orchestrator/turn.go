package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/action/resolver"
	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
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
	slot, st, err := s.current()
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
			if err := s.repairNarrations(ctx, slot); err != nil {
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
	if quick != nil {
		cmd = *quick
	} else {
		rctx, cancel := context.WithTimeout(ctx, s.opts.ResolverTimeout)
		r, err := res.Resolve(rctx, resolver.Input{Text: input, Pkg: s.pkg, State: st})
		cancel()
		if err != nil {
			// 离线解析器不会失败；这里只是防御
			r, _ = resolver.Offline{}.Resolve(ctx, resolver.Input{Text: input, Pkg: s.pkg, State: st})
		}
		resolverName = r.Source
		c, ok := r.Command(input, "resolver:"+r.Source)
		if !ok {
			return s.commitRejection(ctx, slot, cmdID, input, st.Turn+1, r.Reason, r.Options, resolverName)
		}
		cmd = c
	}
	cmd.ID = cmdID
	result, after, err := s.eng.Execute(st, cmd)
	if err != nil {
		return dto.TurnV1{}, err
	}
	if !result.Accepted {
		return s.commitRejection(ctx, slot, cmdID, input, st.Turn+1, result.Reason, nil, resolverName)
	}
	entries := s.q.TurnEntries(st, after, result, input)
	var ses []eventstore.Entry
	for _, e := range entries {
		ses = append(ses, fromEntry(e))
	}
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{
		CommandID: cmdID, Accepted: true, Command: cmd, Result: result, Events: result.Events, After: after,
		Entries: ses, Summary: s.summary(after),
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

	// 叙事：流式输出 → Guard → 必要时模板兜底
	brief := narrator.Build(s.pkg, st, after, cmd, result)
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
	v, err := s.view(ctx, slot, cmdID, true)
	v.Resolver = resolverName
	s.emit(dto.StreamEventV1{Type: "turn_done", CommandID: cmdID})
	return v, err
}

func (s *Session) commitRejection(ctx context.Context, slot, cmdID, input string, turn int, reason string, opts []resolver.Option, resolverName string) (dto.TurnV1, error) {
	if reason == "" {
		reason = "这件事现在做不到。"
	}
	sys := dto.EntryV1{Kind: "system", Turn: turn, Text: reason}
	for _, o := range opts {
		qa := dto.QuickActionV1{Kind: o.Command.Kind, Action: o.Command.Action, Target: o.Command.Target, Item: o.Command.Item, Destination: o.Command.Destination, Label: o.Label}
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
	if s.st == nil {
		return eventstore.Summary{}
	}
	return s.summary(s.st)
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
