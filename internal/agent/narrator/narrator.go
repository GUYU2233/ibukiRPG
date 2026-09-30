package narrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/ai/provider"
	"github.com/GUYU2233/ibukiRPG/internal/narrative/guard"
)

// Output 是叙事结果。
type Output struct {
	Text      string       `json:"text"`
	Source    string       `json:"source"` // template / ai / template(fallback) / template(guard)
	Corrected bool         `json:"corrected"`
	Guard     guard.Report `json:"guard"`
	Err       string       `json:"error,omitempty"`
}

// Narrator 把已结算的事件讲给玩家。
type Narrator interface {
	Name() string
	Narrate(ctx context.Context, b Brief, onDelta func(string)) Output
}

// Template 是离线模板叙事器：直接输出事实稿。永不失败。
type Template struct{}

// Name 返回叙事器名。
func (Template) Name() string { return "template" }

// Narrate 实现 Narrator。模板文本同样会经过 Guard（测试保证其总是通过）。
func (Template) Narrate(_ context.Context, b Brief, onDelta func(string)) Output {
	if onDelta != nil {
		for _, chunk := range chunks(b.Base, 12) {
			onDelta(chunk)
		}
	}
	return Output{Text: b.Base, Source: "template", Guard: guard.Check(b.Base, b.Facts, b.Guard)}
}

func chunks(s string, n int) []string {
	r := []rune(s)
	var out []string
	for i := 0; i < len(r); i += n {
		j := min(i+n, len(r))
		out = append(out, string(r[i:j]))
	}
	return out
}

// LLM 是大模型叙事器：流式输出，结束后经过 Narrative Guard；违规或失败时降级为模板。
type LLM struct {
	Provider provider.Provider
}

// Name 返回叙事器名。
func (l *LLM) Name() string { return "ai:" + l.Provider.Name() }

const narratorSystem = `[CORE_RULES]
你是中文文字 RPG 的叙述者（GM）。游戏引擎已经确定了本回合发生的一切，你只负责把它讲得生动。
- 用第二人称“你”称呼玩家，120-220 字，一到三段。
- 只能叙述 [RESOLVED_EVENTS] 与 [IMMUTABLE_FACTS] 中的结果：不得改变成败，不得杜撰金钱、物品、伤亡或地点变化。
- 不在 [PRESENT] 中的人物不能出场。不要透露任何人物的秘密或来历。
- 人物台词可以润色，但态度与含义必须与事实稿一致。
- 不要写骰子数字、不要列选项、不要跳出角色解释规则（系统会单独展示）。
- 若有 [NPC_MEMORY]：人物记得之前和你聊过什么、看到过什么。台词要符合这些记忆与“本回合要表达的意思”，
  可以自然地提起上次的话题或刚才看到的事；不要把说过的开场白原样再说一遍，也不要让人物知道记忆之外的事。
[STYLE]
简洁、有画面感的奇幻小说笔调，细节克制，结尾留一点让玩家想接着行动的余味。`

// BuildMessages 构造叙事请求（公开给 Eval / 测试复用）。
func BuildMessages(b Brief) []provider.Message {
	facts, _ := json.Marshal(b.Facts)
	var present []string
	for _, p := range b.Present {
		present = append(present, fmt.Sprintf("%s（%s，对你%s）：%s", p.Name, p.Role, p.Attitude, p.Description))
	}
	user := fmt.Sprintf("%s[SCENE]\n地点：%s；时间：%s\n场景事实：%s\n[PRESENT]\n%s\n%s[IMMUTABLE_FACTS]\n%s\n[RESOLVED_EVENTS]（事实稿，请据此改写）\n%s\n[PLAYER_INPUT]\n%s",
		b.StorySoFar, b.Location, b.Time, strings.Join(b.SceneFacts, "；"), strings.Join(present, "\n"), MemorySection(b.Memories), facts, b.Base, b.Input)
	sys := narratorSystem
	if len(b.Lookup) > 0 && b.Retrieval != "" {
		sys += "\n" + strings.TrimRight(b.Retrieval, "\n")
		user += "\n[LOOKUP]\n需要先查证：" + strings.Join(b.Lookup, "；") + "。查不到的内容不要写。"
	}
	return []provider.Message{{Role: "system", Content: sys}, {Role: "user", Content: user}}
}

// MemorySection 渲染 [NPC_MEMORY] 段落（没有记忆时为空串）。
func MemorySection(ms []NPCMemory) string {
	if len(ms) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[NPC_MEMORY]（只包含该人物自己知道的事）\n")
	for _, m := range ms {
		fmt.Fprintf(&sb, "%s（对你%s；", m.Name, m.Attitude)
		if m.Talks == 0 {
			sb.WriteString("第一次和你交谈）\n")
		} else {
			fmt.Fprintf(&sb, "已和你交谈 %d 次，上次在%s）\n", m.Talks, m.SinceLast)
		}
		if len(m.Exchanges) > 0 {
			sb.WriteString("- 最近的交谈：" + strings.Join(m.Exchanges, "；") + "\n")
		}
		if len(m.Topics) > 0 {
			sb.WriteString("- 谈过的话题：" + strings.Join(m.Topics, "、") + "\n")
		}
		if len(m.Episodes) > 0 {
			sb.WriteString("- 记得的事：" + strings.Join(m.Episodes, "；") + "\n")
		}
		if len(m.Seen) > 0 {
			sb.WriteString("- 上次交谈后看到：" + strings.Join(m.Seen, "；") + "\n")
		}
		if m.Intent != "" {
			note := "新话题"
			if m.Repeat {
				note = "之前说过，这次应表现出“说过了”"
			}
			fmt.Fprintf(&sb, "- 本回合要表达的意思：%s（%s）\n", m.Intent, note)
		}
	}
	return sb.String()
}

// Narrate 实现 Narrator。
//
// 当 Brief.Lookup 非空（信息不足）时先跑只读检索循环（PlayerScope）：支持函数调用的模型自己调用工具，
// 不支持的改为关键词预检索；检索后的最终叙事同样经过 Guard。
func (l *LLM) Narrate(ctx context.Context, b Brief, onDelta func(string)) Output {
	msgs := BuildMessages(b)
	if len(b.Lookup) > 0 && b.Research != nil {
		answer, out := b.Research(ctx, l.Provider, msgs, b.Input+" "+strings.Join(b.Lookup, " "))
		if strings.TrimSpace(answer) != "" {
			if onDelta != nil {
				for _, c := range chunks(answer, 12) {
					onDelta(c)
				}
			}
			return l.finish(b, answer)
		}
		msgs = out
	}
	resp, err := l.Provider.Stream(ctx, provider.Request{Messages: msgs, Temperature: 0.8, MaxTokens: 600}, onDelta)
	if err != nil {
		return Output{Text: b.Base, Source: "template(fallback)", Corrected: true, Err: err.Error(), Guard: guard.Check(b.Base, b.Facts, b.Guard)}
	}
	return l.finish(b, resp.Text)
}

func (l *LLM) finish(b Brief, text string) Output {
	text = strings.TrimSpace(text)
	rep := guard.Check(text, b.Facts, b.Guard)
	if !rep.OK {
		// Template Fallback：宁可朴素，也不能篡改结果
		return Output{Text: b.Base, Source: "template(guard)", Corrected: true, Guard: rep}
	}
	return Output{Text: text, Source: "ai", Guard: rep}
}
