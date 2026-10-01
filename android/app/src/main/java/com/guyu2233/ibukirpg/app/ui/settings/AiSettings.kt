package com.guyu2233.ibukirpg.app.ui.settings

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.AutoStories
import androidx.compose.material.icons.outlined.Cancel
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.Cloud
import androidx.compose.material.icons.outlined.Compress
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.EditNote
import androidx.compose.material.icons.outlined.Error
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Psychology
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material.icons.outlined.RateReview
import androidx.compose.material.icons.outlined.SmartToy
import androidx.compose.material.icons.outlined.Smartphone
import androidx.compose.material.icons.outlined.Style
import androidx.compose.material.icons.outlined.TipsAndUpdates
import androidx.compose.material.icons.outlined.Tune
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledTonalIconButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import com.guyu2233.ibukirpg.app.data.GenSettingsV1
import com.guyu2233.ibukirpg.app.data.ProviderV1
import com.guyu2233.ibukirpg.app.data.RouteV1
import com.guyu2233.ibukirpg.app.data.TaskInfoV1
import com.guyu2233.ibukirpg.app.data.providerLabel

/** API 设置 + 生成设置的状态（多服务商）。 */
data class AiSettingsState(
    val providers: List<ProviderV1> = emptyList(),
    /** 已保存密钥的服务商 id（密钥本身从不回显）。 */
    val keys: Set<String> = emptySet(),
    val gen: GenSettingsV1 = GenSettingsV1(),
    val tasks: List<TaskInfoV1> = emptyList(),
)

data class AiActions(
    val onSaveProvider: (ProviderV1, String?) -> Unit = { _, _ -> },
    val onDeleteProvider: (String) -> Unit = {},
    val onGen: (GenSettingsV1) -> Unit = {},
    val onOpenGeneration: () -> Unit = {},
    val onOpenSensitivity: () -> Unit = {},
)

private fun taskIcon(id: String): ImageVector = when (id) {
    "narrate_world" -> Icons.Outlined.AutoStories
    "parse_action" -> Icons.Outlined.Psychology
    "combat_adjudicate" -> Icons.Outlined.Style
    "memory" -> Icons.Outlined.Compress
    "world_sim" -> Icons.Outlined.Public
    "audit" -> Icons.Outlined.RateReview
    "card_gen" -> Icons.Outlined.EditNote
    "char_review" -> Icons.Outlined.SmartToy
    else -> Icons.Outlined.Tune
}

/** 设置页里的“API 设置”分区：已配置的服务商（密钥在 Keystore 加密保存）+ 生成设置 / 提示灵敏度入口。 */
@Composable
fun ApiProvidersSection(state: AiSettingsState, actions: AiActions) {
    var editing by remember { mutableStateOf<ProviderV1?>(null) }
    Surface(shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainer) {
        Column {
            if (state.providers.isEmpty()) {
                Text(
                    "还没有配置服务商：在上面选择 DeepSeek / 通义千问 / 自定义并保存，或导入本地模型。",
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(16.dp),
                )
            }
            state.providers.forEach { p ->
                ListItem(
                    leadingContent = { Icon(if (p.kind == "llamacpp") Icons.Outlined.Smartphone else Icons.Outlined.Cloud, contentDescription = null) },
                    headlineContent = { Text(p.label.ifBlank { providerLabel(p.kind) }) },
                    supportingContent = {
                        Text(
                            buildString {
                                append(p.model.ifBlank { "未设置模型" })
                                if (p.kind != "llamacpp") append(if (p.id in state.keys) " · 密钥已加密保存" else " · 未设置密钥")
                            },
                        )
                    },
                    trailingContent = {
                        Row {
                            if (p.kind != "llamacpp") IconButton(onClick = { editing = p }) { Icon(Icons.Outlined.Key, contentDescription = "修改") }
                            IconButton(onClick = { actions.onDeleteProvider(p.id) }) { Icon(Icons.Outlined.Delete, contentDescription = "删除") }
                        }
                    },
                    colors = ListItemDefaults.colors(containerColor = Color.Transparent),
                )
            }
            HorizontalDivider()
            NavRow("生成设置", if (state.gen.mode == "per_task") "按任务分配模型" else "统一模型", actions.onOpenGeneration)
            NavRow("提示灵敏度", "偏离设定 / 重要角色死亡 / 重大剧情影响", actions.onOpenSensitivity)
        }
    }
    editing?.let { p ->
        ProviderDialog(p, hasKey = p.id in state.keys, onDismiss = { editing = null }, onSave = { pv, key -> editing = null; actions.onSaveProvider(pv, key) })
    }
}

