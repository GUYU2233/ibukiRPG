package com.guyu2233.ibukirpg.app.ui.packs

import android.content.Context
import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.guyu2233.ibukirpg.app.data.CreationOptionsV1
import com.guyu2233.ibukirpg.app.data.CreationReviewV1
import com.guyu2233.ibukirpg.app.data.CreationV1
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.EngineException
import com.guyu2233.ibukirpg.app.data.ImportResultV1
import com.guyu2233.ibukirpg.app.data.PackV1
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File

data class PacksState(
    val loading: Boolean = true,
    val working: Boolean = false,
    val importing: Boolean = false,
    val packs: List<PackV1> = emptyList(),
    /** 一次性提示（Snackbar）。 */
    val message: String? = null,
    /** 导入失败的详细原因（对话框，可能较长）。 */
    val importError: String? = null,
    val imported: ImportResultV1? = null,
    /** 角色创建：正在为哪个故事包创建角色（null = 未打开）。 */
    val creation: CreationState? = null,
)

/** 角色创建界面状态。 */
data class CreationState(
    val pack: PackV1,
    val options: CreationOptionsV1? = null,
    val loading: Boolean = true,
    val reviewing: Boolean = false,
    val review: CreationReviewV1? = null,
    val error: String? = null,
)

/** 与 Go 端 registry.MaxTotalBytes 一致：超过这个大小的文件不必复制。 */
private const val MAX_IMPORT_BYTES = 64L shl 20

class PacksViewModel(private val engine: Engine, private val context: Context) : ViewModel() {
    private val _state = MutableStateFlow(PacksState())
    val state: StateFlow<PacksState> = _state.asStateFlow()

    init { refresh() }

    fun refresh() = viewModelScope.launch {
        runCatching { engine.listPacks() }
            .onSuccess { list -> _state.update { it.copy(loading = false, packs = list) } }
            .onFailure { e -> _state.update { it.copy(loading = false, message = e.message) } }
    }

    fun newGame(pack: PackV1, name: String, creation: CreationV1? = null, onReady: () -> Unit) {
        if (_state.value.working) return
        _state.update { it.copy(working = true) }
        viewModelScope.launch {
            runCatching { engine.newGame(name.trim(), pack.id, creation?.copy(name = name.trim())) }
                .onSuccess { _state.update { it.copy(working = false, creation = null) }; onReady() }
                .onFailure { e ->
                    _state.update {
                        if (it.creation != null) it.copy(working = false, creation = it.creation.copy(error = e.message))
                        else it.copy(working = false, message = e.message)
                    }
                }
        }
    }

    /** 打开角色创建：读取故事包的预设主角 / 出身 / 属性点。 */
    fun openCreation(pack: PackV1) {
        _state.update { it.copy(creation = CreationState(pack)) }
        viewModelScope.launch {
            runCatching { engine.getCreation(pack.id) }
                .onSuccess { o -> _state.update { s -> s.copy(creation = s.creation?.copy(options = o, loading = false)) } }
                .onFailure { e -> _state.update { s -> s.copy(creation = s.creation?.copy(loading = false, error = e.message)) } }
        }
    }

    fun closeCreation() = _state.update { it.copy(creation = null) }

    /** 规则检查 + 审查 Agent（联网模型可用时）：设定契合度、强度、冲突、建议与推荐角色卡。 */
    fun reviewCreation(c: CreationV1) {
        val cs = _state.value.creation ?: return
        if (cs.reviewing) return
        _state.update { s -> s.copy(creation = s.creation?.copy(reviewing = true, error = null)) }
        viewModelScope.launch {
            runCatching { engine.reviewCreation(cs.pack.id, c) }
                .onSuccess { r -> _state.update { s -> s.copy(creation = s.creation?.copy(reviewing = false, review = r)) } }
                .onFailure { e -> _state.update { s -> s.copy(creation = s.creation?.copy(reviewing = false, error = e.message)) } }
        }
    }

    fun clearReview() = _state.update { s -> s.copy(creation = s.creation?.copy(review = null)) }

    /** SAF 选中的 .zip：先复制到缓存目录（引擎只读本地文件），导入完成后删除临时文件。 */
    fun import(uri: Uri) {
        if (_state.value.working) return
        _state.update { it.copy(working = true, importing = true, importError = null) }
        viewModelScope.launch {
            val tmp = File(context.cacheDir, "pack-import-${System.nanoTime()}.zip")
            runCatching {
                withContext(Dispatchers.IO) {
                    val input = context.contentResolver.openInputStream(uri) ?: throw EngineException("无法读取所选文件")
                    input.use { src ->
                        tmp.outputStream().buffered().use { out ->
                            val buf = ByteArray(64 * 1024)
                            var total = 0L
                            while (true) {
                                val n = src.read(buf)
                                if (n < 0) break
                                total += n
                                if (total > MAX_IMPORT_BYTES) throw EngineException("文件太大：故事包不能超过 64 MB")
                                out.write(buf, 0, n)
                            }
                        }
                    }
                }
                engine.importPack(tmp.absolutePath)
            }.onSuccess { r ->
                _state.update { it.copy(working = false, importing = false, imported = r) }
                refresh()
            }.onFailure { e ->
                _state.update { it.copy(working = false, importing = false, importError = e.message ?: "未知错误") }
            }
            withContext(Dispatchers.IO) { tmp.delete() }
        }
    }

    fun delete(pack: PackV1, done: String) {
        if (_state.value.working) return
        _state.update { it.copy(working = true) }
        viewModelScope.launch {
            runCatching { engine.deletePack(pack.id) }
                .onSuccess { _state.update { it.copy(working = false, message = done) }; refresh() }
                .onFailure { e -> _state.update { it.copy(working = false, message = e.message) } }
        }
    }

    fun consumeMessage() = _state.update { it.copy(message = null, imported = null) }
    fun dismissImportError() = _state.update { it.copy(importError = null) }
}
