package com.guyu2233.ibukirpg.app.ui.game

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.guyu2233.ibukirpg.app.data.CharacterV1
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
)

data class PanelsState(
    val loading: Boolean = false,
    val character: CharacterV1? = null,
    val inventory: InventoryV1? = null,
    val npcs: List<NPCV1> = emptyList(),
    val journal: List<JournalEntryV1> = emptyList(),
)

class GameViewModel(private val engine: Engine) : ViewModel() {
    private val _state = MutableStateFlow(GameState())
    val state: StateFlow<GameState> = _state.asStateFlow()

    private val _panels = MutableStateFlow(PanelsState())
    val panels: StateFlow<PanelsState> = _panels.asStateFlow()

    /** 输入框内容放在 ViewModel 中：旋转屏幕、出错都不会丢。 */
    private val _input = MutableStateFlow("")
    val input: StateFlow<String> = _input.asStateFlow()

    /** 上次失败的提交：玩家原样重试时复用 command_id，保证不会重复执行。 */
    private var lastFailed: Pending? = null

    init {
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
        _state.update { it.copy(loading = false, scene = b.scene, entries = b.transcript, suggestions = b.suggestions, fatal = null) }
    }

    fun onInput(v: String) { _input.value = v }

    fun send() {
        val text = _input.value.trim()
        if (text.isEmpty() || _state.value.pending != null) return
        val reuse = lastFailed?.takeIf { it.text == text }
        val p = Pending(reuse?.commandId ?: UUID.randomUUID().toString(), text, text, null)
        _input.value = ""
        run(p)
    }

    fun quick(action: QuickActionV1, label: String) {
        if (_state.value.pending != null) return
        if (action.kind == "text" && !action.text.isNullOrBlank()) {
            _input.value = action.text
            send()
            return
        }
        val reuse = lastFailed?.takeIf { it.action == action }
        run(Pending(reuse?.commandId ?: UUID.randomUUID().toString(), action.label ?: label, null, action))
    }

    private fun run(p: Pending) {
        _state.update { it.copy(pending = p, streaming = "", error = null) }
        viewModelScope.launch {
            runCatching {
                if (p.action != null) engine.quickAction(p.commandId, p.action) else engine.submitText(p.commandId, p.text.orEmpty())
            }.onSuccess { turn ->
                lastFailed = null
                applyTurn(turn)
            }.onFailure { e ->
                lastFailed = p
                // 失败不丢输入：把文字放回输入框（若玩家没有重新输入）。
                if (p.text != null && _input.value.isBlank()) _input.value = p.text
                _state.update { it.copy(pending = null, streaming = "", error = e.message ?: "未知错误") }
            }
        }
    }

    private fun applyTurn(t: TurnV1) {
        _state.update { s ->
            val known = s.entries.map { it.id }.toHashSet()
            val fresh = t.entries.filter { it.id == 0L || it.id !in known }
            s.copy(
                entries = s.entries + fresh,
                scene = t.scene,
                suggestions = t.suggestions,
                pending = null,
                streaming = "",
                freshIds = s.freshIds + fresh.filter { it.kind == "narration" }.map { it.id },
            )
        }
        if (_panels.value.character != null) refreshPanels()
    }

    fun consumeError() = _state.update { it.copy(error = null) }

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
                )
            }.onSuccess { p -> _panels.value = p }
                .onFailure { e -> _panels.update { it.copy(loading = false) }; _state.update { it.copy(error = e.message) } }
        }
    }
}
