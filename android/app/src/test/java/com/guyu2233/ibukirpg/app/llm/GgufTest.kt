package com.guyu2233.ibukirpg.app.llm

import java.io.File
import java.nio.ByteBuffer
import java.nio.ByteOrder
import org.junit.Assert.assertTrue
import org.junit.Test

class GgufTest {
    private fun header(magic: String = "GGUF", version: Int = 3, tensors: Long = 10, kv: Long = 20): ByteArray =
        ByteBuffer.allocate(24).order(ByteOrder.LITTLE_ENDIAN).apply {
            put(magic.toByteArray(Charsets.US_ASCII)); putInt(version); putLong(tensors); putLong(kv)
        }.array()

    @Test fun acceptsValidHeader() {
        assertTrue(Gguf.check(header(), 10_000) is Gguf.Result.Ok)
        assertTrue(Gguf.check(header(version = 2), 10_000) is Gguf.Result.Ok)
    }

    @Test fun rejectsWrongMagic() {
        val r = Gguf.check(header(magic = "PK\u0003\u0004"), 10_000)
        assertTrue(r is Gguf.Result.Bad && r.reason.contains("GGUF"))
    }

    @Test fun rejectsBadVersionAndCounts() {
        assertTrue(Gguf.check(header(version = 1), 10_000) is Gguf.Result.Bad)
        assertTrue(Gguf.check(header(version = 99), 10_000) is Gguf.Result.Bad)
        assertTrue(Gguf.check(header(tensors = 0), 10_000) is Gguf.Result.Bad)
        assertTrue(Gguf.check(header(tensors = -5), 10_000) is Gguf.Result.Bad)
        assertTrue(Gguf.check(header(kv = 10_000_000), 10_000) is Gguf.Result.Bad)
    }

    @Test fun rejectsTruncated() {
        assertTrue(Gguf.check(header(tensors = 1000), 100) is Gguf.Result.Bad)
        assertTrue(Gguf.check(ByteArray(10), 10) is Gguf.Result.Bad)
    }

    @Test fun checksFiles() {
        val missing = File("/nonexistent/model.gguf")
        assertTrue(Gguf.check(missing) is Gguf.Result.Bad)
        val f = File.createTempFile("model", ".gguf")
        try {
            f.writeBytes(header() + ByteArray(4096))
            assertTrue(Gguf.check(f) is Gguf.Result.Ok)
            f.writeBytes("not a model at all, just text....".toByteArray())
            assertTrue(Gguf.check(f) is Gguf.Result.Bad)
        } finally {
            f.delete()
        }
    }
}
