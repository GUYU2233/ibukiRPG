package com.guyu2233.ibukirpg.app.ui.game

import androidx.lifecycle.SavedStateHandle
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.guyu2233.ibukirpg.app.data.CardV1
import com.guyu2233.ibukirpg.app.data.CardsV1
import com.guyu2233.ibukirpg.app.data.CharacterV1
import com.guyu2233.ibukirpg.app.data.CodexV1
import com.guyu2233.ibukirpg.app.data.MechCardV1
import com.guyu2233.ibukirpg.app.data.MechsV1
import com.guyu2233.ibukirpg.app.data.NoticeV1
import com.guyu2233.ibukirpg.app.data.RelationsV1
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.EntryV1
import com.guyu2233.ibukirpg.app.data.GameBundle
import com.guyu2233.ibukirpg.app.data.InventoryV1
import com.guyu2233.ibukirpg.app.data.JournalEntryV1
import com.guyu2233.ibukirpg.app.data.NPCV1
import com.guyu2233.ibukirpg.app.data.QuickActionV1
import com.guyu2233.ibukirpg.app.data.SceneV1
import com.guyu2233.ibukirpg.app.data.SuggestionV1
import com.guyu2233.ibukirpg.app.data.TurnV1
import com.guyu2233.ibukirpg.app.data.BranchV1
import com.guyu2233.ibukirpg.app.data.CheckpointV1
import com.guyu2233.ibukirpg.app.data.PromptSettingsV1
import com.guyu2233.ibukirpg.app.data.TimelineV1
import com.guyu2233.ibukirpg.app.data.WorldChangeV1
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.util.UUID

/** 正在处理中的一次提交（界面上显示为玩家气泡 + 进度）。 */
data class Pending(val commandId: String, val label: String, val text: String?, val action: QuickActionV1?)

data class GameState(
    val loading: Boolean = true,
    val fatal: String? = null,
    val scene: SceneV1 = SceneV1(),
    val entries: List<EntryV1> = emptyList(),
    val suggestions: List<SuggestionV1> = emptyList(),
    val pending: Pending? = null,
    val streaming: String = "",
    val error: String? = null,
    /** 本次会话中新到达的叙事（用于逐字显示动画）。 */
    val freshIds: Set<Long> = emptySet(),
    /** 待显示的提示（升级、获得卡片、机甲状态变化…），显示后清空。 */
    val notices: List<NoticeV1> = emptyList(),
    /** 每次回合提交后递增，用来刷新已打开的机甲卡。 */
    val version: Int = 0,
    /** 内存里只保留最近一段记录；更早的可以按需从存档分页读取。 */
    val hasEarlier: Boolean = false,
    val loadingEarlier: Boolean = false,
)

/** 聊天记录的内存窗口（纯函数，便于单元测试）。 */
object TranscriptWindow {
    /** 回合追加后内存里最多保留的条目数。 */
    const val MAX_ENTRIES = 400
    /** 向上翻页时允许的最大条目数（超过后不再加载更早的记录）。 */
    const val MAX_WITH_EARLIER = 1200
    const val PAGE = 100

    /** 追加新条目并去重；超过上限时丢弃最早的条目。返回 (列表, 是否丢弃过)。 */
    fun append(current: List<EntryV1>, incoming: List<EntryV1>, max: Int = MAX_ENTRIES): Pair<List<EntryV1>, Boolean> {
        val known = current.mapTo(HashSet()) { it.id }
        val fresh = incoming.filter { it.id == 0L || it.id !in known }
        val all = current + fresh
        return if (all.size > max) all.takeLast(max) to true else all to false
    }

    /** 在头部拼接更早的一页（按 id 去重，保持时间顺序）。 */
    fun prepend(current: List<EntryV1>, older: List<EntryV1>): List<EntryV1> {
        val known = current.mapTo(HashSet()) { it.id }
        return older.filter { it.id !in known }.sortedBy { it.id } + current
    }

    /** 最早一条有 id 的记录（翻页游标）。 */
    fun cursor(entries: List<EntryV1>): Long? = entries.firstOrNull { it.id > 0L }?.id
}

data class PanelsState(
    val loading: Boolean = false,
    val character: CharacterV1? = null,
    val inventory: InventoryV1? = null,
    val npcs: List<NPCV1> = emptyList(),
    val journal: List<JournalEntryV1> = emptyList(),
    val codex: CodexV1? = null,
    val relations: RelationsV1? = null,
    val cards: CardsV1? = null,
    val mechs: MechsV1? = null,
)

