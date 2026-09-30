package sections

import (
	"strings"

	"github.com/GUYU2233/ibukiRPG/internal/package/loader"
)

// Agent 名。
const (
	Narrator = "narrator"
	Director = "director"
	NPC      = "npc"
	Memory   = "memory"
)

// 默认段落文字（故事包可以用 content.prompts 追加或替换）。
var defaults = map[string]map[string]string{
	"TOOLS": {
		"": `你可以调用只读检索工具（不会改变游戏状态）：
- pack_search(query, type?)：在整个故事包里全文检索人物 / 地点 / 物品 / 技能 / 敌人 / 机甲 / 势力 / 图鉴；
- pack_get_entity(id)、pack_list(type)、character_get_card(id)、mech_get_card(id)、relationship_get(who, other?)；
- memory_search(query, who?)：检索记忆、事件日志与对话摘要；
- story_get_state()、story_anchors()、world_get_location(id?)、rules_get_action(id)。
工具结果已按你的身份过滤；查不到就说明你（或你扮演的角色）不知道。`,
	},
	"RETRIEVAL_POLICY": {
		"": `- 信息不足时先查，再写：不要凭空编造人物、地点、组织、机甲、物品的设定。
- 以下情况必须先检索：出现上下文里没有的名词或人物；上下文注明“已压缩 / 已省略”的早期内容；自由推演 / 沙盒模式下需要新的地点、人物或敌人；不确定某人是否还活着、在哪里、与谁有什么关系。
- 检索结果与 [IMMUTABLE_FACTS] / 正史（Static Canon）冲突时，以正史为准；绝不改写已发生的事实。
- 宁可少写也不要猜；每次最多查几次，查到够用就停。`,
		Narrator: `- 你是叙述者：只能讲玩家已经知道或正在亲眼看到的事，检索到的“来历 / 背景”只用于保持一致，不要剧透给玩家。`,
		Director: `- 你是导演：可以查阅完整设定与后续锚点，但提出的节点只能使用真实存在的 ID，并且不得与正史矛盾。`,
		NPC:      `- 你扮演一个 NPC：只知道这个角色自己经历、听说或看到的事；查不到的事，这个角色就不知道，不要替别人泄露秘密。`,
	},
}

// Render 返回某 Agent 的一个段落（含 [NAME] 标题），合并默认文字与故事包补丁；没有内容时返回空串。
func Render(p *loader.Package, agent, name string) string {
	var parts []string
	if d, ok := defaults[name]; ok {
		if t := d[""]; t != "" {
			parts = append(parts, t)
		}
		if t := d[agent]; t != "" {
			parts = append(parts, t)
		}
	}
	if p != nil {
		for _, x := range p.Prompts {
			if x.Name != name || (x.Agent != "" && x.Agent != agent) {
				continue
			}
			if x.Mode == "replace" {
				parts = []string{x.Text}
			} else {
				parts = append(parts, x.Text)
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "[" + name + "]\n" + strings.Join(parts, "\n") + "\n"
}
