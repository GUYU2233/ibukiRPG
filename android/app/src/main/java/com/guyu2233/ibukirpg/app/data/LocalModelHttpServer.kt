package com.guyu2233.ibukirpg.app.data

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.put
import java.io.BufferedInputStream
import java.io.ByteArrayOutputStream
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import java.net.Socket
import java.nio.charset.StandardCharsets
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

/** Loopback-only OpenAI Chat Completions bridge used by the Go engine. */
internal class LocalModelHttpServer(
    private val apiKey: String,
    private val generate: (List<Pair<String, String>>) -> String,
) : AutoCloseable {
    companion object {
        const val PORT = 37123
        private const val MAX_HEADER_BYTES = 32 * 1024
        private const val MAX_BODY_BYTES = 4 * 1024 * 1024
        private val json = Json { ignoreUnknownKeys = true }
    }

    private val running = AtomicBoolean(false)
    private val workers = Executors.newCachedThreadPool { r -> Thread(r, "ibuki-local-llm-request").apply { isDaemon = true } }
    @Volatile private var listener: ServerSocket? = null
    @Volatile private var acceptThread: Thread? = null

    @Synchronized
    fun start() {
        if (running.get()) return
        val server = ServerSocket()
        server.reuseAddress = true
        server.bind(InetSocketAddress(InetAddress.getByName("127.0.0.1"), PORT), 8)
        listener = server
        running.set(true)
        acceptThread = Thread({
            while (running.get()) {
                try {
                    val socket = server.accept()
                    workers.execute { handle(socket) }
                } catch (_: Exception) {
                    if (running.get()) continue
                }
            }
        }, "ibuki-local-llm-loopback").apply { isDaemon = true; start() }
    }

    private fun handle(socket: Socket) {
        socket.use { client ->
            runCatching {
                client.soTimeout = 5 * 60 * 1000
                val input = BufferedInputStream(client.getInputStream())
                val header = readHeader(input) ?: return
                val lines = header.split("\r\n")
                val request = lines.firstOrNull()?.split(' ') ?: return writeError(client, 400, "Malformed HTTP request")
                val method = request.getOrNull(0).orEmpty()
                val path = request.getOrNull(1).orEmpty().substringBefore('?')
                val headers = lines.drop(1).mapNotNull { line ->
                    val i = line.indexOf(':')
                    if (i <= 0) null else line.substring(0, i).trim().lowercase() to line.substring(i + 1).trim()
                }.toMap()

                if (method == "GET" && path == "/health") {
                    writeJson(client, 200, buildJsonObject { put("ok", true) }.toString())
                    return
                }
                if (method != "POST" || path != "/v1/chat/completions") {
                    writeError(client, 404, "Not found")
                    return
                }
                if (headers["authorization"] != "Bearer $apiKey") {
                    writeError(client, 401, "Local model authorization failed")
                    return
                }
                val length = headers["content-length"]?.toIntOrNull()
                if (length == null || length !in 1..MAX_BODY_BYTES) {
                    writeError(client, 413, "Request body is missing or too large")
                    return
                }
                val body = ByteArray(length)
                var offset = 0
                while (offset < length) {
                    val n = input.read(body, offset, length - offset)
                    if (n < 0) return writeError(client, 400, "Incomplete request body")
                    offset += n
                }
                val root = json.parseToJsonElement(String(body, StandardCharsets.UTF_8)).jsonObject
                val messages = root["messages"]?.jsonArray?.mapNotNull { element ->
                    val item = element.jsonObject
                    val role = item["role"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null
                    val content = item["content"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null
                    role to content
                }.orEmpty()
                if (messages.isEmpty()) return writeError(client, 400, "No prompt messages")

                val answer = generate(messages)
                val model = root["model"]?.jsonPrimitive?.contentOrNull ?: "mediapipe-local"
                if (root["stream"]?.jsonPrimitive?.contentOrNull == "true" || root["stream"]?.toString() == "true") {
                    val sse = buildString {
                        append("data: ").append(streamChunk(model, answer)).append("\n\n")
                        append("data: [DONE]\n\n")
                    }
                    writeResponse(client, 200, "text/event-stream; charset=utf-8", sse.toByteArray(StandardCharsets.UTF_8))
                } else {
                    writeJson(client, 200, completion(model, answer).toString())
                }
            }.onFailure { error ->
                runCatching { writeError(client, 500, error.message ?: "Local inference failed") }
            }
        }
    }

    private fun readHeader(input: BufferedInputStream): String? {
        val out = ByteArrayOutputStream()
        var matched = 0
        while (out.size() < MAX_HEADER_BYTES) {
            val b = input.read()
            if (b < 0) return null
            out.write(b)
            matched = when {
                matched == 0 && b == '\r'.code -> 1
                matched == 1 && b == '\n'.code -> 2
                matched == 2 && b == '\r'.code -> 3
                matched == 3 && b == '\n'.code -> 4
                b == '\r'.code -> 1
                else -> 0
            }
            if (matched == 4) {
                val bytes = out.toByteArray()
                return String(bytes, 0, bytes.size - 4, StandardCharsets.ISO_8859_1)
            }
        }
        throw IllegalArgumentException("HTTP headers too large")
    }

    private fun completion(model: String, text: String) = buildJsonObject {
        put("id", "chatcmpl-ibuki-local")
        put("object", "chat.completion")
        put("model", model)
        put("choices", buildJsonArray {
            add(buildJsonObject {
                put("index", 0)
                put("message", buildJsonObject { put("role", "assistant"); put("content", text) })
                put("finish_reason", "stop")
            })
        })
    }

    private fun streamChunk(model: String, text: String) = buildJsonObject {
        put("id", "chatcmpl-ibuki-local")
        put("object", "chat.completion.chunk")
        put("model", model)
        put("choices", buildJsonArray {
            add(buildJsonObject {
                put("index", 0)
                put("delta", buildJsonObject { put("content", text) })
                put("finish_reason", "stop")
            })
        })
    }.toString()

    private fun writeJson(socket: Socket, status: Int, body: String) = writeResponse(
        socket, status, "application/json; charset=utf-8", body.toByteArray(StandardCharsets.UTF_8),
    )

    private fun writeError(socket: Socket, status: Int, message: String) = writeJson(
        socket, status, buildJsonObject { put("error", buildJsonObject { put("message", message) }) }.toString(),
    )

    private fun writeResponse(socket: Socket, status: Int, contentType: String, body: ByteArray) {
        val label = when (status) { 200 -> "OK"; 400 -> "Bad Request"; 401 -> "Unauthorized"; 404 -> "Not Found"; 413 -> "Payload Too Large"; else -> "Internal Server Error" }
        val headers = "HTTP/1.1 $status $label\r\nContent-Type: $contentType\r\nContent-Length: ${body.size}\r\nConnection: close\r\nCache-Control: no-store\r\n\r\n"
        val out = socket.getOutputStream()
        out.write(headers.toByteArray(StandardCharsets.ISO_8859_1))
        out.write(body)
        out.flush()
    }

    @Synchronized
    override fun close() {
        running.set(false)
        runCatching { listener?.close() }
        listener = null
        acceptThread = null
        workers.shutdownNow()
    }
}
