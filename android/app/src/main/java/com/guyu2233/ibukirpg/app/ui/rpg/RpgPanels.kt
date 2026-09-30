package com.guyu2233.ibukirpg.app.ui.rpg

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.PrecisionManufacturing
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.CardV1
import com.guyu2233.ibukirpg.app.data.CardsV1
import com.guyu2233.ibukirpg.app.data.CharacterCardV1
import com.guyu2233.ibukirpg.app.data.CodexV1
import com.guyu2233.ibukirpg.app.data.GrowthV1
import com.guyu2233.ibukirpg.app.data.MainlineV1
import com.guyu2233.ibukirpg.app.data.MechCardV1
import com.guyu2233.ibukirpg.app.data.MechsV1
import com.guyu2233.ibukirpg.app.data.QuickActionV1
import com.guyu2233.ibukirpg.app.data.RelationsV1
import kotlin.math.cos
import kotlin.math.roundToInt
import kotlin.math.sin

private fun LazyListScope.title(text: String) {
    item { Text(text, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = 4.dp)) }
}

// ---------------- 成长 ----------------

fun LazyListScope.growthSection(
    g: GrowthV1,
    mainline: MainlineV1?,
    busy: Boolean,
    onAction: (QuickActionV1, String) -> Unit,
    dialogs: RpgDialogState,
) {
    item {
        Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
            Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("Lv ${g.level}", style = MaterialTheme.typography.headlineSmall, modifier = Modifier.weight(1f))
                    if (g.attrPoints > 0) SmallChip("属性点 ${g.attrPoints}", MaterialTheme.colorScheme.tertiary, filled = true)
                    Spacer(Modifier.width(6.dp))
                    if (g.skillPoints > 0) SmallChip("技能点 ${g.skillPoints}", MaterialTheme.colorScheme.secondary, filled = true)
                }
                StatBar("经验", g.xp, g.xpNext, MaterialTheme.colorScheme.tertiary, suffix = "/${g.xpNext}")
                StatBar("生命", g.hp, g.maxHp, Color(0xFF2E7D32), suffix = "/${g.maxHp}")
                if (g.mercuryMax > 0) StatBar(g.resourceName.ifBlank { "能源" }, g.mercury, g.mercuryMax, MaterialTheme.colorScheme.error, suffix = "/${g.mercuryMax}")
                if (g.hasMech && !g.mechName.isNullOrBlank()) {
                    Text("座驾：${g.mechName}", style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
    }
    if (g.attributes.isNotEmpty()) {
        title("属性分配")
        items(g.attributes, key = { "attr" + it.id }) { a ->
            Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.fillMaxWidth()) {
                Column(Modifier.weight(1f)) {
                    Text("${a.name}  ${a.value}", style = MaterialTheme.typography.titleSmall)
                    a.effect?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                }
                a.allocate?.let { qa ->
                    IconButton(onClick = { onAction(qa, qa.label ?: "加点") }, enabled = !busy) { Icon(Icons.Outlined.Add, contentDescription = "提升${a.name}") }
                }
            }
        }
    }
    if (g.stats.isNotEmpty()) {
        title("战斗属性")
        item {
            @OptIn(ExperimentalLayoutApi::class)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                g.stats.forEach { st ->
                    Surface(shape = RoundedCornerShape(8.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh) {
                        Column(Modifier.padding(horizontal = 10.dp, vertical = 6.dp)) {
                            Text(st.name, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Text("${st.value}", style = MaterialTheme.typography.titleMedium)
                        }
                    }
                }
            }
        }
    }
    if (g.equipment.isNotEmpty()) {
        title("装备")
        items(g.equipment, key = { "eq" + it.slot }) { e ->
            ListItem(
                headlineContent = { Text(e.item?.name ?: "（空）", color = if (e.item != null) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.outline) },
                overlineContent = { Text(e.name) },
                leadingContent = { Icon(rpgIcon(e.item?.icon, e.item?.kind ?: "armor"), null, tint = e.item?.let { rarityColor(it.rarityColor) } ?: MaterialTheme.colorScheme.outline) },
                trailingContent = { e.unequip?.let { qa -> TextButton(onClick = { onAction(qa, qa.label ?: "卸下") }, enabled = !busy) { Text("卸下") } } },
                modifier = Modifier.clickable(enabled = e.item != null) { e.item?.let(dialogs::openCard) },
            )
        }
    }
    if (g.skills.isNotEmpty()) {
        title("技能")
        items(g.skills, key = { "sk" + it.card.id }) { s ->
            ListItem(
                headlineContent = { Text(s.card.name) },
                supportingContent = { Text(s.blocked ?: s.card.description.orEmpty(), maxLines = 2, overflow = TextOverflow.Ellipsis) },
                leadingContent = { Icon(rpgIcon(s.card.icon, "skill"), null, tint = if (s.learned) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline) },
                trailingContent = {
                    when {
                        s.learned -> SmallChip("已习得", MaterialTheme.colorScheme.primary)
                        s.learn != null -> FilledTonalButton(onClick = { onAction(s.learn, s.learn.label ?: "学习") }, enabled = !busy) { Text("学习 ${s.cost}") }
                        else -> {}
                    }
                },
                modifier = Modifier.clickable { dialogs.openCard(s.card) },
            )
        }
    }
    if (mainline != null) {
        title("主线敏感度")
        item {
            val cur = when {
                mainline.mild >= 45 -> "relaxed"
                mainline.mild <= 25 -> "strict"
                else -> "standard"
            }
            val opts = listOf("relaxed" to "宽松", "standard" to "标准", "strict" to "严格")
            Column {
                SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                    opts.forEachIndexed { i, (id, label) ->
                        SegmentedButton(
                            selected = cur == id,
                            onClick = { onAction(QuickActionV1(kind = "manage", action = "thresholds", target = id, label = "敏感度：$label"), "敏感度：$label") },
                            enabled = !busy,
                            shape = SegmentedButtonDefaults.itemShape(i, opts.size),
                        ) { Text(label) }
                    }
                }
                Text(
                    "偏离 ${mainline.deviation} · 轻度 ${mainline.mild} / 重度 ${mainline.heavy}",
                    style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp),
                )
            }
        }
    }
}

