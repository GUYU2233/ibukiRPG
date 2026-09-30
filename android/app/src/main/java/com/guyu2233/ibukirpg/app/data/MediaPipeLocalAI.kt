package com.guyu2233.ibukirpg.app.data

import android.content.Context
import com.google.mediapipe.tasks.genai.llminference.LlmInference
import com.google.mediapipe.tasks.genai.llminference.LlmInferenceSession
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.File
import java.security.SecureRandom

/** Loads one app-private MediaPipe .task model and exposes it to the Go engine over loopback only. */
class MediaPipeLocalAI(context: Context) : AutoCloseable {
    companion object {
        private const val BASE_URL = "http://127.0.0.1:${LocalModelHttpServer.PORT}/v1"
        private val random = SecureRandom()
    }

    private val appContext = context.applicationContext
    private val lock = Any()
    @Volatile private var inference: LlmInference? = null
    @Volatile private var loadedPath: String = ""
    @Volatile private var loadedContextTokens: Int = 0
    @Volatile private var temperature: Float = 0.7f
    @Volatile private var topK: Int = 40
    @Volatile private var topP: Float = 0.95f
    @Volatile private var localServer: LocalModelHttpServer? = null
    @Volatile private var token: String = newToken()

    suspend fun configure(
        modelPath: String,
        modelLabel: String,
        temperature: Float,
        contextTokens: Int,
        topK: Int = 40,
        topP: Float = 0.95f,
    ): AIConfig = withContext(Dispatchers.IO) {
        val path = File(modelPath).canonicalFile
        require(path.isFile && path.length() > 0L) { "找不到本地模型，请重新导入 .task 文件。" }
        synchronized(lock) {
            val maxContext = contextTokens.coerceIn(512, 8192)
            val safeTemperature = temperature.coerceIn(0f, 1.5f)
            val safeTopK = topK.coerceIn(1, 100)
            val safeTopP = topP.coerceIn(0.1f, 1f)
            if (inference == null || loadedPath != path.absolutePath || loadedContextTokens != maxContext) {
                // Free the old native model first; holding two large models can OOM lower-memory phones.
                inference?.close()
                inference = null
                loadedPath = ""
                loadedContextTokens = 0
                val options = LlmInference.LlmInferenceOptions.builder()
                    .setModelPath(path.absolutePath)
                    // MediaPipe maxTokens is the total KV-cache budget for input + output.
                    .setMaxTokens(maxContext)
                    .build()
                inference = LlmInference.createFromOptions(appContext, options)
                loadedPath = path.absolutePath
                loadedContextTokens = maxContext
            }
            this@MediaPipeLocalAI.temperature = safeTemperature
            this@MediaPipeLocalAI.topK = safeTopK
            this@MediaPipeLocalAI.topP = safeTopP
            if (localServer == null) {
                token = newToken()
                localServer = LocalModelHttpServer(token, ::generate).also { it.start() }
            }
        }
        AIConfig(kind = "mediapipe", baseUrl = BASE_URL, model = modelLabel.ifBlank { "local-model" }, apiKey = token)
    }

    /** Closes native model memory when the player switches away from local inference. */
    suspend fun deactivate() = withContext(Dispatchers.IO) {
        synchronized(lock) {
            localServer?.close()
            localServer = null
            inference?.close()
            inference = null
            loadedPath = ""
            loadedContextTokens = 0
        }
    }

    private fun generate(messages: List<Pair<String, String>>): String = synchronized(lock) {
        val model = inference ?: error("本地模型尚未加载，请在设置中应用模型配置。")
        val prompt = messages.joinToString("\n\n") { (role, text) ->
            val label = when (role.lowercase()) {
                "system" -> "SYSTEM"
                "assistant" -> "ASSISTANT"
                else -> "USER"
            }
            "$label:\n$text"
        } + "\n\nASSISTANT:\n"
        val options = LlmInferenceSession.LlmInferenceSessionOptions.builder()
            .setTemperature(temperature)
            .setTopK(topK)
            .setTopP(topP)
            .build()
        val session = LlmInferenceSession.createFromOptions(model, options)
        try {
            session.addQueryChunk(prompt)
            session.generateResponse()
        } finally {
            session.close()
        }
    }

    override fun close() {
        synchronized(lock) {
            localServer?.close()
            localServer = null
            inference?.close()
            inference = null
            loadedPath = ""
            loadedContextTokens = 0
        }
    }

    private fun newToken(): String {
        val bytes = ByteArray(24)
        random.nextBytes(bytes)
        return bytes.joinToString("") { (it.toInt() and 0xff).toString(16).padStart(2, '0') }
    }
}
