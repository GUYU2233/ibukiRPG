package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GUYU2233/ibukiRPG/internal/agent/audit"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/ai/router"
	"github.com/GUYU2233/ibukiRPG/internal/ai/structured"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
	"github.com/GUYU2233/ibukiRPG/internal/world/validate"
)

// ---------- 周期性一致性自审（第 12.3 节） ----------

// auditEvery 返回审查间隔（回合）；0 表示关闭。
func (s *Session) auditEvery(g *game) int {
	n := s.router.Settings().AuditEvery
	switch {
	case n < 0:
		return 0
	case n == 0:
		n = g.pkg.Balance.AuditEvery
		if n <= 0 {
			n = 15
		}
	}
	return max(n, 5)
}

// maybeAudit 在回合结束后检查是否到了审查周期；到了就在后台运行（不阻塞回合）。
func (s *Session) maybeAudit(slot string, g *game, st *state.State) {
	every := s.auditEvery(g)
	if every == 0 || st == nil {
		return
	}
	if _, _, ok := s.providerFor(router.TaskAudit); !ok {
		return
	}
	s.auditMu.Lock()
	if s.lastAudit == nil {
		s.lastAudit = map[string]int{}
	}
	last, seen := s.lastAudit[slot]
	if !seen {
		last = st.Turn - st.Turn%every
		s.lastAudit[slot] = last
	}
	due := st.Turn-last >= every
	if due {
		s.lastAudit[slot] = st.Turn // 失败也不立刻重试，等下一个周期
	}
	s.auditMu.Unlock()
	if !due {
		return
	}
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		_, _ = s.auditSlot(ctx, slot, last)
	}
	if s.opts.AuditSync {
		run()
		return
	}
	s.auditWG.Add(1)
	go func() {
		defer s.auditWG.Done()
		run()
	}()
}

// RunAudit 立即对当前存档做一次一致性审查（日志页“立即检查”）。
func (s *Session) RunAudit(ctx context.Context) (dto.AuditV1, error) {
	slot, st, _, err := s.current()
	if err != nil {
		return dto.AuditV1{}, err
	}
	s.auditMu.Lock()
	if s.lastAudit == nil {
		s.lastAudit = map[string]int{}
	}
	since := max(s.lastAudit[slot], st.Turn-30)
	s.lastAudit[slot] = st.Turn
	s.auditMu.Unlock()
	return s.auditSlot(ctx, slot, since)
}

func (s *Session) auditSlot(ctx context.Context, slot string, since int) (dto.AuditV1, error) {
	pr, _, ok := s.providerFor(router.TaskAudit)
	if !ok {
		return dto.AuditV1{}, errors.New("一致性审查需要联网模型（生成设置 → 一致性审查）")
	}
	cur, st, g, err := s.current()
	if err != nil {
		return dto.AuditV1{}, err
	}
	if cur != slot {
		return dto.AuditV1{}, errors.New("存档已切换")
	}
	sl, err := s.store.GetSlot(ctx, slot)
	if err != nil {
		return dto.AuditV1{}, err
	}
	user := s.auditInput(ctx, slot, sl.Branch, g, st, since)
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	resp, err := pr.Generate(cctx, provider.Request{Messages: []provider.Message{{Role: "system", Content: audit.System}, {Role: "user", Content: user}},
		Temperature: 0.2, MaxTokens: 1200, JSON: true})
	if err != nil {
		return dto.AuditV1{}, err
	}
	if structured.IsRefusal(resp.Text, nil) {
		return dto.AuditV1{}, errors.New("审查模型拒绝了这次请求")
	}
	findings, err := audit.Parse(resp.Text)
	if err != nil {
		return dto.AuditV1{}, err
	}
	return s.applyAudit(ctx, slot, g, findings)
}