// ---------------- 人物卡 ----------------

fun LazyListScope.cardsTab(cards: CardsV1, dialogs: RpgDialogState, onPerson: (CharacterCardV1) -> Unit) {
    fun group(label: String, list: List<CharacterCardV1>) {
        if (list.isEmpty()) return
        title("$label（${list.size}）")
        items(list, key = { "cc" + label + it.id }) { c -> CharacterCardItem(c, dialogs, onPerson) }
    }
    group("活跃", cards.active)
    group("归档", cards.archived)
    group("已故", cards.dead)
    if (cards.active.isEmpty() && cards.archived.isEmpty() && cards.dead.isEmpty()) {
        item { Text("还没有认识的人物。", color = MaterialTheme.colorScheme.onSurfaceVariant) }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun CharacterCardItem(c: CharacterCardV1, dialogs: RpgDialogState, onPerson: (CharacterCardV1) -> Unit) {
    var open by rememberSaveable(c.id) { mutableStateOf(false) }
    val dead = c.status == "dead"
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
        modifier = Modifier.fillMaxWidth().clickable { open = !open },
    ) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Portrait(c.id, c.name, c.portrait, size = 56.dp)
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(c.name + if (dead) " †" else "", style = MaterialTheme.typography.titleMedium)
                    Text(listOfNotNull(c.role, c.faction, if (c.level > 0) "Lv${c.level}" else null).joinToString(" · "), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    c.location?.let { Text("位于 $it", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline) }
                }
                if (c.dynamic) SmallChip("新登场", MaterialTheme.colorScheme.tertiary)
            }
            if (c.relation.isNotEmpty()) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    c.relation.forEach { d -> SmallChip("${d.name} ${if (d.value > 0) "+" else ""}${d.value}", if (d.negative == (d.value > 0)) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary) }
                }
            }
            if (c.mechs.isNotEmpty()) {
                FlowRow(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    c.mechs.forEach { m ->
                        AssistChip(onClick = { dialogs.mech = m }, label = { Text("机甲卡") }, leadingIcon = { Icon(Icons.Outlined.PrecisionManufacturing, null, Modifier.size(16.dp)) })
                    }
                }
            }
            if (open) {
                c.description?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
                c.reason?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error) }
                c.lore?.let { Text(it, style = MaterialTheme.typography.bodySmall, fontStyle = FontStyle.Italic, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                if (c.stats.isNotEmpty()) Text(c.stats.joinToString("  ") { "${it.name} ${it.value}" }, style = MaterialTheme.typography.labelMedium)
                c.memories.take(4).forEach { Text("· $it", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                TextButton(onClick = { onPerson(c) }) { Text("查看关系") }
            }
        }
    }
}

// ---------------- 关系网 ----------------

