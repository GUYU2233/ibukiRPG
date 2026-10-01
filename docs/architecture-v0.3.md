# ibukiRPG 架构 V0.3：开放世界模拟 RPG

> 从“带主线的剧本 RPG”转向“设定库 + AI 生成故事 + 引擎记账”的开放世界模拟 RPG。

**文档版本：** V0.3（设计稿，**尚未实现**）  
**对应 App 版本：** 0.2.0（alpha1 / alpha2 两批发布）  
**基线代码：** v0.1.2-rc2（`d38c1fc`）  
**日期：** 2026-10-01  
**前置文档：** [architecture-v0.2.md](architecture-v0.2.md)、[story-pack-format.md](story-pack-format.md)、[mcp.md](mcp.md)、[local-models.md](local-models.md)  
**设计稿图片：** `docs/design/v03/`（HTML 源码，`render.sh` 渲染为 PNG；只用于设计，不随 App 发布）

> 版本号说明：架构文档编号（V0.2 → V0.3）与 App 版本号（0.1.x → 0.2.0）是两套编号。本文的所有设计都在 App 0.2.0 中交付。

---

# 0. 一页纸摘要

| 主题 | v0.1.2-rc2 | 0.2.0（本文） |
|---|---|---|
| 故事结构 | 主线锚点 + 偏离度 + 偏离弹窗 + 沙盒 | **开放世界**：故事包定义按世界时间推进的**世界事件时间线**，玩家可参与 / 破坏 / 无视，错过的事件照常结算 |
| 故事包 | 剧本（台词池、事件、主线） | **设定库**：地理、势力、历史、人物、物品、科技、规则、**隐藏真相（仅 AI 可见）**；故事由玩家自己的 AI 生成 |
| AI 权限 | 只能叙述；改世界只能走白名单轻量事件 | **完全权限改世界 / 设定**，但每一项改变都是**结构化、经过校验、写入事件日志、可回滚**的 `WorldChange` |
| 偏离 | 偏离度累计到阈值弹窗 | **影响评估**：彻底偏离设定、重要 NPC 死亡、严重影响故事时弹出“接受 / 回到上一回合”；三类提示各自可调灵敏度，可设为“只通知不打断” |
| 玩家认知 | 图鉴 / 关系 / 机甲字段有部分隐藏 | **玩家知识层**：一切从“未知”开始，逐字段解锁；引擎校验每一次揭示 |
| 战斗 | 规则解析器把输入映射到技能按钮 | **自由战斗**：自然语言 → AI 解析为规则动作 → 引擎校验合理性、计算修正 + 掷骰 → 成功 / 失败都有后果，荒谬动作降级并给出叙事解释；随机性由故事包在“叙事 ↔ 硬核”刻度上预设 |
| 存档 | 单线自动存档，复制 / 重命名 | **任意回合回滚**（“回到这里”）、**分支时间线**、**检查点**（自动 + 手动）、**导出 / 导入**；0.1.x 存档明确拒绝 |
| AI 设置 | 单一服务商 | **API 设置**（多服务商、多模型、Keystore 加密）+ **生成设置**（统一模型或按任务分配，本地 llama.cpp 可做子 Agent）；叙事 + 世界更新合并为一次结构化输出；显示每回合 token |
| 一致性 | Narrative Guard + 记忆摘要 | + **可搜索的世界变更日志**、叙事前**强制注入**场景相关变更、**周期性 AI 自审**（可修复、可回滚） |
| 角色 | 固定主角 | 预设主角或**自建角色**（也可建 NPC），**审查 Agent** 检查设定契合 / 强度 / 冲突并可给出推荐卡 |
| UI | 聊天流 + HUD + 面板 | 为模拟 RPG 重新设计：叙事流 + 智能输入栏 + 状态条 + **世界面板**（8 个页签）+ 战斗覆盖模式 + 时间线 / 分支导航 + 偏离提示底部面板 |

不变的底线：

1. **骰子与数值结算永远属于引擎。** AI 可以提议“发生了什么”，但不能决定检定结果、伤害数值、随机数。
2. **所有永久变化都是事件。** 新增的 AI 世界修改、知识揭示、时间线结算、回滚分支全部可重放、可审计。
3. **AI 可失败、可降级。** 任何 AI 故障（超时、坏 JSON、内容审核拒绝）都不能写坏存档。
4. **公开仓库不放任何受版权保护的内容。** 本文与设计稿只使用原创示例（《锈钟镇》）。私有故事包只在私有仓库里改。

---

# 1. 与 v0.1.2-rc2 的差异清单

## 1.1 保留

| 模块 | 说明 |
|---|---|
| `internal/core`（Command → Event → State） | 继续是唯一真相源；`event.Data` 共享结构增加若干可选字段（第 5、8、10 节） |
| `internal/rules/rng` 命名流 | 保留；新增**分支盐**（第 10.4 节） |
| `internal/storage/eventstore` | 结构升级到 schema v2（分支、检查点、AI 调用记录），见第 10 节 |
| `internal/agent/tools` 只读工具 + 范围（player / npc / director） | 保留并扩充；写入走新的统一网关（第 14 节） |
| `internal/agent/memory` 记忆压缩 | 保留，按分支隔离 |
| `internal/narrative/guard` | 保留；新增隐藏真相泄露检查与“未知字段”检查 |
| 战斗数值模型（HP / 体力 / 机甲形态 / 过热 / 状态） | 保留为自由战斗的结算底座（第 9 节） |
| 关系网、图鉴、角色卡、机甲卡 | 保留数据；展示改由知识层驱动（第 8 节） |
| llama.cpp 本地推理、前台服务、内存策略 | 保留；增加按任务调度与 JSON 语法约束（第 11.7 节） |
| Android Compose / MD3、琥珀配色、Roborazzi 截图测试 | 保留；界面重新布局（第 15 节） |

## 1.2 删除

| 删除项 | 位置 | 替代 |
|---|---|---|
| 主线贴合度 / 偏离度计算 | `internal/story/director/deviation.go`、`state.Mainline.Deviation` | 影响评估（第 7 节） |
| 强制偏离弹窗、“回到主线 / 进入沙盒”选择 | `command.KindMainline`、UI 弹窗 | 偏离提示底部面板（接受 / 回滚） |
| 主线锚点、`budget_turns`、`nudges` | 故事包 `content.mainline` | 世界事件时间线（第 6 节） |
| 事件类型 `DeviationChanged`、`Mainline*` | `internal/core/event` | `ImpactAssessed`、`DecisionRequested` / `DecisionResolved`、`WorldEvent*` |
| 导演提案 `command.KindDirector` + `NodeProposal` | `internal/agent/canon` | 世界模拟任务 `world_sim` + `WorldChange`（第 5、6 节） |
| HUD 绑定 `deviation` / `mode`，CEL 变量 `world.mode` / `world.deviation` / `world.anchor` | `internal/api/query/hud.go` | HUD 新绑定 `world_event`、`resource:<id>`（第 4.6 节） |
| 设置页“偏离敏感度” | Android 设置 | “提示与灵敏度”页（第 15.9 节） |
| 0.1.x 存档读取 | eventstore schema v1 | 明确拒绝并提示（第 10.7 节） |

沙盒模板（`sandbox.templates`）不删除，改名为**离线委托模板**，供没有 AI 时的 `world_sim` 离线实现使用（第 6.7 节）。

## 1.3 新增模块（Go）

```text
internal/world/overlay      生效世界 = 故事包静态设定 + WorldChange 覆盖层
internal/world/change       WorldChange 结构、校验器、反向操作、影响评分
internal/world/timeline     世界事件时间线运行时（调度、阶段、结算、传闻）
internal/knowledge          玩家知识层（字段目录、揭示校验、过时检测）
internal/combat/freeform    自由战斗：意图结构、合理性检查、修正表、程度判定
internal/storage/branch     分支谱系、回滚、检查点
internal/storage/exchange   存档导出 / 导入（.ibksave）
internal/ai/router          按任务路由的 Provider 选择、备用链、token 记账
internal/ai/structured      合并输出解析、JSON 修复、重试、内容审核识别
internal/agent/audit        一致性自审
internal/agent/creator      卡片生成、角色审查
```

---

# 2. 架构宪法修订

V0.2 第 3.1 节“LLM 不可以直接修改 HP / 物品 / 新增永久角色……”在 V0.3 中改写为：

## 2.1 AI 有完全的**提议**权，引擎有完全的**记账**权

```text
AI（叙事 / 战斗裁定 / 世界模拟 / 审查 / 卡片生成 / MCP 客户端 / 玩家指令）
        ↓   结构化提案：WorldChange[] / KnowledgeReveal[] / CombatIntent / ...
  Validator（schema、引用完整性、数值上限、能力档位、保护字段、频率限制）
        ↓   接受的部分 → Command（KindWorldChange / KindCombat ...）
  Game Core（唯一写入口）
        ↓
  Event（含 before / after，可重放、可反向）
        ↓
  State（静态设定 + 覆盖层 + 知识层）
        ↓
  影响评估 → 必要时：偏离提示（接受 / 回滚）
```

- AI **可以**：新增 / 修改 / 退场任何实体（人物、地点、势力、物品、科技、规则、设定条目、时间线事件），让 NPC 死亡，改变势力格局，改写历史设定，揭示玩家知识。
- AI **不可以**：决定骰子结果、伤害数值、检定成败；绕过校验器直接写状态；在没有被校验的情况下向玩家揭示隐藏真相；修改玩家的 API Key、设置或存档元数据。
- 每次修改都有**来源**（`narrate` / `combat` / `world_sim` / `audit` / `user_request` / `mcp` / `creation` / `pack`）与**原因**文本，都能在“日志 / 世界变更”页查到、撤销或通过分支回滚。

## 2.2 三层事实

| 层 | 内容 | 谁能看到 |
|---|---|---|
| 客观世界（Effective World） | 故事包静态设定 + 全部已接受的 WorldChange | 引擎；Director 范围的 AI |
| 隐藏真相（Hidden Truth） | 故事包或 AI 标记为 `truth: hidden` 的字段 | 仅 AI（Director 范围）；向玩家揭示必须经过知识层校验 |
| 玩家知识（Player Knowledge） | 玩家已知的实体 / 字段 / 关系 / 传闻（可能过时、可能是假传闻） | 玩家、叙事 AI 的 player 范围 |

NPC 信念（Belief）继续独立于客观世界（V0.2 第 14 节），本版不扩展信息传播，只增加“传闻”进入玩家知识（第 6.5 节）。

---

# 3. 总体架构与回合流水线

## 3.1 普通回合（非战斗）

```text
玩家输入（行动 / 对话 / 指令）
  │
  ├─ 指令模式 → 第 14.3 节（用户请求的修改：先预览差异，确认后提交）
  │
  ▼
① 意图解析 parse_action（AI 或离线规则）→ Command
② 引擎执行：规则、检定、时间推进、时间线调度（到期的世界事件开始 / 结算）
③ 提交 A：规则事件 + 玩家输入记录（同一事务；幂等 command_id）
④ 上下文构建：场景、玩家知识、可揭示集合、强制注入的世界变更、相关隐藏真相、检索结果、记忆
⑤ narrate_world（一次调用）：叙事正文 + WORLD 结构化段
     ├─ 叙事流式输出 → Narrative Guard
     └─ WORLD 段 → 解析 / 修复 → 校验 → 接受的变更
⑥ 提交 B：WorldChangeApplied / KnowledgeRevealed / SceneFact / Memory 事件 + 叙事文本（同一回合号，command_id = <id>:world）
⑦ 影响评估：本回合所有变更的影响分 → 超过灵敏度阈值 → DecisionRequested（阻塞输入，等待“接受 / 回滚”）
⑧ 后台：记忆压缩、索引更新、自审计数、世界模拟（场外事件细节）
```

要点：

- **调用预算**：普通回合 = 1 次（`narrate_world`），意图解析由本地模型或离线规则承担时不额外花云端调用；意图解析也用云端时为 2 次。工具调用循环（检索）仍受 3 轮 / 6 次上限。
- **提交 A 与 B 分开**是为了保证“AI 失败不影响规则结果”：③ 之后无论 ⑤ 发生什么，规则结果都已保存；⑤ 失败只会让 ⑥ 变成“仅模板叙事 + `WorldUpdateFailed` 标记”。
- **回滚粒度**以回合号为单位：提交 A 与 B 共享回合号，“回到回合 N”= 回到回合 N 的最后一个事件之后。
- **进程被杀**：A 已提交、B 未提交时，恢复流程沿用 `repairNarrations`：补模板叙事，并写入 `WorldUpdateFailed`（原因 `interrupted`），不再重新调用 AI（避免重复计费与结果漂移）。审查 Agent 会优先检查这些回合（第 12.3 节）。

## 3.2 战斗回合

```text
玩家自然语言 → ① combat_adjudicate（AI，结构化 CombatIntent；不可用时回退 v0.1 规则解析器）
→ ② 引擎合理性检查 + 修正 + 掷骰 + 结算（含敌方行动）→ ③ 提交
→ ④ 叙事（narrate_world 的战斗简版，或模板）→ ⑤ 提交叙事 + 战斗中产生的世界变更（例如场景破坏、部位情报揭示）
```

调用预算：每轮 2 次（裁定 + 叙事）。设置里可让战斗叙事走模板（只在开战 / 结束 / 大成功 / 大失败时调用 AI），每轮降为 1 次。

## 3.3 关键路径之外

| 任务 | 触发 | 是否阻塞 |
|---|---|---|
| 记忆压缩 / 摘要 | 每回合后 | 否（后台，按回合顺序） |
| 世界模拟 `world_sim` | 时间跳跃、场外世界事件结算、进入新地点 | 时间跳跃时阻塞（显示“世界在推进……”），其余后台 |
| 一致性审查 `audit` | 每 N 回合（默认 15）/ 记忆压缩后 / 手动 | 否；修复以“系统回合”提交 |
| 索引更新（世界变更日志 FTS） | 提交 B 之后 | 否（失败可从事件重建） |

