# Android App 构建说明

App 位于 `android/`：Kotlin + Jetpack Compose + Material 3（`androidx.compose.material3`），单 Activity，Navigation Compose。
Go 引擎通过 `gomobile bind` 生成 AAR（`android/app/libs/ibukirpg.aar`，不入库），Kotlin 端经版本化 JSON API（v1）调用 `Mobile.handle(json)`，
叙事增量通过 `Mobile.setEventSink(EventSink)` 推送。

| 项目 | 值 |
|---|---|
| applicationId | `com.guyu2233.ibukirpg`（debug 版为 `.debug` 后缀，可与正式版共存） |
| versionName / versionCode | `0.1.0-rc1` / `1` |
| minSdk / targetSdk / compileSdk | 24 / 36 / 36 |
| 工具链 | Gradle 8.14.3（wrapper）、AGP 8.13.2、Kotlin 2.3.21、Compose BOM 2025.10.01 |
| ABI | armeabi-v7a、arm64-v8a、x86_64 |

## 构建步骤

```bash
source /etc/profile.d/ibukirpg-dev.sh      # JAVA_HOME / ANDROID_HOME / ANDROID_NDK_HOME
make android-aar                           # gomobile bind → android/app/libs/ibukirpg.aar
cd android
./gradlew assembleDebug                    # app/build/outputs/apk/debug/ibukiRPG-v0.1.0-rc1-debug.apk
./gradlew testDebugUnitTest                # JVM 截图测试（Robolectric + Roborazzi）→ ../build/screenshots/*.png
./gradlew assembleRelease                  # app/build/outputs/apk/release/ibukiRPG-v0.1.0-rc1.apk
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

## 截图

`android/app/src/test/.../ScreenshotTest.kt` 用 Robolectric 原生图形模式渲染真实的界面 Composable（首页、游戏、面板、设置，浅色 / 深色）。
游戏数据来自 `go run ./cmd/uifixtures`：真实 Go 引擎离线试玩 5 回合后导出的 JSON（`app/src/test/resources/fixtures/`）。

## 设计要点（易用性）

- 首页“继续游戏”为最醒目的主按钮；没有存档时主按钮变为“新游戏”。
- 游戏页：聊天式记录（玩家气泡 / 叙事正文 / 检定卡片 / 结果小标签 / 事件横幅），新叙事逐字出现（点击立即显示全文）。
- 快捷行动随场景变化（事件提示、交谈对象、点酒、前往…）；澄清时直接给出可点选的选项。
- 提交失败时输入会放回输入框，重试复用同一个 command_id —— 不会重复执行、不会重新掷骰。
- 面板：角色 / 背包 / 人物（信任、畏惧、TA 知道的事及来源、社交行动成功率）/ 日志。
- 设置：离线 / DeepSeek / 通义千问 / 自定义；测试连接；API Key 经 Android Keystore AES-GCM 加密存于 DataStore；文字大小、主题、动态取色（Android 12+）。
- 动态取色不可用时使用以琥珀色 `#8C4A1C` 为种子的 Material 3 配色；全屏 edge-to-edge；中文字符串全部在 `res/values/strings.xml`；图标按钮均有 contentDescription。
