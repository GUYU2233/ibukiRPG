package com.guyu2233.ibukirpg.app.data

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.OpenableColumns
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.floatPreferencesKey
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File

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
    val localModelPath: String = "",
    val localModelName: String = "",
    val localTemperature: Float = 0.7f,
    val localContextTokens: Int = 2048,
    val localTopK: Int = 40,
    val localTopP: Float = 0.95f,
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
        val localModelPath = stringPreferencesKey("mediapipe_model_path")
        val localModelName = stringPreferencesKey("mediapipe_model_name")
        val localTemperature = floatPreferencesKey("mediapipe_temperature")
        val localContextTokens = intPreferencesKey("mediapipe_context_tokens")
        val localTopK = intPreferencesKey("mediapipe_top_k")
        val localTopP = floatPreferencesKey("mediapipe_top_p")
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
        localModelPath = this[K.localModelPath].orEmpty(),
        localModelName = this[K.localModelName].orEmpty(),
        localTemperature = (this[K.localTemperature] ?: 0.7f).coerceIn(0f, 1.5f),
        localContextTokens = (this[K.localContextTokens] ?: 2048).coerceIn(512, 8192),
        localTopK = (this[K.localTopK] ?: 40).coerceIn(1, 100),
        localTopP = (this[K.localTopP] ?: 0.95f).coerceIn(0.1f, 1f),
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

    /** 把用户选中的 MediaPipe .task 复制到应用私有目录；引擎只接收真实文件路径，不保存外部 URI。 */
    suspend fun importLocalModel(uri: Uri): String = withContext(Dispatchers.IO) {
        val resolver = context.contentResolver
        val readFlag = Intent.FLAG_GRANT_READ_URI_PERMISSION
        val persistedGrant = runCatching { resolver.takePersistableUriPermission(uri, readFlag) }.isSuccess
        try {
            val displayName = resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)
                ?.use { cursor -> if (cursor.moveToFirst()) cursor.getString(0) else null }
                ?: uri.lastPathSegment?.substringAfterLast('/')
                ?: "model.task"
            require(displayName.endsWith(".task", ignoreCase = true)) {
                "请选择 MediaPipe .task 模型文件；GGUF 或原始权重不能直接加载。"
            }
            val dir = File(context.filesDir, "mediapipe-models")
            check(dir.exists() || dir.mkdirs()) { "无法创建本地模型目录" }
            val tmp = File(dir, "model.task.importing")
            val target = File(dir, "model.task")
            resolver.openInputStream(uri)?.use { input ->
                tmp.outputStream().buffered().use { output -> input.copyTo(output) }
            } ?: error("无法读取所选模型文件")
            check(tmp.length() > 0L) { "模型文件为空" }
            val backup = File(dir, "model.task.previous")
            if (backup.exists()) backup.delete()
            val hadOldModel = target.exists()
            if (hadOldModel) check(target.renameTo(backup)) { "无法暂存旧模型" }
            if (!tmp.renameTo(target)) {
                if (hadOldModel) backup.renameTo(target)
                error("无法保存模型到应用私有目录")
            }
            try {
                context.dataStore.edit {
                    it[K.localModelPath] = target.absolutePath
                    it[K.localModelName] = displayName
                }
            } catch (error: Throwable) {
                target.delete()
                if (hadOldModel) backup.renameTo(target)
                throw error
            }
            backup.delete()
            displayName
        } finally {
            if (persistedGrant) runCatching { resolver.releasePersistableUriPermission(uri, readFlag) }
        }
    }

    suspend fun removeLocalModel() = withContext(Dispatchers.IO) {
        val path = context.dataStore.data.first()[K.localModelPath].orEmpty()
        val modelDir = File(context.filesDir, "mediapipe-models").canonicalFile
        if (path.isNotBlank()) {
            val file = File(path).canonicalFile
            if (file.parentFile == modelDir && file.exists()) file.delete()
        }
        context.dataStore.edit {
            it.remove(K.localModelPath)
            it.remove(K.localModelName)
        }
    }

    suspend fun saveLocalModelOptions(temperature: Float, contextTokens: Int, topK: Int, topP: Float) = context.dataStore.edit {
        it[K.localTemperature] = temperature.coerceIn(0f, 1.5f)
        it[K.localContextTokens] = contextTokens.coerceIn(512, 8192)
        it[K.localTopK] = topK.coerceIn(1, 100)
        it[K.localTopP] = topP.coerceIn(0.1f, 1f)
    }

    suspend fun setTextScale(v: Float) = context.dataStore.edit { it[K.textScale] = v }
    suspend fun setTheme(v: ThemeMode) = context.dataStore.edit { it[K.theme] = v.name }
    suspend fun setDynamic(v: Boolean) = context.dataStore.edit { it[K.dynamic] = v }
}
