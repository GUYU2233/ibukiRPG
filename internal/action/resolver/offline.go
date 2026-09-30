package resolver

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/GUYU2233/ibukiRPG/internal/action/definition"
	"github.com/GUYU2233/ibukiRPG/internal/core/command"
	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// Offline 是基于关键词与规则的离线解析器：无需任何 API Key，游戏完全可玩。
// 它也是 AI 解析失败时的降级路径（第 3.4 节）。
type Offline struct{}

// Name 返回解析器名。
func (Offline) Name() string { return "offline" }

var (
	moveVerbs    = []string{"前往", "走向", "走到", "走进", "进入", "进去", "回到", "回去", "离开", "出门", "出去", "过去", "去", "回"}
	leaveVerbs   = []string{"离开", "出门", "出去"}
	supernatural = []string{"飞起来", "飞上", "飞到", "瞬移", "传送", "召唤", "施法", "念咒", "火球", "魔法", "变成", "隐身术", "复活", "时间倒流", "读心"}
	violence     = []string{"杀", "砍", "捅", "刺死", "揍", "打死", "打他", "打她", "打人", "攻击", "动手", "拔剑", "拔刀", "掐", "开打", "决斗"}
	// actionPriority：关键词得分相同时，越具体的动作越优先。
	actionPriority = []string{"intimidate", "deceive", "persuade", "investigate", "order_drink", "buy", "use_item", "talk", "rest", "look"}
)

type freeformRule struct {
	tag      string
	keywords []string
	build    func(fr *command.Freeform, target string, in Input)
}

var freeformRules = []freeformRule{
	{"performance", []string{"唱", "弹琴", "弹奏", "跳舞", "跳支舞", "表演", "吟诗", "讲故事", "讲笑话", "吹口哨", "演奏", "说书"}, func(fr *command.Freeform, target string, in Input) {
		fr.Check = &command.SuggestedCheck{Skill: "performance", Difficulty: 11}
		fr.EstimatedMinutes = 5
		fr.Effects = append(fr.Effects,
			command.ProposedEffect{Type: "attention", Text: "众人的目光被吸引了过来"},
			command.ProposedEffect{Type: "noise", Value: 2},
			command.ProposedEffect{Type: "minor_condition", Text: "inspired"},
			command.ProposedEffect{Type: "scene_fact", Text: "还有人在小声议论刚才的表演"})
		if target != "" {
			fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "relationship_nudge", Target: target, Value: 1})
		}
	}},
	{"noise", []string{"踢", "砸", "摔", "拍桌", "掀", "大喊", "大叫", "吼叫", "尖叫", "推倒", "踹", "敲桌子", "摔杯子"}, func(fr *command.Freeform, target string, in Input) {
		fr.Reasonability = AllowWithConsequence
		fr.EstimatedMinutes = 1
		fr.Effects = append(fr.Effects,
			command.ProposedEffect{Type: "noise", Value: 6},
			command.ProposedEffect{Type: "scene_fact", Text: "有人刚刚" + fr.Description, When: "always"})
		// 在店里闹事，店主会不高兴
		if k := in.Pkg.ShopKeeper(in.State.Player.Location); k != "" && in.State.NPCs[k] != nil && in.State.NPCs[k].Location == in.State.Player.Location {
			fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "relationship_nudge", Target: k, Value: -1})
		}
	}},
	{"stealth", []string{"偷偷", "偷", "扒", "顺走", "溜进", "潜入", "躲起来", "躲到", "藏起来", "摸走", "尾随", "跟踪"}, func(fr *command.Freeform, target string, in Input) {
		dc := 13
		if target != "" {
			dc = 15
		}
		fr.Check = &command.SuggestedCheck{Skill: "stealth", Difficulty: dc}
		fr.EstimatedMinutes = 2
		fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "attention", Text: "有人察觉到了可疑的动静", When: "failure"})
		if target != "" {
			fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "relationship_nudge", Target: target, Value: -2, When: "failure"})
		}
	}},
	{"social", []string{"夸", "赞美", "恭维", "调情", "眨眼", "微笑", "敬酒", "干杯", "碰杯", "拥抱", "安慰", "道歉", "道谢", "感谢", "请他喝", "请她喝", "逗"}, func(fr *command.Freeform, target string, in Input) {
		fr.Check = &command.SuggestedCheck{Skill: "persuasion", Difficulty: 11}
		fr.EstimatedMinutes = 2
		fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "minor_condition", Text: "embarrassed"})
		if target != "" {
			fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "relationship_nudge", Target: target, Value: 1})
		}
	}},
	{"observe", []string{"偷听", "倾听", "听听", "闻", "端详", "研究", "盯着", "留意", "注意"}, func(fr *command.Freeform, target string, in Input) {
		fr.Check = &command.SuggestedCheck{Skill: "perception", Difficulty: 12}
		fr.EstimatedMinutes = 3
	}},
	{"help", []string{"帮忙", "帮他", "帮她", "擦桌子", "洗杯子", "打扫", "干活", "搭把手", "收拾"}, func(fr *command.Freeform, target string, in Input) {
		fr.Check = &command.SuggestedCheck{Skill: "athletics", Difficulty: 8}
		fr.EstimatedMinutes = 10
		if k := in.Pkg.ShopKeeper(in.State.Player.Location); target == "" && k != "" && in.State.NPCs[k] != nil && in.State.NPCs[k].Location == in.State.Player.Location {
			target = k
			fr.Targets = append(fr.Targets, target)
		}
		if target != "" {
			fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "relationship_nudge", Target: target, Value: 1})
		}
		fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "minor_condition", Text: "tired"})
	}},
}

