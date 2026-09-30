package com.guyu2233.ibukirpg.app.llm

import com.guyu2233.ibukirpg.app.llm.MemoryPolicy.MB
import java.io.File
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 假的原生层：记录调用，按脚本产出 token。 */
private class FakeNative(var template: Boolean = true) : NativeLlm {
    override var available = true
    val loads = AtomicInteger(0)
    val frees = AtomicInteger(0)
    val live = AtomicInteger(0)
    private val next = AtomicInteger(1)
    val cancelled = AtomicBoolean(false)
    var tokens = listOf("你", "好", "。")
    var tokenDelayMs = 0L
    var started: CountDownLatch? = null
    var lastPrompt: String = ""
    var failLoad = false

    override fun init(nativeLibDir: String) {}
    override fun load(path: String, contextTokens: Int, threads: Int, gpuLayers: Int): Long {
        if (failLoad) return 0L
        loads.incrementAndGet(); live.incrementAndGet()
        check(live.get() == 1) { "more than one model instance" }
        return next.getAndIncrement().toLong()
    }
    override fun contextSize(handle: Long) = 2048
    override fun free(handle: Long) { frees.incrementAndGet(); live.decrementAndGet() }
    override fun cancel(handle: Long) { cancelled.set(true) }
    override fun chatTemplate(handle: Long): String? = null
    override fun applyTemplate(handle: Long, roles: Array<String>, contents: Array<ByteArray>): ByteArray? =
        if (!template) null else roles.indices.joinToString("|") { roles[it] + ":" + String(contents[it]) }.toByteArray()
    override fun generate(handle: Long, prompt: ByteArray, maxTokens: Int, temperature: Float, topP: Float, topK: Int, seed: Int, onBytes: TokenSink): Int {
        lastPrompt = String(prompt)
        cancelled.set(false)
        started?.countDown()
        for (t in tokens) {
            if (cancelled.get()) return 1
            if (tokenDelayMs > 0) Thread.sleep(tokenDelayMs)
            if (cancelled.get()) return 1
            if (!onBytes.onBytes(t.toByteArray())) return 1
        }
        return 0
    }
}

class LocalLlmTest {
    private lateinit var model: File
    private val gb = 1024 * MB
    private var mem = MemInfo(availMem = 6 * gb, totalMem = 8 * gb, lowMemory = false)
    private val events = mutableListOf<String>()
    private val listener = object : GenerationListener {
        override fun onGenerationStart() { synchronized(events) { events += "start" } }
        override fun onGenerationEnd() { synchronized(events) { events += "end" } }
        override fun onModelUnloaded(reason: String) { synchronized(events) { events += "unload:$reason" } }
    }

    @Before fun setUp() {
        model = File.createTempFile("tiny", ".gguf")
        val head = ByteBuffer.allocate(24).order(ByteOrder.LITTLE_ENDIAN).apply {
            put("GGUF".toByteArray()); putInt(3); putLong(4); putLong(8)
        }.array()
        model.writeBytes(head + ByteArray(1024))
    }

    @After fun tearDown() { model.delete() }

    private fun llm(native: FakeNative, idle: Long = 0) = LocalLlm(native, { mem }, listener, idleMillis = idle)
    private fun cfg() = LocalModelConfig(path = model.path, contextTokens = 2048)

    @Test fun streamsTokensAndUsesModelTemplate() {
        val n = FakeNative()
        llm(n).use { l ->
            l.configure(cfg())
            val seen = mutableListOf<String>()
            val out = l.generate(listOf("system" to "旁白", "user" to "你好")) { seen += it; true }
            assertEquals("你好。", out)
            assertEquals(listOf("你", "好", "。"), seen)
            assertEquals("system:旁白|user:你好", n.lastPrompt)
            assertEquals(listOf("start", "end"), events)
        }
    }

    @Test fun fallsBackToChatMLWithoutTemplate() {
        val n = FakeNative(template = false)
        llm(n).use { l ->
            l.configure(cfg())
            l.generate(listOf("user" to "你好"))
            assertTrue(n.lastPrompt.startsWith("<|im_start|>user\n你好<|im_end|>"))
        }
    }

    @Test fun singleInstanceAndReloadOnlyWhenLoadParamsChange() {
        val n = FakeNative()
        llm(n).use { l ->
            l.configure(cfg())
            l.configure(cfg().copy(temperature = 0.2f))   // 采样参数：不重新加载
            assertEquals(1, n.loads.get())
            l.configure(cfg().copy(contextTokens = 4096)) // 上下文：先释放再加载
            assertEquals(2, n.loads.get())
            assertEquals(1, n.frees.get())
            assertEquals(1, n.live.get())
        }
        assertEquals(0, n.live.get())
    }

