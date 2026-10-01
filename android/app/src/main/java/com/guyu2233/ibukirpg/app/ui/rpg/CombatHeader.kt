package com.guyu2233.ibukirpg.app.ui.rpg

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.Favorite
import androidx.compose.material.icons.outlined.GpsFixed
import androidx.compose.material.icons.outlined.HelpOutline
import androidx.compose.material.icons.outlined.HeartBroken
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.CombatPartV1
import com.guyu2233.ibukirpg.app.data.CombatUnitV1
import com.guyu2233.ibukirpg.app.data.CombatV1

/**
 * 战斗时叙事流顶部的敌人卡（0.2.0 自由战斗）：生命、状态、可瞄准的部位（弱点只在识破后标出，未识破显示“未知”）、行动顺序。
 * 文字输入框直接描述行动即可；底部的快捷按钮仍然可用。
 */
@Composable
fun CombatHeader(c: CombatV1, modifier: Modifier = Modifier) {
    val enemy = c.enemies.firstOrNull { !it.down } ?: c.enemies.firstOrNull() ?: return
    val player = c.party.firstOrNull()
    val cs = MaterialTheme.colorScheme
    Surface(
        shape = RoundedCornerShape(18.dp),
        border = BorderStroke(1.dp, cs.outlineVariant),
        color = cs.surfaceContainerLowest,
        modifier = modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(rpgIcon(enemy.icon, "enemy"), contentDescription = null, tint = cs.error, modifier = Modifier.size(24.dp))
                Spacer(Modifier.width(10.dp))
                Text(enemy.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                Spacer(Modifier.width(8.dp))
                Row(Modifier.weight(1f).horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    enemy.statuses.forEach { st ->
                        Tag(st.name + if (st.turns > 0) " · ${st.turns}" else "", if (st.debuff) cs.errorContainer else cs.surfaceContainerHighest, if (st.debuff) cs.onErrorContainer else cs.onSurfaceVariant)
                    }
                }
                Text(tierLabel(enemy.tier), style = MaterialTheme.typography.labelMedium, color = cs.onSurfaceVariant)
            }
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.semantics(mergeDescendants = true) { contentDescription = "生命 ${enemy.hp}/${enemy.maxHp}" }) {
                Text("HP", style = MaterialTheme.typography.labelLarge, modifier = Modifier.width(36.dp))
                LinearProgressIndicator(
                    progress = { if (enemy.maxHp > 0) enemy.hp.toFloat() / enemy.maxHp else 0f },
                    color = cs.error, trackColor = cs.surfaceContainerHighest,
                    modifier = Modifier.weight(1f).height(8.dp),
                    drawStopIndicator = {},
                )
                Spacer(Modifier.width(10.dp))
                Text("${enemy.hp}/${enemy.maxHp}", style = MaterialTheme.typography.labelLarge, color = cs.onSurfaceVariant)
            }
            if (enemy.parts.isNotEmpty()) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("部位", style = MaterialTheme.typography.labelLarge, color = cs.onSurfaceVariant, modifier = Modifier.width(36.dp))
                    Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        enemy.parts.forEach { PartChip(it) }
                    }
                }
            }
            if (c.enemies.size > 1) {
                Text(
                    c.enemies.filter { it.id != enemy.id }.joinToString("  ·  ") { u -> "${u.name} ${if (u.down) "倒下" else "${u.hp}/${u.maxHp}"}" },
                    style = MaterialTheme.typography.labelMedium, color = cs.onSurfaceVariant,
                )
            }
            HorizontalDivider(color = cs.outlineVariant)
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("顺序", style = MaterialTheme.typography.labelLarge, color = cs.onSurfaceVariant, modifier = Modifier.width(36.dp))
                Row(Modifier.weight(1f).horizontalScroll(rememberScrollState()), verticalAlignment = Alignment.CenterVertically) {
                    c.order.take(6).forEachIndexed { i, name ->
                        if (i > 0) Icon(Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null, tint = cs.outline, modifier = Modifier.size(18.dp))
                        val you = name == "你"
                        val foe = c.enemies.any { it.name == name }
                        Tag(
                            name,
                            when { you -> cs.primary; foe -> cs.errorContainer; else -> cs.tertiaryContainer },
                            when { you -> cs.onPrimary; foe -> cs.onErrorContainer; else -> cs.onTertiaryContainer },
                            pill = true,
                        )
                    }
                }
                player?.let { p -> PlayerMini(p) }
            }
        }
    }
}

private fun tierLabel(t: String?) = when (t) {
    "elite" -> "精英"
    "boss" -> "首领"
    "minion" -> "杂兵"
    else -> ""
}

@Composable
private fun PlayerMini(p: CombatUnitV1) {
    val cs = MaterialTheme.colorScheme
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(if (p.hp * 4 < p.maxHp) Icons.Outlined.HeartBroken else Icons.Outlined.Favorite, contentDescription = null, tint = cs.error, modifier = Modifier.size(16.dp))
        Spacer(Modifier.width(2.dp))
        Text("${p.hp}/${p.maxHp}", style = MaterialTheme.typography.labelLarge)
        if (p.maxSp > 0) {
            Spacer(Modifier.width(6.dp))
            Icon(Icons.Outlined.Bolt, contentDescription = null, tint = cs.primary, modifier = Modifier.size(16.dp))
            Text("${p.sp}/${p.maxSp}", style = MaterialTheme.typography.labelLarge)
        }
    }
}

@Composable
private fun PartChip(p: CombatPartV1) {
    val cs = MaterialTheme.colorScheme
    val weak = p.known && p.weak
    Surface(
        shape = RoundedCornerShape(8.dp),
        border = BorderStroke(1.dp, if (weak) cs.primary else cs.outlineVariant),
        color = if (p.broken) cs.surfaceContainerHighest else Color.Transparent,
    ) {
        Row(Modifier.padding(horizontal = 8.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            if (weak) Icon(Icons.Outlined.GpsFixed, contentDescription = null, tint = cs.primary, modifier = Modifier.size(14.dp))
            if (!p.known) Icon(Icons.Outlined.HelpOutline, contentDescription = null, tint = cs.onSurfaceVariant, modifier = Modifier.size(14.dp))
            if (weak || !p.known) Spacer(Modifier.width(4.dp))
            Text(
                p.name + when { p.broken -> " · 已破坏"; weak -> " · 弱点"; else -> "" },
                style = MaterialTheme.typography.labelMedium,
                color = if (weak) cs.primary else cs.onSurface,
            )
            if (!p.known) {
                Spacer(Modifier.width(4.dp))
                Surface(color = cs.surfaceContainerHighest, shape = RoundedCornerShape(4.dp)) {
                    Text("未知", style = MaterialTheme.typography.labelSmall, color = cs.onSurfaceVariant, modifier = Modifier.padding(horizontal = 4.dp))
                }
            }
        }
    }
}

@Composable
private fun Tag(text: String, bg: Color, fg: Color, pill: Boolean = false) {
    Surface(color = bg, contentColor = fg, shape = if (pill) RoundedCornerShape(50) else RoundedCornerShape(8.dp)) {
        Text(text, style = MaterialTheme.typography.labelMedium, modifier = Modifier.padding(horizontal = 10.dp, vertical = 3.dp))
    }
}
