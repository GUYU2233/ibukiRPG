# ibukiRPG

> AI 驱动、事件化、可扩展的中文文字冒险 RPG。**用你自己的话行动，骰子与规则决定结果。**

当前版本：**v0.2.0-alpha1（开放世界引擎，预发布）** —— 三个内置原创故事包（均为新的故事包格式 3）：数值 RPG 示例《锈钟镇 · 黄铜试炼》（开放世界样板：世界事件时间线、隐藏真相、势力、角色创建、自由战斗）、《边境酒馆 · 失窃的钱袋》和小短篇《雾港灯塔 · 守灯人的信》；支持导入第三方故事包（.zip，格式见 [docs/story-pack-format.md](docs/story-pack-format.md)）。更新记录见 [CHANGELOG.md](CHANGELOG.md)，架构见 [docs/architecture-v0.3.md](docs/architecture-v0.3.md)。

> **不兼容变更**：v0.2 只接受格式 3 的故事包，旧的格式 2 故事包导入时会被拒绝（没有迁移工具）；0.1.x 的存档无法读取，会提示重新开始。

v0.2.0-alpha1 新增：
- **开放世界**：主线贴合度与偏离弹窗被**世界事件时间线**取代——世界按自己的时间表推进（事件有前置条件、截止时间与后果），玩家去不去都会发生；`/wait` 可以等待或跳到某个事件。
- **AI 修改世界**：叙事与世界更新合并为一次结构化输出；AI 提出的设定 / 状态变更经过**校验器**（引用、强度、权限、单回合上限）后写入事件日志，可在世界日志里撤销。偏离设定、重要角色死亡、严重剧情影响会弹出**决策底部抽屉**（接受 / 回到上一回合 / 以后只通知），每种提示的灵敏度可调。JSON 写坏时修复 / 重试一次，仍失败则只保留叙事；内容拒绝会被识别并处理。
- **玩家知识层**：世界面板里每个字段单独解锁（一切从“未知”开始），叙事流里显示解锁小标签；没有 `reveal_when` 的隐藏真相可以被 AI 自由揭示（严重影响剧情时触发提示）。
- **自由战斗裁定**：战斗中用自己的话描述动作 → 解析 → 合理性检查（荒谬动作降级）→ 修正值 → 掷骰 → 成功度 → 状态；随机度预设由故事包决定，玩家不能修改。
- **回溯与分支**：回到任意回合（旧分支保留）、切换分支、检查点（重要决定前自动创建，可关闭；也可手动创建）。
- **存档导出 / 导入**（`.ibksave`，不含 API Key，带引擎与故事包版本）。
- **API 设置 / 生成设置**：多个服务商（DeepSeek、通义千问、自定义 OpenAI 兼容、本地 llama.cpp），Key 加密保存；统一模型或按任务分配模型，本地模型可作为子 Agent（ⓘ 提示哪些任务适合本地模型，见 [docs/local-models.md](docs/local-models.md)）；每回合 token 用量。
- **世界变更日志与一致性审查**：相关变更强制注入上下文；定期自检，设定 / 文字类问题自动修正（可撤销），物品 / 金钱 / 经验 / 生死只给建议、由玩家确认。
- **角色创建与卡片**：选择预设主角或自建角色（出身 + 属性点 + 外貌 / 性格 / 来历），审查 Agent 检查设定契合度、强度与冲突并给出建议和推荐角色卡；在世界面板里“让 AI 修改”或新建角色 / 物品 / 设定卡——玩家要求的修改先显示差异预览，确认后写入。
- **MCP 写入通道**（`--allow-write`，默认关闭）：外部工具的修改走同一个校验器和事件日志，带存档槽锁。
- **新界面**：叙事流 + 状态条、8 个标签的世界面板（手机底部抽屉 / 平板侧栏）、战斗头部与裁定卡片、时间线 / 分支导航、决策底部抽屉、审查卡片。

更早的版本（v0.1.x：数值 RPG 战斗与机甲、图鉴、关系网、AI 记忆与只读检索、llama.cpp 本地模型、发布签名与 16 KB 对齐）见 [CHANGELOG.md](CHANGELOG.md)。

- 引擎：Go，事件溯源（Command → Event → State），SQLite 存档，确定性骰子（可重放）。
- 客户端：原生 Android（Kotlin + Jetpack Compose + Material 3），以及用于桌面调试的命令行客户端。
- AI：**默认规则离线模式**（无需联网、无需 API Key）；可选在手机上离线运行 GGUF 本地模型（llama.cpp），或接入 DeepSeek、通义千问等 OpenAI 兼容服务。**LLM 永远不是真相源**：检定、金币、物品、关系都由本地引擎决定。

## 怎么玩

