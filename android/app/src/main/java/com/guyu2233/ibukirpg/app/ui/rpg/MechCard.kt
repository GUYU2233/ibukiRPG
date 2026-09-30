package com.guyu2233.ibukirpg.app.ui.rpg

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Build
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.PrecisionManufacturing
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.guyu2233.ibukirpg.app.data.CardV1
import com.guyu2233.ibukirpg.app.data.MechCardV1
import com.guyu2233.ibukirpg.app.data.MechSlotV1
import com.guyu2233.ibukirpg.app.data.MechStatV1
import com.guyu2233.ibukirpg.app.data.QuickActionV1

/** 按 id 加载机甲卡并以全屏对话框展示。 */
@Composable
fun MechCardDialog(id: String, onDismiss: () -> Unit, onAction: (QuickActionV1) -> Unit, refreshKey: Any? = null) {
    val data = LocalRpgData.current
    val mech by produceState<MechCardV1?>(null, id, refreshKey) { value = runCatching { data.mech(id) }.getOrNull() }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        val m = mech
        if (m == null) {
            Surface(Modifier.fillMaxSize()) { Box(contentAlignment = Alignment.Center) { CircularProgressIndicator() } }
        } else {
            MechCardScreen(m, onBack = onDismiss, onAction = onAction)
        }
    }
}

