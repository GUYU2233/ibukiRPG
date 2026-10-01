package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AutoFixHigh
import androidx.compose.material.icons.outlined.Block
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.FactCheck
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.AuditV1
import com.guyu2233.ibukirpg.app.data.ChangePreviewV1
import com.guyu2233.ibukirpg.app.data.WorldChangeV1

// ---------- 一致性审查卡片（叙事流 kind=audit） ----------

/** 审查结果：自动修复（可在日志页撤销）+ 机械状态建议（应用 / 忽略，第 19 节决定 4）。 */
@Composable
fun AuditCard(a: AuditV1, enabled: Boolean, onSuggestion: (id: String, action: String) -> Unit) {
    val c = MaterialTheme.colorScheme
    val fixed = a.fixed?.changes.orEmpty()
    Surface(
        color = c.surfaceContainerLowest, shape = RoundedCornerShape(14.dp), border = BorderStroke(1.dp, c.outlineVariant),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.FactCheck, contentDescription = null, tint = c.primary, modifier = Modifier.size(20.dp))
                Spacer(Modifier.width(10.dp))
                Text("一致性检查", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                Text(
                    buildString {
                        append("修复 ${fixed.size} 处")
                        if (a.suggestions.isNotEmpty()) append(" · ${a.suggestions.size} 条建议")
                    },
                    style = MaterialTheme.typography.labelMedium, color = c.onSurfaceVariant,
                )
            }
            a.findings.forEach { f ->
                val label = when (f.kind) {
                    "omission" -> "遗漏"
                    "hallucination" -> "叙事与设定不符"
                    "contradiction" -> "前后矛盾"
                    else -> f.kind
                }
                Text("$label：${f.text}", style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant)
            }
            fixed.forEach { ch ->
                Row(verticalAlignment = Alignment.Top) {
                    Icon(Icons.Outlined.AutoFixHigh, contentDescription = null, tint = c.primary, modifier = Modifier.size(16.dp).padding(top = 2.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("已修复：${ch.summary}", style = MaterialTheme.typography.bodySmall)
                }
            }
            a.suggestions.forEach { sg ->
                Surface(color = c.surfaceContainerHigh, shape = RoundedCornerShape(10.dp), modifier = Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(horizontal = 12.dp, vertical = 8.dp)) {
                        Text("建议：${sg.summary}", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
                        if (sg.reason.isNotBlank()) Text(sg.reason, style = MaterialTheme.typography.labelSmall, color = c.onSurfaceVariant)
                        when (sg.status) {
                            "applied" -> Text("已应用", style = MaterialTheme.typography.labelMedium, color = c.primary, modifier = Modifier.padding(top = 4.dp))
                            "ignored" -> Text("已忽略", style = MaterialTheme.typography.labelMedium, color = c.outline, modifier = Modifier.padding(top = 4.dp))
                            else -> Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(top = 4.dp)) {
                                Button(onClick = { onSuggestion(sg.id, "apply") }, enabled = enabled) { Text("应用") }
                                OutlinedButton(onClick = { onSuggestion(sg.id, "ignore") }, enabled = enabled) { Text("忽略") }
                            }
                        }
                    }
                }
            }
        }
    }
}

// ---------- 玩家要求的修改：输入要求 → 预览差异 → 确认（第 14.3 节） ----------

/** 修改流程的状态：target 为空表示新建卡片。 */
data class EditState(
    val target: String = "",
    val targetName: String = "",
    val busy: Boolean = false,
    val preview: ChangePreviewV1? = null,
    val error: String? = null,
)

/** “让 AI 修改”的输入对话框。 */
@Composable
fun EditRequestDialog(e: EditState, onSubmit: (String) -> Unit, onDismiss: () -> Unit) {
    var text by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = { if (!e.busy) onDismiss() },
        title = { Text(if (e.target.isBlank()) "新建卡片" else "让 AI 修改「${e.targetName}」") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    if (e.target.isBlank()) "描述你想加入世界的人物、物品或设定，AI 会生成卡片，你确认后才会生效。"
                    else "说说你想怎么改。AI 会给出修改，你确认后才会生效，之后也可以撤销。",
                    style = MaterialTheme.typography.bodyMedium,
                )
                OutlinedTextField(
                    value = text, onValueChange = { if (it.length <= 400) text = it },
                    placeholder = { Text(if (e.target.isBlank()) "例如：码头上一个卖旧零件的独眼老头" else "例如：把扳手改成带电击的") },
                    minLines = 2, enabled = !e.busy, modifier = Modifier.fillMaxWidth(),
                )
                e.error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
            }
        },
        confirmButton = {
            TextButton(onClick = { onSubmit(text.trim()) }, enabled = !e.busy && text.isNotBlank()) {
                if (e.busy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp) else Text("生成修改")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !e.busy) { Text("取消") } },
    )
}