---

# 4. 故事包 v3：设定库

## 4.1 定位

故事包不再提供“剧本”，而是提供**世界的设定库**和**世界自己会发生的事**。故事（玩家经历什么）由玩家选择的 AI 在运行时生成。

`manifest.yaml` 新增 `format: 3`。0.2.0 只加载 `format: 3` 的包；`format` 缺省或为 2 的包导入时报错：

> 这个故事包是旧格式（v0.1.x），0.2.0 无法直接使用。作者可以用 `packzip migrate` 转换成新格式。

内置的三个包（`demo`、`fog_lighthouse`、`brass_trial`）随 0.2.0 迁移到 format 3（第 17 节）。

## 4.2 目录结构

```text
my_pack/
├── manifest.yaml              # format: 3
├── world/
│   ├── geography.yaml         # 区域、地点、连通与路程（分钟）
│   ├── history.yaml           # 历史条目（年代、事件、真相层）
│   ├── factions.yaml          # 势力：目标、领地、成员、相互关系
│   ├── tech.yaml              # 科技 / 魔法 / 体系
│   ├── rules.yaml             # 世界规则（硬约束：例如“凡人武器对机甲伤害 ×0.2”）
│   └── timeline.yaml          # 世界事件时间线（第 6 节）
├── characters/*.yaml          # 人物（含可选的 playable 预设主角）
├── items/*.yaml               # 物品 / 武器 / 部件
├── combat/*.yaml              # 战斗：状态、技能、敌人（含部位）、机甲、动作修正表
├── codex/*.yaml               # 其他设定条目
├── rules/
│   ├── skills.yaml
│   ├── relations.yaml
│   ├── creation.yaml          # 角色创建规则（第 13 节）
│   └── balance.yaml           # 强度档位、随机性预设、能力边界（第 9.5 节）
├── stories/*.yaml             # 可选：局部事件 / 委托（不按时间推进，由条件触发）
├── prompts/*.yaml             # 提示词段落补丁（文风、检索规则）
└── assets/                    # 立绘
```

`dialogue` 台词池仍可用，但在 AI 模式下只作为“人物说话风格样例”；离线模式继续用台词池。

## 4.3 实体通用信封

所有实体（人物、地点、势力、物品、机甲、设定条目、时间线事件）共用一套信封字段，使 WorldChange、知识层、影响评估能统一处理：

```yaml
id: brass:character/tock
kind: character
importance: 4          # 1–5：影响评估与审查使用（5 = 世界级关键人物）
canon: major           # core / major / minor / flavor：设定层级，改动 core 视为“彻底偏离设定”
locked: [identity.origin]   # 可选：保护字段，修改它们至少按 major 影响计
fields:
  name: 托克
  appearance: 高个子，左耳缺了一块，袖口总有油污
  identity: 铁环竞技场选手
  faction: brass:faction/coal_smoke
  background: 在行会长大，十五岁离开工坊去打擂台
  personality: 嘴硬心软，怕欠人情
knowledge:              # 玩家知识层：字段的可见性
  public: [name, appearance]          # 见到就知道
  discoverable: [identity, faction, background, personality]   # 需要剧情揭示
  hidden:                             # 隐藏真相：仅 AI 可见
    - id: debt
      text: 他欠煤烟帮一笔赌债，送货是在还债
      reveal_when: 'knows("brass:character/tock", "faction")'   # 可选：揭示前提（CEL）
      leak_markers: [赌债]                                      # Guard 用的泄露关键词
alias_unknown: 高个子的擂台选手   # 玩家还不知道名字时的称呼
```

- `fields` 是开放的键值树（字符串 / 数字 / 列表 / 引用），每种 `kind` 有一份**字段目录**规定必需字段、类型与数值范围（第 8.2 节）。
- 数值字段（战斗属性等）仍放在 `combat:` 段，与 v0.1 兼容的结构不变。

## 4.4 世界事件时间线

见第 6.2 节。

## 4.5 随机性与平衡

见第 9.5 节。`balance.yaml` 示例：

```yaml
randomness: 30            # 0 = 纯叙事，100 = 硬核；天之炽私有包预设为叙事向
power_tiers:              # 强度档位：用于角色审查与 AI 生成卡片的上限
  - {id: commoner, name: 普通人, power: [0, 40]}
  - {id: trained,  name: 受训者, power: [40, 90]}
  - {id: elite,    name: 精英,   power: [90, 160]}
  - {id: legend,   name: 传奇,   power: [160, 300]}
player_start_tier: trained
ability_limits:           # 世界能力边界：自由战斗用来判定“荒谬”
  - 没有魔法；超自然力量只存在于隐藏真相中
  - 普通人无法徒手对抗机甲
size_classes: [tiny, small, human, large, huge, colossal]
```

## 4.6 HUD v3

HUD 仍由故事包声明（`ui/hud.yaml`），删除 `deviation` / `mode` / `objective`（主线目标）绑定，新增：

| bind | 值 |
|---|---|
| `world_event` | 最近的、玩家已知的世界事件及倒计时（“议会投票 · 1 天 4 时后”） |
| `resource:<id>` | 故事包定义的资源（体力、蒸汽压、能源……），带上限时显示进度条 |
| `weather` | 天气（世界模拟写入的场景事实） |
| `statuses` | 玩家身上的状态（含剩余回合） |

## 4.7 版权

- 公开仓库内置包全部原创；示例截图 / 设计稿只用《锈钟镇》等原创内容。
- 私有包（例如 `packs/tianzhichi`）只存在于私有仓库，打包产物不提交到公开仓库，测试用环境变量门控（沿用 v0.1.2 的 private pack balance test 方式）。
- 存档导出**不包含**故事包内容，只记录包 ID、版本与内容哈希（第 10.6 节）。

---

# 5. 世界状态模型与 WorldChange

## 5.1 生效世界 = 静态设定 + 覆盖层

```go
// internal/world/overlay
type Overlay struct {
    Entities map[string]*EntityPatch `json:"entities"` // 实体 ID → 覆盖
    Created  map[string]*EntityDoc   `json:"created"`  // AI / 玩家新建的实体（完整文档）
    Retired  map[string]Retirement   `json:"retired"`  // 退场（死亡、毁灭、解散、遗失）
    Rev      map[string]int          `json:"rev"`      // 实体修订号（知识层用来判断“可能已过时”）
}
```

`state.State` 新增 `World *overlay.Overlay`、`Knowledge *knowledge.Store`、`Timeline *timeline.State`、`Pending *Decision`。读取实体一律经过 `overlay.Resolve(pkg, st, id)`：静态文档 → 应用补丁 → 标记退场。检索工具、叙事上下文、UI 视图都只看生效世界。

## 5.2 WorldChange 结构

```go
// internal/world/change
type Change struct {
    ID       string          `json:"id"`        // wc-<回合>-<序号>，由引擎分配
    Op       string          `json:"op"`        // create / patch / retire / restore / link / unlink / timeline_add / timeline_patch / timeline_cancel
    Target   string          `json:"target"`    // 实体 ID（create 时为新 ID，命名空间固定为 <pack>.gen）
    Kind     string          `json:"kind,omitempty"`     // create 时必填
    Path     string          `json:"path,omitempty"`     // patch：字段路径，如 fields.faction、combat.stats.atk、knowledge.hidden[debt]
    Value    json.RawMessage `json:"value,omitempty"`    // 新值 / 新文档
    Before   json.RawMessage `json:"before,omitempty"`   // 引擎填写：修改前的值（用于差异显示与反向操作）
    Reason   string          `json:"reason"`              // 一句话原因（玩家可见，隐藏真相相关时可见版本会被替换）
    Source   string          `json:"source"`              // narrate / combat / world_sim / audit / user_request / mcp / creation
    Impact   Impact          `json:"impact"`              // 引擎计算（第 7.2 节）
    Evidence string          `json:"evidence,omitempty"`  // 叙事中的依据（引用原句，审查用）
}
```

AI 输出的变更只填 `op / target / kind / path / value / reason / evidence` 和自评影响等级；`ID / Before / Impact / Source` 一律由引擎填写，AI 写了也会被覆盖。

## 5.3 校验器

| 检查 | 规则 | 失败处理 |
|---|---|---|
| Schema | `kind` 的字段目录：必需字段、类型、文本长度（单字段 ≤ 400 字）、列表长度 | 丢弃该项 |
| 引用完整性 | 引用的实体存在且未退场（`restore` 例外）；新 ID 不与已有冲突 | 丢弃 |
| 数值上限 | 战斗数值 / 物品属性按 `balance.power_tiers` 计算强度分，不得超过目标档位；单次 patch 数值变化 ≤ 档位宽度的 50% | 夹紧到上限并在原因后标注“（已按世界强度上限调整）” |
| 玩家资源 | 金钱 / 经验 / 物品数量的单回合增量上限（故事包可配，默认：金钱 ≤ 当前 50% + 50，经验 ≤ 升级所需 50%） | 夹紧 |
| 能力档位 | 按模型能力档位限制可用操作（第 11.7 节） | 丢弃并记录 |
| 保护字段 | `locked` 字段与 `canon: core` 实体：允许修改，但影响分至少为 major / break | 不拒绝，只抬高影响分 |
| 隐藏真相 | 修改 `knowledge.hidden` 仅 Director 范围来源可用（`narrate` 来源的叙事 Agent 也算 Director，因为它可见相关真相）；任何来源都不能借 WorldChange 直接把真相写进玩家知识 | 丢弃 |
| 战斗中 | 战斗进行时，`combat` 以外来源不能修改参战单位的 HP / 体力 / 状态 | 丢弃 |
| 频率 | 单回合最多 12 项（云端）/ 3 项（本地）；同一路径单回合只改一次 | 超出部分丢弃 |
| 一致性 | 同一提案内互相矛盾（先 retire 再 patch 同一实体） | 丢弃后者 |

被丢弃的提案写入诊断表 `world_change_rejects`（不是游戏事件，不影响重放），审查 Agent 与“日志”页的“被拒绝的修改”开关可以查看。

## 5.4 事件

| 事件 | 载荷（`event.Data` 新字段） | 说明 |
|---|---|---|
| `WorldChangeApplied` | `Change *change.Change` | 唯一的世界修改事件；重放时只读 `Value` / `Before`，不再调用 AI |
| `WorldChangeReverted` | `Change`（反向操作）+ `Key = 被撤销的 change ID` | 单项撤销（第 12.5 节）；不删除原事件 |
| `ImpactAssessed` | `Impact` 汇总、`Tags = change IDs` | 每个提交 B 之后写一次（无影响时省略） |
| `WorldUpdateFailed` | `Reason = parse / refused / timeout / interrupted / validator_all_rejected` | 无状态效果；标记本回合需要审查 |

`event.Data` 继续沿用“共享结构 + omitempty”，新增可选指针字段 `Change`、`Reveal`、`WorldEvent`、`Decision`、`Intent`、`Adjudication`。旧字段不变。

## 5.5 失败模式

| 情况 | 处理 |
|---|---|
| AI 新建了与已有实体同名的人物 | 校验器提示“同名实体已存在”，丢弃；叙事里出现的同名人物视为已有实体 |
| AI 把已退场的人物写回场景 | Guard 检查“已故 / 退场人物出现在场景中” → 重写或模板兜底；变更丢弃 |
| AI 改动了玩家当前不知道的字段 | 正常接受；知识层把该字段的已知值标为“可能已过时”（第 8.5 节） |
| 覆盖层过大（长局） | 覆盖层是状态的一部分，随快照保存；单实体补丁合并为最新值，历史在事件里 |

---

# 6. 开放世界：世界事件时间线

## 6.1 原则

- 世界不等玩家。时间线事件按**世界时间**到期，玩家在场就能参与，不在场就照常结算。
- 没有“主线”。故事包可以把原作剧情写成时间线事件（`pivotal: true`），但它们只是“如果没人插手就会发生的事”。
- 玩家通过**参与**（加入一方）、**破坏**（阻止 / 改变条件）、**无视**三种方式影响时间线；AI 也可以通过 WorldChange 增加、修改、取消时间线事件。

## 6.2 故事包格式

```yaml
# world/timeline.yaml
events:
  - id: brass:event/council_vote
    title: 钟楼议会投票
    importance: 4
    canon: major
    pivotal: true                  # 原作 / 关键剧情点：偏离它会触发自动检查点（第 10.5 节）
    window: {start: "D3 09:00", end: "D4 09:00"}   # 世界时间窗口；也可写 at: "D4 09:00"
    location: brass:location/plaza
    participants: [brass:character/orin, brass:faction/council]
    preconditions: 'alive("brass:character/orin")'
    rumor:                         # 玩家如何提前得知
      - {at: "D2 12:00", where: [brass:location/plaza, brass:location/workshop], text: 听说议会要投票决定钟楼归属}
    stages:
      - {id: gather, at: "D3 09:00", text: 广场上搭起木台}
      - {id: vote,   at: "D4 08:00", text: 投票开始}
    hooks:                         # 玩家可以参与的切入点（给 AI 与快捷行动用）
      - {id: side_guild,   label: 替行会拉票,   tags: [join]}
      - {id: expose_bribe, label: 揭发贿选,     tags: [disrupt], requires: 'knows("brass:item/ledger", "contents")'}
    outcomes:
      - id: guild_wins
        when: 'event.vars.guild_votes >= event.vars.council_votes'
        effects: [{effect: {type: flag_set, values: {flag: bell_to_guild}}}]
      - id: council_wins
        default: true              # 没人插手时的结局
        effects: [{effect: {type: flag_set, values: {flag: bell_to_council}}}]
    resolve: rules                 # rules：按 outcomes 条件 + world 随机流；ai：交给 world_sim 决定细节（仍受 outcomes 白名单约束）
    vars: {guild_votes: 4, council_votes: 6}
```

