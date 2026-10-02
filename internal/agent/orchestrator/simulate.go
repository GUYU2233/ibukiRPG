package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/agent/simulator"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/core/engine"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/timeline"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
	"github.com/GUYU2233/ibukiRPG/internal/world/worldtime"
)

// ---------- 场外世界模拟（v0.2.0-rc1） ----------
//
// 回合之间（游戏时间跨过模拟间隔）或玩家等待（/wait）时，世界模拟 Agent 推进场外的势力、NPC 与事件。
// 提案走与叙事完全相同的校验器与事件日志（来源 world_sim，可单项 / 级联撤销），影响分超过灵敏度阈值时同样弹出偏离提示。
// 联网时使用“世界模拟”任务路由的模型（本地模型用精简格式）；离线、超出每日 AI 次数上限、模型拒绝或输出无效时
// 按故事包的 rules/simulation.yaml 推进（确定性）。

// SimStatus 是世界模拟的设置摘要（生成设置页）。
type SimStatus struct {
	EveryMinutes int    `json:"every_minutes"`
	AICalls      int    `json:"ai_calls_per_day"`
	Mode         string `json:"mode"` // ai / rules / off
}

// simEvery 返回模拟间隔（游戏内分钟）；0 表示关闭。
func (s *Session) simEvery(g *game) int {
	n := s.router.Settings().SimEvery
	switch {
	case n < 0:
		return 0
	case n == 0:
		n = g.pkg.Simulation.EveryMinutes
	}
	return max(n, 30)
}

// simAICap 返回每个游戏日的 AI 调用上限；0 表示只用规则。
func (s *Session) simAICap(g *game) int {
	n := s.router.Settings().SimAICalls
	switch {
	case n < 0:
		return 0
	case n == 0:
		n = g.pkg.Simulation.MaxAICallsPerDay
	}
	return max(n, 0)
}

// simDue 判断本回合之后是否要做一次场外模拟：游戏时间跨过了模拟间隔，或玩家等待了至少一小时。
// period 是时间段序号（规则轮换用）。
func (s *Session) simDue(g *game, before, after *state.State, cmd command.Command) (trigger string, period int64, ok bool) {
	every := int64(s.simEvery(g))
	if every == 0 || after.RPG != nil && after.RPG.Combat != nil || after.Pending != nil {
		return "", 0, false
	}
	elapsed := after.Minute - before.Minute
	if elapsed <= 0 {
		return "", 0, false
	}
	period = after.Minute / every
	switch {
	case cmd.Kind == command.KindWait && elapsed >= 60:
		return "wait", period, true
	case before.Minute/every != period:
		return "time", period, true
	}
	return "", 0, false
}

// takeSimCall 记一次 AI 模拟调用；超过当日上限时返回 false。
func (s *Session) takeSimCall(slot string, day, limit int) bool {
	if limit <= 0 {
		return false
	}
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	if s.simCalls == nil {
		s.simCalls = map[string]int{}
	}
	key := fmt.Sprintf("%s#%d", slot, day)
	if s.simCalls[key] >= limit {
		return false
	}
	s.simCalls[key]++
	return true
}

// simulate 做一次场外模拟并提交（调用方持有 turnMu）。返回提交后的状态（没有变化时原样返回）。
func (s *Session) simulate(ctx context.Context, slot, branch string, g *game, cmdID string, st *state.State, trigger string, period int64) (*state.State, error) {
	out, source, tier := s.simPropose(ctx, slot, g, st, trigger, period)
	if out.Empty() {
		return st, nil
	}
	return s.commitSim(ctx, slot, branch, g, cmdID, st, out, source, tier)
}

// simPropose 产生提案：AI 优先（受每日次数上限约束），失败回退规则。source 为 ai / rules。
func (s *Session) simPropose(ctx context.Context, slot string, g *game, st *state.State, trigger string, period int64) (simulator.Output, string, string) {
	if pr, tgt, ok := s.providerFor(router.TaskWorldSim); ok && s.takeSimCall(slot, worldtime.Day(st.Minute), s.simAICap(g)) {
		compact := tgt.Tier == router.TierLimited || tgt.Tier == router.TierMinimal
		sys, maxTok := simulator.System, 700
		if compact {
			sys, maxTok = simulator.SystemCompact, 400
		}
		cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		resp, err := pr.Generate(cctx, provider.Request{Messages: []provider.Message{{Role: "system", Content: sys}, {Role: "user", Content: s.simInput(g, st, trigger)}},
			Temperature: 0.4, MaxTokens: maxTok, JSON: true})
		cancel()
		if err == nil && !structured.IsRefusal(resp.Text, nil) {
			if out, perr := simulator.Parse(resp.Text, compact); perr == nil && !out.Empty() {
				return out, "ai", tgt.Tier
			}
		}
	}
	return s.simRules(g, st, trigger, period), "rules", router.TierFull
}

