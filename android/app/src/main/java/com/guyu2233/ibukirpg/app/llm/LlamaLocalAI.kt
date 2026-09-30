package com.guyu2233.ibukirpg.app.llm

import android.app.ActivityManager
import android.content.Context
import android.util.Log
import com.guyu2233.ibukirpg.app.data.AIConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.withContext
import java.security.SecureRandom

/**
 * Android 端的本地模型（llama.cpp）入口：管理 [LocalLlm]，并通过仅监听 127.0.0.1 的
 * OpenAI 兼容接口把它交给 Go 引擎（kind = "llamacpp"，引擎对本地模型不做函数调用，改用关键词预取）。
 *
 * 生成期间启动前台服务（[InferenceService]），避免切到后台时被系统杀掉；空闲 5 分钟或内存告急时释放模型。
 */
class LlamaLocalAI(context: Context) : AutoCloseable {
    companion object {
        const val KIND = "llamacpp"
        private const val TAG = "ibuki-llama"
        private val random = SecureRandom()
    }

    private val appContext = context.applicationContext
    private val _busy = MutableStateFlow(false)
    /** 是否正在生成（前台服务据此决定何时结束）。 */
    val busy: StateFlow<Boolean> = _busy.asStateFlow()
    private val _status = MutableStateFlow("")
    /** 给设置页显示的状态（已加载 / 已因空闲释放 / 警告…）。 */
    val status: StateFlow<String> = _status.asStateFlow()

    private val llm = LocalLlm(
        native = LlamaNative,
        memInfo = ::memInfo,
        listener = object : GenerationListener {
            override fun onGenerationStart() {
                _busy.value = true
                InferenceService.start(appContext)
            }
            override fun onGenerationEnd() { _busy.value = false }
            override fun onModelUnloaded(reason: String) {
                Log.i(TAG, "model unloaded: $reason")
                _status.value = when {
                    reason == "idle" -> "模型空闲，已释放内存（下次生成时自动重新加载）"
                    reason.startsWith("trim") -> "系统内存不足，已释放模型（下次生成时自动重新加载）"
                    else -> ""
                }
            }
        },
    )
    @Volatile private var server: LocalModelHttpServer? = null
    @Volatile private var token: String = newToken()
    @Volatile private var initialized = false

    val available: Boolean get() = LlamaNative.available
    val lastWarning: String? get() = llm.lastWarning

    fun memInfo(): MemInfo {
        val am = appContext.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        val mi = ActivityManager.MemoryInfo()
        am.getMemoryInfo(mi)
        return MemInfo(mi.availMem, mi.totalMem, mi.lowMemory)
    }

    /** 校验 GGUF、检查内存、加载模型（后台线程），并返回交给 Go 引擎的配置。 */
    suspend fun configure(cfg: LocalModelConfig, load: Boolean = true): AIConfig = withContext(Dispatchers.IO) {
        check(LlamaNative.available) { "此设备的 CPU 架构没有打包 llama.cpp 原生库，无法使用本地模型。" }
        if (!initialized) {
            LlamaNative.init(appContext.applicationInfo.nativeLibraryDir)
            initialized = true
        }
        llm.configure(cfg, load)
        _status.value = if (load) llm.lastWarning ?: "模型已加载" else "模型将在第一次生成时加载"
        val srv = server ?: synchronized(this@LlamaLocalAI) {
            server ?: LocalModelHttpServer(token, LocalGenerate { messages, maxTokens, temperature, emit ->
                llm.generate(messages, maxTokens, temperature, emit)
            }).also { it.start(); server = it }
        }
        AIConfig(kind = KIND, baseUrl = "http://127.0.0.1:${srv.boundPort}/v1", model = cfg.label.ifBlank { "local-gguf" }, apiKey = token)
    }

    /** 取消正在进行的生成（通知栏“停止”按钮）。 */
    fun cancel() = llm.cancel()

    /** 切换到其他 AI 模式：释放模型并关闭回环服务。 */
    suspend fun deactivate() = withContext(Dispatchers.IO) {
        synchronized(this@LlamaLocalAI) {
            server?.close()
            server = null
            token = newToken()
        }
        llm.deactivate()
        _status.value = ""
    }

    /** Application.onTrimMemory 转发到这里。 */
    fun onTrimMemory(level: Int) = llm.onTrimMemory(level)

    override fun close() {
        server?.close()
        server = null
        llm.close()
    }

    private fun newToken(): String {
        val bytes = ByteArray(24)
        random.nextBytes(bytes)
        return bytes.joinToString("") { (it.toInt() and 0xff).toString(16).padStart(2, '0') }
    }
}
