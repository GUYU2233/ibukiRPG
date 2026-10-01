# 故事包格式 3（ibukiRPG v0.2.0-alpha1）

ibukiRPG 的每个故事（剧本）都是一个独立的**故事包**：一个目录，根目录放 `manifest.yaml`，其余是 YAML 内容文件。
引擎只负责规则与判定，故事包提供世界、人物、台词、事件与界面上的实时信息（HUD）。

**0.2.0 起只接受 `format: 3`。** format 3 把故事包从“剧本”改成了“开放世界设定库”：

- 主线贴合度（mainline）与偏离弹窗被**世界事件时间线**（第 13 节）取代：世界按自己的日程运转，玩家可以加入、阻止、错过或不管。
- 每个实体都有**知识层**（第 14 节）：哪些字段见到就知道、哪些要剧情揭示、哪些是只有 AI 知道的隐藏真相。玩家开局对大多数内容都是“未知”。
- AI 可以在游戏中**修改世界**（地点描述、人物去留、势力控制……），所有修改都经过校验、记入世界变更日志、可撤销；修改的约束来自实体信封（`importance` / `canon` / `locked`，第 14 节）。
- **平衡与随机性**（`rules/balance.yaml`，第 15 节）由故事包决定，玩家不能修改；**角色创建**规则见第 16 节。

> 旧格式（没有 `format` 或 `format: 2`，即 v0.1.x 的故事包）导入时会被拒绝，并提示“这个故事包是旧格式（v0.1.x），ibukiRPG 0.2.0 不再支持”。**没有迁移工具**：请按本文档把内容改写为 format 3。
> v0.1.x 的旧存档同样无法读取（首页会提示，可一键删除）。

内置故事包：

| id | 名称 | 说明 |
|----|------|------|
| `brass_trial` | 锈钟镇 ·《黄铜试炼》 | **format 3 完整示例（原创）**：世界事件时间线、势力、知识层与隐藏真相、AI 世界变更、自由战斗（部位 / 体型 / 情境标签）、机甲、角色创建、平衡设置、图鉴、关系网（`packages/brass`） |
| `demo` | 边境酒馆 ·《失窃的钱袋》 | 台词池、HUD、故事变量的示例，已升级到 format 3（`packages/demo`） |
| `fog_lighthouse` | 雾港灯塔 ·《守灯人的信》 | 最小示例：2 个地点、2 位 NPC，format 3（`packages/lighthouse`） |

> 内置故事包通过 `go:embed` 编进引擎库，随 APK 一起分发（不在 Android `assets/` 里单独放一份，避免两份内容不同步）。
> 玩家导入的故事包解压在 App 私有目录 `files/packs/<id>/`（CLI 为 `<存档目录>/packs/<id>/`）。

## 1. 目录结构

```
my_pack/
├── manifest.yaml          # 必需：清单
├── cover.png              # 可选：封面（png/jpg/webp，≤ 1 MB）
├── rules/skills.yaml      # 技能表（可从 demo 复制）
├── rules/pacing.yaml      # 节奏与环境事件
├── world/locations/*.yaml # 地点
├── characters/*.yaml      # 玩家角色（type: player）与 NPC
├── items/items.yaml       # 物品
├── actions/*.yaml         # 可执行动作（看、聊天、调查、说服……）
├── story/*.yaml           # 事件（Story Node）
├── ui/hud.yaml            # 可选：HUD 与当前目标
├── combat/*.yaml          # 可选：数值 RPG（战斗规则、状态、技能、敌人、机甲、遭遇战）
├── codex/*.yaml           # 可选：额外图鉴条目
├── rules/relations.yaml   # 可选：关系维度与 NPC 之间的初始关系
├── world/factions.yaml    # 可选（format 3）：势力
├── story/timeline.yaml    # 可选（format 3）：世界事件时间线
├── rules/balance.yaml     # 可选（format 3）：随机性、强度档位、能力边界、单回合上限
├── rules/creation.yaml    # 可选（format 3）：角色创建（背景、属性点、强度上限）
├── prompts/*.yaml         # 可选：AI 提示词段落补丁
└── assets/                # 可选：立绘图片（portraits / mechs）
```

`manifest.yaml` 的 `content:` 列出所有内容文件，**没列出的文件不会被加载**。

## 2. manifest.yaml

