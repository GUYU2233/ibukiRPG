// Package memory 是记忆整理 Agent（Memory Agent，第 15 节）：在关键路径之外把对话记录压缩为摘要。
//
//   - 滚动对话摘要（rolling）：保留最近 Keep 个回合原文，更早的回合每攒够 Chunk 个就压成一条摘要；
//   - 长期记忆（longterm）：滚动摘要超过 MaxRolling 条时，把较早的几条再压缩成一条长期记忆；
//   - NPC 记忆摘要（npc）：某个 NPC 的记忆条目超过 NPCThreshold 时，为其生成一条“这个人记得什么”的摘要。
//
// 每条摘要都记录“压缩掉了什么”（Compressed），Agent 看到后知道细节需要用 memory.search / pack.search 去查。
// 默认使用确定性的离线摘要器（无网络、可重放）；可选接入 LLM 摘要器，失败时回退离线结果。
// 摘要不是游戏事实，删除或重建都不影响事件重放。
package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/agent/tools"
	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/core/state"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
	"github.com/GUYU2233/ibukiRPG/internal/storage/eventstore"
)

// Config 是整理参数。
type Config struct {
	Keep         int // 最近几个回合保留原文（不摘要）
	Chunk        int // 每条滚动摘要至少覆盖几个回合
	MaxRolling   int // 滚动摘要超过这个条数就做长期压缩
	NPCThreshold int // NPC 记忆条目超过这个数就生成 NPC 摘要
	MaxRunes     int // 单条摘要的字数上限
}

// DefaultConfig 返回默认参数。
func DefaultConfig() Config {
	return Config{Keep: 4, Chunk: 6, MaxRolling: 4, NPCThreshold: 8, MaxRunes: 260}
}

// 被压缩掉的内容类别（写进摘要，提醒 Agent 需要时去查）。
var (
	RollingCompressed  = []string{"逐字对话", "环境描写", "检定数值", "可用 memory.search 查询"}
	LongTermCompressed = []string{"次要回合细节", "人物台词原文", "可用 memory.search / pack.search 查询"}
	NPCCompressed      = []string{"较早的交谈原话", "零散的观察", "可用 memory.search 查询"}
)

// Summarizer 把若干条目压缩成一句摘要。
type Summarizer interface {
	Summarize(ctx context.Context, kind string, items []string, maxRunes int) (string, error)
}

// Offline 是确定性离线摘要器：按顺序拼接条目并截断。
type Offline struct{}

// Summarize 实现 Summarizer。
func (Offline) Summarize(_ context.Context, _ string, items []string, maxRunes int) (string, error) {
	return clipRunes(strings.Join(items, "；"), maxRunes), nil
}

// LLM 是大模型摘要器（离线结果作为兜底）。
type LLM struct{ Provider provider.Provider }

const memorySystem = `[CORE_RULES]
你是文字 RPG 的记忆整理员。把给出的条目压缩成一段中文摘要，只保留：发生了什么、和谁有关、结果如何、留下的线索或承诺。
- 不要添加条目里没有的事实；不要写骰子数字；用第二人称“你”指代玩家。
- 只输出摘要本身，不超过指定字数。`

// Summarize 实现 Summarizer。
func (l LLM) Summarize(ctx context.Context, kind string, items []string, maxRunes int) (string, error) {
	resp, err := l.Provider.Generate(ctx, provider.Request{Messages: []provider.Message{
		{Role: "system", Content: memorySystem},
		{Role: "user", Content: fmt.Sprintf("[KIND]%s\n[LIMIT]%d 字\n[ITEMS]\n- %s", kind, maxRunes, strings.Join(items, "\n- "))},
	}, Temperature: 0.2, MaxTokens: 400})
	if err != nil {
		return "", err
	}
	t := strings.TrimSpace(resp.Text)
	if t == "" {
		return "", fmt.Errorf("empty summary")
	}
	return clipRunes(t, maxRunes), nil
}

// Agent 是记忆整理 Agent。
type Agent struct {
	Config     Config
	Summarizer Summarizer // nil 表示离线
}

func (a *Agent) cfg() Config {
	c, d := a.Config, DefaultConfig()
	if c.Keep <= 0 {
		c.Keep = d.Keep
	}
	if c.Chunk <= 0 {
		c.Chunk = d.Chunk
	}
	if c.MaxRolling <= 0 {
		c.MaxRolling = d.MaxRolling
	}
	if c.NPCThreshold <= 0 {
		c.NPCThreshold = d.NPCThreshold
	}
	if c.MaxRunes <= 0 {
		c.MaxRunes = d.MaxRunes
	}
	return c
}

