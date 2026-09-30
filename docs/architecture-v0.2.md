# AI RPG Engine Architecture V0.2

> AI 驱动、事件化、可扩展、可 Mod 的文字世界模拟 RPG 框架

**版本：** V0.2  
**状态：** Architecture Draft  
**核心目标：** Android 优先，Windows / Linux 方便开发与调试  
**核心语言：** Go  
**AI：** 云端模型优先，预留本地模型接口  
**产品定位：** 类 RimWorld 的世界模拟 + AI GM + 可替换世界包 / 剧情包 / Mod

---

# 1. V0.2 相比 V0.1 的主要变化

V0.2 新增并强化以下设计：

- `FreeformAction`：为自然语言长尾行为提供兜底。
- `ActionDefinition`：动作数据化，不让玩法依赖大量 Go Command。
- `Action Resolver`：合并 Intent Parsing 与 Reasonability。
- `Narrative Guard`：叙事必须和确定性事件一致。
- `Scene Fact / Scene State`：当前场景短期细节缓存。
- `Observation / Witness System`：事件通过“观察”进入 NPC 信念。
- `Deferred Simulation Materialization`：低精度世界在被观察时再具体化。
- `Deterministic RNG Streams`：随机数按命名空间和用途分流。
- `LLM Eval`：为意图解析、合理性和叙事一致性建立回归测试。
- `Pacing System`：Director 不只推动剧情，还控制节奏。
- MCP 从核心概念降为 `Command / Query API` 上的一层适配器。
- Phase 顺序改为先做 Vertical Slice 验证体验，再正式收敛 Core。

---

# 2. 项目愿景

目标不是制作“AI 聊天 RPG”，而是制作一个真正存在、持续变化、可被 AI 理解和叙述的游戏世界。

系统应满足：

1. 玩家主要通过自然语言表达意图。
2. AI 是 GM、导演、叙述者和角色扮演者。
3. AI 不直接拥有游戏事实。
4. 数值、规则、随机数、状态变更由 Core 决定。
5. 世界即使没有被玩家观察，也可以持续演化。
6. 演化颗粒度随距离、重要性和剧情关联动态变化。
7. NPC 拥有独立信念、记忆、目标和关系。
8. 世界包、剧情包、规则包和 Mod 统一走 Package 机制。
9. 小型跑团、十万字世界包、小说改编包都可以适配。
10. Core 与 UI、AI Provider、MCP、具体世界内容解耦。

---

# 3. 架构宪法

## 3.1 LLM 不是真相源

LLM 可以：

- 理解玩家输入；
- 匹配 Action；
- 提出 FreeformAction；
- 建议技能检定和难度；
- 扮演 NPC；
- 提议世界事件；
- 提议 Dynamic Canon；
- 生成叙事；
- 做 Story / Pacing 规划。

LLM 不可以直接：

- 修改 HP；
- 修改物品；
- 修改金币；
- 判定死亡；
- 推进任务；
- 修改世界时间；
- 新增永久角色；
- 创建永久地点；
- 修改关系数值；
- 写入世界事实。

所有永久改变都必须：

```text
LLM / UI / Script / MCP
        ↓
Command / Proposal
        ↓
Validator
        ↓
Game Core
        ↓
Event
        ↓
State
```

---

## 3.2 所有永久状态变化必须事件化

示例：

```text
DamageTaken
CharacterDied
ItemAdded
RelationshipChanged
QuestAdvanced
LocationChanged
TimeAdvanced
FactCreated
CharacterCreated
FactionRelationChanged
EffectApplied
ObservedEventCreated
BeliefUpdated
SceneFactPromoted
```

Event Log 原则：

- Append-only；
- 可重放；
- 可审计；
- 可调试；
- 可做存档迁移；
- 可做回滚；
- 可做 NPC Memory 输入源。

---

## 3.3 内容必须优先数据化

Core 尽量只实现机制。

具体：

- 动作；
- 角色；
- 地点；
- 物品；
- 技能；
- 状态；
- 故事节点；
- 世界规则；
- Prompt；
- Lore；

应尽量由 Package 提供。

---

## 3.4 AI 必须可替换、可失败、可降级

