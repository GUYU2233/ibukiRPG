package com.guyu2233.ibukirpg.app

import android.app.Application
import com.guyu2233.ibukirpg.app.data.Engine
import com.guyu2233.ibukirpg.app.data.SettingsStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

class IbukiApp : Application() {
    lateinit var engine: Engine
        private set
    lateinit var settings: SettingsStore
        private set
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Default)

    override fun onCreate() {
        super.onCreate()
        engine = Engine(filesDir.absolutePath)
        settings = SettingsStore(this)
        // 启动时把保存的 AI 设置交给引擎（失败时引擎保持离线模式）。
        appScope.launch { runCatching { engine.configureAI(settings.aiConfig()) } }
    }
}