```yaml
format: 3                   # 必需：0.2.0 只接受 3
id: my_pack                 # 必需：全局唯一，小写字母开头，只能用 a-z 0-9 _ 和 .
namespace: mine             # 必需：内容 ID 前缀（mine:character/xxx），不能与已安装的包重复
name: "我的故事"            # 必需：显示名称
version: 0.1.0              # 必需：语义化版本
type: story                 # story（或 world）才能开新游戏
engine: ">=0.2.0"           # 最低引擎版本（不满足时无法导入，并提示“请先更新 App”）
save_compat: ">=0.1.0"      # 可选：本版本能继续读取哪些版本的存档；留空 = 只读同版本存档
tagline: 一句话简介          # 故事包卡片上的副标题
description: 较长的介绍……   # 中文建议写成一行：YAML 的 > 折行会在行与行之间插入空格
author: 你的名字            # 或 authors: [a, b]
icon: lighthouse            # 可选：Material Symbols 图标名（没有封面时显示）
accent: "#2F5D7C"           # 可选：卡片主色
cover: cover.png            # 可选：包内封面图片
tags: [温情, 短篇]
dependencies:               # 可选：依赖的其他包（只检查是否已安装及版本，暂不合并内容）
  - id: some_pack
    version: ">=1.0.0"
start:
  location: mine:location/dock   # 开局地点
  day: 1
  time: "06:30"
  known:                         # 开局就知道的实体（整份）或字段（实体ID#字段）；其余一律“未知”
    - mine:location/dock
    - "mine:character/su#name"
  intro: |                       # 开场白
    ……
  variables:                     # 故事变量初始值（整数），CEL 中为 world.vars.<名字>
    suspicion: 0
content:
  rules: [rules/skills.yaml, rules/pacing.yaml]
  locations: [...]
  characters: [...]
  items: [...]
  actions: [...]
  stories: [...]
  hud: [ui/hud.yaml]
  combat: [combat/combat.yaml]     # 以下为数值 RPG 扩展（v0.1.1 起），都可省略
  codex: [codex/codex.yaml]
  relations: [rules/relations.yaml]
  prompts: [prompts/retrieval.yaml]
  factions: [world/factions.yaml]  # 以下为 format 3 新增，都可省略
  timeline: [story/timeline.yaml]
  balance: [rules/balance.yaml]
  creation: [rules/creation.yaml]
```

版本约束支持 `>=1.2.0`、`>1.0`、`<2`、`=1.0.0`、`^1.2`、`~1.2`，多个条件用逗号或空格分隔（同时满足）。

### 存档与故事包绑定

新存档会记录故事包的 **id + 版本**（架构文档第 44 节）。读档时：

- 故事包已被删除 → 存档列表标出“缺少故事包”，无法读取（重新导入即可恢复）；
- 版本不同 → 只有当已安装版本的 `save_compat` 覆盖存档版本时才能读取，否则给出中文原因。

## 3. 内容 ID

形如 `<namespace>:<类型>/<名字>`，例如 `mine:character/su`、`mine:location/dock`、`mine:action/talk`。
包内所有引用都必须指向本包存在的 ID，否则导入时报错并指出文件与字段。

## 4. 对话台词池（NPC 记忆）

NPC 的 `dialogue` 是一个**台词池**。每次玩家和 NPC 交谈，引擎：

1. 过滤出 `when`（CEL）成立、且（`once: true` 时）没说过的台词；
2. 优先挑**没说过**的：`priority` 大的优先，相同优先级按书写顺序；
3. 能说的都说过了 → 挑优先级最高、最久没说的一句，标记为“重复”，叙事会改用 `repeat` 模板（“刚才不是说过了吗……”），而不是原样复读；
4. 写入 `DialogueOccurred` 事件：记下说了哪句、什么话题、摘要（`memory`）。这是 NPC 的**对话记忆**，随存档保存，读档后继续生效；
5. 台词有 `remember` 时再写一条 `MemoryRecorded`（情节记忆，键为 `dialogue:<台词id>`）。

```yaml
dialogue:
  topics:                      # 话题 id → 显示名（用于 {topic} 与 AI 提示词）
    lamp: 灯塔
    letter: 那封信
  lines:
    - id: first_greet          # 必需，同一 NPC 内唯一
      topic: lamp
      priority: 30
      once: true               # 只说一次
      when: 'npc.talks == 0'   # CEL 条件
      text: 哈罗咳了两声：“外乡人？……”
      memory: 请你帮忙在桌上找东西    # 对话记忆摘要（AI 提示词与人物面板使用）
    - id: letter_found
      topic: letter
      priority: 40
      once: true
      when: '"letter_found" in world.flags'
      text: “二十年了……替我交给码头上的苏吧。”
      memory: 托你把信交给苏
      remember: 把信托付给了{player}   # 写入情节记忆
  repeat:                      # 台词池耗尽时的“说过了”模板
    - 哈罗摆摆手：“{topic}的事，刚才不是说过了？”
  callback:                    # 自上次交谈后 NPC 看到玩家做了值得一提的事时追加一句
    - 哈罗看了你一眼：“刚才你{what}，我都看见了。”
```

台词文本可用占位符：`{player}` 玩家名、`{name}` NPC 名、`{topic}` 话题名、`{last_topic}` 上次话题、`{last_said}` 上次说的摘要、`{what}` 上次交谈后看到的事、`{talks}` 交谈次数。

旧格式（`greet` / `friendly` / `wary` / `story` / `after_story`）仍然可用，加载时会自动转换成台词池。

