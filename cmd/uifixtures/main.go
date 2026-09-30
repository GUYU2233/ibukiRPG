// Command uifixtures 用真实引擎（离线模式）跑一段试玩，导出 Android 截图测试所需的 JSON 夹具。
//
//	go run ./cmd/uifixtures -out android/app/src/test/resources/fixtures
//
// 另外会用内置数值 RPG 示例包（锈钟镇·黄铜试炼）跑一段战斗，导出 rpg_*.json。
// 私有故事包可以用 -pack-zip / -pack / -script 生成本地夹具（不要提交）：
//
//	go run ./cmd/uifixtures -out /tmp/fx -pack-zip my.zip -pack my_pack -script steps.txt -only-rpg
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	adapter "github.com/GUYU2233/ibukiRPG/internal/adapter/mobile"
)

// brassScript 是内置示例包的截图脚本。
const brassScript = `和欧琳聊聊
去钟楼广场
去竞技场
@start brass:encounter/trial_bout1
@act attack
@snap rpg_combat
@auto
去钟楼广场
和小铆钉聊聊`

// runScript 逐行执行脚本：普通文字 = 输入；@start <遭遇>；@act <动作> [目标] [技能/道具]；
// @auto 自动攻击直到战斗结束；@quick <kind> <action> [target]；@snap <名字> 导出当前 bundle。
func runScript(script, prefix string, files map[string]json.RawMessage) {
	n := 0
	id := func() string { n++; return fmt.Sprintf("%s-%d", prefix, n) }
	sc := bufio.NewScanner(strings.NewReader(script))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		arg := func(i int) string {
			if i < len(f) {
				return f[i]
			}
			return ""
		}
		switch f[0] {
		case "@start":
			call("quick_action", id(), map[string]string{"kind": "combat", "action": "start", "target": arg(1), "label": "迎战"})
		case "@act":
			labels := map[string]string{"attack": "攻击", "skill": "技能", "item": "道具", "defend": "防御", "flee": "逃跑", "mech": "启动机甲", "eject": "脱离机甲"}
			qa := map[string]string{"kind": "combat", "action": arg(1), "target": arg(2), "label": labels[arg(1)]}
			if arg(1) == "item" {
				qa["item"] = arg(3)
			} else {
				qa["skill"] = arg(3)
			}
			call("quick_action", id(), qa)
		case "@quick":
			call("quick_action", id(), map[string]string{"kind": arg(1), "action": arg(2), "target": arg(3), "label": arg(2)})
		case "@auto":
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
		case "@snap":
			files[arg(1)+".json"] = call("get_bundle", "", nil)
		default:
			call("submit_text", id(), map[string]string{"text": line})
		}
	}
}

// exportRPG 导出数值 RPG 面板夹具，以及出现过的立绘（base64）。
func exportRPG(prefix string, files map[string]json.RawMessage) {
	files[prefix+"game.json"] = call("get_bundle", "", nil)
	files[prefix+"character.json"] = call("get_character", "", nil)
	files[prefix+"inventory.json"] = call("get_inventory", "", nil)
	files[prefix+"npcs.json"] = call("get_npcs", "", nil)
	files[prefix+"journal.json"] = call("get_journal", "", nil)
	files[prefix+"codex.json"] = call("get_codex", "", nil)
	files[prefix+"relations.json"] = call("get_relations", "", nil)
	files[prefix+"cards.json"] = call("get_cards", "", nil)
	mechs := call("get_mechs", "", nil)
	files[prefix+"mechs.json"] = mechs
	// 立绘：人物卡 / 关系网 / 机甲卡里标记了 portrait 的全部导出
	ids := map[string]bool{"player": true}
	var people struct {
		People []struct {
			ID       string `json:"id"`
			Portrait bool   `json:"portrait"`
		} `json:"people"`
		Mechs []struct {
			ID       string `json:"id"`
			Portrait bool   `json:"portrait"`
		} `json:"mechs"`
	}
	_ = json.Unmarshal(files[prefix+"relations.json"], &people)
	_ = json.Unmarshal(mechs, &people)
	for _, p := range people.People {
		if p.Portrait {
			ids[p.ID] = true
		}
	}
	for _, m := range people.Mechs {
		if m.Portrait {
			ids[m.ID] = true
		}
	}
	var cards map[string][]struct {
		ID       string `json:"id"`
		Portrait bool   `json:"portrait"`
	}
	_ = json.Unmarshal(call("get_cards", "", nil), &cards)
	for _, list := range cards {
		for _, c := range list {
			if c.Portrait {
				ids[c.ID] = true
			}
		}
	}
	portraits := map[string]json.RawMessage{}
	for id := range ids {
		var p struct {
			Base64 string `json:"base64"`
		}
		raw := call("get_portrait", "", map[string]string{"id": id})
		if json.Unmarshal(raw, &p) == nil && p.Base64 != "" {
			portraits[id] = raw
		}
	}
	b, _ := json.Marshal(portraits)
	files[prefix+"portraits.json"] = b
}

