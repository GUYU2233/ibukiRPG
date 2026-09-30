# MCP 适配器与只读检索工具

ibukiRPG 的 AI Agent（叙述者、导演 / 自由推演规划、NPC）在信息不足时会**主动查资料**：
上下文被压缩成摘要之后、玩家跑出预设剧本（自由推演 / 沙盒）时、或者玩家提到了当前上下文里没有的人物和名词时。
查资料用的是一组**只读检索工具**。这组工具有两种暴露方式：

| 使用方 | 通道 | 说明 |
| --- | --- | --- |
| 游戏内 AI Agent | 进程内工具网关（`internal/agent/tools`） | OpenAI 兼容函数调用；模型不支持时退化为关键词预检索 |
| Android 客户端 | 移动端 JSON API：`list_tools` / `call_tool` | 同一套工具，进程内调用（手机上不跑 MCP） |
| 桌面 / CLI 上的 MCP 客户端 | `ibukirpg mcp`（stdio） | 本文重点 |

所有工具都是**只读**的：不会提交输入、不会执行动作、不会存档或读档，也不会改动任何游戏状态。
MCP 服务器只暴露这些读取工具（架构文档第 48 节：MCP 只是 Command / Query API 之上的一层适配器）。

## 工具一览

MCP 里的工具名用下划线（`pack_search`），与游戏内函数调用的名字一致；调用时写成带点的名字（`pack.search`）也可以。

| 工具 | 作用 | 可用范围 |
| --- | --- | --- |
| `pack_search(query, type?, limit?)` | 在整个故事包里全文检索：人物、地点、物品、技能、敌人、机甲、势力、图鉴、剧情节点 | 全部 |
| `pack_get_entity(id)` | 按 ID 或名字读取一个实体（当前身份可见的部分） | 全部 |
| `pack_list(type)` | 列出某类实体的 ID 与名字 | 全部 |
| `character_get_card(id)` | 人物卡：身份、状态、所在地、与玩家的关系 | 全部 |
| `mech_get_card(id)` | 机甲卡：型号、驾驶者、规格、挂载；未掌握的字段显示“未知” | 全部 |
| `relationship_get(who, other?)` | 人物关系（信任 / 好感 / 敌意……） | 全部 |
| `memory_search(query, who?, limit?)` | NPC 记忆、事件日志、滚动对话摘要与长期记忆 | 全部（NPC 只能查自己的） |
| `story_get_state()` | 主线模式、当前锚点与目标、进行中的事件、自由推演节点 | player / director |
| `story_anchors()` | 主线锚点列表 | player / director |
| `world_get_location(id?)` | 地点描述、场景事实、出口、在场人物（省略 id 为玩家当前位置） | 全部 |
| `rules_get_action(id)` | 动作规则：说明、关键词、检定技能、耗时 | 全部 |

结果是 JSON 文本，大小有上限（每次最多 8 条结果、单字段 280 字、总计约 6 KB），同样的输入总是得到同样的输出。

## 范围（scope）：谁能看到什么

| 范围 | 对应 Agent | 可见内容 |
| --- | --- | --- |
| `player`（默认，别名 `narrator`） | 叙述者 | 玩家已知的内容（PlayerScope）：见过、交谈过、图鉴已解锁的实体；只能看到当前锚点（允许的伏笔），看不到后续锚点和任何人物秘密 |
| `npc:<角色ID>` | NPC Agent | 该 NPC 知道的内容（NPCScope）：公开资料 + 自己的来历与秘密 + 自己的记忆；看不到别人的秘密和记忆，看不到剧本节点与锚点 |
| `director`（别名 `planner`） | 导演 / 自由推演规划 | 完整设定：全部秘密、隐藏的机甲规格、全部锚点与剧情节点 |

“不存在”和“无权查看”返回同样的错误，因此不会通过报错泄露“这里有个秘密”。

**用 MCP 客户端玩自己的存档时，建议用默认的 `player` 范围**，否则会被剧透。

## 启动 MCP 服务器

```bash
# 使用默认存档目录，读取最近更新的存档，玩家范围
ibukirpg mcp

# 指定存档数据库（或存档目录）、存档 ID 与范围
ibukirpg mcp --save ~/.config/ibukiRPG/ibukirpg.db --slot <存档ID> --scope player
ibukirpg mcp --save ./saves --scope npc:demo:character/mira
ibukirpg mcp --save ./saves --scope director
```

参数：

