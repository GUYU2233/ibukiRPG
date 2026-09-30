package com.guyu2233.ibukirpg.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.guyu2233.ibukirpg.app.data.AppSettings
import com.guyu2233.ibukirpg.app.ui.AppNavHost
import com.guyu2233.ibukirpg.app.ui.theme.IbukiTheme

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        val app = application as IbukiApp
        setContent {
            val settings by app.settings.settings.collectAsStateWithLifecycle(initialValue = AppSettings())
            IbukiTheme(theme = settings.theme, dynamicColor = settings.dynamicColor, textScale = settings.textScale) {
                AppNavHost(app)
            }
        }
    }
}