## 6.3 运行时状态机

```text
scheduled ──(到达 window.start，preconditions 成立)──▶ active ──(阶段推进)──▶ ... ──(到达 end 或 hook 完成)──▶ resolved
    │                                     │
    │(preconditions 永久不成立 / AI 取消)   │(玩家参与 / 破坏：修改 event.vars，或 AI 通过 timeline_patch 改条件)
    ▼                                     ▼
 cancelled                           (仍然 active)
```

- **调度**：`TimeAdvanced` 之后，引擎扫描到期事件（按 `window.start`、ID 排序，保证确定性）。
- **玩家在场**：事件地点 = 玩家位置，或玩家是参与者 → 事件进入“活跃场景”：上下文注入 `[WORLD_EVENT]` 段，快捷行动出现 `hooks`。
- **玩家不在场**：到 `end` 时自动结算：`resolve: rules` 由引擎按条件选结局（`world` 随机流打破平局）；`resolve: ai` 让 `world_sim` 在 `outcomes` 白名单中选择并写细节（失败时退回 `default`）。
- **错过的事件**：结算后写 `WorldEventResolved{by: world}`，并按地点距离生成传闻（第 6.5 节）。世界继续前进，没有“失败结局”。

## 6.4 事件

| 事件 | 说明 |
|---|---|
| `WorldEventScheduled` | AI / 玩家指令新增时间线事件（故事包内的事件不需要这条） |
| `WorldEventStarted` / `WorldEventStageAdvanced` | 进入窗口 / 阶段推进 |
| `WorldEventJoined` / `WorldEventDisrupted` | 玩家通过 hook 或自由行动参与 / 破坏（`Key = hook`，`Values = vars 增量`） |
| `WorldEventResolved` | `Outcome`、`Source = world / player / ai` |
| `WorldEventCancelled` | 前提失效或 AI 取消 |
| `RumorHeard` | 玩家听到传闻（进入知识层，`level = rumored`） |

## 6.5 传闻与认知

- 传闻是知识层的一种：`level = rumored`，可能与真相不同（故事包或 AI 可以写假传闻，`claim` 文本独立于客观值）。
- 结算后的世界事件按地点距离（路程分钟数）延迟扩散：同地点立即，邻近地点 6 小时，远处 1–3 天；玩家到达或与相关 NPC 交谈时触发 `RumorHeard`。
- “时间线 / 传闻”页只显示玩家已知的事件：未来事件显示倒计时（“约 1 天后”），已错过的显示“你错过了：……（结果）”。

## 6.6 时间跳跃

输入栏“等待……”提供：10 分钟 / 1 小时 / 到天亮 / 到某个已知事件开始。跳跃时：

1. 引擎逐段推进时间（每段 ≤ 6 小时），每段处理到期事件；
2. 期间玩家所在地发生的事件会**打断跳跃**（“你正要睡下，广场上传来喧哗”）；
3. 跳跃结束后 `world_sim` 生成一段“期间发生”的摘要（离线模式用模板拼接事件标题）。

单次跳跃上限 7 天；超过 30 个到期事件时剩余事件批量按规则结算，只汇总标题。

## 6.7 世界模拟 `world_sim`

替代 v0.1 的导演提案。职责：

- 为 `resolve: ai` 的事件写结局细节；为离屏事件生成后果（WorldChange）；
- 在世界“太安静”时（Pacing：连续 N 回合无事件）提议新的时间线事件或委托（`timeline_add`），受影响评估约束；
- 时间跳跃摘要。

离线（无模型）时：用离线委托模板 + 规则结算，不新增设定实体。

## 6.8 失败模式

| 情况 | 处理 |
|---|---|
| 关键参与者已死，事件前提失效 | `WorldEventCancelled`（原因可见）；若是 `pivotal`，计入影响评估 |
| 玩家在事件进行中离开 | 事件继续，按离开时的 vars 结算 |
| AI 不断新增事件导致信息爆炸 | 每世界日最多新增 3 个时间线事件；importance ≥ 4 的新事件计 major 影响 |
| 时间跳跃中 AI 失败 | 用规则结算 + 模板摘要；不重试 |

---

# 7. 影响评估与偏离提示

## 7.1 三类提示

| 类型 ID | 名称 | 触发 | 回滚目标 |
|---|---|---|---|
| `lore_deviation` | 彻底偏离设定 | 玩家决定或 AI 修改导致设定层级 `core` 的内容被改写、`pivotal` 时间线事件被取消 / 结局被改变、世界规则被突破 | 上一回合 |
| `major_death` | 重要角色死亡 | importance ≥ 3（可调）的人物 `retire(reason=death)`，包括玩家角色在硬核模式下死亡 | 上一段对话状态（= 上一回合） |
| `story_impact` | 严重影响故事 | 势力毁灭 / 易主、重要地点毁灭、隐藏真相被大面积揭示、一次性影响分 ≥ 阈值的组合变更 | 上一回合 |

## 7.2 影响分（引擎计算，0–100）

每项变更：

```text
base(op)        create 5 · patch 10 · link/unlink 5 · retire 30 · timeline_cancel 25 · timeline 结局改变 30
× importance    importance 1..5 → ×0.4 / 0.7 / 1.0 / 1.4 / 2.0
+ canon         flavor 0 · minor 5 · major 15 · core 35
+ locked        修改保护字段 +20
+ pivotal       涉及 pivotal 事件 +25
```

本回合影响 = 最大单项分 + 其余各项分之和 × 0.25（上限 100）。AI 自评等级（`none / minor / major / break`，对应 0 / 30 / 60 / 85）只能**抬高**不能降低引擎的分数。类型按触发条件判定；一个回合同时满足多种时只弹一次，按 `major_death` > `lore_deviation` > `story_impact` 的优先级显示，面板里列出全部变更。

## 7.3 灵敏度

每类提示独立设置（“设置 → 提示与灵敏度”）：

| 灵敏度 | `lore_deviation` / `story_impact` 阈值 | `major_death` 触发的 importance |
|---|---|---|
| 关 | 不提示（仍写日志） | 不提示 |
| 低 | ≥ 80 | 5 |
| **中（默认）** | ≥ 60 | ≥ 4 |
| 高 | ≥ 40 | ≥ 3 |

每类还有**模式**：`弹窗确认`（默认）/ `仅通知，不打断`。仅通知时在叙事流里插入一张变更卡片，附“回到上一回合”按钮（在接下来 3 个回合内有效；之后需要用时间线导航）。

## 7.4 运行时

```go
type Decision struct {
    ID        string   `json:"id"`
    Type      string   `json:"type"`       // lore_deviation / major_death / story_impact
    Score     int      `json:"score"`
    Changes   []string `json:"changes"`    // WorldChange IDs
    Summary   string   `json:"summary"`    // 玩家可见摘要（不含隐藏真相）
    Rollback  Ref      `json:"rollback"`   // {branch, seq, turn}：上一回合末尾
    Checkpoint string  `json:"checkpoint,omitempty"` // 自动检查点 ID
}
```

1. 提交 B 后发现超过阈值 → 先创建自动检查点（第 10.5 节），再写 `DecisionRequested`，`state.Pending = decision`。
2. `Pending` 非空时引擎拒绝新的玩家命令（只接受 `resolve_decision`、查询、回滚），App 重新打开时自动弹出面板 → **进程被杀也不会丢失待决选择**。
3. **接受**：写 `DecisionResolved{outcome: accepted}`，清空 `Pending`，继续。
4. **回滚**：写 `DecisionResolved{outcome: rolled_back}` 到当前分支末尾，然后执行回滚到 `Rollback`（第 10.3 节）：被回滚的分支保留，标记为“已回滚”（是否默认保留见第 19 节待决问题 2）。
5. 面板勾选“以后这类情况只通知”= 直接把该类型的模式改为“仅通知”，可在设置里改回。

## 7.5 失败模式

| 情况 | 处理 |
|---|---|
| 用户在面板出现前就退出 | `Pending` 持久化，下次进入游戏立即弹出 |
| 影响由场外 `world_sim` 产生（玩家没做什么） | 照常提示，文案改为“世界发生了重大变化”；回滚目标为该系统回合之前 |
| 时间跳跃中途产生多次高影响 | 跳跃暂停在第一个高影响点，提示后由玩家决定是否继续跳跃 |
| 审查修复（audit）触发阈值 | 不弹窗，只通知（审查修复不应打断玩家）；可单项撤销 |

---

# 8. 玩家知识层

## 8.1 原则

- **一切从未知开始**：人物、关系、图鉴、角色卡、机甲卡、地点、势力、时间线事件，包括它们的每一个字段。
- 例外：玩家自己的角色卡（全部已知）、开局地点名称与外观、故事包 `start.known` 列出的条目、预设主角的 `start_knowledge`（例如“认识自己的妹妹”）。
- 揭示**按字段**进行，由 AI 在 WORLD 段里提出 `reveals`，由引擎校验后写入 `KnowledgeRevealed`。
- 未揭示的字段在 UI 中显示 **未知**；知道存在但不知道名字的人物显示 `alias_unknown`（“高个子的擂台选手”）。

## 8.2 字段目录（每种实体可揭示的字段）

| kind | 字段 |
|---|---|
| character | `existence` `name` `appearance` `identity` `faction` `background` `personality` `goals` `abilities` `stats` `equipment` `mech` `location` `status`（生死）`hidden:<id>` |
| relation（a>b） | `existence` `label`（师徒 / 仇人……）`dims`（各维度数值）`history` |
| location | `existence` `name` `description` `exits` `controller` `dangers` `hidden:<id>` |
| faction | `existence` `name` `leader` `goals` `members` `territory` `stance`（对玩家）`hidden:<id>` |
| item / weapon | `existence` `name` `description` `stats` `effects` `origin` `hidden:<id>` |
| mech | v0.1.2 的 `model` `spec:*` `slot:*` `lore` + `pilot` `status` |
| codex / lore / tech | `existence` `title` `summary` `details` `hidden:<id>` |
| world_event | `existence` `time` `location` `participants` `outcome` |
| enemy | `existence` `name` `stats` `part:<id>`（部位 / 弱点）`skills` |

## 8.3 数据模型

```go
// internal/knowledge
type Level int // 0 未知 · 1 传闻 · 2 已知
type Fact struct {
    Level   Level           `json:"l"`
    Turn    int             `json:"t"`
    Channel string          `json:"c"`             // witness / dialogue / document / inference / rumor / system
    Source  string          `json:"s,omitempty"`   // 说出它的 NPC / 文件物品 ID
    Rev     int             `json:"r"`             // 揭示时实体的修订号
    Claim   string          `json:"claim,omitempty"` // 传闻文本（可能为假）
    Value   json.RawMessage `json:"v,omitempty"`   // 数值类字段的快照（关系维度、生死等）
}
type Store struct {
    Facts map[string]map[string]*Fact `json:"facts"` // 实体 ID → 字段 → 认知
}
```

## 8.4 揭示提案与引擎校验

AI 输出：

```json
{"entity": "brass:character/tock", "fields": ["faction"], "channel": "dialogue", "source": "brass:character/tock", "evidence": "“议会那帮人要是知道我替煤烟帮跑腿……”"}
```

| 检查 | 规则 |
|---|---|
| 存在性 | 实体与字段存在于生效世界 |
| 渠道合理性 | `witness`：实体在场或事件对玩家可见；`dialogue`：说话者在场，且说话者**自己知道**该字段（NPC 知识：本人字段、所属势力的公开信息、故事包 `known_by`、NPC 记忆 / 信念）；`document`：文件物品在背包或场景中；`inference`：只能揭示到“传闻”级，每回合 1 项，且不能是 `hidden`；`system`：只有引擎可用 |
| 隐藏真相 | `hidden:<id>` 需要 `reveal_when` 成立；没有 `reveal_when` 的真相只要渠道合理即可揭示，但计入 `story_impact` 影响分（每条 +15）。（是否允许 AI 在没有前提条件时揭示真相见第 19 节待决问题 1） |
| 频率 | 每回合最多 8 个字段（云端）/ 3 个（本地） |
| 单调性 | 已知不会降级为未知；传闻可以升级为已知（真相与传闻不同时，UI 标注“传闻有误”） |

被拒绝的揭示进入诊断表；若叙事正文已经说出该内容，Guard 会尝试改写（见下），否则由审查 Agent 跟进。

## 8.5 与叙事的配合

- **可揭示集合**：上下文构建时，引擎预先计算本回合“渠道已满足”的字段（在场 NPC 自己知道的事、场景中的文件、当前可见实体），作为 `[REVEALABLE]` 段交给叙事 AI。叙事只应揭示这个集合里的内容。
- **隐藏真相**：叙事 AI 只拿到与本场景相关、且**属于可揭示集合**的真相原文；其他相关真相只给“存在一个与 X 有关的秘密，不要说破”的提示，用于埋伏笔。
- **Guard 新增两项检查**：①正文出现隐藏真相的 `leak_markers` 而该真相未在本回合被接受揭示 → 删除该句或重写；②正文用名字称呼玩家还不知道名字的人物 → 替换为 `alias_unknown`。
- **过时**：实体修订号大于 `Fact.Rev` 时，UI 在该字段旁显示“（第 3 天得知，可能已过时）”。关系数值和生死状态显示快照值，而不是当前真值。

## 8.6 与现有系统的关系

