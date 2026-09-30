# 故事包格式（v0.1.2）

ibukiRPG 的每个故事（剧本）都是一个独立的**故事包**：一个目录，根目录放 `manifest.yaml`，其余是 YAML 内容文件。
引擎只负责规则与判定，故事包提供世界、人物、台词、事件与界面上的实时信息（HUD）。

内置故事包：

| id | 名称 | 说明 |
|----|------|------|
| `demo` | 边境酒馆 ·《失窃的钱袋》 | 完整示例：4 位 NPC、台词池、HUD、故事变量（`packages/demo`） |
| `fog_lighthouse` | 雾港灯塔 ·《守灯人的信》 | 最小示例：2 个地点、2 位 NPC，没有声明 HUD（`packages/lighthouse`） |

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
└── ui/hud.yaml            # 可选：HUD 与主线目标
```

`manifest.yaml` 的 `content:` 列出所有内容文件，**没列出的文件不会被加载**。

## 2. manifest.yaml

```yaml
id: my_pack                 # 必需：全局唯一，小写字母开头，只能用 a-z 0-9 _ 和 .
namespace: mine             # 必需：内容 ID 前缀（mine:character/xxx），不能与已安装的包重复
name: "我的故事"            # 必需：显示名称
version: 0.1.0              # 必需：语义化版本
type: story                 # story（或 world）才能开新游戏
engine: ">=0.1.2"           # 最低引擎版本（不满足时无法导入，并提示“请先更新 App”）
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
云端 AI 与本地 MediaPipe 模式下，叙事提示词包含一个 `[NPC_MEMORY]` 段落，只含本回合涉及的在场 NPC 自己的记忆（最近几次交谈摘要、谈过的话题、重要经历、上次交谈后看到的事、本回合要表达的意思），不会泄露其他人的记忆。

## 5. 故事变量

`start.variables` 声明整数变量；在事件 / 动作的效果里修改：

```yaml
effects:
  - effect: {type: var_set, target: world, values: {var: suspicion, value: 70}}
  - effect: {type: var_add, target: world, values: {var: clues, value: 1}}
```

变量写入 `VarChanged` 事件（可重放），CEL 中读取 `world.vars.suspicion`。

## 6. HUD（聊天界面的实时信息）

`content.hud` 里的文件包含 `hud:`（字段列表）与 `objectives:`（主线目标）。每回合引擎计算一次，通过 `get_scene` / `get_hud` 返回，App 在顶栏下方显示：紧凑模式是一行信息条，点开后是完整的状态卡片。

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
| `objective` | 当前主线目标（来自 `objectives`） |
| `conditions` | 玩家状态（微醺、吃饱……） |
| `var:<名字>` | 故事变量 |
| `flag:<名字>` | 世界标记（是 / 否） |

没有声明 HUD 的故事包使用默认 HUD：进行中的事件、位置、时间、铜币。

## 7. 打包与导入

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
| 找到 `manifest.yaml` 并解析；id / namespace / 版本格式；必须有 name | “压缩包里找不到 manifest.yaml：它应该在压缩包根目录，或者唯一的顶层文件夹里” |
| 包类型为 story / world；引擎版本满足 `engine` | “需要引擎版本 >=0.2.0，当前是 0.1.2rc1。请先更新 App。” |
| id 不能与内置包相同；namespace 不能与其他已安装包相同 | “命名空间 "demo" 已被故事包「边境酒馆」使用，请换一个命名空间” |
| 依赖已安装且版本满足 | “缺少依赖的故事包 …，请先导入它” |
| 完整加载：YAML 结构、引用完整性、所有 CEL 表达式编译 | “故事包内容校验失败：…” |

再次导入同 id 的包会**覆盖更新**（提示旧版本号）。删除导入的包不会删除存档，但这些存档在重新导入前无法读取。内置故事包不能删除。

## 8. 当前限制

- `dependencies` 只做存在性与版本检查，不合并被依赖包的内容；每个故事包需自带完整的动作、技能等定义（可从 demo 复制）。
- 台词、HUD 与事件条件使用 CEL；表达式在导入时编译校验，但逻辑错误（例如永远为假）不会被发现，请用 CLI 试玩。
- 封面图片只在故事包选择界面显示，≤ 1 MB。