- `--save`：存档数据库文件 `ibukirpg.db`，或者包含它的目录。默认是 CLI 的存档目录：Linux 为 `~/.config/ibukiRPG/`，Windows 为 `%AppData%\ibukiRPG\`，macOS 为 `~/Library/Application Support/ibukiRPG/`。
- `--slot`：存档 ID。留空时使用最近更新的存档。存档 ID 可以在 CLI 里用 `/saves` 查看。
- `--scope`：`player` / `director` / `npc:<角色ID>`。
- `--packs`：导入故事包所在的目录，默认是存档目录下的 `packs/`。存档用的是导入的故事包时，需要它才能载入。

服务器使用 stdio 传输：每行一条 JSON-RPC 2.0 消息。stdout 只输出协议消息，日志写到 stderr。
每次工具调用都会重新以只读方式载入存档，所以可以一边在 CLI 或桌面端玩，一边在 MCP 客户端里查。
已实现的方法有 `initialize`、`notifications/initialized`、`ping`、`tools/list` 和 `tools/call`。`resources/list` 与 `prompts/list` 返回空列表。

> 说明：服务器不会写任何游戏数据。第一次用新版本打开旧存档时，数据库会像游戏本身一样自动补建空的 `memory` 表（记忆摘要表），这一步不影响存档内容。

## 在 MCP 客户端里配置

以下示例里的 `ibukirpg` 要换成可执行文件的完整路径，例如 `make build` 生成的 `bin/ibukirpg`。

### Claude Desktop

编辑 `claude_desktop_config.json`。macOS 上位于 `~/Library/Application Support/Claude/`，Windows 上位于 `%APPDATA%\Claude\`。

```json
{
  "mcpServers": {
    "ibukirpg": {
      "command": "/path/to/ibukirpg",
      "args": ["mcp", "--save", "/home/me/.config/ibukiRPG", "--scope", "player"]
    }
  }
}
```

### Cursor

项目级配置写在 `.cursor/mcp.json`，全局配置写在 `~/.cursor/mcp.json`：

```json
{
  "mcpServers": {
    "ibukirpg": {
      "command": "/path/to/ibukirpg",
      "args": ["mcp", "--scope", "director"]
    }
  }
}
```

### VS Code（Copilot Chat 等）

`.vscode/mcp.json`：

```json
{
  "servers": {
    "ibukirpg": {
      "type": "stdio",
      "command": "/path/to/ibukirpg",
      "args": ["mcp", "--save", "${workspaceFolder}/saves"]
    }
  }
}
```

### 其它客户端 / 手动测试

凡是支持 stdio MCP 服务器的客户端，都填入“命令 + 参数”即可。也可以直接用管道手动测试：

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"pack_search","arguments":{"query":"钟楼"}}}' \
  | ibukirpg mcp --save ./saves
```

## 游戏内的检索循环（供故事包作者参考）

- **何时触发**：
  - **叙述者**：以下任一情况会先检索再叙述：处于自由推演或沙盒模式；玩家输入里出现了当前上下文中没有的故事包名词；已经有记忆摘要且玩家在回忆早期的事（“上次”“之前”“还记得”……）。其它回合不检索，提示词保持不变。
  - **导演**：在自由推演模式下提案前总会检索。
- **怎么检索**：
  - 支持函数调用的模型会自己调用工具。默认预算是最多 3 轮、6 次调用，工具结果合计 6000 字。超出预算后，模型只能根据已有信息作答。
  - 本地模型（`local` / `llamacpp`，旧配置 `mediapipe`）不支持函数调用；服务端以 400/404/422 拒绝 `tools` 参数时也一样。这两种情况会自动改成关键词预检索：把最相关的资料作为 `[RETRIEVED]` 段放进上下文。
- **提示词**：检索时，系统提示词会加入 `[TOOLS]` 和 `[RETRIEVAL_POLICY]` 段，大意是：信息不足时先查再写；不与 `[IMMUTABLE_FACTS]` 和正史矛盾；宁可少写也不猜；遵守范围，不泄露秘密。故事包可以通过 `content.prompts` 追加或替换这些段落，格式见 [story-pack-format.md](story-pack-format.md)（“提示词段落补丁”一节），示例见 `packages/brass/prompts/retrieval.yaml`。
- **记忆**：
  - 记忆 Agent 在每回合结束后于后台运行，不占用关键路径。
    - **滚动摘要**：最近 4 个回合保留原文，更早的回合每 6 个压成一条滚动摘要。
    - **长期记忆**：滚动摘要超过 4 条时，把较早的几条再压成一条长期记忆。
    - **NPC 摘要**：某个 NPC 的记忆超过 8 条时，为它生成一条摘要。
  - 每条摘要都会注明压缩掉了什么（例如“逐字对话、环境描写、检定数值——可用 memory.search 查询”）。叙述者会看到 `[STORY_SO_FAR]` 段，因此知道细节需要去查。
  - 摘要保存在存档里，读档后仍然有效，也可以用 `memory_search` 检索。