- `MechState.Known`、`RPG.Known`（关系）、`RPG.Codex` 合并进 `knowledge.Store`；0.1.x 存档不兼容，所以无需迁移。
- 只读工具的 `player` 范围改为完全由知识层驱动：`pack_get_entity` 只返回已知字段，未知字段返回 `"未知"`。

## 8.7 事件

| 事件 | 说明 |
|---|---|
| `KnowledgeRevealed` | `Reveal{entity, fields, level, channel, source, claim, values}` |
| `RumorHeard` | 见第 6.4 节（本质是 `level = rumored` 的揭示） |

---

# 9. 自由战斗

## 9.1 流程

```text
“趁它喷完蒸汽的空档，滑到它左后腿下面，用扳手卡进膝关节”
   │ combat_adjudicate（AI，结构化输出）
   ▼
CombatIntent{ actions: [move(贴近), attack(target=看门犬, part=左后膝关节, means=重型扳手)],
              circumstances: [exploit_opening(依据：喷气冷却)], self_check: "合理" }
   │ 引擎：合理性检查 → 修正表 → 程度判定掷骰 → 伤害 / 状态 / 消耗 → 敌方行动
   ▼
ActionAdjudicated{ plausibility: ok, mods: [+2 知晓弱点, +2 抓住破绽, -1 移动后攻击], dice: 2d10=13, total: 16, dc: 14, degree: success }
UnitHPChanged(-18) · UnitStatusApplied(关节卡死, 2) · UnitResourceChanged(体力 -6)
   │ 叙事（AI 或模板）
   ▼
“蒸汽从它的鳃缝里喷尽的一瞬……”
```

## 9.2 CombatIntent（AI 输出）

```json
{
  "actions": [
    {"kind": "move", "to": "near:enemy-1"},
    {"kind": "attack", "target": "enemy-1", "part": "knee_l", "means": "brass:item/heavy_wrench", "skill": null, "style": "pin"}
  ],
  "circumstances": [{"tag": "exploit_opening", "why": "它刚喷完蒸汽，正在冷却"}],
  "absurd": null,
  "self_check": "合理"
}
```

- `kind` 白名单：`attack` `skill` `maneuver`（推撞 / 绊倒 / 缴械 / 擒抱 / 致盲……，由故事包 `combat.maneuvers` 定义）`move` `defend` `item` `mech`（启动 / 脱离）`talk`（劝降 / 挑衅）`flee` `env`（利用场景物体）。
- 每回合最多 2 个动作（1 次移动 + 1 次主动作）；多余的丢弃并在叙事里说明“你只来得及……”。
- `circumstances.tag` 只能从故事包 / 引擎的**修正标签表**里选；AI **不能写数值**。

## 9.3 合理性检查（引擎，确定性）

| 检查 | 依据 | 结果 |
|---|---|---|
| 装备 | `means` 必须已装备，或在背包里且可快速取用（`quick: true`）；否则消耗移动动作取出 | 不满足 → 换成徒手 / 已装备武器 |
| 技能 | 技能已掌握、资源（体力 / 热量 / 能源）足够、不在冷却 | 不满足 → 降级为普通攻击 |
| 体力 | 体力 < 25% 时 −2；不足以支付消耗 → 降级 | — |
| 距离 | 距离带 `near / mid / far`；近战需要 near；移动后攻击 −1 | 够不着 → 只执行移动 |
| 部位 / 弱点 | 部位必须存在于目标定义（`enemy.parts`）；**弱点加成只有玩家知识层已知该部位时才给**；未知时按“碰运气”−1，成功则揭示该部位 | — |
| 场景 | `env` 引用的物体 / 条件必须是当前场景事实（“油桶”“火”“高处”） | 不存在 → 丢弃该修正 |
| 修正标签前提 | 每个标签有前提：`exploit_opening` 需目标带“冷却 / 硬直 / 倒地”类状态；`ambush` 需目标未察觉；`high_ground` 需场景事实 `high_ground` | 不满足 → 丢弃该修正 |
| 物理尺度 | `size_classes` 相差 ≥ 2 级时，擒抱 / 投掷 / 推飞类动作视为荒谬 | 降级 |
| 能力边界 | 使用角色没有的能力（无魔法的世界里“放火球”）或违背 `ability_limits` | 降级或拒绝 |

合理性等级：`合理` / `勉强`（额外 −2）/ `荒谬 → 降级`（换成同意图里最接近的合法动作，例如“踢上钟楼”→ 推撞，并附 `downgrade_reason` 供叙事解释）/ `不可能 → 拒绝`（目标不存在等，返回可选动作，不消耗回合）。

## 9.4 修正与掷骰

```text
总值 = 骰子 + 属性修正 + 技能修正 + Σ情境修正（夹紧在 ±6）
难度 DC = 10 + 目标防御修正 + 动作难度（部位 / 动作类型）+ 状态修正
差值 = 总值 − DC
```

| 差值 | 程度 | 效果 |
|---|---|---|
| ≤ −6 | 大失败（随机性 ≥ 40 才启用） | 严重后果 |
| −5 … −1 | 失败 | 后果 |
| 0 … +1 | 勉强成功 | 伤害 ×0.5，不附加状态 |
| +2 … +7 | 成功 | 伤害 ×1.0，附加动作状态 |
| ≥ +8 | 大成功 | 伤害 ×1.5，状态持续 +1，可能额外揭示弱点 / 破坏部位 |

伤害沿用 v0.1 公式（攻击 × 技能威力 vs 防御），再乘程度倍率、部位倍率（故事包定义，弱点通常 ×1.5）与随机浮动。失败后果按随机性档位从表中取：失衡（下回合 −2）、被反击（敌方获得一次优势攻击）、额外体力消耗、武器卡住（硬核）、暴露位置。

敌方行动沿用故事包 `ai` 规则选择技能与目标，用同一套程度判定结算（敌方攻击 vs 玩家防御 DC），保证双方规则对称。

## 9.5 随机性刻度（故事包预设）

`balance.randomness`：0（叙事）↔ 100（硬核）。

| 参数 | 叙事 0–30 | 平衡 31–69 | 硬核 70–100 |
|---|---|---|---|
| 骰子 | 3d6（均值 10.5，方差最小） | 2d10（均值 11） | d20（均值 10.5，方差最大） |
| 大失败 | 关闭 | 差值 ≤ −8 | 差值 ≤ −6 或掷出 1 |
| 伤害浮动 | ±10% | ±20% | ±35% |
| 合理性判定 | 宽松（“勉强”不扣分） | 正常 | 严格（“勉强”−3） |
| 失败后果 | 轻（失衡 / 体力） | 中（+ 被反击） | 重（+ 武器卡住 / 受伤状态） |
| 玩家战败 | 不会死亡：被俘 / 被救 / 撤退 | 重伤（长期状态） | 可能死亡（触发 `major_death` 提示，可回滚） |

天之炽私有包预设 `randomness: 25`（叙事向）；原创示例《锈钟镇》预设 `45`。玩家能否在开局覆盖预设见第 19 节待决问题 3。

## 9.6 机甲

机甲形态的 HP / 热量 / 能源规则不变。自由战斗额外支持：部位（机甲卡新增 `parts`，例如“左臂挂点”“冷却鳍”），凡人对机甲的动作受 `world/rules.yaml` 约束（例如只有“破甲”类修正标签能让凡人武器有效）。

## 9.7 降级与离线

| 情况 | 处理 |
|---|---|
| `combat_adjudicate` 超时 / 坏 JSON | 修复 + 重试 1 次 → v0.1 确定性规则解析器（`resolver.ResolveCombat`）→ 仍失败则拒绝并显示可选动作按钮 |
| 本地模型做裁定 | 只输出 `actions`，不允许 `circumstances` 之外的自由标签；合理性一律按“正常”档 |
| 内容审核拒绝 | 回退规则解析器；叙事用模板 |

## 9.8 事件

新增 `CombatIntentParsed`（原文、意图、来源）、`ActionAdjudicated`（合理性、修正明细、骰子规格与点数、DC、程度、降级前后）、`PartHit`（部位、倍率、是否破坏）。v0.1 的 `CombatActed` 保留给敌方 AI 动作。重放只依赖事件与 RNG 计数器，不重新调用 AI。

---

# 10. 存档：回滚、分支、检查点、导出导入

## 10.1 概念

| 概念 | 说明 |
|---|---|
| 存档（slot） | 一局游戏。包含若干分支 |
| 分支（branch） | 一条时间线。根分支叫“主干”；从某个回合回滚后再行动，就产生新分支。旧分支**永远保留**（除非玩家删除），可以随时切换 |
| 头（head） | 当前分支的最新位置（分支 ID + 事件序号） |
| 检查点（checkpoint） | 指向某个分支某个位置的命名书签；自动或手动创建 |
| 回滚 | 把“当前位置”移动到某个回合末尾；下一次提交时才真正分叉，避免产生空分支 |

## 10.2 存储结构（eventstore schema v2）

```sql
-- 新数据库文件 ibukirpg-v2.db（不覆盖 0.1.x 的 ibukirpg.db，降级安装旧版 App 仍能玩旧档）
CREATE TABLE branches (
  slot_id TEXT, branch_id TEXT, name TEXT, parent_branch TEXT, fork_seq INTEGER, fork_turn INTEGER,
  rng_salt INTEGER, status TEXT,            -- active / rolled_back / archived
  created_at INTEGER, head_seq INTEGER, PRIMARY KEY (slot_id, branch_id));
-- events / commands / snapshots / transcript / memory 增加 branch_id 列；
-- events.seq 在存档内全局单调（跨分支），因此 (slot_id, seq) 仍是主键
CREATE TABLE checkpoints (
  slot_id TEXT, id TEXT, branch_id TEXT, seq INTEGER, turn INTEGER, name TEXT,
  kind TEXT,                                 -- auto / manual
  reason TEXT, created_at INTEGER, PRIMARY KEY (slot_id, id));
CREATE TABLE ai_calls (                      -- token 记账（非游戏事实，不参与重放）
  slot_id TEXT, branch_id TEXT, command_id TEXT, task TEXT, provider TEXT, model TEXT,
  prompt_tokens INTEGER, completion_tokens INTEGER, cached_tokens INTEGER, latency_ms INTEGER, ok INTEGER, error TEXT);
CREATE TABLE world_change_index (...);       -- 世界变更日志投影（第 12.1 节），可从事件重建
CREATE TABLE world_change_rejects (...);     -- 被拒绝的提案（诊断）
-- saves 表增加 current_branch、pending_fork_branch、pending_fork_seq
```

**谱系**：分支 B 的状态 = 沿父链取 `(祖先分支, ≤ fork_seq)` 的事件 + B 自己的事件。`StateAt(slot, branch, seq)`：取谱系上不晚于目标的最近快照（祖先的快照只要 `seq ≤ fork_seq` 就可复用），再重放之后的事件。v0.1.2 已有的 `StateAt` 是这一逻辑的单分支特例。

## 10.3 回滚（“回到这里”）

1. 玩家长按叙事流中的某条消息 →“回到这里”，或在时间线导航里选某个回合。
2. 引擎计算目标 `(branch, seq)` = 该回合最后一个事件；`saves.pending_fork_* = 目标`；状态与叙事流立即显示为目标位置的样子（叙事流在分叉点之后的消息不再显示，顶部出现“已回到回合 52 · 继续行动将创建新分支 · 取消”横幅）。
3. 玩家**取消** → 清空 `pending_fork`，回到原分支头部，没有任何写入。
4. 玩家**提交新行动** → 事务内创建新分支（`parent = 原分支, fork_seq = 目标 seq, rng_salt = 新盐`），`current_branch` 指向新分支，然后正常提交。
5. 回滚目标就是当前分支头部时为空操作。

待决选择（`Pending`）存在时，回滚只能回到它的 `Rollback` 目标或更早。

## 10.4 随机数与分支

如果分支沿用父分支的 RNG 计数器，回滚后输入同样的行动会得到同样的骰子——回滚就失去了意义。因此**每个新分支有一个随机盐**（记录在 `branches.rng_salt` 与分支的第一个事件里），RNG 键变为 `rng(namespace, entity, purpose, salt)`。重放仍然确定：同一分支、同一盐、同样的命令得到同样的结果。

## 10.5 检查点

| 来源 | 时机 | 默认 |
|---|---|---|
| 自动 · 重要决定 | 影响评估触发任一提示类型之前（无论最终接受还是回滚）；玩家的行动使 `pivotal` 时间线事件偏离默认结局之前；进入战斗之前（importance ≥ 4 的敌人） | 开启（设置可关） |
| 自动 · 时间 | 每个世界日开始 | 开启 |
| 手动 | 叙事流长按“设为检查点”、时间线导航“新建检查点”、输入 `/cp 名字`（CLI） | — |

自动检查点保留最近 20 个（可调 5–100），手动检查点不自动删除。检查点只是指针，不复制数据，占用空间可忽略。

## 10.6 导出 / 导入

文件：`<存档名>-<日期>.ibksave`（zip）。

```text
manifest.json     {format: "ibukirpg-save", format_version: 1, engine_version: "0.2.0-alpha1", api: "v2",
                   schema: 2, pack: {id, version, content_hash}, player_name, exported_at,
                   branches: "all" | "current", includes_debug: false}
branches.json     分支元数据
events.jsonl      事件（按 seq）
snapshots.jsonl   每个分支最新快照（加速导入，可省略）
transcript.jsonl  叙事记录
memory.jsonl      记忆摘要
checkpoints.json
debug/ai_calls.jsonl   可选：调试包才包含（token 与错误信息，不含请求正文）
```

