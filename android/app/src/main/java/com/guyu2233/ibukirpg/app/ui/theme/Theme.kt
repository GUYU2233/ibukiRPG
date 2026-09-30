package com.guyu2233.ibukirpg.app.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.sp
import com.guyu2233.ibukirpg.app.data.ThemeMode

// 回退配色：以“琥珀 / 麦酒”色 #8C4A1C 为种子，按 Material 3 色调规则生成（Material Theme Builder）。
private val LightScheme = lightColorScheme(
    primary = Color(0xFF8C4A1C),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFFFDBC8),
    onPrimaryContainer = Color(0xFF6E3406),
    secondary = Color(0xFF765848),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFFFDBC8),
    onSecondaryContainer = Color(0xFF5C4132),
    tertiary = Color(0xFF636032),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFE9E5AB),
    onTertiaryContainer = Color(0xFF4B481D),
    error = Color(0xFFBA1A1A),
    onError = Color(0xFFFFFFFF),
    errorContainer = Color(0xFFFFDAD6),
    onErrorContainer = Color(0xFF93000A),
    background = Color(0xFFFFF8F5),
    onBackground = Color(0xFF221A15),
    surface = Color(0xFFFFF8F5),
    onSurface = Color(0xFF221A15),
    surfaceVariant = Color(0xFFF4DED4),
    onSurfaceVariant = Color(0xFF52443C),
    outline = Color(0xFF85746B),
    outlineVariant = Color(0xFFD7C3B8),
    inverseSurface = Color(0xFF382E29),
    inverseOnSurface = Color(0xFFFFEDE5),
    inversePrimary = Color(0xFFFFB68B),
    surfaceDim = Color(0xFFE8D7CF),
    surfaceBright = Color(0xFFFFF8F5),
    surfaceContainerLowest = Color(0xFFFFFFFF),
    surfaceContainerLow = Color(0xFFFFF1EB),
    surfaceContainer = Color(0xFFFCEAE3),
    surfaceContainerHigh = Color(0xFFF6E5DD),
    surfaceContainerHighest = Color(0xFFF0DFD7),
)

private val DarkScheme = darkColorScheme(
    primary = Color(0xFFFFB68B),
    onPrimary = Color(0xFF522300),
    primaryContainer = Color(0xFF6E3406),
    onPrimaryContainer = Color(0xFFFFDBC8),
    secondary = Color(0xFFE6BEAB),
    onSecondary = Color(0xFF432B1D),
    secondaryContainer = Color(0xFF5C4132),
    onSecondaryContainer = Color(0xFFFFDBC8),
    tertiary = Color(0xFFCDC991),
    onTertiary = Color(0xFF343208),
    tertiaryContainer = Color(0xFF4B481D),
    onTertiaryContainer = Color(0xFFE9E5AB),
    error = Color(0xFFFFB4AB),
    onError = Color(0xFF690005),
    errorContainer = Color(0xFF93000A),
    onErrorContainer = Color(0xFFFFDAD6),
    background = Color(0xFF19120D),
    onBackground = Color(0xFFF0DFD7),
    surface = Color(0xFF19120D),
    onSurface = Color(0xFFF0DFD7),
    surfaceVariant = Color(0xFF52443C),
    onSurfaceVariant = Color(0xFFD7C3B8),
    outline = Color(0xFF9F8D84),
    outlineVariant = Color(0xFF52443C),
    inverseSurface = Color(0xFFF0DFD7),
    inverseOnSurface = Color(0xFF382E29),
    inversePrimary = Color(0xFF8C4A1C),
    surfaceDim = Color(0xFF19120D),
    surfaceBright = Color(0xFF423732),
    surfaceContainerLowest = Color(0xFF140D09),
    surfaceContainerLow = Color(0xFF221A15),
    surfaceContainer = Color(0xFF261E19),
    surfaceContainerHigh = Color(0xFF312823),
    surfaceContainerHighest = Color(0xFF3D332D),
)

private val base = Typography()

// 中文阅读：略大的行高。
private val AppTypography = base.copy(
    bodyLarge = base.bodyLarge.copy(lineHeight = 28.sp, letterSpacing = 0.2.sp),
    bodyMedium = base.bodyMedium.copy(lineHeight = 22.sp),
    titleLarge = base.titleLarge.copy(fontWeight = FontWeight.SemiBold),
    displaySmall = base.displaySmall.copy(fontWeight = FontWeight.Bold),
)

/** 叙事正文样式。 */
val NarrativeStyle: TextStyle
    @Composable get() = MaterialTheme.typography.bodyLarge

@Composable
fun IbukiTheme(
    theme: ThemeMode = ThemeMode.SYSTEM,
    dynamicColor: Boolean = true,
    textScale: Float = 1f,
    content: @Composable () -> Unit,
) {
    val dark = when (theme) {
        ThemeMode.SYSTEM -> isSystemInDarkTheme()
        ThemeMode.LIGHT -> false
        ThemeMode.DARK -> true
    }
    val ctx = LocalContext.current
    val scheme: ColorScheme = when {
        dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (dark) dynamicDarkColorScheme(ctx) else dynamicLightColorScheme(ctx)
        dark -> DarkScheme
        else -> LightScheme
    }
    val density = LocalDensity.current
    CompositionLocalProvider(LocalDensity provides Density(density.density, density.fontScale * textScale)) {
        MaterialTheme(colorScheme = scheme, typography = AppTypography, content = content)
    }
}
