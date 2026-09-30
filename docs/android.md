# Android App 构建说明

App 位于 `android/`：Kotlin + Jetpack Compose + Material 3（`androidx.compose.material3`），单 Activity，Navigation Compose。
Go 引擎通过 `gomobile bind` 生成 AAR（`android/app/libs/ibukirpg.aar`，不入库），Kotlin 端经版本化 JSON API（v1）调用 `Mobile.handle(json)`，
叙事增量通过 `Mobile.setEventSink(EventSink)` 推送。

| 项目 | 值 |
|---|---|
| applicationId | `com.guyu2233.ibukirpg`（debug 版为 `.debug` 后缀，可与正式版共存） |
| versionName / versionCode | `0.1.2rc1` / `3` |
| minSdk / targetSdk / compileSdk | 24 / 36 / 36 |
| 工具链 | Gradle 8.14.3（wrapper）、AGP 8.13.2、Kotlin 2.3.21、Compose BOM 2025.10.01 |
| ABI | armeabi-v7a、arm64-v8a、x86_64 |

## 构建步骤

```bash
source /etc/profile.d/ibukirpg-dev.sh      # JAVA_HOME / ANDROID_HOME / ANDROID_NDK_HOME
make android-aar                           # gomobile bind → android/app/libs/ibukirpg.aar
cd android
./gradlew assembleDebug                    # app/build/outputs/apk/debug/ibukiRPG-v0.1.2rc1-debug.apk
./gradlew testDebugUnitTest                # JVM 截图测试（Robolectric + Roborazzi）→ ../build/screenshots/*.png
./gradlew assembleRelease                  # app/build/outputs/apk/release/ibukiRPG-v0.1.2rc1.apk
```

或在仓库根目录执行 `make apk`（产物复制到 `build/release/`）。

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

找不到该文件时 release 构建仍会成功，但 APK 未签名（无法直接安装）。**请备份密钥库**：后续版本必须用同一个密钥签名才能覆盖安装。

## MediaPipe 本地模型

Android App 集成 `com.google.mediapipe:tasks-genai:0.10.27`。设置页的“离线本地模型（MediaPipe）”使用系统文件选择器导入 MediaPipe `.task` bundle，并复制到 `filesDir/mediapipe-models/`；不会把外部 `content://` URI 传给原生推理。推理只由应用内 loopback HTTP bridge 暴露给 Go 引擎（绑定 `127.0.0.1` 且带随机 Bearer token），模型文件不上传。

`.task` 必须是 MediaPipe LLM Inference 支持/打包的模型；不能直接选 GGUF、普通 safetensors 或原始权重。上下文设置对应 MediaPipe 的 max-token/KV-cache 总预算（输入和输出合计），另可调 temperature、Top-K、Top-P。设备内存、模型上下文上限和硬件加速要求各不相同，加载/推理错误会显示在设置页；切回规则离线模式会卸载本地模型释放内存。

注意：Google 将 MediaPipe LLM Inference 标记为 maintenance-only，并建议面向高端 Android 设备；项目最低 SDK 仍为 API 24，但这不代表 API 24 或低内存设备能运行任意模型。发布自制/微调模型前，请核实模型本身的许可证和分发条款。

## 截图

`android/app/src/test/.../ScreenshotTest.kt` 用 Robolectric 原生图形模式渲染真实的界面 Composable（首页、游戏、面板、设置，浅色 / 深色）。
游戏数据来自 `go run ./cmd/uifixtures`：真实 Go 引擎离线试玩 5 回合后导出的 JSON（`app/src/test/resources/fixtures/`）。

## 设计要点（易用性）

- 首页“继续游戏”为最醒目的主按钮；没有存档时主按钮变为“新游戏”。
- 游戏页：聊天式记录（玩家气泡 / 叙事正文 / 检定卡片 / 结果小标签 / 事件横幅），新叙事逐字出现（点击立即显示全文）。
- 快捷行动随场景变化（事件提示、交谈对象、点酒、前往…）；澄清时直接给出可点选的选项。
- 提交失败时输入会放回输入框，重试复用同一个 command_id —— 不会重复执行、不会重新掷骰。
- 面板：角色 / 背包 / 人物（信任、畏惧、TA 知道的事及来源、社交行动成功率）/ 日志。
- 设置：规则离线 / MediaPipe 本地模型 / DeepSeek / 通义千问 / 自定义；MediaPipe 支持本地 `.task` 导入和生成参数，在线 API Key 经 Android Keystore AES-GCM 加密存于 DataStore；文字大小、主题、动态取色（Android 12+）。
- 动态取色不可用时使用以琥珀色 `#8C4A1C` 为种子的 Material 3 配色；全屏 edge-to-edge；中文字符串全部在 `res/values/strings.xml`；图标按钮均有 contentDescription。
