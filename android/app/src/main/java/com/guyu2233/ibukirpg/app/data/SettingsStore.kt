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
    val localThreads: Int = 0,
    val localGpuLayers: Int = 0,
    val localMaxTokens: Int = 512,
    /** 从 v0.1.1/v0.1.2rc1 的 MediaPipe 模式迁移过来时的一次性提示。 */
    val migrationNotice: String? = null,
) {
    fun localConfig() = com.guyu2233.ibukirpg.app.llm.LocalModelConfig(
        path = localModelPath, label = localModelName, contextTokens = localContextTokens, threads = localThreads,
        gpuLayers = localGpuLayers, temperature = localTemperature, topP = localTopP, topK = localTopK, maxTokens = localMaxTokens,
    )
}

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
        val localModelPath = stringPreferencesKey("local_model_path")
        val localModelName = stringPreferencesKey("local_model_name")
        val localTemperature = floatPreferencesKey("local_temperature")
        val localContextTokens = intPreferencesKey("local_context_tokens")
        val localTopK = intPreferencesKey("local_top_k")
        val localTopP = floatPreferencesKey("local_top_p")
        val localThreads = intPreferencesKey("local_threads")
        val localGpuLayers = intPreferencesKey("local_gpu_layers")
        val localMaxTokens = intPreferencesKey("local_max_tokens")
        val migrationNotice = stringPreferencesKey("migration_notice")
        // v0.1.1 ~ v0.1.2rc1（MediaPipe）的旧键，仅用于迁移
        val oldPath = stringPreferencesKey("mediapipe_model_path")
        val oldName = stringPreferencesKey("mediapipe_model_name")
        val oldTemperature = floatPreferencesKey("mediapipe_temperature")
        val oldContext = intPreferencesKey("mediapipe_context_tokens")
        val oldTopK = intPreferencesKey("mediapipe_top_k")
        val oldTopP = floatPreferencesKey("mediapipe_top_p")
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
        localThreads = (this[K.localThreads] ?: 0).coerceIn(0, 8),
        localGpuLayers = (this[K.localGpuLayers] ?: 0).coerceIn(0, 999),
        localMaxTokens = (this[K.localMaxTokens] ?: 512).coerceIn(32, 2048),
        migrationNotice = this[K.migrationNotice],
    )

    /**
     * 一次性迁移：MediaPipe 本地模型已被 llama.cpp 取代。旧的 .task 模型无法再使用——删除它释放空间，
     * 采样参数沿用，AI 模式退回离线并留下提示。
     */
    suspend fun migrateFromMediaPipe() = withContext(Dispatchers.IO) {
        val p = context.dataStore.data.first()
        val wasMediaPipe = p[K.kind] == "mediapipe"
        val hadOld = p[K.oldPath] != null || wasMediaPipe
        val oldDir = File(context.filesDir, "mediapipe-models")
        if (oldDir.exists()) oldDir.deleteRecursively()
        if (!hadOld) return@withContext
        context.dataStore.edit {
            it[K.oldTemperature]?.let { v -> it[K.localTemperature] = v }
            it[K.oldContext]?.let { v -> it[K.localContextTokens] = v }
            it[K.oldTopK]?.let { v -> it[K.localTopK] = v }
            it[K.oldTopP]?.let { v -> it[K.localTopP] = v }
            listOf(K.oldPath, K.oldName).forEach { k -> it.remove(k) }
            it.remove(K.oldTemperature); it.remove(K.oldContext); it.remove(K.oldTopK); it.remove(K.oldTopP)
            if (wasMediaPipe) {
                it[K.kind] = "offline"
                it[K.migrationNotice] = "本地模型已从 MediaPipe 换成 llama.cpp：请在设置中重新导入 GGUF 格式的模型（旧的 .task 文件已删除）。"
            }
        }
    }

    suspend fun clearMigrationNotice() = context.dataStore.edit { it.remove(K.migrationNotice) }

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

    /**
     * 把用户选中的 GGUF 模型复制到应用私有目录（SAF 导入）；引擎只接收真实文件路径，不保存外部 URI。
     * 复制前检查剩余空间，复制后校验 GGUF 文件头；失败时保留旧模型。
     */
    suspend fun importLocalModel(uri: Uri): String = withContext(Dispatchers.IO) {
        val resolver = context.contentResolver
        val readFlag = Intent.FLAG_GRANT_READ_URI_PERMISSION
        val persistedGrant = runCatching { resolver.takePersistableUriPermission(uri, readFlag) }.isSuccess
        try {
            val displayName = resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)
                ?.use { cursor -> if (cursor.moveToFirst()) cursor.getString(0) else null }
                ?: uri.lastPathSegment?.substringAfterLast('/')
                ?: "model.gguf"
            require(displayName.endsWith(".gguf", ignoreCase = true)) {
                "请选择 GGUF 格式的模型文件（.gguf）。MediaPipe .task、safetensors 等格式需要先转换。"
            }
            val size = resolver.query(uri, arrayOf(OpenableColumns.SIZE), null, null, null)
                ?.use { c -> if (c.moveToFirst() && !c.isNull(0)) c.getLong(0) else -1L } ?: -1L
            val dir = File(context.filesDir, "models")
            check(dir.exists() || dir.mkdirs()) { "无法创建本地模型目录" }
            if (size > 0) check(dir.usableSpace > size + 200L * 1024 * 1024) { "存储空间不足：模型需要约 ${size / (1024 * 1024)} MB。" }
            val tmp = File(dir, "model.gguf.importing")
            val target = File(dir, "model.gguf")
            resolver.openInputStream(uri)?.use { input ->
                tmp.outputStream().buffered(1 shl 20).use { output -> input.copyTo(output, 1 shl 20) }
            } ?: error("无法读取所选模型文件")
            when (val r = com.guyu2233.ibukirpg.app.llm.Gguf.check(tmp)) {
                is com.guyu2233.ibukirpg.app.llm.Gguf.Result.Bad -> { tmp.delete(); error(r.reason) }
                is com.guyu2233.ibukirpg.app.llm.Gguf.Result.Ok -> Unit
            }
            val backup = File(dir, "model.gguf.previous")
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
        val modelDir = File(context.filesDir, "models").canonicalFile
        if (path.isNotBlank()) {
            val file = File(path).canonicalFile
            if (file.parentFile == modelDir && file.exists()) file.delete()
        }
        context.dataStore.edit {
            it.remove(K.localModelPath)
            it.remove(K.localModelName)
        }
    }

    suspend fun saveLocalModelOptions(c: com.guyu2233.ibukirpg.app.llm.LocalModelConfig) = context.dataStore.edit {
        val n = c.normalized()
        it[K.localTemperature] = n.temperature
        it[K.localContextTokens] = n.contextTokens
        it[K.localTopK] = n.topK
        it[K.localTopP] = n.topP
        it[K.localThreads] = n.threads
        it[K.localGpuLayers] = n.gpuLayers
        it[K.localMaxTokens] = n.maxTokens
    }

    suspend fun setTextScale(v: Float) = context.dataStore.edit { it[K.textScale] = v }
    suspend fun setTheme(v: ThemeMode) = context.dataStore.edit { it[K.theme] = v.name }
    suspend fun setDynamic(v: Boolean) = context.dataStore.edit { it[K.dynamic] = v }
}
