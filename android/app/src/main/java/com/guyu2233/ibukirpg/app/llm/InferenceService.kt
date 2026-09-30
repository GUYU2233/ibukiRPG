package com.guyu2233.ibukirpg.app.llm

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.guyu2233.ibukirpg.app.IbukiApp
import com.guyu2233.ibukirpg.app.MainActivity
import com.guyu2233.ibukirpg.app.R
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * 本地模型生成期间的前台服务：显示一条进行中的通知，让系统在玩家切到后台时不要杀掉进程。
 * 服务自己观察 [LlamaLocalAI.busy]：先 startForeground，再在生成结束后 stopSelf，
 * 避免“startForegroundService 之后来不及 startForeground”的崩溃。
 */
class InferenceService : Service() {
    companion object {
        private const val TAG = "ibuki-llama"
        private const val CHANNEL = "local_inference"
        private const val NOTIFICATION_ID = 42
        private const val ACTION_CANCEL = "com.guyu2233.ibukirpg.CANCEL_INFERENCE"
        fun start(context: Context) {
            try {
                // 每次都发送：服务在 onStartCommand 里立即 startForeground，重复启动是安全的
                ContextCompat.startForegroundService(context, Intent(context, InferenceService::class.java))
            } catch (e: Exception) {
                // Android 12+ 在后台时不允许启动前台服务：继续生成，只是没有保活
                Log.w(TAG, "cannot start foreground service", e)
            }
        }
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val app = application as IbukiApp
        if (intent?.action == ACTION_CANCEL) {
            app.localAI.cancel()
            return START_NOT_STICKY
        }
        startInForeground()
        scope.launch {
            // 等生成结束（busy 变为 false）后退出前台
            app.localAI.busy.first { !it }
            // 只有这是最新一次启动时才真正停止（期间又开始了新的生成则保持运行）
            stopSelf(startId)
        }
        return START_NOT_STICKY
    }

    private fun startInForeground() {
        val nm = getSystemService(NotificationManager::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && nm.getNotificationChannel(CHANNEL) == null) {
            nm.createNotificationChannel(NotificationChannel(CHANNEL, getString(R.string.inference_channel), NotificationManager.IMPORTANCE_LOW))
        }
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP), PendingIntent.FLAG_IMMUTABLE)
        val cancel = PendingIntent.getService(this, 1, Intent(this, InferenceService::class.java).setAction(ACTION_CANCEL), PendingIntent.FLAG_IMMUTABLE)
        val n: Notification = NotificationCompat.Builder(this, CHANNEL)
            .setSmallIcon(R.drawable.ic_stat_inference)
            .setContentTitle(getString(R.string.inference_title))
            .setContentText(getString(R.string.inference_text))
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setProgress(0, 0, true)
            .setContentIntent(open)
            .addAction(0, getString(R.string.inference_cancel), cancel)
            .setCategory(NotificationCompat.CATEGORY_PROGRESS)
            .build()
        val type = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE else 0
        ServiceCompat.startForeground(this, NOTIFICATION_ID, n, type)
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    override fun onTimeout(startId: Int, fgsType: Int) {
        // Android 15 对部分前台服务类型有时长限制：到时取消生成并退出
        (application as IbukiApp).localAI.cancel()
        stopSelf()
    }
}