/** MD3 机械甲胄卡：大图、状态、字段、属性条、挂载 / 改装、技能、履历。未掌握的字段显示“未知”。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MechCardScreen(m: MechCardV1, onBack: () -> Unit, onAction: (QuickActionV1) -> Unit) {
    var partCard by remember { mutableStateOf<CardV1?>(null) }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(m.name, maxLines = 1) },
                navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = "返回") } },
                actions = {
                    if (m.unknown > 0) {
                        AssistChip(onClick = {}, label = { Text("未知 ${m.unknown} 项") }, leadingIcon = { Icon(Icons.Outlined.Lock, null, Modifier.size(16.dp)) })
                        Spacer(Modifier.width(8.dp))
                    }
                },
            )
        },
    ) { pad ->
        Column(
            Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            MechHero(m)
            MechFields(m)
            if (m.specs.isNotEmpty()) Section("机体规格") { m.specs.forEach { SpecBar(it) } }
            if (m.stats.isNotEmpty()) Section(if (m.statsKnown) "战斗属性（含改装加成）" else "战斗属性") { m.stats.forEach { SpecBar(it, MaterialTheme.colorScheme.secondary) } }
            if (m.energy.isNotEmpty()) Section("能源") {
                m.energy.forEach { f -> FieldRow(f.label, f.value, f.known) }
            }
            if (m.hardpoints.isNotEmpty()) Section("武器挂点") { m.hardpoints.forEach { SlotRow(it, onAction) { c -> partCard = c } } }
            if (m.slots.isNotEmpty()) Section("改装槽") { m.slots.forEach { SlotRow(it, onAction) { c -> partCard = c } } }
            if (m.skills.isNotEmpty()) Section("机体技能") {
                m.skills.forEach { s ->
                    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth().clickable(enabled = s.known) { partCard = s }.padding(vertical = 4.dp)) {
                        Icon(rpgIcon(s.icon, "skill"), null, tint = if (s.known) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline)
                        Spacer(Modifier.width(8.dp))
                        UnknownText(s.name, s.known)
                    }
                }
            }
            Section("档案") {
                m.description?.takeIf { it.isNotBlank() }?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
                if (m.loreKnown && !m.lore.isNullOrBlank()) {
                    Text(m.lore, style = MaterialTheme.typography.bodySmall, fontStyle = FontStyle.Italic, color = MaterialTheme.colorScheme.onSurfaceVariant)
                } else {
                    UnknownText("来历：未知", false)
                }
            }
            if (m.history.isNotEmpty()) Section("履历") {
                m.history.forEach { h ->
                    Row(Modifier.padding(vertical = 2.dp)) {
                        Text("T${h.turn}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline, modifier = Modifier.width(44.dp))
                        Text(h.text, style = MaterialTheme.typography.bodySmall)
                    }
                }
            }
            Spacer(Modifier.height(24.dp))
        }
    }
    partCard?.let { CardDialog(it, onDismiss = { partCard = null }) }
}

@Composable
private fun MechHero(m: MechCardV1) {
    val rc = rarityColor(m.rarityColor)
    Card(shape = RoundedCornerShape(20.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
        Box(Modifier.fillMaxWidth().height(220.dp)) {
            Portrait(m.id, m.name, m.portrait, size = 96.dp, shapeRadius = 0.dp, icon = Icons.Outlined.PrecisionManufacturing, fill = true)
            Box(Modifier.fillMaxSize()) {
                Box(Modifier.align(Alignment.BottomCenter).fillMaxWidth().height(96.dp).background(Brush.verticalGradient(listOf(Color.Transparent, Color.Black.copy(alpha = 0.65f)))))
            }
            Column(Modifier.align(Alignment.BottomStart).padding(16.dp)) {
                Text(m.name, style = MaterialTheme.typography.headlineSmall, color = Color.White, fontWeight = FontWeight.Bold)
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                    StatusChip(m)
                    m.rarityName?.takeIf { it.isNotBlank() && m.known }?.let { SmallChip(it, rc, filled = true) }
                    if (m.enemy) SmallChip("敌方", MaterialTheme.colorScheme.error, filled = true)
                    if (m.owned) SmallChip("己方", MaterialTheme.colorScheme.primary, filled = true)
                }
            }
        }
    }
}

@Composable
fun StatusChip(m: MechCardV1) {
    val c = if (m.status.known) toneColor(m.status.tone) else MaterialTheme.colorScheme.outline
    Surface(shape = RoundedCornerShape(8.dp), color = c.copy(alpha = 0.9f)) {
        Text(
            if (m.status.known) m.status.name else "状态未知",
            style = MaterialTheme.typography.labelMedium, color = Color.White,
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 4.dp),
        )
    }
}

@Composable
private fun MechFields(m: MechCardV1) {
    OutlinedCard(shape = RoundedCornerShape(16.dp)) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            m.fields.forEach { FieldRow(it.label, it.value, it.known) }
        }
    }
}

@Composable
private fun FieldRow(label: String, value: String, known: Boolean) {
    Row(Modifier.fillMaxWidth()) {
        Text(label, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.width(96.dp))
        UnknownText(if (known) value else "未知", known)
    }
}

@Composable
fun UnknownText(text: String, known: Boolean) {
    Text(
        text,
        style = MaterialTheme.typography.bodyMedium,
        fontStyle = if (known) FontStyle.Normal else FontStyle.Italic,
        color = if (known) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.outline,
    )
}

@Composable
private fun Section(title: String, content: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(title, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary)
        content()
        HorizontalDivider(Modifier.padding(top = 4.dp))
    }
}

@Composable
private fun SpecBar(s: MechStatV1, color: Color = MaterialTheme.colorScheme.primary) {
    StatBar(s.name, s.value, s.max, color, known = s.known, suffix = s.unit.orEmpty(), bonus = s.bonus)
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun SlotRow(s: MechSlotV1, onAction: (QuickActionV1) -> Unit, onPart: (CardV1) -> Unit) {
    val part = s.part
    Surface(shape = RoundedCornerShape(12.dp), color = MaterialTheme.colorScheme.surfaceContainer, border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant)) {
        Column(Modifier.fillMaxWidth().padding(10.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(if (s.hardpoint) rpgIcon(null, "weapon") else Icons.Outlined.Build, null, tint = MaterialTheme.colorScheme.secondary, modifier = Modifier.size(20.dp))
                Spacer(Modifier.width(8.dp))
                Column(Modifier.weight(1f)) {
                    Text(s.name + (s.kind?.takeIf { it.isNotBlank() }?.let { " · $it" } ?: ""), style = MaterialTheme.typography.labelLarge)
                    when {
                        !s.known -> UnknownText("未知", false)
                        part == null -> UnknownText("空", false)
                        else -> Text(part.name, style = MaterialTheme.typography.bodyMedium, color = rarityColor(part.rarityColor), modifier = Modifier.clickable { onPart(part) })
                    }
                }
                s.remove?.let { a -> TextButton(onClick = { onAction(a) }) { Text(a.label ?: "卸下") } }
            }
            if (s.install.isNotEmpty()) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    s.install.forEach { a -> AssistChip(onClick = { onAction(a) }, label = { Text(a.label ?: "安装") }) }
                }
            }
        }
    }
}
