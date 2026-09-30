package com.guyu2233.ibukirpg.app.ui.home

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.guyu2233.ibukirpg.app.data.AIStatusV1
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.SettingsStore
import com.guyu2233.ibukirpg.app.data.SlotV1
import kotlinx.coroutines.Deferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class HomeState(
    val loading: Boolean = true,
    val working: Boolean = false,
    val latest: SlotV1? = null,
    val saveCount: Int = 0,
    val ai: AIStatusV1 = AIStatusV1(),
    val error: String? = null,
    val version: String = "",
)

/** 本地崩溃记录的读写（IO 线程调用）；测试里可替换。 */
interface CrashLogSource {
    fun pending(): String?
    fun clear()
}

class HomeViewModel(
    private val engine: Engine,
    private val settings: SettingsStore,
    private val aiStartupReady: Deferred<Unit>,
    private val crashLog: CrashLogSource? = null,
) : ViewModel() {
    private val _state = MutableStateFlow(HomeState())
    val state: StateFlow<HomeState> = _state.asStateFlow()

    /** 上次运行的崩溃 / 被系统终止记录（只在本机显示，不上传）。 */
    private val _crash = MutableStateFlow<String?>(null)
    val crash: StateFlow<String?> = _crash.asStateFlow()

    init {
        crashLog?.let { src ->
            viewModelScope.launch(Dispatchers.IO) { _crash.value = runCatching { src.pending() }.getOrNull() }
        }
    }

    fun dismissCrash() {
        _crash.value = null
        crashLog?.let { src -> viewModelScope.launch(Dispatchers.IO) { runCatching { src.clear() } } }
    }

    fun refresh() {
        viewModelScope.launch {
            aiStartupReady.await()
            runCatching {
                // 等待启动时恢复的 AI 配置，并刷新引擎状态与存档。
                val ai = engine.aiStatus()
                val saves = engine.listSaves()
                Triple(ai, saves, engine.version)
            }.onSuccess { (ai, saves, v) ->
                _state.update { it.copy(loading = false, latest = saves.firstOrNull(), saveCount = saves.size, ai = ai, version = v, error = null) }
            }.onFailure { e ->
                _state.update { it.copy(loading = false, error = e.message) }
            }
        }
    }

    fun continueLatest(onReady: () -> Unit) {
        val slot = _state.value.latest ?: return
        run(onReady) { engine.loadGame(slot.id) }
    }

    private fun run(onReady: () -> Unit, block: suspend () -> Unit) {
        if (_state.value.working) return
        _state.update { it.copy(working = true, error = null) }
        viewModelScope.launch {
            runCatching { block() }
                .onSuccess { _state.update { it.copy(working = false) }; onReady() }
                .onFailure { e -> _state.update { it.copy(working = false, error = e.message) } }
        }
    }

    fun dismissError() = _state.update { it.copy(error = null) }
}