// Resolve 用规则解析玩家输入。
func (Offline) Resolve(_ context.Context, in Input) (Resolution, error) {
	p, s := in.Pkg, in.State
	text := normalize(in.Text)
	res := Resolution{Source: "offline", Reasonability: Allow, Confidence: 600}
	if text == "" {
		res.Kind, res.Reasonability, res.Reason = KindReject, Reject, "你想做什么？可以试试下方的建议。"
		return res, nil
	}
	here := s.Player.Location
	present := s.NPCsAt(p, here)
	npcHits := findAliases(text, p.NPCIDs, npcAliases(p))
	target := ""
	for _, h := range npcHits {
		target = h.id
		break
	}
	itemHits := findAliases(text, p.ItemIDs, itemAliases(p))
	item := ""
	if len(itemHits) > 0 {
		item = itemHits[0].id
	}

	// 1) 合理性：明显不可能 / 当前版本不支持的行为
	if w := firstContained(text, supernatural); w != "" {
		res.Kind, res.Reasonability, res.Intent = KindReject, Reject, "supernatural"
		res.Reason = "这个世界里没有这样的力量——至少现在还没有。"
		res.Confidence = 800
		return res, nil
	}
	if w := firstContained(text, violence); w != "" {
		res.Kind, res.Reasonability, res.Intent = KindReject, Reject, "violence"
		res.Reason = "试玩版还没有战斗系统。在这里动手只会让局面失控——也许可以试试威吓、说服，或者换个办法？"
		if target != "" && slices.Contains(present, target) {
			for _, a := range []struct{ key, label string }{{"intimidate", "威吓"}, {"persuade", "说服"}} {
				if id := p.ActionID(a.key); id != "" {
					res.Options = append(res.Options, Option{Label: a.label + p.EntityName(target), Command: command.Command{Kind: command.KindAction, Action: id, Target: target}})
				}
			}
		}
		res.Confidence = 800
		return res, nil
	}

	// 2) 已声明 Action 的关键词匹配（先算分，移动判定之后再用）
	best, bestScore := "", 0
	for _, short := range actionPriority {
		id := p.ActionID(short)
		d, ok := p.Actions[id]
		if !ok {
			continue
		}
		score := 0
		for _, k := range d.Keywords {
			if strings.Contains(text, k) && utf8.RuneCountInString(k) > score {
				score = utf8.RuneCountInString(k)
			}
		}
		if score > bestScore {
			best, bestScore = id, score
		}
	}
	// 3) 移动
	// 明确的社交动作（“说服伯林让我去储藏室”）优先于移动
	socialFirst := best != "" && bestScore >= 2 && len(npcHits) > 0 && p.Actions[best].Target == definition.TargetCharacter
	if verb := firstContained(text, moveVerbs); verb != "" && !socialFirst {
		locHits := findAliases(text, p.LocationIDs, locationAliases(p))
		dest := ""
		for _, h := range locHits {
			if h.id != here {
				dest = h.id
				break
			}
		}
		if dest == "" && (slices.Contains(leaveVerbs, verb) || (len(locHits) > 0 && locHits[0].id == here && slices.Contains(leaveVerbs, firstContained(text, leaveVerbs)))) {
			dest = defaultExit(p, here)
		}
		if dest != "" && (len(npcHits) == 0 || len(locHits) > 0) {
			res.Kind, res.Intent, res.Destination, res.Confidence = KindMove, "move", dest, 850
			return res, nil
		}
	}

	// 4) 使用匹配到的 Action
	// “看看莉娜”之类：观察人物 → look(target)
	if best != "" {
		d := p.Actions[best]
		res.Kind, res.Action, res.Intent = KindAction, best, loader.Key(best)
		res.Confidence = 700 + 50*min(bestScore, 4)
		if len(d.Checks) > 0 {
			res.Reasonability = AllowWithCheck
		}
		switch d.Target {
		case definition.TargetCharacter, definition.TargetOptionalChar:
			if target != "" && !slices.Contains(present, target) {
				res.Kind, res.Reasonability = KindReject, Reject
				res.Reason = fmt.Sprintf("%s不在这里。", p.EntityName(target))
				return res, nil
			}
			if target == "" && d.Target == definition.TargetCharacter {
				switch len(present) {
				case 0:
					res.Kind, res.Reasonability, res.Reason = KindReject, Reject, "这里没有可以"+d.Name+"的人。"
					return res, nil
				case 1:
					target = present[0]
				default:
					res.Kind, res.Reason = KindClarify, "你想"+d.Name+"谁？"
					for _, id := range present {
						res.Options = append(res.Options, Option{Label: labelFor(p, d, id, ""), Command: command.Command{Kind: command.KindAction, Action: d.ID, Target: id}})
					}
					return res, nil
				}
			}
			res.Target = target
		case definition.TargetShopItem:
			if item == "" {
				res.Kind, res.Reason = KindClarify, "你想买什么？"
				if loc := p.Locations[here]; loc.Shop != nil {
					for _, it := range loc.Shop.Items {
						res.Options = append(res.Options, Option{Label: fmt.Sprintf("购买%s（%d 铜币）", p.Items[it].Name, p.Items[it].Price), Command: command.Command{Kind: command.KindAction, Action: d.ID, Item: it}})
					}
				}
				if len(res.Options) == 0 {
					res.Kind, res.Reasonability, res.Reason = KindReject, Reject, "这里没有人卖东西。"
				}
				return res, nil
			}
			res.Item = item
		case definition.TargetInventoryItem:
			if item == "" {
				res.Kind, res.Reason = KindClarify, "你想使用哪样东西？"
				for _, id := range p.ItemIDs {
					if s.Player.Inventory[id] > 0 && p.Items[id].Use != nil {
						res.Options = append(res.Options, Option{Label: p.Items[id].Use.Verb + p.Items[id].Name, Command: command.Command{Kind: command.KindAction, Action: d.ID, Item: id}})
					}
				}
				if len(res.Options) == 0 {
					res.Kind, res.Reasonability, res.Reason = KindReject, Reject, "你身上没有可以使用的东西。"
				}
				return res, nil
			}
			if s.Player.Inventory[item] == 0 {
				// 身上没有：如果这里能买到，给出购买选项而不是自动花钱
				res.Kind, res.Reason = KindClarify, fmt.Sprintf("你身上没有%s。", p.Items[item].Name)
				if loc := p.Locations[here]; loc.Shop != nil && slices.Contains(loc.Shop.Items, item) {
					act := p.ActionID("buy")
					label := fmt.Sprintf("购买%s（%d 铜币）", p.Items[item].Name, p.Items[item].Price)
					if item == p.DrinkItem() {
						act, label = p.ActionID("order_drink"), fmt.Sprintf("点一杯%s（%d 铜币）", p.Items[item].Name, p.Items[item].Price)
					}
					res.Options = []Option{{Label: label, Command: command.Command{Kind: command.KindAction, Action: act, Item: item}}}
				} else {
					res.Kind, res.Reasonability = KindReject, Reject
				}
				return res, nil
			}
			res.Item = item
		default:
			if best == p.ActionID("look") && target != "" && slices.Contains(present, target) {
				res.Target = target
			}
		}
		return res, nil
	}

	// 5) FreeformAction：自然语言长尾
	fr := &command.Freeform{Description: describe(text), EstimatedMinutes: 2, Reasonability: Allow}
	if target != "" && slices.Contains(present, target) {
		fr.Targets = []string{target}
	} else {
		target = ""
	}
	for _, rule := range freeformRules {
		if firstContained(text, rule.keywords) != "" {
			fr.Tags = append(fr.Tags, rule.tag)
			rule.build(fr, target, in)
			break
		}
	}
	if fr.Check != nil && fr.Reasonability == Allow {
		fr.Reasonability = AllowWithCheck
	}
	if len(fr.Tags) == 0 {
		fr.Tags = []string{"misc"}
		fr.Effects = append(fr.Effects, command.ProposedEffect{Type: "none"})
	}
	res.Kind, res.Intent, res.Freeform, res.Reasonability, res.Confidence = KindFreeform, fr.Tags[0], fr, fr.Reasonability, 500
	return res, nil
}