### 台词条件可用的 CEL 变量

| 变量 | 说明 |
|------|------|
| `npc.talks` | 和玩家交谈过的次数（本次之前） |
| `npc.turns_since_talk` / `npc.minutes_since_talk` | 距上次交谈的回合 / 分钟（从未交谈为 -1） |
| `npc.last_topic` / `npc.last_line` | 上次的话题 / 台词 id |
| `npc.said` | 说过的台词 id 列表，例如 `"first_greet" in npc.said` |
| `npc.topics` | 谈过的话题列表 |
| `npc.memories` | 情节记忆键列表，例如 `"dialogue:fish_rumor" in npc.memories` |
| `npc.seen` | 上次交谈后该 NPC **亲眼看到**玩家做过的动作（动作短名，如 `order_drink`） |
| `npc.seen_all` | 该 NPC 看到过的全部动作 |
| `npc.trust` / `npc.fear` / `npc.attitude` | 关系数值与态度文字（敌视/戒备/畏惧/中立/友好/亲近） |
| `npc.present` / `npc.location` | 是否和玩家在同一地点 / 所在地点 |
| `stories.<短名>.status` | 事件状态：`inactive` / `active` / `resolved` |
| `stories.<短名>.outcome` / `.steps` | 结局 id / 已完成的步骤 id |
| `world.flags` / `world.vars` | 世界标记 / 故事变量 |
| `world.turn` `world.hour` `world.day` `world.player_location` | 回合、时间、玩家位置 |
| `actor.gold` `actor.inventory` `actor.conditions` | 玩家铜币、背包（物品 ID → 数量）、状态 |
| `npcs["<id>"].trust` 等 | 其他 NPC 的关系 |

记忆只写给**亲历者或目击者**（witness）：NPC 不在场就不知道玩家做了什么。
云端 AI 与本地模型（llama.cpp）模式下，叙事提示词包含一个 `[NPC_MEMORY]` 段落，只含本回合涉及的在场 NPC 自己的记忆（最近几次交谈摘要、谈过的话题、重要经历、上次交谈后看到的事、本回合要表达的意思），不会泄露其他人的记忆。

## 5. 故事变量

`start.variables` 声明整数变量；在事件 / 动作的效果里修改：

```yaml
effects:
  - effect: {type: var_set, target: world, values: {var: suspicion, value: 70}}
  - effect: {type: var_add, target: world, values: {var: clues, value: 1}}
```

变量写入 `VarChanged` 事件（可重放），CEL 中读取 `world.vars.suspicion`。

## 6. HUD（聊天界面的实时信息）

`content.hud` 里的文件包含 `hud:`（字段列表）与 `objectives:`（当前目标）。每回合引擎计算一次，通过 `get_scene` / `get_hud` 返回，App 在顶栏下方显示：紧凑模式是一行信息条，点开后是完整的状态卡片。

```yaml
hud:
  - id: objective
    label: 主线
    icon: flag            # Material Symbols 图标提示（App 不认识时显示默认图标）
    bind: objective       # 内置绑定（见下表）
    wide: true            # 紧凑模式下单独占一行
    order: 0              # 排序（小的在前）
  - id: suspicion
    label: 米拉的嫌疑
    icon: report
    value: world.vars.mira_suspicion   # 或者用 CEL 表达式计算
    format: "{value}%"
    max: 100                            # 设置后显示进度条
    visible: 'stories.lost_purse.status != "inactive"'   # CEL，可选
    compact: false                      # false = 只在展开的卡片里显示
    tone: warning                       # 默认色调：normal / success / warning / danger
    tones:                              # 条件色调（按 danger → warning → success 检查）
      danger: world.vars.mira_suspicion >= 70
      success: world.vars.mira_suspicion == 0
objectives:                             # 从上到下取第一个 when 成立的
  - when: 'stories.lost_purse.status == "inactive"'
    text: 在酒馆里坐坐，和客人们聊聊
```

| bind | 值 |
|------|----|
| `location` | 当前地点名 |
| `time` / `clock` / `day` / `period` | 完整时间 / 时:分 / 第几天 / 时段 |
| `gold` | 铜币 |
| `turn` | 回合数 |
| `story` | 进行中的事件（没有时隐藏） |
| `objective` | 当前目标（来自 `objectives`） |
| `conditions` | 玩家状态（微醺、吃饱……） |
| `var:<名字>` | 故事变量 |
| `flag:<名字>` | 世界标记（是 / 否） |

没有声明 HUD 的故事包使用默认 HUD：进行中的事件、位置、时间、铜币。

## 7. 数值 RPG：战斗规则（combat）

`content.combat` 里的文件声明战斗规则、状态、技能、敌人、机甲与遭遇战（完整示例：`packages/brass/combat/combat.yaml`）。没有 `combat` 的故事包仍是纯文字冒险，界面不显示战斗 / 成长相关面板。