func (a *Agent) summarize(ctx context.Context, kind string, items []string, maxRunes int) string {
	if a.Summarizer != nil {
		if t, err := a.Summarizer.Summarize(ctx, kind, items, maxRunes); err == nil {
			return t
		}
	}
	t, _ := Offline{}.Summarize(ctx, kind, items, maxRunes)
	return t
}

// Plan 计算需要删除与新增的摘要（纯函数：同样的输入得到同样的输出）。
func (a *Agent) Plan(ctx context.Context, p *loader.Package, st *state.State, transcript []eventstore.Entry, mems []eventstore.Memory) (remove []int64, add []eventstore.Memory) {
	c := a.cfg()
	// ---- 滚动摘要 ----
	covered := 0
	for _, m := range mems {
		if (m.Kind == "rolling" || m.Kind == "longterm") && m.ToTurn > covered {
			covered = m.ToTurn
		}
	}
	turns := map[int]*turnText{}
	var order []int
	for _, e := range transcript {
		if e.Turn <= covered || e.Turn > st.Turn-c.Keep || e.Turn <= 0 {
			continue
		}
		t := turns[e.Turn]
		if t == nil {
			t = &turnText{}
			turns[e.Turn] = t
			order = append(order, e.Turn)
		}
		t.add(e)
	}
	sort.Ints(order)
	var rolling []eventstore.Memory
	for len(order) >= c.Chunk {
		n := c.Chunk
		if len(order) < c.Chunk*2 {
			n = len(order) // 余下不足两块时并成一块
		}
		chunk := order[:n]
		order = order[n:]
		var items []string
		for _, t := range chunk {
			if s := turns[t].line(); s != "" {
				items = append(items, s)
			}
		}
		if len(items) == 0 {
			continue
		}
		text := a.summarize(ctx, "rolling", items, c.MaxRunes)
		rolling = append(rolling, eventstore.Memory{Kind: "rolling", FromTurn: chunk[0], ToTurn: chunk[len(chunk)-1], Text: text, Compressed: RollingCompressed})
	}
	add = append(add, rolling...)

	// ---- 长期压缩 ----
	var allRolling []eventstore.Memory
	for _, m := range mems {
		if m.Kind == "rolling" {
			allRolling = append(allRolling, m)
		}
	}
	allRolling = append(allRolling, rolling...)
	if len(allRolling) > c.MaxRolling {
		old := allRolling[:len(allRolling)-2] // 最近两条滚动摘要保留
		var items []string
		for _, m := range old {
			items = append(items, keyPoints(m.Text))
		}
		lt := eventstore.Memory{Kind: "longterm", FromTurn: old[0].FromTurn, ToTurn: old[len(old)-1].ToTurn,
			Text: a.summarize(ctx, "longterm", items, c.MaxRunes), Compressed: LongTermCompressed}
		// 新生成的滚动摘要还没入库：直接不写；已入库的删除
		var keep []eventstore.Memory
		for _, m := range add {
			drop := false
			for _, o := range old {
				if o.ID == 0 && o.FromTurn == m.FromTurn && o.ToTurn == m.ToTurn && m.Kind == "rolling" {
					drop = true
				}
			}
			if !drop {
				keep = append(keep, m)
			}
		}
		add = append([]eventstore.Memory{lt}, keep...)
		for _, o := range old {
			if o.ID != 0 {
				remove = append(remove, o.ID)
			}
		}
	}

	// ---- NPC 摘要 ----
	last := map[string]eventstore.Memory{}
	for _, m := range mems {
		if m.Kind == "npc" {
			last[m.Subject] = m
		}
	}
	for _, id := range p.NPCIDs {
		n := st.NPCs[id]
		if n == nil {
			continue
		}
		count := len(n.Memories) + len(n.Exchanges) + len(n.Episodes)
		if count <= c.NPCThreshold {
			continue
		}
		latest := 0
		for _, x := range n.Memories {
			latest = max(latest, x.Turn)
		}
		for _, x := range n.Exchanges {
			latest = max(latest, x.Turn)
		}
		for _, x := range n.Episodes {
			latest = max(latest, x.Turn)
		}
		prev, had := last[id]
		if had && latest-prev.ToTurn < c.Chunk {
			continue // 变化不大，不必重写
		}
		first := latest
		for _, x := range n.Memories {
			first = min(first, x.Turn)
		}
		for _, x := range n.Exchanges {
			first = min(first, x.Turn)
		}
		items := npcItems(p, id, n)
		m := eventstore.Memory{Kind: "npc", Subject: id, FromTurn: first, ToTurn: latest,
			Text: a.summarize(ctx, "npc", items, c.MaxRunes), Compressed: NPCCompressed}
		if had {
			remove = append(remove, prev.ID)
		}
		add = append(add, m)
	}
	return remove, add
}