@Composable
private fun NavRow(title: String, sub: String, onClick: () -> Unit) {
    ListItem(
        headlineContent = { Text(title) },
        supportingContent = { Text(sub) },
        trailingContent = { Icon(Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null) },
        colors = ListItemDefaults.colors(containerColor = Color.Transparent),
        modifier = Modifier.fillMaxWidth().clickable(onClick = onClick),
    )
}

@Composable
private fun ProviderDialog(p: ProviderV1, hasKey: Boolean, onDismiss: () -> Unit, onSave: (ProviderV1, String?) -> Unit) {
    var base by rememberSaveable { mutableStateOf(p.baseUrl) }
    var model by rememberSaveable { mutableStateOf(p.model) }
    var key by rememberSaveable { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(p.label.ifBlank { providerLabel(p.kind) }) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (p.kind == "custom") OutlinedTextField(base, { base = it }, label = { Text("Base URL") }, singleLine = true)
                OutlinedTextField(model, { model = it }, label = { Text("模型") }, singleLine = true)
                OutlinedTextField(
                    key, { key = it }, label = { Text(if (hasKey) "新的 API Key（留空则不变）" else "API Key") },
                    singleLine = true, visualTransformation = PasswordVisualTransformation(),
                )
                Text("密钥用 Android Keystore 加密保存，不会写进存档或导出文件。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        },
        confirmButton = { TextButton(onClick = { onSave(p.copy(baseUrl = base.trim(), model = model.trim()), key.trim().ifEmpty { null }) }) { Text("保存") } },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}

/** 全屏“生成设置”（第 13 节 / 设计稿 06）。 */
@Composable
fun GenerationSettingsDialog(state: AiSettingsState, onGen: (GenSettingsV1) -> Unit, onClose: () -> Unit, onImportLocal: () -> Unit = {}) {
    Dialog(onDismissRequest = onClose, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(Modifier.fillMaxSize(), color = MaterialTheme.colorScheme.background) {
            GenerationSettingsContent(state, onGen, onClose, onImportLocal)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GenerationSettingsContent(
    state: AiSettingsState,
    onGen: (GenSettingsV1) -> Unit,
    onClose: () -> Unit,
    onImportLocal: () -> Unit = {},
    initialFitSheet: Boolean = false,
    /** 截图测试：提示面板直接渲染在页面底部，不弹出窗口。 */
    inlineFitSheet: Boolean = false,
) {
    var fit by rememberSaveable { mutableStateOf(initialFitSheet) }
    val g = state.gen
    val c = MaterialTheme.colorScheme
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("生成设置") },
                navigationIcon = { IconButton(onClick = onClose) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = "返回") } },
                actions = {
                    FilledTonalIconButton(onClick = { fit = true }) { Icon(Icons.Outlined.TipsAndUpdates, contentDescription = "哪些任务适合本地模型") }
                },
            )
        },
    ) { pad ->
        Box(Modifier.padding(pad).fillMaxSize()) {
            Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                val modes = listOf("unified" to "统一模型", "per_task" to "按任务分配")
                SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                    modes.forEachIndexed { i, (id, label) ->
                        SegmentedButton(selected = g.mode == id, onClick = { onGen(g.copy(mode = id)) }, shape = SegmentedButtonDefaults.itemShape(i, modes.size)) { Text(label) }
                    }
                }
                if (g.mode == "unified") {
                    Text("所有任务使用同一个模型", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 8.dp))
                    RoutePicker("模型", g.unified, state.providers, allowDefault = false) { onGen(g.copy(unified = it)) }
                } else {
                    Text("每个任务使用的模型", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 8.dp))
                    state.tasks.forEach { t ->
                        val r = g.tasks[t.id] ?: RouteV1()
                        TaskRow(t, r, state.providers) { nr -> onGen(g.copy(tasks = if (nr.provider.isBlank()) g.tasks - t.id else g.tasks + (t.id to nr))) }
                    }
                }
                HorizontalDivider(Modifier.padding(vertical = 8.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("显示每回合 token 用量", style = MaterialTheme.typography.bodyLarge)
                        Text("叙事流里每回合末尾显示 ↑输入 ↓输出 与调用次数", style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant)
                    }
                    Switch(checked = g.showTokenUsage, onCheckedChange = { onGen(g.copy(showTokenUsage = it)) })
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("一致性审查", style = MaterialTheme.typography.bodyLarge)
                        Text(
                            when { g.auditEveryTurns < 0 -> "关闭"; g.auditEveryTurns == 0 -> "使用故事包默认间隔"; else -> "每 ${g.auditEveryTurns} 回合" },
                            style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant,
                        )
                    }
                    Switch(checked = g.auditEveryTurns >= 0, onCheckedChange = { onGen(g.copy(auditEveryTurns = if (it) 0 else -1)) })
                }
                Spacer(Modifier.size(if (inlineFitSheet && fit) 420.dp else 24.dp))
            }
            if (fit && inlineFitSheet) {
                Box(Modifier.fillMaxSize().padding(0.dp)) {
                    Surface(Modifier.fillMaxSize(), color = Color.Black.copy(alpha = 0.32f)) {}
                    Surface(
                        shape = RoundedCornerShape(topStart = 28.dp, topEnd = 28.dp), color = c.surfaceContainerLow,
                        modifier = Modifier.align(Alignment.BottomCenter).fillMaxWidth(),
                    ) { LocalFitContent(state.tasks, onImportLocal, onClose = { fit = false }) }
                }
            }
        }
    }
    if (fit && !inlineFitSheet) {
        val sheet = rememberModalBottomSheetState(skipPartiallyExpanded = true)
        ModalBottomSheet(onDismissRequest = { fit = false }, sheetState = sheet) {
            LocalFitContent(state.tasks, onImportLocal, onClose = { fit = false })
        }
    }
}