1. 安装 APK（见 [Releases](https://github.com/GUYU2233/ibukiRPG/releases)），打开后点 **新游戏**，选择一个故事包，选择预设主角或自己创建角色。
2. 在底部输入框里直接写你想做的事，例如：
   - “环顾四周”“和老板打个招呼”“来一杯麦酒”
   - “仔细搜查奥托的座位附近”“说服伯林让我去储藏室看看”“威胁奥托闭嘴”
   - “我跳上桌子唱一首家乡的歌”（自由行动也会产生后果）
3. 不想打字？点输入框上方的**快捷行动**（环顾四周 / 与某人交谈 / 点酒 / 前往…）。
4. 需要检定时会显示一张**检定卡片**：`d20 掷出 17 + 说服 4 = 21（需要 ≥ 15）→ 成功`，结果一目了然。
5. 右上角按钮打开**世界面板**：角色 / 关系网 / 图鉴 / 装备 / 地图 / 势力 / 时间线 / 日志——只显示你已经知道的内容，其余是“未知”。
6. 走错了？打开**时间线**回到任意回合（原来的分支会保留），或恢复检查点。
7. 每一回合都会自动保存。回到首页点 **继续游戏** 即可从上次的位置继续；**存档** 页可以复制、重命名、删除、导出 / 导入存档。

小提示：酒馆里过一会儿会出事。仔细调查、取得老板的信任、弄清真相——结局有好几种。

## 离线模式与 AI 模式

| | 离线规则模式（默认） | 离线本地模型（llama.cpp · GGUF） | 在线 AI 模式 |
|---|---|---|---|
| 需要网络 / Key | 否 | 否 | 是（DeepSeek / 通义千问 / 自定义 OpenAI 兼容） |
| 输入理解 | 关键词 + 规则解析 | 设备本地模型结构化解析 | 在线大模型结构化解析，失败自动退回离线规则 |
| 叙事 | 模板叙事 | 设备本地模型，经 **Narrative Guard** 校验 | 在线大模型，经 **Narrative Guard** 校验 |
| 世界更新 / 审查 / 卡片生成 | 不做（规则引擎照常推进时间线） | 不建议（可按任务分配，校验更严格） | 合并输出 + 校验器 |
| 规则 / 骰子 / 存档 | 本地引擎 | 本地引擎（完全相同） | 本地引擎（完全相同） |

在 Android **设置 → API 设置** 中添加服务商（可以添加多个）并填写 API Key，在 **生成设置** 中选择“统一模型”或按任务分配模型。Key 使用 Android Keystore（AES-GCM）加密后保存在本机，不写入存档、不进入导出文件、不上传。超时或出错时不会重复执行行动，也不会重新掷骰。

选择 **离线本地模型（llama.cpp · GGUF）** 后，导入任意 GGUF 模型文件（推荐 **Qwen2.5-1.5B-Instruct Q4_K_M**，约 1.1 GB，4 GB 以上内存的手机；或 3B Q4_K_M，约 2 GB，6～8 GB 内存），可调整上下文长度、CPU 线程、温度、Top-P / Top-K、单次最多生成 token 数。模型复制到 App 私有目录，只在本机推理；本地模型不使用函数调用，改用关键词预取故事资料。模型推荐、内存需求与后台运行说明见 [docs/local-models.md](docs/local-models.md)。

预设：

- DeepSeek：`https://api.deepseek.com`，模型 `deepseek-chat`
- 通义千问（DashScope 兼容模式）：`https://dashscope.aliyuncs.com/compatible-mode/v1`，模型 `qwen-plus`

## 桌面命令行客户端（Linux / Windows）

命令行客户端和 App 调用同一套 JSON API，适合调试和快速试玩：

```bash
make build
./build/bin/ibukirpg                 # 离线模式；自动继续最近的存档
./build/bin/ibukirpg -new -name 阿澈  # 新游戏
DEEPSEEK_API_KEY=sk-... ./build/bin/ibukirpg -ai deepseek
DASHSCOPE_API_KEY=sk-... ./build/bin/ibukirpg -ai qwen
IBUKI_AI_KEY=sk-... ./build/bin/ibukirpg -ai custom -base-url https://example.com/v1 -model my-model
```

输入数字执行对应的快捷建议；常用命令：`/s` 场景、`/me` 角色、`/inv` 背包、`/npc` 人物、`/log` 日志、`/saves` 存档、`/load N`、`/new`、`/ai` AI 状态、`/help`、`/quit`。存档默认位于用户配置目录（`-data` 可指定）。

数值 RPG 命令：`/fight` 可发起的战斗、`/attack` `/skill 技能 目标` `/item 道具` `/defend` `/flee` `/mech` `/eject`、`/grow` 成长、`/mechs` `/mechcard` 机甲卡、`/codex` 图鉴、`/rel` 关系网、`/cards` 角色卡。战斗中也可以直接用自己的话描述动作（自由战斗裁定）。

开放世界命令：`/world [标签]` 世界面板、`/wait 1h|dawn|event:<ID>` 等待、`/changes` 世界变更日志、`/revert <变更ID>`、`/decision accept|rollback|dismiss`、`/timeline`、`/rollback <回合>`、`/branch <分支ID>`、`/cp [名称]` 检查点、`/restore <检查点ID>`、`/export` / `/importsave` 存档导出导入、`/usage` token 用量、`/audit` 一致性检查、`/suggest apply|ignore <ID>`、`/edit <实体ID|new> <要求>` + `/confirm` 让 AI 修改卡片。

MCP 服务：`./build/bin/ibukirpg mcp --save <存档目录或 .db>`（默认只读；`--allow-write --scope director` 开放经过校验的世界写入工具），详见 [docs/mcp.md](docs/mcp.md)。

## 构建

环境：Go 1.27.1+、golangci-lint v2；Android 需要 JDK 17、Android SDK（platform 36、build-tools 36.1.0）、NDK r27（27.3.13750724）、CMake 3.31.6（构建 llama.cpp）、gomobile。详见 [docs/dev-environment.md](docs/dev-environment.md) 和 [docs/android.md](docs/android.md)。

| 命令 | 说明 |
|---|---|
| `make build` | 编译全部包，生成 `build/bin/ibukirpg`（CLI）与 `build/bin/ibukirpg-server` |
| `make test` / `make vet` / `make lint` | 单元测试 / go vet / golangci-lint |
| `make eval` | LLM Eval：解析器、叙事守卫、结构化世界更新、知识层、战斗裁定、审查等（录音回放 / 假模型，不访问网络） |
| `make eval-synthesize` | Prompt 改动后按用例重新生成（合成）录音 |
| `make mobile-smoke` | gomobile 生成 `build/android/ibukirpg.aar`（冒烟） |
| `make android-aar` | 为 App 生成 `android/app/libs/ibukirpg.aar`（arm / arm64 / x86_64） |
| `make apk-debug` | 调试版 APK |
| `make apk VERSION=0.2.0-alpha1` | 发布版 `build/release/ibukiRPG-v0.2.0-alpha1.apk`（签名配置见 docs/android.md） |
| `make pack-zip PACK=lighthouse` | 校验并把 `packages/<PACK>` 打包成可导入的 `build/packs/<id>-<version>.zip` |

## 目录结构

```text
cmd/cli            命令行客户端        cmd/eval        Eval Runner        cmd/uifixtures  截图测试夹具导出
mobile/            gomobile 导出包（Handle(json) / SetEventSink）
internal/core      engine / command / event / state（事件溯源核心）
internal/action    definition / resolver（离线 + LLM）/ freeform
internal/agent     orchestrator（回合编排、提案网关、审查、卡片生成、角色审查）/ narrator / tools（检索工具）/ memory / structured（合并输出解析与修复）/ audit（一致性审查）
internal/adapter   mobile（gomobile JSON API）/ mcp（MCP stdio 服务）
internal/combat    战斗规则、机甲、数值模拟器（sim）
internal/ai        provider（OpenAI 兼容）/ router（按任务分配模型）/ transport（录音回放）
internal/world     change（世界变更）/ validate（校验器）/ knowledge（玩家知识层）/ timeline（世界事件）
internal/narrative guard（不可变事实校验）
internal/storage   sqlite / eventstore（事件、快照、命令幂等、存档槽）
internal/api       dto（V1 视图）/ query（场景、建议、面板）
internal/eval      Eval Runner
packages/brass     内置开放世界 + 数值 RPG 样板《锈钟镇 · 黄铜试炼》（格式 3，原创）
packages/demo      内置故事包《边境酒馆》   packages/lighthouse  最小结构示例《雾港灯塔》
internal/package   manifest / loader / registry（故事包发现、导入校验、删除）
tests/eval         Eval 用例与录音
android/           Android App（Kotlin + Compose + Material 3）
docs/              架构文档（architecture-v0.3.md）、设计稿（design/v03）与开发文档
```

## 已知限制（v0.2.0-alpha1）

- **alpha 版**：尚未在真机上验证（截图来自 JVM 渲染），AI 功能只用假模型 / 合成录音测试过，未经过真实 API 的大规模测试。
- 世界模拟没有单独的 Agent：时间线事件按规则推进，AI 的世界改动来自合并输出（叙事 + 世界更新）。离线规则模式不会产生 AI 世界变更。
- 撤销一条变更时不会级联提示依赖它的后续变更；MCP 外部写入只在世界日志里可见，下次打开 App 时没有单独的提醒。
- 卡片生成的强度只受校验器上限约束，没有更细的平衡调整；游戏内 AI 还不能通过函数调用直接提出世界变更 / 生成实体（使用 JSON 提示）。
- 离线模式的台词来自故事包台词池；离线解析基于关键词，过于含糊的句子会被当作自由行动。
- 故事包依赖（dependencies）只检查是否已安装及版本，暂不合并内容；NPC 只知道亲眼看到的事，暂不传播流言。
- llama.cpp 本地模型只有 CPU 后端，32 位 ARM 设备不可用。

仓库：<https://github.com/GUYU2233/ibukiRPG> · 架构文档：[docs/architecture-v0.3.md](docs/architecture-v0.3.md)
