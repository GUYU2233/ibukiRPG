# ibukiRPG

> AI 驱动、事件化、可扩展的中文文字冒险 RPG。**用你自己的话行动，骰子与规则决定结果。**

当前版本：**v0.1.2-rc2（试玩版，预发布）** —— 三个内置故事包：《边境酒馆 · 失窃的钱袋》、小短篇《雾港灯塔 · 守灯人的信》，以及数值 RPG 示例《锈钟镇 · 黄铜试炼》；支持导入第三方故事包（.zip，格式见 [docs/story-pack-format.md](docs/story-pack-format.md)）。

v0.1.2-rc2 新增：
- **数值 RPG**：回合制战斗（技能、状态、敌人 AI、同伴、掉落）、**机甲形态**（能源 / 过热、改装槽与武器挂点、机甲卡与情报揭示）、成长与装备、**图鉴**、**关系网**（多维关系与变化原因）、**角色卡**（主要 / 次要角色升格、归档）、**立绘**（故事包内图片）。
- **主线贴合度**：自由行动会累计偏离度，严重偏离时可以回到主线或进入**沙盒 / 自由推演**（AI 基于故事包已有内容生成新委托，离线模式用模板）。
- **AI 记忆与检索**：滚动摘要 + 长期记忆压缩（后台运行，不阻塞回合）；信息不足时 AI 会通过**只读检索工具**查阅故事包与存档（按可见范围过滤，NPC 查不到秘密），不支持工具调用的模型改为关键词预取。
- **离线本地模型改用 llama.cpp**：可导入任意 GGUF 模型（推荐 Qwen2.5-1.5B / 3B Instruct Q4_K_M），流式输出、可取消；生成时使用前台服务防止被系统中断，空闲或内存告急时自动释放模型，加载前检查内存与文件。详见 [docs/local-models.md](docs/local-models.md)。
- **稳定性**：进程被系统回收后恢复输入与未完成的提交（不会重复执行）；长聊天记录分页加载；本地崩溃记录在下次启动时显示（不上传）。
- **MCP 服务**：`ibukirpg mcp --save <存档>` 以 stdio 方式把同一套只读工具提供给 Claude Desktop / Cursor / VS Code 等 MCP 客户端，配置方法见 [docs/mcp.md](docs/mcp.md)。

v0.1.2 新增：NPC **对话记忆**（记得聊过什么、看到你做了什么，不再复读，存档后依然记得）、**故事包选择 / 导入**、聊天界面顶部的**实时状态栏**（位置、时间、铜币、当前目标、故事变量，由故事包定义）。

- 引擎：Go，事件溯源（Command → Event → State），SQLite 存档，确定性骰子（可重放）。
- 客户端：原生 Android（Kotlin + Jetpack Compose + Material 3），以及用于桌面调试的命令行客户端。
- AI：**默认规则离线模式**（无需联网、无需 API Key）；可选在手机上离线运行 GGUF 本地模型（llama.cpp），或接入 DeepSeek、通义千问等 OpenAI 兼容服务。**LLM 永远不是真相源**：检定、金币、物品、关系都由本地引擎决定。

## 怎么玩

