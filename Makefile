# ibukiRPG — 开发命令
GO        ?= go
PKG       := github.com/GUYU2233/ibukiRPG
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS   := -s -w -X $(PKG)/internal/buildinfo.Version=$(VERSION) -X $(PKG)/internal/buildinfo.Commit=$(COMMIT)
BIN       := build/bin
ANDROID_API ?= 24
AAR       := build/android/ibukirpg.aar

.PHONY: all build test lint fmt vet run-cli run-server eval mobile-smoke tidy clean

all: fmt vet lint test build

build: ## 编译全部包与可执行文件
	$(GO) build ./...
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/ibukirpg ./cmd/cli
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN)/ibukirpg-server ./cmd/server

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

eval: ## LLM Eval（占位：尚未实现 Runner）
	@echo "LLM Eval runner 尚未实现；用例位于 tests/eval/*。"
	@find tests/eval -name '*.yaml' | sort

mobile-smoke: ## gomobile 生成 Android AAR（需 ANDROID_HOME / ANDROID_NDK_HOME）
	@mkdir -p $(dir $(AAR))
	gomobile bind -target=android -androidapi $(ANDROID_API) -javapkg com.guyu2233.ibukirpg \
		-ldflags "$(LDFLAGS)" -o $(AAR) ./mobile
	@ls -lh $(AAR)

tidy:
	$(GO) mod tidy

clean:
	rm -rf build
