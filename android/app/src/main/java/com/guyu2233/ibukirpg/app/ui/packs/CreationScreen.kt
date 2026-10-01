package com.guyu2233.ibukirpg.app.ui.packs

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.selection.selectable
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Remove
import androidx.compose.material.icons.outlined.ReportProblem
import androidx.compose.material.icons.outlined.RateReview
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.guyu2233.ibukirpg.app.data.CreationBackgroundV1
import com.guyu2233.ibukirpg.app.data.CreationOptionsV1
import com.guyu2233.ibukirpg.app.data.CreationPresetV1
import com.guyu2233.ibukirpg.app.data.CreationReviewV1
import com.guyu2233.ibukirpg.app.data.CreationV1

/** 全屏角色创建（预设主角 / 自建角色 + 审查）。 */
@Composable
fun CreationDialog(
    state: CreationState,
    working: Boolean,
    defaultName: String,
    onDismiss: () -> Unit,
    onReview: (CreationV1) -> Unit,
    onClearReview: () -> Unit,
    onStart: (String, CreationV1?) -> Unit,
) {
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false, decorFitsSystemWindows = false)) {
        CreationContent(state, working, defaultName, onDismiss, onReview, onClearReview, onStart)
    }
}

/** 无状态内容（便于截图测试）；[initial] 用于截图时预填自建角色。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CreationContent(
    state: CreationState,
    working: Boolean,
    defaultName: String,
    onDismiss: () -> Unit,
    onReview: (CreationV1) -> Unit,
    onClearReview: () -> Unit,
    onStart: (String, CreationV1?) -> Unit,
    initial: CreationV1? = null,
) {
    val o = state.options
    var custom by rememberSaveable { mutableStateOf(initial?.custom ?: false) }
    var presetId by rememberSaveable { mutableStateOf(initial?.preset ?: "") }
    var name by rememberSaveable { mutableStateOf(initial?.name ?: "") }
    var background by rememberSaveable { mutableStateOf(initial?.background ?: "") }
    var appearance by rememberSaveable { mutableStateOf(initial?.appearance ?: "") }
    var personality by rememberSaveable { mutableStateOf(initial?.personality ?: "") }
    var story by rememberSaveable { mutableStateOf(initial?.story ?: "") }
    val alloc = remember { mutableStateMapOf<String, Int>().apply { initial?.attributes?.let { putAll(it) } } }

    val preset = o?.presets?.firstOrNull { it.id == presetId } ?: o?.presets?.firstOrNull()
    val bg = o?.backgrounds?.firstOrNull { it.id == background }
    val spent = alloc.values.sum()
    val remaining = (o?.attributePoints ?: 0) - spent
    val effectiveName = name.ifBlank { if (custom) defaultName else preset?.name ?: defaultName }

    fun current(): CreationV1? = when {
        o == null || o.simple -> null
        custom -> CreationV1(
            name = effectiveName, custom = true, background = background, appearance = appearance.trim(),
            personality = personality.trim(), story = story.trim(), attributes = alloc.filterValues { it != 0 },
        )
        else -> CreationV1(name = effectiveName, preset = preset?.id ?: "")
    }

    fun adopt(r: CreationV1) {
        background = r.background; appearance = r.appearance; personality = r.personality; story = r.story
        if (r.name.isNotBlank()) name = r.name
        alloc.clear(); alloc.putAll(r.attributes)
        onClearReview()
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("创建角色")
                        Text(state.pack.name, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                navigationIcon = { IconButton(onClick = onDismiss) { Icon(Icons.Outlined.Close, contentDescription = "关闭") } },
            )
        },
        bottomBar = {
            Surface(tonalElevation = 3.dp) {
                Row(
                    Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 12.dp).imePadding(),
                    horizontalArrangement = Arrangement.spacedBy(12.dp), verticalAlignment = Alignment.CenterVertically,
                ) {
                    if (custom && o != null && !o.simple) {
                        OutlinedButton(onClick = { current()?.let(onReview) }, enabled = !state.reviewing && !working) {
                            if (state.reviewing) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                            else Icon(Icons.Outlined.RateReview, contentDescription = null, modifier = Modifier.size(18.dp))
                            Spacer(Modifier.width(8.dp)); Text("审查角色")
                        }
                    }
                    Spacer(Modifier.weight(1f))
                    Button(onClick = { onStart(effectiveName, current()) }, enabled = !working && o != null && remaining >= 0) {
                        if (working) { CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp); Spacer(Modifier.width(8.dp)) }
                        Text("开始冒险")
                    }
                }
            }
        },
    ) { pad ->
        Box(Modifier.fillMaxSize().padding(pad), contentAlignment = Alignment.TopCenter) {
            if (o == null) {
                if (state.loading) CircularProgressIndicator(Modifier.align(Alignment.Center))
                else Text(state.error ?: "无法读取角色创建选项", color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(24.dp))
                return@Box
            }
            LazyColumn(
                Modifier.widthIn(max = 640.dp).fillMaxSize(),
                contentPadding = PaddingValues(16.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
            ) {
                state.error?.let { e -> item("err") { ErrorBanner(e) } }
                if (!o.simple) {
                    item("mode") {
                        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                            SegmentedButton(!custom, { custom = false; onClearReview() }, SegmentedButtonDefaults.itemShape(0, 2)) { Text("预设角色") }
                            SegmentedButton(custom, { custom = true; onClearReview() }, SegmentedButtonDefaults.itemShape(1, 2)) { Text("自己创建") }
                        }
                    }
                }
                item("name") {
                    OutlinedTextField(
                        value = name, onValueChange = { if (it.length <= 12) name = it },
                        label = { Text("名字") }, placeholder = { Text(if (custom) defaultName else preset?.name ?: defaultName) },
                        singleLine = true, modifier = Modifier.fillMaxWidth(),
                    )
                }
                if (!custom) {
                    o.presets.forEach { p ->
                        item("preset-${p.id}") { PresetCard(p, o, selected = p.id == preset?.id) { presetId = p.id } }
                    }
                } else {
                    state.review?.let { r -> item("review") { ReviewCard(r, o, onAdopt = { r.recommended?.let(::adopt) }) } }
                    if (o.backgrounds.isNotEmpty()) {
                        item("bg") { BackgroundPicker(o.backgrounds, background) { background = if (background == it) "" else it; onClearReview() } }
                    }
                    if (o.attributePoints > 0) {
                        item("attrs") {
                            AttributeAllocator(o, bg, alloc, remaining) { k, d ->
                                val v = (alloc[k] ?: 0) + d
                                if (v <= 0) alloc.remove(k) else alloc[k] = v
                                onClearReview()
                            }
                        }
                    }
                    item("text") {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            OutlinedTextField(appearance, { appearance = it.take(120) }, label = { Text("外貌") }, modifier = Modifier.fillMaxWidth())
                            OutlinedTextField(personality, { personality = it.take(120) }, label = { Text("性格") }, modifier = Modifier.fillMaxWidth())
                            OutlinedTextField(story, { story = it.take(400) }, label = { Text("来历") }, minLines = 3, modifier = Modifier.fillMaxWidth())
                        }
                    }
                    if (o.rules.isNotEmpty()) item("rules") { RulesCard(o.rules) }
                }
            }
        }
    }
}

@Composable
private fun ErrorBanner(text: String) {
    Surface(color = MaterialTheme.colorScheme.errorContainer, shape = MaterialTheme.shapes.medium) {
        Text(text, color = MaterialTheme.colorScheme.onErrorContainer, modifier = Modifier.fillMaxWidth().padding(12.dp))
    }
}

private fun attrLine(o: CreationOptionsV1, attrs: Map<String, Int>): String =
    attrs.entries.sortedBy { it.key }.joinToString(" · ") { "${o.attributeNames[it.key] ?: it.key} ${it.value}" }

@Composable
private fun PresetCard(p: CreationPresetV1, o: CreationOptionsV1, selected: Boolean, onSelect: () -> Unit) {
    val c = MaterialTheme.colorScheme
    OutlinedCard(
        modifier = Modifier.fillMaxWidth().selectable(selected, onClick = onSelect, role = Role.RadioButton),
        border = BorderStroke(if (selected) 2.dp else 1.dp, if (selected) c.primary else c.outlineVariant),
        colors = CardDefaults.outlinedCardColors(containerColor = if (selected) c.secondaryContainer.copy(alpha = 0.35f) else c.surface),
    ) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.Top) {
            RadioButton(selected, onClick = null)
            Spacer(Modifier.width(12.dp))
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(p.name, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                if (p.description.isNotBlank()) Text(p.description, style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant)
                if (p.attributes.isNotEmpty()) Text(attrLine(o, p.attributes), style = MaterialTheme.typography.labelMedium, color = c.primary)
            }
        }
    }
}

@Composable
private fun BackgroundPicker(list: List<CreationBackgroundV1>, selected: String, onPick: (String) -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("出身", style = MaterialTheme.typography.titleSmall)
        FlowRow(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            list.forEach { b -> FilterChip(selected = b.id == selected, onClick = { onPick(b.id) }, label = { Text(b.name) }) }
        }
        list.firstOrNull { it.id == selected }?.let { b ->
            Text(
                buildString {
                    append(b.description)
                    if (b.gold > 0) append("  起始 ${b.gold} 枚")
                },
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

@Composable
private fun AttributeAllocator(o: CreationOptionsV1, bg: CreationBackgroundV1?, alloc: Map<String, Int>, remaining: Int, onDelta: (String, Int) -> Unit) {
    val c = MaterialTheme.colorScheme
    Card(colors = CardDefaults.cardColors(containerColor = c.surfaceContainerLow)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text("属性", style = MaterialTheme.typography.titleSmall, modifier = Modifier.weight(1f))
                Text(
                    "剩余点数 $remaining / ${o.attributePoints}", style = MaterialTheme.typography.labelLarge,
                    color = if (remaining < 0) c.error else c.primary,
                )
            }
            o.attributeNames.keys.sorted().forEach { k ->
                val base = (o.baseAttributes[k] ?: 0) + (bg?.attributes?.get(k) ?: 0)
                val add = alloc[k] ?: 0
                val total = base + add
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(o.attributeNames[k] ?: k, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
                    FilledTonalIconButton(onClick = { onDelta(k, -1) }, enabled = add > 0, modifier = Modifier.size(36.dp)) {
                        Icon(Icons.Outlined.Remove, contentDescription = "降低${o.attributeNames[k] ?: k}")
                    }
                    Text(
                        if (add > 0) "$total (+$add)" else "$total", style = MaterialTheme.typography.titleMedium,
                        color = if (o.attrMax in 1 until total) c.error else c.onSurface,
                        modifier = Modifier.width(72.dp).padding(horizontal = 8.dp),
                    )
                    FilledTonalIconButton(
                        onClick = { onDelta(k, 1) }, enabled = remaining > 0 && (o.attrMax == 0 || total < o.attrMax),
                        modifier = Modifier.size(36.dp),
                    ) { Icon(Icons.Outlined.Add, contentDescription = "提高${o.attributeNames[k] ?: k}") }
                }
            }
        }
    }
}

private fun loreFitLabel(f: String) = when (f) {
    "ok", "good" -> "契合"
    "warn", "minor" -> "略有出入"
    "bad" -> "不符合世界观"
    else -> f
}

@Composable
private fun ReviewCard(r: CreationReviewV1, o: CreationOptionsV1, onAdopt: () -> Unit) {
    val c = MaterialTheme.colorScheme
    Card(colors = CardDefaults.cardColors(containerColor = if (r.ok) c.secondaryContainer else c.surfaceContainerHigh)) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(
                    if (r.ok) Icons.Outlined.CheckCircle else Icons.Outlined.ReportProblem, contentDescription = null,
                    tint = if (r.ok) c.primary else c.error,
                )
                Spacer(Modifier.width(8.dp))
                Text(if (r.ok) "审查通过" else "需要调整", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
                Text(if (r.source == "ai") "规则 + 审查 Agent" else "规则检查", style = MaterialTheme.typography.labelMedium, color = c.onSurfaceVariant)
            }
            if (r.loreFit.isNotBlank()) Text("设定契合：${loreFitLabel(r.loreFit)}", style = MaterialTheme.typography.bodyMedium)
            if (r.maxPower > 0) {
                Text("强度 ${r.power} / ${r.maxPower}", style = MaterialTheme.typography.bodyMedium, color = if (r.power > r.maxPower) c.error else c.onSurface)
                LinearProgressIndicator(
                    progress = { (r.power.toFloat() / r.maxPower).coerceIn(0f, 1f) }, modifier = Modifier.fillMaxWidth(),
                    color = if (r.power > r.maxPower) c.error else c.primary,
                )
            }
            Bullets("问题", r.problems, c.error)
            Bullets("冲突", r.conflicts, c.error)
            Bullets("建议", r.suggestions, c.onSurface)
            r.recommended?.let { rec ->
                Spacer(Modifier.height(4.dp))
                Text("推荐角色卡", style = MaterialTheme.typography.labelLarge)
                Text(
                    buildString {
                        append(rec.name.ifBlank { "（同名）" })
                        o.backgrounds.firstOrNull { it.id == rec.background }?.let { append(" · ").append(it.name) }
                        if (rec.attributes.isNotEmpty()) append(" · ").append(rec.attributes.entries.sortedBy { it.key }.joinToString("、") { "${o.attributeNames[it.key] ?: it.key}+${it.value}" })
                        if (rec.story.isNotBlank()) append("\n").append(rec.story)
                    },
                    style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant,
                )
                TextButton(onClick = onAdopt) { Text("采用推荐") }
            }
        }
    }
}

@Composable
private fun Bullets(title: String, list: List<String>, color: androidx.compose.ui.graphics.Color) {
    if (list.isEmpty()) return
    Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(title, style = MaterialTheme.typography.labelLarge)
        list.forEach { Text("• $it", style = MaterialTheme.typography.bodyMedium, color = color) }
    }
}

@Composable
private fun RulesCard(rules: List<String>) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text("故事包的创建规则", style = MaterialTheme.typography.titleSmall)
            rules.forEach { Text("• $it", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
    }
}
