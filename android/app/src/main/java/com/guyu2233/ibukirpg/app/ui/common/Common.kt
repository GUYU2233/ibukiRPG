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

fun formatTime(ms: Long): String =
    if (ms <= 0) "" else SimpleDateFormat("M月d日 HH:mm", Locale.CHINA).format(Date(ms))