func call(typ, cmdID string, payload any) json.RawMessage {
	req := map[string]any{"version": "v1", "type": typ}
	if cmdID != "" {
		req["command_id"] = cmdID
	}
	if payload != nil {
		req["payload"] = payload
	}
	b, _ := json.Marshal(req)
	var resp struct {
		OK    bool            `json:"ok"`
		Error string          `json:"error"`
		Data  json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(adapter.Handle(context.Background(), string(b))), &resp); err != nil || !resp.OK {
		fmt.Fprintf(os.Stderr, "%s failed: %v %s\n", typ, err, resp.Error)
		os.Exit(1)
	}
	return resp.Data
}

func main() {
	out := flag.String("out", "android/app/src/test/resources/fixtures", "输出目录")
	seed := flag.Uint64("seed", 3, "世界种子")
	packZip := flag.String("pack-zip", "", "先导入这个故事包 zip（私有包本地截图用）")
	packID := flag.String("pack", "", "数值 RPG 夹具使用的故事包 id（默认 brass_trial）")
	scriptFile := flag.String("script", "", "数值 RPG 试玩脚本文件（默认内置黄铜试炼脚本）")
	player := flag.String("player", "", "玩家名（留空用故事包默认）")
	onlyRPG := flag.Bool("only-rpg", false, "只导出 rpg_*.json")
	flag.Parse()
	dir, err := os.MkdirTemp("", "ibuki-fixtures")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()
	call("init", "", map[string]string{"data_dir": dir})
	files := map[string]json.RawMessage{}
	if !*onlyRPG {
		// 另一个故事包的存档（存档列表 / 故事包卡片上的存档数）
		call("new_game", "", map[string]any{"player_name": "小雾", "seed": *seed, "pack_id": "fog_lighthouse"})
		call("submit_text", "fixture-fog", map[string]string{"text": "和姑娘打个招呼"})
		call("new_game", "", map[string]any{"player_name": "阿澈", "seed": *seed})
		inputs := []string{"和老板打个招呼", "来一杯麦酒", "和伯林聊聊", "仔细搜查奥托的座位附近", "说服伯林让我去储藏室看看"}
		for i, in := range inputs {
			call("submit_text", fmt.Sprintf("fixture-%d", i+1), map[string]string{"text": in})
		}
		files["game.json"] = call("get_bundle", "", nil)
		files["character.json"] = call("get_character", "", nil)
		files["inventory.json"] = call("get_inventory", "", nil)
		files["npcs.json"] = call("get_npcs", "", nil)
		files["journal.json"] = call("get_journal", "", nil)
		files["saves.json"] = call("list_saves", "", nil)
		files["presets.json"] = call("presets", "", nil)
		files["packs.json"] = call("list_packs", "", nil)
	}
	// 数值 RPG：战斗中快照 + 战后各面板
	if *packZip != "" {
		call("import_pack", "", map[string]string{"path": *packZip})
	}
	pid, script := "brass_trial", brassScript
	if *packID != "" {
		pid = *packID
	}
	if *scriptFile != "" {
		b, err := os.ReadFile(*scriptFile)
		if err != nil {
			panic(err)
		}
		script = string(b)
	}
	ng := map[string]any{"seed": *seed, "pack_id": pid}
	if *player != "" {
		ng["player_name"] = *player
	}
	call("new_game", "", ng)
	runScript(script, "rpg", files)
	exportRPG("rpg_", files)
	if err := os.MkdirAll(*out, 0o750); err != nil {
		panic(err)
	}
	for name, data := range files {
		var v any
		_ = json.Unmarshal(data, &v)
		pretty, _ := json.MarshalIndent(v, "", "  ")
		if err := os.WriteFile(filepath.Join(*out, name), pretty, 0o600); err != nil {
			panic(err)
		}
	}
	fmt.Println("fixtures written to", *out)
}
