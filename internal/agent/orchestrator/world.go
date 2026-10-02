package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/event"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// ---------- 叙事前的强制注入（第 12.2 节）----------

// worldSections 构造 [WORLD_CHANGES]（相关变更，优先于故事包原设定）、[REVEALABLE]（渠道已满足、可以揭示的字段）
// 与 [ENTITIES]（场景实体 ID，供 WORLD 段引用）。
func (s *Session) worldSections(g *game, st *state.State, input, tier string) string {
	var sb strings.Builder
	env := g.eng.ValidationEnv(st, change.SourceNarrate, tier)
	present := validate.Present(env)
	budget := 900 * 3 // 约 900 token（中文约 1 字 / token，留余量按字符计）
	if tier != router.TierFull {
		budget = 400 * 3
	}
	if st.World != nil && len(st.World.Log) > 0 {
		var lines []string
		used := 0
		more := 0
		for i := len(st.World.Log) - 1; i >= 0; i-- {
			e := st.World.Log[i]
			if e.RevertedBy != "" || e.Reverts != "" {
				continue
			}
			name := g.pkg.EntityName(e.Target)
			if d := st.Doc(g.pkg, e.Target); d != nil {
				name = d.Name()
			}
			relevant := slices.Contains(present, e.Target) || e.Target == st.Player.Location ||
				(st.Turn-e.Turn <= 10 && e.Impact.Score >= 30) || (name != "" && strings.Contains(input, name))
			if !relevant {
				continue
			}
			line := fmt.Sprintf("- %s（回合 %d，%s）：%s", e.ID, e.Turn, name, e.Summary)
			if e.Reason != "" {
				line += "；原因：" + e.Reason
			}
			if used+len(line) > budget {
				more++
				continue
			}
			used += len(line)
			lines = append(lines, line)
		}
		if len(lines) > 0 {
			sb.WriteString("[WORLD_CHANGES]（这些变更优先于故事包原设定）\n")
			sb.WriteString(strings.Join(lines, "\n"))
			if more > 0 {
				fmt.Fprintf(&sb, "\n（另有 %d 条更早的变更未列出）", more)
			}
			sb.WriteString("\n")
		}
	}
	opts := validate.Revealable(env, validate.MaxReveals(tier)*2)
	if len(opts) > 0 {
		sb.WriteString("[REVEALABLE]（本回合可以让玩家得知的信息；只有叙事里确实交代了才写进 reveals）\n")
		for _, o := range opts {
			fmt.Fprintf(&sb, "- %s / %s（%s，渠道 %s，来源 %s）：%s\n", o.Entity, o.Field, o.Name, o.Channel, o.Source, o.Text)
		}
		sb.WriteString("隐藏真相（hidden:*）属于重大揭示：只有剧情自然走到时才揭示。\n")
	}
	s.mu.RLock()
	slot := s.slot
	s.mu.RUnlock()
	if cs := s.takeCorrections(slot); len(cs) > 0 {
		sb.WriteString("[CORRECTION]（一致性检查发现之前的叙事与设定不符；在本回合叙事中自然地更正，不要直接点破）\n")
		for _, c := range cs {
			sb.WriteString("- " + c + "\n")
		}
	}
	var ents []string
	for _, id := range present {
		ents = append(ents, id+"="+g.pkg.EntityName(id))
	}
	ents = append(ents, st.Player.Location+"="+g.pkg.EntityName(st.Player.Location), "player="+st.Player.Name)
	fmt.Fprintf(&sb, "[ENTITIES]\n%s\n命名空间：%s\n", strings.Join(ents, "；"), validate.GenNamespace(g.pkg))
	return sb.String()
}

// ---------- 提交 B：世界更新 ----------

// worldResult 是提交 B 的结果。
type worldResult struct {
	after    *state.State
	log      *dto.WorldLogV1
	decision *change.Decision
}

