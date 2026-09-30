# 开发环境说明

记录日期：2026-09-30。参考环境：Debian 13 (trixie) x86_64。

## 已安装版本

| 组件 | 版本 | 安装位置 / 方式 |
|---|---|---|
| Go | 1.27.1 | 官方 tarball → `/usr/local/go`（已移除 Debian 自带的 golang-1.24） |
| gopls | v0.23.0 | `go install golang.org/x/tools/gopls@latest` → `~/go/bin` |
| golangci-lint | v2.14.0 | 官方 install.sh → `~/go/bin` |
| staticcheck | 2026.2.1 (0.8.1) | `go install` |
| govulncheck | v1.8.0 | `go install`（当前依赖：No vulnerabilities found） |
| gomobile / gobind | golang.org/x/mobile v0.0.0-20260908204917-8b95e45f8d3e | `go install`；同时在 `go.mod` 中以 `tool` 指令固定 |
| git | 2.47.3 | apt |
| make | GNU Make 4.4.1 | apt |
| sqlite3 CLI | 3.46.1 | apt（`modernc.org/sqlite` v1.60.1 内嵌 SQLite 3.53.4） |
| JDK | Eclipse Temurin 17.0.20.1+1 | tarball → `/opt/jdk-17.0.20.1+1`，软链 `/opt/jdk17`（Debian 13 仓库无 OpenJDK 17） |
| Android cmdline-tools | 13114758 (latest) | `/opt/android-sdk/cmdline-tools/latest` |
| Android platform-tools | 37.0.1 | sdkmanager |
| Android Platform | android-36 | sdkmanager |
| Build-Tools | 36.1.0 | sdkmanager |
| NDK | r27d LTS（27.3.13750724） | `/opt/android-sdk/ndk/27.3.13750724` |

## 环境变量

写入 `/etc/profile.d/ibukirpg-dev.sh`（`/etc/bash.bashrc` 与 `~/.bashrc` 会 source 它），并在 `/etc/environment` 中写入等价的字面值：

```bash
export GOROOT=/usr/local/go
export JAVA_HOME=/opt/jdk17
export ANDROID_HOME=/opt/android-sdk
export ANDROID_SDK_ROOT=/opt/android-sdk
export ANDROID_NDK_HOME=/opt/android-sdk/ndk/27.3.13750724
export PATH=/usr/local/go/bin:$HOME/go/bin:$JAVA_HOME/bin:$ANDROID_HOME/cmdline-tools/latest/bin:$PATH:$ANDROID_HOME/platform-tools
```

> platform-tools 放在 PATH 末尾：它自带一个 `sqlite3`，否则会覆盖系统的 sqlite3 CLI。

## 在 Linux 上复现

```bash
# Go
curl -LO https://go.dev/dl/go1.27.1.linux-amd64.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.27.1.linux-amd64.tar.gz
# 工具
go install golang.org/x/tools/gopls@latest
go install honnef.co/go/tools/cmd/staticcheck@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(go env GOPATH)/bin
sudo apt-get install -y git make sqlite3 unzip
# Android（仅需构建 AAR 时）
#   JDK 17: https://adoptium.net  → /opt/jdk17
#   cmdline-tools: https://developer.android.com/studio#command-line-tools-only
#   解压到 $ANDROID_HOME/cmdline-tools/latest 后：
yes | sdkmanager --licenses
sdkmanager "platform-tools" "platforms;android-36" "build-tools;36.1.0" "ndk;27.3.13750724"
go install golang.org/x/mobile/cmd/gomobile@latest golang.org/x/mobile/cmd/gobind@latest
gomobile init
make mobile-smoke
```

## 在 Windows 上复现（简要）

1. 安装 Go 1.27.1 MSI（https://go.dev/dl/），Git for Windows；`make` 可用 `winget install ezwinports.make` 或 MSYS2 / Git Bash 提供（也可直接执行 Makefile 中的 `go` 命令）。
2. `winget install golangci.golangci-lint` 或 `go install`。
3. 因为 SQLite 使用纯 Go 驱动，**无需 CGO / gcc** 即可 `go build`、`go test`。
4. Android：安装 Android Studio（或 cmdline-tools）+ JDK 17，通过 SDK Manager 安装 NDK 27.3.13750724，设置 `ANDROID_HOME`、`ANDROID_NDK_HOME` 用户环境变量，然后 `gomobile init` 与 `make mobile-smoke`（建议在 Git Bash 中执行）。

## Android 冒烟测试结果

`make mobile-smoke`（`gomobile bind -target=android -androidapi 24 -javapkg com.guyu2233.ibukirpg ./mobile`）成功：

- 产物：`build/android/ibukirpg.aar`（约 13 MB，含 armeabi-v7a / arm64-v8a / x86 / x86_64 四个 `libgojni.so`，每个约 7–8 MB）
- Java API：`com.guyu2233.ibukirpg.mobile.Mobile.version()`、`Mobile.handle(String json)`
- 耗时约 40 秒（8 核）。

## 已知注意事项

- **cel-go 导入路径已迁移**：新版本模块路径为 `cel.dev/cel-go`（`github.com/google/cel-go@v0.32.0` 的 go.mod 声明为 `cel.dev/cel-go`），代码中使用 `cel.dev/cel-go/cel`。
- **gopkg.in/yaml.v3** 已归档，官方维护的后续版本为 `go.yaml.in/yaml/v3`；目前按要求使用 `gopkg.in/yaml.v3`，后续可无痛替换。
- **gomobile 与 internal 包**：gomobile 绑定的包放在仓库根下 `mobile/`（导出类型仅限 string），它调用 `internal/adapter/mobile`。`golang.org/x/mobile` 通过 `go.mod` 的 `tool` 指令固定版本，这是 `gomobile bind` 所需。
- **AAR 体积**：四 ABI 合计约 13 MB（压缩）。正式发布可用 `-target=android/arm64,android/arm` 只保留真机 ABI，并保留 `-ldflags "-s -w"`。
- **内存 SQLite**：`:memory:` 每个连接独立，`storage/sqlite.Open` 对内存库限制单连接。
- **race 检测**需要 CGO 与 C 编译器；默认 `make test` 不开启。
- 真机 / 模拟器运行测试尚未进行（本机无模拟器），当前仅验证 AAR 可成功生成。
- 架构文档中的 `world/time` 模块在代码中命名为 `internal/world/worldtime`，避免与标准库 `time` 同名。