fun LazyListScope.relationsTab(r: RelationsV1, focus: String?, onFocus: (String?) -> Unit) {
    item {
        Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            FilterChip(selected = focus == null, onClick = { onFocus(null) }, label = { Text("全部") })
            r.people.forEach { p -> FilterChip(selected = focus == p.id, onClick = { onFocus(p.id) }, label = { Text(p.name) }) }
        }
    }
    item { RelationGraph(r, focus, onFocus) }
    val edges = r.edges.filter { focus == null || it.from == focus || it.to == focus }
    title("关系（${edges.size}）")
    items(edges, key = { "e" + it.from + ">" + it.to }) { e ->
        val c = toneColor(e.tone)
        Surface(shape = RoundedCornerShape(12.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, border = BorderStroke(1.dp, c.copy(alpha = 0.5f))) {
            Column(Modifier.fillMaxWidth().padding(10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("${e.fromName} → ${e.toName}", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                    SmallChip(e.label, c, filled = true)
                }
                e.values.forEach { v -> RelMeter(v.name, v.value, v.negative) }
                Text(
                    listOfNotNull(sourceLabel(e.source), if (e.turn > 0) "第 ${e.turn} 回合" else null, e.note).joinToString(" · "),
                    style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline,
                )
            }
        }
    }
    val hist = r.history.filter { focus == null || it.from == focus || it.to == focus }
    if (hist.isNotEmpty()) {
        title("变化记录")
        items(hist.asReversed().take(30), key = { "h" + it.turn + it.from + it.to + it.text.hashCode() }) { h ->
            Row {
                Text("T${h.turn}", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline, modifier = Modifier.width(44.dp))
                Text(h.text, style = MaterialTheme.typography.bodySmall)
            }
        }
    }
}

private fun sourceLabel(src: String): String? = when (src) {
    "" -> null
    "self" -> "亲身经历"
    "player", "witness" -> "亲眼所见"
    "public" -> "众所周知"
    "rumor" -> "传闻"
    "told" -> "听人说起"
    "inferred" -> "推测"
    else -> src
}

@Composable
private fun RelMeter(name: String, value: Int, negative: Boolean) {
    val bad = if (negative) value > 0 else value < 0
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.semantics(mergeDescendants = true) { contentDescription = "$name $value" }) {
        Text(name, style = MaterialTheme.typography.labelMedium, modifier = Modifier.width(48.dp))
        LinearProgressIndicator(
            progress = { (kotlin.math.abs(value.coerceIn(-100, 100)) / 100f).coerceAtLeast(0.02f) },
            color = if (bad) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
            modifier = Modifier.weight(1f).clip(RoundedCornerShape(3.dp)),
        )
        Text("$value", style = MaterialTheme.typography.labelMedium, modifier = Modifier.width(40.dp).padding(start = 8.dp))
    }
}

/** 简易环形关系图：人物排成一圈，连线颜色表示关系倾向；点头像筛选。 */
@Composable
private fun RelationGraph(r: RelationsV1, focus: String?, onFocus: (String?) -> Unit) {
    val people = r.people.take(12)
    if (people.size < 2) return
    val pos = remember(people) {
        people.mapIndexed { i, p ->
            val a = if (p.player) -Math.PI / 2 else -Math.PI / 2 + 2 * Math.PI * i / people.size
            p.id to Offset(cos(a).toFloat(), sin(a).toFloat())
        }.toMap()
    }
    val tones = r.edges.map { it.tone }.distinct()
    val colors = tones.associateWith { toneColor(it) }
    val neutral = MaterialTheme.colorScheme.outlineVariant
    val density = LocalDensity.current
    Surface(shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainerLow, modifier = Modifier.fillMaxWidth().aspectRatio(1.1f)) {
        Box(Modifier.fillMaxSize().padding(start = 28.dp, end = 28.dp, top = 24.dp, bottom = 40.dp)) {
            Canvas(Modifier.fillMaxSize()) {
                val cx = size.width / 2
                val cy = size.height / 2
                val rad = minOf(cx, cy)
                r.edges.forEach { e ->
                    val a = pos[e.from] ?: return@forEach
                    val b = pos[e.to] ?: return@forEach
                    val hi = focus == null || e.from == focus || e.to == focus
                    drawLine(
                        (colors[e.tone] ?: neutral).copy(alpha = if (hi) 0.85f else 0.15f),
                        Offset(cx + a.x * rad, cy + a.y * rad), Offset(cx + b.x * rad, cy + b.y * rad),
                        strokeWidth = if (hi) 4f else 2f,
                    )
                }
            }
            androidx.compose.foundation.layout.BoxWithConstraints(Modifier.fillMaxSize()) {
                val w = with(density) { maxWidth.toPx() }
                val h = with(density) { maxHeight.toPx() }
                val rad = minOf(w, h) / 2
                val node = with(density) { 40.dp.toPx() }
                people.forEach { p ->
                    val o = pos[p.id] ?: return@forEach
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        modifier = Modifier.offset { IntOffset((w / 2 + o.x * rad - node / 2).roundToInt(), (h / 2 + o.y * rad - node / 2).roundToInt()) }
                            .clickable { onFocus(if (focus == p.id) null else p.id) },
                    ) {
                        Surface(shape = RoundedCornerShape(50), border = if (focus == p.id) BorderStroke(2.dp, MaterialTheme.colorScheme.primary) else null) {
                            Portrait(p.id, p.name, p.portrait, size = 40.dp)
                        }
                        Text(p.name, style = MaterialTheme.typography.labelSmall, maxLines = 1)
                    }
                }
            }
        }
    }
}

