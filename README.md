# ibukiRPG

> AI 驱动、事件化、可扩展的中文文字冒险 RPG。**用你自己的话行动，骰子与规则决定结果。**

当前版本：**v0.1.0-rc1（试玩版）** —— 一个小而完整的可玩切片：边境小镇的“锈酒杯”酒馆、4 位 NPC、3 个地点、1 个小事件《失窃的钱袋》。

- 引擎：Go，事件溯源（Command → Event → State），SQLite 存档，确定性骰子（可重放）。
- 客户端：原生 Android（Kotlin + Jetpack Compose + Material 3），以及用于桌面调试的命令行客户端。
- AI：**默认离线模式**（规则解析 + 模板叙事，无需联网、无需 API Key）；可选接入 DeepSeek、通义千问或任意 OpenAI 兼容服务，让大模型理解更自由的输入并生成更生动的叙事。**LLM 永远不是真相源**：检定、金币、物品、关系都由本地引擎决定。

## 怎么玩

1. 安装 APK（见 [Releases](https://github.com/GUYU2233/ibukiRPG/releases)），打开后点 **新游戏**，输入名字。
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

| | 离线模式（默认） | AI 模式 |
|---|---|---|
| 需要网络 / Key | 否 | 是（DeepSeek / 通义千问 / 自定义 OpenAI 兼容） |
| 输入理解 | 关键词 + 规则解析，常见说法都能懂 | 大模型结构化解析（一次调用），失败自动退回离线解析 |
| 叙事 | 模板叙事 | 大模型流式叙事，经 **Narrative Guard** 规则校验，违规时自动换回模板 |
| 规则 / 骰子 / 存档 | 本地引擎 | 本地引擎（完全相同） |

在 **设置 → AI 叙事** 中选择服务商、填写 API Key，点 **测试连接**，再 **保存并应用**。Key 使用 Android Keystore（AES-GCM）加密后保存在本机，不写入存档、不上传。超时或出错时不会重复执行行动，也不会重新掷骰。

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

## 构建

环境：Go 1.27.1+、golangci-lint v2；Android 需要 JDK 17、Android SDK（platform 36、build-tools 36.1.0）、NDK r27、gomobile。详见 [docs/dev-environment.md](docs/dev-environment.md) 和 [docs/android.md](docs/android.md)。

| 命令 | 说明 |
|---|---|
| `make build` | 编译全部包，生成 `build/bin/ibukirpg`（CLI）与 `build/bin/ibukirpg-server` |
| `make test` / `make vet` / `make lint` | 单元测试 / go vet / golangci-lint |
| `make eval` | LLM Eval：离线解析器 + 录音回放的 LLM 解析器 + 叙事守卫（不访问网络） |
| `make eval-synthesize` | Prompt 改动后按用例重新生成（合成）录音 |
| `make mobile-smoke` | gomobile 生成 `build/android/ibukirpg.aar`（冒烟） |
| `make android-aar` | 为 App 生成 `android/app/libs/ibukirpg.aar`（arm / arm64 / x86_64） |
| `make apk-debug` | 调试版 APK |
| `make apk` | 签名发布版 `build/release/ibukiRPG-v0.1.0-rc1.apk`（签名配置见 docs/android.md） |

## 目录结构

```text
cmd/cli            命令行客户端        cmd/eval        Eval Runner        cmd/uifixtures  截图测试夹具导出
mobile/            gomobile 导出包（Handle(json) / SetEventSink）
internal/core      engine / command / event / state（事件溯源核心）
internal/action    definition / resolver（离线 + LLM）/ freeform
internal/agent     orchestrator（回合编排）/ narrator（模板 + LLM + Guard）
internal/ai        provider（OpenAI 兼容）/ transport（录音回放）
internal/narrative guard（不可变事实校验）
internal/storage   sqlite / eventstore（事件、快照、命令幂等、存档槽）
internal/api       dto（V1 视图）/ query（场景、建议、面板）
internal/eval      Eval Runner
packages/demo      Demo 内容包（YAML：地点、人物、物品、动作、事件、规则）
tests/eval         Eval 用例与录音
android/           Android App（Kotlin + Compose + Material 3）
docs/              架构文档（architecture-v0.2.md）与开发文档
```

## 已知限制（v0.1.0-rc1）

- 内容规模很小：3 个地点、4 位 NPC、1 个事件；没有战斗系统（暴力行为会被拒绝并给出替代方案）。
- NPC 只知道自己亲眼看到的事（Witness → Belief），暂不传播流言。
- 离线解析基于关键词，过于复杂或含糊的句子可能被当作“自由行动”处理。
- AI 模式尚未经过真实 API 的大规模测试；Eval 录音为按预期手写的合成录音。

仓库：<https://github.com/GUYU2233/ibukiRPG> · 架构文档：[docs/architecture-v0.2.md](docs/architecture-v0.2.md)
