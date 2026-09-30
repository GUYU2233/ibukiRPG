package com.guyu2233.ibukirpg.app.crash

import android.app.ActivityManager
import android.app.ApplicationExitInfo
import android.content.Context
import android.os.Build
import java.io.File
import java.io.PrintWriter
import java.io.StringWriter
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * 本地崩溃记录（不联网上传）：
 * - Kotlin / Java 未捕获异常 → 写入 files/crash/last_crash.txt，再交给系统默认处理；
 * - Android 11+ 下次启动时读取 ApplicationExitInfo，补记原生崩溃（例如 llama.cpp）、ANR、被系统因内存回收。
 * 首页在下次启动时显示这份记录，玩家可以复制后自行反馈。
 */
object CrashReporter {
    private const val DIR = "crash"
    private const val FILE = "last_crash.txt"
    private const val PREFS = "crash_reporter"
    private const val KEY_LAST_EXIT = "last_exit_ts"

    fun install(context: Context, version: String) {
        val app = context.applicationContext
        val previous = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, error ->
            runCatching { write(app, format(version, thread.name, error)) }
            previous?.uncaughtException(thread, error)
        }
        runCatching { recordExitReasons(app, version) }
    }

    /** 未查看的崩溃记录；没有时返回 null。 */
    fun pending(context: Context): String? = file(context).takeIf { it.isFile }?.readText()?.takeIf { it.isNotBlank() }

    fun clear(context: Context) {
        file(context).delete()
    }

    internal fun format(version: String, thread: String, error: Throwable, now: Date = Date()): String {
        val sw = StringWriter()
        error.printStackTrace(PrintWriter(sw))
        return buildString {
            append("时间：").append(stamp(now)).append('\n')
            append("版本：").append(version).append('\n')
            append("设备：").append(Build.MANUFACTURER).append(' ').append(Build.MODEL).append(" / Android ").append(Build.VERSION.RELEASE).append('\n')
            append("线程：").append(thread).append('\n')
            append("类型：未捕获异常\n\n")
            append(sw.toString().take(16_000))
        }
    }

    private fun recordExitReasons(context: Context, version: String) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) return
        val am = context.getSystemService(ActivityManager::class.java) ?: return
        val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val last = prefs.getLong(KEY_LAST_EXIT, 0L)
        val infos = am.getHistoricalProcessExitReasons(context.packageName, 0, 5)
        val newest = infos.maxOfOrNull { it.timestamp } ?: return
        prefs.edit().putLong(KEY_LAST_EXIT, newest).apply()
        if (last == 0L) return // 第一次运行：只记录基准时间
        val info = infos.filter { it.timestamp > last }.maxByOrNull { it.timestamp } ?: return
        val kind = when (info.reason) {
            ApplicationExitInfo.REASON_CRASH_NATIVE -> "原生崩溃（通常来自本地模型推理库）"
            ApplicationExitInfo.REASON_ANR -> "应用无响应（ANR）"
            ApplicationExitInfo.REASON_LOW_MEMORY -> "系统内存不足，进程被回收"
            ApplicationExitInfo.REASON_EXCESSIVE_RESOURCE_USAGE -> "资源占用过高，被系统终止"
            else -> return // 正常退出 / 用户划掉 / Java 崩溃（已由上面的处理器记录）
        }
        if (pending(context) != null && info.reason != ApplicationExitInfo.REASON_CRASH_NATIVE) return
        val text = buildString {
            append("时间：").append(stamp(Date(info.timestamp))).append('\n')
            append("版本：").append(version).append('\n')
            append("设备：").append(Build.MANUFACTURER).append(' ').append(Build.MODEL).append(" / Android ").append(Build.VERSION.RELEASE).append('\n')
            append("类型：").append(kind).append('\n')
            info.description?.let { append("说明：").append(it).append('\n') }
            append("进程内存（PSS）：").append(info.pss / 1024).append(" MB\n")
            if (info.reason == ApplicationExitInfo.REASON_LOW_MEMORY || info.reason == ApplicationExitInfo.REASON_CRASH_NATIVE) {
                append("\n如果当时启用了本地模型，请换用更小的量化版本或降低上下文长度。\n")
            }
        }
        write(context, text)
    }

    private fun file(context: Context) = File(File(context.filesDir, DIR), FILE)

    private fun write(context: Context, text: String) {
        val f = file(context)
        f.parentFile?.mkdirs()
        f.writeText(text)
    }

    private fun stamp(d: Date) = SimpleDateFormat("yyyy-MM-dd HH:mm:ss", Locale.US).format(d)
}
