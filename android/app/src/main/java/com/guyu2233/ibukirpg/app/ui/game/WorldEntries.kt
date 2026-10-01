package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Campaign
import androidx.compose.material.icons.outlined.EditNote
import androidx.compose.material.icons.outlined.Gavel
import androidx.compose.material.icons.outlined.Hub
import androidx.compose.material.icons.outlined.LockOpen
import androidx.compose.material.icons.outlined.SouthEast
import androidx.compose.material.icons.outlined.Token
import androidx.compose.material.icons.outlined.Undo
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.WarningAmber
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.AdjudicationV1
import com.guyu2233.ibukirpg.app.data.KnowledgeChipV1
import com.guyu2233.ibukirpg.app.data.UsageV1
import com.guyu2233.ibukirpg.app.data.WorldChangeV1
import com.guyu2233.ibukirpg.app.data.WorldLogV1

// 0.2.0 叙事流里的新条目：知识解锁 chip、世界变更 chip、行动裁定卡、回合用量行。

/** 知识解锁（tertiaryContainer）+ 世界变更（surfaceContainerHigh）chip。 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun WorldEntry(w: WorldLogV1, onChange: (WorldChangeV1) -> Unit = {}) {
    FlowRow(
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
        modifier = Modifier.fillMaxWidth(),
    ) {
        mergeReveals(w.reveals).forEach { (icon, text) -> KnowledgeChip(icon, text) }
        w.changes.filter { !it.hidden || it.summary.isNotBlank() }.forEach { c ->
            WorldChangeChip(c, onClick = { onChange(c) })
        }
        if (w.failed != null && w.changes.isEmpty() && w.reveals.isEmpty()) {
            WorldChangeChip(
                WorldChangeV1(summary = if (w.failed == "refused") "模型拒绝了这一段世界更新，已只保留叙事" else "这回合的世界更新没能解析，已只保留叙事"),
                warning = true,
            )
        }
    }
}

/** 同一实体的多个字段合并成一个 chip；“新发现”只在没有其它字段时单独显示。 */
private fun mergeReveals(rs: List<KnowledgeChipV1>): List<Pair<ImageVector, String>> {
    val out = mutableListOf<Pair<ImageVector, String>>()
    rs.groupBy { it.entity }.forEach { (_, group) ->
        val first = group.first()
        when (first.kind) {
            "relation" -> out += Icons.Outlined.Hub to "新关系：${first.name}"
            "rumor" -> out += Icons.Outlined.Campaign to "传闻：${first.name}"
            else -> {
                val fields = group.filter { it.field != "existence" && it.field != "name" }.map { it.fieldLabel }
                out += if (fields.isEmpty()) Icons.Outlined.Visibility to "新发现：${first.name}"
                else Icons.Outlined.LockOpen to "${first.name} · ${fields.joinToString("、")}"
            }
        }
    }
    return out
}

@Composable
fun KnowledgeChip(icon: ImageVector, text: String) {
    Surface(
        color = MaterialTheme.colorScheme.tertiaryContainer,
        contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        shape = RoundedCornerShape(8.dp),
        modifier = Modifier.semantics(mergeDescendants = true) { contentDescription = "解锁：$text" },
    ) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(16.dp))
            Spacer(Modifier.width(6.dp))
            Text(text, style = MaterialTheme.typography.labelLarge)
        }
    }
}