Provider 故障不允许破坏 Game State。

```text
Preferred Provider
   ↓ fail
Secondary Provider
   ↓ fail
Local Provider
   ↓ fail
Template / Rule Fallback
```

降级时：

- 不重新掷骰；
- 不重复执行 Command；
- 不改变已确定事件；
- 只降低智能解析或叙事质量。

---

# 4. 总体架构

```text
┌─────────────────────────────────────────┐
│                   UI                    │
│ Android / Desktop Dev Client            │
└───────────────────┬─────────────────────┘
                    │
               Player Input
                    │
       ┌────────────▼────────────┐
       │     Action Resolver     │
       │ Intent + Reasonability  │
       │ Action Matching         │
       └────────────┬────────────┘
                    │
          Action / FreeformAction
                    │
       ┌────────────▼────────────┐
       │       Validator         │
       └────────────┬────────────┘
                    │
                  Command
                    │
       ┌────────────▼────────────┐
       │        Game Core        │
       │ Rules / Time / RNG      │
       │ Character / Inventory   │
       │ Story / Simulation      │
       │ Perception / Canon      │
       └────────────┬────────────┘
                    │
                  Events
                    │
       ┌────────────▼────────────┐
       │        Event Bus        │
       └───────┬────────┬────────┘
               │        │
            State    Memory/Belief
               │        │
               └───┬────┘
                   │
            Context Builder
                   │
       ┌───────────▼────────────┐
       │      Narrator Agent    │
       └───────────┬────────────┘
                   │
             Narrative Draft
                   │
       ┌───────────▼────────────┐
       │     Narrative Guard    │
       └───────────┬────────────┘
                   │
                 Player
```

旁路系统：

```text
Package Manager
Mod Manager
Story Director
Pacing System
Lore Retrieval
AI Provider Manager
Prompt Manager
Save Manager
Snapshot Manager
Script Runtime
Command/Query API
MCP Adapter
Diagnostics
LLM Eval Runner
```

---

# 5. Action 模型

V0.2 将“玩家能做什么”从固定 Go Command 中抽离。

## 5.1 ActionDefinition

Package 可以声明动作。

示例：

```yaml
id: core:action/intimidate
category: social

requirements:
  - actor.can_speak == true
  - target.type == "character"

checks:
  - skill: intimidation
    difficulty: target.resolve

cost:
  time: 30s

outcomes:
  success:
    - effect:
        type: relationship_delta
        target: target
        values:
          fear: 5
  failure:
    - effect:
        type: relationship_delta
        target: target
        values:
          trust: -3
```

ActionDefinition 是内容。

Command 是执行协议。

---

## 5.2 Core Command 应保持少量

建议核心 Command：

```text
ExecuteActionCommand
AdvanceTimeCommand
MoveCommand
InventoryCommand
StoryTransitionCommand
CanonProposalCommand
SystemCommand
```

大多数玩法通过：

```text
ExecuteActionCommand(action_id, args)
```

进入规则引擎。

---

# 6. FreeformAction

自然语言永远存在长尾：

- 唱歌；
- 调情；
- 观察壁画；
- 故意踢翻桌子；
- 学猫叫；
- 往杯子里丢石子；
- 用奇怪方式试探 NPC。

如果不能匹配已声明 Action，则进入：

```text
FreeformAction
```

建议结构：

```json
{
  "actor": "player",
  "description": "踢翻桌子吸引守卫注意",
  "targets": ["table.01", "guard.01"],
  "suggested_check": {
    "skill": "deception",
    "difficulty": 8
  },
  "estimated_time": 5,
  "tags": ["distraction", "noise"],
  "proposed_effects": [
    "attention_shift",
    "noise_generated"
  ]
}
```

AI 的技能、难度和效果都只是建议。

Core 必须校验。

---

## 6.1 FreeformAction 允许的结果

第一阶段只允许产生白名单轻量事件：

```text
SkillCheckResolved
TimeAdvanced
NoiseGenerated
AttentionChanged
SceneFactChanged
RelationshipNudge
MinorConditionApplied
ObservationTriggered
NoMechanicalEffect
```

禁止通过 FreeformAction 直接：

