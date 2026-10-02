package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/api/dto"
)

const worldHelp = `
开放世界（0.2.0）：
  /world [角色|关系网|图鉴|装备|地图|势力|时间线|日志]  世界面板（只显示你已知的内容）
  /wait 10m|1h|dawn|event:<ID>  等待 / 时间跳跃     /usage [回合]  token 用量
  /changes [关键词|外部]  世界变更日志                  /revert <变更ID> [chain|single]  撤销（有依赖时先列出）
  /decision accept|rollback|dismiss [notify]  处理偏离提示（notify = 以后同类只通知）
  /timeline  回合与分支        /rollback <回合>  回到某回合（继续行动将创建新分支）
  /cancel    取消回溯          /branch <分支ID>  切换分支
  /cp [名称] 创建检查点        /restore <检查点ID>  恢复检查点
  /export [目录]  导出存档 .ibksave（不含 API Key）  /importsave <路径>  导入存档
  /audit     立即运行一致性检查（需要联网模型）  /suggest apply|ignore <建议ID>  处理审查建议
  /edit <实体ID> <要求>  让 AI 修改卡片（/edit new <描述> 新建），预览后 /confirm 提交`

var tabAlias = map[string]string{"角色": "characters", "关系网": "relations", "关系": "relations", "图鉴": "codex", "装备": "equipment",
	"地图": "map", "势力": "factions", "时间线": "timeline", "日志": "log"}

