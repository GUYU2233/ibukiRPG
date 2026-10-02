# MCP 适配器：检索工具与（门控的）世界写入

ibukiRPG 的 AI Agent（叙述者、导演 / 自由推演规划、NPC）在信息不足时会**主动查资料**：
上下文被压缩成摘要之后、世界被改写之后、或者玩家提到了当前上下文里没有的人物和名词时。
查资料用的是一组**只读检索工具**。这组工具有两种暴露方式：

| 使用方 | 通道 | 说明 |
| --- | --- | --- |
| 游戏内 AI Agent | 进程内工具网关（`internal/agent/tools`） | OpenAI 兼容函数调用；模型不支持时退化为关键词预检索 |
| Android 客户端 | 移动端 JSON API：`list_tools` / `call_tool` | 同一套工具，进程内调用（手机上不跑 MCP） |
| 桌面 / CLI 上的 MCP 客户端 | `ibukirpg mcp`（stdio） | 本文重点 |

检索工具都是**只读**的：不会提交输入、不会执行动作、不会存档或读档，也不会改动任何游戏状态。
MCP 服务器默认只暴露读取工具（架构文档第 48 节：MCP 只是 Command / Query API 之上的一层适配器）。
0.2.0 起，故事包作者可以用 `--allow-write` 打开**世界写入工具**（见下文“世界写入（门控）”）：它们和游戏内 AI 的修改走同一个校验网关与事件日志，不存在第二条写入路径。

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
| `story_get_state()` | 回合、所在地、当前目标、进行中的剧情事件 | player / director |
| `world_get_location(id?)` | 地点描述、场景事实、出口、在场人物（省略 id 为玩家当前位置） | 全部 |
| `rules_get_action(id)` | 动作规则：说明、关键词、检定技能、耗时 | 全部 |

| `world_change_log(query?, since_turn?)` | 世界变更日志（谁、何时、为什么改了什么；隐藏真相相关的只给脱敏摘要） | 全部 |
| `world_revert_plan(change_id)` | 撤销前检查（rc1）：之后依赖这条变更的全部变更（撤销顺序），以及能否只撤销这一条。不修改存档 | director / author |
| `timeline_get()` | 世界事件时间线（玩家知道的：公开日程、传闻、亲眼所见） | 全部 |
| `knowledge_get(entity)` | 一个实体对玩家可见的字段（未解锁的显示“未知”） | 全部 |

结果是 JSON 文本，大小有上限（每次最多 8 条结果、单字段 280 字、总计约 6 KB），同样的输入总是得到同样的输出。

## 范围（scope）：谁能看到什么

| 范围 | 对应 Agent | 可见内容 |
| --- | --- | --- |
| `player`（默认，别名 `narrator`） | 叙述者 | 玩家已知的内容（PlayerScope）：按知识层逐字段解锁的内容；看不到任何隐藏真相 |
| `npc:<角色ID>` | NPC Agent | 该 NPC 知道的内容（NPCScope）：公开资料 + 自己的来历与秘密 + 自己的记忆；看不到别人的秘密和记忆，看不到剧本节点 |
| `director`（别名 `planner`） | 导演 / 世界模拟 / 审查 | 完整设定：全部隐藏真相、隐藏的机甲规格、全部剧情节点 |
| `author` | 故事包作者调试 | 读权限同 `director`；配合 `--allow-write` 可以写入世界 |

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
- `--scope`：`player` / `director` / `author` / `npc:<角色ID>`。
- `--allow-write`：打开世界写入工具。只对 `director` / `author` 范围有效；`player` 与 `npc:*` 范围永远只读。
- `--packs`：导入故事包所在的目录，默认是存档目录下的 `packs/`。存档用的是导入的故事包时，需要它才能载入。

服务器使用 stdio 传输：每行一条 JSON-RPC 2.0 消息。stdout 只输出协议消息，日志写到 stderr。
每次工具调用都会重新载入存档，所以可以一边在 CLI 或桌面端玩，一边在 MCP 客户端里查。
已实现的方法有 `initialize`、`notifications/initialized`、`ping`、`tools/list` 和 `tools/call`。`resources/list` 与 `prompts/list` 返回空列表。

## 世界写入（门控）

```bash
ibukirpg mcp --save ./saves --scope author --allow-write
```

写入需要同时满足三个条件：`--allow-write`、范围为 `director` / `author`、存档**没有被游戏占用**。CLI 或桌面游戏打开存档时会持有存档锁（`<存档目录>/locks/<存档ID>.lock`，每 10 秒刷新一次，30 秒过期）；这时写入会返回“存档正在被游戏使用，请先退出游戏或只读查询”。读取工具不受影响。

| 工具 | 说明 |
| --- | --- |
| `world_preview_change(changes[])` | 预览一组修改：返回字段级差异、影响分、被拒绝项与原因，以及 `preview_token`。**不会修改存档** |
| `world_apply_change(preview_token, accept_impact?)` | 只接受预览过的提案。预览之后存档发生了变化（例如游戏又走了一回合）则失败，需要重新预览；影响分超过 50 时必须带 `accept_impact: true` |
| `world_revert_change(change_id, mode?)` | 撤销一条世界变更。之后有依赖它的变更时需要 `mode`：`chain` = 连同依赖一起撤销；`single` = 只撤销这一条（被后续变更覆盖的值保持不变；新建 / 退场等不能单独撤销）。先用 `world_revert_plan` 查看依赖链 |
| `checkpoint_create(name?)` | 在当前回合新建手动检查点 |

变更格式与游戏内 AI 的 WORLD 段相同，例如：

```json
{"changes":[{"op":"patch","target":"brass:location/north_dock","path":"fields.description","value":"三号仓只剩焦黑的木桩。","reason":"作者调试"}]}
```

`op` 可以是 `patch` / `create` / `retire` / `restore` / `link` / `unlink`。所有写入都经过与游戏内相同的校验（实体存在、`locked` 路径、重要度上限、玩家资源单回合上限……），以“外部工具”来源记入世界变更日志，可以在 App 的“世界面板 → 日志”里看到并撤销。MCP 写入不会弹出偏离提示（没有玩家界面）。Android 不运行 MCP 服务。

**只读打开（v0.2.0-rc1）**：没有 `--allow-write`（或范围不是 director / author）时，服务器以只读方式打开存档数据库（SQLite `mode=ro`）：不做迁移、不加存档锁、不写任何数据；读档时发现的叙事模板问题只在内存里修复。即使开了 `--allow-write`，读取工具也走只读加载，只有写入工具才以读写方式打开。

**外部修改提醒（v0.2.0-rc1）**：MCP 写入的变更以“外部工具”来源记入世界日志。玩家下次打开 App 或读档时会看到“外部工具修改了 N 项设定”的横幅（只计未撤销的），“查看日志”打开世界面板日志页并只显示外部工具的修改；CLI 载入时同样提示（`/changes 外部` 筛选）。

> 游戏内的 AI 另有一组**写入工具**（`world_propose_change` / `entity_generate` / `rules_power_budget`，v0.2.0-rc1），只在游戏进程内、写入范围下列出，MCP 永远拿不到它们；MCP 写入始终走上面的 preview → apply 两步。

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
