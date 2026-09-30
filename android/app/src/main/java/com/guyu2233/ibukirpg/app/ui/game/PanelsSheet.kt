package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyListScope
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Backpack
import androidx.compose.material.icons.outlined.Groups
import androidx.compose.material.icons.outlined.History
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material.icons.outlined.Place
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.PrimaryTabRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Tab
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.guyu2233.ibukirpg.app.R
import com.guyu2233.ibukirpg.app.data.CharacterV1
import com.guyu2233.ibukirpg.app.data.InventoryV1
import com.guyu2233.ibukirpg.app.data.JournalEntryV1
import com.guyu2233.ibukirpg.app.data.NPCV1
import com.guyu2233.ibukirpg.app.data.QuickActionV1
import com.guyu2233.ibukirpg.app.data.StatV1

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PanelsSheet(
    panels: PanelsState,
    busy: Boolean,
    onDismiss: () -> Unit,
    onAction: (QuickActionV1, String) -> Unit,
) {
    val sheet = rememberModalBottomSheetState(skipPartiallyExpanded = true)
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = sheet) {
        PanelsContent(panels, busy, onAction, Modifier.fillMaxHeight(0.88f))
    }
}

/** 角色 / 背包 / 人物 / 日志 四个标签页的内容。 */
@Composable
fun PanelsContent(
    panels: PanelsState,
    busy: Boolean,
    onAction: (QuickActionV1, String) -> Unit,
    modifier: Modifier = Modifier,
    initialTab: Int = 0,
) {
    var tab by rememberSaveable { mutableIntStateOf(initialTab) }
    val tabs = listOf(
        Triple(R.string.tab_character, Icons.Outlined.Person, 0),
        Triple(R.string.tab_inventory, Icons.Outlined.Backpack, 1),
        Triple(R.string.tab_people, Icons.Outlined.Groups, 2),
        Triple(R.string.tab_journal, Icons.Outlined.History, 3),
    )
    Column(modifier) {
        PrimaryTabRow(selectedTabIndex = tab) {
            tabs.forEach { (label, icon, i) ->
                Tab(
                    selected = tab == i, onClick = { tab = i },
                    text = { Text(stringResource(label)) },
                    icon = { Icon(icon, contentDescription = null) },
                )
            }
        }
        if (panels.loading && panels.character == null) {
            Box(Modifier.fillMaxWidth().padding(48.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
            return@Column
        }
        LazyColumn(
            contentPadding = PaddingValues(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
            modifier = Modifier.fillMaxWidth(),
        ) {
            when (tab) {
                0 -> panels.character?.let { characterTab(it) }
                1 -> panels.inventory?.let { inventoryTab(it, busy, onAction) }
                2 -> peopleTab(panels.npcs, busy, onAction)
                else -> journalTab(panels.journal)
            }
        }
    }
}

private fun LazyListScope.sectionTitle(text: @Composable () -> String) {
    item { Text(text(), style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(top = 4.dp)) }
}

private fun LazyListScope.characterTab(c: CharacterV1) {
    item {
        Column {
            Text(c.name, style = MaterialTheme.typography.headlineSmall)
            Text("${c.role} · ${stringResource(R.string.game_gold, c.gold)}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
    sectionTitle { stringResource(R.string.panel_attributes) }
    item { StatGrid(c.attributes, showValue = true) }
    sectionTitle { stringResource(R.string.panel_skills) }
    item { StatGrid(c.skills, showValue = false) }
    sectionTitle { stringResource(R.string.panel_conditions) }
    if (c.conditions.isEmpty()) {
        item { Text(stringResource(R.string.panel_no_conditions), color = MaterialTheme.colorScheme.onSurfaceVariant) }
    } else {
        items(c.conditions) { st ->
            ListItem(headlineContent = { Text(st.name) }, supportingContent = { st.note?.let { Text(it) } })
        }
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun StatGrid(stats: List<StatV1>, showValue: Boolean) {
    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        stats.forEach { st ->
            val mod = if (st.modifier >= 0) "+${st.modifier}" else "${st.modifier}"
            Surface(
                color = MaterialTheme.colorScheme.surfaceContainerHigh,
                shape = MaterialTheme.shapes.medium,
                modifier = Modifier.width(100.dp).semantics(mergeDescendants = true) {},
            ) {
                Column(Modifier.padding(10.dp)) {
                    Text(st.name, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Row(verticalAlignment = Alignment.Bottom) {
                        Text(if (showValue) "${st.value}" else mod, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
                        if (showValue) {
                            Spacer(Modifier.width(4.dp))
                            Text("($mod)", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
        }
    }
}

private fun LazyListScope.inventoryTab(inv: InventoryV1, busy: Boolean, onAction: (QuickActionV1, String) -> Unit) {
    sectionTitle { stringResource(R.string.panel_items) }
    if (inv.items.isEmpty()) {
        item { Text(stringResource(R.string.panel_no_items), color = MaterialTheme.colorScheme.onSurfaceVariant) }
    }
    items(inv.items, key = { "i" + it.id }) { it2 ->
        Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
            ListItem(
                headlineContent = { Text(if (it2.qty > 1) "${it2.name} ×${it2.qty}" else it2.name) },
                supportingContent = { Text(it2.description) },
                trailingContent = {
                    it2.use?.let { qa ->
                        FilledTonalButton(onClick = { onAction(qa, it2.useLabel ?: it2.name) }, enabled = !busy) { Text(it2.useLabel ?: "使用") }
                    }
                },
                colors = androidx.compose.material3.ListItemDefaults.colors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh),
            )
        }
    }
    if (inv.shop.isNotEmpty()) {
        sectionTitle { stringResource(R.string.panel_shop, inv.shopSeller ?: "") }
        items(inv.shop, key = { "s" + it.id }) { it2 ->
            ListItem(
                headlineContent = { Text(it2.name) },
                supportingContent = { Text(it2.description) },
                trailingContent = {
                    it2.buy?.let { qa ->
                        OutlinedButton(onClick = { onAction(qa, "买${it2.name}") }, enabled = !busy && it2.affordable) {
                            Text(stringResource(R.string.panel_buy, it2.price))
                        }
                    }
                },
            )
        }
    }
    item { Text(stringResource(R.string.game_gold, inv.gold), style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant) }
}

@OptIn(ExperimentalLayoutApi::class)
private fun LazyListScope.peopleTab(npcs: List<NPCV1>, busy: Boolean, onAction: (QuickActionV1, String) -> Unit) {
    items(npcs, key = { it.id }) { n ->
        Card(colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainerHigh)) {
            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text(n.name, style = MaterialTheme.typography.titleMedium)
                        Text(n.role, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Surface(
                        color = if (n.present) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceContainerHighest,
                        shape = MaterialTheme.shapes.small,
                    ) {
                        Row(Modifier.padding(horizontal = 8.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                            Icon(Icons.Outlined.Place, contentDescription = null, modifier = Modifier.size(14.dp))
                            Spacer(Modifier.width(2.dp))
                            Text(if (n.present) stringResource(R.string.panel_present) else n.locationName, style = MaterialTheme.typography.labelMedium)
                        }
                    }
                }
                Text("态度：${n.attitude}", style = MaterialTheme.typography.bodyMedium)
                Meter(stringResource(R.string.panel_trust), n.trust, MaterialTheme.colorScheme.primary)
                Meter(stringResource(R.string.panel_fear), n.fear, MaterialTheme.colorScheme.error)
                Text(
                    if (n.talks > 0) stringResource(R.string.panel_memories_count, n.talks) else stringResource(R.string.panel_memories),
                    style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 4.dp),
                )
                if (n.memories.isEmpty()) {
                    Text(stringResource(R.string.panel_no_memories), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                } else {
                    n.memories.take(4).forEach { m ->
                        Text("· $m", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
                Text(stringResource(R.string.panel_beliefs), style = MaterialTheme.typography.labelLarge, modifier = Modifier.padding(top = 4.dp))
                if (n.beliefs.isEmpty()) {
                    Text(stringResource(R.string.panel_no_beliefs), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                } else {
                    n.beliefs.takeLast(5).forEach { b ->
                        Text(
                            "· ${b.text}（${b.source} · ${b.`when`}）",
                            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                if (n.present && n.actions.isNotEmpty()) {
                    FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(top = 4.dp)) {
                        n.actions.forEach { a ->
                            OutlinedButton(onClick = { onAction(a.action, a.label) }, enabled = !busy) {
                                Text(if (a.chance >= 0) "${a.label} · ${a.chance}%" else a.label)
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun Meter(label: String, value: Int, color: androidx.compose.ui.graphics.Color) {
    val v = value.coerceIn(-100, 100)
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.semantics(mergeDescendants = true) { contentDescription = "$label $value" }) {
        Text(label, style = MaterialTheme.typography.labelMedium, modifier = Modifier.width(40.dp))
        LinearProgressIndicator(
            progress = { (kotlin.math.abs(v) / 100f).coerceAtLeast(0.02f) },
            color = if (v < 0) MaterialTheme.colorScheme.error else color,
            modifier = Modifier.weight(1f).height(6.dp),
        )
        Text("$value", style = MaterialTheme.typography.labelMedium, modifier = Modifier.width(36.dp).padding(start = 8.dp))
    }
}

private fun LazyListScope.journalTab(journal: List<JournalEntryV1>) {
    if (journal.isEmpty()) {
        item { Text(stringResource(R.string.panel_journal_empty), color = MaterialTheme.colorScheme.onSurfaceVariant) }
        return
    }
    items(journal.asReversed(), key = { it.seq }) { j ->
        Column {
            Text(j.time, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.outline)
            Text(j.text, style = MaterialTheme.typography.bodyMedium)
            HorizontalDivider(Modifier.padding(top = 8.dp), color = MaterialTheme.colorScheme.outlineVariant)
        }
    }
}
