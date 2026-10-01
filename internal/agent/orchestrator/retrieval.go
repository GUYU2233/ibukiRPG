package orchestrator

import (
	"context"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/agent/memory"
	"github.com/GUYU2233/ibukiRPG/internal/agent/narrator"
	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/prompt/sections"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
)

// 事件日志只取最近这么多条给检索工具（memory.search 的日志部分）。
const toolEventWindow = 400

// ToolEnv 返回当前存档的只读检索环境（故事包 + 状态 + 最近事件 + 记忆摘要）。
// 供 MCP 适配器、移动端 call_tool 与测试使用；工具本身不写任何状态。
func (s *Session) ToolEnv(ctx context.Context) (*tools.Env, error) {
	slot, st, g, err := s.current()
	if err != nil {
		return nil, err
	}
	s.memWait() // 读到最新的摘要
	return s.toolEnv(ctx, slot, g, st), nil
}

func (s *Session) toolEnv(ctx context.Context, slot string, g *game, st *state.State) *tools.Env {
	env := &tools.Env{Pkg: g.pkg, State: st, Q: g.q}
	if sl, err := s.store.GetSlot(ctx, slot); err == nil {
		after := max(sl.LastSeq-toolEventWindow, 0)
		if evs, err := s.store.Events(ctx, slot, after, toolEventWindow); err == nil {
			env.Events = evs
		}
	}
	if ms, err := s.store.Memories(ctx, slot); err == nil {
		env.Summaries = memory.ToSummaries(ms, st.Turn)
	}
	return env
}

// 玩家这样说时，多半在回忆早期的事（可能已被压缩为摘要）。
var recallCues = []string{"之前", "上次", "以前", "当初", "那次", "还记得", "记不记得", "曾经", "刚开始", "最早"}

// lookupReasons 判断叙述者是否信息不足、需要先检索（返回原因；为空表示不需要）。
func lookupReasons(b narrator.Brief, st *state.State, env *tools.Env) []string {
	var out []string
	if refs := tools.UnknownRefs(env, tools.Player(), b.Input, b.Known()); len(refs) > 0 {
		out = append(out, "玩家提到了上下文里没有的："+strings.Join(refs, "、"))
	}
	if b.StorySoFar != "" {
		for _, c := range recallCues {
			if strings.Contains(b.Input, c) {
				out = append(out, "玩家在回忆早期的事，而那部分已被压缩为摘要")
				break
			}
		}
	}
	return out
}

// prepareBrief 为 AI 叙述者补上记忆摘要与检索设置（离线模板叙事不需要）。
// 只有确有需要时才加段落，避免无谓地改变提示词（录音哈希）与增加延迟。
func (s *Session) prepareBrief(ctx context.Context, slot string, g *game, after *state.State, b *narrator.Brief) {
	env := s.toolEnv(ctx, slot, g, after)
	b.StorySoFar = memory.StorySoFar(env.Summaries, 900)
	b.Lookup = lookupReasons(*b, after, env)
	if len(b.Lookup) == 0 {
		return
	}
	b.Retrieval = sections.Render(g.pkg, sections.Narrator, "TOOLS") + sections.Render(g.pkg, sections.Narrator, "RETRIEVAL_POLICY")
	b.Research = func(ctx context.Context, p provider.Provider, msgs []provider.Message, hint string) (string, []provider.Message) {
		o := (&tools.Loop{Provider: p, Env: env, Scope: tools.Player(), Temperature: 0.8}).Run(ctx, msgs, hint)
		if o.Answer != nil {
			return o.Answer.Text, nil
		}
		return "", tools.Flatten(msgs, o)
	}
}

// ---------- 记忆 Agent（关键路径之外） ----------

// scheduleMemory 在回合结束后整理记忆摘要。默认在后台 goroutine 里运行（不阻塞玩家），
// Options.MemorySync 为 true 时同步运行（测试）。同一时间只有一个整理任务。
func (s *Session) scheduleMemory(slot string, g *game, st *state.State) {
	s.memQ.Lock()
	s.memJobs = append(s.memJobs, memJob{slot: slot, g: g, st: st})
	s.memQ.Unlock()
	run := func() {
		s.memMu.Lock()
		defer s.memMu.Unlock()
		s.memQ.Lock()
		if len(s.memJobs) == 0 {
			s.memQ.Unlock()
			return
		}
		job := s.memJobs[0]
		s.memJobs = s.memJobs[1:]
		s.memQ.Unlock()
		slot := job.slot
		ctx := context.Background()
		ag := &memory.Agent{}
		if s.opts.MemoryLLM {
			s.mu.RLock()
			cfg := s.ai
			s.mu.RUnlock()
			if cfg.Online() {
				ag.Summarizer = memory.LLM{Provider: provider.NewOpenAICompatible(cfg, s.opts.Transport)}
			}
		}
		tr, err := s.store.Transcript(ctx, slot, 600, 0)
		if err != nil {
			return
		}
		ms, err := s.store.Memories(ctx, slot)
		if err != nil {
			return
		}
		remove, add := ag.Plan(ctx, job.g.pkg, job.st, tr, ms)
		if len(remove) == 0 && len(add) == 0 {
			return
		}
		_ = s.store.ReplaceMemories(ctx, slot, remove, add)
	}
	if s.opts.MemorySync {
		run()
		return
	}
	s.memWG.Add(1)
	go func() {
		defer s.memWG.Done()
		run()
	}()
}

func (s *Session) memWait() { s.memWG.Wait() }

// Memories 返回当前存档的记忆摘要（调试 / UI）。
func (s *Session) Memories(ctx context.Context) ([]eventstore.Memory, error) {
	slot, _, _, err := s.current()
	if err != nil {
		return nil, err
	}
	s.memWait()
	return s.store.Memories(ctx, slot)
}

// SlotToolEnv 以只读方式载入指定存档（slotID 为空时取最近更新的存档）并返回检索环境，
// 不切换当前存档、不补写叙事、不执行任何命令（MCP 适配器使用）。
func (s *Session) SlotToolEnv(ctx context.Context, slotID string) (*tools.Env, string, error) {
	if slotID == "" {
		slots, err := s.store.ListSlots(ctx)
		if err != nil {
			return nil, "", err
		}
		if len(slots) == 0 {
			return nil, "", ErrNoGame
		}
		slotID = slots[0].ID
	}
	sl, err := s.store.GetSlot(ctx, slotID)
	if err != nil {
		return nil, "", err
	}
	g, err := s.gameForSlot(sl)
	if err != nil {
		return nil, "", err
	}
	st, err := s.store.Load(ctx, slotID)
	if err != nil {
		return nil, "", err
	}
	return s.toolEnv(ctx, slotID, g, st), slotID, nil
}
