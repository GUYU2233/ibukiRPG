# ibukiRPG

> AI 驱动、事件化、可扩展、可 Mod 的文字世界模拟 RPG 框架（Go）。

- 核心：Go，事件溯源（Event Sourcing），内容数据化（YAML Package），可 Mod。
- 目标平台：**Android 优先**（Go Core 通过 `gomobile bind` 生成 AAR），Windows / Linux 用于开发调试。
- 存储：SQLite，使用纯 Go 驱动 [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite)（无 CGO）。
- 表达式：[CEL](https://github.com/google/cel-go)（导入路径 `cel.dev/cel-go`）。
- AI：云端 LLM 优先（DeepSeek / Qwen 等 OpenAI 兼容 API），LLM 不是真相源。
- 架构文档：[docs/architecture-v0.2.md](docs/architecture-v0.2.md)；开发环境：[docs/dev-environment.md](docs/dev-environment.md)。

仓库：<https://github.com/GUYU2233/ibukiRPG>，Go 模块路径 `github.com/GUYU2233/ibukiRPG`。

## 环境要求

| 工具 | 版本 | 用途 |
|---|---|---|
| Go | 1.27.1+（见 `go.mod`） | 编译 |
| golangci-lint | v2.x | `make lint` |
| make、git | 任意较新版本 | 构建命令 |
| sqlite3 CLI | 可选 | 调试存档 |
| JDK 17、Android SDK、NDK r27 LTS、gomobile | 仅 Android 构建需要 | `make mobile-smoke` |

详细安装步骤与版本见 [docs/dev-environment.md](docs/dev-environment.md)。

## 快速开始

```bash
cp .env.example .env        # 填写 API Key（切勿提交 .env）
make build                  # 编译，产物在 build/bin/
make test                   # 单元测试
make run-cli                # 启动 CLI REPL（/help、/quit）
```

## 常用命令

| 命令 | 说明 |
|---|---|
| `make build` | `go build ./...` 并生成 `build/bin/ibukirpg`、`build/bin/ibukirpg-server` |
| `make test` | 运行全部单元测试（`RACE=1` 开启 race 检测，需 CGO） |
| `make lint` | golangci-lint（配置见 `.golangci.yml`） |
| `make fmt` | gofmt + go mod tidy |
| `make vet` | go vet |
| `make run-cli` / `make run-server` | 启动 CLI / HTTP 调试服务（`127.0.0.1:8080`，`GET /healthz`、`POST /v1/handle`） |
| `make eval` | LLM Eval 占位（列出 `tests/eval` 用例，Runner 尚未实现） |
| `make mobile-smoke` | `gomobile bind -target=android -androidapi 24` 生成 `build/android/ibukirpg.aar` |

## 目录结构

```text
.
├── cmd/
│   ├── cli/            # 命令行 REPL（调试入口）
│   └── server/         # HTTP 调试服务
├── mobile/             # gomobile bind 导出包：Version() / Handle(json) string
├── internal/           # 架构文档第 50 节模块划分
│   ├── core/           # engine, command, event, state, transaction
│   ├── action/         # definition, resolver, freeform, validator
│   ├── rules/          # stats, checks, expression(CEL), rng, fixedpoint
│   ├── world/          # worldtime, simulation, materialization, location, faction, canon, scene
│   ├── perception/     # visibility, witness, observation
│   ├── character/      # character, belief, memory, relationship
│   ├── combat/  inventory/
│   ├── story/          # graph, director, timeline, pacing
│   ├── agent/          # orchestrator, narrator, npc, memory, simulator, canon
│   ├── ai/             # provider, router, fallback, context, transport
│   ├── narrative/      # guard, claims, fallback
│   ├── package/        # loader, manifest, dependency, patch, migration
│   ├── mod/            # runtime, sandbox
│   ├── api/            # command, query, dto
│   ├── adapter/        # mcp, mobile, debug
│   ├── storage/        # sqlite, eventstore, snapshot, save
│   ├── retrieval/      # lore, index
│   ├── prompt/         # builder, sections, patch
│   └── buildinfo/      # 版本信息（-ldflags 注入）
├── packages/demo/      # Demo 世界包（demo: 命名空间；酒馆 + 3 NPC + Action）
├── tests/eval/         # LLM Eval 用例（intent, reasonability, ...）
├── docs/               # 架构文档与开发文档
├── tools/              # 开发辅助工具
└── build/              # 构建产物（git 忽略）
```

## 已实现的最小可运行部件

| 包 | 内容 |
|---|---|
| `internal/storage/sqlite` | 打开 SQLite（内存 / 文件 WAL），`WithTx` 事务（全部成功或全部回滚） |
| `internal/rules/expression` | CEL 求值器（`actor` / `target` / `scene` / `world` 变量，编译缓存，代价上限） |
| `internal/rules/rng` | `RNG(namespace, entity, purpose)` 确定性分流，可序列化状态 |
| `internal/rules/fixedpoint` | 定点数（1000 = 1.000） |
| `internal/package/manifest` | `manifest.yaml` 解析与校验，命名空间 ID 校验 |
| `internal/action/definition` | ActionDefinition YAML 解析；测试中用 CEL 编译 demo 包全部 requirements |
| `internal/adapter/mobile` + `mobile/` | 版本化 JSON DTO 入口（`ping` / `sqlite_smoke` / `roll`） |

其余子包目前只有 `doc.go`（包说明），按 Phase 0 / Phase 1 逐步实现。