func (s *Session) auditInput(ctx context.Context, slot, branch string, g *game, st *state.State, since int) string {
	var sb strings.Builder
	// 叙事原文（自上次审查以来，最多约 6000 字，保留最新的）
	var lines []string
	if es, err := s.store.BranchTranscript(ctx, slot, branch, -1, 400, 0); err == nil {
		for _, e := range es {
			if e.Turn <= since || strings.TrimSpace(e.Text) == "" {
				continue
			}
			switch e.Kind {
			case "player":
				lines = append(lines, fmt.Sprintf("回合 %d 玩家：%s", e.Turn, e.Text))
			case "narration":
				lines = append(lines, fmt.Sprintf("回合 %d 叙事：%s", e.Turn, e.Text))
			}
		}
	}
	used := 0
	start := len(lines)
	for start > 0 && used+len([]rune(lines[start-1])) <= 6000 {
		start--
		used += len([]rune(lines[start]))
	}
	sb.WriteString("[NARRATION]\n")
	sb.WriteString(strings.Join(lines[start:], "\n"))
	sb.WriteString("\n[WORLD_CHANGES]\n")
	targets := []string{}
	if st.World != nil {
		for _, e := range st.World.Log {
			if e.Turn <= since || e.RevertedBy != "" {
				continue
			}
			fmt.Fprintf(&sb, "- %s（回合 %d，%s）：%s；原因：%s\n", e.ID, e.Turn, g.pkg.EntityName(e.Target), e.Summary, e.Reason)
			targets = append(targets, e.Target)
		}
	}
	if rs, err := s.store.Rejects(ctx, slot, branch); err == nil {
		head := true
		for _, r := range rs {
			if r.Turn <= since {
				continue
			}
			if head {
				sb.WriteString("[REJECTED]（校验器丢弃的提案，可能意味着叙事与世界不一致）\n")
				head = false
			}
			fmt.Fprintf(&sb, "- 回合 %d：%s\n", r.Turn, r.Reason)
		}
	}
	var inv []string
	for id, n := range st.Player.Inventory {
		if n > 0 {
			inv = append(inv, fmt.Sprintf("%s=%s×%d", id, g.pkg.EntityName(id), n))
		}
	}
	slices.Sort(inv)
	fmt.Fprintf(&sb, "[PLAYER]\n名字：%s；位置：%s；金钱：%d；背包：%s\n", st.Player.Name, g.pkg.EntityName(st.Player.Location), st.Player.Gold, strings.Join(inv, "、"))
	env := g.eng.ValidationEnv(st, change.SourceAudit, router.TierFull)
	ids := append(validate.Present(env), targets...)
	ids = append(ids, st.Player.Location)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var ents []string
	for _, id := range ids {
		if id != "" && id != "player" {
			ents = append(ents, id+"="+g.pkg.EntityName(id))
		}
	}
	fmt.Fprintf(&sb, "[ENTITIES]\n%s\n命名空间：%s\n", strings.Join(ents, "；"), validate.GenNamespace(g.pkg))
	return sb.String()
}

// applyAudit 把审查结果落地：设定 / 文字类修复自动提交（可撤销）；机械状态修复变成待确认的建议；
// 幻觉类发现记为下一次叙事的 [CORRECTION]。最后在叙事流追加一条审查卡片。
func (s *Session) applyAudit(ctx context.Context, slot string, g *game, findings []audit.Finding) (dto.AuditV1, error) {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	cur, st, _, err := s.current()
	if err != nil {
		return dto.AuditV1{}, err
	}
	if cur != slot {
		return dto.AuditV1{}, errors.New("存档已切换")
	}
	id := fmt.Sprintf("audit-%d-%s", st.Turn, NewCommandID()[:6])
	rep := dto.AuditV1{ID: id}
	plan := audit.Classify(findings, g.pkg.KindOf)
	for _, f := range plan.Findings {
		rep.Findings = append(rep.Findings, dto.AuditFindingV1{Kind: f.Kind, Turn: f.Turn, Text: f.Text, Note: f.Note})
	}
	for _, c := range plan.Suggestions {
		b, _ := json.Marshal([]change.Change{c})
		rep.Suggestions = append(rep.Suggestions, dto.AuditSuggestionV1{ID: fmt.Sprintf("%s-s%d", id, len(rep.Suggestions)+1),
			Summary: suggestionSummary(g, c), Reason: c.Reason, Status: "pending", Changes: b})
	}
	autos := plan.Autos
	if len(plan.Corrections) > 0 {
		s.auditMu.Lock()
		if s.corrections == nil {
			s.corrections = map[string][]string{}
		}
		s.corrections[slot] = append(s.corrections[slot], plan.Corrections...)
		s.auditMu.Unlock()
	}
	turn := st.Turn
	if len(autos) > 0 {
		if log, err := s.commitSystemChanges(ctx, slot, g, st, id, change.SourceAudit, autos); err == nil && len(log.Changes) > 0 {
			rep.Fixed = &log
		}
	}
	if len(rep.Findings) == 0 {
		return rep, nil
	}
	if err := s.store.AppendEntries(ctx, slot, []eventstore.Entry{fromEntry(dto.EntryV1{CommandID: id + ":report", Kind: "audit", Turn: turn, Audit: &rep})}); err != nil {
		return rep, err
	}
	fixed := 0
	if rep.Fixed != nil {
		fixed = len(rep.Fixed.Changes)
	}
	s.emit(dto.StreamEventV1{Type: "audit_done", Text: fmt.Sprintf("一致性检查修复了 %d 处，%d 条建议待确认", fixed, len(rep.Suggestions))})
	return rep, nil
}

