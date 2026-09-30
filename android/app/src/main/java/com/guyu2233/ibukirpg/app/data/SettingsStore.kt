package com.guyu2233.ibukirpg.app.data

import android.content.Context
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.floatPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.dataStore by preferencesDataStore(name = "settings")

enum class ThemeMode { SYSTEM, LIGHT, DARK }

data class AppSettings(
    val aiKind: String = "offline",
    val baseUrl: String = "",
    val model: String = "",
    val hasKey: Boolean = false,
    val textScale: Float = 1f,
    val theme: ThemeMode = ThemeMode.SYSTEM,
    val dynamicColor: Boolean = true,
)

/** DataStore 持久化设置；API Key 以 Keystore 加密后的密文保存。 */
class SettingsStore(private val context: Context) {
    private object K {
        val kind = stringPreferencesKey("ai_kind")
        val baseUrl = stringPreferencesKey("ai_base_url")
        val model = stringPreferencesKey("ai_model")
        val keyEnc = stringPreferencesKey("ai_key_enc")
        val textScale = floatPreferencesKey("text_scale")
        val theme = stringPreferencesKey("theme")
        val dynamic = booleanPreferencesKey("dynamic_color")
    }

    val settings: Flow<AppSettings> = context.dataStore.data.map { it.toSettings() }

    private fun Preferences.toSettings() = AppSettings(
        aiKind = this[K.kind] ?: "offline",
        baseUrl = this[K.baseUrl] ?: "",
        model = this[K.model] ?: "",
        hasKey = !this[K.keyEnc].isNullOrEmpty(),
        textScale = this[K.textScale] ?: 1f,
        theme = runCatching { ThemeMode.valueOf(this[K.theme] ?: "SYSTEM") }.getOrDefault(ThemeMode.SYSTEM),
        dynamicColor = this[K.dynamic] ?: true,
    )

    /** 当前 AI 配置（含解密后的密钥），仅用于传给引擎。 */
    suspend fun aiConfig(): AIConfig {
        val p = context.dataStore.data.first()
        val key = p[K.keyEnc]?.let { KeyCipher.decrypt(it) }.orEmpty()
        return AIConfig(kind = p[K.kind] ?: "offline", baseUrl = p[K.baseUrl].orEmpty(), model = p[K.model].orEmpty(), apiKey = key)
    }

    /** 保存 AI 设置。newKey 为 null 表示保持原密钥，空字符串表示清除。 */
    suspend fun saveAI(kind: String, baseUrl: String, model: String, newKey: String?) {
        val enc = newKey?.takeIf { it.isNotBlank() }?.let { KeyCipher.encrypt(it.trim()) }
        context.dataStore.edit {
            it[K.kind] = kind
            it[K.baseUrl] = baseUrl.trim()
            it[K.model] = model.trim()
            when {
                newKey == null -> Unit
                enc == null -> it.remove(K.keyEnc)
                else -> it[K.keyEnc] = enc
            }
        }
    }

    suspend fun setTextScale(v: Float) = context.dataStore.edit { it[K.textScale] = v }
    suspend fun setTheme(v: ThemeMode) = context.dataStore.edit { it[K.theme] = v.name }
    suspend fun setDynamic(v: Boolean) = context.dataStore.edit { it[K.dynamic] = v }
}