// worldCommand 处理开放世界相关命令；返回 false 表示不是这类命令。
func (c *client) worldCommand(cmd, arg string) bool {
	args := strings.Fields(arg)
	first := ""
	if len(args) > 0 {
		first = args[0]
	}
	switch cmd {
	case "/world", "/w":
		c.showWorld(first)
	case "/wait":
		if first == "" {
			first = "1h"
		}
		var v dto.TurnV1
		c.streamed = false
		if err := c.call("wait", c.nextID(), map[string]string{"target": first}, &v); err != nil {
			c.printf("出错了：%v\n", err)
			return true
		}
		c.showTurn(v)
	case "/usage":
		turn, _ := strconv.Atoi(first)
		var u dto.UsageV1
		if err := c.call("get_usage", "", map[string]int{"turn": turn}, &u); err != nil {
			c.printf("%v\n", err)
			return true
		}
		c.printf("%s\n", usageLine(&u))
		for _, t := range u.Tasks {
			c.printf("  %s（%s）：%d 次 · 输入 %d · 输出 %d · %d ms\n", t.Name, t.Model, t.Calls, t.PromptTokens, t.CompletionTokens, t.LatencyMS)
		}
	case "/changes":
		var list []dto.WorldChangeV1
		q, src := arg, ""
		if arg == "外部" || arg == "mcp" {
			q, src = "", "mcp"
		}
		if err := c.call("search_world_changes", "", map[string]any{"query": q, "source": src, "limit": 30}, &list); err != nil {
			c.printf("%v\n", err)
			return true
		}
		if len(list) == 0 {
			c.printf("世界还没有发生变化。\n")
		}
		for _, ch := range list {
			mark := ""
			if ch.RevertedBy != "" {
				mark = "（已撤销）"
			}
			c.printf("  [%s] 第%d回合 %s · %s%s  〔%s〕\n", ch.ID, ch.Turn, ch.TargetName, ch.Summary, mark, ch.SourceLabel)
		}
	case "/revert":
		mode := ""
		if len(args) > 1 {
			mode = args[1]
		}
		if mode == "" {
			var plan dto.RevertPlanV1
			if err := c.call("revert_plan", "", map[string]string{"id": first}, &plan); err != nil {
				c.printf("无法撤销：%v\n", err)
				return true
			}
			if len(plan.Dependents) > 0 {
				c.printf("之后有 %d 项变更依赖它（撤销顺序）：\n", len(plan.Dependents))
				for _, d := range plan.Dependents {
					c.printf("  [%s] 第%d回合 %s · %s\n", d.ID, d.Turn, d.TargetName, d.Summary)
				}
				c.printf("/revert %s chain  连同撤销全部 %d 项\n", first, len(plan.Dependents)+1)
				if plan.CanSingle {
					c.printf("/revert %s single 只撤销这一条（%s）\n", first, plan.SingleNote)
				} else {
					c.printf("不能只撤销这一条：%s\n", plan.SingleNote)
				}
				return true
			}
		}
		var log dto.WorldLogV1
		if err := c.call("revert_change", "", map[string]string{"id": first, "mode": mode}, &log); err != nil {
			c.printf("无法撤销：%v\n", err)
			return true
		}
		c.printf("已撤销 %d 项变更。\n", len(log.Changes))
	case "/decision":
		c.resolveDecision(first, len(args) > 1 && args[1] == "notify")
	case "/audit":
		var a dto.AuditV1
		if err := c.call("run_audit", "", nil, &a); err != nil {
			c.printf("一致性检查失败：%v\n", err)
			return true
		}
		if len(a.Findings) == 0 {
			c.printf("一致性检查没有发现问题。\n")
			return true
		}
		for _, f := range a.Findings {
			c.printf("  · [%s] %s\n", f.Kind, f.Text)
		}
		if a.Fixed != nil {
			for _, ch := range a.Fixed.Changes {
				c.printf("  已修复 [%s] %s\n", ch.ID, ch.Summary)
			}
		}
		for _, sg := range a.Suggestions {
			c.printf("  建议 [%s] %s（/suggest apply|ignore %s）\n", sg.ID, sg.Summary, sg.ID)
		}
	case "/suggest":
		if len(args) < 2 {
			c.printf("用法：/suggest apply|ignore <建议ID>\n")
			return true
		}
		var log dto.WorldLogV1
		if err := c.call("resolve_audit_suggestion", "", map[string]string{"id": args[1], "action": first}, &log); err != nil {
			c.printf("%v\n", err)
			return true
		}
		c.printf("已处理建议 %s。\n", args[1])
	case "/edit":
		if len(args) < 2 {
			c.printf("用法：/edit <实体ID|new> <要求>\n")
			return true
		}
		target := first
		if target == "new" {
			target = ""
		}
		var pv dto.ChangePreviewV1
		if err := c.call("request_edit", "", map[string]string{"target": target, "instruction": strings.Join(args[1:], " ")}, &pv); err != nil {
			c.printf("%v\n", err)
			return true
		}
		for _, ch := range pv.Changes {
			c.printf("  %s\n", ch.Summary)
		}
		if pv.HiddenCount > 0 {
			c.printf("  另有 %d 个你还不知道的字段会被修改\n", pv.HiddenCount)
		}
		for _, r := range pv.Rejected {
			c.printf("  未通过校验：%s\n", r.Reason)
		}
		c.pendingEdit = pv.Token
		if pv.Token != "" {
			c.printf("输入 /confirm 提交修改（不确认则不会生效）。\n")
		}
	case "/confirm":
		if c.pendingEdit == "" {
			c.printf("没有待确认的修改。\n")
			return true
		}
		var log dto.WorldLogV1
		err := c.call("apply_preview", "", map[string]string{"preview_token": c.pendingEdit}, &log)
		c.pendingEdit = ""
		if err != nil {
			c.printf("%v\n", err)
			return true
		}
		c.printf("已修改 %d 项设定（/changes 查看，/revert 撤销）。\n", len(log.Changes))
	case "/timeline", "/tl":
		c.showTimeline()
	case "/rollback":
		turn, err := strconv.Atoi(first)
		if err != nil {
			c.printf("用法：/rollback <回合>\n")
			return true
		}
		c.reloadAfter("rollback_to", map[string]int{"turn": turn}, fmt.Sprintf("已回到第 %d 回合。继续行动将创建新分支（/cancel 取消）。", turn))
	case "/cancel":
		c.reloadAfter("cancel_rollback", nil, "已取消回溯。")
	case "/branch":
		c.reloadAfter("switch_branch", map[string]string{"branch": first}, "已切换分支。")
	case "/cp":
		var cp dto.CheckpointV1
		if err := c.call("create_checkpoint", "", map[string]string{"name": arg}, &cp); err != nil {
			c.printf("%v\n", err)
			return true
		}
		c.printf("已创建检查点 %s（%s，第 %d 回合）。\n", cp.ID, cp.Name, cp.Turn)
	case "/restore":
		c.reloadAfter("restore_checkpoint", map[string]string{"id": first}, "已恢复检查点。")
	case "/export":
		dir := arg
		if dir == "" {
			dir = "."
		}
		var out map[string]any
		if err := c.call("export_save", "", map[string]string{"dir": dir}, &out); err != nil {
			c.printf("导出失败：%v\n", err)
			return true
		}
		c.printf("已导出：%v（%v 字节，不含 API Key）\n", out["path"], out["bytes"])
	case "/importsave":
		var out struct {
			SlotID string `json:"slot_id"`
			Check  struct {
				Warnings []string `json:"warnings"`
			} `json:"check"`
		}
		if err := c.call("import_save", "", map[string]string{"path": arg}, &out); err != nil {
			c.printf("导入失败：%v\n", err)
			return true
		}
		for _, w := range out.Check.Warnings {
			c.printf("  ⚠ %s\n", w)
		}
		c.printf("已导入为新存档（/saves 查看）。\n")
	default:
		return false
	}
	return true
}