// commitWorld 把 WORLD 段作为系统命令（同一回合号）提交：校验 → 事件 → 覆盖层；记录被拒绝的提案；
// 影响分超过灵敏度阈值时创建自动检查点并请求偏离提示（第 7 节）。
func (s *Session) commitWorld(ctx context.Context, slot, branch string, g *game, cmdID string, before, st *state.State, wo narrator.WorldOutput, tier string) (worldResult, error) {
	wr := worldResult{after: st}
	var cmd command.Command
	switch {
	case wo.Refused:
		cmd = command.Command{Kind: command.KindWorldChange, Action: "failed", Target: "refused"}
	case wo.WorldErr != "":
		cmd = command.Command{Kind: command.KindWorldChange, Action: "failed", Target: wo.WorldErr}
	case wo.World == nil || wo.World.Empty():
		return wr, nil
	default:
		w := wo.World
		cmd = command.Command{Kind: command.KindWorldChange, Changes: w.AllChanges(), Reveals: w.Reveals, SelfLevel: w.Impact.Level, Tier: tier, Facts: w.SceneFacts}
	}
	cmd.ID, cmd.Source = cmdID+":world", change.SourceNarrate
	res, after, err := g.eng.Execute(st, cmd)
	if err != nil {
		return wr, err
	}
	if !res.Accepted || len(res.Events) == 0 {
		return wr, nil
	}
	wr.log = g.q.WorldLog(after, res.Events)
	var entries []eventstore.Entry
	if wr.log != nil {
		entries = append(entries, fromEntry(dto.EntryV1{Kind: "world", Turn: res.Turn, World: wr.log}))
	}
	empty := ""
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{CommandID: cmd.ID, Accepted: true, Command: cmd, Result: res, Events: res.Events, After: after,
		Entries: entries, Summary: g.summary(after), Narration: &empty}); err != nil && !errors.Is(err, eventstore.ErrDuplicateCommand) {
		return wr, err
	}
	wr.after = after
	var rejects []eventstore.Reject
	for _, r := range res.Rejects {
		b, _ := json.Marshal(r)
		rejects = append(rejects, eventstore.Reject{Branch: branch, CommandID: cmd.ID, Turn: res.Turn, Kind: r.Kind, Proposal: string(b), Reason: r.Reason})
	}
	_ = s.store.RecordRejects(ctx, slot, rejects)
	if res.Impact == nil {
		return wr, nil
	}
	settings := s.PromptSettings()
	p := change.Decide(*res.Impact, settings)
	if p == nil {
		return wr, nil
	}
	d, after2, err := s.requestDecision(ctx, slot, branch, g, cmdID, before, after, *res.Impact, p, settings, wr.log)
	if err != nil {
		return wr, err
	}
	wr.after, wr.decision = after2, d
	return wr, nil
}

