package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AltRoute
import androidx.compose.material.icons.outlined.AutoStories
import androidx.compose.material.icons.outlined.BookmarkAdded
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.HeartBroken
import androidx.compose.material.icons.outlined.MoreHoriz
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material.icons.outlined.Undo
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.SheetValue
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.DecisionV1

/**
 * 偏离 / 重要角色死亡 / 重大影响提示（第 7 节）。待决时不能下滑关闭：玩家必须选择接受或回到上一回合。
 * 仅通知模式（notify=true）可以直接关闭。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DecisionSheet(
    d: DecisionV1,
    busy: Boolean,
    onAccept: (notifyOnly: Boolean) -> Unit,
    onRollback: (notifyOnly: Boolean) -> Unit,
    onSensitivity: () -> Unit,
) {
    val state = rememberModalBottomSheetState(
        skipPartiallyExpanded = true,
        confirmValueChange = { d.notify || it != SheetValue.Hidden },
    )
    ModalBottomSheet(
        onDismissRequest = { if (d.notify) onAccept(false) },
        sheetState = state,
        properties = androidx.compose.material3.ModalBottomSheetProperties(shouldDismissOnBackPress = d.notify),
    ) {
        DecisionContent(d, busy, onAccept, onRollback, onSensitivity, Modifier.navigationBarsPadding())
    }
}

@Composable
fun DecisionContent(
    d: DecisionV1,
    busy: Boolean,
    onAccept: (notifyOnly: Boolean) -> Unit,
    onRollback: (notifyOnly: Boolean) -> Unit,
    onSensitivity: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val c = MaterialTheme.colorScheme
    var notifyOnly by rememberSaveable(d.id) { mutableStateOf(false) }
    val death = d.type == "major_death"
    Column(modifier.fillMaxWidth().padding(horizontal = 24.dp).padding(bottom = 24.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(
                when (d.type) {
                    "major_death" -> Icons.Outlined.HeartBroken
                    "lore_deviation" -> Icons.Outlined.AltRoute
                    else -> Icons.Outlined.AutoStories
                },
                contentDescription = null, tint = if (death) c.error else c.primary, modifier = Modifier.size(28.dp),
            )
            Spacer(Modifier.width(12.dp))
            val label = buildString {
                if (d.world) append("场外世界 · ")
                append(d.typeLabel)
                if (d.sensitivity.isNotBlank()) append(" · 灵敏度 ").append(d.sensitivity)
                if (d.notify) append(" · 仅通知")
            }
            Badge(label, if (death) c.errorContainer else c.secondaryContainer, if (death) c.onErrorContainer else c.onSecondaryContainer)
        }
        Text(d.title, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        if (d.summary.isNotBlank()) Text(d.summary, style = MaterialTheme.typography.bodyLarge, color = c.onSurfaceVariant)
        if (d.lines.isNotEmpty()) {
            Text("将会改变", style = MaterialTheme.typography.labelLarge)
            Surface(shape = RoundedCornerShape(14.dp), border = BorderStroke(1.dp, c.outlineVariant), color = c.surfaceContainerLowest) {
                Column(Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    d.lines.forEach { l -> ChangeLine(l) }
                    if (d.more > 0) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Outlined.MoreHoriz, contentDescription = null, modifier = Modifier.size(18.dp), tint = c.onSurfaceVariant)
                            Spacer(Modifier.width(10.dp))
                            Text("另有 ${d.more} 项次要变更", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
                            Text("在世界日志里查看", style = MaterialTheme.typography.labelLarge, color = c.primary)
                        }
                    }
                }
            }
        }
        d.checkpoint?.takeIf { it.isNotBlank() }?.let {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.BookmarkAdded, contentDescription = null, modifier = Modifier.size(18.dp), tint = c.onSurfaceVariant)
                Spacer(Modifier.width(8.dp))
                Text("已自动创建检查点「$it」（回合 ${d.rollbackTurn}）", style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant)
            }
        }
        Button(onClick = { onAccept(notifyOnly) }, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp)) {
            Icon(Icons.Outlined.Check, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text(if (d.notify) "知道了" else "接受，继续故事")
        }
        OutlinedButton(onClick = { onRollback(notifyOnly) }, enabled = !busy, modifier = Modifier.fillMaxWidth().heightIn(min = 52.dp)) {
            Icon(Icons.Outlined.Undo, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text(d.rollbackText ?: "回到上一回合")
        }
        Row(verticalAlignment = Alignment.CenterVertically) {
            if (!d.notify) {
                Checkbox(checked = notifyOnly, onCheckedChange = { notifyOnly = it })
                Text("以后这类情况只通知，不打断", style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
            } else {
                Spacer(Modifier.weight(1f))
            }
            TextButton(onClick = onSensitivity) {
                Icon(Icons.Outlined.Tune, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(6.dp))
                Text("灵敏度")
            }
        }
    }
}

/** 一行变更：“托克 死亡” / “北码头：描述 → 新值”。箭头后的新值加粗。 */
@Composable
private fun ChangeLine(l: String) {
    val c = MaterialTheme.colorScheme
    val (head, tail) = l.split(" → ", limit = 2).let { it[0] to it.getOrNull(1) }
    Row(verticalAlignment = Alignment.Top) {
        Text("•", color = c.onSurfaceVariant, modifier = Modifier.width(16.dp))
        Text(head, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f), maxLines = 2, overflow = TextOverflow.Ellipsis)
        if (tail != null) {
            Text(" → ", style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant)
            Text(tail, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1.2f), maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
    }
}

