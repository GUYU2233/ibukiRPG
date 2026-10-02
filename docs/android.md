# Android App 构建说明

App 位于 `android/`：Kotlin + Jetpack Compose + Material 3（`androidx.compose.material3`），单 Activity，Navigation Compose。
Go 引擎通过 `gomobile bind` 生成 AAR（`android/app/libs/ibukirpg.aar`，不入库），Kotlin 端经版本化 JSON API（v1）调用 `Mobile.handle(json)`，
叙事增量通过 `Mobile.setEventSink(EventSink)` 推送。

| 项目 | 值 |
|---|---|
| applicationId | `com.guyu2233.ibukirpg`（debug 版为 `.debug` 后缀，可与正式版共存） |
| versionName / versionCode | `0.2.0-alpha1` / `6` |
| minSdk / targetSdk / compileSdk | 24 / 36 / 36 |
| 工具链 | Gradle 8.14.3（wrapper）、AGP 8.13.2、Kotlin 2.3.21、Compose BOM 2025.10.01 |
| ABI | armeabi-v7a、arm64-v8a、x86_64（llama.cpp 本地模型：arm64-v8a、x86_64） |
| 原生构建 | NDK 27.3.13750724、CMake 3.31.6；llama.cpp v0.5.0（`app/src/main/cpp/CMakeLists.txt`） |

## 构建步骤

```bash
source /etc/profile.d/ibukirpg-dev.sh      # JAVA_HOME / ANDROID_HOME / ANDROID_NDK_HOME
make android-aar                           # gomobile bind → android/app/libs/ibukirpg.aar
cd android
./gradlew assembleDebug                    # app/build/outputs/apk/debug/ibukiRPG-v0.2.0-alpha1-debug.apk（含 llama.cpp 原生库）
./gradlew testDebugUnitTest                # JVM 单元测试 + 截图测试（Robolectric + Roborazzi）→ ../build/screenshots/*.png
./gradlew assembleRelease                  # app/build/outputs/apk/release/ibukiRPG-v0.2.0-alpha1.apk
```

或在仓库根目录执行 `make apk`（产物复制到 `build/release/`）。

第一次构建会下载 llama.cpp v0.5.0 源码（校验 SHA256）到 `android/.llama-cache/`，并为每个 ABI 编译一次（8 核约 1～2 分钟），之后增量构建。
离线环境可用 `-Pibuki.llama.src=/path/to/llama.cpp` 指定本地源码；`-Pibuki.llama.abis=arm64-v8a` 只编译真机 ABI 以缩短构建时间。
CI 缓存 NDK / CMake、`android/.llama-cache` 与 `android/app/.cxx`。

APK 大小（v0.1.2-rc2，本地构建）：debug 62.2 MB、release（R8）40.4 MB。三个 ABI 的 Go 引擎各约 22 MB（未压缩）；llama.cpp 在 arm64-v8a 上约 12 MB、x86_64 上约 22 MB（含多个 CPU 变体与 libc++_shared）。

## 发布签名（密钥库不入库）

`app/build.gradle.kts` 从仓库**之外**读取签名配置，默认路径 `~/.ibukirpg/keystore.properties`（可用环境变量
`IBUKIRPG_KEYSTORE_PROPERTIES` 指定其他路径）：

```properties
storeFile=/home/<you>/.ibukirpg/keystore
storePassword=********
keyAlias=ibukirpg
keyPassword=********
```

生成一个自签名密钥库（与 debug 签名类似，仅用于侧载分发）：

```bash
mkdir -p ~/.ibukirpg && chmod 700 ~/.ibukirpg
keytool -genkeypair -keystore ~/.ibukirpg/keystore -storetype PKCS12 -alias ibukirpg \
  -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=ibukiRPG, O=GUYU2233, C=CN"
# 然后按上面的格式写 ~/.ibukirpg/keystore.properties，并 chmod 600
```

