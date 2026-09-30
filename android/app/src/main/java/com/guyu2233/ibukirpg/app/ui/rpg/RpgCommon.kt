package com.guyu2233.ibukirpg.app.ui.rpg

import android.graphics.BitmapFactory
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Image
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
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.DirectionsRun
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material.icons.outlined.Eject
import androidx.compose.material.icons.outlined.Backpack
import androidx.compose.material.icons.outlined.Bolt
import androidx.compose.material.icons.outlined.Build
import androidx.compose.material.icons.outlined.Groups
import androidx.compose.material.icons.outlined.HelpOutline
import androidx.compose.material.icons.outlined.LocalHospital
import androidx.compose.material.icons.outlined.MenuBook
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material.icons.outlined.PrecisionManufacturing
import androidx.compose.material.icons.outlined.Science
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material.icons.outlined.SportsMartialArts
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.CardV1
import com.guyu2233.ibukirpg.app.data.MechCardV1
import com.guyu2233.ibukirpg.app.ui.common.parseAccent
import com.guyu2233.ibukirpg.app.ui.common.symbolFor

/** 数值 RPG 界面需要的按需数据（立绘 / 介绍卡 / 机甲卡）。截图测试可以注入假数据。 */
class RpgData(
    val portrait: suspend (String) -> ByteArray? = { null },
    val card: suspend (String) -> CardV1? = { null },
    val mech: suspend (String) -> MechCardV1? = { null },
)

val LocalRpgData = compositionLocalOf { RpgData() }

/** 故事包图标名 → Material 图标（找不到时按卡片种类给默认图标）。 */
fun rpgIcon(name: String?, kind: String = ""): ImageVector {
    val fallback = when (kind) {
        "skill" -> Icons.Outlined.AutoAwesome
        "enemy" -> Icons.Outlined.SportsMartialArts
        "mech" -> Icons.Outlined.PrecisionManufacturing
        "character" -> Icons.Outlined.Person
        "faction" -> Icons.Outlined.Groups
        "tech" -> Icons.Outlined.Science
        "lore" -> Icons.Outlined.MenuBook
        "weapon" -> Icons.Outlined.SportsMartialArts
        "armor", "accessory" -> Icons.Outlined.Shield
        "mech_part", "mech_weapon" -> Icons.Outlined.Build
        "consumable" -> Icons.Outlined.LocalHospital
        "item", "key", "material" -> Icons.Outlined.Backpack
        else -> Icons.Outlined.HelpOutline
    }
    return when (name) {
        null, "" -> fallback
        "bolt", "flash_on" -> Icons.Outlined.Bolt
        "precision_manufacturing", "smart_toy" -> Icons.Outlined.PrecisionManufacturing
        "swords", "sword", "attack" -> Icons.Outlined.SportsMartialArts
        "shield" -> Icons.Outlined.Shield
        "directions_run", "flee" -> Icons.AutoMirrored.Outlined.DirectionsRun
        "eject" -> Icons.Outlined.Eject
        else -> symbolFor(name, fallback)
    }
}

@Composable
fun rarityColor(hex: String?): Color = parseAccent(hex.orEmpty()) ?: MaterialTheme.colorScheme.outline

@Composable
fun toneColor(tone: String): Color = when (tone) {
    "positive", "success" -> MaterialTheme.colorScheme.primary
    "warning" -> MaterialTheme.colorScheme.tertiary
    "danger", "negative" -> MaterialTheme.colorScheme.error
    "mixed" -> MaterialTheme.colorScheme.secondary
    else -> MaterialTheme.colorScheme.outline
}

