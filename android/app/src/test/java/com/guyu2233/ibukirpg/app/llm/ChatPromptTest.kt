package com.guyu2233.ibukirpg.app.llm

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ChatPromptTest {
    private val msgs = listOf("system" to "你是旁白", "user" to "去锡兰", "assistant" to "好的", "user" to "继续")

    @Test fun defaultsToChatML() {
        val p = ChatPrompt.fallback(msgs, null)
        assertTrue(p.startsWith("<|im_start|>system\n你是旁白<|im_end|>\n"))
        assertTrue(p.endsWith("<|im_start|>assistant\n"))
        assertEquals(4, Regex("<\\|im_end\\|>").findAll(p).count())
    }

    @Test fun gemmaFoldsSystemIntoUser() {
        val p = ChatPrompt.fallback(msgs, "{{ '<start_of_turn>' + role }}")
        assertFalse(p.contains("system"))
        assertTrue(p.contains("<start_of_turn>user\n你是旁白\n\n去锡兰<end_of_turn>"))
        assertTrue(p.contains("<start_of_turn>model\n好的<end_of_turn>"))
        assertTrue(p.endsWith("<start_of_turn>model\n"))
    }

    @Test fun llama3Headers() {
        val p = ChatPrompt.fallback(msgs, "<|start_header_id|>")
        assertTrue(p.startsWith("<|begin_of_text|><|start_header_id|>system<|end_header_id|>"))
        assertTrue(p.endsWith("<|start_header_id|>assistant<|end_header_id|>\n\n"))
    }
}