CI（`.github/workflows/ci.yml`）改用环境变量签名：`ANDROID_KEYSTORE_FILE`（由 secret `ANDROID_KEYSTORE_BASE64` 解码到临时文件）、
`ANDROID_KEYSTORE_PASSWORD`、`ANDROID_KEY_ALIAS`、`ANDROID_KEY_PASSWORD`。四个变量齐全时优先使用，否则回退到 keystore.properties。
release 签名只在本仓库的 push / 手动触发上运行，pull_request（含 fork）不会接触密钥；`v*` 标签构建必须产出 release 签名 APK，
并用 `apksigner verify --print-certs` 校验证书 SHA-256（`5a9ac40d7ed48d05e2a1e1b7f989fe100f82b886a9101feb166ea71ed2323339`）后附到 GitHub Release。

找不到该文件时 release 构建仍会成功，但 APK 未签名（无法直接安装）。**请备份密钥库**：后续版本必须用同一个密钥签名才能覆盖安装。

## 16 KB 页对齐

Android 15+ 的 16 KB 页设备要求每个 `.so` 的 ELF LOAD 段按 16384 对齐。NDK 27 编译的 llama.cpp 默认已满足；
gomobile 生成的 `libgojni.so` 需显式传 `-ldflags '… -extldflags=-Wl,-z,max-page-size=16384'`（Makefile 的 `MOBILE_LDFLAGS`，三个 ABI 都生效）。
`tools/check-apk-alignment.sh <apk>`（或 `make check-apk-align APK=…`）检查 APK 内所有 `.so` 的 `p_align >= 16384` 并运行
`zipalign -c -P 16 -v 4`；CI 对 debug 与 release APK 都执行该检查。

注意：v0.1.2-rc2 及更早发布的是 debug 签名 APK，与 v0.1.3-rc1 起的 release 签名不同，需先卸载旧版再安装。

## 本地模型（llama.cpp）

设置页的“离线本地模型（llama.cpp · GGUF）”用系统文件选择器导入 `.gguf` 文件，校验文件头后复制到 `filesDir/models/`；不会把外部 `content://` URI 传给原生代码。
推理由 `llm/LocalLlm`（单实例、加载前内存检查、空闲 / onTrimMemory 释放）+ JNI（`libibuki_llama.so`）完成，经应用内 loopback HTTP bridge（`127.0.0.1` 随机端口 + 随机 Bearer token，SSE 流式）暴露给 Go 引擎；生成期间运行 `specialUse` 前台服务并显示可取消的通知。
模型推荐、内存估算、后台运行与迁移说明见 [local-models.md](local-models.md)。

## 稳定性

- 引擎 / DataStore / 文件 IO 都在 `Dispatchers.IO`；原生推理在后台线程；聊天列表使用稳定 key 与 contentType，内存中最多保留 400 条记录，更早的点“加载更早的记录”分页读取。
- `GameViewModel` 用 `SavedStateHandle` 保存输入草稿与进行中的提交（command_id），进程被回收后恢复；每回合事件在提交时已写入 SQLite。
- 未捕获异常写入 `files/crash/last_crash.txt`，Android 11+ 另读取 `ApplicationExitInfo`（原生崩溃 / ANR / 低内存）；首页下次启动时显示，可复制，不上传。
- 不使用 `largeHeap`：模型权重是 mmap 的原生内存，不在 Java 堆里。
- 电池优化豁免只作为可选引导（设置页按钮打开系统设置页），不申请 `REQUEST_IGNORE_BATTERY_OPTIMIZATIONS`。

## 截图

`android/app/src/test/.../ScreenshotTest.kt` 用 Robolectric 原生图形模式渲染真实的界面 Composable（首页、游戏、世界面板、战斗、
决策提示、时间线、审查卡片、卡片修改预览、API 设置 / 生成设置 / 提示灵敏度，浅色 / 深色，手机与平板宽度），输出到仓库根目录的 `build/screenshots/`。
游戏数据来自 `go run ./cmd/uifixtures`：真实 Go 引擎用格式 3 示例包《锈钟镇》离线试玩后导出的 JSON（`app/src/test/resources/fixtures/`，
v0.2 截图使用 `v02-*.json` 夹具：叙事流、世界面板、时间线、战斗裁定、变更预览等；rc1 新增 `v02_sim.json`、`v02_revert_plan.json`、`v02_external.json`，对应截图 50–55）。界面设计稿见 `docs/design/v03/`。

## 设计要点（易用性）