    @Test fun lazyLoadAndReloadAfterUnload() {
        val n = FakeNative()
        llm(n).use { l ->
            l.configure(cfg(), load = false)
            assertFalse(l.isLoaded)
            assertEquals(0, n.loads.get())
            l.generate(listOf("user" to "x"))
            assertTrue(l.isLoaded)
            l.unload("test")
            assertFalse(l.isLoaded)
            l.generate(listOf("user" to "x"))
            assertEquals(2, n.loads.get())
        }
    }

    @Test fun idleUnload() {
        val n = FakeNative()
        llm(n, idle = 100).use { l ->
            l.configure(cfg())
            assertTrue(l.isLoaded)
            val deadline = System.currentTimeMillis() + 5000
            while (l.isLoaded && System.currentTimeMillis() < deadline) Thread.sleep(20)
            assertFalse(l.isLoaded)
            assertTrue(events.contains("unload:idle"))
        }
    }

    @Test fun trimMemoryLevels() {
        assertTrue(LocalLlm.shouldUnloadOnTrim(LocalLlm.TRIM_MEMORY_RUNNING_CRITICAL))
        assertTrue(LocalLlm.shouldUnloadOnTrim(LocalLlm.TRIM_MEMORY_COMPLETE))
        assertFalse(LocalLlm.shouldUnloadOnTrim(5))   // RUNNING_MODERATE
        assertFalse(LocalLlm.shouldUnloadOnTrim(20))  // UI_HIDDEN：切到后台不立即释放
        assertFalse(LocalLlm.shouldUnloadOnTrim(40))
        val n = FakeNative()
        llm(n).use { l ->
            l.configure(cfg())
            l.onTrimMemory(20)
            assertTrue(l.isLoaded)
            l.onTrimMemory(15)
            assertFalse(l.isLoaded)
        }
    }

    @Test fun cancelStopsGenerationAndReturnsPartial() {
        val n = FakeNative().apply { tokens = List(200) { "字" }; tokenDelayMs = 5; started = CountDownLatch(1) }
        llm(n).use { l ->
            l.configure(cfg())
            var result: String? = null
            val t = Thread { result = l.generate(listOf("user" to "x")) }
            t.start()
            assertTrue(n.started!!.await(5, TimeUnit.SECONDS))
            Thread.sleep(30)
            assertTrue(l.isBusy)
            l.cancel()
            t.join(5000)
            assertFalse(t.isAlive)
            assertTrue(result!!.length in 1 until 200)
            assertFalse(l.isBusy)
            assertTrue(l.isLoaded) // 取消不释放模型
        }
    }

    @Test fun emitFalseStops() {
        val n = FakeNative().apply { tokens = List(50) { "a" } }
        llm(n).use { l ->
            l.configure(cfg())
            var count = 0
            val out = l.generate(listOf("user" to "x")) { ++count < 3 }
            assertEquals("aaa", out)
        }
    }

    @Test fun memoryRefuseAndWarn() {
        val n = FakeNative()
        llm(n).use { l ->
            mem = MemInfo(availMem = 100 * MB, totalMem = 400 * MB, lowMemory = true)
            val e = assertThrows(IllegalStateException::class.java) { l.configure(cfg()) }
            assertTrue(e.message!!.contains("内存"))
            assertEquals(0, n.loads.get())

            mem = MemInfo(availMem = 200 * MB, totalMem = 8 * gb, lowMemory = false)
            l.configure(cfg())
            assertNotNull(l.lastWarning)
            assertTrue(l.isLoaded)

            mem = MemInfo(availMem = 6 * gb, totalMem = 8 * gb, lowMemory = false)
            l.configure(cfg().copy(contextTokens = 1024))
            assertNull(l.lastWarning)
        }
    }

    @Test fun rejectsBadFileBeforeNativeLoad() {
        val n = FakeNative()
        llm(n).use { l ->
            model.writeBytes("PK\u0003\u0004 this is a zip".toByteArray())
            assertThrows(IllegalArgumentException::class.java) { l.configure(cfg()) }
            assertEquals(0, n.loads.get())
        }
    }

    @Test fun unavailableNativeLibrary() {
        val n = FakeNative().apply { available = false }
        llm(n).use { l -> assertThrows(IllegalStateException::class.java) { l.configure(cfg()) } }
    }

    @Test fun nativeLoadFailureIsReported() {
        val n = FakeNative().apply { failLoad = true }
        llm(n).use { l ->
            assertThrows(IllegalStateException::class.java) { l.configure(cfg()) }
            assertFalse(l.isLoaded)
        }
    }

    @Test fun deactivateForgetsConfig() {
        val n = FakeNative()
        llm(n).use { l ->
            l.configure(cfg())
            l.deactivate()
            assertFalse(l.isLoaded)
            assertThrows(IllegalStateException::class.java) { l.generate(listOf("user" to "x")) }
        }
    }
}
