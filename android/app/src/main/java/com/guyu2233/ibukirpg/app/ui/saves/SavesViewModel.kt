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

class SavesViewModel(private val engine: Engine, private val app: android.app.Application? = null) : ViewModel() {
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

    /** 导出到应用缓存目录（.ibksave，不含 API 密钥），返回文件供界面通过 SAF 保存到玩家选择的位置。 */
    fun export(slot: SlotV1, onFile: (java.io.File, String) -> Unit) {
        val ctx = app ?: return
        act(null) {
            val dir = java.io.File(ctx.cacheDir, "exports").apply { mkdirs() }
            val r = engine.exportSave(slot.id, dir.absolutePath)
            onFile(java.io.File(r.path), r.name)
        }
    }

    /** 把导出文件写到 SAF 目标 URI。 */
    fun writeExport(file: java.io.File, uri: android.net.Uri) {
        val ctx = app ?: return
        act("存档已导出（不含 API 密钥）") {
            kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
                ctx.contentResolver.openOutputStream(uri)?.use { out -> file.inputStream().use { it.copyTo(out) } } ?: error("无法写入所选位置")
                file.delete()
            }
        }
    }

    /** 从 SAF 选择的 .ibksave 导入：先复制到缓存，引擎检查引擎 / 故事包版本后导入为新存档。 */
    fun import(uri: android.net.Uri) {
        val ctx = app ?: return
        act("存档已导入") {
            val tmp = java.io.File(ctx.cacheDir, "import.ibksave")
            kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
                ctx.contentResolver.openInputStream(uri)?.use { input -> tmp.outputStream().use { input.copyTo(it) } } ?: error("无法读取所选文件")
            }
            try {
                val check = engine.inspectSave(tmp.absolutePath)
                if (!check.ok) error(check.problems.joinToString("\n").ifBlank { "这个存档无法导入" })
                engine.importSave(tmp.absolutePath)
            } finally {
                tmp.delete()
            }
        }
    }

    fun consumeMessage() = _state.update { it.copy(message = null) }
}