func (c *client) reloadAfter(typ string, payload any, msg string) {
	var b struct {
		Scene dto.SceneV1 `json:"scene"`
	}
	if err := c.call(typ, "", payload, &b); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printf("%s\n%s\n", msg, hudLine(b.Scene))
	c.refreshSuggestions()
}

func (c *client) resolveDecision(action string, notify bool) {
	var d struct {
		Decision *dto.DecisionV1 `json:"decision"`
	}
	if err := c.call("get_pending_decision", "", nil, &d); err != nil || d.Decision == nil {
		c.printf("没有待处理的提示。\n")
		return
	}
	if action == "" {
		c.showDecision(d.Decision)
		return
	}
	var b struct {
		Scene dto.SceneV1 `json:"scene"`
	}
	if err := c.call("resolve_decision", "", map[string]any{"id": d.Decision.ID, "action": action, "notify_only": notify}, &b); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printf("已处理。\n%s\n", hudLine(b.Scene))
}

func (c *client) showDecision(d *dto.DecisionV1) {
	if d == nil {
		return
	}
	c.printf("\n  ⚠ 【%s】%s（%s · 影响 %d）\n", d.TypeLabel, d.Title, d.Sensitivity, d.Score)
	c.printf("    %s\n", d.Summary)
	for _, l := range d.Lines {
		c.printf("    · %s\n", l)
	}
	if d.Notify {
		c.printf("    （仅通知）\n")
		return
	}
	c.printf("    /decision accept 接受   /decision rollback %s   /decision dismiss 不再提示\n", d.RollbackText)
}

func (c *client) showTimeline() {
	var tl dto.TimelineV1
	if err := c.call("get_timeline", "", nil, &tl); err != nil {
		c.printf("%v\n", err)
		return
	}
	c.printf("分支：\n")
	for _, b := range tl.Branches {
		cur := ""
		if b.Current {
			cur = " ◀ 当前"
		}
		c.printf("  %s「%s」 第%d~%d回合 [%s]%s\n", b.ID, b.Name, b.ForkTurn, b.HeadTurn, b.Status, cur)
	}
	c.printf("回合：\n")
	for _, t := range tl.Turns {
		mark := " "
		if t.Current {
			mark = "▶"
		}
		if t.Fork {
			mark += "⑂"
		}
		c.printf("  %s %3d  %s  %s\n", mark, t.Turn, t.Time, t.Summary)
	}
	if len(tl.Checkpoints) > 0 {
		c.printf("检查点：\n")
		for _, cp := range tl.Checkpoints {
			c.printf("  %s「%s」第%d回合（%s）\n", cp.ID, cp.Name, cp.Turn, cp.Kind)
		}
	}
	if tl.PendingTurn > 0 {
		c.printf("（已回到第 %d 回合：继续行动将创建新分支）\n", tl.PendingTurn)
	}
}

