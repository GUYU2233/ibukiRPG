package com.guyu2233.ibukirpg.app.llm

/**
 * 加载前的内存检查（纯函数，便于单元测试）。
 *
 * 估算：权重以 mmap 映射，运行时实际驻留约等于文件大小；再加上 KV 缓存（与上下文长度成正比）与计算缓冲。
 * 与 ActivityManager.MemoryInfo 的 availMem / totalMem / lowMemory 比较。
 */
object MemoryPolicy {
    const val MB = 1024L * 1024L

    sealed interface Decision {
        val estimateBytes: Long
        data class Ok(override val estimateBytes: Long) : Decision
        data class Warn(override val estimateBytes: Long, val message: String) : Decision
        data class Refuse(override val estimateBytes: Long, val message: String) : Decision
    }

    /** KV 缓存粗估：按 1.5B～3B 量级模型每 token 约 0.1 MB（f16 KV）估算，外加 256 MB 计算缓冲与运行时开销。 */
    fun estimate(modelBytes: Long, contextTokens: Int): Long =
        modelBytes + contextTokens.toLong() * 100 * 1024 + 256 * MB

    fun decide(modelBytes: Long, contextTokens: Int, availMem: Long, totalMem: Long, lowMemory: Boolean): Decision {
        val need = estimate(modelBytes, contextTokens)
        val gb = { v: Long -> "%.1f GB".format(v.toDouble() / (1024 * MB)) }
        return when {
            // 模型比整机内存的 60% 还大：几乎必然被系统杀掉
            need > totalMem * 6 / 10 -> Decision.Refuse(need, "这个模型大约需要 ${gb(need)} 内存，本机总内存只有 ${gb(totalMem)}。请换用更小的量化版本（例如 1.5B Q4_K_M）或降低上下文长度。")
            lowMemory || need > availMem -> Decision.Warn(need, "当前可用内存约 ${gb(availMem)}，模型大约需要 ${gb(need)}。可以尝试加载，但系统可能会关闭后台应用，推理也可能很慢；建议先关闭其他应用或降低上下文长度。")
            else -> Decision.Ok(need)
        }
    }
}