class GameViewModel(
    private val engine: Engine,
    /** 进程被系统回收后恢复：输入框草稿、正在进行 / 失败的提交（command_id 不变，引擎按它去重）。 */
    private val saved: SavedStateHandle = SavedStateHandle(),
) : ViewModel() {
    private val _state = MutableStateFlow(GameState())
    val state: StateFlow<GameState> = _state.asStateFlow()

    private val _panels = MutableStateFlow(PanelsState())
    val panels: StateFlow<PanelsState> = _panels.asStateFlow()

    /** 0.2.0：世界面板（按标签页加载）、时间线（非 null 时显示全屏时间线）、提示灵敏度（非 null 时显示设置对话框）。 */
    private val _world = MutableStateFlow(WorldState())
    val world: StateFlow<WorldState> = _world.asStateFlow()
    private val _timeline = MutableStateFlow<TimelineV1?>(null)
    val timeline: StateFlow<TimelineV1?> = _timeline.asStateFlow()
    private val _prompts = MutableStateFlow<PromptSettingsV1?>(null)
    val prompts: StateFlow<PromptSettingsV1?> = _prompts.asStateFlow()

    /** 输入框内容放在 SavedStateHandle 中：旋转屏幕、出错、进程被回收都不会丢。 */
    val input: StateFlow<String> = saved.getStateFlow(KEY_INPUT, "")
    private var inputValue: String
        get() = input.value
        set(v) { saved[KEY_INPUT] = v }

    /** 上次失败的提交：玩家原样重试时复用 command_id，保证不会重复执行。 */
    private var lastFailed: Pending? = null

    init {
        // 上次进程在回合进行中被系统结束：把那条文字放回输入框，并记住 command_id，
        // 玩家点发送时原样重试；若引擎已经处理完那一回合，会直接返回同一结果而不会重复执行。
        val interruptedId = saved.get<String>(KEY_PENDING_ID)
        val interruptedText = saved.get<String>(KEY_PENDING_TEXT)
        if (interruptedId != null && interruptedText != null) {
            lastFailed = Pending(interruptedId, interruptedText, interruptedText, null)
            if (inputValue.isBlank()) inputValue = interruptedText
        }
        viewModelScope.launch {
            engine.events.collect { e ->
                val p = _state.value.pending ?: return@collect
                if (e.commandId != p.commandId) return@collect
                when (e.type) {
                    "turn_started" -> _state.update { it.copy(streaming = "") }
                    "narration_delta" -> _state.update { it.copy(streaming = it.streaming + (e.text ?: "")) }
                }
            }
        }
        load()
    }

    fun load() {
        _state.update { it.copy(loading = true, fatal = null) }
        viewModelScope.launch {
            runCatching {
                try {
                    engine.bundle()
                } catch (e: Exception) {
                    // 进程被系统回收后重建：自动读取最近的存档。
                    val latest = engine.listSaves().firstOrNull() ?: throw e
                    engine.loadGame(latest.id)
                }
            }.onSuccess { apply(it) }
                .onFailure { e -> _state.update { it.copy(loading = false, fatal = e.message) } }
        }
    }

    private fun apply(b: GameBundle) {
        val (entries, trimmed) = TranscriptWindow.append(emptyList(), b.transcript)
        _state.update {
            it.copy(
                loading = false, scene = b.scene, entries = entries, suggestions = b.suggestions, fatal = null,
                // get_bundle 只返回最近 300 条：满页说明可能还有更早的
                hasEarlier = trimmed || b.transcript.size >= BUNDLE_PAGE,
            )
        }
    }

    /** 从存档读取更早的一页记录（聊天记录很长时不一次性放进内存）。 */
    fun loadEarlier() {
        val s0 = _state.value
        if (s0.loadingEarlier || !s0.hasEarlier) return
        val before = TranscriptWindow.cursor(s0.entries) ?: return
        _state.update { it.copy(loadingEarlier = true) }
        viewModelScope.launch {
            runCatching { engine.transcript(TranscriptWindow.PAGE, before) }
                .onSuccess { older ->
                    _state.update { s ->
                        val merged = TranscriptWindow.prepend(s.entries, older)
                        s.copy(
                            entries = merged, loadingEarlier = false,
                            hasEarlier = older.size >= TranscriptWindow.PAGE && merged.size < TranscriptWindow.MAX_WITH_EARLIER,
                        )
                    }
                }
                .onFailure { e -> _state.update { it.copy(loadingEarlier = false, error = e.message) } }
        }
    }

    fun onInput(v: String) { inputValue = v }

    fun send() {
        val text = inputValue.trim()
        if (text.isEmpty() || _state.value.pending != null) return
        val reuse = lastFailed?.takeIf { it.text == text }
        val p = Pending(reuse?.commandId ?: UUID.randomUUID().toString(), text, text, null)
        inputValue = ""
        run(p)
    }

    fun quick(action: QuickActionV1, label: String) {
        if (_state.value.pending != null) return
        if (action.kind == "text" && !action.text.isNullOrBlank()) {
            inputValue = action.text
            send()
            return
        }
        val reuse = lastFailed?.takeIf { it.action == action }
        run(Pending(reuse?.commandId ?: UUID.randomUUID().toString(), action.label ?: label, null, action))
    }

    private fun run(p: Pending) {
        _state.update { it.copy(pending = p, streaming = "", error = null) }
        if (p.text != null) {
            saved[KEY_PENDING_ID] = p.commandId
            saved[KEY_PENDING_TEXT] = p.text
        }
        viewModelScope.launch {
            runCatching {
                if (p.action != null) engine.quickAction(p.commandId, p.action) else engine.submitText(p.commandId, p.text.orEmpty())
            }.onSuccess { turn ->
                lastFailed = null
                saved.remove<String>(KEY_PENDING_ID)
                saved.remove<String>(KEY_PENDING_TEXT)
                applyTurn(turn)
            }.onFailure { e ->
                lastFailed = p
                // 失败不丢输入：把文字放回输入框（若玩家没有重新输入）。
                if (p.text != null && inputValue.isBlank()) inputValue = p.text
                _state.update { it.copy(pending = null, streaming = "", error = e.message ?: "未知错误") }
            }
        }
    }

    private fun applyTurn(t: TurnV1) {
        _state.update { s ->
            val known = s.entries.mapTo(HashSet()) { it.id }
            val fresh = t.entries.filter { it.id == 0L || it.id !in known }
            val (entries, trimmed) = TranscriptWindow.append(s.entries, fresh)
            s.copy(
                entries = entries,
                hasEarlier = s.hasEarlier || trimmed,
                scene = t.scene,
                suggestions = t.suggestions,
                pending = null,
                streaming = "",
                freshIds = s.freshIds + fresh.filter { it.kind == "narration" }.map { it.id },
                notices = s.notices + t.notices,
                version = s.version + 1,
            )
        }
        if (_panels.value.character != null) refreshPanels()
        refreshWorld()
    }

    // ---------- 0.2.0：世界面板 / 偏离提示 / 回溯与分支 / 检查点 ----------

    fun loadWorldTab(tab: String, force: Boolean = false) {
        if (!force && _world.value.tabs.containsKey(tab)) return
        _world.update { it.copy(loading = tab) }
        viewModelScope.launch {
            runCatching { engine.worldPanel(tab) }
                .onSuccess { p -> _world.update { it.copy(tabs = it.tabs + (tab to p), loading = null) } }
                .onFailure { e -> _world.update { it.copy(loading = null) }; _state.update { it.copy(error = e.message) } }
        }
    }

    /** 回合推进后：已经打开过的标签页重新加载。 */
    private fun refreshWorld() {
        val open = _world.value.tabs.keys.toList()
        _world.value = WorldState()
        open.forEach { loadWorldTab(it) }
    }

    fun revertChange(c: WorldChangeV1) = bundleOp { engine.revertChange(c.id); engine.bundle() }

    fun resolveDecision(accept: Boolean, notifyOnly: Boolean) {
        val d = _state.value.scene.decision ?: return
        bundleOp { engine.resolveDecision(d.id, if (accept) "accept" else "rollback", notifyOnly) }
    }

    fun openTimeline() {
        viewModelScope.launch {
            runCatching { engine.timeline() }
                .onSuccess { _timeline.value = it }
                .onFailure { e -> _state.update { it.copy(error = e.message) } }
        }
    }

    fun closeTimeline() { _timeline.value = null }

    fun rollbackTo(turn: Int) = bundleOp(closeTimeline = true) { engine.rollbackTo(turn) }
    fun cancelRollback() = bundleOp { engine.cancelRollback() }
    fun switchBranch(b: BranchV1) = bundleOp(closeTimeline = true) { engine.switchBranch(b.id) }
    fun restoreCheckpoint(c: CheckpointV1) = bundleOp(closeTimeline = true) { engine.restoreCheckpoint(c.id) }

    fun createCheckpoint(name: String, turn: Int = 0) {
        viewModelScope.launch {
            runCatching { engine.createCheckpoint(name, turn); engine.timeline() }
                .onSuccess { t -> if (_timeline.value != null) _timeline.value = t; _state.update { it.copy(notices = it.notices + NoticeV1(text = "已创建检查点「$name」")) } }
                .onFailure { e -> _state.update { it.copy(error = e.message) } }
        }
    }

    fun wait(target: String, label: String) {
        if (_state.value.pending != null) return
        run(Pending(UUID.randomUUID().toString(), label, null, QuickActionV1(kind = "wait", target = target, label = label)))
    }

    fun openPrompts() {
        viewModelScope.launch { _prompts.value = runCatching { engine.promptSettings() }.getOrDefault(PromptSettingsV1()) }
    }

    fun savePrompts(p: PromptSettingsV1?) {
        _prompts.value = null
        if (p == null) return
        viewModelScope.launch {
            runCatching { engine.setPromptSettings(p); onPromptsSaved(p) }
                .onFailure { e -> _state.update { it.copy(error = e.message) } }
        }
    }

    /** 由宿主设置：把提示灵敏度持久化到 SettingsStore（下次启动时随 configure_ai 一起下发）。 */
    var onPromptsSaved: suspend (PromptSettingsV1) -> Unit = {}

    /** 返回整包数据的操作（回溯 / 切换分支 / 偏离提示…）：整体替换界面状态。 */
    private fun bundleOp(closeTimeline: Boolean = false, op: suspend () -> GameBundle) {
        if (_state.value.pending != null) return
        _state.update { it.copy(pending = Pending(UUID.randomUUID().toString(), "", null, null)) }
        viewModelScope.launch {
            runCatching { op() }
                .onSuccess { b ->
                    if (closeTimeline) _timeline.value = null
                    apply(b)
                    _state.update { it.copy(pending = null, version = it.version + 1) }
                    refreshWorld()
                    if (_timeline.value != null) openTimeline()
                }
                .onFailure { e -> _state.update { it.copy(pending = null, error = e.message) } }
        }
    }

    fun consumeError() = _state.update { it.copy(error = null) }

    fun consumeNotices() = _state.update { it.copy(notices = emptyList()) }

    private val portraitCache = HashMap<String, ByteArray?>()

    /** 立绘（带内存缓存）。没有图片时返回 null，界面显示占位头像。 */
    suspend fun portrait(id: String): ByteArray? {
        if (portraitCache.containsKey(id)) return portraitCache[id]
        val bytes = runCatching { engine.portrait(id) }.getOrNull()
            ?.takeIf { it.base64.isNotBlank() }
            ?.let { android.util.Base64.decode(it.base64, android.util.Base64.DEFAULT) }
        portraitCache[id] = bytes
        return bytes
    }

    suspend fun card(id: String): CardV1? = runCatching { engine.card(id) }.getOrNull()

    suspend fun mech(id: String): MechCardV1? = runCatching { engine.mech(id) }.getOrNull()

    fun refreshPanels() {
        _panels.update { it.copy(loading = true) }
        viewModelScope.launch {
            runCatching {
                PanelsState(
                    loading = false,
                    character = engine.character(),
                    inventory = engine.inventory(),
                    npcs = engine.npcs(),
                    journal = engine.journal(),
                ).let { base ->
                    if (!_state.value.scene.hasRpg) base else base.copy(
                        codex = runCatching { engine.codex() }.getOrNull(),
                        relations = runCatching { engine.relations() }.getOrNull(),
                        cards = runCatching { engine.cards() }.getOrNull(),
                        mechs = runCatching { engine.mechs() }.getOrNull(),
                    )
                }
            }.onSuccess { p -> _panels.value = p }
                .onFailure { e -> _panels.update { it.copy(loading = false) }; _state.update { it.copy(error = e.message) } }
        }
    }

    private companion object {
        const val KEY_INPUT = "input"
        const val KEY_PENDING_ID = "pending_command_id"
        const val KEY_PENDING_TEXT = "pending_text"
        const val BUNDLE_PAGE = 300
    }
}