- **永不包含**：API Key、服务商配置、本地模型路径、设备信息、故事包内容本身。导出前对整个包做一次“密钥模式扫描”（`sk-`、`Bearer ` 等），命中则中止并报错（防御性检查）。
- 选项：仅当前分支 / 全部分支；“调试包”（附 AI 调用记录，便于反馈问题）。
- Android 通过 SAF（`ACTION_CREATE_DOCUMENT` / `ACTION_OPEN_DOCUMENT`）读写，CLI 用 `/export 路径`、`/import 路径`。

导入校验（任何一步失败都不留下残留数据）：

| 检查 | 结果 |
|---|---|
| zip 安全（同故事包导入的限制） | 拒绝 |
| `format` 不是 `ibukirpg-save` 或没有 manifest | 拒绝：“这不是 ibukiRPG 存档文件” |
| `schema` < 2 或 `engine_version` < 0.2.0 | 拒绝：见第 10.7 节文案 |
| `engine_version` 比当前 App 新 | 拒绝：“存档来自更新的版本 x.y.z，请先更新 App” |
| 故事包未安装 | **警告并允许导入**：存档标记“缺少故事包「名称 id@版本」”，安装后可读 |
| 故事包版本不同 | 按 `save_compat` 判断：覆盖则警告后导入，不覆盖则导入但无法读取（标明原因） |
| 版本相同但 `content_hash` 不同 | 警告：“你安装的故事包内容与导出时不同，可能出现不一致” |
| 事件重放校验 | 导入后在后台重放到各分支头部，与快照比对；不一致则标记“存档可能已损坏”，仍可读取 |

导入的存档总是新建一个存档槽（新 slot ID），不会覆盖已有存档。

## 10.7 0.1.x 存档

- 升级到 0.2.0 后，App 打开新数据库 `ibukirpg-v2.db`；检测到旧的 `ibukirpg.db` 时，存档列表底部显示折叠分组“旧版本存档（3）”，点开说明：

> 这些存档来自 v0.1.x。0.2.0 改成了开放世界，存档结构完全不同，无法继续。如需继续旧存档，请安装 v0.1.2；也可以在这里删除它们释放空间。

- 导入 0.1.x 的数据库文件或旧格式导出：拒绝，同样文案。
- 不做有损转换。

## 10.8 失败模式

| 情况 | 处理 |
|---|---|
| 回滚时存在进行中的 AI 生成 | 先取消生成（叙事以模板补齐），再回滚 |
| 分支过多导致存档膨胀 | 存档详情页显示每个分支的事件数与大小；可删除分支（不能删除当前分支和其祖先段） |
| 导入时磁盘空间不足 | 事务回滚，提示所需空间 |
| 分支谱系很深导致加载慢 | 快照间隔 25 事件不变；跨分支切换时为新分支头部立即写一个快照 |

---

# 11. AI 层：服务商、任务路由、合并输出

## 11.1 服务商注册表（“API 设置”）

```kotlin
// Android：DataStore 保存非敏感配置；Key 用 Android Keystore（AES-GCM，沿用 KeyCipher）加密后保存
data class ProviderConfig(
  val id: String,              // 本地 UUID
  val type: ProviderType,      // DEEPSEEK / QWEN / OPENAI_COMPAT / LOCAL_LLAMA
  val name: String,            // 显示名，可改
  val baseUrl: String,
  val keyRef: String?,         // Keystore 中加密 Key 的引用（LOCAL_LLAMA 为空）
  val models: List<ModelConfig>,
)
data class ModelConfig(
  val id: String,              // 服务商的模型名，如 deepseek-chat / qwen-plus
  val label: String?,
  val contextTokens: Int,      // 上下文长度
  val supportsTools: Boolean,  // 函数调用
  val supportsJsonMode: Boolean,
  val tier: CapabilityTier,    // FULL / LIMITED / MINIMAL（本地模型按参数量自动判定，可手动调低）
)
```

- 预设：DeepSeek（`https://api.deepseek.com`，`deepseek-chat` / `deepseek-reasoner`）、通义千问（DashScope 兼容模式，`qwen-plus` / `qwen-max` / `qwen-turbo`）、自定义 OpenAI 兼容、本地 llama.cpp（模型来自导入的 GGUF，可登记多个文件，同一时间只驻留一个）。
- “测试连接”：发送一次最小请求，顺带探测 `/models` 列表、`tools` 与 `response_format` 支持情况。
- Go 引擎**从不持久化 Key**：Kotlin 在调用 `configure_ai` 时把解密后的 Key 放在内存里传给 Go，Go 只保存在进程内存中；存档、导出、日志都不包含 Key（沿用 v0.1.2 的原则）。

## 11.2 任务（“生成设置”）

| 任务 ID | 名称 | 范围 | 输出 | 默认建议 |
|---|---|---|---|---|
| `narrate_world` | 叙事 + 世界更新（合并） | player + 相关隐藏真相 | 叙事正文 + WORLD 段 | 云端主力模型 |
| `parse_action` | 意图解析 | player | Command 结构 | 本地 3B 或云端快速模型 |
| `combat_adjudicate` | 战斗裁定 | player + 敌人公开信息 | CombatIntent | 云端 |
| `memory` | 检索 / 摘要 / 记忆压缩 | 按调用方范围 | 文本 / 关键词 | 本地 |
| `world_sim` | 世界模拟（场外事件、传闻、时间跳跃摘要） | director | WorldChange[] + 文本 | 云端 |
| `audit` | 一致性审查 | director | AuditReport | 云端推理模型 |
| `card_gen` | 卡片生成（人物 / 武器 / 物品 / 机甲 / 设定） | director | EntityDoc 提案 | 云端 |
| `char_review` | 角色审查 | director（输出过滤为玩家可见） | CharacterReview | 云端 |

配置：

```json
{
  "mode": "per_task",                       // unified：所有任务用同一个模型
  "unified": {"provider": "p-deepseek", "model": "deepseek-chat"},
  "tasks": {
    "narrate_world": {"provider": "p-deepseek", "model": "deepseek-chat", "fallback": [{"provider": "p-qwen", "model": "qwen-plus"}], "temperature": 0.8, "max_tokens": 1200},
    "parse_action":  {"provider": "local", "model": "qwen2.5-3b-q4km", "fallback": [{"provider": "offline"}]},
    "memory":        {"provider": "local", "model": "qwen2.5-1.5b-q4km"}
  },
  "audit_every_turns": 15,
  "show_token_usage": true
}
```

每个任务的降级链：主模型 → 备用模型（可多个）→ 本地模型（若配置）→ 离线规则 / 模板。降级不重新掷骰、不重复执行命令（V0.2 第 3.4 节）。

## 11.3 合并输出格式

叙事与世界更新合并为一次调用。为了**流式友好**并让坏 JSON 不影响叙事，采用“正文 + 分隔段”格式，而不是把叙事放进 JSON 字符串：

```text
托克犹豫了一下，还是把传单抖开……（叙事正文，流式显示）
<<<WORLD>>>
{"v":1,
 "changes":[{"op":"create","kind":"codex","target":"brass.gen:codex/bell_stop_flyer","value":{...},"reason":"托克给你看了传单","evidence":"『明晚，钟停之时。』"}],
 "reveals":[{"entity":"brass:character/tock","fields":["faction"],"channel":"dialogue","source":"brass:character/tock"}],
 "scene_facts":["喷泉边有一张被踩皱的传单"],
 "memories":[{"who":"brass:character/tock","text":"向阿砾透露了替煤烟帮送货的事","importance":3}],
 "timeline":[],
 "impact":{"level":"minor","why":"新增一条图鉴"},
 "suggestions":["追问“钟停之时”","把传单交给议会"],
 "minutes":10}
<<<END>>>
```

- App 流式显示时遇到 `<<<WORLD>>>` 即停止显示，后续内容只给解析器。
- 支持 `response_format: json_schema` 的服务也**不**用 JSON 模式包裹整体（否则叙事无法流式）；只在“修复重试”那一次使用 JSON 模式。
- `suggestions` 取代 v0.1 的独立建议计算（规则快捷行动仍由引擎给出，两者合并显示，AI 建议带 ✨ 标记）。
- `minutes` 只是建议，引擎在动作耗时规则范围内裁决（V0.2 第 18 节）。

## 11.4 坏 JSON、重试、降级

```text
解析 WORLD 段
  ├─ 成功 → schema 校验 → 校验器（第 5.3 节）
  └─ 失败 → 本地修复（去掉 ``` 围栏、补全括号、去尾逗号、单引号、未转义换行、截断到最后一个完整元素）
        ├─ 成功 → 继续
        └─ 失败 → 重试 1 次：只请求 WORLD 段（附上原叙事与解析错误；JSON 模式；max_tokens 800；同一任务模型）
              ├─ 成功 → 继续
              └─ 失败 → 仅叙事：世界更新跳过，写 WorldUpdateFailed(parse)，叙事流显示一行提示
                        “本回合的世界变化没能记录（AI 输出格式错误），叙事已保留。审查时会补上。”
```

叙事本身失败（超时 / 网络）时按备用链切换；全部失败用模板叙事（v0.1.2 行为）。

## 11.5 内容审核拒绝

识别：HTTP 400 且错误码 / 文案包含 `content_filter`、`data_inspection_failed`、`Content Exists Risk` 等；`finish_reason = content_filter`；或输出命中拒绝模式（“抱歉，我无法……”且没有 WORLD 段）。

处理：

1. 本回合 **不提交任何 AI 生成内容**（叙事、世界变更都不写）；规则结果（提交 A）已保存，叙事用模板；写 `WorldUpdateFailed(refused)`。
2. 叙事流显示提示卡：“AI 服务商拒绝生成本回合内容（内容审核）。规则结果已保存。你可以：**换种说法**（回到上一回合并把原输入放回输入框）/ **回到上一回合** / **为叙事换一个模型**（跳到生成设置）。”
3. 不自动换服务商重试（避免把被拒内容发给另一家），除非玩家在生成设置里打开“被拒时尝试备用模型”。
4. 意图解析被拒 → 回退离线解析器；战斗裁定被拒 → 回退规则解析器。

## 11.6 token 用量

- 每次调用记录到 `ai_calls`（任务、模型、输入 / 输出 / 缓存命中 token、耗时、成败）。本地模型由 llama.cpp 返回 token 数。
- 叙事流每回合末尾的小字：“回合 41 · ↑2.1k ↓420 · 1 次调用”，点开看分任务明细（含后台任务归属到触发它的回合）。
- 设置里可关闭显示；“存档详情”显示本局累计；可选填写模型单价以估算费用（默认不显示金额）。

## 11.7 本地模型（llama.cpp）作为子 Agent

| 能力档位 | 判定 | 允许的世界修改 |
|---|---|---|
| FULL | 云端模型 | 全部操作，每回合 ≤ 12 项 |
| LIMITED | 本地 ≥ 3B | `scene_facts`、`memories`、`reveals`（仅 witness / dialogue）、关系微调（单维度 ±5）、patch importance ≤ 2 的实体的非保护字段、create importance ≤ 2 的实体；每回合 ≤ 3 项 |
| MINIMAL | 本地 < 3B | `scene_facts`、`memories`、`reveals`（仅 witness）；每回合 ≤ 2 项 |

- 本地模型输出使用 **llama.cpp 语法约束**（GBNF / JSON schema → grammar）生成 WORLD 段与 CombatIntent，基本消除坏 JSON；本地 HTTP 服务（`LocalModelHttpServer`）新增 `response_format` / `grammar` 参数透传。
- 本地模型叙事 + 世界更新时，提示词使用精简 schema（只含该档位允许的字段）。
- “离线规则模式”（无模型）：世界只能被故事包规则、时间线规则结算改变，不新增设定实体。
- 同一时间只驻留一个本地模型：给不同任务分配了两个不同的 GGUF 时，App 提示“切换模型需要重新加载（约 N 秒），建议所有本地任务使用同一个模型”。

**生成设置右上角提示图标**打开的列表：

| 适合本地模型 | 可以但会变保守 | 不推荐本地模型 |
|---|---|---|
| 检索关键词、摘要与记忆压缩、简单意图解析、传闻 / 环境描写 | 战斗裁定（荒谬动作更易被降级）、世界模拟（只做小改动） | 叙事 + 世界更新、一致性审查、角色审查、卡片生成 |

## 11.8 失败模式汇总

| 情况 | 处理 |
|---|---|
| 某任务未配置模型 | 统一模式回退到统一模型；按任务模式回退到 `narrate_world` 的模型；都没有 → 离线 |
| Key 失效（401） | 本回合按降级链继续；设置页对应服务商显示红点“Key 无效” |
| 速率限制（429） | 指数退避一次（≤ 3 秒），再走备用链 |
| 本地模型内存不足 | 沿用 v0.1.2 的内存策略；拒绝加载时该任务走降级链，并提示 |
| 后台任务（memory / audit）失败 | 不提示，下一个周期重试；连续 3 次失败在设置页显示 |

---

# 12. 一致性：世界变更日志、强制注入、自审

## 12.1 世界变更日志

- 真相源是 `WorldChangeApplied` / `WorldChangeReverted` 事件；`world_change_index` 是可重建的投影：`(slot, branch, change_id, turn, minute, op, target, target_name, path, summary, source, impact, reverted_by)`，并建 SQLite FTS5 全文索引（`summary`、`target_name`、`reason`）；FTS5 不可用时退回 `LIKE`。
- 分支感知：查询只返回当前分支谱系上的变更。
- 查询入口：“世界面板 → 日志 / 世界变更”页（搜索框 + 来源筛选：AI 叙事 / 世界模拟 / 审查修复 / 你的指令 / 外部工具）、只读工具 `world_change_log(query, entity?, since_turn?)`、MCP 同名工具。
- 涉及隐藏真相的变更对玩家显示脱敏摘要（“与托克有关的一项隐藏设定发生了变化”），完整内容只有 Director 范围可见。

## 12.2 叙事前强制注入

每次 `narrate_world` 与 `combat_adjudicate` 之前，上下文构建器选出与本场景相关的变更，作为 `[WORLD_CHANGES]` 段放在 `[RETRIEVED]` 之前，并声明“这些变更优先于故事包原设定”：

1. 目标是场景内实体（在场人物、当前地点、场景物品、在场人物所属势力、当前活跃的世界事件）；
2. 最近 10 回合内 impact ≥ 30 的任意变更；
3. 玩家输入里提到的实体（名称匹配，含别名）的变更。

按影响分与时间排序，预算约 900 token（本地模型 400），超出部分只留一行摘要并提示“更多变更可用 world_change_log 查询”。检索工具返回的实体本来就是生效世界（第 5.1 节），所以即使没有注入，查到的也是改过之后的值，并带“（已变更，见 wc-41-2）”标注。

## 12.3 周期性自审

触发：每 `audit_every_turns` 回合（默认 15，可设 5–50 或关闭）、记忆压缩产生新的长期记忆之后、玩家在日志页点“立即检查”。在后台运行，不阻塞回合。

输入（Director 范围）：自上次审查以来的叙事原文、这些回合的世界变更与被拒绝提案、`WorldUpdateFailed` 标记的回合（优先检查）、涉及实体的生效文档。审查 Agent 可以使用全部只读工具。

输出：

```json
{"findings":[
  {"kind":"omission","turn":44,"text":"叙事中托克把传单交给了你，但背包里没有该物品","fix":[{"op":"patch","target":"player","path":"inventory","value":{"brass.gen:item/flyer":1},"reason":"补记：托克交给你的传单"}]},
  {"kind":"hallucination","turn":46,"text":"叙事称欧琳是议会成员，与设定不符","fix":[],"note":"建议下次叙事更正"},
  {"kind":"contradiction","turn":47,"text":"wc-45-1 与 wc-47-3 对北码头控制者的描述矛盾","fix":[...]}
]}
```

- `omission`（叙事说了但没记账）→ 补记变更；`hallucination`（叙事编造、与世界冲突）→ 优先**不改世界**，而是在下一次叙事注入一条 `[CORRECTION]` 让叙事自然更正；只有当玩家已经基于它行动时才把它正典化为变更；`contradiction`（变更之间矛盾）→ 修正较新的一条。
- 修复以“系统回合”提交（`command_id = audit-<n>`，`source = audit`），走同一套校验器与事件；每轮最多 5 项修复。
- 涉及玩家物品 / 金钱 / 经验 / 生死等**机械状态**的修复**不直接应用**，而是在叙事流插入“审查建议”卡片，由玩家点“应用”或“忽略”。（自动应用的范围见第 19 节待决问题 4。）
- 修复后叙事流顶部出现一枚通知 chip：“一致性检查修复了 2 处 · 查看”。

## 12.4 修复也可以回滚

审查修复是普通的 WorldChange：可在日志页单项撤销（第 12.5 节），也可以通过分支回滚撤销整次审查。

## 12.5 单项撤销

撤销 = 追加一条反向的 `WorldChangeReverted`（`patch` 恢复 `Before`、`create` → `retire`、`retire` → `restore`、`link` ↔ `unlink`）。

- 前提：该变更之后没有其他变更修改过同一路径 / 依赖它新建的实体；否则提示“之后的 3 项变更依赖它”，提供“一并撤销”或“改为回滚到它之前（新分支）”。
- 撤销本身也会进入影响评估（例如撤销一次死亡 = 复活，按 `restore` 计分），但不会弹窗，只通知。

---

# 13. 角色创建与审查

## 13.1 开局流程

```text
选择故事包 → 选择主角：
   ├─ 预设角色（故事包 playable: true 的人物，显示角色卡与开局知识）
   └─ 自建角色 → 填写表单（可“让 AI 补全”）→ 规则检查 → 审查 Agent → 采用 / 修改 / 仍然使用