// ---------------- 图鉴 ----------------

fun LazyListScope.codexTab(codex: CodexV1, mechs: MechsV1?, category: String?, onCategory: (String?) -> Unit, dialogs: RpgDialogState) {
    item {
        Column {
            Text("已收录 ${codex.unlocked} / ${codex.total}", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Row(Modifier.horizontalScroll(rememberScrollState()).padding(top = 6.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                FilterChip(selected = category == null, onClick = { onCategory(null) }, label = { Text("全部") })
                if (mechs != null && mechs.mechs.isNotEmpty()) FilterChip(selected = category == "mechs", onClick = { onCategory("mechs") }, label = { Text("机械甲胄") })
                codex.categories.forEach { c -> FilterChip(selected = category == c.kind, onClick = { onCategory(c.kind) }, label = { Text("${c.name} ${c.unlocked}") }) }
            }
        }
    }
    if (mechs != null && mechs.mechs.isNotEmpty() && (category == null || category == "mechs")) {
        title("机械甲胄")
        items(mechs.mechs, key = { "m" + it.id }) { m -> MechListItem(m) { dialogs.mech = m.id } }
    }
    codex.categories.filter { category == null || category == it.kind }.forEach { cat ->
        if (cat.kind == "mech" && mechs != null && mechs.mechs.isNotEmpty()) return@forEach
        title("${cat.name}（${cat.unlocked}/${cat.entries.size}）")
        item(key = "cat" + cat.kind) {
            @OptIn(ExperimentalLayoutApi::class)
            FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                cat.entries.forEach { e -> CodexTile(e) { if (e.known) dialogs.openCard(e) } }
            }
        }
    }
}

@Composable
private fun CodexTile(e: CardV1, onClick: () -> Unit) {
    val rc = if (e.known) rarityColor(e.rarityColor) else MaterialTheme.colorScheme.outlineVariant
    Surface(
        shape = RoundedCornerShape(12.dp),
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        border = BorderStroke(1.dp, rc),
        modifier = Modifier.width(104.dp).clip(RoundedCornerShape(12.dp)).clickable(enabled = e.known, onClick = onClick),
    ) {
        Column(Modifier.padding(8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
            Icon(rpgIcon(if (e.known) e.icon else "help", e.kind), null, tint = rc, modifier = Modifier.size(28.dp))
            Text(if (e.known) e.name else "？？？", style = MaterialTheme.typography.labelMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}

@Composable
fun MechListItem(m: MechCardV1, onClick: () -> Unit) {
    Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh), modifier = Modifier.fillMaxWidth().clickable(enabled = m.known, onClick = onClick)) {
        ListItem(
            colors = ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
            leadingContent = { Portrait(m.id, m.name, m.portrait, size = 56.dp, shapeRadius = 12.dp, icon = Icons.Outlined.PrecisionManufacturing) },
            headlineContent = { Text(m.name) },
            supportingContent = {
                if (!m.known) {
                    Text("尚未解锁：在故事中遭遇后记录", color = MaterialTheme.colorScheme.outline)
                    return@ListItem
                }
                val model = m.fields.firstOrNull { it.id == "model" }
                val pilot = m.fields.firstOrNull { it.id == "pilot" }
                Text(
                    listOfNotNull(model?.let { if (it.known) it.value else "型号未知" }, pilot?.let { "驾驶者：" + if (it.known) it.value else "未知" }).joinToString(" · "),
                    maxLines = 2, overflow = TextOverflow.Ellipsis,
                )
            },
            trailingContent = { StatusChip(m) },
        )
    }
}

@Composable
fun MechLinkButton(mech: String, onOpen: (String) -> Unit) {
    OutlinedButton(onClick = { onOpen(mech) }) {
        Icon(Icons.Outlined.PrecisionManufacturing, null, Modifier.size(16.dp))
        Spacer(Modifier.width(4.dp))
        Text("机甲卡")
    }
}