func npcItems(p *loader.Package, id string, n *state.NPC) []string {
	items := []string{fmt.Sprintf("%s对你%s，交谈过 %d 次", p.EntityName(id), n.Attitude(), n.Talks)}
	eps := append([]state.Episode(nil), n.Episodes...)
	sort.SliceStable(eps, func(i, j int) bool { return eps[i].Importance > eps[j].Importance })
	for i, e := range eps {
		if i >= 3 {
			break
		}
		items = append(items, "记得："+e.Text)
	}
	if k := len(n.Exchanges); k > 0 {
		for _, x := range n.Exchanges[max(0, k-2):] {
			items = append(items, "聊过："+clipRunes(x.Text, 40))
		}
	}
	var notable []string
	for _, m := range n.Memories {
		if m.Notable {
			notable = append(notable, clipRunes(m.Text, 30))
		}
	}
	if len(notable) > 3 {
		notable = notable[len(notable)-3:]
	}
	if len(notable) > 0 {
		items = append(items, "看到过："+strings.Join(notable, "，"))
	}
	return items
}

// keyPoints 从一条滚动摘要里挑出关键条目（主线 / 战斗 / 获得 / 人物），做长期压缩。
func keyPoints(text string) string {
	parts := strings.Split(text, "；")
	var keep []string
	for _, s := range parts {
		for _, k := range []string{"主线", "锚点", "击败", "战斗", "获得", "得到", "交给", "答应", "发现", "来到", "抵达", "偏离", "自由推演", "结局", "事件"} {
			if strings.Contains(s, k) {
				keep = append(keep, s)
				break
			}
		}
	}
	if len(keep) == 0 && len(parts) > 0 {
		keep = parts[:1]
	}
	return strings.Join(keep, "；")
}

type turnText struct {
	turn      int
	input     string
	narration string
	story     []string
}

func (t *turnText) add(e eventstore.Entry) {
	t.turn = e.Turn
	switch e.Kind {
	case "player":
		t.input = e.Text
	case "narration":
		if t.narration == "" {
			t.narration = firstSentence(e.Text)
		}
	case "story", "system":
		if len(t.story) < 2 && strings.TrimSpace(e.Text) != "" {
			t.story = append(t.story, clipRunes(firstSentence(e.Text), 30))
		}
	}
}

func (t *turnText) line() string {
	var parts []string
	if t.input != "" {
		parts = append(parts, "你「"+clipRunes(t.input, 16)+"」")
	}
	if t.narration != "" {
		parts = append(parts, clipRunes(t.narration, 28))
	}
	parts = append(parts, t.story...)
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("第%d回合 %s", t.turn, strings.Join(parts, "→"))
}

func firstSentence(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	for i, r := range s {
		if strings.ContainsRune("。！？!?", r) {
			return s[:i+utf8.RuneLen(r)]
		}
	}
	return s
}

func clipRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

// ToSummaries 把存储的摘要转换为检索工具使用的格式（只保留不晚于当前回合的）。
func ToSummaries(ms []eventstore.Memory, turn int) []tools.Summary {
	var out []tools.Summary
	for _, m := range ms {
		if m.FromTurn > turn {
			continue
		}
		out = append(out, tools.Summary{Kind: m.Kind, Subject: m.Subject, FromTurn: m.FromTurn, ToTurn: m.ToTurn, Text: m.Text, Compressed: m.Compressed})
	}
	return out
}

// StorySoFar 渲染 [STORY_SO_FAR] 段：长期记忆 + 最近的滚动摘要（旧 → 新），并注明被压缩掉的内容。
// 没有摘要时返回空串（不改变提示词）。
func StorySoFar(sums []tools.Summary, maxRunes int) string {
	var lines []string
	var comp []string
	seen := map[string]bool{}
	for _, s := range sums {
		if s.Kind != "rolling" && s.Kind != "longterm" {
			continue
		}
		tag := "摘要"
		if s.Kind == "longterm" {
			tag = "长期记忆"
		}
		lines = append(lines, fmt.Sprintf("- （%s·第%d-%d回合）%s", tag, s.FromTurn, s.ToTurn, s.Text))
		for _, c := range s.Compressed {
			if !seen[c] {
				seen[c] = true
				comp = append(comp, c)
			}
		}
	}
	if len(lines) == 0 {
		return ""
	}
	// 超长时从最早的开始丢（最近的最重要）
	for len(lines) > 1 && utf8.RuneCountInString(strings.Join(lines, "\n")) > maxRunes {
		lines = lines[1:]
	}
	return "[STORY_SO_FAR]（早期回合已被压缩为摘要；已省略：" + strings.Join(comp, "、") + "。细节不确定时先检索，不要猜）\n" + strings.Join(lines, "\n") + "\n"
}
