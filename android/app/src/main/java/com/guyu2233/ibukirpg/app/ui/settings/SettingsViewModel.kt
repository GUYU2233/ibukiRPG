package com.guyu2233.ibukirpg.app.ui.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.guyu2233.ibukirpg.app.data.AIConfig
import com.guyu2233.ibukirpg.app.data.AppSettings
import com.guyu2233.ibukirpg.app.llm.LlamaLocalAI
import com.guyu2233.ibukirpg.app.llm.LocalModelConfig
import android.net.Uri
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.PresetV1
import com.guyu2233.ibukirpg.app.data.SettingsStore
import com.guyu2233.ibukirpg.app.data.ThemeMode
import kotlinx.coroutines.Deferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

sealed interface TestState {
    data object Idle : TestState
    data object Running : TestState
    data class Ok(val ms: Long) : TestState
    data class Fail(val msg: String) : TestState
}

data class AIForm(
    val kind: String = "offline",
    val baseUrl: String = "",
    val model: String = "",
    val key: String = "",
    val hasSavedKey: Boolean = false,
    val test: TestState = TestState.Idle,
    val saving: Boolean = false,
    val dirty: Boolean = false,
    val localModelPath: String = "",
    val localModelName: String = "",
    val temperature: Float = 0.7f,
    val contextTokens: Int = 2048,
    val topK: Int = 40,
    val topP: Float = 0.95f,
    val threads: Int = 0,
    val gpuLayers: Int = 0,
    val maxTokens: Int = 512,
    val importingModel: Boolean = false,
    val modelError: String? = null,
    val memoryWarning: String? = null,
    val localStatus: String = "",
    val localAvailable: Boolean = true,
) {
    fun localConfig() = LocalModelConfig(
        path = localModelPath, label = localModelName, contextTokens = contextTokens, threads = threads, gpuLayers = gpuLayers,
        temperature = temperature, topP = topP, topK = topK, maxTokens = maxTokens,
    )
}