```text
CharacterDied
FactionDestroyed
QuestCompleted
LegendaryItemCreated
MassiveStatRewrite
```

重要改变必须转入正式 Action 或专门规则。

原则：

> 已声明 Action 负责强规则行为，FreeformAction 负责自然语言长尾。

---

# 7. Action Resolver

V0.2 合并：

```text
Intent Parser
+
Reasonability Agent
```

为单次结构化调用。

输入：

- 玩家文本；
- 当前可用 Action 目录；
- 当前场景摘要；
- PlayerScope；
- 必要角色和世界事实。

输出：

```json
{
  "intent": "distraction",
  "matched_action": null,
  "freeform": true,
  "reasonability": "ALLOW_WITH_CHECK",
  "targets": ["table.01", "guard.01"],
  "arguments": {},
  "suggested_check": {
    "skill": "deception",
    "difficulty": 8
  },
  "confidence": 0.91
}
```

---

# 8. 合理性系统

输出仅允许：

```text
ALLOW
ALLOW_WITH_CHECK
ALLOW_WITH_CONSEQUENCE
REJECT
```

判断维度：

```text
World Constraint
Capability
Physics
Knowledge
Story Constraint
Game Rule
Target Availability
Resource Availability
```

原则：

> 判断“能不能尝试”，而不是判断“玩家说的话是不是真的”。

例如：

> 我告诉守卫我是国王派来的。

这是：

```text
Deception Attempt
```

不是 Knowledge Violation。

---

# 9. 快速通道

明确 UI 操作和明确命令不应强制经过 LLM。

例如：

```text
[攻击]
[休息]
[查看背包]
[前往酒馆]
```

可以直接构造：

```text
ExecuteActionCommand
```

关键路径：

```text
UI Action
→ Core
→ Narrator
```

而非：

```text
UI Action
→ Resolver LLM
→ Core
→ Narrator
```

---

# 10. 延迟与成本目标

第一阶段建议记录：

```text
P50 首字延迟 < 3 秒
P95 首字延迟 < 6 秒
```

这是目标，不是硬 SLA。

优化策略：

- Intent + Reasonability 合并；
- Narrator 流式输出；
- 小模型做结构化解析；
- 大模型做叙事；
- UI 快速通道；
- Context 精简；
- 利用 Provider Prompt Cache / Prefix Cache；
- 稳定 Prompt 段放在前部；
- 非关键 Agent 延迟执行或批处理。

---

# 11. Narrative Guard

Narrator 不能只靠 Prompt 保持一致。

流程：

```text
Events
  ↓
Immutable Facts
  ↓
Narrator
  ↓
Draft
  ↓
Narrative Guard
  ↓
Final Narrative
```

Narrator 输入中明确包含：

```json
{
  "immutable_facts": {
    "guard_alive": true,
    "damage": 18,
    "player_location": "tavern",
    "item_changes": []
  }
}
```

Guard 检查：

- 死亡状态；
- 伤害；
- 物品变化；
- 地点；
- 时间；
- 关键任务状态；
- NPC 身份；
- 不应泄露的信息。

策略：

```text
Rule Check
→ Lightweight Structured Claim Check
→ Optional Rewrite
→ Template Fallback
```

---

# 12. Canon 与 Scene State

V0.2 采用四层事实。

## 12.1 Static Canon

Package 明确声明。

---

## 12.2 Dynamic Canon

游戏过程中永久产生。

---

## 12.3 Scene Fact

当前场景短期存在。

例如：

```text
桌上有一道裂痕
窗户开着
地上洒了酒
柜台旁有两个陌生商人
```

存放于：

```text
SceneState
```

离开场景后默认可丢弃。

如果玩家与其交互：

```text
SceneFact
↓
Interaction
↓
CanonProposal
↓
Dynamic Canon
```

---

## 12.4 Ephemeral Prose

纯修辞细节。

不进入 Scene State。

---

# 13. Character System

角色采用：

```text
Structured Fields
+
Natural Language Description
```

示例：

```yaml
id: demo:character/lena

identity:
  name: Lena
  age: 24

attributes:
  strength: 7
  agility: 12
  intelligence: 14

skills:
  sword: 8
  persuasion: 10

personality:
  description: >
    谨慎、现实，但重视承诺。

goals:
  - protect_family

traits:
  - cautious
  - loyal

relationships: {}
beliefs: {}
memories: []
secrets: []
inventory: []
tags: []
```