- 首页“继续游戏”为最醒目的主按钮；没有存档时主按钮变为“新游戏”。
- 游戏页（v0.2）：叙事流（玩家行动 / 叙事正文 / 检定与**战斗裁定卡片**（解析→合理性→修正→掷骰→成功度→状态）/ 世界变更与**知识解锁小标签** / 事件横幅 /
  每回合 **token 用量**行），顶部**状态条**（位置、时间、HP、货币、当前目标，由故事包 HUD 定义），战斗中在叙事流上方显示**战斗头部**（敌我状态、回合）。
- **世界面板**（8 个标签：角色 / 关系网 / 图鉴 / 装备 / 地图 / 势力 / 时间线 / 日志）：手机上是底部抽屉，平板（≥ 840dp）上是右侧常驻栏；
  未解锁的字段显示“未知”，解锁后显示来源回合。角色卡可以“让 AI 修改”或“新建角色 / 卡片”，修改先显示**差异预览**，确认后才写入（可在日志中撤销）。
- **决策底部抽屉**：AI 的改动偏离设定、重要角色死亡或严重影响剧情时弹出（接受并继续 / 回到上一回合 / 以后只通知），并显示自动创建的检查点，灵敏度按提示类型在设置中调整，也可设为“只通知”。
- **场外世界（v0.2.0-rc1）**：世界模拟的结果在叙事流里显示为“场外”卡片（传闻 + 变更小标签，标明 AI / 规则来源）；场外推进触发的决策提示标为“场外世界”，回滚只撤掉这次场外推进。
- **级联撤销（v0.2.0-rc1）**：在日志里撤销一条有依赖的变更时，先弹出确认对话框，列出之后依赖它的变更，可以“连同撤销 N 项”或“只撤销这一条”（不能单独撤销时按钮禁用并说明原因）。
- **外部修改提醒（v0.2.0-rc1）**：MCP 改过设定后，打开游戏时叙事流顶部显示“外部工具修改了 N 项设定”横幅；“查看日志”打开世界面板日志页并只显示外部工具的修改，“知道了”确认后不再提示。
- **时间线 / 分支导航**：回到任意回合（旧分支保留）、切换分支、手动检查点（重要决定前自动创建，可关闭）。
- **一致性审查卡片**：定期或手动审查后显示自动修正（设定 / 文字，可撤销）与需确认的建议（物品、金钱、经验、生死）。
- 存档页支持**导出 / 导入 `.ibksave`**（不含 API Key，带引擎与故事包版本；0.1.x 存档会被拒绝并提示）。
- 快捷行动随场景变化；澄清时直接给出可点选的选项。提交失败时输入会放回输入框，重试复用同一个 command_id —— 不会重复执行、不会重新掷骰。
- **API 设置**：多个服务商（DeepSeek / 通义千问 / 自定义 OpenAI 兼容 / 本地 llama.cpp），每个服务商的 Key 经 Android Keystore AES-GCM 加密存于 DataStore。
  **生成设置**：统一模型或按任务分配（叙事 + 世界更新、意图解析、战斗裁定、记忆、世界模拟、一致性审查、卡片生成、角色审查），ⓘ 图标说明哪些任务适合本地模型；
  token 用量显示、审查间隔；**世界模拟**的推进间隔（关闭 / 默认 / 2 小时 / 8 小时 / 每天）与每个游戏日的 AI 次数上限（只用规则 / 默认 / 2 / 8），以及“游戏内写入工具”开关（v0.2.0-rc1）。另有文字大小、主题、动态取色（Android 12+）。
- 动态取色不可用时使用以琥珀色 `#8C4A1C` 为种子的 Material 3 配色；全屏 edge-to-edge；中文字符串全部在 `res/values/strings.xml`；图标按钮均有 contentDescription。

## AI 检索工具与 MCP

手机端不启动 MCP 服务：AI 需要查阅故事包 / 存档时，直接在进程内调用同一套只读检索工具（Tool Gateway，按 NPC / 玩家 / 导演的可见范围过滤）。
调试时可以通过 JSON API 的 `list_tools` / `call_tool` 请求查看工具列表并手动调用（只读，不会修改存档）。
桌面端的 MCP 服务（`ibukirpg mcp --save <存档>`）见 [mcp.md](mcp.md)。
