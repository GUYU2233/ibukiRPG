// Package structured 解析“叙事 + 世界更新”的合并输出（架构 V0.3 第 11.3–11.5 节）：
// 正文流式显示，`<<<WORLD>>>` 之后的 JSON 只交给解析器；坏 JSON 先本地修复，再由调用方重试一次，最后降级为仅叙事。
package structured

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/knowledge"
	"github.com/GUYU2233/ibukiRPG/internal/world/change"
)

// 分隔符。
const (
	Sep = "<<<WORLD>>>"
	End = "<<<END>>>"
)

// Memory 是 AI 建议写入的人物记忆。
type Memory struct {
	Who        string `json:"who"`
	Text       string `json:"text"`
	Importance int    `json:"importance,omitempty"`
}

// Impact 是 AI 对本回合影响的自评（只能抬高引擎评分）。
type Impact struct {
	Level string `json:"level,omitempty"`
	Why   string `json:"why,omitempty"`
}

// World 是 WORLD 段。
type World struct {
	V           int                `json:"v"`
	Changes     []change.Change    `json:"changes,omitempty"`
	Reveals     []knowledge.Reveal `json:"reveals,omitempty"`
	SceneFacts  []string           `json:"scene_facts,omitempty"`
	Memories    []Memory           `json:"memories,omitempty"`
	Timeline    []change.Change    `json:"timeline,omitempty"`
	Impact      Impact             `json:"impact,omitempty"`
	Suggestions []string           `json:"suggestions,omitempty"`
	Minutes     int                `json:"minutes,omitempty"`
}

// AllChanges 返回 changes + timeline。
func (w *World) AllChanges() []change.Change {
	if w == nil {
		return nil
	}
	return append(append([]change.Change{}, w.Changes...), w.Timeline...)
}

// Empty 报告 WORLD 段是否没有任何内容。
func (w *World) Empty() bool {
	return w == nil || (len(w.Changes) == 0 && len(w.Reveals) == 0 && len(w.Timeline) == 0 && len(w.SceneFacts) == 0 && len(w.Memories) == 0)
}

// Split 把模型输出分成叙事正文与 WORLD 原文。found=false 表示没有分隔符。
func Split(text string) (narration, world string, found bool) {
	i := strings.Index(text, Sep)
	if i < 0 {
		// 缺分隔符但末尾带一个 JSON 对象：尝试从最后一个以 {"v" 或 {"changes" 开头的位置切开
		for _, k := range []string{"{\"v\"", "{\"changes\"", "{ \"v\"", "```json"} {
			if j := strings.LastIndex(text, k); j > 0 {
				return strings.TrimSpace(text[:j]), strings.TrimSpace(text[j:]), true
			}
		}
		return strings.TrimSpace(text), "", false
	}
	world = text[i+len(Sep):]
	if j := strings.Index(world, End); j >= 0 {
		world = world[:j]
	}
	return strings.TrimSpace(text[:i]), strings.TrimSpace(world), true
}

// ErrNoWorld 表示输出里没有 WORLD 段。
var ErrNoWorld = errors.New("missing WORLD section")

// Parse 解析 WORLD 原文：先严格解析，失败则本地修复后再解析。repaired 表示用了修复。
func Parse(raw string) (w *World, repaired bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, ErrNoWorld
	}
	w = &World{}
	if err = json.Unmarshal([]byte(raw), w); err == nil {
		return w, false, nil
	}
	fixed := Repair(raw)
	w = &World{}
	if err2 := json.Unmarshal([]byte(fixed), w); err2 != nil {
		return nil, true, fmt.Errorf("parse WORLD: %w", err)
	}
	// 截断修复可能留下空元素（{"op":"crea → {}），丢弃
	w.Changes = dropEmpty(w.Changes)
	w.Timeline = dropEmpty(w.Timeline)
	return w, true, nil
}

func dropEmpty(cs []change.Change) []change.Change {
	out := cs[:0]
	for _, c := range cs {
		if c.Op != "" {
			out = append(out, c)
		}
	}
	return out
}