@Composable
private fun WorldChangeChip(c: WorldChangeV1, warning: Boolean = false, onClick: (() -> Unit)? = null) {
    val reverted = c.revertedBy != null
    Surface(
        color = if (warning) MaterialTheme.colorScheme.errorContainer else MaterialTheme.colorScheme.surfaceContainerHighest,
        contentColor = if (warning) MaterialTheme.colorScheme.onErrorContainer else MaterialTheme.colorScheme.onSurfaceVariant,
        shape = RoundedCornerShape(8.dp),
        onClick = { onClick?.invoke() },
        enabled = onClick != null,
    ) {
        Row(Modifier.padding(horizontal = 10.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
            Icon(
                when {
                    warning -> Icons.Outlined.WarningAmber
                    c.reverts != null || reverted -> Icons.Outlined.Undo
                    else -> Icons.Outlined.EditNote
                },
                contentDescription = null, modifier = Modifier.size(16.dp),
            )
            Spacer(Modifier.width(6.dp))
            Text(
                if (warning) c.summary else "世界变更 · ${c.summary}" + if (reverted) "（已撤销）" else "",
                style = MaterialTheme.typography.labelLarge,
                maxLines = 2,
            )
        }
    }
}

/** 行动裁定卡：解析 / 修正 / 掷骰 / 结果，右上角是合理性标签；被降级时上方提示降级原因。 */
@Composable
fun AdjudicationCard(a: AdjudicationV1) {
    val c = MaterialTheme.colorScheme
    val ok = a.degree == "success" || a.degree == "crit" || a.degree == "partial"
    Column(verticalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
        a.downgrade?.takeIf { it.isNotBlank() }?.let { d ->
            Surface(color = c.tertiaryContainer.copy(alpha = 0.5f), contentColor = c.onTertiaryContainer, shape = RoundedCornerShape(10.dp), border = BorderStroke(1.dp, c.tertiaryContainer)) {
                Row(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp), verticalAlignment = Alignment.Top) {
                    Icon(Icons.Outlined.SouthEast, contentDescription = null, modifier = Modifier.size(16.dp).padding(top = 2.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("降级：$d", style = MaterialTheme.typography.bodySmall)
                }
            }
        }
        Surface(
            color = c.surfaceContainerLowest,
            shape = RoundedCornerShape(14.dp),
            border = BorderStroke(1.dp, c.outlineVariant),
            modifier = Modifier.fillMaxWidth().semantics(mergeDescendants = true) {},
        ) {
            Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Outlined.Gavel, contentDescription = null, tint = c.primary, modifier = Modifier.size(20.dp))
                    Spacer(Modifier.width(10.dp))
                    Text("行动裁定", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                    val (bg, fg) = when (a.plausibility) {
                        "absurd" -> c.errorContainer to c.onErrorContainer
                        "stretch" -> c.tertiaryContainer to c.onTertiaryContainer
                        else -> c.secondaryContainer to c.onSecondaryContainer
                    }
                    if (a.plausibilityLabel.isNotBlank()) Badge(a.plausibilityLabel, bg, fg)
                }
                AdjRow("解析", a.parse)
                if (a.mods.isNotEmpty()) {
                    val sign = if (a.modTotal >= 0) "+" else "−"
                    AdjRow("修正", a.mods.joinToString(" · ") + " → $sign${kotlin.math.abs(a.modTotal)}")
                }
                if (a.dice != null && a.rolls.isNotEmpty()) {
                    val sum = a.rolls.sum()
                    val sign = if (a.modTotal >= 0) "+" else "−"
                    val text = buildAnnotatedString {
                        append("${a.dice} = ${a.rolls.joinToString(" + ")} = $sum，$sign${kotlin.math.abs(a.modTotal)} = ")
                        withStyle(SpanStyle(fontWeight = FontWeight.Bold)) { append("${a.total}") }
                        append(" / 难度 ${a.dc} → ")
                        withStyle(SpanStyle(fontWeight = FontWeight.Bold, color = if (ok) c.primary else c.error)) {
                            append(a.degreeLabel ?: "")
                            if (a.margin != 0) append("（${if (a.margin > 0) "+" else "−"}${kotlin.math.abs(a.margin)}）")
                        }
                    }
                    Row {
                        Text("掷骰", style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant, modifier = Modifier.width(40.dp))
                        Text(text, style = MaterialTheme.typography.bodySmall)
                    }
                }
                a.result?.takeIf { it.isNotBlank() }?.let { AdjRow("结果", it) }
            }
        }
    }
}

@Composable
private fun AdjRow(label: String, value: String) {
    Row {
        Text(label, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.width(40.dp))
        Text(value, style = MaterialTheme.typography.bodySmall)
    }
}

@Composable
fun Badge(text: String, bg: Color, fg: Color) {
    Surface(color = bg, contentColor = fg, shape = RoundedCornerShape(50)) {
        Text(text, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 10.dp, vertical = 3.dp))
    }
}

/** 本回合用量：“回合 N · ↑2.1k ↓420 · 1 次调用”；长按“回到这里”。 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun UsageLine(u: UsageV1, onRollback: ((Int) -> Unit)? = null) {
    val c = MaterialTheme.colorScheme
    val ok = u.calls - u.failed
    val text = buildString {
        append("回合 ${u.turn} · ↑${compactTokens(u.promptTokens)} ↓${compactTokens(u.completionTokens)} · $ok 次调用")
        if (u.failed > 0) append(" · ${u.failed} 次重试")
    }
    Row(
        verticalAlignment = Alignment.CenterVertically,
        modifier = Modifier
            .fillMaxWidth()
            .combinedClickable(onClick = {}, onLongClick = { onRollback?.invoke(u.turn) }, onLongClickLabel = "回到这里")
            .padding(vertical = 2.dp),
    ) {
        Icon(Icons.Outlined.Token, contentDescription = null, tint = c.outline, modifier = Modifier.size(14.dp))
        Spacer(Modifier.width(6.dp))
        Text(text, style = MaterialTheme.typography.labelMedium, color = c.outline, modifier = Modifier.weight(1f), maxLines = 1)
    }
}

fun compactTokens(n: Int): String = when {
    n >= 10_000 -> "%.0fk".format(n / 1000.0)
    n >= 1000 -> "%.1fk".format(n / 1000.0)
    else -> n.toString()
}