/** 立绘：有图时显示图片，否则显示带首字的圆形占位头像。 */
@Composable
fun Portrait(id: String, name: String, has: Boolean, size: Dp = 48.dp, shapeRadius: Dp? = null, icon: ImageVector? = null, fill: Boolean = false) {
    val data = LocalRpgData.current
    val bmp by produceState<ImageBitmap?>(null, id, has) {
        value = if (!has) null else runCatching {
            data.portrait(id)?.let { b -> BitmapFactory.decodeByteArray(b, 0, b.size)?.asImageBitmap() }
        }.getOrNull()
    }
    val shape = if (shapeRadius == null) CircleShape else RoundedCornerShape(shapeRadius)
    val mod = (if (fill) Modifier.fillMaxSize() else Modifier.size(size)).clip(shape).semantics { contentDescription = name }
    val img = bmp
    if (img != null) {
        Image(img, contentDescription = name, contentScale = ContentScale.Crop, modifier = mod)
    } else {
        Surface(color = MaterialTheme.colorScheme.secondaryContainer, contentColor = MaterialTheme.colorScheme.onSecondaryContainer, modifier = mod) {
            Box(contentAlignment = Alignment.Center) {
                if (icon != null) {
                    Icon(icon, contentDescription = null, modifier = Modifier.size(size * 0.55f))
                } else {
                    Text(name.take(1).ifBlank { "?" }, style = if (size >= 64.dp) MaterialTheme.typography.headlineMedium else MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                }
            }
        }
    }
}

/** 带标签的数值条。value<0 时显示“未知”。 */
@Composable
fun StatBar(label: String, value: Int, max: Int, color: Color, known: Boolean = true, suffix: String = "", bonus: Int = 0) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth().semantics(mergeDescendants = true) {
        contentDescription = if (known) "$label $value$suffix" else "$label 未知"
    }) {
        Text(label, style = MaterialTheme.typography.labelMedium, modifier = Modifier.width(64.dp))
        LinearProgressIndicator(
            progress = { if (known && max > 0) (value.toFloat() / max).coerceIn(0.02f, 1f) else 0f },
            color = color,
            trackColor = MaterialTheme.colorScheme.surfaceContainerHighest,
            modifier = Modifier.weight(1f).height(8.dp).clip(RoundedCornerShape(4.dp)),
        )
        val text = when {
            !known -> "未知"
            bonus != 0 -> "$value$suffix (${if (bonus > 0) "+" else ""}$bonus)"
            else -> "$value$suffix"
        }
        Text(
            text, style = MaterialTheme.typography.labelMedium, fontStyle = if (known) FontStyle.Normal else FontStyle.Italic,
            color = if (known) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.outline,
            modifier = Modifier.width(76.dp).padding(start = 8.dp),
        )
    }
}

@Composable
fun SmallChip(text: String, color: Color, filled: Boolean = false) {
    Surface(
        shape = RoundedCornerShape(8.dp),
        color = if (filled) color.copy(alpha = 0.16f) else MaterialTheme.colorScheme.surface,
        border = BorderStroke(1.dp, color.copy(alpha = 0.6f)),
    ) {
        Text(text, style = MaterialTheme.typography.labelSmall, color = color, modifier = Modifier.padding(horizontal = 8.dp, vertical = 3.dp))
    }
}

/** 统一的介绍卡内容（物品 / 装备 / 技能 / 敌人 / 势力 / 地点 / 图鉴条目）。 */
@OptIn(ExperimentalLayoutApi::class)
@Composable
fun CardBody(c: CardV1) {
    val rc = rarityColor(c.rarityColor)
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Surface(shape = RoundedCornerShape(12.dp), color = rc.copy(alpha = 0.14f), border = BorderStroke(1.dp, rc), modifier = Modifier.size(52.dp)) {
                Box(contentAlignment = Alignment.Center) { Icon(rpgIcon(c.icon, c.kind), contentDescription = null, tint = rc, modifier = Modifier.size(30.dp)) }
            }
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(c.name, style = MaterialTheme.typography.titleLarge)
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    SmallChip(c.kindName, MaterialTheme.colorScheme.secondary)
                    c.rarityName?.takeIf { it.isNotBlank() && c.known }?.let { SmallChip(it, rc, filled = true) }
                }
            }
        }
        if (c.stats.isNotEmpty()) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                c.stats.forEach { kv ->
                    Surface(shape = RoundedCornerShape(8.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh) {
                        Column(Modifier.padding(horizontal = 10.dp, vertical = 6.dp)) {
                            Text(kv.label, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Text(kv.value, style = MaterialTheme.typography.labelLarge)
                        }
                    }
                }
            }
        }
        c.description?.takeIf { it.isNotBlank() }?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
        c.lore?.takeIf { it.isNotBlank() }?.let {
            Text("“$it”", style = MaterialTheme.typography.bodySmall, fontStyle = FontStyle.Italic, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        if (c.tags.isNotEmpty()) {
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) { c.tags.filter { it != "mech_card" }.forEach { SmallChip(it, MaterialTheme.colorScheme.outline) } }
        }
    }
}

@Composable
fun CardDialog(c: CardV1, onDismiss: () -> Unit, onOpenMech: ((String) -> Unit)? = null) {
    AlertDialog(
        onDismissRequest = onDismiss,
        confirmButton = { TextButton(onClick = onDismiss) { Text("关闭") } },
        dismissButton = if (c.kind == "mech" && c.known && onOpenMech != null) {
            { TextButton(onClick = { onOpenMech(c.id) }) { Text("打开机甲卡") } }
        } else null,
        text = { Column(Modifier.verticalScroll(rememberScrollState())) { CardBody(c) } },
    )
}
