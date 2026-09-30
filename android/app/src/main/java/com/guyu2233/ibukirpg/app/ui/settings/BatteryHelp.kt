package com.guyu2233.ibukirpg.app.ui.settings

import android.content.Context
import android.content.Intent
import android.provider.Settings

/**
 * 打开系统的“电池优化”列表，让玩家自行决定是否把 ibukiRPG 设为“不优化”。
 * 只是跳转，不申请 REQUEST_IGNORE_BATTERY_OPTIMIZATIONS，也不会强制要求。
 */
object BatteryHelp {
    fun open(context: Context) {
        val intents = listOf(
            Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS),
            Intent(Settings.ACTION_SETTINGS),
        )
        for (i in intents) {
            if (runCatching { context.startActivity(i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }.isSuccess) return
        }
    }
}
