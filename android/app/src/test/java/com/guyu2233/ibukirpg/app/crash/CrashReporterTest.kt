package com.guyu2233.ibukirpg.app.crash

import android.app.Application
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class CrashReporterTest {
    @Test
    fun writesCrashLogAndChainsToPreviousHandler() {
        val app = RuntimeEnvironment.getApplication()
        CrashReporter.clear(app)
        assertNull(CrashReporter.pending(app))
        val original = Thread.getDefaultUncaughtExceptionHandler()
        var chained = false
        Thread.setDefaultUncaughtExceptionHandler { _, _ -> chained = true }
        try {
            CrashReporter.install(app, "9.9.9-test")
            val t = Thread { throw IllegalStateException("boom in test") }
            t.start()
            t.join()
            val log = CrashReporter.pending(app)!!
            assertTrue(log.contains("9.9.9-test"))
            assertTrue(log.contains("IllegalStateException: boom in test"))
            assertTrue(log.contains("未捕获异常"))
            assertTrue(chained)
            CrashReporter.clear(app)
            assertNull(CrashReporter.pending(app))
        } finally {
            Thread.setDefaultUncaughtExceptionHandler(original)
        }
    }
}
