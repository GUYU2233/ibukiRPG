# ibukiRPG — 开发命令
GO        ?= go
PKG       := github.com/GUYU2233/ibukiRPG
VERSION   ?= 0.1.2-rc2
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS   := -s -w -X $(PKG)/internal/buildinfo.Version=$(VERSION) -X $(PKG)/internal/buildinfo.Commit=$(COMMIT)
BIN       := build/bin
ANDROID_API ?= 24
AAR       := build/android/ibukirpg.aar

# Windows：产物加 .exe 后缀 + POSIX shell 目录前置（否则 System32 的 find/sort 会遮蔽 Unix 版）
ifeq ($(OS),Windows_NT)
EXE := .exe
ifneq ($(findstring sh.exe,$(SHELL)),)
export PATH := $(subst /,\,$(dir $(SHELL)));$(PATH)
endif
endif

.PHONY: all build test lint fmt vet run-cli run-server eval eval-synthesize mobile-smoke android-aar apk apk-debug tidy clean pack-zip

all: fmt vet lint test build

build: ## 编译全部包与可执行文件
	@mkdir -p $(BIN)
	$(GO) build ./...
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/ibukirpg$(EXE) ./cmd/cli
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/ibukirpg-server$(EXE) ./cmd/server

test: ## 运行单元测试（含 race 检测需 CGO，可用 make test RACE=1）
	$(GO) test $(if $(RACE),-race,) -count=1 ./...

lint: ## golangci-lint
	golangci-lint run ./...

fmt: ## 格式化
	gofmt -s -w .
	$(GO) mod tidy

vet:
	$(GO) vet ./...

run-cli: ## 启动 CLI REPL
	$(GO) run ./cmd/cli

run-server:
	$(GO) run ./cmd/server

eval: ## LLM Eval：离线解析 + 录音回放 + 叙事守卫（不访问网络）
	$(GO) run ./cmd/eval -v

PACK ?= lighthouse
pack-zip: ## 校验并打包故事包为可导入的 .zip：make pack-zip PACK=lighthouse（输出到 build/packs/）
	$(GO) run ./cmd/packzip packages/$(PACK)

eval-synthesize: ## 根据用例 llm_output 重新生成合成录音（Prompt 改动后）
	$(GO) run ./cmd/eval -synthesize

mobile-smoke: ## gomobile 生成 Android AAR（需 ANDROID_HOME / ANDROID_NDK_HOME）
	@mkdir -p $(dir $(AAR))
	gomobile bind -target=android -androidapi $(ANDROID_API) -javapkg com.guyu2233.ibukirpg \
		-ldflags "$(LDFLAGS)" -o $(AAR) ./mobile
	@ls -lh $(AAR)

APP_AAR   := android/app/libs/ibukirpg.aar
APP_ABIS  ?= android/arm,android/arm64,android/amd64

android-aar: ## 为 Android App 生成 AAR（arm/arm64/x86_64）
	@mkdir -p $(dir $(APP_AAR))
	gomobile bind -target=$(APP_ABIS) -androidapi $(ANDROID_API) -javapkg com.guyu2233.ibukirpg \
		-ldflags "$(LDFLAGS)" -o $(APP_AAR) ./mobile
	@ls -lh $(APP_AAR)

apk-debug: android-aar ## 构建调试版 APK
	cd android && ./gradlew --no-daemon assembleDebug

apk: android-aar ## 构建签名发布版 APK（签名配置见 docs/android.md）
	cd android && ./gradlew --no-daemon assembleRelease
	@mkdir -p build/release
	cp android/app/build/outputs/apk/release/ibukiRPG-v$(VERSION).apk build/release/
	@ls -lh build/release/ibukiRPG-v$(VERSION).apk

tidy:
	$(GO) mod tidy

clean:
	rm -rf build
