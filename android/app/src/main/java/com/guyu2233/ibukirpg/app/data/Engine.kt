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
    suspend fun newGame(playerName: String): GameBundle = call(
        "new_game", buildJsonObject { put("player_name", playerName); put("save_name", "") }, decode = decoder(),
    )
    suspend fun loadGame(slotId: String): GameBundle =
        call("load_game", buildJsonObject { put("slot_id", slotId) }, decode = decoder())
    suspend fun bundle(): GameBundle = call("get_bundle", decode = decoder())
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
    suspend fun presets(): List<PresetV1> = call("presets", decode = decoder())
    suspend fun configureAI(cfg: AIConfig): AIStatusV1 = call("configure_ai", payload(cfg), decode = decoder())
    suspend fun aiStatus(): AIStatusV1 = call("ai_status", decode = decoder())
    suspend fun testAI(cfg: AIConfig): TestAIResult = call("test_ai", payload(cfg), decode = decoder())

    @Suppress("unused")
    private fun JsonObject.str(k: String) = this[k]?.jsonPrimitive?.contentOrNull
}