→ （可选）创建同伴或其他 NPC → 开局设置（随机性展示、提示灵敏度快捷入口）→ 开始
```

## 13.2 创建规则（`rules/creation.yaml`）

```yaml
attributes: {points: 12, min: 6, max: 16, base: 10}
skills: {picks: 3, max_rank: 3, pool: [brass:skill/mechanics, brass:skill/brawl, brass:skill/persuade]}
backgrounds:                       # 出身：决定开局地点、势力、装备、开局知识
  - id: guild_apprentice
    name: 行会学徒
    start: {location: brass:location/workshop, faction: brass:faction/guild, items: [brass:item/wrench]}
    knows: [brass:character/orin]  # 开局已知的实体（字段按 public + 本条列出的字段）
equipment_budget: 30
unique_roles:                      # 世界中唯一的身份；被占用时不可再选
  - {id: bell_keeper, name: 钟楼守钟人, holder: brass:character/old_ham}
tier: trained                      # 开局强度档位
free_text_fields: [appearance, personality, background]   # 自由文字（审查 Agent 检查）
```

## 13.3 两段审查

**① 规则检查（引擎，确定性，必须通过）**：点数与上下限、技能池、装备预算、出身存在、`unique_roles` 未被占用、名字不与已有人物重名（含别名）、文字长度。不通过时表单内标红，不进入 AI 审查。

**② 审查 Agent（`char_review`，Director 范围）**：

```json
{
  "lore_fit":  {"rating": "ok|warn|conflict", "notes": ["锈钟镇没有魔法，背景里的“祖传咒语”需要改写"]},
  "power":     {"rating": "too_weak|ok|too_strong", "score": 112, "band": [40, 90], "notes": ["力量 16 + 擂台冠军背景偏强"]},
  "conflicts": [{"with": "brass:character/tock", "kind": "role_overlap", "note": "同为竞技场头牌选手"}],
  "suggestions": ["把背景改为“曾在竞技场打过两季”", "力量降到 14"],
  "recommended_card": { "...完整的建议角色卡..." }
}
```

- 强度分由**引擎**按 `power_tiers` 公式计算（属性、技能、装备、背景加成），AI 只补充定性意见；`too_weak / too_strong` 以引擎分为准。
- 审查 Agent 能看到隐藏真相以发现冲突，但输出给玩家前会被过滤：引用隐藏信息的意见改写为“与世界中的某个隐藏设定可能冲突”，`recommended_card` 也不得包含隐藏信息（校验器检查 `leak_markers`）。
- UI：三张结论卡（设定契合 / 强度 / 冲突，红黄绿标记）+ 建议列表 + 按钮：**采用推荐卡**（先显示差异）/ **按建议修改**（回到表单并高亮相关字段）/ **仍然使用我的角色**。`conflict` 级别的设定冲突仍允许使用，但会提示“AI 叙事可能会调整这部分设定”；只有规则检查失败会阻止开始。
- 离线 / 本地模型：只做规则检查与引擎强度分，提示“未进行 AI 审查”。

## 13.4 创建 NPC

- 开局向导和游戏中（世界面板 → 角色 → “新建角色”）都可以创建 NPC：表单（名字、身份、所在地、所属势力、与玩家的关系、性格、可选战斗数值）→ 同样两段审查（NPC 用 `power_tiers` 的任意档位，审查重点是设定契合与冲突）→ 作为 `user_request` 来源的 `create` 变更提交，先显示差异确认（第 14.3 节）。
- 玩家创建的 NPC 默认 importance 2，可在表单里调到 3；对玩家**完全已知**（玩家是作者），但 NPC 的 `hidden` 字段可以留给 AI 填写（勾选“让 AI 给 TA 留一个秘密”）。

---

# 14. Agent 能力扩展、写入网关与 MCP

## 14.1 统一写入网关

所有会改变世界的 AI 能力都只产出**提案**，经由同一个网关：

```text
Proposal（WorldChange[] / KnowledgeReveal[] / EntityDoc）
  → change.Validate（第 5.3 节，按来源与能力档位）
  → change.Assess（影响分）
  → Command{Kind: "world_change", Source, Changes, ExpectedHead}
  → engine.Execute → WorldChangeApplied ...
```

游戏内工具（函数调用）、移动端 API、MCP 写入工具全部调用这个网关，**不存在第二条写入路径**。

## 14.2 生成与修改卡片

`card_gen` 任务可以生成 / 修改：人物卡、武器 / 物品卡、机甲卡（含部位、挂点、隐藏字段）、设定条目（势力、地点、科技、历史）、时间线事件。

- 生成的数值必须落在目标强度档位内（引擎计算强度分，超出自动夹紧并标注）。
- 新实体 ID 统一放在 `<pack>.gen` 命名空间，避免与故事包未来版本冲突。
- 立绘：AI 生成的实体没有立绘，显示图标 / 首字（与 v0.1.2 相同）。

## 14.3 两种生效方式

| 情况 | 例子 | 生效方式 |
|---|---|---|
| **游戏过程中**由 AI 产生的修改 | 叙事中托克送你一把改装扳手；世界模拟让煤烟帮占领北码头 | **直接生效**，叙事流显示变更 chip；可在日志页撤销，或回滚 |
| **玩家明确要求**的修改 | 输入栏“指令”模式：“把我的扳手改成带电击的”；卡片上的“✎ 让 AI 修改”；新建 NPC | **先预览**：`preview_change` 返回差异 → 底部面板显示字段级 before / after（新增绿色、删除红色）→ 玩家点“确认修改”才提交；“取消”不留任何记录 |

预览细节：

- 预览返回 `preview_token`（提案内容 + 当前头部的哈希），确认时带上；头部已变化（例如后台审查提交了修复）则要求重新预览。
- 修改玩家不知道的字段时，差异只显示“另有 2 个你还不知道的字段会被修改”，不剧透。
- “指令”模式的输入不推进世界时间、不产生叙事回合，提交后作为一条“你修改了设定”系统消息出现在叙事流（可回滚）。

## 14.4 游戏内工具（函数调用）

在 v0.1.2 只读工具基础上新增：

| 工具 | 类型 | 可用 Agent |
|---|---|---|
| `world_change_log(query, entity?, since_turn?)` | 读 | 全部（按范围脱敏） |
| `knowledge_get(entity)` | 读 | narrator / audit |
| `timeline_get(window?)` | 读 | narrator（只看已知）/ world_sim / audit |
| `world_propose_change(changes[])` | 写（提案） | world_sim / audit / card_gen |
| `entity_generate(kind, brief, tier)` | 写（提案） | card_gen |

`narrate_world` 不通过工具写入，而是通过 WORLD 段（省调用）；工具写入用于多步推理的任务（审查、卡片生成）。

## 14.5 MCP 写入（门控）

MCP 仍是 Command / Query API 之上的适配器（V0.2 第 48 节），默认**只读**。写入需要同时满足：

1. 启动参数 `ibukirpg mcp --allow-write`；
2. 范围为 `director` 或新增的 `author`（作者范围 = director 的读权限 + 写权限，供故事包作者调试；`player` 与 `npc:*` 范围永远只读）；
3. 存档未被游戏进程占用：存档槽持有 `slot_lock`（进程 ID + 心跳，30 秒过期）；CLI / 桌面游戏运行同一存档时，MCP 写入返回“存档正在被游戏使用，请先退出游戏或只读查询”。

新增 MCP 工具：

| 工具 | 说明 |
|---|---|
| `world_preview_change(changes[])` | 返回差异与 `preview_token`、影响分、校验结果（被拒绝项与原因） |
| `world_apply_change(preview_token)` | 只接受预览过的提案；头部变化则失败；以“外部工具”系统回合提交（`source = mcp:<客户端名>`） |
| `world_revert_change(change_id)` | 单项撤销（同第 12.5 节的前提检查） |
| `checkpoint_create(name)` | 新建手动检查点 |
| `world_change_log` / `timeline_get` / `knowledge_get` | 只读 |

MCP 写入**不**弹偏离提示（没有玩家界面），但影响分超过阈值的写入会被拒绝，除非调用时带 `accept_impact: true`；所有 MCP 写入在游戏日志里带“外部工具”标记，下次在 App 里打开存档时显示通知“外部工具修改了 3 项设定”。Android 不运行 MCP 服务（与 v0.1.2 相同）。

---

# 15. App 界面：为模拟 RPG 重新设计

## 15.1 设计原则

1. **叙事流是舞台**：主屏幕始终是叙事流，其他信息以条、chip、底部面板、侧栏的形式叠加，不跳页。
2. **信息分层**：一眼可见（状态条）→ 一次点击（世界面板页签）→ 深入（角色卡 / 变更差异）。
3. **未知可见**：未知不是隐藏，而是显示为带斜纹底的“未知”，让玩家知道“这里还有东西没发现”。
4. **自由优先**：任何时候都能直接打字；快捷行动与 AI 建议只是捷径。
5. **可回头**：每条消息都能“回到这里”；重大变化有明确的接受 / 回滚。
6. **手机竖屏优先，平板自适应**：按 Material 3 窗口尺寸类（Compact < 600dp、Medium 600–839dp、Expanded ≥ 840dp）布局。

设计稿（HTML 渲染，非实机截图；源码在 `docs/design/v03/`，运行 `docs/design/v03/render.sh` 生成 PNG 到 `build/design/v03/`，PNG 不提交）：

| 图 | 内容 |
|---|---|
| `01-main.png` | 主界面：顶栏 + 状态条 + 叙事流（检定卡、知识解锁 chip、世界变更 chip、token 小字）+ 智能输入栏 |
| `02-world-relations.png` | 世界面板（底部面板展开）· 关系网页签：图谱中的未知节点 / 未知关系、选中人物的已知字段 |
| `03-combat.png` | 战斗覆盖模式：固定在叙事流顶部的战斗坞（敌人 HP、部位与弱点、行动顺序）、降级提示、行动裁定卡 |
| `04-deviation-sheet.png` | 偏离提示底部面板（重要角色死亡）：将会改变的内容、自动检查点、接受 / 回到上一回合 |
| `05-timeline-branches.png` | 时间线与检查点：分支切换 chip、带分叉线的回合列表、“回到这里”、检查点 |
| `06-generation-settings.png` | 生成设置（按任务分配）+ 右上角提示打开的“哪些任务适合本地模型” |

## 15.2 导航结构

```text
首页 ── 继续游戏 / 新游戏 / 存档 / 故事包 / 设置
新游戏向导 ── 选择故事包 → 选择主角（预设 / 自建 → 审查）→ 可选 NPC → 开局设置 → 游戏
游戏 ── 叙事流（主屏）
        ├─ 世界面板（底部面板 / 侧栏，8 个页签）
        ├─ 时间线与检查点（全屏对话框）
        ├─ 偏离提示 / 差异确认 / 消息菜单（底部面板）
        └─ 溢出菜单：存档详情、导出存档、设置