---

# 14. 客观事实与 NPC 信念分离

世界可能是：

```text
king.dead = true
```

NPC 可能相信：

```text
belief.king_dead = false
```

这是：

- 谎言；
- 谣言；
- 欺骗；
- 情报；
- 秘密；
- 误会；
- 调查；

的基础。

---

# 15. Observation / Witness System

NPC 不允许直接消费所有世界 Event。

标准链路：

```text
Objective Event
   ↓
Perception Resolution
   ↓
ObservedEvent
   ↓
Interpretation
   ↓
Belief Update
```

Event 可以携带：

```json
{
  "visibility": {
    "mode": "local",
    "radius": 12,
    "requires_line_of_sight": true,
    "audible_radius": 20
  }
}
```

Perception System 根据：

- 距离；
- 视线；
- 听觉；
- 隐身；
- 环境；
- 注意力；
- 技能；
- 特殊能力；

确定目击者。

---

# 16. 信息传播

观察不是唯一来源。

NPC 信念还可以来自：

```text
Witness
Dialogue
Rumor
Letter
Media
Faction Report
Memory
Inference
```

建议 Belief 记录：

```yaml
fact: king_is_dead
value: true
confidence: 0.7
source: rumor
source_entity: npc.merchant.01
world_time: 124000
```

---

# 17. Context Permission

不同 Agent 拥有不同 Scope。

```text
PlayerScope
NarratorScope
DirectorScope
NPCScope
SimulatorScope
ReasonabilityScope
```

关键调整：

> 默认 Narrator 不拥有全知权限。

Narrator 默认读取：

```text
Player Observable Facts
Player Knowledge
Current Scene
Allowed Foreshadowing
Resolved Events
```

Director 可以拥有更广的全局视角。

NPC Agent 只能读取：

```text
NPC Beliefs
NPC Memories
Observed Scene
Public / Known Facts
```

---

# 18. 世界时间

采用：

```text
Continuous World Time
+
Player Interaction Turns
```

时间推进由 Core 决定。

AI 可以建议：

```text
estimated_duration
```

Core 最终裁决。

---

# 19. Simulation Level

## L0 — Frozen

只保存数据。

## L1 — Abstract

统计演化。

## L2 — Simulated

事件级演化。

## L3 — Active

玩家附近和直接交互对象，高精度模拟。

---

# 20. Simulation Priority

等级动态计算：

```text
Distance
Story Relevance
Player Relationship
Recent Interaction
Scheduled Event
Faction Importance
Narrative Importance
```

---

# 21. Deferred Simulation Materialization

低精度区域不要求后台持续模拟每个 NPC。

例如一个村庄三个月处于 L1：

```text
population -8%
food -20%
prosperity -12%
bandit_pressure +30%
```

玩家再次抵达时：

```text
Abstract State
+
Elapsed Time
+
Region Seed
+
Relevant Story State
↓
Materialization
↓
Concrete Events / Current State
```

例如具体化为：

```text
3 户家庭离开
部分农田荒废
酒馆换了老板
治安恶化
```

同时可以生成：

```text
WhileAwaySummary
```

---

# 22. 世界事件生成

推荐：

```text
Rules decide WHEN / TYPE
AI decides DETAIL
Core validates RESULT
```

规则产生：

```json
{
  "type": "faction_conflict",
  "faction_a": "north_empire",
  "faction_b": "silver_clan",
  "severity": 0.65
}
```

AI 补全：

```text
银狼氏族袭击北境第三粮仓。
```

若成为永久事实：

```text
CanonProposal
→ Validation
→ Event
```

---

# 23. Story Graph

不采用单纯线性章节。

Story Node：

```yaml
id: demo:story/king_assassination

priority: major

requirements:
  - world.day >= 10

conditions:
  - king.alive == true

constraint: soft

outcomes:
  - assassination_success
  - assassination_failed
  - assassination_prevented
```

约束类型：

```text
FREE
SOFT
HARD
```

---

# 24. Timeline

