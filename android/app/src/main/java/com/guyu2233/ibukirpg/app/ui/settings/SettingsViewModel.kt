package com.guyu2233.ibukirpg.app.ui.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.guyu2233.ibukirpg.app.data.AIConfig
import com.guyu2233.ibukirpg.app.data.AppSettings
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.PresetV1
import com.guyu2233.ibukirpg.app.data.SettingsStore
import com.guyu2233.ibukirpg.app.data.ThemeMode
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
)

class SettingsViewModel(private val engine: Engine, private val store: SettingsStore) : ViewModel() {
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
            _form.value = AIForm(kind = s.aiKind, baseUrl = s.baseUrl, model = s.model, hasSavedKey = s.hasKey)
        }
    }

    fun selectKind(kind: String) {
        val p = _presets.value.firstOrNull { it.kind == kind }
        _form.update {
            it.copy(
                kind = kind,
                baseUrl = p?.baseUrl?.takeIf { u -> u.isNotEmpty() } ?: if (kind == "custom") it.baseUrl else "",
                model = p?.model?.takeIf { m -> m.isNotEmpty() } ?: if (kind == "custom") it.model else "",
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
        val key = f.key.trim().ifEmpty { if (f.hasSavedKey) store.aiConfig().apiKey else "" }
        return AIConfig(kind = f.kind, baseUrl = f.baseUrl.trim(), model = f.model.trim(), apiKey = key)
    }

    fun test() {
        _form.update { it.copy(test = TestState.Running) }
        viewModelScope.launch {
            val r = runCatching { engine.testAI(effectiveConfig()) }
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
            val f = _form.value
            val cfg = effectiveConfig()
            store.saveAI(f.kind, f.baseUrl, f.model, f.key.trim().ifEmpty { null })
            runCatching { engine.configureAI(cfg) }
            _form.update { it.copy(saving = false, key = "", hasSavedKey = it.hasSavedKey || f.key.isNotBlank(), dirty = false) }
            _saved.value = true
        }
    }

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
