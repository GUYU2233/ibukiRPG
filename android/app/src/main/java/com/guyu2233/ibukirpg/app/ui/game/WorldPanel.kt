package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.MenuBook
import androidx.compose.material.icons.outlined.Backpack
import androidx.compose.material.icons.outlined.Campaign
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.EditNote
import androidx.compose.material.icons.outlined.Event
import androidx.compose.material.icons.outlined.Flag
import androidx.compose.material.icons.outlined.Hub
import androidx.compose.material.icons.outlined.Map
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material.icons.outlined.Star
import androidx.compose.material.icons.outlined.Timeline
import androidx.compose.material.icons.outlined.Undo
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.OutlinedButton
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.PrimaryScrollableTabRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.data.EntityFieldV1
import com.guyu2233.ibukirpg.app.data.EntityViewV1
import com.guyu2233.ibukirpg.app.data.QuickActionV1
import com.guyu2233.ibukirpg.app.data.WorldChangeV1
import com.guyu2233.ibukirpg.app.data.WorldEventChipV1
import com.guyu2233.ibukirpg.app.data.WorldPanelV1
import com.guyu2233.ibukirpg.app.ui.rpg.RpgDialogHost
import com.guyu2233.ibukirpg.app.ui.rpg.codexTab
import com.guyu2233.ibukirpg.app.ui.rpg.relationsTab
import com.guyu2233.ibukirpg.app.ui.rpg.rememberRpgDialogState

/** 世界面板的 8 个标签页（与 Go 端 dto.WorldTabs 顺序一致）。 */
enum class WorldTab(val id: String, val label: String, val icon: ImageVector) {
    Characters("characters", "角色", Icons.Outlined.Person),
    Relations("relations", "关系网", Icons.Outlined.Hub),
    Codex("codex", "图鉴", Icons.AutoMirrored.Outlined.MenuBook),
    Equipment("equipment", "装备", Icons.Outlined.Backpack),
    Map("map", "地图", Icons.Outlined.Map),
    Factions("factions", "势力", Icons.Outlined.Flag),
    Timeline("timeline", "时间线", Icons.Outlined.Timeline),
    Log("log", "日志", Icons.Outlined.EditNote),
}

/** 世界面板数据：每个标签页按需加载。 */
data class WorldState(val tabs: Map<String, WorldPanelV1> = emptyMap(), val loading: String? = null)

/** 世界面板的附加操作：让 AI 修改 / 新建卡片、立即运行一致性检查。为空时不显示对应按钮。 */
data class WorldExtras(
    val onEdit: ((EntityViewV1) -> Unit)? = null,
    val onNewCard: (() -> Unit)? = null,
    val onRunAudit: (() -> Unit)? = null,
    val auditBusy: Boolean = false,
    /** 0.2.0-rc1：从“外部工具修改了 N 项设定”打开时直接进入日志页，并按来源筛选（mcp）。 */
    val logSource: String? = null,
)

/** 手机：底部抽屉。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun WorldPanelSheet(
    world: WorldState,
    busy: Boolean,
    onDismiss: () -> Unit,
    onTab: (String) -> Unit,
    onAction: (QuickActionV1, String) -> Unit,
    onRevert: (WorldChangeV1) -> Unit,
    extras: WorldExtras = WorldExtras(),
) {
    val sheet = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheet) {
        WorldPanelContent(world, busy, onTab, onAction, onRevert, Modifier.fillMaxHeight(0.9f), extras = extras)
    }
}

/** 平板（≥ 840dp）：右侧常驻侧栏。 */
@Composable
fun WorldPanelPane(
    world: WorldState,
    busy: Boolean,
    onClose: () -> Unit,
    onTab: (String) -> Unit,
    onAction: (QuickActionV1, String) -> Unit,
    onRevert: (WorldChangeV1) -> Unit,
    modifier: Modifier = Modifier,
    extras: WorldExtras = WorldExtras(),
) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainerLow, modifier = modifier.fillMaxHeight()) {
        WorldPanelContent(world, busy, onTab, onAction, onRevert, onClose = onClose, extras = extras)
    }
}