世界事件不等待玩家。

```text
Day 12  北境军抵达
Day 14  城门受到攻击
Day 15  国王撤离
```

Timeline 触发：

```text
Scheduled Event
→ Simulation
→ Events
→ State
```

---

# 25. Pacing System

V0.2 增加 Pacing。

Director 不只判断“应该发生什么”，还控制：

```text
事件密度
信息释放
压力
奖励
休息窗口
剧情钩子
危机升级
```

示例状态：

```yaml
tension: 0.65
danger: 0.40
mystery: 0.80
player_progress: 0.35
quiet_turns: 2
recent_combat: true
```

剧情包可以提供：

```text
PacingProfile
```

例如：

```text
slow_burn
high_pressure
romance
mystery
survival
sandbox
```

Pacing 只能影响提议和事件选择，不允许绕过 Game Core 强制写状态。

---

# 26. Rules Engine

Core 提供通用原语：

```text
Stat
Resource
Skill
Trait
Modifier
Effect
Tag
Item
Action
Relationship
Event
Check
```

内容由 Package 定义。

---

# 27. 表达式系统

推荐优先考虑：

```text
CEL
```

用于：

- Story 条件；
- Action requirements；
- Mod predicates；
- Rule predicates；
- 权限条件；
- 简单数值表达式。

复杂脚本后续交给：

```text
Lua / WASM Sandbox
```

表达式不应演化成无限制脚本语言。

---

# 28. 数值确定性

关键游戏数值优先：

```text
Integer
Fixed Point
```

避免依赖浮点导致跨平台结果漂移。

示例：

```text
1000 = 1.000
```

---

# 29. RNG

RNG 必须由 Core 管理。

不使用单一 Global RNG。

推荐命名流：

```text
combat
loot
world
npc
story
mod:<namespace>
```

更进一步：

```text
RNG(namespace, entity, purpose)
```

例如：

```text
rng("combat", battle_id, "hit")
rng("combat", battle_id, "damage")
rng("world", region_id, "weather")
```

目标：

> 安装一个无关 Mod 不应让所有后续随机结果整体漂移。

---

# 30. Deterministic Replay

影响规则结果的集合必须显式排序。

禁止依赖 Go map 遍历顺序。

规则：

```text
extract keys
→ sort
→ iterate
```

Replay 至少依赖：

```text
Initial State
Package Versions
Commands
Events
RNG Streams
Engine Version
```

AI 文本不保证字面复现，但游戏事实应可复现。

---

# 31. Package System

统一目录：

```text
package/
├ manifest.yaml
├ world/
├ story/
├ characters/
├ actions/
├ items/
├ rules/
├ effects/
├ prompts/
├ lore/
├ assets/
├ scripts/
└ migrations/
```

---

# 32. Namespace

所有 ID 强制命名空间化：

```text
core:item/iron_sword
demo:character/lena
magic.mod:effect/black_blood
```

---

# 33. Package 类型

统一 Loader：

```text
core
world
story
mod
expansion
ruleset
content
```

---

# 34. Mod Patch

支持 Patch 而不是整对象覆盖。

例如：

```yaml
target: core:item/iron_sword

patch:
  damage:
    add: 2
```

Prompt 也使用 Section Patch。

---

# 35. Prompt System

建议 Section：

```text
[CORE_RULES]
[WORLD_RULES]
[STORY_CONSTRAINTS]
[CHARACTER_BEHAVIOR]
[STYLE]
[OUTPUT_SCHEMA]
```

稳定段落优先放前部，方便 Provider 使用 Prompt / Prefix Cache。

---

# 36. AI Provider Layer

概念接口：

```go
type Provider interface {
    Generate(ctx context.Context, req Request) (Response, error)
}
```

Provider Manager 负责：

```text
routing
timeout
retry
fallback
rate limit
token accounting
capability
structured output
streaming
cache hints
```

---

# 37. Multi-Agent

逻辑角色：

```text
Action Resolver
Story Director
World Simulator
NPC Agent
Narrator
Memory Agent
Canon Agent
```

但：

> 逻辑多 Agent ≠ 每回合多个昂贵串行调用。

同一模型可以承担不同 Role。

---