```yaml
rules:
  attribute_base: 10                 # 属性基准值：高出的每点按 attribute_effects 加成
  attribute_effects:
    strength: {atk: 1, hp: 2}
    agility: {spd: 1, eva: 1}
  xp_base: 100                       # 升级所需经验 = xp_base × 当前等级
  max_level: 10
  resource_name: 蒸汽压              # 机甲能源的显示名（天之炽私有包里叫“红水银”）
  mercury_max: 100
  defeat_conditions: {injured: injured, rescued: injured}   # 战败分支 → 附加的玩家状态
  # mech_statuses / mech_specs：可选，覆盖默认的机甲状态与规格项
statuses:
  - {id: my:status/bleed, name: 流血, icon: water_drop, turns: 3, dot: 4, debuff: true}
  - {id: my:status/stagger, name: 踉跄, turns: 1, stun: true, debuff: true}
  - {id: my:status/guarded, name: 架势, turns: 2, mods: {def: 6}}
skills:
  - id: my:skill/smash
    name: 重击
    power: 150                       # 攻击力百分比；0 = 不造成伤害
    cost: {sp: 8}                    # sp 体力 / heat 热量 / mercury 能源
    target: enemy                    # enemy（默认）/ all_enemies / ally / self
    hits: 1                          # 多段攻击次数
    acc_bonus: 0                     # 命中加成
    pierce: 0                        # 无视防御的百分比
    heal: 0                          # 治疗量
    cleanse: false                   # 清除负面状态
    restore: {sp: 0, heat: 0}        # 恢复体力 / 降温
    status: {id: my:status/stagger, chance: 25, turns: 1}
    human_only: false                # 仅人形可用
    mech_only: false                 # 仅机甲可用
    learn: {level: 2, cost: 1}       # 需要等级与技能点；不写 = 初始或由效果授予
enemies:
  - id: my:enemy/rat
    name: 发条鼠
    tier: minion                     # minion / elite / boss
    level: 1
    stats: {hp: 24, sp: 10, atk: 8, def: 2, spd: 11, acc: 80, eva: 12, crit: 5}
    skills: [my:skill/smash]
    ai:                              # 从上到下取第一条 when 成立（且 chance 命中）的
      - when: 'self.hp_pct < 35'
        skill: defend
      - when: 'battle.round % 2 == 1'
        skill: my:skill/smash
        target: lowest_hp            # player / lowest_hp / highest_hp / random
    xp: 25
    gold: 3
    drops: [{item: my:item/tea, chance: 30}]
    mech: false                      # true = 机甲单位（凡人武器伤害大减）；mech_card 指向机甲卡
encounters:
  - id: my:encounter/bout1
    title: 第一场
    label: 挑战第一场                # 出现在建议列表与 /fight 里
    location: my:location/arena
    intro: 铁闸拉起……
    enemies: [my:enemy/rat, my:enemy/rat]
    allies: [my:character/friend]    # 可选：同伴（角色需有 combat 段）
    available: 'world.combats.bout1.wins == 0'   # CEL：何时可以主动发起
    repeatable: false                # 可重复挑战（陪练）
    no_flee: false                   # 禁止逃跑
    allow_mech: true                 # 允许启动机甲
    victory:                         # 胜利效果（见第 17 节）
      - effect: {type: flag_set, values: {flag: bout1_won}}
    defeat:                          # 战败不会 Game Over：分支 rescued / injured
      branch: rescued
      text: 你被拖回了工坊。
      move_to: my:location/workshop
      effects: []
```

- 事件（story）的步骤也可以用 `combat_start` 效果强制开战；战斗结果写入 `world.combats.<遭遇短名>.{result,wins,losses}`，事件步骤 / 结局用它推进。
- 玩家角色写 `combat:`（`level`、`stats`、`growth` 每级成长、`skills`、`equipment`、`mech`、`mercury`）；同伴 NPC 写 `combat:`（`level`、`stats`、`skills`、`ai`）。
- 物品：`kind: weapon/armor/accessory` + `slot` + `mods`（装备加成）+ `level`（需求等级）；`kind: consumable` + `combat: {heal, sp, mercury, cool, cure, field}`（`field: true` 表示战斗外也能用）。
- 战斗中的自然语言输入由规则解析器识别（“用重击打第二只老鼠”）；AI 只负责叙述，**伤害、命中、掉落全部由引擎掷骰**，同样的种子重放结果完全一致。
- 数值平衡可以用 `go test ./internal/combat/sim` 的模拟器验证（每场遭遇 200 个种子的胜率与剩余生命）。

## 8. 机甲卡（mechs）

`combat.yaml` 的 `mechs:` 声明机甲卡。玩家的机体（`pilot: player`，并在玩家 `combat.mech` 里引用）可以在战斗中“启动”，敌方机体（`enemy: true`）只作为图鉴与情报卡展示。