@Composable
fun WorldPanelContent(
    world: WorldState,
    busy: Boolean,
    onTab: (String) -> Unit,
    onAction: (QuickActionV1, String) -> Unit,
    onRevert: (WorldChangeV1) -> Unit,
    modifier: Modifier = Modifier,
    initialTab: Int = 0,
    onClose: (() -> Unit)? = null,
    extras: WorldExtras = WorldExtras(),
) {
    var tab by rememberSaveable(extras.logSource) { mutableStateOf(if (extras.logSource != null) WorldTab.Log.ordinal else initialTab) }
    var onlySource by rememberSaveable(extras.logSource) { mutableStateOf(extras.logSource) }
    var focus by rememberSaveable { mutableStateOf<String?>(null) }
    var category by rememberSaveable { mutableStateOf<String?>(null) }
    val dialogs = rememberRpgDialogState()
    val current = WorldTab.entries[tab]
    LaunchedEffect(current) { onTab(current.id) }
    Column(modifier) {
        Row(Modifier.fillMaxWidth().padding(start = 20.dp, end = 8.dp, top = 4.dp), verticalAlignment = Alignment.CenterVertically) {
            Text("世界面板", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            if (onClose != null) IconButton(onClick = onClose) { Icon(Icons.Outlined.Close, contentDescription = "关闭") }
        }
        PrimaryScrollableTabRow(selectedTabIndex = tab, edgePadding = 8.dp) {
            WorldTab.entries.forEachIndexed { i, t ->
                Tab(selected = tab == i, onClick = { tab = i }, text = { Text(t.label) }, icon = { Icon(t.icon, contentDescription = null) })
            }
        }
        val data = world.tabs[current.id]
        if (data == null) {
            Box(Modifier.fillMaxWidth().padding(48.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
            return@Column
        }
        LazyColumn(
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier.fillMaxWidth(),
        ) {
            when (current) {
                WorldTab.Characters, WorldTab.Map, WorldTab.Factions -> {
                    if (current == WorldTab.Characters && extras.onNewCard != null) {
                        item(key = "new-card") {
                            OutlinedButton(onClick = extras.onNewCard, enabled = !busy) {
                                Icon(Icons.Outlined.Add, contentDescription = null, modifier = Modifier.size(18.dp))
                                Spacer(Modifier.width(6.dp))
                                Text("新建角色 / 卡片")
                            }
                        }
                    }
                    entities(data.entities, current, extras.onEdit, busy)
                }
                WorldTab.Relations -> data.relations?.let { relationsTab(it, focus) { f -> focus = f } } ?: empty("还没有认识的人")
                WorldTab.Codex -> data.codex?.let { codexTab(it, null, category, { c -> category = c }, dialogs) } ?: empty("图鉴还是空的")
                WorldTab.Equipment -> data.inventory?.let { inventoryTab(it, busy, onAction, dialogs) } ?: empty("身上什么都没有")
                WorldTab.Timeline -> events(data.events)
                WorldTab.Log -> {
                    extras.onRunAudit?.let { run ->
                        item(key = "audit") {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text("一致性检查会对照叙事与设定，补记遗漏、修正矛盾。", style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                                OutlinedButton(onClick = run, enabled = !busy && !extras.auditBusy) {
                                    if (extras.auditBusy) CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp) else Text("立即检查")
                                }
                            }
                        }
                    }
                    if (onlySource != null) {
                        item(key = "source-filter") {
                            FilterChip(selected = true, onClick = { onlySource = null }, label = { Text("只看外部工具的修改 ✕") })
                        }
                    }
                    changes(onlySource?.let { src -> data.changes.filter { it.source == src } } ?: data.changes, busy, onRevert)
                }
            }
            item {
                Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(top = 4.dp)) {
                    Icon(Icons.Outlined.VisibilityOff, contentDescription = null, modifier = Modifier.size(14.dp), tint = MaterialTheme.colorScheme.outline)
                    Spacer(Modifier.width(6.dp))
                    Text(
                        "这里只显示你的角色知道的事；“未知”的内容会随剧情揭示。",
                        style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline,
                    )
                }
            }
        }
    }
    RpgDialogHost(dialogs, onAction, null)
}

private fun LazyListScope.empty(text: String) {
    item { Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(24.dp)) }
}

private fun LazyListScope.entities(list: List<EntityViewV1>, tab: WorldTab, onEdit: ((EntityViewV1) -> Unit)?, busy: Boolean) {
    if (list.isEmpty()) {
        empty(
            when (tab) {
                WorldTab.Factions -> "还没有听说过任何势力"
                WorldTab.Map -> "还没有去过任何地方"
                else -> "还没有认识的人"
            },
        )
        return
    }
    items(list, key = { it.id }) { e -> EntityCard(e, onEdit?.takeIf { !busy && e.id != "player" }) }
}

