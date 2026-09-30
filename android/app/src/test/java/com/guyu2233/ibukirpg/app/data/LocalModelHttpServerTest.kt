package com.guyu2233.ibukirpg.app.data

import java.net.Socket
import java.nio.charset.StandardCharsets
import org.junit.Assert.assertTrue
import org.junit.Test

class LocalModelHttpServerTest {
    @Test
    fun servesAuthorizedOpenAICompatibleChatAndSSE() {
        val server = LocalModelHttpServer("unit-test-key") { messages ->
            check(messages.any { it.first == "user" && it.second == "你好" })
            "本地回答"
        }
        server.start()
        try {
            val json = """{"model":"tiny.task","messages":[{"role":"user","content":"你好"}],"stream":false}"""
            val response = request(json, authorization = null)
            assertTrue(response.startsWith("HTTP/1.1 401"))

            val regular = request(json, authorization = "Bearer unit-test-key")
            assertTrue(regular.startsWith("HTTP/1.1 200"))
            assertTrue(regular.contains("\"content\":\"本地回答\""))

            val streamed = request(json.replace("false", "true"), authorization = "Bearer unit-test-key")
            assertTrue(streamed.startsWith("HTTP/1.1 200"))
            assertTrue(streamed.contains("text/event-stream"))
            assertTrue(streamed.contains("data: [DONE]"))
        } finally {
            server.close()
        }
    }

    private fun request(body: String, authorization: String?): String {
        val bytes = body.toByteArray(StandardCharsets.UTF_8)
        return Socket("127.0.0.1", LocalModelHttpServer.PORT).use { socket ->
            val out = socket.getOutputStream()
            val headers = buildString {
                append("POST /v1/chat/completions HTTP/1.1\r\n")
                append("Host: 127.0.0.1:${LocalModelHttpServer.PORT}\r\n")
                append("Content-Type: application/json\r\n")
                append("Content-Length: ${bytes.size}\r\n")
                authorization?.let { append("Authorization: $it\r\n") }
                append("Connection: close\r\n\r\n")
            }
            out.write(headers.toByteArray(StandardCharsets.ISO_8859_1))
            out.write(bytes)
            out.flush()
            socket.getInputStream().bufferedReader(StandardCharsets.UTF_8).readText()
        }
    }
}