```yaml
mechs:
  - id: my:mech/warden
    name: 黄铜守卫
    model: 行会二号蒸汽甲胄
    maker: 工匠行会
    class: 蒸汽甲胄
    pilot: player
    status: inactive                 # ready / damaged / inactive / lost / sealed / repair（可在 rules.mech_statuses 扩展）
    rarity: epic
    portrait: assets/mechs/warden.png   # 可选立绘
    stats: {hp: 140, atk: 26, def: 20, spd: 8, acc: 85, eva: 3, crit: 8}
    specs: {armor: 55, output: 48, mobility: 30, capacity: 100, heat_threshold: 100, sync: 62}
    skills: [my:skill/sweep]
    mod_slots: [{id: core, name: 锅炉, kind: core}]
    hardpoints: [{id: arm, name: 右臂挂点, kind: arm}]
    installed: {core: my:item/boiler}
    mounted: {arm: my:item/fist}
    hidden: [spec:sync, lore]        # 开局未知的字段（界面显示“？？？”），用 mech_reveal 效果揭示
    engage_cost: 15                  # 启动消耗的能源
    mercury_per_turn: 6              # 每回合消耗
    heat_max: 100                    # 过热上限
    attack_heat: 12                  # 普通攻击产生的热量
    cool_per_turn: 8                 # 每回合自然降温
    overheat_damage: 10              # 过热时驾驶员受到的伤害
    requires: '"warden_granted" in world.flags'   # CEL：何时允许启动
```

- 能源（`resource_name`）耗尽、机体被打爆或主动 `eject` 时驾驶员脱离，继续以人形作战。
- 热量超过 `heat_max` 会强制冷却一回合，并烫伤驾驶员 `overheat_damage`。
- 改装件 `kind: mech_part`、挂载武器 `kind: mech_weapon`，用物品的 `mech: {slot, hardpoint, stats, specs, skills, heat_max, mercury_per_turn}` 描述；CLI `/install 槽位 物品`，App 在机甲卡里安装。
- 相关效果：`mech_status`（改状态）、`mech_reveal`（`fields: "model,spec:sync,slot:core,lore"` 或 `*`）、`mech_install` / `mech_remove`、`mech_pilot`。

## 9. 图鉴（codex）

`content.codex` 的 `entries:` 是额外的图鉴条目（势力、设定、科技……）。人物、地点、物品、技能、敌人、机甲会**自动**进入图鉴，玩家见过 / 获得 / 交过手后解锁；这里只需要写没有实体的条目。

```yaml
entries:
  - id: my:codex/guild
    kind: faction                    # item / skill / enemy / mech / location / character / faction / lore / tech
    name: 工匠行会
    icon: engineering
    rarity: rare
    description: 掌管全镇锅炉与甲胄的行会。
    lore: 门楣上刻着：“先学会修，再学会打。”
    stats: {能源: 蒸汽压}            # 可选：在详情里显示的键值
    unlock: 'world.player_location == "my:location/plaza"'   # CEL；不写 = 开局解锁
```

角色的 `faction:` 可以指向一个 `kind: faction` 条目；`codex_unlock` 效果可以直接解锁某一条。

## 10. 关系网（relations）

默认关系维度：信任 `trust`、好感 `affection`、敬畏 `awe`、恩情 `debt`、敌意 `hostility`、畏惧 `fear`（多数为 -100～100，恩情为 0～100）。`content.relations` 可以改维度，并声明 NPC 之间的初始关系：

```yaml
# dimensions: [{id: trust, name: 信任, min: -100, max: 100}, {id: hostility, name: 敌意, negative: true}]
edges:
  - from: my:character/orin
    to: my:character/tock
    values: {trust: 10, hostility: 15}
    public: true                     # 公开关系：玩家一开始就知道；否则需要 relation_reveal / 台词 reveals 揭示
    note: 师徒，但托克顶撞过她
```

- 角色的 `relationship_to_player` 是对玩家的初始关系；台词的 `relation:`、效果 `relation`（`from/to/values/reason`）会修改关系，每次变化都记录原因，关系网面板里可以看到来龙去脉。
- `relation_reveal` 效果（或台词 `reveals: ["a>b"]`）揭示一条隐藏关系。

## 11. 角色卡（cards）

```yaml
card: major                          # major：开局就有角色卡；minor：满足条件后升格；none（默认）
promote:
  score: 5                           # 互动分达到这个值（交谈、同行战斗、赠礼……）
  when: 'world.combats.bout2.wins > 0'   # 或者 CEL 条件成立
  reason: 在擂台上交过手
faction: my:codex/guild
icon: sports_mma
portrait: assets/portraits/tock.png  # 见第 12 节
```

效果 `card_create` / `card_archive` / `card_restore` / `npc_death` 可以在剧情中新建、归档、恢复或让角色退场（角色卡保留在“已故 / 归档”页）。AI 在自由推演中临时生成的角色只有角色卡，没有立绘与台词池。

## 12. 立绘（portraits）

