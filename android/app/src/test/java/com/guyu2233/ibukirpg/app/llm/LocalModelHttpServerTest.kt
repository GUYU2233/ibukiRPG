package com.guyu2233.ibukirpg.app.llm

import java.net.Socket
import java.nio.charset.StandardCharsets
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicBoolean
import java.util.concurrent.atomic.AtomicInteger
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class LocalModelHttpServerTest {
    @Test
    fun servesAuthorizedOpenAICompatibleChatAndSSE() {
        val server = LocalModelHttpServer("unit-test-key", { messages, _, _, emit ->
            check(messages.any { it.first == "user" && it.second == "你好" })
            listOf("本地", "回答").forEach { emit(it) }
            "本地回答"
        })
        server.start()
        try {
            assertTrue(server.boundPort > 0)
            val json = """{"model":"tiny.gguf","messages":[{"role":"user","content":"你好"}],"stream":false}"""
            assertTrue(request(server.boundPort, json, authorization = null).startsWith("HTTP/1.1 401"))

            val regular = request(server.boundPort, json, authorization = "Bearer unit-test-key")
            assertTrue(regular.startsWith("HTTP/1.1 200"))
            assertTrue(regular.contains("\"content\":\"本地回答\""))

            val streamed = request(server.boundPort, json.replace("false", "true"), authorization = "Bearer unit-test-key")
            assertTrue(streamed.startsWith("HTTP/1.1 200"))
            assertTrue(streamed.contains("text/event-stream"))
            // 逐 token 推送：两个 delta 分别出现，最后是 [DONE]
            val deltas = Regex("\"content\":\"([^\"]*)\"").findAll(streamed).map { it.groupValues[1] }.filter { it.isNotEmpty() }.toList()
            assertEquals(listOf("本地", "回答"), deltas)
            assertTrue(streamed.trimEnd().endsWith("data: [DONE]"))
        } finally {
            server.close()
        }
    }

    @Test
    fun passesSamplingOverrides() {
        var seen: Pair<Int?, Float?>? = null
        val server = LocalModelHttpServer("k", { _, maxTokens, temperature, _ -> seen = maxTokens to temperature; "ok" })
        server.start()
        try {
            val json = """{"model":"m","messages":[{"role":"user","content":"x"}],"max_tokens":77,"temperature":0.3}"""
            assertTrue(request(server.boundPort, json, "Bearer k").startsWith("HTTP/1.1 200"))
            assertEquals(77, seen?.first)
            assertEquals(0.3f, seen?.second ?: 0f, 1e-6f)
        } finally {
            server.close()
        }
    }

    @Test
    fun clientDisconnectStopsStreaming() {
        val stopped = AtomicBoolean(false)
        val emitted = AtomicInteger(0)
        val done = CountDownLatch(1)
        val server = LocalModelHttpServer("k", { _, _, _, emit ->
            try {
                var i = 0
                while (i < 10_000 && !stopped.get()) {
                    if (!emit("字".repeat(64))) stopped.set(true) else emitted.incrementAndGet()
                    Thread.sleep(1)
                    i++
                }
                ""
            } finally {
                done.countDown()
            }
        })
        server.start()
        try {
            val json = """{"model":"m","messages":[{"role":"user","content":"x"}],"stream":true}"""
            val body = json.toByteArray(StandardCharsets.UTF_8)
            Socket("127.0.0.1", server.boundPort).use { s ->
                s.getOutputStream().write(headers(server.boundPort, body.size, "Bearer k").toByteArray(StandardCharsets.ISO_8859_1))
                s.getOutputStream().write(body)
                s.getOutputStream().flush()
                s.getInputStream().read(ByteArray(256)) // 收到第一段后立刻断开
            }
            assertTrue("generation should finish after disconnect", done.await(10, TimeUnit.SECONDS))
            assertTrue(stopped.get())
            assertTrue(emitted.get() < 10_000)
        } finally {
            server.close()
        }
    }

    private fun headers(port: Int, size: Int, authorization: String?) = buildString {
        append("POST /v1/chat/completions HTTP/1.1\r\n")
        append("Host: 127.0.0.1:$port\r\n")
        append("Content-Type: application/json\r\n")
        append("Content-Length: $size\r\n")
        authorization?.let { append("Authorization: $it\r\n") }
        append("Connection: close\r\n\r\n")
    }

    private fun request(port: Int, body: String, authorization: String?): String {
        val bytes = body.toByteArray(StandardCharsets.UTF_8)
        return Socket("127.0.0.1", port).use { socket ->
            val out = socket.getOutputStream()
            out.write(headers(port, bytes.size, authorization).toByteArray(StandardCharsets.ISO_8859_1))
            out.write(bytes)
            out.flush()
            socket.getInputStream().bufferedReader(StandardCharsets.UTF_8).readText()
        }
    }
}