设置 ── API 设置 · 生成设置 · 提示与灵敏度 · 存档与检查点 · 显示 · 关于
```

## 15.3 主界面（Compact 竖屏）

自上而下：

1. **顶栏**（56dp）：返回；标题 = 当前地点；副标题 = “故事包 · 第 N 天 时:分 · 天气”；操作：**时间线**（`account_tree`）、**世界面板**（`public`，有未读解锁时显示红点）、溢出菜单。
2. **状态条**（单行，可横向滚动）：生命、体力、故事包资源（蒸汽压 / 能源……）、玩家状态（带剩余回合）、最近的已知世界事件倒计时。点击展开为完整状态卡（v0.1.2 HUD 卡片的演进版）。战斗中状态条并入战斗坞。
3. **叙事流**（`LazyColumn`，沿用分页加载的 TranscriptWindow）。条目类型：
   - 玩家消息（右侧气泡）、叙事正文（无气泡，大字号行距 1.75）；
   - **检定卡**（技能、骰子规格、点数、修正、难度、结果标签）；
   - **知识解锁 chip**（第三色 tertiary 底色，`lock_open` / `campaign`（传闻）/ `hub`（关系）图标）：点击直接打开世界面板对应条目并高亮新字段；
   - **世界变更 chip**（中性色，`edit_note` 图标）：点击打开日志页该变更的差异；
   - **世界事件卡**（“广场上：钟楼议会开始投票”），带“前往 / 加入 / 忽略”；
   - **系统提示**（世界更新失败、内容审核、审查建议、外部工具修改）；
   - **分支标记**（“从回合 52 分出 · 分支 B”）。
   - 每回合末尾的小字：回合号、token 用量、调用次数。
4. **智能输入栏**（底部，`surfaceContainer` 背景）：
   - 第一行：**建议 chip**，混合引擎快捷行动（环顾四周、前往……）与 AI 建议（✨ 标记，来自 WORLD 段 `suggestions`）；
   - 第二行：**模式分段按钮** `行动 | 对话 | 指令`（对话 = 以角色身份直接说话，自动加引号；指令 = 对 AI 的元请求，走预览差异流程），右侧“等待……”（时间跳跃）与“物品”；
   - 第三行：输入框（左侧 `add_circle` 打开动作面板：移动、使用物品、交谈对象、观察目标）+ 发送按钮。
5. **通知 chip**：后台事件（审查修复、场外世界事件结算、外部工具修改）以小 chip 从状态条下方滑出，4 秒后收起，同时记入日志页。

**消息长按菜单**（底部面板）：回到这里 · 设为检查点 · 复制 · 本回合详情（事件、世界变更、token、调用的模型）。回滚后叙事流顶部显示横幅：“已回到回合 52 · 继续行动将创建新分支 · 取消”。

## 15.4 世界面板

- Compact：`ModalBottomSheet`，半展开（约 60%）与全展开两档，顶部有搜索与“全屏”按钮；从顶栏 `public` 按钮或上滑输入栏上方的把手打开。
- Medium：从右侧滑出的模态侧栏（400dp）。
- Expanded（平板横屏）：**常驻右侧栏**（380dp），叙事流居左（最大宽度 720dp），状态条并入顶栏。
- 页签（`ScrollableTabRow`，图标 + 文字）：

| 页签 | 内容 |
|---|---|
| 角色 | 我的角色卡（全部已知）；已知人物列表（筛选：在场 / 已知 / 已故 / 我创建的）；人物卡字段逐项显示，未知字段为“未知”，可能过时的字段带提示；“✎ 让 AI 修改”“新建角色” |
| 关系网 | 力导向图（预计算布局，Canvas 绘制）：实线 = 已知关系（标签与数值），虚线 = 只知道存在的关系，“？”节点 = 只知道存在的人物（显示别名）；势力用方形节点；下方为选中人物的关系详情与变化原因；提供列表视图（无障碍） |
| 图鉴 | 分类（人物 / 地点 / 物品 / 技能 / 敌人 / 机甲 / 势力 / 设定 / 科技）；未发现的条目显示剪影与“未发现”；传闻条目带“传闻”标签 |
| 装备 / 机甲 | 装备槽、背包、机甲卡（未知字段“未知”，部位 / 挂点 / 改装） |
| 地图 / 地点 | 已知地点的连通图（节点 + 路程分钟数），听说过但没去过的地点显示为“？”；点击地点 → “前往”快捷行动 |
| 势力 | 已知势力：对玩家的态度、已知成员、领地、近期变化 |
| 时间线 / 传闻 | 已知的未来事件（倒计时、地点、可参与的切入点）、传闻（可信度）、已结算 / 错过的事件 |
| 日志 / 世界变更 | 搜索框 + 来源筛选；每条变更显示摘要、来源、影响分，展开为差异，可“撤销”；“立即检查一致性”；“被拒绝的修改”开关（调试） |

## 15.5 战斗覆盖模式

战斗不切换页面，而是同一叙事流的一种**模式**：

- 顶栏变为错误色容器（`errorContainer`），标题“战斗 · 第 N 轮”。
- **战斗坞**固定在状态条位置：敌人卡（HP 条、状态、精英 / 首领标记、部位 chip：已知弱点高亮、未知部位显示“未知”），行动顺序 chip 行，玩家 HP / 体力；多个敌人时横向滑动；可折叠为一行。
- 叙事流继续：玩家描述 → **行动裁定卡**（解析、修正明细、骰子、程度、结果）→ 叙事。降级以一行警示卡说明原因。
- 输入栏边框变为错误色、占位文字“描述你的行动……”，建议 chip 变为战斗建议（AI 建议 + 防御 / 启动机甲 / 撤退等引擎动作）。
- 战斗结束时战斗坞收起，叙事流插入战斗结算卡（经验、掉落、伤势）。
- v0.1.2 的 `CombatPanel.kt` 拆为战斗坞与裁定卡组件复用。

## 15.6 偏离提示（底部面板）

- 不可通过下滑或点遮罩关闭（`sheetState` 拒绝隐藏），返回键等同“稍后决定”并保持输入禁用。
- 内容：类型图标与标签（含当前灵敏度）、一句话标题、说明、“将会改变”列表（before → after，最多 3 行 + “查看全部”）、自动检查点提示、主按钮“接受，继续故事”、次按钮“回到上一回合（<上一回合摘要>）”、复选框“以后这类情况只通知，不打断”、“灵敏度”入口。
- 隐藏真相相关的变化以脱敏文案显示。

## 15.7 时间线与检查点

- 全屏对话框（平板为宽对话框）。顶部：分支 chip（名称、回合数、当前标记），可重命名 / 删除 / 导出该分支。
- 主体：纵向回合列表，左侧绘制分支线（当前分支实线，其他分支虚线分出），每行：回合号、世界时间、一句话摘要（取叙事首句或记忆摘要）；检查点以方形标记插入；分叉点带“分叉点”标签，其他分支折叠为一行“主干 · 回合 53–58 · 切换”。
- 选中一行展开操作：“回到这里”“设为检查点”。
- 右下角“新建检查点”；顶栏“导出存档”。

## 15.8 其他关键界面

- **差异确认**（底部面板）：标题“确认修改：改装扳手”，字段级对比（左旧右新，或上下排列；新增绿色、删除红色），未知字段只显示数量；“确认修改”/“取消”。
- **新游戏向导 · 角色**：预设角色卡片网格（立绘、名字、一句话、开局知识数量）+ “自建角色”卡片；自建表单分步（基本信息 → 属性与技能（点数实时计算）→ 出身与装备 → 自由文字）；审查结果页（三张红黄绿结论卡 + 建议 + 推荐卡差异）。
- **存档详情**：分支列表、检查点列表、累计 token、故事包版本、导出按钮。

## 15.9 设置页

| 页面 | 内容 |
|---|---|
| API 设置 | 服务商卡片列表（名称、类型、Base URL、Key 状态：已保存 / 无效、模型数）；右下角“添加服务商”（DeepSeek / 通义千问 / 自定义 OpenAI 兼容 / 本地 llama.cpp）；服务商详情：Key（遮挡显示，Keystore 加密）、测试连接、模型列表（从 `/models` 拉取或手动添加，每个模型的上下文长度、工具调用、JSON 模式、能力档位）；本地 llama.cpp：导入 GGUF、v0.1.2 的推理参数 |
| 生成设置 | 分段按钮“统一模型 / 按任务分配”；任务列表（图标、任务名、当前服务商 · 模型，本地模型带手机图标）；点击任务 → 选择服务商与模型、备用链、温度、最大输出；“被拒时尝试备用模型”开关；“每回合显示 token 用量”开关；一致性审查频率；**右上角提示图标**打开适用性列表（第 11.7 节） |
| 提示与灵敏度 | 三类提示各一张卡：灵敏度四档滑块（关 / 低 / 中 / 高，下方实时说明“importance ≥ 4 的人物死亡时提示”）+ 模式单选（弹窗确认 / 仅通知）；“重要决定前自动创建检查点”开关（默认开）与保留数量 |
| 存档与检查点 | 自动检查点保留数、每日检查点开关、导出默认选项（全部分支 / 当前分支、调试包） |
| 显示 | 主题、字号、叙事行距、是否显示骰子细节 |

## 15.10 实现说明

- 组件与状态：`GameViewModel` 拆分为 `FeedState`、`StatusState`、`InputState`、`WorldPanelState`、`DecisionState`；新的引擎 DTO 见第 16 节。
- 关系图：布局由引擎预计算并随存档缓存（确定性），App 只做绘制与缩放，避免手机上实时力导向。
- 无障碍：未知不仅靠颜色表示（文字“未知”）；图谱提供列表视图；所有 chip 有内容描述。
- 设计稿在实现阶段用 Roborazzi 渲染真实 Composable 截图替换（沿用 `cmd/uifixtures` 导出的真实引擎夹具）。本次设计因当前开发机缺少 JDK / Android SDK，先用 HTML 设计稿表达。

---

# 16. 移动端 API（DTO v2）

`dto.V1` 升级为 `v2`（App 与引擎同包发布，不保留 v1）。新增 / 修改的请求类型：

| 请求 | 说明 |
|---|---|
| `configure_ai` | 载荷改为服务商列表 + 任务路由（Key 仅内存） |
| `get_creation` / `review_character` / `new_game` | 角色创建规则、审查、以预设或自建角色开局（可附 NPC） |
| `submit_text` | 增加 `mode`：`act` / `say` / `meta`；`meta` 返回预览而非回合 |
| `preview_change` / `apply_change` | 差异预览与确认（`preview_token`） |
| `get_pending_decision` / `resolve_decision` | 偏离提示 |
| `get_world_panel(tab)` / `get_entity(id)` | 世界面板数据（全部经过知识层过滤） |
| `search_world_changes` / `revert_change` / `run_audit` | 日志页 |
| `get_timeline` / `rollback_to` / `cancel_rollback` / `switch_branch` / `rename_branch` / `delete_branch` | 时间线与分支 |
| `list_checkpoints` / `create_checkpoint` / `rename_checkpoint` / `delete_checkpoint` | 检查点 |
| `export_save` / `inspect_save` / `import_save` | 导出 / 导入（`inspect_save` 返回版本与故事包匹配情况，供导入前警告） |
| `get_usage` | token 用量（回合 / 存档） |
| `wait` | 时间跳跃 |

删除：`get_hud` 中的主线字段、`quick_action` 的 `mainline` 种类。流式事件新增 `world_update_done`、`decision_requested`、`knowledge_unlocked`、`background_notice`。

---

# 17. 发布计划：0.2.0

## 17.1 两批 alpha

| 批次 | 版本 / versionCode | 仓库 | 内容 |
|---|---|---|---|
| **alpha1** | `0.2.0-alpha1` / 5 | 公开仓库 | 引擎 + App 全部新能力：知识层、世界变更、开放世界时间线、影响评估与提示、自由战斗、回滚 / 分支 / 检查点、存档导出导入、API 设置与生成设置、角色创建与审查、扩展 Agent 与 MCP 写入、新界面；附带迁移到 format 3 的**原创示例包** |
| **alpha2** | `0.2.0-alpha2` / 6 | 私有仓库（故事包）+ 公开仓库（alpha1 反馈修复） | 天之炽私有包按 format 3 重制：大幅扩充设定库；原作剧情改为背景世界事件时间线；可选西泽尔或其他预设主角，或自建角色 |

之后视反馈进入 beta 与 0.2.0 正式版（不在本文范围）。

## 17.2 示例包（alpha1）

- **《锈钟镇》开放世界版**（`brass_trial` → format 3，原创）：钟楼广场、行会工坊、铁环竞技场、下水道、北码头；势力（工匠行会、钟楼议会、煤烟帮）；时间线（议会投票、码头罢工、钟停之时）；人物带 `hidden` 真相；敌人带部位；`randomness: 45`；创建规则含 3 个出身。是所有设计稿的示例内容来源。
- `demo`（边境酒馆）与 `fog_lighthouse`（雾港灯塔）机械迁移到 format 3：事件改为时间线 / 局部事件，人物加知识分层；保留为小型示例。
- `packzip migrate`：把 format 2 包转换为 format 3 骨架（mainline 锚点 → `pivotal` 时间线事件草稿，人物字段 → `discoverable`），输出需要作者手工检查的清单。

## 17.3 天之炽重制（alpha2，私有）

在私有仓库 `packs/tianzhichi` 中进行，公开仓库只出现格式与引擎改动：

- 设定库扩充：地理（城市与区域连通）、势力（及其目标与相互关系）、历史、科技体系、机甲与部件、人物（大幅增加次要人物）、隐藏真相；
- 原作剧情 → 背景时间线：原作关键事件写为带时间窗口、参与者、默认结局的 `pivotal` 事件，玩家可参与 / 破坏 / 无视；
- 预设主角：西泽尔与若干其他人物（`playable: true`，各自的开局知识与出身）；自建角色的出身选项与 `unique_roles`；
- `randomness: 25`（叙事向）；敌人与机甲的部位 / 弱点；
- 私有平衡测试（环境变量门控）扩展到自由战斗的三档随机性；
- 私有截图用 `-Pibuki.fixtures.dir` 渲染到公开仓库之外。

## 17.4 里程碑与任务分解（alpha1）

| 里程碑 | 任务 | 依赖 | 完成标准 |
|---|---|---|---|
| **M0 设计** | 本文档；UI 原型（一次性分支 / 独立 source set，不发布） | — | 用户确认第 19 节待决问题 |
| **M1 存储 v2** | schema v2（新数据库文件）；分支谱系与 `StateAt`；回滚 / 取消回滚 / 切换 / 删除分支；分支 RNG 盐；检查点表与自动规则；0.1.x 检测与拒绝 | M0 | 分支重放 = 线性重放（属性测试）；回滚后同输入骰子不同、同分支重放相同 |
| **M2 世界模型** | 故事包 format 3 加载器与字段目录；覆盖层与 `Resolve`；WorldChange 结构、校验器、反向操作；`WorldChangeApplied/Reverted` 事件；删除主线 / 偏离 / 导演代码；变更日志投影 + FTS | M1 | 随机变更序列 apply → revert = 恒等；旧格式包被拒绝且提示正确 |
| **M3 知识层** | `knowledge.Store`；字段目录；揭示校验（渠道 / 隐藏真相 / 频率）；可揭示集合；Guard 两项新检查；查询与工具全部经过知识层 | M2 | 新局所有非公开字段为“未知”；非法揭示被拒并记录 |
| **M4 开放世界** | 时间线格式与运行时；规则结算与传闻扩散；时间跳跃；`world_sim`；影响评分；`Decision` 持久化与接受 / 回滚 | M2、M3 | 无玩家参与时时间线按默认结局推进到最后；三类提示按灵敏度准确触发 |
| **M5 AI 层** | 服务商注册表与任务路由；合并输出解析、修复、重试、仅叙事降级；内容审核识别；`ai_calls` 记账；本地档位与 GBNF 语法约束；`LocalModelHttpServer` 透传 `grammar` | M2 | 录音回放覆盖：坏 JSON / 被拒 / 超时 / 429 / 401 全部不写坏存档 |
| **M6 自由战斗** | CombatIntent 与裁定提示词；合理性检查表；修正标签表；程度判定；随机性三档；部位与弱点（含知识联动）；降级叙事；规则解析器回退 | M3、M5 | 战斗模拟器在三档随机性下胜率落在目标区间；荒谬动作用例 100% 被降级 |
| **M7 一致性与 Agent** | 强制注入；审查 Agent 与修复；单项撤销；`card_gen`；预览 / 确认；游戏内写入工具；MCP 写入工具与 `slot_lock` | M2、M5 | 注入预算不超限；MCP 写入在游戏占用存档时被拒；审查修复可撤销 |
| **M8 角色创建** | `creation.yaml`；规则检查；强度分；`char_review` 与输出过滤；开局向导；游戏中新建 NPC | M2、M5 | 隐藏信息不出现在审查输出（泄露用例） |
| **M9 Android 新界面** | 主界面（状态条、叙事流新条目、智能输入栏、通知 chip）；世界面板 8 页签与平板自适应；战斗坞与裁定卡；偏离提示；差异确认；时间线与检查点；新游戏向导；API 设置 / 生成设置 / 提示与灵敏度；SAF 导出导入；DTO v2 | 可与 M2 起并行（先用夹具） | 全部新界面有 Roborazzi 截图（浅色 / 深色 / 平板宽度） |
| **M10 内容与发布** | 《锈钟镇》开放世界版；demo / lighthouse 迁移；`packzip migrate`；Eval 用例与录音；文档（README、story-pack-format v3、mcp、local-models）；`0.2.0-alpha1` 构建与发布说明 | M1–M9 | 发布检查清单全部通过（第 18.4 节） |

建议顺序：M1 → M2 →（M3 ∥ M5）→（M4 ∥ M6 ∥ M8）→ M7 → M10；M9 从 M2 开始并行。

## 17.5 alpha2 任务

| 任务 | 说明 |
|---|---|
| T1 天之炽设定库 | 私有仓库，按 format 3 重写与扩充 |
| T2 背景时间线 | 原作剧情 → `pivotal` 事件、传闻、默认结局 |
| T3 预设与创建 | 西泽尔及其他预设主角、出身、`unique_roles` |
| T4 战斗内容 | 敌人 / 机甲部位与弱点、修正标签、`randomness: 25` 调平（私有平衡测试） |
| T5 alpha1 反馈修复 | 公开仓库 |
| T6 私有试玩 | 至少 3 条不同主角的 50 回合试玩记录；检查一致性审查与提示频率 |

---

# 18. 测试策略与风险

## 18.1 引擎（Go）

| 层 | 内容 |
|---|---|
| 单元测试 | 校验器表驱动测试（每条规则的接受 / 夹紧 / 拒绝）；影响分计算；知识揭示渠道检查；合理性检查表；程度判定边界；时间线状态机；导出导入往返；0.1.x 拒绝文案 |
| 属性 / 模糊测试 | 随机 WorldChange 序列：`apply` 后逐个 `revert` 回到初始状态；随机分支树：任一分支头部的 `StateAt` = 沿谱系线性重放；JSON 修复器模糊测试（任意截断 / 噪声输入不 panic，修复结果要么通过 schema 要么报错） |
| 确定性 | 录制会话的黄金重放（事件字节级一致）；分支盐：同分支重放一致、不同分支骰子不同；map 遍历顺序检查（沿用 V0.2 第 30 节） |
| 战斗模拟 | 扩展 `internal/combat/sim`：三档随机性下每场遭遇 200 个种子的胜率与剩余生命；自由战斗用意图脚本驱动 |
| 存储 | 进程中断注入：提交 A 后 / 提交 B 前崩溃 → 恢复为模板叙事 + `WorldUpdateFailed`；导入损坏文件不留残留 |
| 性能 | 2000 事件存档在中端手机上加载 < 300 ms；跨分支切换 < 500 ms；单回合引擎开销（不含 AI）< 50 ms |

## 18.2 AI（Eval，录音回放，不联网）

在 `tests/eval` 新增分类：

| 分类 | 用例示例 |
|---|---|
| `merged_output` | 正常 / 缺分隔符 / 截断 JSON / 带围栏 / 多余文字 → 解析或降级正确 |
| `world_change_validity` | 越档位数值被夹紧；同名实体被拒；退场人物复出被拒 |
| `knowledge_reveal` | NPC 说出自己不知道的事 → 拒绝；隐藏真相无前提 → 按规则处理 |
| `secret_leakage` | 叙事 / 审查输出 / 推荐卡中出现 `leak_markers` → Guard 处理 |
| `combat_adjudication` | “一拳打爆太阳”→ 拒绝或降级；“踢飞五倍体重的敌人”→ 推撞；未知弱点 → 碰运气 −1 |
| `impact` | 重要 NPC 死亡、势力毁灭、`pivotal` 事件取消 → 正确类型与分数 |
| `char_review` | 过强 / 过弱 / 重名 / 唯一身份冲突 |
| `audit` | 遗漏补记、幻觉不改世界、矛盾修复 |
| `refusal` | 各服务商内容审核错误响应 → 不写入 AI 内容 |

本地模型：GBNF 约束下的输出有效率测试（小模型，手动 / 夜间运行，不进 CI）。

## 18.3 Android

- JVM 单元测试：ViewModel（回滚横幅与取消、待决选择在进程重建后恢复、差异确认的 `preview_token` 失效）、`ProviderConfig` 与 Keystore 加解密、token 统计。
- Roborazzi 截图：全部新界面，夹具由 `cmd/uifixtures` 从真实引擎状态导出（《锈钟镇》），浅色 / 深色 / 平板宽度三套。
- 模拟器冒烟（x86_64）：SAF 导出导入、本地模型分配到任务、进程被杀后恢复待决选择。
- 私有包截图只用 `-Pibuki.fixtures.dir` 渲染到仓库之外。

## 18.4 发布检查清单（每个 alpha）

`make test vet lint eval` 通过；Roborazzi 截图人工过目；《锈钟镇》离线 / 本地 / 云端各完整试玩 30 回合（至少一次战斗、一次回滚、一次偏离提示、一次导出再导入）；存档导出文件密钥扫描为空；公开仓库版权检查（`git grep` 私有包专有名词为空）；README 与文档更新。

## 18.5 主要风险

| 风险 | 缓解 |
|---|---|
| AI 改世界过于激进，玩家体验失控 | 校验器上限 + 影响提示 + 单项撤销 + 分支回滚；默认灵敏度“中” |
| 合并输出让叙事质量下降（模型分心写 JSON） | WORLD 段放在正文之后；Eval 对比叙事长度与 Guard 修正率；必要时提供“拆成两次调用”的高级开关 |
| 知识层让叙事变得拘谨 / 频繁“未知” | 可揭示集合主动提供素材；公开字段范围由故事包控制；试玩调参 |
| 分支导致存档膨胀 | 事件小（JSON 行），快照只在分支头部；存档详情显示大小并可删分支 |
| 本地模型能力不足 | 档位限制 + 语法约束 + 适用性提示；关键任务默认不推荐本地 |
| 隐藏真相泄露 | 范围隔离 + 可揭示集合 + `leak_markers` Guard + Eval 泄露用例 |
| 工作量大 | 里程碑可独立验收；UI 与引擎并行；alpha2 只做内容 |

---

# 19. 待决问题（需要 lk 决定）

1. **隐藏真相的揭示门槛**：没有写 `reveal_when` 的隐藏真相，是否允许 AI 在渠道合理时自由揭示（本文默认：允许，但计入“严重影响故事”分数）？还是一律要求故事包写明前提条件才可揭示？
2. **偏离提示里“回到上一回合”后，被放弃的那段要不要保留为分支？** 本文默认保留并标记“已回滚”（可在时间线里切回去，可手动删除）；另一种是直接丢弃，时间线更干净。
3. **玩家能否覆盖故事包的随机性预设？** 本文默认不能（保证故事包作者的体验设计）；可选方案是开局时允许在预设基础上 ±1 档（叙事 / 平衡 / 硬核）。
4. **一致性审查的自动修复范围**：本文默认设定 / 文字类修复自动应用（可撤销），物品 / 金钱 / 经验 / 生死等机械状态只给建议、由玩家确认。是否全部自动，或全部需要确认？
5. **第三方旧格式（format 2）故事包**：本文默认 0.2.0 直接拒绝导入并提供 `packzip migrate` 转换工具；是否需要 App 内自动转换（质量较差但省事）？

---

# 附录 A：新增 / 删除事件一览

| 分类 | 新增 | 删除 |
|---|---|---|
| 世界 | `WorldChangeApplied` `WorldChangeReverted` `ImpactAssessed` `WorldUpdateFailed` | — |
| 时间线 | `WorldEventScheduled` `WorldEventStarted` `WorldEventStageAdvanced` `WorldEventJoined` `WorldEventDisrupted` `WorldEventResolved` `WorldEventCancelled` `RumorHeard` | `StoryStarted` 等局部事件类型保留 |
| 知识 | `KnowledgeRevealed` | `RelationRevealed` `MechRevealed` `CodexUnlocked`（并入知识揭示） |
| 决策 | `DecisionRequested` `DecisionResolved` | `DeviationChanged` `MainlineNudged` `MainlinePrompted` `MainlineModeChanged` `MainlineAnchorReached` `MainlineThresholdsSet` `MainlineNodeCanonized` `MainlineNodeCompleted` |
| 战斗 | `CombatIntentParsed` `ActionAdjudicated` `PartHit` | — |
| 分支 | `BranchCreated`（分支第一个事件，记录父分支、分叉点与随机盐） | — |

检查点、分支元数据、AI 调用记录、被拒绝的提案都是**元数据表**而不是游戏事件：它们不影响重放结果。

# 附录 B：新增 Command 类型

| Kind | 说明 |
|---|---|
| `world_change` | 统一写入网关提交的世界变更（`Source`、`Changes`、`ExpectedHead`、`PreviewToken`） |
| `reveal` | 仅内部使用：提交 B 中的知识揭示 |
| `decision` | 接受 / 回滚偏离提示 |
| `wait` | 时间跳跃 |
| `combat`（修改） | 载荷改为 `CombatIntent`；`Action` 枚举保留给快捷按钮 |
| `mainline` / `director` | 删除 |