角色与机甲可以写 `portrait: assets/...`：包内相对路径，必须位于 `assets/` 下，png / jpg / jpeg / webp，单个文件 ≤ 2 MB；文件不存在或超限时导入报错。没有立绘时界面显示图标 / 首字。
记得把图片文件放进 zip（`packzip` 会自动带上 `assets/`）。使用他人作品的图片时请自行确认授权，公开分发的故事包不要包含未授权图片。

## 13. 世界事件时间线（timeline，format 3）

`content.timeline` 列出世界自己的日程。事件不需要玩家参与也会按时发生；玩家可以通过切入点（hooks）**加入**或**阻止**，也可以错过。AI 叙事只会提到玩家**知道**的事件（公开日程、听到的传闻、亲眼所见）。

```yaml
events:
  - id: mine:event/raid
    title: 拾荒者夜袭钟楼
    summary: 拾荒者趁钟楼停摆，计划在夜里撬开钟楼底座。
    importance: 4                  # 1–5；≥ 4 的事件结果会进入影响评估
    canon: major                   # flavor / minor / major / core
    pivotal: true                  # 关键事件：错过或改变会显著影响世界（界面标出）
    known: false                   # true = 开局就知道（公开日程）
    window: {start: "D3 22:00", end: "D4 02:00"}   # 发生窗口（D<天> HH:MM）；单点事件用 at
    location: mine:location/plaza
    participants: [mine:character/rivet]
    preconditions: '!("golem_down" in world.flags)'  # CEL：不满足时事件取消
    rumor:                         # 传闻：在指定时间、地点被玩家听到（false: true 为假传闻）
      - {at: "D2 12:00", where: [mine:location/sewer], text: 管道里有人在磨撬棍……}
    stages:                        # 进行中的阶段描写
      - {id: pry, at: "D3 23:00", text: 钟楼底座的检修门被撬开了一条缝。}
    hooks:                         # 玩家可以做的切入点（显示为快捷行动）
      - {id: warn, label: 去工坊报信, tags: [disrupt], minutes: 15, vars: {guard: 2}}
      - {id: help, label: 帮拾荒者望风, tags: [join], minutes: 30, vars: {loot: 2}}
    outcomes:                      # 按顺序匹配 when（CEL，可用 event.vars.*）；都不满足时取 default
      - id: foiled
        title: 夜袭被挫败
        when: 'event.vars.guard >= 2'
        text: 行会的人提着灯冲进广场，拾荒者四散而逃。
        effects: [...]             # 与第 17 节相同的效果
      - id: looted
        title: 铜齿轮被拆走
        default: true
        text: 天亮时，钟楼底座空了一大块。
    resolve: rules                 # rules（按 outcomes 判定）/ ai（联网时由世界模拟决定，离线退回 rules）
```

时间推进：每个行动都消耗游戏时间；玩家也可以“等待”（10 分钟 / 1 小时 / 到天亮 / 到某个已知事件开始）。界面状态栏显示即将发生的已知事件与倒计时。

## 14. 实体信封与知识层（format 3）

所有实体（地点、人物、物品、势力、事件……）都可以带同一组信封字段：

```yaml
importance: 3          # 1–5：重要度。≥ 3 的人物死亡会触发“重要角色死亡”提示；AI 不能新建 ≥ 4 的实体
canon: major           # flavor / minor / major / core：AI 修改 major / core 设定会计入“偏离原设定”
locked: [fields.name]  # AI 永远不能修改的路径
fields: {controller: mine:faction/council}   # 额外字段（世界面板与 AI 检索可见）
alias_unknown: 戴面具的人   # 玩家还不知道名字时显示的称呼
knowledge:
  public: [name, description]        # 见到就知道（缺省：人物 = 外貌；地点 = 名称 / 描述 / 出口；物品 = 名称 / 描述）
  discoverable: [goals, members]     # 需要剧情揭示（对话、调查、亲眼所见）后才解锁
  known_by: [mine:character/su]      # 这些人物知道全部可揭示字段（对话渠道）
  hidden:                            # 隐藏真相：只有 AI 能看到
    - id: prototype
      text: 钟楼傀儡是行会二十年前的原型机。
      reveal_when: '"golem_down" in world.flags'   # 可选：满足后才允许揭示
      leak_markers: [原型机, 二十年前的图纸]        # 叙事出现这些词即视为泄露（守卫检查）
```

- 知识按**字段**解锁，每个字段有三种状态：未知 / 传闻 / 已知。世界面板与 AI 叙事只使用玩家已知的内容。
- 没有 `reveal_when` 的隐藏真相可以由 AI 在合适时揭示，但揭示会计入影响评估；影响严重时会弹出“对故事影响很大”的提示。有 `reveal_when` 的真相在条件满足前不能揭示。
- 人物的 `secrets`（旧字段）会自动转换为 `hidden`。

## 15. 平衡与随机性（rules/balance.yaml，format 3）