func (c *client) showWorld(tab string) {
	if t, ok := tabAlias[tab]; ok {
		tab = t
	}
	var p dto.WorldPanelV1
	if err := c.call("get_world_panel", "", map[string]string{"tab": tab}, &p); err != nil {
		c.printf("%v\n", err)
		return
	}
	switch {
	case p.Relations != nil:
		c.showRelations("")
	case p.Codex != nil:
		c.showCodex("")
	case p.Inventory != nil:
		c.showInventory()
	case p.Tab == "timeline":
		for _, e := range p.Events {
			c.printf("  %s  %s  [%s] %s %s\n", e.Time, e.Title, e.StatusLabel, e.Countdown, e.Outcome)
		}
	case p.Tab == "log":
		c.worldCommand("/changes", "")
	default:
		if len(p.Entities) == 0 {
			c.printf("你还一无所知。\n")
		}
		for _, e := range p.Entities {
			c.printf("  %s %s %s %s\n", e.Icon, e.Name, e.Progress, e.Retired)
			for _, f := range e.Fields {
				v := f.Value
				switch f.Level {
				case "unknown":
					v = "未知"
				case "rumored":
					v += "（传闻）"
				}
				c.printf("      %s：%s\n", f.Label, v)
			}
			for _, s := range e.Secrets {
				c.printf("      秘密：%s\n", s)
			}
		}
	}
}

func usageLine(u *dto.UsageV1) string {
	if u == nil {
		return ""
	}
	return fmt.Sprintf("  ⓘ AI %d 次调用 · 输入 %d · 输出 %d tokens", u.Calls, u.PromptTokens, u.CompletionTokens)
}

// showWorldEntries 打印本回合的世界更新 / 裁定 / 用量条目。
func (c *client) showWorldEntry(e dto.EntryV1) {
	switch e.Kind {
	case "adjudication":
		a := e.Adjudication
		if a == nil {
			return
		}
		c.printf("  ⚖ %s  〔%s · %s〕", a.Parse, a.PlausibilityLabel, strings.Join(a.Mods, " "))
		if len(a.Rolls) > 0 {
			c.printf(" %s=%d vs %d → %s", a.Dice, a.Total, a.DC, a.DegreeLabel)
		}
		if a.Downgrade != "" {
			c.printf("（降级：%s）", a.Downgrade)
		}
		if a.Result != "" {
			c.printf(" · %s", a.Result)
		}
		c.printf("\n")
	case "sim":
		// 场外世界模拟：传闻 + 变更
		for _, l := range strings.Split(e.Text, "\n") {
			if strings.TrimSpace(l) != "" {
				c.printf("  〔场外〕%s\n", l)
			}
		}
		if e.World != nil {
			for _, ch := range e.World.Changes {
				c.printf("  🌐 %s（世界模拟，/revert %s 撤销）\n", ch.Summary, ch.ID)
			}
		}
	case "world":
		w := e.World
		if w == nil {
			return
		}
		for _, r := range w.Reveals {
			c.printf("  💡 %s · %s\n", r.Name, r.FieldLabel)
		}
		for _, ch := range w.Changes {
			c.printf("  🌐 %s\n", ch.Summary)
		}
		if w.Failed != "" {
			c.printf("  （本回合世界更新失败：%s，只保留了叙事）\n", w.Failed)
		}
	}
}
