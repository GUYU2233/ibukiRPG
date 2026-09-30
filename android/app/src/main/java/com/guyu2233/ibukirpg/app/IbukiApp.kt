package com.guyu2233.ibukirpg.app

import android.app.Application
import com.guyu2233.ibukirpg.app.crash.CrashReporter
import com.guyu2233.ibukirpg.app.data.AIConfig
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.SettingsStore
import com.guyu2233.ibukirpg.app.llm.LlamaLocalAI
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class IbukiApp : Application() {
    lateinit var engine: Engine
        private set
    lateinit var settings: SettingsStore
        private set
    lateinit var localAI: LlamaLocalAI
        private set
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    val aiStartupReady = CompletableDeferred<Unit>()

    override fun onCreate() {
        super.onCreate()
        CrashReporter.install(this, BuildConfig.VERSION_NAME)
        engine = Engine(filesDir.absolutePath)
        settings = SettingsStore(this)
        localAI = LlamaLocalAI(this)
        // 恢复 AI 设置（后台线程）。本地模型模式先校验 / 加载 GGUF 并启动仅监听回环地址的适配器；失败时退回离线模式。
        appScope.launch(Dispatchers.IO) {
            try {
                runCatching { settings.migrateFromMediaPipe() }
                val saved = settings.settings.first()
                val cfg = if (saved.aiKind == LlamaLocalAI.KIND) {
                    runCatching { localAI.configure(saved.localConfig(), load = false) }.getOrElse { error ->
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

    override fun onTrimMemory(level: Int) {
        super.onTrimMemory(level)
        // 内存告急 / 即将被回收：释放本地模型（下次生成时按原配置重新加载）
        localAI.onTrimMemory(level)
    }
}
