package com.guyu2233.ibukirpg.app.ui.home

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.guyu2233.ibukirpg.app.data.AIStatusV1
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.SettingsStore
import com.guyu2233.ibukirpg.app.data.SlotV1
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

class HomeViewModel(private val engine: Engine, private val settings: SettingsStore) : ViewModel() {
    private val _state = MutableStateFlow(HomeState())
    val state: StateFlow<HomeState> = _state.asStateFlow()

    fun refresh() {
        viewModelScope.launch {
            runCatching {
                // 保证引擎拿到最新 AI 配置（设置页返回后也会刷新）。
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

    fun newGame(name: String, onReady: () -> Unit) = run(onReady) { engine.newGame(name.trim()) }

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
