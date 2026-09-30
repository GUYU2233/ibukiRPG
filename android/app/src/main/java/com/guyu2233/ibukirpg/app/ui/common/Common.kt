package com.guyu2233.ibukirpg.app.ui.common

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.DirectionsWalk
import androidx.compose.material.icons.outlined.AutoStories
import androidx.compose.material.icons.outlined.Backpack
import androidx.compose.material.icons.outlined.ChatBubbleOutline
import androidx.compose.material.icons.outlined.HourglassEmpty
import androidx.compose.material.icons.outlined.LocalBar
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.TouchApp
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.Flag
import androidx.compose.material.icons.outlined.Favorite
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Lightbulb
import androidx.compose.material.icons.outlined.Mood
import androidx.compose.material.icons.outlined.Payments
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material.icons.outlined.Report
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material.icons.outlined.SportsBar
import androidx.compose.material.icons.outlined.Star
import androidx.compose.material.icons.outlined.WbTwilight
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/** Go 端 SuggestionV1.icon → Material 图标。 */
fun iconFor(name: String): ImageVector = when (name) {
    "auto_stories" -> Icons.Outlined.AutoStories
    "visibility" -> Icons.Outlined.Visibility
    "chat" -> Icons.Outlined.ChatBubbleOutline
    "local_bar" -> Icons.Outlined.LocalBar
    "backpack" -> Icons.Outlined.Backpack
    "search" -> Icons.Outlined.Search
    "directions_walk" -> Icons.AutoMirrored.Outlined.DirectionsWalk
    "lock" -> Icons.Outlined.Lock
    "hourglass" -> Icons.Outlined.HourglassEmpty
    else -> Icons.Outlined.TouchApp
}

/** 故事包声明的图标提示（HUD / 故事包卡片，Material Symbols 名称）→ Material 图标；不认识的名字用 [fallback]。 */
fun symbolFor(name: String, fallback: ImageVector = Icons.Outlined.Info): ImageVector = when (name) {
    "flag" -> Icons.Outlined.Flag
    "place", "location_on" -> Icons.Outlined.Place
    "schedule", "time", "clock" -> Icons.Outlined.Schedule
    "payments", "gold", "coin", "paid" -> Icons.Outlined.Payments
    "report", "warning" -> Icons.Outlined.Report
    "mood" -> Icons.Outlined.Mood
    "favorite", "heart" -> Icons.Outlined.Favorite
    "star" -> Icons.Outlined.Star
    "shield" -> Icons.Outlined.Shield
    "lightbulb" -> Icons.Outlined.Lightbulb
    "sports_bar" -> Icons.Outlined.SportsBar
    "lighthouse", "wb_twilight" -> Icons.Outlined.WbTwilight
    "info" -> Icons.Outlined.Info
    else -> iconFor(name).takeIf { it != Icons.Outlined.TouchApp } ?: fallback
}

/** 解析 "#RRGGBB" 颜色；无效时返回 null。 */
fun parseAccent(hex: String): Color? = runCatching {
    if (hex.isBlank()) null else Color(android.graphics.Color.parseColor(hex.trim()))
}.getOrNull()

fun formatTime(ms: Long): String =
    if (ms <= 0) "" else SimpleDateFormat("M月d日 HH:mm", Locale.CHINA).format(Date(ms))