@Composable
fun EntityCard(e: EntityViewV1, onEdit: ((EntityViewV1) -> Unit)? = null) {
    val c = MaterialTheme.colorScheme
    Surface(
        shape = RoundedCornerShape(16.dp),
        border = BorderStroke(1.dp, c.outlineVariant),
        color = c.surfaceContainerLowest,
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(e.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                if (e.here) {
                    Spacer(Modifier.width(8.dp))
                    Icon(Icons.Outlined.Place, contentDescription = "你在这里", tint = c.primary, modifier = Modifier.size(16.dp))
                }
                e.retired?.let { Spacer(Modifier.width(8.dp)); Badge(it, c.errorContainer, c.onErrorContainer) }
                Spacer(Modifier.weight(1f))
                e.progress?.let {
                    Text(it, style = MaterialTheme.typography.labelMedium, color = c.tertiary, modifier = Modifier.padding(start = 8.dp))
                }
            }
            e.fields.forEach { f -> FieldRow(f) }
            e.secrets.forEach { s ->
                Row(verticalAlignment = Alignment.Top) {
                    Icon(Icons.Outlined.Star, contentDescription = null, tint = c.tertiary, modifier = Modifier.size(16.dp).padding(top = 2.dp))
                    Spacer(Modifier.width(6.dp))
                    Text(s, style = MaterialTheme.typography.bodySmall, color = c.tertiary)
                }
            }
            if (onEdit != null) {
                TextButton(onClick = { onEdit(e) }, contentPadding = PaddingValues(horizontal = 0.dp)) {
                    Icon(Icons.Outlined.Edit, contentDescription = null, modifier = Modifier.size(16.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("让 AI 修改")
                }
            }
        }
    }
}

@Composable
private fun FieldRow(f: EntityFieldV1) {
    val c = MaterialTheme.colorScheme
    Row(verticalAlignment = Alignment.Top) {
        Text(f.label, style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant, modifier = Modifier.width(72.dp))
        when {
            f.value.isNullOrBlank() || f.level == "unknown" -> UnknownTag()
            else -> Text(
                (if (f.level == "rumored") "（传闻）" else "") + f.value,
                style = MaterialTheme.typography.bodyMedium,
                color = if (f.changed) c.primary else c.onSurface,
                modifier = Modifier.weight(1f),
            )
        }
    }
}

@Composable
fun UnknownTag() {
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHighest, shape = RoundedCornerShape(6.dp)) {
        Text("未知", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp))
    }
}

private fun LazyListScope.events(list: List<WorldEventChipV1>) {
    if (list.isEmpty()) { empty("还不知道接下来会发生什么"); return }
    items(list, key = { it.id }) { ev -> EventCard(ev) }
}

@Composable
private fun EventCard(ev: WorldEventChipV1) {
    val c = MaterialTheme.colorScheme
    Surface(shape = RoundedCornerShape(16.dp), border = BorderStroke(1.dp, if (ev.pivotal) c.tertiary else c.outlineVariant), color = c.surfaceContainerLowest, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(if (ev.rumored) Icons.Outlined.Campaign else Icons.Outlined.Event, contentDescription = null, tint = c.primary, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(8.dp))
                Text(ev.title, style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                Badge(
                    ev.statusLabel + (ev.countdown?.takeIf { ev.status == "scheduled" }?.let { " · $it" } ?: ""),
                    if (ev.status == "active") c.primaryContainer else c.surfaceContainerHighest,
                    if (ev.status == "active") c.onPrimaryContainer else c.onSurfaceVariant,
                )
            }
            Text(
                listOfNotNull(ev.time, ev.location?.takeIf { it.isNotBlank() }).joinToString(" · "),
                style = MaterialTheme.typography.labelMedium, color = c.onSurfaceVariant,
            )
            ev.summary?.takeIf { it.isNotBlank() }?.let { Text(it, style = MaterialTheme.typography.bodyMedium) }
            ev.outcome?.takeIf { it.isNotBlank() }?.let { Text("结果：$it", style = MaterialTheme.typography.bodyMedium, color = c.primary) }
            if (ev.pivotal) Text("关键事件：错过或改变它会显著影响世界", style = MaterialTheme.typography.labelSmall, color = c.tertiary)
        }
    }
}

private fun LazyListScope.changes(list: List<WorldChangeV1>, busy: Boolean, onRevert: (WorldChangeV1) -> Unit) {
    if (list.isEmpty()) { empty("世界还没有发生变化"); return }
    items(list, key = { it.id }) { ch -> ChangeCard(ch, busy, onRevert) }
}

@Composable
private fun ChangeCard(ch: WorldChangeV1, busy: Boolean, onRevert: (WorldChangeV1) -> Unit) {
    val c = MaterialTheme.colorScheme
    Surface(shape = RoundedCornerShape(16.dp), border = BorderStroke(1.dp, c.outlineVariant), color = c.surfaceContainerLowest, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("回合 ${ch.turn} · ${ch.sourceLabel}", style = MaterialTheme.typography.labelMedium, color = c.onSurfaceVariant, modifier = Modifier.weight(1f))
                ch.time?.let { Text(it, style = MaterialTheme.typography.labelSmall, color = c.outline) }
            }
            Text(
                ch.summary + if (ch.revertedBy != null) "（已撤销）" else "",
                style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium,
            )
            ch.reason?.takeIf { it.isNotBlank() }?.let { Text("原因：$it", style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant) }
            if (ch.canRevert && ch.revertedBy == null) {
                TextButton(onClick = { onRevert(ch) }, enabled = !busy, contentPadding = PaddingValues(horizontal = 0.dp)) {
                    Icon(Icons.Outlined.Undo, contentDescription = null, modifier = Modifier.size(16.dp))
                    Spacer(Modifier.width(6.dp))
                    Text(if (ch.mechanical) "撤销（需要确认）" else "撤销这项变更")
                }
            }
        }
    }
}