@Composable
private fun TaskRow(t: TaskInfoV1, r: RouteV1, providers: List<ProviderV1>, onChange: (RouteV1) -> Unit) {
    var open by remember { mutableStateOf(false) }
    val p = providers.firstOrNull { it.id == r.provider }
    Box {
        ListItem(
            leadingContent = { Icon(taskIcon(t.id), contentDescription = null) },
            headlineContent = {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text(t.name)
                    if (t.id == "narrate_world") {
                        Spacer(Modifier.width(8.dp))
                        Surface(color = MaterialTheme.colorScheme.tertiaryContainer, shape = RoundedCornerShape(50)) {
                            Text("合并输出", style = MaterialTheme.typography.labelSmall, modifier = Modifier.padding(horizontal = 8.dp, vertical = 2.dp))
                        }
                    }
                }
            },
            supportingContent = {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (p?.kind == "llamacpp") {
                        Icon(Icons.Outlined.Smartphone, contentDescription = null, modifier = Modifier.size(14.dp))
                        Spacer(Modifier.width(4.dp))
                    }
                    Text(
                        when {
                            p == null -> "跟随“叙事 + 世界更新”"
                            else -> "${p.label.ifBlank { providerLabel(p.kind) }} · ${r.model.ifBlank { p.model }}"
                        } + if (p?.kind == "llamacpp" && t.localFit == "not_recommended") "  ⚠ 不推荐本地模型" else "",
                    )
                }
            },
            trailingContent = { Icon(Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null) },
            colors = ListItemDefaults.colors(containerColor = Color.Transparent),
            modifier = Modifier.clickable { open = true },
        )
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            if (t.id != "narrate_world") DropdownMenuItem(text = { Text("跟随“叙事 + 世界更新”") }, onClick = { open = false; onChange(RouteV1()) })
            providers.forEach { pv ->
                DropdownMenuItem(
                    text = { Text("${pv.label.ifBlank { providerLabel(pv.kind) }} · ${pv.model}") },
                    leadingIcon = { Icon(if (pv.kind == "llamacpp") Icons.Outlined.Smartphone else Icons.Outlined.Cloud, contentDescription = null) },
                    onClick = { open = false; onChange(RouteV1(provider = pv.id)) },
                )
            }
        }
    }
}