# 38. Critical Path

自然语言输入推荐：

```text
Player
↓
Action Resolver
↓
Core
↓
Essential Simulation
↓
Narrator Streaming
↓
Narrative Guard
↓
Player
```

非关键任务：

```text
Memory Compression
Remote Simulation
NPC Long-term Planning
Summary Update
Index Update
```

应批处理、延后或并行。

---

# 39. LLM Eval

建立：

```text
/tests/eval/
```

建议分类：

```text
intent/
reasonability/
action_matching/
freeform_action/
tool_selection/
narrative_consistency/
secret_leakage/
npc_belief/
```

示例：

```yaml
input: "我假装自己是国王派来的使者"

expected:
  action: deceive
  reject: false
  target: guard

forbidden:
  - knowledge_violation
```

每次：

- 更换模型；
- 更新 Prompt；
- 修改 Context Builder；
- 修改 Action 目录；

都运行回归。

---

# 40. Recorded LLM Transport

测试环境支持：

```text
Request Hash
→ Recorded Response
```

好处：

- CI 不依赖真实 API；
- Prompt regression 可稳定复现；
- 无网络也能跑大部分自动化测试；
- 降低测试成本。

---

# 41. Lore Retrieval

不将十万字世界包整包塞入上下文。

Context 由：

```text
Current Scene
Recent Turns
Relevant Characters
Relevant Actions
Relevant Story Nodes
Relevant Lore
Relevant Memories
Player Knowledge
Resolved Events
```

组合。

可逐步支持：

```text
Full-text Search
Embedding Search
Hybrid Retrieval
```

Retrieval 不拥有游戏状态。

---

# 42. Storage

默认候选：

```text
SQLite
```

Go 驱动优先评估：

```text
modernc.org/sqlite
```

但领域层只依赖 Storage Interface。

---

# 43. Event Store + Snapshot

Event：

```text
Append-only
```

状态恢复：

```text
Snapshot
+
Events After Snapshot
```

---

# 44. Save

存档必须记录：

```yaml
engine_version: 0.2.0

packages:
  - id: core
    version: 0.2.0
  - id: demo.world
    version: 0.1.0
```

---

# 45. Missing Package

缺失对象不得直接坏档。

转成：

```text
MissingObject
```

保留：

```text
original_id
original_package
serialized_data
```

重新安装后尝试恢复。

---

# 46. Migration

Package 提供：

```text
migrations/
```

迁移必须版本化。

---

# 47. Command / Query API

Core 对外暴露统一 API：

```text
Command API
Query API
```

所有外部入口都走这里：

```text
UI
Internal Agent Tool
Debug Console
Script Runtime
MCP Adapter
Future Network Layer
```

---

# 48. MCP 的定位

MCP 不是 Domain。

它只是：

```text
Command / Query API
        ↑
     Adapter
        ↑
       MCP
```

因此 Core 不依赖 MCP。

---

# 49. Mobile Boundary

Go Core 与 Android UI 之间保持窄接口。

推荐早期：

```text
Versioned JSON DTO
```

例如：

```text
ActionRequestV1
ActionResponseV1
QueryRequestV1
QueryResponseV1
```

可以通过 gomobile bind 或其他桥接暴露。

UI 可选：

```text
Kotlin / Compose
Flutter
```

不影响 Core。

---

# 50. 建议 Go 模块

```text
/internal

  core/
    engine
    command
    event
    state
    transaction

  action/
    definition
    resolver
    freeform
    validator

  rules/
    stats
    checks
    expression
    rng
    fixedpoint

  world/
    time
    simulation
    materialization
    location
    faction
    canon
    scene

  perception/
    visibility
    witness
    observation

  character/
    character
    belief
    memory
    relationship

  combat/
  inventory/

  story/
    graph
    director
    timeline
    pacing

  agent/
    orchestrator
    narrator
    npc
    memory
    simulator
    canon

  ai/
    provider
    router
    fallback
    context
    transport

  narrative/
    guard
    claims
    fallback

  package/
    loader
    manifest
    dependency
    patch
    migration

  mod/
    runtime
    sandbox

  api/
    command
    query
    dto

  adapter/
    mcp
    mobile
    debug

  storage/
    sqlite
    eventstore
    snapshot
    save

  retrieval/
    lore
    index

  prompt/
    builder
    sections
    patch
```