class SettingsViewModel(
    private val engine: Engine,
    private val store: SettingsStore,
    private val localAI: LlamaLocalAI,
    private val aiStartupReady: Deferred<Unit>,
) : ViewModel() {
    val settings: StateFlow<AppSettings> = store.settings.stateIn(viewModelScope, SharingStarted.Eagerly, AppSettings())

    private val _form = MutableStateFlow(AIForm())
    val form: StateFlow<AIForm> = _form.asStateFlow()

    private val _presets = MutableStateFlow<List<PresetV1>>(emptyList())
    val presets: StateFlow<List<PresetV1>> = _presets.asStateFlow()

    private val _saved = MutableStateFlow(false)
    val saved: StateFlow<Boolean> = _saved.asStateFlow()

    val engineVersion: String get() = engine.version

    init {
        viewModelScope.launch {
            val s = store.settings.first()
            _presets.value = runCatching { engine.presets() }.getOrDefault(emptyList())
            _form.value = AIForm(
                kind = s.aiKind, baseUrl = s.baseUrl, model = s.model, hasSavedKey = s.hasKey,
                localModelPath = s.localModelPath, localModelName = s.localModelName,
                temperature = s.localTemperature, contextTokens = s.localContextTokens,
                topK = s.localTopK, topP = s.localTopP, threads = s.localThreads, gpuLayers = s.localGpuLayers, maxTokens = s.localMaxTokens,
                localAvailable = localAI.available, modelError = s.migrationNotice,
            )
            if (s.migrationNotice != null) store.clearMigrationNotice()
        }
        viewModelScope.launch { localAI.status.collect { st -> _form.update { it.copy(localStatus = st) } } }
    }

    fun selectKind(kind: String) {
        val p = _presets.value.firstOrNull { it.kind == kind }
        _form.update {
            it.copy(
                kind = kind,
                baseUrl = p?.baseUrl?.takeIf { u -> u.isNotEmpty() } ?: if (kind == "custom") it.baseUrl else "",
                model = p?.model?.takeIf { m -> m.isNotEmpty() } ?: when (kind) {
                    "custom" -> it.model
                    "llamacpp" -> it.localModelName
                    else -> ""
                },
                test = TestState.Idle,
                dirty = true,
            )
        }
    }

    fun setBaseUrl(v: String) = _form.update { it.copy(baseUrl = v, test = TestState.Idle, dirty = true) }
    fun setModel(v: String) = _form.update { it.copy(model = v, test = TestState.Idle, dirty = true) }
    fun setKey(v: String) = _form.update { it.copy(key = v, test = TestState.Idle, dirty = true) }

    private suspend fun effectiveConfig(): AIConfig {
        val f = _form.value
        if (f.kind == LlamaLocalAI.KIND) {
            require(f.localModelPath.isNotBlank()) { "请先导入 GGUF 模型文件。" }
            val cfg = localAI.configure(f.localConfig())
            _form.update { it.copy(memoryWarning = localAI.lastWarning) }
            return cfg
        }
        val key = f.key.trim().ifEmpty { if (f.hasSavedKey) store.aiConfig().apiKey else "" }
        return AIConfig(kind = f.kind, baseUrl = f.baseUrl.trim(), model = f.model.trim(), apiKey = key)
    }

    fun test() {
        _form.update { it.copy(test = TestState.Running) }
        viewModelScope.launch {
            val r = runCatching {
                aiStartupReady.await()
                engine.testAI(effectiveConfig())
            }
            _form.update {
                it.copy(
                    test = r.fold(
                        onSuccess = { res -> if (res.ok) TestState.Ok(res.latencyMs) else TestState.Fail(res.error ?: "未知错误") },
                        onFailure = { e -> TestState.Fail(e.message ?: "未知错误") },
                    ),
                )
            }
        }
    }

    fun save() {
        _form.update { it.copy(saving = true) }
        viewModelScope.launch {
            aiStartupReady.await()
            val f = _form.value
            val result = runCatching {
                store.saveAI(f.kind, f.baseUrl, f.model, f.key.trim().ifEmpty { null })
                store.saveLocalModelOptions(f.localConfig())
                val cfg = effectiveConfig()
                if (f.kind != LlamaLocalAI.KIND) localAI.deactivate()
                engine.configureAI(cfg)
            }
            result.onSuccess {
                _form.update { it.copy(saving = false, key = "", hasSavedKey = it.hasSavedKey || f.key.isNotBlank(), dirty = false, test = TestState.Idle, modelError = null) }
                _saved.value = true
            }.onFailure { e ->
                _form.update { it.copy(saving = false, test = TestState.Fail(e.message ?: "配置本地模型失败")) }
            }
        }
    }

    fun importLocalModel(uri: Uri) {
        _form.update { it.copy(importingModel = true, modelError = null) }
        viewModelScope.launch {
            runCatching {
                localAI.deactivate()
                store.importLocalModel(uri)
            }
                .onSuccess { name ->
                    val saved = store.settings.first()
                    _form.update { it.copy(importingModel = false, localModelPath = saved.localModelPath, localModelName = name, model = name, dirty = true, test = TestState.Idle) }
                }
                .onFailure { error ->
                    _form.update { it.copy(importingModel = false, modelError = error.message ?: "导入模型失败") }
                }
        }
    }

    fun removeLocalModel() {
        viewModelScope.launch {
            aiStartupReady.await()
            runCatching {
                localAI.deactivate()
                store.removeLocalModel()
            }.onSuccess {
                _form.update { it.copy(localModelPath = "", localModelName = "", model = "", dirty = true, test = TestState.Idle, modelError = null) }
            }.onFailure { e -> _form.update { it.copy(modelError = e.message ?: "删除模型失败") } }
        }
    }

    fun setTemperature(value: Float) = _form.update { it.copy(temperature = value.coerceIn(0f, 1.5f), dirty = true) }
    fun setContextTokens(value: Int) = _form.update { it.copy(contextTokens = value.coerceIn(512, 8192), dirty = true) }
    fun setTopK(value: Int) = _form.update { it.copy(topK = value.coerceIn(1, 100), dirty = true) }
    fun setTopP(value: Float) = _form.update { it.copy(topP = value.coerceIn(0.1f, 1f), dirty = true) }
    fun setThreads(value: Int) = _form.update { it.copy(threads = value.coerceIn(0, 8), dirty = true) }
    fun setGpuLayers(value: Int) = _form.update { it.copy(gpuLayers = value.coerceIn(0, 999), dirty = true) }
    fun setMaxTokens(value: Int) = _form.update { it.copy(maxTokens = value.coerceIn(32, 2048), dirty = true) }

    fun clearKey() {
        viewModelScope.launch {
            val f = _form.value
            store.saveAI(f.kind, f.baseUrl, f.model, "")
            _form.update { it.copy(key = "", hasSavedKey = false, test = TestState.Idle) }
            runCatching { engine.configureAI(AIConfig(kind = f.kind, baseUrl = f.baseUrl, model = f.model, apiKey = "")) }
        }
    }

    fun consumeSaved() { _saved.value = false }

    fun setTextScale(v: Float) = viewModelScope.launch { store.setTextScale(v) }
    fun setTheme(v: ThemeMode) = viewModelScope.launch { store.setTheme(v) }
    fun setDynamic(v: Boolean) = viewModelScope.launch { store.setDynamic(v) }
}
