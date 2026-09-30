package com.guyu2233.ibukirpg.app

import android.app.Application
import com.guyu2233.ibukirpg.app.data.AIConfig
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.MediaPipeLocalAI
import com.guyu2233.ibukirpg.app.data.SettingsStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.CompletableDeferred

class IbukiApp : Application() {
    lateinit var engine: Engine
        private set
    lateinit var settings: SettingsStore
        private set
    lateinit var localAI: MediaPipeLocalAI
        private set
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    val aiStartupReady = CompletableDeferred<Unit>()

    override fun onCreate() {
        super.onCreate()
        engine = Engine(filesDir.absolutePath)
        settings = SettingsStore(this)
        localAI = MediaPipeLocalAI(this)
        // 恢复 AI 设置。MediaPipe 本地模式先加载模型并启动仅监听回环地址的适配器。
        appScope.launch {
            try {
                val saved = settings.settings.first()
                val cfg = if (saved.aiKind == "mediapipe") {
                    runCatching {
                        localAI.configure(saved.localModelPath, saved.localModelName, saved.localTemperature, saved.localContextTokens, saved.localTopK, saved.localTopP)
                    }.getOrElse { error ->
                        android.util.Log.w("ibukiRPG", "无法恢复本地模型，退回规则离线模式", error)
                        AIConfig(kind = "offline")
                    }
                } else settings.aiConfig()
                runCatching { engine.configureAI(cfg) }
            } finally {
                aiStartupReady.complete(Unit)
            }
        }
    }
}