// requestDecision 创建自动检查点（指向本回合之前）并提交 DecisionRequested。
func (s *Session) requestDecision(ctx context.Context, slot, branch string, g *game, cmdID string, before, after *state.State, imp change.TurnImpact, p *change.Prompt,
	settings change.Settings, log *dto.WorldLogV1) (*change.Decision, *state.State, error) {
	prevTurn := before.Turn
	seq, err := s.store.SeqAtTurnEnd(ctx, slot, branch, prevTurn)
	if err != nil {
		return nil, after, err
	}
	d := &change.Decision{ID: fmt.Sprintf("dec-%d", after.Turn), Type: p.Type, Score: p.Score, Changes: imp.Changes, Notify: p.Notify,
		Rollback: change.Ref{Branch: branch, Seq: seq, Turn: prevTurn}}
	var lines []string
	if log != nil {
		for _, c := range log.Changes {
			// Summary 已含对象名与新值；过长时截断（详情在世界变更日志里）
			l := []rune(c.Summary)
			if len(l) > 48 {
				l = append(l[:47], '…')
			}
			lines = append(lines, string(l))
		}
		for _, r := range log.Reveals {
			if strings.HasPrefix(r.Field, "hidden:") {
				lines = append(lines, r.Name+" · 一项隐藏真相被揭示")
			}
		}
	}
	d.Lines = lines
	switch p.Type {
	case change.TypeMajorDeath:
		d.Title = "重要人物死亡"
		for _, c := range log.Changes {
			if slices.Contains(c.Types, change.TypeMajorDeath) {
				d.Title = c.TargetName + "死了"
			}
		}
		d.Summary = "这是你关系网里的重要人物。接受这个结果，故事会照此继续；也可以回到上一回合重新选择。"
	case change.TypeLoreDeviation:
		d.Title = "这一回合改变了世界的原设定"
		d.Summary = fmt.Sprintf("影响分 %d。接受后，这些变化会成为这个世界的新事实。", p.Score)
	default:
		d.Title = "这一回合对故事影响很大"
		d.Summary = fmt.Sprintf("影响分 %d。接受这个结果继续，或者回到上一回合。", p.Score)
	}
	if settings.AutoCheckpointOn() {
		cp, err := s.store.CreateCheckpoint(ctx, slot, eventstore.Checkpoint{Branch: branch, Seq: seq, Turn: prevTurn, Name: fmt.Sprintf("回合 %d 之前", after.Turn),
			Kind: "auto", Reason: change.TypeLabel(p.Type) + "前自动保存"}, max(settings.KeepAuto, 1))
		if err == nil {
			d.Checkpoint = cp.Name
		}
	}
	cmd := command.Command{ID: cmdID + ":decision", Kind: command.KindDecision, Action: "request", Decision: d, Source: "engine"}
	res, after2, err := g.eng.Execute(after, cmd)
	if err != nil || !res.Accepted {
		return nil, after, err
	}
	empty := ""
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{CommandID: cmd.ID, Accepted: true, Command: cmd, Result: res, Events: res.Events, After: after2,
		Summary: g.summary(after2), Narration: &empty}); err != nil && !errors.Is(err, eventstore.ErrDuplicateCommand) {
		return nil, after, err
	}
	s.emit(dto.StreamEventV1{Type: "decision_requested", CommandID: cmdID, Text: d.Title})
	return d, after2, nil
}

// ---------- 偏离提示的选择 ----------

// PendingDecision 返回待决的偏离提示。
func (s *Session) PendingDecision() (*dto.DecisionV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	return g.q.DecisionView(st, s.PromptSettings()), nil
}

// ResolveDecision 处理偏离提示：accept（接受）/ rollback（回到上一回合，原分支保留为“已回滚”）/ dismiss（关闭通知）。
// notifyOnly=true 时把该类提示改为“仅通知”（返回新的设置，App 负责持久化）。
func (s *Session) ResolveDecision(ctx context.Context, id, action string, notifyOnly bool) (dto.SceneV1, error) {
	s.turnMu.Lock()
	slot, st, g, err := s.current()
	if err != nil {
		s.turnMu.Unlock()
		return dto.SceneV1{}, err
	}
	d := st.Pending
	if d == nil {
		d = st.Notice
	}
	if d == nil {
		s.turnMu.Unlock()
		return dto.SceneV1{}, errors.New("没有待决的选择")
	}
	if notifyOnly {
		set := s.PromptSettings()
		sens := set.For(d.Type)
		sens.Mode = change.ModeNotify
		switch d.Type {
		case change.TypeLoreDeviation:
			set.LoreDeviation = sens
		case change.TypeMajorDeath:
			set.MajorDeath = sens
		default:
			set.StoryImpact = sens
		}
		s.SetPromptSettings(set)
	}
	cmd := command.Command{ID: NewCommandID(), Kind: command.KindDecision, Action: action, Target: d.ID, Source: "ui"}
	res, after, err := g.eng.Execute(st, cmd)
	if err != nil {
		s.turnMu.Unlock()
		return dto.SceneV1{}, err
	}
	if !res.Accepted {
		s.turnMu.Unlock()
		return dto.SceneV1{}, errors.New(res.Reason)
	}
	empty := ""
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{CommandID: cmd.ID, Accepted: true, Command: cmd, Result: res, Events: res.Events, After: after,
		Summary: g.summary(after), Narration: &empty}); err != nil {
		s.turnMu.Unlock()
		return dto.SceneV1{}, err
	}
	s.mu.Lock()
	s.st = after
	s.mu.Unlock()
	s.turnMu.Unlock()
	if action == "rollback" {
		if err := s.RollbackTo(ctx, d.Rollback.Turn); err != nil {
			return dto.SceneV1{}, err
		}
	}
	return s.Scene(ctx)
}

