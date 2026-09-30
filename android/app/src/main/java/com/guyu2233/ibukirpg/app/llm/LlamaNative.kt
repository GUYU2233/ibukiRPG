package com.guyu2233.ibukirpg.app.llm

import android.util.Log

/** 原生推理接口（便于在单元测试里替换成假实现）。所有方法都会阻塞，必须在后台线程调用。 */
interface NativeLlm {
    val available: Boolean
    fun init(nativeLibDir: String)
    fun load(path: String, contextTokens: Int, threads: Int, gpuLayers: Int): Long
    fun contextSize(handle: Long): Int
    fun free(handle: Long)
    fun cancel(handle: Long)
    fun chatTemplate(handle: Long): String?
    fun applyTemplate(handle: Long, roles: Array<String>, contents: Array<ByteArray>): ByteArray?
    /** 返回 0 正常结束，1 被取消，负数为错误。 */
    fun generate(handle: Long, prompt: ByteArray, maxTokens: Int, temperature: Float, topP: Float, topK: Int, seed: Int, onBytes: TokenSink): Int
}

/** 原生层每生成一段完整的 UTF-8 文本就回调一次；返回 false 立即停止生成。 */
fun interface TokenSink {
    fun onBytes(bytes: ByteArray): Boolean
}

/** llama.cpp JNI 绑定（libibuki_llama.so）。当前 ABI 没有打包原生库时 [available] 为 false。 */
object LlamaNative : NativeLlm {
    private const val TAG = "ibuki-llama"

    override val available: Boolean by lazy {
        runCatching { System.loadLibrary("ibuki_llama"); true }
            .onFailure { Log.w(TAG, "llama.cpp 原生库不可用（此 CPU 架构未打包？）", it) }
            .getOrDefault(false)
    }

    override fun init(nativeLibDir: String) = nativeInit(nativeLibDir)
    override fun load(path: String, contextTokens: Int, threads: Int, gpuLayers: Int) = nativeLoad(path, contextTokens, threads, gpuLayers)
    override fun contextSize(handle: Long) = nativeContextSize(handle)
    override fun free(handle: Long) = nativeFree(handle)
    override fun cancel(handle: Long) = nativeCancel(handle)
    override fun chatTemplate(handle: Long): String? = nativeChatTemplate(handle)
    override fun applyTemplate(handle: Long, roles: Array<String>, contents: Array<ByteArray>): ByteArray? = nativeApplyTemplate(handle, roles, contents)
    override fun generate(handle: Long, prompt: ByteArray, maxTokens: Int, temperature: Float, topP: Float, topK: Int, seed: Int, onBytes: TokenSink) =
        nativeGenerate(handle, prompt, maxTokens, temperature, topP, topK, seed, onBytes)

    fun systemInfo(): String = if (available) nativeSystemInfo() else ""

    @JvmStatic private external fun nativeInit(libDir: String)
    @JvmStatic private external fun nativeSystemInfo(): String
    @JvmStatic private external fun nativeLoad(path: String, nCtx: Int, nThreads: Int, nGpuLayers: Int): Long
    @JvmStatic private external fun nativeContextSize(handle: Long): Int
    @JvmStatic private external fun nativeFree(handle: Long)
    @JvmStatic private external fun nativeCancel(handle: Long)
    @JvmStatic private external fun nativeChatTemplate(handle: Long): String?
    @JvmStatic private external fun nativeApplyTemplate(handle: Long, roles: Array<String>, contents: Array<ByteArray>): ByteArray?
    @JvmStatic private external fun nativeGenerate(handle: Long, prompt: ByteArray, maxTokens: Int, temp: Float, topP: Float, topK: Int, seed: Int, callback: TokenSink): Int
}
