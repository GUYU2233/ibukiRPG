package com.guyu2233.ibukirpg.app.ui.rpg

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
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
import androidx.compose.material.icons.outlined.PrecisionManufacturing
import androidx.compose.material3.AssistChip
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.CombatActionV1
import com.guyu2233.ibukirpg.app.data.CombatUnitV1
import com.guyu2233.ibukirpg.app.data.CombatV1
import com.guyu2233.ibukirpg.app.data.QuickActionV1

private enum class Menu { Main, Skills, Items }

/** 战斗面板：双方单位（生命 / 技力 / 过热 / 状态）+ 行动按钮 + 目标选择。放在输入框上方，文字输入仍然可用。 */
@Composable
fun CombatPanel(c: CombatV1, busy: Boolean, onAction: (QuickActionV1, String) -> Unit, onOpenMech: (String) -> Unit = {}) {
    val alive = c.enemies.filter { !it.down }
    var target by rememberSaveable(c.encounter) { mutableStateOf(alive.firstOrNull()?.id ?: "") }
    if (alive.none { it.id == target }) target = alive.firstOrNull()?.id ?: ""
    var ally by rememberSaveable(c.encounter) { mutableStateOf(c.party.firstOrNull()?.id ?: "") }
    var menu by rememberSaveable(c.encounter) { mutableStateOf(Menu.Main) }

    fun fire(a: CombatActionV1) {
        val t = when (a.target) {
            "enemy" -> target
            "ally" -> ally
            else -> null
        }
        val qa = QuickActionV1(
            kind = "combat", action = a.kind, target = t, label = a.label,
            skill = if (a.kind == "skill" || a.kind == "mech") a.id else null,
            item = if (a.kind == "item") a.id else null,
        )
        menu = Menu.Main
        onAction(qa, a.label)
    }

    Column(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(c.title, style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text("第 ${c.round} 回合", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (c.mercuryMax > 0) {
                Spacer(Modifier.width(8.dp))
                SmallChip("${c.resourceName} ${c.mercury}/${c.mercuryMax}", MaterialTheme.colorScheme.error)
            }
        }
        Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            c.enemies.forEach { u -> UnitCard(u, selected = u.id == target, onClick = { if (!u.down) target = u.id }) }
        }
        Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            c.party.forEach { u -> UnitCard(u, selected = u.id == ally && c.party.size > 1, onClick = { ally = u.id }) }
        }
        val list = when (menu) {
            Menu.Main -> c.actions
            Menu.Skills -> c.skills
            Menu.Items -> c.items
        }
        Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            if (menu != Menu.Main) {
                FilterChip(selected = true, onClick = { menu = Menu.Main }, label = { Text("返回") })
            } else {
                if (c.skills.isNotEmpty()) FilterChip(selected = false, enabled = !busy && c.yourTurn, onClick = { menu = Menu.Skills }, label = { Text("技能 ▸") })
                if (c.items.isNotEmpty()) FilterChip(selected = false, enabled = !busy && c.yourTurn, onClick = { menu = Menu.Items }, label = { Text("道具 ▸") })
            }
            list.forEach { a ->
                AssistChip(
                    onClick = { fire(a) },
                    enabled = !busy && c.yourTurn && !a.disabled,
                    label = { Text(if (a.hint.isNullOrBlank()) a.label else "${a.label} · ${a.hint}") },
                    leadingIcon = { Icon(rpgIcon(a.icon, if (a.kind == "skill") "skill" else if (a.kind == "item") "consumable" else ""), null, Modifier.size(AssistChipDefaults.IconSize)) },
                    modifier = Modifier.semantics { if (a.disabled && !a.reason.isNullOrBlank()) contentDescription = "${a.label}（${a.reason}）" },
                )
            }
        }
        list.firstOrNull { it.disabled && !it.reason.isNullOrBlank() }?.let {
            Text("${it.label}：${it.reason}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
        }
    }
}

@Composable
private fun UnitCard(u: CombatUnitV1, selected: Boolean, onClick: () -> Unit) {
    val enemy = u.side == "enemy"
    val accent = if (enemy) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary
    Surface(
        shape = RoundedCornerShape(12.dp),
        color = if (selected) accent.copy(alpha = 0.10f) else MaterialTheme.colorScheme.surfaceContainerHigh,
        border = BorderStroke(if (selected) 2.dp else 1.dp, if (selected) accent else MaterialTheme.colorScheme.outlineVariant),
        modifier = Modifier.width(168.dp).alpha(if (u.down) 0.45f else 1f).clip(RoundedCornerShape(12.dp))
            .clickable(enabled = !u.down, onClick = onClick)
            .semantics(mergeDescendants = true) {
                this.selected = selected
                contentDescription = "${u.name} 生命 ${u.hp}/${u.maxHp}" + if (u.down) "，已倒下" else ""
            },
    ) {
        Column(Modifier.padding(8.dp), verticalArrangement = Arrangement.spacedBy(3.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Portrait(u.ref.ifBlank { u.id }, u.name, u.portrait, size = 28.dp, icon = if (u.mech) Icons.Outlined.PrecisionManufacturing else null)
                Spacer(Modifier.width(6.dp))
                Column(Modifier.weight(1f)) {
                    Text(u.name + if (u.current) " ◀" else "", style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    val sub = listOfNotNull("Lv${u.level}", u.tier?.takeIf { it.isNotBlank() }, u.mechName?.takeIf { u.mech }, if (u.defending) "防御中" else null)
                    Text(sub.joinToString(" · "), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                }
            }
            MiniBar("HP", u.hp, u.maxHp, if (enemy) MaterialTheme.colorScheme.error else Color(0xFF2E7D32))
            if (u.maxSp > 0) MiniBar("SP", u.sp, u.maxSp, MaterialTheme.colorScheme.primary)
            if (u.heatMax > 0) MiniBar("热", u.heat, u.heatMax, Color(0xFFE65100))
            if (u.statuses.isNotEmpty()) {
                Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    u.statuses.take(3).forEach { st ->
                        SmallChip(st.name + if (st.turns > 0) "·${st.turns}" else "", if (st.debuff) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.tertiary)
                    }
                }
            }
        }
    }
}

@Composable
private fun MiniBar(label: String, v: Int, max: Int, color: Color) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(label, style = MaterialTheme.typography.labelSmall, modifier = Modifier.width(22.dp))
        LinearProgressIndicator(
            progress = { if (max > 0) (v.toFloat() / max).coerceIn(0f, 1f) else 0f },
            color = color, trackColor = MaterialTheme.colorScheme.surfaceContainerHighest,
            modifier = Modifier.weight(1f).height(6.dp).clip(RoundedCornerShape(3.dp)),
        )
        Text("$v/$max", style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(start = 4.dp))
    }
}