/** 预览底部面板。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChangePreviewSheet(e: EditState, onConfirm: () -> Unit, onCancel: () -> Unit) {
    val pv = e.preview ?: return
    val sheet = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(onDismissRequest = { if (!e.busy) onCancel() }, sheetState = sheet) {
        ChangePreviewContent(pv, e.busy, e.error, onConfirm, onCancel)
    }
}

/** 字段级 before / after 差异（新值绿色、旧值划线）；玩家不知道的字段只计数，不剧透。 */
@Composable
fun ChangePreviewContent(pv: ChangePreviewV1, busy: Boolean, error: String?, onConfirm: () -> Unit, onCancel: () -> Unit) {
    val c = MaterialTheme.colorScheme
    Column(
        Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 24.dp).navigationBarsPadding(),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text("确认修改", style = MaterialTheme.typography.headlineSmall)
        if (pv.note.isNotBlank()) Text(pv.note, style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant)
        pv.changes.forEach { ch -> DiffRow(ch) }
        if (pv.hiddenCount > 0) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Outlined.VisibilityOff, contentDescription = null, tint = c.outline, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("另有 ${pv.hiddenCount} 个你还不知道的字段会被修改", style = MaterialTheme.typography.bodySmall, color = c.outline)
            }
        }
        pv.rejected.forEach { r ->
            Row(verticalAlignment = Alignment.Top) {
                Icon(Icons.Outlined.Block, contentDescription = null, tint = c.error, modifier = Modifier.size(16.dp).padding(top = 2.dp))
                Spacer(Modifier.width(6.dp))
                Text("未通过校验：${r.reason}", style = MaterialTheme.typography.bodySmall, color = c.error)
            }
        }
        if (pv.impact >= 30) Text("影响分 ${pv.impact}：这项修改会明显改变世界。", style = MaterialTheme.typography.labelMedium, color = c.tertiary)
        error?.let { Text(it, color = c.error, style = MaterialTheme.typography.bodySmall) }
        Spacer(Modifier.height(4.dp))
        Button(onClick = onConfirm, enabled = !busy && pv.previewToken.isNotBlank(), modifier = Modifier.fillMaxWidth().height(52.dp)) {
            Icon(Icons.Outlined.Check, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text("确认修改")
        }
        OutlinedButton(onClick = onCancel, enabled = !busy, modifier = Modifier.fillMaxWidth().height(52.dp)) { Text("取消（不留任何记录）") }
        Spacer(Modifier.height(16.dp))
    }
}

@Composable
private fun DiffRow(ch: WorldChangeV1) {
    val c = MaterialTheme.colorScheme
    Surface(color = c.surfaceContainerLowest, shape = RoundedCornerShape(12.dp), border = BorderStroke(1.dp, c.outlineVariant), modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                listOfNotNull(ch.targetName.ifBlank { null }, ch.path?.substringAfterLast('.')?.ifBlank { null }?.let { fieldLabel(it) }).joinToString(" · ").ifBlank { ch.summary },
                style = MaterialTheme.typography.labelLarge, color = c.onSurfaceVariant,
            )
            ch.before?.takeIf { it.isNotBlank() }?.let {
                Text(
                    "− $it", style = MaterialTheme.typography.bodyMedium.copy(textDecoration = androidx.compose.ui.text.style.TextDecoration.LineThrough),
                    color = c.error,
                )
            }
            Text("+ ${ch.after?.takeIf { it.isNotBlank() } ?: ch.summary}", style = MaterialTheme.typography.bodyMedium, color = androidx.compose.ui.graphics.Color(0xFF2E7D32))
        }
    }
}

private val fieldLabels = mapOf(
    "description" to "描述", "appearance" to "外貌", "name" to "名字", "title" to "名称", "personality" to "性格",
    "background" to "背景", "identity" to "身份", "goal" to "目标", "tags" to "标签", "location" to "所在地",
    "faction" to "所属势力", "effect" to "效果", "attack" to "攻击", "defense" to "防御", "price" to "价格",
)

private fun fieldLabel(key: String): String = fieldLabels[key] ?: key