// ---------- 回滚 / 分支 / 检查点（第 10 节）----------

func (s *Session) slotMeta(ctx context.Context) (eventstore.Slot, error) {
	slot, _, _, err := s.current()
	if err != nil {
		return eventstore.Slot{}, err
	}
	return s.store.GetSlot(ctx, slot)
}

// RollbackTo 回到当前分支的某个回合末尾（0 = 开局）。不会立刻删除或分叉：下一次行动时才创建新分支，
// 原分支保留（第 19 节决定 2）。
func (s *Session) RollbackTo(ctx context.Context, turn int) error {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return err
	}
	b, err := s.store.GetBranch(ctx, sl.ID, sl.Branch)
	if err != nil {
		return err
	}
	if turn < 0 || turn > b.HeadTurn {
		return fmt.Errorf("回合 %d 不在当前分支上", turn)
	}
	return s.rollbackRef(ctx, sl.ID, eventstore.ForkRef{Branch: sl.Branch, Turn: turn})
}

func (s *Session) rollbackRef(ctx context.Context, slot string, ref eventstore.ForkRef) error {
	seq, err := s.store.SeqAtTurnEnd(ctx, slot, ref.Branch, ref.Turn)
	if err != nil {
		return err
	}
	if ref.Seq == 0 || ref.Seq > seq {
		ref.Seq = seq
	}
	b, err := s.store.GetBranch(ctx, slot, ref.Branch)
	if err != nil {
		return err
	}
	if ref.Seq >= b.HeadSeq {
		// 回到头部 = 取消回滚
		if err := s.store.ClearPendingFork(ctx, slot); err != nil {
			return err
		}
	} else if err := s.store.SetPendingFork(ctx, slot, ref); err != nil {
		return err
	}
	st, err := s.store.BranchStateAt(ctx, slot, ref.Branch, ref.Seq)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.slot == slot {
		s.st = st
	}
	s.mu.Unlock()
	return nil
}

// CancelRollback 取消待定回滚，回到分支头部。
func (s *Session) CancelRollback(ctx context.Context) error {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return err
	}
	if err := s.store.ClearPendingFork(ctx, sl.ID); err != nil {
		return err
	}
	return s.reloadHead(ctx, sl.ID)
}

func (s *Session) reloadHead(ctx context.Context, slot string) error {
	st, err := s.store.Load(ctx, slot)
	if err != nil {
		return err
	}
	s.mu.Lock()
	if s.slot == slot {
		s.st = st
	}
	s.mu.Unlock()
	return nil
}

// SwitchBranch 切换到另一条分支的头部。
func (s *Session) SwitchBranch(ctx context.Context, branch string) error {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return err
	}
	if err := s.store.SwitchBranch(ctx, sl.ID, branch); err != nil {
		return err
	}
	if b, err := s.store.GetBranch(ctx, sl.ID, branch); err == nil && b.Status == eventstore.BranchRolledBack {
		_ = s.store.SetBranchStatus(ctx, sl.ID, branch, eventstore.BranchActive)
	}
	return s.reloadHead(ctx, sl.ID)
}

// RenameBranch / DeleteBranch 管理分支。
func (s *Session) RenameBranch(ctx context.Context, branch, name string) error {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return err
	}
	return s.store.RenameBranch(ctx, sl.ID, branch, strings.TrimSpace(name))
}

// DeleteBranch 删除非当前分支。
func (s *Session) DeleteBranch(ctx context.Context, branch string) error {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return err
	}
	return s.store.DeleteBranch(ctx, sl.ID, branch)
}

