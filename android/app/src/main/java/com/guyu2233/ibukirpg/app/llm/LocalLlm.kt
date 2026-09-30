package com.guyu2233.ibukirpg.app.llm

import java.io.File
import java.nio.charset.CodingErrorAction
import java.nio.charset.StandardCharsets
import java.util.concurrent.Executors
import java.util.concurrent.ScheduledFuture
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.locks.ReentrantLock
import kotlin.concurrent.withLock

/** 本地模型配置（设置页）。 */
data class LocalModelConfig(
    val path: String,
    val label: String = "",
    val contextTokens: Int = 2048,
    val threads: Int = 0,          // 0 = 自动（CPU 核数 - 2，2～4）
    val gpuLayers: Int = 0,        // 当前构建只有 CPU 后端，保留设置以便将来启用 Vulkan / OpenCL
    val temperature: Float = 0.7f,
    val topP: Float = 0.95f,
    val topK: Int = 40,
    val maxTokens: Int = 512,
) {
    fun normalized() = copy(
        contextTokens = contextTokens.coerceIn(512, 8192),
        threads = threads.coerceIn(0, 8),
        gpuLayers = gpuLayers.coerceIn(0, 999),
        temperature = temperature.coerceIn(0f, 1.5f),
        topP = topP.coerceIn(0.1f, 1f),
        topK = topK.coerceIn(1, 100),
        maxTokens = maxTokens.coerceIn(32, 2048),
    )

    /** 改变这些参数需要重新加载模型；采样参数可以随时生效。 */
    fun sameLoad(o: LocalModelConfig) = path == o.path && contextTokens == o.contextTokens && threads == o.threads && gpuLayers == o.gpuLayers
}

/** 可用内存信息（Android 端来自 ActivityManager.MemoryInfo，测试里用假数据）。 */
data class MemInfo(val availMem: Long, val totalMem: Long, val lowMemory: Boolean)

/** 推理开始 / 结束通知（Android 端用于启动 / 停止前台服务）。 */
interface GenerationListener {
    fun onGenerationStart() {}
    fun onGenerationEnd() {}
    fun onModelUnloaded(reason: String) {}
}

/**
 * 单实例的本地模型管理：加载前校验 GGUF 头与内存；同一时间只允许一个生成任务；
 * 空闲 [idleMillis] 后或系统内存告急时释放模型，下次请求时按原配置重新加载。
 * 所有方法都可能阻塞（加载 / 推理），调用方负责放在后台线程。
 */
