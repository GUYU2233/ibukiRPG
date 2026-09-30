package com.guyu2233.ibukirpg.app.ui.saves

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.SlotV1
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class SavesState(
    val loading: Boolean = true,
    val working: Boolean = false,
    val slots: List<SlotV1> = emptyList(),
    val message: String? = null,
)

class SavesViewModel(private val engine: Engine) : ViewModel() {
    private val _state = MutableStateFlow(SavesState())
    val state: StateFlow<SavesState> = _state.asStateFlow()

    init { refresh() }

    fun refresh() = viewModelScope.launch {
        runCatching { engine.listSaves() }
            .onSuccess { list -> _state.update { it.copy(loading = false, slots = list) } }
            .onFailure { e -> _state.update { it.copy(loading = false, message = e.message) } }
    }

    fun load(slot: SlotV1, onReady: () -> Unit) = act(null, onReady) { engine.loadGame(slot.id) }
    fun delete(slot: SlotV1, done: String) = act(done) { engine.deleteSave(slot.id) }
    fun copy(slot: SlotV1, done: String) = act(done) { engine.copySave(slot.id, "") }
    fun rename(slot: SlotV1, name: String) = act(null) { engine.renameSave(slot.id, name.trim()) }

    private fun act(done: String?, onReady: (() -> Unit)? = null, block: suspend () -> Unit) {
        if (_state.value.working) return
        _state.update { it.copy(working = true) }
        viewModelScope.launch {
            runCatching { block() }
                .onSuccess {
                    _state.update { it.copy(working = false, message = done) }
                    if (onReady != null) onReady() else refresh()
                }
                .onFailure { e -> _state.update { it.copy(working = false, message = e.message) } }
        }
    }

    fun consumeMessage() = _state.update { it.copy(message = null) }
}
