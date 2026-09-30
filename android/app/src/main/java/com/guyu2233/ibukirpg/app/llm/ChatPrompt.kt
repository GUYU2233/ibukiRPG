package com.guyu2233.ibukirpg.app.llm

/**
 * 模型没有内置聊天模板、或模板不被 llama_chat_apply_template 识别时的退回格式。
 * 按模板名的特征选择常见格式，都不像时使用 ChatML（Qwen / 许多开源模型的默认格式）。
 */
object ChatPrompt {
    fun fallback(messages: List<Pair<String, String>>, templateHint: String?): String {
        val hint = templateHint.orEmpty()
        return when {
            hint.contains("<start_of_turn>") -> gemma(messages)
            hint.contains("<|start_header_id|>") -> llama3(messages)
            else -> chatml(messages)
        }
    }

    private fun role(r: String) = when (r.lowercase()) {
        "system" -> "system"
        "assistant" -> "assistant"
        else -> "user"
    }

    fun chatml(messages: List<Pair<String, String>>): String = buildString {
        for ((r, c) in messages) append("<|im_start|>").append(role(r)).append('\n').append(c).append("<|im_end|>\n")
        append("<|im_start|>assistant\n")
    }

    private fun gemma(messages: List<Pair<String, String>>): String = buildString {
        // Gemma 没有 system 角色：并入第一条 user
        var system = ""
        for ((r, c) in messages) {
            when (role(r)) {
                "system" -> system += c + "\n\n"
                "assistant" -> append("<start_of_turn>model\n").append(c).append("<end_of_turn>\n")
                else -> { append("<start_of_turn>user\n").append(system).append(c).append("<end_of_turn>\n"); system = "" }
            }
        }
        append("<start_of_turn>model\n")
    }

    private fun llama3(messages: List<Pair<String, String>>): String = buildString {
        append("<|begin_of_text|>")
        for ((r, c) in messages) append("<|start_header_id|>").append(role(r)).append("<|end_header_id|>\n\n").append(c).append("<|eot_id|>")
        append("<|start_header_id|>assistant<|end_header_id|>\n\n")
    }
}