// forkIfPending 在执行命令前处理待定回滚：分出新分支（原分支标记为“已回滚”并保留），
// 写入 BranchCreated（随机盐让新分支的骰子与原分支不同，重放仍确定）。
func (s *Session) forkIfPending(ctx context.Context, slot string, g *game, st *state.State, cmdID string) (*state.State, error) {
	sl, err := s.store.GetSlot(ctx, slot)
	if err != nil {
		return st, err
	}
	pf := sl.PendingFork
	if pf.Branch == "" {
		return st, nil
	}
	salt := eventstore.NewSalt()
	b, err := s.store.Fork(ctx, slot, pf, "", salt)
	if err != nil {
		return st, err
	}
	_ = s.store.SetBranchStatus(ctx, slot, pf.Branch, eventstore.BranchRolledBack)
	base, err := s.store.BranchStateAt(ctx, slot, b.ID, pf.Seq)
	if err != nil {
		return st, err
	}
	e := event.Event{Seq: base.LastSeq + 1, CommandID: cmdID + ":fork", Turn: base.Turn, Minute: base.Minute, Type: event.BranchCreated,
		Data: event.Data{Key: b.ID, Seed: salt, Title: b.Name, Delta: pf.Turn}}
	after := base.Clone()
	if err := state.Apply(after, e); err != nil {
		return st, err
	}
	text := fmt.Sprintf("从回合 %d 分出 · %s", pf.Turn, b.Name)
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{CommandID: e.CommandID, Accepted: true, Command: map[string]string{"kind": "fork"}, Result: map[string]any{"branch": b.ID},
		Events: []event.Event{e}, After: after, Entries: []eventstore.Entry{{Kind: "branch", Turn: pf.Turn, Text: text}}, Summary: g.summary(after), ForceSnapshot: true}); err != nil {
		return st, err
	}
	return after, nil
}

// Timeline 返回时间线与检查点视图（当前分支的回合列表、分支、检查点）。
func (s *Session) Timeline(ctx context.Context) (dto.TimelineV1, error) {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return dto.TimelineV1{}, err
	}
	v := dto.TimelineV1{Branch: sl.Branch, Turns: []dto.TimelineTurnV1{}, Checkpoints: []dto.CheckpointV1{}, Branches: []dto.BranchV1{}}
	if sl.PendingFork.Branch != "" {
		v.PendingTurn = max(sl.PendingFork.Turn, 0)
		if sl.PendingFork.Turn == 0 {
			v.PendingTurn = -1
		}
	}
	bs, err := s.store.Branches(ctx, sl.ID)
	if err != nil {
		return v, err
	}
	forks := map[int]bool{}
	for _, b := range bs {
		v.Branches = append(v.Branches, dto.BranchV1{ID: b.ID, Name: branchName(b), Parent: b.Parent, ForkTurn: b.ForkTurn, HeadTurn: b.HeadTurn, Status: b.Status, Current: b.ID == sl.Branch})
		if b.Parent == sl.Branch || (b.ID == sl.Branch && b.Parent != "") {
			forks[b.ForkTurn] = true
		}
	}
	tr, err := s.store.BranchTranscript(ctx, sl.ID, sl.Branch, -1, 2000, 0)
	if err != nil {
		return v, err
	}
	byTurn := map[int]string{}
	var order []int
	for _, e := range tr {
		if e.Kind != "narration" && e.Kind != "intro" && e.Kind != "player" {
			continue
		}
		if _, ok := byTurn[e.Turn]; !ok {
			order = append(order, e.Turn)
			byTurn[e.Turn] = ""
		}
		// 优先用玩家的行动做摘要（“你：……”），更容易认出是哪一回合；没有时用叙事首句。
		switch {
		case e.Kind == "player":
			byTurn[e.Turn] = "你：" + firstSentence(e.Text)
		case byTurn[e.Turn] == "":
			byTurn[e.Turn] = firstSentence(e.Text)
		}
	}
	_, st, _, _ := s.current()
	for i := len(order) - 1; i >= 0; i-- {
		t := order[i]
		row := dto.TimelineTurnV1{Turn: t, Summary: byTurn[t], Fork: forks[t]}
		if st != nil && t == st.Turn {
			row.Current = true
			row.Time = worldtime.Format(st.Minute)
		}
		v.Turns = append(v.Turns, row)
	}
	cps, err := s.store.Checkpoints(ctx, sl.ID)
	if err != nil {
		return v, err
	}
	for _, c := range cps {
		v.Checkpoints = append(v.Checkpoints, dto.CheckpointV1{ID: c.ID, Branch: c.Branch, Turn: c.Turn, Name: c.Name, Kind: c.Kind, Reason: c.Reason})
	}
	return v, nil
}

