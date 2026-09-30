// Command cli 是 ibukiRPG 的命令行文字客户端（Linux / Windows 开发与试玩用）。
//
// 它与 Android 使用完全相同的版本化 JSON API（internal/adapter/mobile），
// 因此 CLI 能玩通的流程，移动端也走同一条路径。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
	"github.com/GUYU2233/ibukiRPG/internal/buildinfo"
)

type options struct {
	dataDir  string
	provider string
	baseURL  string
	model    string
	apiKey   string
	newGame  bool
	player   string
	seed     uint64
}

func defaultDataDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "ibukiRPG")
	}
	return "saves"
}

func main() {
	var o options
	showVersion := flag.Bool("version", false, "打印版本并退出")
	flag.StringVar(&o.dataDir, "data", defaultDataDir(), "存档目录")
	flag.StringVar(&o.provider, "ai", envOr("IBUKI_AI_PROVIDER", "offline"), "AI 模式：offline / deepseek / qwen / custom")
	flag.StringVar(&o.baseURL, "base-url", os.Getenv("IBUKI_AI_BASE_URL"), "自定义 OpenAI 兼容 Base URL")
	flag.StringVar(&o.model, "model", os.Getenv("IBUKI_AI_MODEL"), "模型名（留空使用预设）")
	flag.BoolVar(&o.newGame, "new", false, "直接开始新游戏")
	flag.StringVar(&o.player, "name", "", "新游戏的角色名")
	flag.Uint64Var(&o.seed, "seed", 0, "新游戏的世界种子（0 为随机，用于复现）")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.String())
		return
	}
	o.apiKey = firstEnv("IBUKI_AI_KEY", "DEEPSEEK_API_KEY", "DASHSCOPE_API_KEY", "OPENAI_API_KEY")
	if err := run(os.Stdin, os.Stdout, o); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

type client struct {
	out   io.Writer
	mu    sync.Mutex
	seq   int
	sugg  []dto.SuggestionV1
	opts  []dto.OptionV1
	saves []dto.SlotV1
	// streamed 表示本回合叙事已经流式打印过。
	streamed bool
}

type response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error"`
	Data  json.RawMessage `json:"data"`
}

func (c *client) call(typ, cmdID string, payload any, out any) error {
	req := map[string]any{"version": dto.V1, "type": typ}
	if cmdID != "" {
		req["command_id"] = cmdID
	}
	if payload != nil {
		req["payload"] = payload
	}
	b, _ := json.Marshal(req)
	var r response
	if err := json.Unmarshal([]byte(adapter.Handle(contextBG(), string(b))), &r); err != nil {
		return err
	}
	if !r.OK {
		return fmt.Errorf("%s", r.Error)
	}
	if out != nil && len(r.Data) > 0 {
		return json.Unmarshal(r.Data, out)
	}
	return nil
}

func (c *client) printf(format string, a ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.out, format, a...)
}

func (c *client) onEvent(js string) {
	var e dto.StreamEventV1
	if json.Unmarshal([]byte(js), &e) != nil {
		return
	}
	switch e.Type {
	case "narration_delta":
		if !c.streamed {
			c.printf("\n")
		}
		c.streamed = true
		c.printf("%s", e.Text)
	case "narration_done":
		if e.Corrected {
			c.printf("\n\n（叙事与事实不符或 AI 不可用，已改用规则叙事：）\n%s", e.Text)
		}
		c.printf("\n")
	}
}

const help = `命令：
  直接输入中文描述你的行动，例如“和老板打个招呼”“劝奥托冷静”“去储藏室看看”
  数字 N         执行第 N 条快捷建议 / 选项
  /s             显示快捷建议          /scene   当前场景
  /me            角色面板              /inv     背包与商店
  /npc           人物关系与 NPC 知道的事  /log  日志（事件时间线）
  /saves         存档列表              /load N  读取第 N 个存档
  /new [名字]    新游戏                /copy    复制当前存档   /del N  删除第 N 个存档
  /ai offline|deepseek|qwen|custom [base_url] [model]   切换 AI（密钥用 /key 或环境变量 IBUKI_AI_KEY）
  /key <API Key> 设置密钥（仅内存）    /test    测试 AI 连接
  /help          帮助                  /quit    退出（进度已自动保存）`