func suggestionSummary(g *game, c change.Change) string {
	name := g.pkg.EntityName(c.Target)
	if c.Target == "player" || name == "" {
		name = "你"
	}
	switch c.Op {
	case change.OpRetire:
		return name + "：标记为离场 / 死亡"
	case change.OpRestore:
		return name + "：恢复"
	}
	v := string(c.Value)
	if len([]rune(v)) > 40 {
		v = string([]rune(v)[:39]) + "…"
	}
	return fmt.Sprintf("%s · %s → %s", name, c.Path, v)
}

// takeCorrections 取出并清空当前存档的待注入更正提示（叙事提示词的 [CORRECTION] 段）。
func (s *Session) takeCorrections(slot string) []string {
	s.auditMu.Lock()
	defer s.auditMu.Unlock()
	c := s.corrections[slot]
	delete(s.corrections, slot)
	return c
}

// findSuggestion 在叙事流里找到一条审查建议。
func (s *Session) findSuggestion(ctx context.Context, slot, branch, id string) (dto.AuditSuggestionV1, error) {
	es, err := s.store.BranchTranscript(ctx, slot, branch, -1, 2000, 0)
	if err != nil {
		return dto.AuditSuggestionV1{}, err
	}
	for i := len(es) - 1; i >= 0; i-- {
		if es[i].Kind != "audit" {
			continue
		}
		v := toEntry(es[i])
		if v.Audit == nil {
			continue
		}
		for _, sg := range v.Audit.Suggestions {
			if sg.ID == id {
				return sg, nil
			}
		}
	}
	return dto.AuditSuggestionV1{}, errors.New("找不到这条审查建议")
}

// ResolveAuditSuggestion 处理一条审查建议：apply（应用，作为审查来源的系统回合提交，可撤销）/ ignore（忽略）。
func (s *Session) ResolveAuditSuggestion(ctx context.Context, id, action string) (dto.WorldLogV1, error) {
	sl, err := s.slotMeta(ctx)
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	sg, err := s.findSuggestion(ctx, sl.ID, sl.Branch, id)
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	slot, st, g, err := s.current()
	if err != nil {
		return dto.WorldLogV1{}, err
	}
	switch action {
	case "apply":
		var cs []change.Change
		if err := json.Unmarshal(sg.Changes, &cs); err != nil {
			return dto.WorldLogV1{}, err
		}
		return s.commitSystemChanges(ctx, slot, g, st, "audit-sug:"+id, change.SourceAudit, cs)
	case "ignore":
		return dto.WorldLogV1{}, s.store.AppendEntries(ctx, slot, []eventstore.Entry{{CommandID: "audit-sug:" + id + ":ignored", Turn: st.Turn, Kind: "system",
			Text: "已忽略审查建议：" + sg.Summary}})
	}
	return dto.WorldLogV1{}, fmt.Errorf("未知操作 %q", action)
}

// markAudit 根据同页中的“应用 / 忽略”记录更新审查建议的状态。
func markAudit(es []dto.EntryV1) {
	done := map[string]string{}
	for _, e := range es {
		if rest, ok := strings.CutPrefix(e.CommandID, "audit-sug:"); ok {
			if id, ok := strings.CutSuffix(rest, ":ignored"); ok {
				done[id] = "ignored"
			} else {
				done[rest] = "applied"
			}
		}
	}
	if len(done) == 0 {
		return
	}
	for i := range es {
		if es[i].Audit == nil {
			continue
		}
		a := *es[i].Audit
		a.Suggestions = slices.Clone(a.Suggestions)
		for j, sg := range a.Suggestions {
			if st, ok := done[sg.ID]; ok {
				a.Suggestions[j].Status = st
			}
		}
		es[i].Audit = &a
	}
}