```yaml
randomness: 45          # 0–100：自由战斗与检定的随机性（故事包决定，玩家不能修改）
size_classes: [tiny, small, human, large, huge, colossal]
power_tiers:            # 强度档位（角色创建、卡片生成、审查都用它判断“太强 / 太弱”）
  - {id: trained, name: 学徒 / 拳手, power: [40, 90]}
player_start_tier: trained
ability_limits:         # 世界的能力边界（写进 AI 提示词；违反的自由行动会被降级）
  - 这个世界没有魔法。
absurd_words: [火球, 咒语, 魔法]   # 自由战斗中出现即判为“荒谬”并降级为可行的动作
caps: {gold_pct: 50, gold_flat: 40, xp_pct: 50, item_qty: 3}   # AI 单回合给玩家资源的上限
mortal_vs_mech: 20      # 凡人武器对机甲的伤害倍率（%）
audit_every: 12         # 一致性自审的默认间隔（回合；玩家可在生成设置里改或关闭）
```

## 16. 角色创建（rules/creation.yaml，format 3）

```yaml
attribute_points: 6     # 自建角色可分配的属性点
attr_min: 8
attr_max: 15
max_power: 115          # 引擎强度分上限（超过不能开局）
rules: [自建角色属于这座小镇……]     # 给审查 Agent 的设定约束
forbidden: [魔法, 不死]             # 背景 / 外貌 / 性格里出现即判为不符合设定
backgrounds:
  - id: apprentice
    name: 行会学徒候选人
    description: 在工坊长大。
    attributes: {intelligence: 1}
    items: {mine:item/bandage: 2}
    gold: 10
    location: mine:location/workshop   # 可选：开局地点
    known: [mine:character/orin]       # 开局就知道的实体
```

开局时玩家可以选择预设主角（`characters/player.yaml`），或自建角色：引擎先做规则检查（点数、上限、强度分、禁用词），联网时再由审查 Agent 检查设定契合度、强度与冲突，并给出建议与推荐卡。

## 17. 效果（effects）一览

事件步骤、结局、台词、遭遇战胜负、时间线事件结局都使用同一套效果：`- effect: {type: <类型>, target: <可选>, values: {...}}`。

| type | values | 说明 |
|------|--------|------|
| `relationship_delta` | `trust` `fear` … | 旧式 NPC 关系增减（target 为 NPC） |
| `relation` | `from` `to` `trust` `affection` … `reason` | 关系网增减（`to: player` 表示对玩家） |
| `relation_reveal` | `from` `to` `source` | 揭示隐藏关系 |
| `gold_delta` | `amount` | 铜币 |
| `item_add` / `item_remove` | `item` `qty` | 物品 |
| `scene_fact` | `text` | 场景事实（叙事与 AI 可见） |
| `var_add` / `var_set` | `var` `value` | 故事变量 |
| `flag_set` | `flag` | 世界标记 |
| `condition_add` / `condition_remove` | `condition` | 玩家状态 |
| `npc_move` | `location` | 移动 NPC（`{location.id}` = 玩家当前位置） |
| `move_player` | `location` | 移动玩家 |
| `combat_start` | `encounter` | 立刻开战 |
| `xp` | `amount` `reason` | 经验（可能升级） |
| `heal` | `amount` 或 `pct` | 回复生命 |
| `mercury` | `amount` | 机甲能源增减 |
| `learn_skill` | `skill` | 习得技能 |
| `codex_unlock` | `id` | 解锁图鉴条目 |
| `card_create` / `card_archive` / `card_restore` / `npc_death` | `npc` `reason` | 角色卡 |
| `mech_status` / `mech_reveal` / `mech_install` / `mech_remove` / `mech_pilot` | 见第 8 节 | 机甲 |

数值字段可以写整数，也可以写 CEL 表达式字符串。

## 18. 数值 RPG 的 HUD 绑定与 CEL 变量

HUD 额外的 `bind`：

| bind | 值 |
|------|----|
| `level` | 等级 `Lv.N` |
| `xp` | 经验 `当前/升级所需` |
| `hp` / `sp` | 生命 / 体力（战斗中取战斗单位的实时值） |
| `mercury` | 机甲能源（配合 `max:` 显示进度条） |

CEL 额外变量：

| 变量 | 说明 |
|------|------|
| `actor.level` `actor.xp` `actor.hp` `actor.max_hp` `actor.mercury` `actor.in_combat` | 成长与生命 |
| `actor.equipment` / `actor.combat_skills` | 装备（槽位 → 物品 ID）/ 已掌握的战斗技能 |
| `world.combats.<遭遇短名>.result` / `.wins` / `.losses` | 遭遇战记录 |
| `world.cards` | 角色卡状态 |
| `npcs["<id>"].relation` / `.card` / `.alive` / `.interaction` | 关系网、角色卡状态、是否在场、互动分 |
| 敌人 AI：`self.hp` `self.hp_pct` `self.sp` `self.heat_pct` `self.mech` `self.statuses`，`battle.round` | 仅在 `ai.when` 中可用 |