func run(in io.Reader, out io.Writer, o options) error {
	c := &client{out: out}
	adapter.SetSink(c.onEvent)
	defer adapter.SetSink(nil)
	fmt.Fprintf(out, "%s — 文字冒险《边境酒馆》\n", buildinfo.String())
	var initRes map[string]any
	if err := c.call("init", "", map[string]string{"data_dir": o.dataDir}, &initRes); err != nil {
		return err
	}
	fmt.Fprintf(out, "存档位置：%v\n", initRes["db"])
	if err := c.configureAI(o.provider, o.baseURL, o.model, o.apiKey); err != nil {
		fmt.Fprintln(out, "AI 配置失败，使用离线模式：", err)
	}
	c.listSaves()
	switch {
	case o.newGame || len(c.saves) == 0:
		c.newGame(o.player, o.seed)
	default:
		// 默认继续最近的存档（与 App 的“继续游戏”一致）。
		c.load("1")
		fmt.Fprintln(out, "（/new 开始新游戏，/saves 查看全部存档，/help 查看全部命令）")
	}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for {
		fmt.Fprint(out, "\n> ")
		if !sc.Scan() {
			fmt.Fprintln(out)
			return sc.Err()
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if quit := c.handle(line); quit {
			fmt.Fprintln(out, "进度已保存。再见。")
			return nil
		}
	}
}

func (c *client) configureAI(kind, base, model, key string) error {
	var st dto.AIStatusV1
	if err := c.call("configure_ai", "", map[string]string{"kind": kind, "base_url": base, "model": model, "api_key": key}, &st); err != nil {
		return err
	}
	if st.Online {
		c.printf("AI 模式：%s（%s）\n", st.Kind, st.Model)
	} else {
		if kind != "offline" {
			c.printf("缺少 API Key，暂用离线模式（规则解析 + 模板叙事）。\n")
		} else {
			c.printf("离线模式：规则解析 + 模板叙事，无需联网。\n")
		}
	}
	return nil
}

func (c *client) nextID() string {
	c.seq++
	return fmt.Sprintf("cli-%d-%d", time.Now().UnixNano(), c.seq)
}

func (c *client) handle(line string) bool {
	if n, err := strconv.Atoi(line); err == nil {
		c.pick(n)
		return false
	}
	if !strings.HasPrefix(line, "/") {
		c.submit(line)
		return false
	}
	f := strings.Fields(line)
	arg := strings.TrimSpace(strings.TrimPrefix(line, f[0]))
	switch f[0] {
	case "/quit", "/exit", "/q":
		return true
	case "/help", "/h":
		c.printf("%s\n", help)
	case "/version":
		c.printf("%s\n", buildinfo.String())
	case "/s", "/suggest":
		c.refreshSuggestions()
		c.showSuggestions()
	case "/scene", "/look":
		c.showScene()
	case "/me", "/status":
		c.showCharacter()
	case "/inv", "/bag":
		c.showInventory()
	case "/npc", "/npcs":
		c.showNPCs()
	case "/log", "/journal":
		c.showJournal()
	case "/saves":
		c.listSaves()
	case "/load":
		c.load(arg)
	case "/new":
		c.newGame(arg, 0)
	case "/copy":
		var sc dto.SceneV1
		if err := c.call("get_scene", "", nil, &sc); err != nil {
			c.printf("%v\n", err)
			break
		}
		var r map[string]string
		if err := c.call("copy_save", "", map[string]string{"slot_id": sc.SlotID}, &r); err != nil {
			c.printf("复制失败：%v\n", err)
		} else {
			c.printf("已复制为新存档。\n")
		}
	case "/del", "/delete":
		c.del(arg)
	case "/ai":
		kind, base, model := "offline", "", ""
		if len(f) > 1 {
			kind = f[1]
		}
		if len(f) > 2 {
			base = f[2]
		}
		if len(f) > 3 {
			model = f[3]
		}
		var st dto.AIStatusV1
		_ = c.call("ai_status", "", nil, &st)
		_ = c.configureAI(kind, base, model, firstEnv("IBUKI_AI_KEY", "DEEPSEEK_API_KEY", "DASHSCOPE_API_KEY"))
	case "/key":
		var st dto.AIStatusV1
		_ = c.call("ai_status", "", nil, &st)
		_ = c.configureAI(st.Kind, st.BaseURL, st.Model, arg)
	case "/test":
		var st dto.AIStatusV1
		_ = c.call("ai_status", "", nil, &st)
		var r map[string]any
		if err := c.call("test_ai", "", map[string]string{"kind": st.Kind, "base_url": st.BaseURL, "model": st.Model, "api_key": firstEnv("IBUKI_AI_KEY", "DEEPSEEK_API_KEY", "DASHSCOPE_API_KEY")}, &r); err != nil {
			c.printf("%v\n", err)
		} else if r["ok"] == true {
			c.printf("连接成功（%vms）：%v\n", r["latency_ms"], r["reply"])
		} else {
			c.printf("连接失败：%v\n", r["error"])
		}
	default:
		c.printf("未知命令，输入 /help 查看帮助。\n")
	}
	return false
}

func (c *client) submit(text string) {
	var v dto.TurnV1
	c.streamed = false
	if err := c.call("submit_text", c.nextID(), map[string]string{"text": text}, &v); err != nil {
		c.printf("出错了（你的输入没有丢失，可以再试一次）：%v\n", err)
		return
	}
	c.showTurn(v)
}

func (c *client) quick(qa dto.QuickActionV1) {
	var v dto.TurnV1
	c.streamed = false
	if qa.Label != "" {
		c.printf("（%s）", qa.Label)
	}
	if err := c.call("quick_action", c.nextID(), qa, &v); err != nil {
		c.printf("出错了：%v\n", err)
		return
	}
	c.showTurn(v)
}

func (c *client) pick(n int) {
	if len(c.opts) > 0 {
		if n >= 1 && n <= len(c.opts) {
			o := c.opts[n-1]
			c.opts = nil
			c.quick(o.Action)
			return
		}
	}
	if n >= 1 && n <= len(c.sugg) {
		s := c.sugg[n-1]
		if s.Disabled {
			c.printf("暂时不可用：%s\n", s.Hint)
			return
		}
		c.quick(s.Action)
		return
	}
	c.printf("没有第 %d 项。输入 /s 查看快捷建议。\n", n)
}

func (c *client) showTurn(v dto.TurnV1) {
	c.opts = nil
	for _, e := range v.Entries {
		switch e.Kind {
		case "check":
			c.printf("\n  [检定] %s：%s\n", e.Check.Label, e.Text)
		case "narration":
			if !c.streamed {
				c.printf("\n%s\n", e.Text)
			}
		case "system":
			c.printf("\n  • %s\n", e.Text)
			for i, o := range e.Options {
				c.printf("    %d) %s\n", i+1, o.Label)
			}
			c.opts = append(c.opts, e.Options...)
		case "story":
			c.printf("\n  【%s】\n", e.Text)
		}
	}
	if v.Duplicate {
		c.printf("  （该命令已执行过，未重复执行）\n")
	}
	c.sugg = v.Suggestions
	c.printf("\n— %s · %s · 铜币 %d · 第 %d 回合\n", v.Scene.LocationName, v.Scene.TimeText, v.Scene.Gold, v.Scene.Turn)
	if len(c.opts) == 0 {
		c.showSuggestions()
	}
}

func (c *client) refreshSuggestions() {
	var s []dto.SuggestionV1
	if err := c.call("get_suggestions", "", nil, &s); err == nil {
		c.sugg = s
	}
}

func (c *client) showSuggestions() {
	var parts []string
	for i, s := range c.sugg {
		label := s.Label
		if s.Hint != "" {
			label += "（" + s.Hint + "）"
		}
		parts = append(parts, fmt.Sprintf("%d.%s", i+1, label))
	}
	c.printf("建议：%s\n", strings.Join(parts, "  "))
}

func (c *client) showBundle(b struct {
	Scene       dto.SceneV1        `json:"scene"`
	Transcript  []dto.EntryV1      `json:"transcript"`
	Suggestions []dto.SuggestionV1 `json:"suggestions"`
}) {
	start := 0
	if len(b.Transcript) > 12 {
		start = len(b.Transcript) - 12
		c.printf("……（更早的记录已省略）\n")
	}
	for _, e := range b.Transcript[start:] {
		switch e.Kind {
		case "player":
			c.printf("\n> %s\n", e.Text)
		case "check":
			c.printf("  [检定] %s\n", e.Text)
		case "system":
			c.printf("  • %s\n", e.Text)
		case "story":
			c.printf("  【%s】\n", e.Text)
		default:
			c.printf("\n%s\n", e.Text)
		}
	}
	c.sugg = b.Suggestions
	c.printf("\n— %s · %s · 铜币 %d · 第 %d 回合\n", b.Scene.LocationName, b.Scene.TimeText, b.Scene.Gold, b.Scene.Turn)
	c.showSuggestions()
}

type bundleT = struct {
	Scene       dto.SceneV1        `json:"scene"`
	Transcript  []dto.EntryV1      `json:"transcript"`
	Suggestions []dto.SuggestionV1 `json:"suggestions"`
}

func (c *client) newGame(name string, seed uint64) {
	var b bundleT
	if err := c.call("new_game", "", map[string]any{"player_name": name, "seed": seed}, &b); err != nil {
		c.printf("新游戏失败：%v\n", err)
		return
	}
	c.printf("\n=== 新的旅程：%s ===\n", b.Scene.SaveName)
	c.showBundle(b)
}

func (c *client) listSaves() {
	c.saves = nil
	if err := c.call("list_saves", "", nil, &c.saves); err != nil {
		c.printf("%v\n", err)
		return
	}
	if len(c.saves) == 0 {
		c.printf("还没有存档。\n")
		return
	}
	c.printf("存档：\n")
	for i, s := range c.saves {
		cur := ""
		if s.Current {
			cur = " ← 当前"
		}
		c.printf("  %d) %s · %s · %s · 第 %d 回合 · %s%s\n", i+1, s.Name, s.Location, s.Time, s.Turn,
			time.UnixMilli(s.UpdatedAt).Format("01-02 15:04"), cur)
	}
}

func (c *client) slotArg(arg string) (string, bool) {
	if len(c.saves) == 0 {
		c.listSaves()
	}
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	if arg == "" {
		n, err = 1, nil
	}
	if err != nil || n < 1 || n > len(c.saves) {
		c.printf("请指定存档序号（/saves 查看）。\n")
		return "", false
	}
	return c.saves[n-1].ID, true
}

func (c *client) load(arg string) {
	id, ok := c.slotArg(arg)
	if !ok {
		return
	}
	var b bundleT
	if err := c.call("load_game", "", map[string]string{"slot_id": id}, &b); err != nil {
		c.printf("读档失败：%v\n", err)
		return
	}
	c.printf("\n=== 继续：%s ===\n", b.Scene.SaveName)
	c.showBundle(b)
}

func (c *client) del(arg string) {
	id, ok := c.slotArg(arg)
	if !ok {
		return
	}
	if err := c.call("delete_save", "", map[string]string{"slot_id": id}, nil); err != nil {
		c.printf("删除失败：%v\n", err)
		return
	}
	c.printf("已删除。\n")
	c.listSaves()
}

func (c *client) showScene() {
	var s dto.SceneV1
	if err := c.call("get_scene", "", nil, &s); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printf("%s · %s\n%s\n", s.LocationName, s.TimeText, s.Description)
	var names []string
	for _, p := range s.Present {
		names = append(names, fmt.Sprintf("%s（%s，%s）", p.Name, p.Role, p.Attitude))
	}
	c.printf("在场：%s\n", strings.Join(names, "、"))
	c.printf("场景：%s\n", strings.Join(s.Facts, "；"))
	if s.Story != nil {
		c.printf("进行中的事件：《%s》 提示：%s\n", s.Story.Title, strings.Join(s.Story.Hints, " / "))
	}
}

func (c *client) showCharacter() {
	var ch dto.CharacterV1
	if err := c.call("get_character", "", nil, &ch); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printf("%s（%s）铜币 %d\n属性：", ch.Name, ch.Role, ch.Gold)
	for _, a := range ch.Attributes {
		c.printf("%s %d(%+d)  ", a.Name, a.Value, a.Modifier)
	}
	c.printf("\n技能（d20 + 修正）：")
	for _, s := range ch.Skills {
		c.printf("%s %+d  ", s.Name, s.Modifier)
	}
	c.printf("\n")
	for _, s := range ch.Conditions {
		c.printf("状态：%s %s\n", s.Name, s.Note)
	}
}

func (c *client) showInventory() {
	var inv dto.InventoryV1
	if err := c.call("get_inventory", "", nil, &inv); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printf("铜币：%d\n背包：", inv.Gold)
	if len(inv.Items) == 0 {
		c.printf("空")
	}
	for _, it := range inv.Items {
		c.printf("%s×%d  ", it.Name, it.Qty)
	}
	c.printf("\n")
	if len(inv.Shop) > 0 {
		c.printf("%s的柜台：", inv.ShopSeller)
		for _, it := range inv.Shop {
			c.printf("%s %d 铜币  ", it.Name, it.Price)
		}
		c.printf("\n")
	}
}

func (c *client) showNPCs() {
	var ns []dto.NPCV1
	if err := c.call("get_npcs", "", nil, &ns); err != nil {
		c.printf("%v\n", err)
		return
	}
	for _, n := range ns {
		where := "在场"
		if !n.Present {
			where = "在" + n.LocationName
		}
		c.printf("%s（%s）%s · 态度：%s · 信任 %d · 畏惧 %d\n", n.Name, n.Role, where, n.Attitude, n.Trust, n.Fear)
		for _, a := range n.Actions {
			if a.Chance >= 0 {
				c.printf("    %s 成功率约 %d%%\n", a.Label, a.Chance)
			}
		}
		for _, b := range n.Beliefs {
			c.printf("    知道：%s（%s，%s）\n", b.Text, b.Source, b.When)
		}
	}
}

func (c *client) showJournal() {
	var js []dto.JournalEntryV1
	if err := c.call("get_journal", "", nil, &js); err != nil {
		c.printf("%v\n", err)
		return
	}
	start := 0
	if len(js) > 30 {
		start = len(js) - 30
	}
	for _, j := range js[start:] {
		c.printf("[%s] %s\n", j.Time, j.Text)
	}
}
