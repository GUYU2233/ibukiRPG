package com.guyu2233.ibukirpg.app.llm

import java.io.File
import java.io.RandomAccessFile
import java.nio.ByteBuffer
import java.nio.ByteOrder

/**
 * GGUF 文件头的轻量校验：在把文件交给 llama.cpp 之前先排除明显损坏 / 格式不对的文件，
 * 避免原生代码在解析异常数据时崩溃（原生崩溃无法被 Kotlin 捕获）。
 *
 * 文件头布局（小端）：magic "GGUF" | uint32 version | uint64 tensor_count | uint64 metadata_kv_count
 */
object Gguf {
    const val MIN_BYTES = 24L
    private const val MAX_TENSORS = 1_000_000L
    private const val MAX_KV = 100_000L

    data class Header(val version: Int, val tensorCount: Long, val kvCount: Long)

    sealed interface Result {
        data class Ok(val header: Header) : Result
        data class Bad(val reason: String) : Result
    }

    fun check(file: File): Result {
        if (!file.isFile) return Result.Bad("找不到模型文件，请重新导入。")
        if (file.length() < MIN_BYTES) return Result.Bad("模型文件太小，可能没有复制完整。")
        val head = ByteArray(MIN_BYTES.toInt())
        RandomAccessFile(file, "r").use { it.readFully(head) }
        return check(head, file.length())
    }

    fun check(head: ByteArray, fileSize: Long): Result {
        if (head.size < MIN_BYTES) return Result.Bad("模型文件太小，可能没有复制完整。")
        if (head[0] != 'G'.code.toByte() || head[1] != 'G'.code.toByte() || head[2] != 'U'.code.toByte() || head[3] != 'F'.code.toByte()) {
            return Result.Bad("这不是 GGUF 模型文件（文件头不是 GGUF）。MediaPipe .task、safetensors 等格式需要先转换为 GGUF。")
        }
        val b = ByteBuffer.wrap(head).order(ByteOrder.LITTLE_ENDIAN)
        val version = b.getInt(4)
        if (version !in 2..3) return Result.Bad("不支持的 GGUF 版本 $version（支持 v2 / v3）。")
        val tensors = b.getLong(8)
        val kv = b.getLong(16)
        if (tensors <= 0 || tensors > MAX_TENSORS) return Result.Bad("GGUF 文件头损坏（张量数 $tensors 不合理）。")
        if (kv < 0 || kv > MAX_KV) return Result.Bad("GGUF 文件头损坏（元数据条目数 $kv 不合理）。")
        // 每个张量描述至少几十字节，文件过小说明被截断
        if (fileSize < MIN_BYTES + tensors * 24) return Result.Bad("模型文件不完整（可能下载或复制中断）。")
        return Result.Ok(Header(version, tensors, kv))
    }
}
