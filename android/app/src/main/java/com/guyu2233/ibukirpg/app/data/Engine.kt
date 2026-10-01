package com.guyu2233.ibukirpg.app.data

import com.guyu2233.ibukirpg.mobile.EventSink
import com.guyu2233.ibukirpg.mobile.Mobile
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNamingStrategy
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.decodeFromJsonElement
import kotlinx.serialization.json.encodeToJsonElement
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.boolean
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonPrimitive

/** 引擎调用失败（错误信息已是给玩家看的中文）。 */
class EngineException(message: String) : Exception(message)

/**
 * Go 引擎（gomobile AAR）的 Kotlin 包装：版本化 JSON 请求 / 响应（API v1）。
 *
 * 所有调用都在 Dispatchers.IO 上串行执行；叙事增量通过 [events] 推送。
 */
class Engine(private val dataDir: String) {
    @OptIn(ExperimentalSerializationApi::class)
    val json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        coerceInputValues = true
        encodeDefaults = true
        namingStrategy = JsonNamingStrategy.SnakeCase
    }

    private val mutex = Mutex()
    private val _events = MutableSharedFlow<StreamEventV1>(extraBufferCapacity = 256)
    val events: SharedFlow<StreamEventV1> = _events

    @Volatile private var initialized = false

    val version: String get() = runCatching { Mobile.version() }.getOrDefault("?")

    private val sink = object : EventSink {
        override fun onEvent(eventJSON: String?) {
            if (eventJSON == null) return
            runCatching { json.decodeFromString(StreamEventV1.serializer(), eventJSON) }
                .onSuccess { _events.tryEmit(it) }
        }
    }

    private suspend fun ensureInit() {
        if (initialized) return
        Mobile.setEventSink(sink)
        raw("init", buildJsonObject { put("data_dir", dataDir) }, null)
        initialized = true
    }

    private fun raw(type: String, payload: JsonElement?, commandId: String?): JsonElement {
        val req = buildJsonObject {
            put("version", "v1")
            put("type", type)
            if (commandId != null) put("command_id", commandId)
            if (payload != null && payload !is JsonNull) put("payload", payload)
        }
        val out = Mobile.handle(req.toString())
        val resp = json.parseToJsonElement(out).jsonObject
        val ok = resp["ok"]?.jsonPrimitive?.boolean ?: false
        if (!ok) {
            throw EngineException(resp["error"]?.jsonPrimitive?.contentOrNull ?: "未知错误")
        }
        return resp["data"] ?: JsonNull
    }

    /** 发送请求并解码 data。 */
    suspend fun <T> call(
        type: String,
        payload: JsonElement? = null,
        commandId: String? = null,
        decode: (JsonElement) -> T,
    ): T = withContext(Dispatchers.IO) {
        mutex.withLock {
            ensureInit()
            decode(raw(type, payload, commandId))
        }
    }

    inline fun <reified P> payload(p: P): JsonElement = json.encodeToJsonElement(p)
    inline fun <reified T> decoder(): (JsonElement) -> T = { json.decodeFromJsonElement<T>(it) }

    // ---------- 便捷方法 ----------

    suspend fun listSaves(): List<SlotV1> = call("list_saves", decode = decoder())
    suspend fun newGame(playerName: String, packId: String = ""): GameBundle = call(
        "new_game", buildJsonObject { put("player_name", playerName); put("save_name", ""); put("pack_id", packId) }, decode = decoder(),
    )
    suspend fun listPacks(): List<PackV1> = call("list_packs", decode = decoder())
    /** 导入故事包 .zip（path 为应用可读的本地文件）；校验失败时抛出带中文原因的 [EngineException]。 */
    suspend fun importPack(path: String): ImportResultV1 =
        call("import_pack", buildJsonObject { put("path", path) }, decode = decoder())
    suspend fun deletePack(id: String) { call("delete_pack", buildJsonObject { put("id", id) }) { } }
    suspend fun loadGame(slotId: String): GameBundle =
        call("load_game", buildJsonObject { put("slot_id", slotId) }, decode = decoder())
    suspend fun bundle(): GameBundle = call("get_bundle", decode = decoder())
    /** 分页读取更早的记录：id < beforeId 的最近 limit 条（时间顺序）。 */
    suspend fun transcript(limit: Int, beforeId: Long): List<EntryV1> =
        call("get_transcript", buildJsonObject { put("limit", limit); put("before_id", beforeId) }, decode = decoder())
    suspend fun deleteSave(slotId: String) { call("delete_save", buildJsonObject { put("slot_id", slotId) }) { } }
    suspend fun copySave(slotId: String, name: String) {
        call("copy_save", buildJsonObject { put("slot_id", slotId); put("name", name) }) { }
    }
    suspend fun renameSave(slotId: String, name: String) {
        call("rename_save", buildJsonObject { put("slot_id", slotId); put("name", name) }) { }
    }
    suspend fun submitText(commandId: String, text: String): TurnV1 =
        call("submit_text", buildJsonObject { put("text", text) }, commandId, decoder())
    suspend fun quickAction(commandId: String, action: QuickActionV1): TurnV1 =
        call("quick_action", payload(action), commandId, decoder())
    suspend fun character(): CharacterV1 = call("get_character", decode = decoder())
    suspend fun inventory(): InventoryV1 = call("get_inventory", decode = decoder())
    suspend fun npcs(): List<NPCV1> = call("get_npcs", decode = decoder())
    suspend fun journal(): List<JournalEntryV1> = call("get_journal", decode = decoder())
    suspend fun codex(): CodexV1 = call("get_codex", decode = decoder())
    suspend fun card(id: String): CardV1 = call("get_card", buildJsonObject { put("id", id) }, decode = decoder())
    suspend fun relations(): RelationsV1 = call("get_relations", decode = decoder())
    suspend fun cards(): CardsV1 = call("get_cards", decode = decoder())
    suspend fun mechs(): MechsV1 = call("get_mechs", decode = decoder())
    suspend fun mech(id: String): MechCardV1 = call("get_mech", buildJsonObject { put("id", id) }, decode = decoder())
    /** 角色 / 机甲立绘（base64）；没有时返回空的 [PortraitV1]。 */
    suspend fun portrait(id: String): PortraitV1 = call("get_portrait", buildJsonObject { put("id", id) }, decode = decoder())
    suspend fun presets(): List<PresetV1> = call("presets", decode = decoder())
    suspend fun configureAI(cfg: AIConfig): AIStatusV1 = call("configure_ai", payload(cfg), decode = decoder())
    suspend fun aiStatus(): AIStatusV1 = call("ai_status", decode = decoder())
    suspend fun testAI(cfg: AIConfig): TestAIResult = call("test_ai", payload(cfg), decode = decoder())

    // ---------- 0.2.0：开放世界 / 回溯 / 设置 / 存档交换 ----------

    /** init 的结果：legacy_notice 非空表示数据目录里还有 0.1.x 的旧存档（无法继续）。 */
    suspend fun legacyNotice(): String? = call("init", buildJsonObject { put("data_dir", dataDir) }) {
        (it as? JsonObject)?.get("legacy_notice")?.jsonPrimitive?.contentOrNull
    }
    suspend fun deleteLegacySaves() { call("delete_legacy_saves", buildJsonObject { put("data_dir", dataDir) }) { } }
    suspend fun configureProviders(cfg: AIConfigV2): AIStatusV1 = call("configure_ai", payload(cfg), decode = decoder())
    suspend fun tasks(): TasksV1 = call("get_tasks", decode = decoder())
    suspend fun setPromptSettings(p: PromptSettingsV1) { call("set_prompt_settings", payload(p)) { } }
    suspend fun promptSettings(): PromptSettingsV1 = call("get_prompt_settings", decode = decoder())
    suspend fun resolveDecision(id: String, action: String, notifyOnly: Boolean): GameBundle = call(
        "resolve_decision", buildJsonObject { put("id", id); put("action", action); put("notify_only", notifyOnly) }, decode = decoder(),
    )
    suspend fun timeline(): TimelineV1 = call("get_timeline", decode = decoder())
    suspend fun rollbackTo(turn: Int): GameBundle = call("rollback_to", buildJsonObject { put("turn", turn) }, decode = decoder())
    suspend fun cancelRollback(): GameBundle = call("cancel_rollback", decode = decoder())
    suspend fun switchBranch(branch: String): GameBundle = call("switch_branch", buildJsonObject { put("branch", branch) }, decode = decoder())
    suspend fun renameBranch(branch: String, name: String) { call("rename_branch", buildJsonObject { put("branch", branch); put("name", name) }) { } }
    suspend fun deleteBranch(branch: String) { call("delete_branch", buildJsonObject { put("branch", branch) }) { } }
    suspend fun createCheckpoint(name: String, turn: Int = 0): CheckpointV1 =
        call("create_checkpoint", buildJsonObject { put("name", name); put("turn", turn) }, decode = decoder())
    suspend fun restoreCheckpoint(id: String): GameBundle = call("restore_checkpoint", buildJsonObject { put("id", id) }, decode = decoder())
    suspend fun deleteCheckpoint(id: String) { call("delete_checkpoint", buildJsonObject { put("id", id) }) { } }
    suspend fun worldPanel(tab: String): WorldPanelV1 = call("get_world_panel", buildJsonObject { put("tab", tab) }, decode = decoder())
    suspend fun worldChanges(query: String = "", limit: Int = 100): List<WorldChangeV1> =
        call("search_world_changes", buildJsonObject { put("query", query); put("limit", limit) }, decode = decoder())
    suspend fun revertChange(id: String): WorldLogV1 = call("revert_change", buildJsonObject { put("id", id) }, decode = decoder())
    /** 让 AI 按要求修改 / 新建卡片：返回预览，确认后 applyPreview。target 为空 = 新建。 */
    suspend fun requestEdit(target: String, instruction: String): ChangePreviewV1 =
        call("request_edit", buildJsonObject { put("target", target); put("instruction", instruction) }, decode = decoder())
    suspend fun applyPreview(token: String): WorldLogV1 = call("apply_preview", buildJsonObject { put("preview_token", token) }, decode = decoder())
    suspend fun runAudit(): AuditV1 = call("run_audit", decode = decoder())
    suspend fun resolveAuditSuggestion(id: String, action: String): WorldLogV1 =
        call("resolve_audit_suggestion", buildJsonObject { put("id", id); put("action", action) }, decode = decoder())
    suspend fun usage(turn: Int = 0): UsageV1 = call("get_usage", buildJsonObject { put("turn", turn) }, decode = decoder())
    suspend fun wait(commandId: String, target: String): TurnV1 = call("wait", buildJsonObject { put("target", target) }, commandId, decoder())
    /** 导出存档到 dir（应用缓存目录），再由界面通过 SAF 复制到玩家选择的位置。 */
    suspend fun exportSave(slotId: String, dir: String): ExportResultV1 =
        call("export_save", buildJsonObject { put("slot_id", slotId); put("dir", dir) }, decode = decoder())
    suspend fun inspectSave(path: String): SaveCheckV1 = call("inspect_save", buildJsonObject { put("path", path) }, decode = decoder())
    suspend fun importSave(path: String): ImportSaveResultV1 = call("import_save", buildJsonObject { put("path", path) }, decode = decoder())

    @Suppress("unused")
    private fun JsonObject.str(k: String) = this[k]?.jsonPrimitive?.contentOrNull
}