// simRules 按故事包规则推进（离线 / 回退）。
func (s *Session) simRules(g *game, st *state.State, trigger string, period int64) simulator.Output {
	eval := func(expr string) (bool, error) { return g.eng.Eval.EvalBool(expr, engine.Vars(g.pkg, st, "", "")) }
	fired := func(id string) bool {
		for i := range g.pkg.Simulation.Rules {
			r := &g.pkg.Simulation.Rules[i]
			if r.ID != id {
				continue
			}
			reason := simulator.RuleReason(r)
			if st.World == nil || reason == "" {
				return false
			}
			for _, e := range st.World.Log {
				if e.Source == change.SourceWorldSim && e.Reason == reason && e.RevertedBy == "" {
					return true
				}
			}
		}
		return false
	}
	r := simulator.Pick(g.pkg.Simulation, trigger, period, eval, fired)
	if r == nil {
		return simulator.Output{}
	}
	return simulator.RuleOutput(r)
}

// simInput 构造世界模拟的输入（只给公开设定与目标，不给隐藏真相）。
func (s *Session) simInput(g *game, st *state.State, trigger string) string {
	var sb strings.Builder
	every := s.simEvery(g)
	how := fmt.Sprintf("距上次场外推进约 %d 小时", max(every/60, 1))
	if trigger == "wait" {
		how = "玩家在原地等待，时间流逝"
	}
	fmt.Fprintf(&sb, "[TIME]\n%s；%s。\n", worldtime.Format(st.Minute), how)
	fmt.Fprintf(&sb, "[PLAYER]\n玩家在%s（不要改动玩家）。\n", g.pkg.EntityName(st.Player.Location))
	ids := []string{}
	if len(g.pkg.FactionIDs) > 0 {
		sb.WriteString("[FACTIONS]\n")
		for _, id := range g.pkg.FactionIDs {
			f := g.pkg.Factions[id]
			d := st.Doc(g.pkg, id)
			if d == nil || d.Retired != nil {
				continue
			}
			line := fmt.Sprintf("- %s=%s；目标：%s", id, d.Name(), strings.Join(f.Goals, "、"))
			if v := d.Field("stance"); v != "" {
				line += "；当前动向：" + v
			}
			sb.WriteString(line + "\n")
			ids = append(ids, id)
		}
	}
	sb.WriteString("[OFFSCREEN]\n")
	for _, id := range g.pkg.NPCIDs {
		n := st.NPCs[id]
		if n == nil || n.Location == "" || n.Location == st.Player.Location {
			continue
		}
		c := g.pkg.Characters[id]
		line := fmt.Sprintf("- %s=%s，在%s", id, c.Name(), g.pkg.EntityName(n.Location))
		if len(c.Goals) > 0 {
			line += "；目标：" + strings.Join(c.Goals, "、")
		}
		sb.WriteString(line + "\n")
		ids = append(ids, id, n.Location)
	}
	sb.WriteString("[TIMELINE]\n")
	for _, ev := range g.q.TimelineEvents(st) {
		if ev.Status == timeline.Resolved || ev.Status == timeline.Cancelled || ev.Rumored {
			continue
		}
		fmt.Fprintf(&sb, "- %s=%s（%s，%s）\n", ev.ID, ev.Title, ev.StatusLabel, ev.Time)
	}
	sb.WriteString("[RECENT]\n")
	if st.World != nil {
		n := 0
		for i := len(st.World.Log) - 1; i >= 0 && n < 8; i-- {
			e := st.World.Log[i]
			if e.RevertedBy != "" || e.Reverts != "" || e.Hidden {
				continue
			}
			fmt.Fprintf(&sb, "- %s：%s\n", g.pkg.EntityName(e.Target), e.Summary)
			n++
		}
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var ents []string
	for _, id := range ids {
		ents = append(ents, id+"="+g.pkg.EntityName(id))
	}
	fmt.Fprintf(&sb, "[ENTITIES]\n%s\n命名空间：%s\n", strings.Join(ents, "；"), validate.GenNamespace(g.pkg))
	return sb.String()
}

// commitSim 以系统回合提交模拟结果：变更走校验器（来源 world_sim），传闻写进叙事流的“场外”卡片；
// 影响分超过灵敏度阈值时请求偏离提示（标记为场外世界）。
func (s *Session) commitSim(ctx context.Context, slot, branch string, g *game, cmdID string, st *state.State, out simulator.Output, source, tier string) (*state.State, error) {
	id := cmdID + ":sim"
	after := st
	var log *dto.WorldLogV1
	var res *engine.Result
	if len(out.Changes) > 0 {
		cmd := command.Command{ID: id, Kind: command.KindWorldChange, Changes: out.Changes, Source: change.SourceWorldSim, Tier: tier}
		r, a, err := g.eng.Execute(st, cmd)
		if err != nil {
			return st, err
		}
		var rejects []eventstore.Reject
		for _, rj := range r.Rejects {
			b, _ := json.Marshal(rj)
			rejects = append(rejects, eventstore.Reject{Branch: branch, CommandID: id, Turn: r.Turn, Kind: rj.Kind, Proposal: string(b), Reason: rj.Reason})
		}
		if r.Accepted && len(r.Events) > 0 {
			res, after = r, a
			log = g.q.WorldLog(after, r.Events)
		}
		defer func() { _ = s.store.RecordRejects(context.WithoutCancel(ctx), slot, rejects) }()
	}
	if log == nil && len(out.News) == 0 {
		return st, nil
	}
	entry := fromEntry(dto.EntryV1{CommandID: id, Kind: "sim", Turn: st.Turn, Text: strings.Join(out.News, "\n"), World: log, Source: change.SourceWorldSim + ":" + source})
	if res == nil {
		// 只有传闻（变更全部被拒绝或没有变更）：只写叙事流
		return st, s.store.AppendEntries(ctx, slot, []eventstore.Entry{entry})
	}
	empty := ""
	cmd := command.Command{ID: id, Kind: command.KindWorldChange, Changes: out.Changes, Source: change.SourceWorldSim, Tier: tier}
	if err := s.store.CommitTurn(ctx, slot, eventstore.Commit{CommandID: id, Accepted: true, Command: cmd, Result: res, Events: res.Events, After: after,
		Entries: []eventstore.Entry{entry}, Summary: g.summary(after), Narration: &empty}); err != nil {
		if errors.Is(err, eventstore.ErrDuplicateCommand) {
			return st, nil
		}
		return st, err
	}
	s.mu.Lock()
	if s.slot == slot {
		s.st = after
	}
	s.mu.Unlock()
	s.emit(dto.StreamEventV1{Type: "world_sim_done", CommandID: cmdID, Text: strings.Join(out.News, " ")})
	if res.Impact == nil {
		return after, nil
	}
	settings := s.PromptSettings()
	p := change.Decide(*res.Impact, settings)
	if p == nil {
		return after, nil
	}
	_, after2, err := s.requestDecision(ctx, slot, branch, g, id, st, after, *res.Impact, p, settings, log, true)
	if err != nil {
		return after, err
	}
	return after2, nil
}

// SimulateNow 立即做一次场外模拟（日志页 / 调试；按等待处理）。
func (s *Session) SimulateNow(ctx context.Context) (*dto.EntryV1, error) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	slot, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	sl, err := s.store.GetSlot(ctx, slot)
	if err != nil {
		return nil, err
	}
	every := int64(max(s.simEvery(g), 30))
	cmdID := fmt.Sprintf("simnow-%d-%s", st.Turn, NewCommandID()[:6])
	if _, err := s.simulate(ctx, slot, sl.Branch, g, cmdID, st, "wait", st.Minute/every); err != nil {
		return nil, err
	}
	es, err := s.store.EntriesByCommand(ctx, slot, cmdID)
	if err != nil || len(es) == 0 {
		return nil, err
	}
	v := toEntry(es[len(es)-1])
	return &v, nil
}