@Composable
private fun RoutePicker(label: String, r: RouteV1, providers: List<ProviderV1>, allowDefault: Boolean, onChange: (RouteV1) -> Unit) {
    var open by remember { mutableStateOf(false) }
    val p = providers.firstOrNull { it.id == r.provider }
    Box {
        ListItem(
            headlineContent = { Text(label) },
            supportingContent = { Text(p?.let { "${it.label.ifBlank { providerLabel(it.kind) }} · ${r.model.ifBlank { it.model }}" } ?: "离线（规则模板）") },
            trailingContent = { Icon(Icons.AutoMirrored.Outlined.KeyboardArrowRight, contentDescription = null) },
            colors = ListItemDefaults.colors(containerColor = Color.Transparent),
            modifier = Modifier.clickable { open = true },
        )
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            DropdownMenuItem(text = { Text("离线（规则模板）") }, onClick = { open = false; onChange(RouteV1(provider = if (allowDefault) "" else "offline")) })
            providers.forEach { pv ->
                DropdownMenuItem(text = { Text("${pv.label.ifBlank { providerLabel(pv.kind) }} · ${pv.model}") }, onClick = { open = false; onChange(RouteV1(provider = pv.id)) })
            }
        }
    }
}

/** “哪些任务适合本地模型”提示面板（内容来自引擎 get_tasks 的 local_fit）。 */
@Composable
fun LocalFitContent(tasks: List<TaskInfoV1>, onImportLocal: () -> Unit, onClose: () -> Unit) {
    val c = MaterialTheme.colorScheme
    Column(Modifier.fillMaxWidth().padding(horizontal = 24.dp).padding(top = 16.dp, bottom = 16.dp).navigationBarsPadding(), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.TipsAndUpdates, contentDescription = null, tint = c.primary)
            Spacer(Modifier.width(10.dp))
            Text("哪些任务适合本地模型", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
        }
        Text("本地 1.5B–3B 模型速度快、免费、离线，但长 JSON 与复杂推理不稳定。", style = MaterialTheme.typography.bodyMedium, color = c.onSurfaceVariant)
        FitGroup(Icons.Outlined.CheckCircle, Color(0xFF2E7D32), "适合", tasks.filter { it.localFit == "good" })
        FitGroup(Icons.Outlined.Error, Color(0xFFB26A00), "可以，但会变保守", tasks.filter { it.localFit == "caution" })
        FitGroup(Icons.Outlined.Cancel, c.error, "不推荐", tasks.filter { it.localFit == "not_recommended" })
        Surface(shape = RoundedCornerShape(12.dp), border = BorderStroke(1.dp, c.outlineVariant), color = Color.Transparent) {
            Row(Modifier.padding(12.dp), verticalAlignment = Alignment.Top) {
                Icon(Icons.Outlined.Info, contentDescription = null, modifier = Modifier.size(18.dp), tint = c.onSurfaceVariant)
                Spacer(Modifier.width(8.dp))
                Text(
                    "本地模型也能修改世界，但受限：每回合最多 3 项、只能改动次要设定，不能新建或删除重要人物 / 势力。",
                    style = MaterialTheme.typography.bodySmall, color = c.onSurfaceVariant,
                )
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically) {
            TextButton(onClick = onImportLocal) { Text("在 API 设置中导入本地模型") }
            Spacer(Modifier.weight(1f))
            Button(onClick = onClose) { Text("知道了") }
        }
    }
}

@Composable
private fun FitGroup(icon: ImageVector, tint: Color, title: String, tasks: List<TaskInfoV1>) {
    if (tasks.isEmpty()) return
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(6.dp))
            Text(title, style = MaterialTheme.typography.titleSmall, color = tint)
        }
        Text(
            tasks.joinToString(" · ") { t -> t.name + (t.localWhy.takeIf { it.isNotBlank() && t.localFit == "caution" }?.let { "（$it）" } ?: "") },
            style = MaterialTheme.typography.bodyMedium,
        )
    }
}