## 19. 提示词段落补丁（prompts）

`content.prompts` 可以给 AI Agent 的提示词段落追加或替换文字，用于写本故事特有的检索规则与文风。当前可补丁的段落：

| 段落 | 用途 |
|------|------|
| `TOOLS` | 只读检索工具说明（`pack_search`、`character_get_card`、`memory_search` 等，见 [mcp.md](mcp.md)） |
| `RETRIEVAL_POLICY` | 什么时候必须先查再写：信息不足、刚发生过记忆压缩、玩家提到陌生名词、自由推演 |
| `STYLE` 等其他段名 | 追加到对应 Agent 的系统提示词末尾 |

```yaml
sections:
  - name: RETRIEVAL_POLICY
    agent: narrator                  # narrator / director / npc；留空 = 所有 Agent
    mode: append                     # append（默认，追加到引擎默认文字之后）/ replace（整段替换）
    text: |
      - 写到某台甲胄或“过热”时，先用 mech_get_card / pack_search 核对。
```

引擎默认的检索规则已经包含：不编造故事包里查得到的事实、不与静态设定冲突、遵守可见范围（NPC 只能查到自己知道的，叙事者只能查到玩家可见的信息与允许的伏笔）。`replace` 会去掉这些默认规则，除非确有必要请用 `append`。
检索工具只在需要时启用（云端模型支持 function calling 时走工具调用循环；本地模型或不支持工具调用的服务改为按关键词预取，放进 `[RETRIEVED]` 段落），每回合有调用次数与结果长度上限。

## 20. 打包与导入

1. 确认目录根部就是 `manifest.yaml`（也可以把整个目录作为 zip 里唯一的顶层文件夹）；
2. 打包成 zip：

   ```bash
   # 推荐：先完整校验再打包（跨平台，输出 build/packs/<id>-<version>.zip）
   make pack-zip PACK=lighthouse          # 等价于 go run ./cmd/packzip packages/lighthouse
   # 或者用任意 zip 工具：
   cd packages/lighthouse && zip -r ../../my_pack.zip .
   ```

3. 导入：
   - **Android**：首页「新游戏」→ 故事包列表右上角「导入」→ 在系统文件选择器里选择 `.zip`；
   - **CLI**：`/import /path/to/pack.zip`，`/packs` 查看，`/new 名字 N` 用第 N 个故事包开始新游戏，`/delpack N` 删除导入的故事包。

导入时的校验（任何一步失败都不会留下残留文件，并给出中文原因）：

| 检查 | 失败提示示例 |
|------|-------------|
| zip 安全：≤ 2000 个文件、解压后总计 ≤ 64 MB、单个文件 ≤ 16 MB，不允许 `..` 路径与符号链接 | “压缩包里有不安全的路径：../evil.txt” |
| 找到 `manifest.yaml` 并解析；`format: 3`；id / namespace / 版本格式；必须有 name | “这个故事包是旧格式（v0.1.x），ibukiRPG 0.2.0 不再支持。请向作者索取 format 3 版本的故事包” |
| （同上） | “压缩包里找不到 manifest.yaml：它应该在压缩包根目录，或者唯一的顶层文件夹里” |
| 包类型为 story / world；引擎版本满足 `engine` | “需要引擎版本 >=0.2.0，当前是 0.1.3-rc1。请先更新 App。” |
| id 不能与内置包相同；namespace 不能与其他已安装包相同 | “命名空间 "demo" 已被故事包「边境酒馆」使用，请换一个命名空间” |
| 依赖已安装且版本满足 | “缺少依赖的故事包 …，请先导入它” |
| 完整加载：YAML 结构、引用完整性、所有 CEL 表达式编译 | “故事包内容校验失败：…” |

再次导入同 id 的包会**覆盖更新**（提示旧版本号）。删除导入的包不会删除存档，但这些存档在重新导入前无法读取。内置故事包不能删除。

## 21. 当前限制

- `dependencies` 只做存在性与版本检查，不合并被依赖包的内容；每个故事包需自带完整的动作、技能等定义（可从 demo 复制）。
- 台词、HUD 与事件条件使用 CEL；表达式在导入时编译校验，但逻辑错误（例如永远为假）不会被发现，请用 CLI 试玩。
- 封面图片只在故事包选择界面显示，≤ 1 MB；立绘 ≤ 2 MB，图鉴条目（除人物 / 机甲立绘外）暂不支持配图。
- AI 生成的实体（`<namespace>.gen:*`）只有文档与角色卡，没有立绘和台词池。
- 自由战斗离线时用规则解析器识别动作，联网时由战斗意图解析器与裁定流程处理；`absurd_words` 只做关键词匹配。
- 时间线事件的 `resolve: ai` 在离线时退回规则判定。