1. 安装 APK（见 [Releases](https://github.com/GUYU2233/ibukiRPG/releases)），打开后点 **新游戏**，选择一个故事包，输入名字。
2. 在底部输入框里直接写你想做的事，例如：
   - “环顾四周”“和老板打个招呼”“来一杯麦酒”
   - “仔细搜查奥托的座位附近”“说服伯林让我去储藏室看看”“威胁奥托闭嘴”
   - “我跳上桌子唱一首家乡的歌”（自由行动也会产生后果）
3. 不想打字？点输入框上方的**快捷行动**（环顾四周 / 与某人交谈 / 点酒 / 前往…）。
4. 需要检定时会显示一张**检定卡片**：`d20 掷出 17 + 说服 4 = 21（需要 ≥ 15）→ 成功`，结果一目了然。
5. 右上角人像按钮打开面板：**角色**（属性、技能）、**背包**（使用物品、在柜台购买）、**人物**（信任/畏惧、TA 知道的事、各种社交行动的成功率）、**日志**。
6. 每一回合都会自动保存。回到首页点 **继续游戏** 即可从上次的位置继续；**存档** 页可以复制、重命名、删除存档。

小提示：酒馆里过一会儿会出事。仔细调查、取得老板的信任、弄清真相——结局有好几种。

## 离线模式与 AI 模式

| | 离线规则模式（默认） | 离线本地模型（llama.cpp · GGUF） | 在线 AI 模式 |
|---|---|---|---|
| 需要网络 / Key | 否 | 否 | 是（DeepSeek / 通义千问 / 自定义 OpenAI 兼容） |
| 输入理解 | 关键词 + 规则解析 | 设备本地模型结构化解析 | 在线大模型结构化解析，失败自动退回离线规则 |
| 叙事 | 模板叙事 | 设备本地模型，经 **Narrative Guard** 校验 | 在线大模型，经 **Narrative Guard** 校验 |
| 规则 / 骰子 / 存档 | 本地引擎 | 本地引擎（完全相同） | 本地引擎（完全相同） |

在 Android **设置 → AI 叙事** 中选择服务商；在线服务填写 API Key，点 **测试连接**，再 **保存并应用**。Key 使用 Android Keystore（AES-GCM）加密后保存在本机，不写入存档、不上传。超时或出错时不会重复执行行动，也不会重新掷骰。

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

数值 RPG 命令：`/fight` 可发起的战斗、`/attack` `/skill 技能 目标` `/item 道具` `/defend` `/flee` `/mech` `/eject`、`/grow` 成长、`/mechs` `/mechcard` 机甲卡、`/codex` 图鉴、`/rel` 关系网、`/cards` 角色卡、`/main` 主线、`/mode return|free`。

MCP 服务（只读）：`./build/bin/ibukirpg mcp --save <存档目录或 .db>`，详见 [docs/mcp.md](docs/mcp.md)。

## 构建

环境：Go 1.27.1+、golangci-lint v2；Android 需要 JDK 17、Android SDK（platform 36、build-tools 36.1.0）、NDK r27（27.3.13750724）、CMake 3.31.6（构建 llama.cpp）、gomobile。详见 [docs/dev-environment.md](docs/dev-environment.md) 和 [docs/android.md](docs/android.md)。

| 命令 | 说明 |
|---|---|
| `make build` | 编译全部包，生成 `build/bin/ibukirpg`（CLI）与 `build/bin/ibukirpg-server` |
| `make test` / `make vet` / `make lint` | 单元测试 / go vet / golangci-lint |
| `make eval` | LLM Eval：离线解析器 + 录音回放的 LLM 解析器 + 叙事守卫（不访问网络） |
| `make eval-synthesize` | Prompt 改动后按用例重新生成（合成）录音 |
| `make mobile-smoke` | gomobile 生成 `build/android/ibukirpg.aar`（冒烟） |
| `make android-aar` | 为 App 生成 `android/app/libs/ibukirpg.aar`（arm / arm64 / x86_64） |
| `make apk-debug` | 调试版 APK |
| `make apk VERSION=0.1.2-rc2` | 发布版 `build/release/ibukiRPG-v0.1.2-rc2.apk`（签名配置见 docs/android.md） |
| `make pack-zip PACK=lighthouse` | 校验并把 `packages/<PACK>` 打包成可导入的 `build/packs/<id>-<version>.zip` |

## 目录结构

```text
cmd/cli            命令行客户端        cmd/eval        Eval Runner        cmd/uifixtures  截图测试夹具导出
mobile/            gomobile 导出包（Handle(json) / SetEventSink）
internal/core      engine / command / event / state（事件溯源核心）
internal/action    definition / resolver（离线 + LLM）/ freeform
internal/agent     orchestrator（回合编排）/ narrator（模板 + LLM + Guard）/ tools（只读检索工具 + 工具调用循环）/ memory（记忆压缩）
internal/adapter   mobile（gomobile JSON API）/ mcp（MCP stdio 服务）
internal/combat    战斗规则、机甲、数值模拟器（sim）
internal/ai        provider（OpenAI 兼容）/ transport（录音回放）
internal/narrative guard（不可变事实校验）
internal/storage   sqlite / eventstore（事件、快照、命令幂等、存档槽）
internal/api       dto（V1 视图）/ query（场景、建议、面板）
internal/eval      Eval Runner
packages/demo      内置故事包《边境酒馆》（YAML：地点、人物、台词池、物品、动作、事件、HUD）
packages/lighthouse 内置示例故事包《雾港灯塔》（最小结构示例）
packages/brass     内置数值 RPG 示例《锈钟镇 · 黄铜试炼》
internal/package   manifest / loader / registry（故事包发现、导入校验、删除）
tests/eval         Eval 用例与录音
android/           Android App（Kotlin + Compose + Material 3）
docs/              架构文档（architecture-v0.2.md）与开发文档
```

## 已知限制（v0.1.2-rc2）

- 内容规模很小：三个短故事。没有 `combat` 的故事包里暴力行为仍会被拒绝；有战斗的故事包里，场景中动粗是有后果的自由行动并计入主线偏离。
- 战斗中的自然语言输入由规则解析器识别；AI 自由推演临时生成的角色只有角色卡（没有立绘与台词池）；图鉴条目除人物 / 机甲立绘外暂不支持配图。
- 离线模式的台词来自故事包的台词池：说完所有台词后 NPC 会提起“说过了”，而不会编出新内容；想要更自由的对话请开启 AI 模式。
- 故事包依赖（dependencies）只检查是否已安装及版本，暂不合并内容。
- NPC 只知道自己亲眼看到的事（Witness → Belief），暂不传播流言。
- 离线解析基于关键词，过于复杂或含糊的句子可能被当作“自由行动”处理。
- AI 模式尚未经过真实 API 的大规模测试；Eval 录音为按预期手写的合成录音。
- llama.cpp 本地模型只在 CI / 构建机上完成编译与 JVM 单元测试，尚未在真机上验证推理速度与稳定性；当前只有 CPU 后端（不支持 GPU 加速），32 位 ARM 设备不可用。

仓库：<https://github.com/GUYU2233/ibukiRPG> · 架构文档：[docs/architecture-v0.2.md](docs/architecture-v0.2.md)