func branchName(b eventstore.Branch) string {
	if b.ID == eventstore.MainBranch && b.Name == "" {
		return "主干"
	}
	if b.Name == "" {
		return b.ID
	}
	return b.Name
}

func firstSentence(t string) string {
	t = strings.TrimSpace(t)
	for _, sep := range []string{"。", "！", "？", "\n"} {
		if i := strings.Index(t, sep); i > 0 {
			t = t[:i+len(sep)]
			break
		}
	}
	if utf8.RuneCountInString(t) > 40 {
		t = string([]rune(t)[:40]) + "…"
	}
	return strings.TrimSpace(t)
}

// CreateCheckpoint 在当前位置（或当前分支的某回合）创建手动检查点。
func (s *Session) CreateCheckpoint(ctx context.Context, name string, turn int) (dto.CheckpointV1, error) {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return dto.CheckpointV1{}, err
	}
	_, st, _, _ := s.current()
	branch := sl.Branch
	if turn <= 0 {
		turn = st.Turn
	}
	seq, err := s.store.SeqAtTurnEnd(ctx, sl.ID, branch, turn)
	if err != nil {
		return dto.CheckpointV1{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("回合 %d", turn)
	}
	cp, err := s.store.CreateCheckpoint(ctx, sl.ID, eventstore.Checkpoint{Branch: branch, Seq: seq, Turn: turn, Name: strings.TrimSpace(name), Kind: "manual"}, 0)
	return dto.CheckpointV1{ID: cp.ID, Branch: cp.Branch, Turn: cp.Turn, Name: cp.Name, Kind: cp.Kind}, err
}

// RestoreCheckpoint 回到检查点（必要时先切换分支；之后行动会分出新分支）。
func (s *Session) RestoreCheckpoint(ctx context.Context, id string) error {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return err
	}
	cps, err := s.store.Checkpoints(ctx, sl.ID)
	if err != nil {
		return err
	}
	for _, c := range cps {
		if c.ID != id {
			continue
		}
		s.turnMu.Lock()
		defer s.turnMu.Unlock()
		if c.Branch != sl.Branch {
			if err := s.store.SwitchBranch(ctx, sl.ID, c.Branch); err != nil {
				return err
			}
		}
		return s.rollbackRef(ctx, sl.ID, eventstore.ForkRef{Branch: c.Branch, Seq: c.Seq, Turn: c.Turn})
	}
	return errors.New("没有这个检查点")
}

// RenameCheckpoint / DeleteCheckpoint 管理检查点。
func (s *Session) RenameCheckpoint(ctx context.Context, id, name string) error {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return err
	}
	return s.store.RenameCheckpoint(ctx, sl.ID, id, name)
}

// DeleteCheckpoint 删除检查点。
func (s *Session) DeleteCheckpoint(ctx context.Context, id string) error {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return err
	}
	return s.store.DeleteCheckpoint(ctx, sl.ID, id)
}

// dailyCheckpoint：跨天时自动创建“第 N 天”检查点（设置可关闭）。
func (s *Session) dailyCheckpoint(ctx context.Context, slot, branch string, before, after *state.State) {
	set := s.PromptSettings()
	if !set.DailyCheckpointOn() || before.Minute/(24*60) == after.Minute/(24*60) {
		return
	}
	seq, err := s.store.SeqAtTurnEnd(ctx, slot, branch, after.Turn)
	if err != nil {
		return
	}
	day := after.Minute/(24*60) + 1
	_, _ = s.store.CreateCheckpoint(ctx, slot, eventstore.Checkpoint{Branch: branch, Seq: seq, Turn: after.Turn, Name: fmt.Sprintf("第 %d 天", day), Kind: "auto", Reason: "每日检查点"}, max(set.KeepAuto, 1))
}