class LocalLlm(
    private val native: NativeLlm,
    private val memInfo: () -> MemInfo,
    private val listener: GenerationListener = object : GenerationListener {},
    private val idleMillis: Long = 5 * 60 * 1000L,
    private val scheduler: java.util.concurrent.ScheduledExecutorService = Executors.newSingleThreadScheduledExecutor { r -> Thread(r, "ibuki-llm-idle").apply { isDaemon = true } },
) : AutoCloseable {
    private val lock = ReentrantLock()
    @Volatile private var handle = 0L
    @Volatile private var loaded: LocalModelConfig? = null
    @Volatile var config: LocalModelConfig? = null
        private set
    @Volatile var lastWarning: String? = null
        private set
    @Volatile private var busyHandle = 0L
    private var idleTask: ScheduledFuture<*>? = null
    private val seed = AtomicInteger(1234)

    val isLoaded: Boolean get() = handle != 0L
    val isBusy: Boolean get() = busyHandle != 0L

    /**
     * 校验并加载模型（已按相同参数加载时直接返回）。内存明显不足时抛出异常并给出中文原因。
     * [load] 为 false 时只校验文件并记住配置，第一次生成时再加载（应用启动时用，避免一打开就占用大量内存）。
     */
    fun configure(cfg: LocalModelConfig, load: Boolean = true) {
        val c = cfg.normalized()
        check(native.available) { "此设备的 CPU 架构没有打包 llama.cpp 原生库，无法使用本地模型。" }
        val file = File(c.path)
        when (val r = Gguf.check(file)) {
            is Gguf.Result.Bad -> throw IllegalArgumentException(r.reason)
            is Gguf.Result.Ok -> Unit
        }
        lock.withLock {
            config = c
            val cur = loaded
            if (!load) {
                if (handle != 0L && cur != null && !cur.sameLoad(c)) unloadLocked("reconfigure")
                return
            }
            if (handle != 0L && cur != null && cur.sameLoad(c)) {
                scheduleIdle()
                return
            }
            unloadLocked("reconfigure")
            loadLocked(c, file)
        }
    }

    private fun loadLocked(c: LocalModelConfig, file: File) {
        val m = memInfo()
        when (val d = MemoryPolicy.decide(file.length(), c.contextTokens, m.availMem, m.totalMem, m.lowMemory)) {
            is MemoryPolicy.Decision.Refuse -> throw IllegalStateException(d.message)
            is MemoryPolicy.Decision.Warn -> lastWarning = d.message
            is MemoryPolicy.Decision.Ok -> lastWarning = null
        }
        val h = native.load(c.path, c.contextTokens, c.threads, c.gpuLayers)
        check(h != 0L) { "llama.cpp 无法加载这个模型：可能是不支持的模型架构、文件损坏，或内存不足。" }
        handle = h
        loaded = c
        scheduleIdle()
    }

    /** 释放模型（切换到其他 AI 模式、空闲超时、内存告急）。正在生成时先取消。 */
    fun unload(reason: String) {
        val b = busyHandle
        if (b != 0L) native.cancel(b)
        lock.withLock { unloadLocked(reason) }
    }

    private fun unloadLocked(reason: String) {
        idleTask?.cancel(false)
        idleTask = null
        val h = handle
        if (h != 0L) {
            handle = 0L
            loaded = null
            native.free(h)
            listener.onModelUnloaded(reason)
        }
    }

    /** 彻底停用（不再自动重新加载）。 */
    fun deactivate() {
        unload("deactivate")
        config = null
    }

    /** 取消当前生成（不释放模型）。 */
    fun cancel() {
        val b = busyHandle
        if (b != 0L) native.cancel(b)
    }

    /**
     * 流式生成。[emit] 返回 false（例如客户端已断开）会停止生成。
     * 返回生成的完整文本；取消时返回已生成的部分。
     */
    fun generate(messages: List<Pair<String, String>>, maxTokens: Int? = null, temperature: Float? = null, emit: (String) -> Boolean = { true }): String {
        val c = config ?: error("本地模型尚未启用，请在设置中应用模型配置。")
        lock.withLock {
            if (handle == 0L) loadLocked(c, File(c.path))   // 空闲 / 内存告急后按需重新加载
            idleTask?.cancel(false)
            val h = handle
            val prompt = buildPrompt(h, messages)
            val out = StringBuilder()
            val decoder = StandardCharsets.UTF_8.newDecoder()
                .onMalformedInput(CodingErrorAction.REPLACE)
                .onUnmappableCharacter(CodingErrorAction.REPLACE)
            busyHandle = h
            listener.onGenerationStart()
            try {
                val rc = native.generate(
                    h, prompt,
                    (maxTokens ?: c.maxTokens).coerceIn(16, c.maxTokens.coerceAtLeast(16)),
                    temperature?.coerceIn(0f, 1.5f) ?: c.temperature,
                    c.topP, c.topK, seed.incrementAndGet(),
                ) { bytes ->
                    val text = decoder.decode(java.nio.ByteBuffer.wrap(bytes)).toString()
                    out.append(text)
                    emit(text)
                }
                if (rc < 0) error("本地模型推理失败（代码 $rc）。可以尝试降低上下文长度或换一个模型。")
            } finally {
                busyHandle = 0L
                listener.onGenerationEnd()
                scheduleIdle()
            }
            return out.toString()
        }
    }

    /** 优先使用模型自带的聊天模板；不支持时退回通用格式（按模板特征选 ChatML / Gemma / Llama 3）。 */
    internal fun buildPrompt(h: Long, messages: List<Pair<String, String>>): ByteArray {
        val roles = messages.map { it.first.lowercase().let { r -> if (r == "system" || r == "assistant") r else "user" } }.toTypedArray()
        val contents = messages.map { it.second.toByteArray(StandardCharsets.UTF_8) }.toTypedArray()
        native.applyTemplate(h, roles, contents)?.let { return it }
        return ChatPrompt.fallback(messages, native.chatTemplate(h)).toByteArray(StandardCharsets.UTF_8)
    }

    /** onTrimMemory：内存告急（RUNNING_CRITICAL）或进程即将被回收（COMPLETE）时释放模型。 */
    fun onTrimMemory(level: Int) {
        if (shouldUnloadOnTrim(level)) unload("trim:$level")
    }

    private fun scheduleIdle() {
        idleTask?.cancel(false)
        if (idleMillis > 0 && handle != 0L) {
            idleTask = scheduler.schedule({ if (!isBusy) unload("idle") }, idleMillis, TimeUnit.MILLISECONDS)
        }
    }

    override fun close() {
        deactivate()
        scheduler.shutdownNow()
    }

    companion object {
        // 与 android.content.ComponentCallbacks2 的常量一致（这里不依赖 Android 类，便于 JVM 单元测试）
        const val TRIM_MEMORY_RUNNING_CRITICAL = 15
        const val TRIM_MEMORY_COMPLETE = 80

        fun shouldUnloadOnTrim(level: Int) = level == TRIM_MEMORY_RUNNING_CRITICAL || level >= TRIM_MEMORY_COMPLETE
    }
}