func labelFor(p *loader.Package, d *definition.Definition, target, item string) string {
	l := d.Label
	if l == "" {
		l = d.Name + "{target}"
	}
	return strings.NewReplacer("{target}", p.EntityName(target), "{item}", p.EntityName(item)).Replace(l)
}

func defaultExit(p *loader.Package, here string) string {
	loc := p.Locations[here]
	for _, e := range loc.Exits {
		if slices.Contains(p.Locations[e.To].Tags, "outdoor") {
			return e.To
		}
	}
	if len(loc.Exits) > 0 {
		return loc.Exits[0].To
	}
	return ""
}

func firstContained(text string, words []string) string {
	for _, w := range words {
		if strings.Contains(text, w) {
			return w
		}
	}
	return ""
}

func normalize(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "。！!？?~～…，,、 \t\r\n\"“”")
	return strings.Join(strings.Fields(s), " ")
}

// describe 把“我想在雨里唱歌”变成适合拼在角色名之后的描述“在雨里唱歌”。
func describe(text string) string {
	for _, pre := range []string{"我想要", "我打算", "我试着", "我试图", "我决定", "我准备", "我想", "我要", "我会", "我先", "我就", "我"} {
		if strings.HasPrefix(text, pre) {
			text = strings.TrimPrefix(text, pre)
			break
		}
	}
	text = strings.ReplaceAll(text, "我的", "自己的")
	text = strings.ReplaceAll(text, "我", "自己")
	text = strings.TrimSpace(text)
	if text == "" {
		return "做了点什么"
	}
	return text
}