// ---------- 世界变更日志 / 撤销 ----------

// WorldChanges 返回世界变更日志（日志页）。
func (s *Session) WorldChanges(query, source string, limit int) ([]dto.WorldChangeV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	return g.q.WorldChanges(st, query, source, limit), nil
}

// RevertChange 撤销一条世界变更（写反向操作；后续依赖它的变更需要先撤销）。
func (s *Session) RevertChange(ctx context.Context, id string) (dto.WorldLogV1, error) {
	return s.RevertChangeMode(ctx, id, "")
}

// RevertPlan 返回撤销前的级联检查：依赖链（新的在前）与能否只撤销这一条。
func (s *Session) RevertPlan(id string) (dto.RevertPlanV1, error) {
	_, st, g, err := s.current()
	if err != nil {
		return dto.RevertPlanV1{}, err
	}
	return g.q.RevertPlan(st, id)
}

// RevertChangeMode 按模式撤销：""（有依赖时失败）、chain（连同依赖一起撤销）、single（只撤销这一条）。
func (s *Session) RevertChangeMode(ctx context.Context, id, mode string) (dto.WorldLogV1, error) {
	switch mode {
	case "", "chain", "single":
	default:
		return dto.WorldLogV1{}, fmt.Errorf("未知的撤销模式 %q（chain / single）", mode)
	}
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	slot, st, g, err := s.current()
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	cmd := command.Command{ID: NewCommandID(), Kind: command.KindWorldChange, Action: "revert", Target: id, Item: mode, Source: change.SourceUserRequest}
	res, after, err := g.eng.Execute(st, cmd)
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	if !res.Accepted {
		return dto.WorldLogV1{}, errors.New(res.Reason)
	}
	log := g.q.WorldLog(after, res.Events)
	var entries []eventstore.Entry
	if log != nil {
		entries = append(entries, fromEntry(dto.EntryV1{Kind: "world", Turn: res.Turn, World: log}))
	}
	empty := ""
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{CommandID: cmd.ID, Accepted: true, Command: cmd, Result: res, Events: res.Events, After: after,
		Entries: entries, Summary: g.summary(after), Narration: &empty}); err != nil {
		return dto.WorldLogV1{}, err
	}
	s.mu.Lock()
	s.st = after
	s.mu.Unlock()
	if log == nil {
		return dto.WorldLogV1{}, nil
	}
	return *log, nil
}

// ---------- token 用量 ----------

// Usage 返回 token 用量：turn>0 为该回合（当前分支），turn=0 为存档累计。
func (s *Session) Usage(ctx context.Context, turn int) (dto.UsageV1, error) {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return dto.UsageV1{}, err
	}
	branch := sl.Branch
	if turn == 0 {
		branch = ""
	}
	calls, err := s.store.AICalls(ctx, sl.ID, branch, turn)
	if err != nil {
		return dto.UsageV1{}, err
	}
	return summarizeUsage(turn, calls), nil
}

func summarizeUsage(turn int, calls []eventstore.AICall) dto.UsageV1 {
	u := dto.UsageV1{Turn: turn}
	idx := map[string]int{}
	for _, c := range calls {
		u.Calls++
		u.PromptTokens += c.PromptTokens
		u.CompletionTokens += c.CompletionTokens
		key := c.Task + "|" + c.Model
		i, ok := idx[key]
		if !ok {
			i = len(u.Tasks)
			idx[key] = i
			u.Tasks = append(u.Tasks, dto.UsageTaskV1{Task: c.Task, Name: router.TaskName(c.Task), Model: c.Model})
		}
		t := &u.Tasks[i]
		t.Calls++
		t.PromptTokens += c.PromptTokens
		t.CompletionTokens += c.CompletionTokens
		t.LatencyMS += c.LatencyMS
		if !c.OK {
			t.Failed++
			u.Failed++
		}
	}
	return u
}
