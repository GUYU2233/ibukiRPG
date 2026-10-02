package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Undo
import androidx.compose.material.icons.outlined.Extension
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.EntryV1
import com.guyu2233.ibukirpg.app.data.ExternalNoticeV1
import com.guyu2233.ibukirpg.app.data.RevertPlanV1

// 0.2.0-rc1：场外世界推进卡、“外部工具修改了 N 项设定”横幅、级联撤销确认。

/** 叙事流里的场外世界推进（kind=sim）：传闻 / 消息 + 世界变更 chip。 */
@Composable
fun SimEntry(e: EntryV1) {
    Surface(
        color = MaterialTheme.colorScheme.secondaryContainer.copy(alpha = 0.55f),
        shape = RoundedCornerShape(12.dp),
        modifier = Modifier.fillMaxWidth().semantics(mergeDescendants = true) { contentDescription = "场外世界：${e.text}" },
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.Public, contentDescription = null, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("场外", style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.Bold)
                Spacer(Modifier.width(8.dp))
                Text(
                    if (e.source == "world_sim:ai") "世界模拟 · AI" else "世界模拟 · 规则",
                    style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (e.text.isNotBlank()) Text(e.text, style = MaterialTheme.typography.bodyMedium)
            e.world?.let { WorldEntry(it) }
        }
    }
}

/** 打开游戏 / 载入存档时：外部工具（MCP）修改了设定。 */
@Composable
fun ExternalChangesBanner(n: ExternalNoticeV1, onOpenLog: () -> Unit, onDismiss: () -> Unit, modifier: Modifier = Modifier) {
    Surface(
        color = MaterialTheme.colorScheme.tertiaryContainer,
        contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        shape = RoundedCornerShape(12.dp),
        modifier = modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 6.dp),
    ) {
        Row(Modifier.padding(start = 12.dp, end = 4.dp, top = 4.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.Extension, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text(n.text.ifBlank { "外部工具修改了 ${n.count} 项设定" }, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.weight(1f))
            TextButton(onClick = onOpenLog) { Text("查看日志") }
            TextButton(onClick = onDismiss) { Text("知道了") }
        }
    }
}

/** 级联撤销确认的内容（对话框与截图测试共用）。 */
@Composable
fun RevertPlanContent(p: RevertPlanV1) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("要撤销：${p.change.summary}", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Bold)
        if (p.dependents.isNotEmpty()) {
            Text("之后有 ${p.dependents.size} 项变更依赖它（会按下面的顺序一起撤销）：", style = MaterialTheme.typography.bodySmall)
            p.dependents.take(8).forEach { d ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.Undo, contentDescription = null, modifier = Modifier.size(14.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("回合 ${d.turn} · ${d.summary}", style = MaterialTheme.typography.bodySmall)
                }
            }
            if (p.dependents.size > 8) Text("……还有 ${p.dependents.size - 8} 项", style = MaterialTheme.typography.bodySmall)
        }
        HorizontalDivider()
        Text(
            p.singleNote ?: if (p.canSingle) "只撤销这一条：依赖它的后续变更保留。" else "这条变更不能单独撤销。",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** 撤销一条有依赖的变更：连同依赖一起撤销 / 只撤销这一条。 */
@Composable
fun RevertPlanDialog(p: RevertPlanV1, onRevert: (mode: String) -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        icon = { Icon(Icons.Outlined.Undo, contentDescription = null) },
        title = { Text("撤销世界变更") },
        text = { RevertPlanContent(p) },
        confirmButton = {
            Button(onClick = { onRevert("chain") }) { Text(if (p.dependents.isEmpty()) "撤销" else "连同撤销 ${p.dependents.size} 项") }
        },
        dismissButton = {
            Row {
                if (p.dependents.isNotEmpty()) OutlinedButton(onClick = { onRevert("single") }, enabled = p.canSingle) { Text("只撤销这一条") }
                TextButton(onClick = onDismiss) { Text("取消") }
            }
        },
    )
}