---

# 51. Transaction

一次行为产生多个 Event 时：

```text
全部成功
或
全部失败
```

例如交易：

```text
GoldRemoved
ItemRemovedFromMerchant
ItemAddedToPlayer
```

不能出现部分执行。

---

# 52. Idempotency

Command 必须有：

```text
command_id
```

同一 command_id 不得重复执行。

防止：

```text
超时
→ 重试
→ 重复扣钱
```

---

# 53. Diagnostics

开发模式建议：

```text
State Inspector
Event Timeline
Prompt Inspector
Context Inspector
Agent Call Log
Tool Call Log
Observation Inspector
Belief Inspector
Simulation Inspector
Materialization Log
Package Dependency Graph
RNG Log
Narrative Guard Report
```

---

# 54. Phase 0 — Vertical Slice

先验证最大未知数：是否好玩。

范围：

```text
1 个酒馆
3 个 NPC
1 个小事件
少量 Action
FreeformAction
基础掷骰
基础 Scene Fact
```

内容从第一天放在 YAML / Markdown 中。

Core 可极简。

必须跑通：

```text
输入
→ Resolver
→ 判定
→ Event
→ Narrator
→ Guard
→ 输出
```

收集：

```text
Intent Accuracy
Reasonability Accuracy
Action Match Rate
Freeform Success Rate
Narrative Consistency
Secret Leakage
P50 / P95 Latency
Token Cost / Turn
Fun / Agency Feedback
```

---

# 55. Phase 1 — Formal Core

实现：

```text
Command
Event
State
Transaction
SQLite
Snapshot
Time
RNG Streams
ActionDefinition
Package Loader
```

同时做 Android 冒烟测试：

```text
Go Core
SQLite
Mobile Bridge
```

---

# 56. Phase 2 — Production AI Pipeline

实现：

```text
Provider Manager
Streaming
Fallback
Fast Path
Recorded Transport
LLM Eval
Narrative Guard
```

---

# 57. Phase 3 — NPC Cognition

实现：

```text
Observation
Witness
Belief
Memory
Relationship
Rumor
Information Propagation
```

---

# 58. Phase 4 — Story + Simulation

实现：

```text
Story Graph
Timeline
Director
Pacing
L0-L3 Simulation
Deferred Materialization
```

---

# 59. Phase 5 — Mod Capability

实现：

```text
Dependency
Patch
Migration
Expression
Namespace
Sandbox Extension
```

---

# 60. Phase 6 — Android Productization

完善：

```text
Compose / Flutter UI
Streaming UX
Save Management
Package Manager
Mod Manager
Debug Tools
Performance
```

---

# 61. MVP 成功标准

一个小世界中，玩家可以：

```text
自由输入任何合理行为
→ 系统能匹配 Action 或 Freeform
→ Core 产生可靠结果
→ 世界状态持续变化
→ NPC 只知道自己应该知道的事
→ AI 叙事不篡改结果
→ 离开和返回区域时世界有合理变化
→ 存档和恢复可靠
```

---

# 62. V0.2 架构底线

每次新增功能都检查：

## 问题一

> 如果暂时移除所有 AI Provider，世界事实、数值和存档是否仍然完整？

如果不是，说明 AI 获得了过多状态权力。

## 问题二

> 第三方世界包能否仅通过公开数据格式和接口实现它，而无需修改 Core？

如果不能，说明 Package 边界不足。

## 问题三

> 一个玩家没有被预定义过的合理行为，系统能否给出自然反馈，而不是“我听不懂”？

如果不能，说明 FreeformAction 体系不足。

## 问题四

> NPC 为什么知道某件事，能否从 Event → Observation → Belief 路径解释？

如果不能，说明认知模型失控。

## 问题五

> Narrator 写出的每个关键事实，是否可以追溯到 PlayerScope 或本轮 Events？

如果不能，存在泄密或幻觉风险。

最终目标：

> **世界真实存在于确定性的游戏状态中；AI 负责理解世界、参与世界、导演体验并把世界讲给玩家。**