// Repair 做本地 JSON 修复：去掉围栏与多余文字、单引号、未转义换行、尾逗号，截断到最后一个完整元素并补全括号。
func Repair(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	if i := strings.LastIndex(s, "```"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "{"); i >= 0 {
		s = s[i:]
	} else {
		return s
	}
	// 逐字符扫描：处理字符串内的换行与单引号字符串，记录括号栈，截断到最后一个完整值
	var out strings.Builder
	var stack []byte
	inStr, esc := false, false
	quote := byte('"')
	lastSafe := 0 // out 中最后一个“完整值之后”的位置
	var safeStack []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
				out.WriteByte(c)
			case c == '\\':
				esc = true
				out.WriteByte(c)
			case c == quote:
				inStr = false
				out.WriteByte('"')
				if len(stack) > 0 {
					lastSafe, safeStack = out.Len(), append([]byte{}, stack...)
				}
			case c == '\n':
				out.WriteString("\\n")
			case c == '"' && quote == '\'':
				out.WriteString("\\\"")
			default:
				out.WriteByte(c)
			}
			continue
		}
		switch c {
		case '"', '\'':
			inStr, quote = true, c
			out.WriteByte('"')
		case '{', '[':
			stack = append(stack, c)
			out.WriteByte(c)
		case '}', ']':
			if len(stack) == 0 {
				continue
			}
			stack = stack[:len(stack)-1]
			// 去掉尾逗号
			cur := strings.TrimRight(out.String(), " \n\t\r")
			if strings.HasSuffix(cur, ",") {
				cur = cur[:len(cur)-1]
				out.Reset()
				out.WriteString(cur)
			}
			out.WriteByte(c)
			lastSafe, safeStack = out.Len(), append([]byte{}, stack...)
			if len(stack) == 0 {
				return out.String()
			}
		default:
			out.WriteByte(c)
			if (c >= '0' && c <= '9') || c == 'e' || c == 'l' { // 数字 / true / false / null 结尾
				if len(stack) > 0 && i+1 < len(s) && strings.ContainsRune(",}] \n", rune(s[i+1])) {
					lastSafe, safeStack = out.Len(), append([]byte{}, stack...)
				}
			}
		}
	}
	// 被截断：回到最后一个完整值，去掉尾部逗号 / 半个键，然后补全括号
	res := out.String()[:lastSafe]
	stack = safeStack
	res = strings.TrimRight(res, " \n\t\r")
	for {
		trimmed := strings.TrimRight(res, " \n\t\r")
		switch {
		case strings.HasSuffix(trimmed, ","):
			res = trimmed[:len(trimmed)-1]
			continue
		case strings.HasSuffix(trimmed, ":"):
			// 键之后没有值：删掉这个键
			k := strings.LastIndex(trimmed[:len(trimmed)-1], "\"")
			k = strings.LastIndex(trimmed[:max(k, 0)], "\"")
			if k > 0 {
				res = trimmed[:k]
				continue
			}
		}
		res = trimmed
		break
	}
	// 对象里最后一个元素是孤立的键（"key"）也要删掉
	if len(stack) > 0 && stack[len(stack)-1] == '{' {
		t := strings.TrimRight(res, " \n\t\r")
		if strings.HasSuffix(t, "\"") {
			k := strings.LastIndex(t[:len(t)-1], "\"")
			before := strings.TrimRight(t[:max(k, 0)], " \n\t\r")
			if k > 0 && (strings.HasSuffix(before, ",") || strings.HasSuffix(before, "{")) {
				res = strings.TrimSuffix(before, ",")
			}
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == '{' {
			res += "}"
		} else {
			res += "]"
		}
	}
	return res
}

// refusalMarkers 是服务商内容审核 / 模型拒答的识别标记（第 11.5 节）。
var refusalMarkers = []string{"content_filter", "data_inspection_failed", "Content Exists Risk", "inappropriate content", "content policy"}

var refusalPhrases = []string{"抱歉，我无法", "抱歉，我不能", "我无法协助", "我不能提供", "无法满足该请求", "I'm sorry, but I can", "I can't help with", "I cannot assist"}

// IsRefusal 判断一次调用是否被内容审核拒绝：错误文案命中标记，或输出命中拒答句式且没有 WORLD 段。
func IsRefusal(text string, err error) bool {
	if err != nil {
		msg := err.Error()
		for _, m := range refusalMarkers {
			if strings.Contains(msg, m) {
				return true
			}
		}
		return false
	}
	if strings.Contains(text, Sep) {
		return false
	}
	t := strings.TrimSpace(text)
	head := t
	if r := []rune(t); len(r) > 60 {
		head = string(r[:60])
	}
	for _, p := range refusalPhrases {
		if strings.Contains(head, p) {
			return true
		}
	}
	for _, m := range refusalMarkers {
		if strings.Contains(t, m) {
			return true
		}
	}
	return false
}

// Filter 包装流式回调：遇到 `<<<WORLD>>>` 即停止向玩家显示（分隔符可能被拆在两个分片里）。
type Filter struct {
	Out     func(string)
	buf     string
	stopped bool
}

// Write 接收一个分片。
func (f *Filter) Write(d string) {
	if f.stopped {
		return
	}
	f.buf += d
	if i := strings.Index(f.buf, Sep); i >= 0 {
		f.emit(f.buf[:i])
		f.buf, f.stopped = "", true
		return
	}
	// 保留可能是分隔符前缀的尾部
	keep := 0
	for k := min(len(Sep)-1, len(f.buf)); k > 0; k-- {
		if strings.HasSuffix(f.buf, Sep[:k]) {
			keep = k
			break
		}
	}
	f.emit(f.buf[:len(f.buf)-keep])
	f.buf = f.buf[len(f.buf)-keep:]
}

// Flush 输出剩余内容（没有遇到分隔符时）。
func (f *Filter) Flush() {
	if !f.stopped && f.buf != "" {
		f.emit(f.buf)
		f.buf = ""
	}
}

func (f *Filter) emit(s string) {
	if s != "" && f.Out != nil {
		f.Out(s)
	}
}
