package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// fakeAI 是一个本地的 OpenAI 兼容服务：按玩家输入里的关键词返回预先写好的“叙事 + 世界更新”，
// 让截图夹具走真实的引擎路径（结构化输出解析、校验、知识层、偏离提示、token 用量），而不依赖真实模型。
type fakeAI struct {
	mu      sync.Mutex
	replies []fakeReply
	n       int
}

type fakeReply struct {
	key  string // 玩家输入里的关键词
	text string // 叙事 + <<<WORLD>>> JSON
	used bool
}

var defaultLines = []string{
	"风从钟楼那边吹过来，带着煤烟味。",
	"街角的煤气灯一盏接一盏亮了起来，齿轮在头顶的管道里咔哒作响。",
	"一队搬运工扛着木箱从你身边挤过去，嘴里骂着议会又涨了码头税。",
	"远处传来汽笛声，运煤驳船正慢吞吞地靠岸。",
}

func (f *fakeAI) reply(user string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.replies {
		r := &f.replies[i]
		if !r.used && strings.Contains(user, r.key) {
			r.used = true
			return r.text
		}
	}
	f.n++
	return world(defaultLines[(f.n-1)%len(defaultLines)], `{"v":1}`)
}

func (f *fakeAI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req struct {
		Stream   bool `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &req)
	sys, user := "", ""
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			sys += m.Content
		case "user":
			user = m.Content
		}
	}
	// 只扮演叙事任务；解析器与记忆任务返回错误，引擎回退到规则层。
	if strings.Contains(sys, "的“动作解析器”") || strings.Contains(sys, "的战斗意图解析器") || strings.Contains(sys, "文字 RPG 的记忆") {
		http.Error(w, `{"error":{"message":"fixture: task not scripted"}}`, http.StatusServiceUnavailable)
		return
	}
	// 只用 [PLAYER_INPUT] 之后的内容匹配关键词（上下文里会出现之前的叙事）。
	if i := strings.LastIndex(user, "[PLAYER"); i >= 0 {
		user = user[i:]
	}
	text := "铁闸后传来齿轮咬合的声音，空气里全是机油味。"
	if strings.Contains(sys, "<<<WORLD>>>") {
		text = f.reply(user)
	}
	usage := map[string]int{"prompt_tokens": 2100 + len(user)/3, "completion_tokens": 300 + len([]rune(text))/2}
	if !req.Stream {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": text}, "finish_reason": "stop"}}, "usage": usage})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, chunk := range splitRunes(text, 24) {
		b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": chunk}}}})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	}
	b, _ := json.Marshal(map[string]any{"choices": []any{}, "usage": usage})
	_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", b)
}

func splitRunes(s string, n int) []string {
	rs := []rune(s)
	var out []string
	for len(rs) > 0 {
		k := min(n, len(rs))
		out = append(out, string(rs[:k]))
		rs = rs[k:]
	}
	return out
}

func world(narr string, js string) string {
	return narr + "\n<<<WORLD>>>\n" + js + "\n<<<END>>>"
}

// worldReplies 是 0.2.0 截图脚本的 AI 回复（原创内容，锈钟镇）。
var worldReplies = []fakeReply{
	{key: "口袋", text: world(
		"托克犹豫了一下，还是把传单抖开——上面印着一只被齿轮咬住的手，下面一行小字：『明晚，钟停之时。』\n“别声张，”他压低声音，“议会那帮人要是知道我替煤烟帮跑腿，行会的门我就再也进不去了。”",
		`{"v":1,"changes":[{"op":"patch","target":"brass:character/tock","path":"fields.goals","value":"替煤烟帮送完最后一批货，攒钱给妹妹买药","reason":"托克亲口承认在替煤烟帮跑腿"}],`+
			`"reveals":[{"entity":"brass:character/tock","fields":["faction"],"channel":"dialogue","source":"brass:character/tock"},`+
			`{"entity":"brass:event/clock_stop","fields":["existence"],"level":1,"channel":"rumor","source":"brass:character/tock"},`+
			`{"entity":"brass:faction/soot_gang","fields":["existence","name"],"channel":"dialogue","source":"brass:character/tock"}],`+
			`"scene_facts":["托克口袋里有一张煤烟帮的传单"],"impact":{"level":"minor"},"suggestions":["追问“钟停之时”","把传单交给议会"]}`)},
	{key: "仓库", text: world(
		"火舌顺着油迹一路舔上了木梁。浓烟里有人在喊你的名字——是托克。你冲进去的时候，横梁已经塌了下来……",
		`{"v":1,"changes":[{"op":"retire","target":"brass:character/tock","value":{"reason":"death","text":"死于北码头三号仓的大火"},"reason":"横梁塌落，托克没能逃出来"},`+
			`{"op":"patch","target":"brass:location/north_dock","path":"fields.description","value":"三号仓只剩下焦黑的木桩，煤油味混着焦糊味。","reason":"仓库被烧毁"}],`+
			`"impact":{"level":"major","why":"重要人物死亡"}}`)},
	{key: "小铆钉", text: world(
		"小铆钉二话不说，把托克架到了工坊后屋，翻出一卷干净绷带。“欠我一顿肉包子，”她冲托克眨眨眼。",
		`{"v":1,"changes":[{"op":"patch","target":"brass:character/rivet>brass:character/tock","path":"dims.trust","value":3,"reason":"小铆钉照顾了受伤的托克"}],"impact":{"level":"minor"},"suggestions":["问托克“钟停之时”到底是什么","去北码头看看火场"]}`)},
	{key: "拖出来", text: world(
		"你扯下帆布裹住托克，在横梁塌下来之前把他拖出了仓库。他咳得直不起腰，却死死攥着那张传单。",
		`{"v":1,"changes":[{"op":"patch","target":"brass:location/north_dock","path":"fields.description","value":"三号仓的屋顶塌了一半，焦黑的横梁斜插在水里。","reason":"仓库失火，托克被救出"}],"scene_facts":["托克被救了出来"],"impact":{"level":"minor"}}`)},
}

// runWorld 用本地假 AI 跑一段 0.2.0 开放世界试玩，导出 v02_*.json（主界面 / 战斗裁定 / 偏离提示 / 时间线 / 世界面板 / 生成设置）。
func runWorld(seed uint64, files map[string]json.RawMessage) {
	fake := &fakeAI{replies: append([]fakeReply{}, worldReplies...)}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	call("new_game", "", map[string]any{"seed": seed, "pack_id": "brass_trial"})
	call("configure_ai", "", map[string]any{
		"providers": []map[string]any{
			{"id": "deepseek", "kind": "custom", "label": "DeepSeek", "base_url": srv.URL + "/v1", "model": "deepseek-chat", "api_key": "fixture"},
			{"id": "qwen", "kind": "qwen", "label": "通义千问", "model": "qwen-plus"},
			{"id": "phone", "kind": "llamacpp", "label": "本地", "base_url": "http://127.0.0.1:1/v1", "model": "Qwen2.5-3B Q4_K_M", "size_b": 3},
		},
		"settings": map[string]any{"mode": "unified", "unified": map[string]string{"provider": "deepseek"}, "show_token_usage": true},
	})
	n := 0
	id := func() string { n++; return fmt.Sprintf("w-%d", n) }
	say := func(text string) { call("submit_text", id(), map[string]string{"text": text}) }
	say("和欧琳聊聊")
	say("去钟楼广场")
	say("去竞技场")
	say("走过去，问托克口袋里藏的是什么")
	files["v02_game.json"] = call("get_bundle", "", nil)
	// 自由战斗：一句话描述行动 → 解析 / 合理性 / 修正 / 掷骰 / 程度
	call("quick_action", id(), map[string]string{"kind": "combat", "action": "start", "target": "brass:encounter/trial_bout1", "label": "迎战"})
	say("趁它扑过来的空档，扫腿绊倒发条鼠，再抡扳手砸它背上的发条钥匙")
	files["v02_combat.json"] = call("get_bundle", "", nil)
	for i := 0; i < 40; i++ {
		var sc struct {
			Combat *json.RawMessage `json:"combat"`
		}
		_ = json.Unmarshal(call("get_scene", "", nil), &sc)
		if sc.Combat == nil {
			break
		}
		call("quick_action", id(), map[string]string{"kind": "combat", "action": "attack", "label": "攻击"})
	}
	call("create_checkpoint", "", map[string]string{"name": "进议会之前"})
	say("去钟楼广场")
	say("去北码头")
	say("踹开仓库门，把油桶推进火里，逼他们出来")
	files["v02_decision.json"] = call("get_bundle", "", nil)
	var d struct {
		Decision struct {
			ID string `json:"id"`
		} `json:"decision"`
	}
	_ = json.Unmarshal(call("get_pending_decision", "", nil), &d)
	if d.Decision.ID != "" {
		call("resolve_decision", "", map[string]any{"id": d.Decision.ID, "action": "rollback"})
	}
	say("冲进火场，把托克拖出来")
	say("把托克交给小铆钉照顾")
	files["v02_timeline.json"] = call("get_timeline", "", nil)
	files["v02_after.json"] = call("get_bundle", "", nil)
	for _, tab := range []string{"characters", "relations", "codex", "equipment", "map", "factions", "timeline", "log"} {
		files["v02_world_"+tab+".json"] = call("get_world_panel", "", map[string]string{"tab": tab})
	}
	files["v02_tasks.json"] = call("get_tasks", "", nil)
	files["v02_creation.json"] = call("get_creation", "", map[string]string{"pack_id": "brass_trial"})
	files["v02_creation_review.json"] = call("review_creation", "", map[string]any{"pack_id": "brass_trial", "creation": map[string]any{
		"name": "白鸦", "custom": true, "background": "scavenger", "personality": "嘴硬心软，记仇也记恩",
		"story":      "在下水道长大，据说会一点魔法，能让齿轮自己转起来。",
		"attributes": map[string]int{"agility": 4, "strength": 3, "resolve": 1},
	}})
	call("configure_ai", "", map[string]any{"kind": "offline"})
}
