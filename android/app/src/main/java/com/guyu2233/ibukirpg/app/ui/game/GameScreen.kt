package com.guyu2233.ibukirpg.app.ui.game

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.union
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.Send
import androidx.compose.material.icons.outlined.KeyboardDoubleArrowDown
import androidx.compose.material.icons.outlined.PersonOutline
import androidx.compose.material.icons.outlined.AccountTree
import androidx.compose.material.icons.outlined.Public
import androidx.compose.material.icons.outlined.AutoAwesome
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.TextButton
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.foundation.layout.width
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SmallFloatingActionButton
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.guyu2233.ibukirpg.app.R
import com.guyu2233.ibukirpg.app.data.PromptSettingsV1
import com.guyu2233.ibukirpg.app.data.TimelineV1
import com.guyu2233.ibukirpg.app.ui.rpg.CombatHeader
import com.guyu2233.ibukirpg.app.data.QuickActionV1
import com.guyu2233.ibukirpg.app.data.SceneV1
import com.guyu2233.ibukirpg.app.ui.rpg.CombatPanel
import com.guyu2233.ibukirpg.app.ui.rpg.LocalRpgData
import com.guyu2233.ibukirpg.app.ui.rpg.RpgData
import com.guyu2233.ibukirpg.app.ui.rpg.RpgDialogHost
import com.guyu2233.ibukirpg.app.ui.rpg.RpgDialogState
import com.guyu2233.ibukirpg.app.ui.rpg.rememberRpgDialogState
import com.guyu2233.ibukirpg.app.data.SuggestionV1
import com.guyu2233.ibukirpg.app.ui.common.iconFor
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GameScreen(vm: GameViewModel, onBack: () -> Unit) {
    val s by vm.state.collectAsStateWithLifecycle()
    val input by vm.input.collectAsStateWithLifecycle()
    val panels by vm.panels.collectAsStateWithLifecycle()
    val world by vm.world.collectAsStateWithLifecycle()
    val timeline by vm.timeline.collectAsStateWithLifecycle()
    val prompts by vm.prompts.collectAsStateWithLifecycle()
    val rpgData = remember(vm) { RpgData(portrait = vm::portrait, card = vm::card, mech = vm::mech) }
    CompositionLocalProvider(LocalRpgData provides rpgData) { GameContent(
        s = s,
        input = input,
        panels = panels,
        onBack = onBack,
        onInput = vm::onInput,
        onSend = vm::send,
        onQuick = vm::quick,
        onOpenPanels = vm::refreshPanels,
        onRetry = vm::load,
        onErrorShown = vm::consumeError,
        onNoticesShown = vm::consumeNotices,
        onLoadEarlier = vm::loadEarlier,
        world = world,
        timeline = timeline,
        prompts = prompts,
        actions = WorldActions(
            onWorldTab = { vm.loadWorldTab(it) },
            onRevert = vm::revertChange,
            onDecision = vm::resolveDecision,
            onSensitivity = vm::openPrompts,
            onSavePrompts = vm::savePrompts,
            onOpenTimeline = vm::openTimeline,
            onRollbackTurn = vm::rollbackTo,
            onCancelRollback = vm::cancelRollback,
            onWait = vm::wait,
            timeline = TimelineActions(
                onClose = vm::closeTimeline,
                onRollback = vm::rollbackTo,
                onCheckpointAt = { t -> vm.createCheckpoint("回合 $t", t) },
                onRestore = vm::restoreCheckpoint,
                onSwitch = vm::switchBranch,
                onNewCheckpoint = { vm.createCheckpoint("手动 · 回合 ${s.scene.turn}") },
            ),
        ),
    ) }
}

/** 0.2.0 开放世界相关的回调。 */
data class WorldActions(
    val onWorldTab: (String) -> Unit = {},
    val onRevert: (com.guyu2233.ibukirpg.app.data.WorldChangeV1) -> Unit = {},
    val onDecision: (accept: Boolean, notifyOnly: Boolean) -> Unit = { _, _ -> },
    val onSensitivity: () -> Unit = {},
    val onSavePrompts: (com.guyu2233.ibukirpg.app.data.PromptSettingsV1?) -> Unit = {},
    val onOpenTimeline: () -> Unit = {},
    val onRollbackTurn: (Int) -> Unit = {},
    val onCancelRollback: () -> Unit = {},
    val onWait: (target: String, label: String) -> Unit = { _, _ -> },
    val timeline: TimelineActions = TimelineActions(),
)

/** 无状态的游戏界面（便于截图测试与预览）。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun GameContent(
    s: GameState,
    input: String,
    panels: PanelsState,
    onBack: () -> Unit,
    onInput: (String) -> Unit,
    onSend: () -> Unit,
    onQuick: (com.guyu2233.ibukirpg.app.data.QuickActionV1, String) -> Unit,
    onOpenPanels: () -> Unit,
    onRetry: () -> Unit,
    onErrorShown: () -> Unit,
    initialPanels: Boolean = false,
    initialHudExpanded: Boolean = false,
    onNoticesShown: () -> Unit = {},
    dialogs: RpgDialogState = rememberRpgDialogState(),
    onLoadEarlier: () -> Unit = {},
    world: WorldState = WorldState(),
    timeline: TimelineV1? = null,
    prompts: PromptSettingsV1? = null,
    actions: WorldActions = WorldActions(),
    initialWorld: Boolean = false,
    /** 截图测试：不弹出偏离提示抽屉（改为直接渲染 DecisionContent）。 */
    showDecisionSheet: Boolean = true,
) {
    val snackbar = remember { SnackbarHostState() }
    val combat = s.scene.combat
    val decision = s.scene.decision
    val wide = LocalConfiguration.current.screenWidthDp >= 840
    var showWorld by rememberSaveable { mutableStateOf(initialWorld) }

    LaunchedEffect(s.notices) {
        if (s.notices.isEmpty()) return@LaunchedEffect
        val text = s.notices.joinToString("\n") { it.text }
        onNoticesShown()
        snackbar.showSnackbar(text)
    }
    var showPanels by rememberSaveable { mutableStateOf(initialPanels) }
    val keptMsg = stringResource(R.string.game_input_kept)
    val list = rememberLazyListState()
    val scope = rememberCoroutineScope()
    var animated by remember { mutableStateOf(setOf<Long>()) }

    LaunchedEffect(s.error) {
        s.error?.let {
            onErrorShown()
            snackbar.showSnackbar("$it\n$keptMsg")
        }
    }

    // 新回合到达：滚动到本回合第一条（玩家行动），方便从头阅读；提交时滚到底部。
    // 以最后一条记录的 id 判断“新回合”，这样向上加载更早的记录（头部插入）不会触发滚动。
    val header = if (s.hasEarlier) 1 else 0
    var lastTail by remember { mutableStateOf<Long?>(null) }
    var lastVersion by remember { mutableIntStateOf(-1) }
    LaunchedEffect(s.version, s.loading, s.entries.isEmpty()) {
        if (s.loading) return@LaunchedEffect
        val n = s.entries.size
        if (n == 0) return@LaunchedEffect
        if (lastVersion < 0) {
            list.scrollToItem(n - 1 + header)
        } else if (s.version != lastVersion) {
            val prev = s.entries.indexOfLast { it.id == lastTail && it.id != 0L }
            list.animateScrollToItem((prev + 1).coerceIn(0, n - 1) + header)
        }
        lastVersion = s.version
        lastTail = s.entries.lastOrNull { it.id != 0L }?.id
    }
    LaunchedEffect(s.pending) {
        if (s.pending != null) list.animateScrollToItem(s.entries.size + header)
    }
    val atBottom by remember { derivedStateOf { !list.canScrollForward } }

    Scaffold(
        topBar = {
            Column {
                TopAppBar(
                    title = { SceneTitle(s.scene) },
                    navigationIcon = {
                        IconButton(onClick = onBack) {
                            Icon(Icons.AutoMirrored.Outlined.ArrowBack, contentDescription = stringResource(R.string.back))
                        }
                    },
                    actions = {
                        IconButton(onClick = actions.onOpenTimeline) {
                            Icon(Icons.Outlined.AccountTree, contentDescription = "时间线与检查点")
                        }
                        IconButton(onClick = { showWorld = !showWorld }) {
                            BadgedBox(badge = { if (s.entries.lastOrNull { it.kind == "world" }?.turn == s.scene.turn && s.scene.turn > 0) Badge() }) {
                                Icon(Icons.Outlined.Public, contentDescription = "世界面板")
                            }
                        }
                        IconButton(onClick = { showPanels = true; onOpenPanels() }) {
                            Icon(Icons.Outlined.PersonOutline, contentDescription = stringResource(R.string.game_panels))
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surfaceContainer),
                )
                if (!s.loading && combat == null) HudBar(s.scene.hud, s.scene.story, initiallyExpanded = initialHudExpanded, upcoming = s.scene.upcoming, conditions = s.scene.conditions)
                if (s.pending != null) LinearProgressIndicator(Modifier.fillMaxWidth()) else HorizontalDivider()
            }
        },
        bottomBar = {
            InputArea(
                combat = {
                    if (combat != null) {
                        CombatPanel(combat, busy = s.pending != null || s.loading, onAction = onQuick)
                        HorizontalDivider()
                    }
                },
                input = input,
                onInput = onInput,
                onSend = onSend,
                suggestions = s.suggestions,
                busy = s.pending != null || s.loading || (decision != null && !decision.notify),
                onSuggestion = { onQuick(it.action, it.label) },
                aiSuggestions = s.scene.aiSuggestions,
                onAiSuggestion = { onQuick(QuickActionV1(kind = "text", text = it, label = it), it) },
                inCombat = combat != null,
                pendingTurn = s.scene.pendingTurn,
                onCancelRollback = actions.onCancelRollback,
                onWait = if (combat == null) actions.onWait else null,
            )
        },
        snackbarHost = { SnackbarHost(snackbar) },
        floatingActionButton = {
            AnimatedVisibility(!atBottom && !s.loading, enter = fadeIn(), exit = fadeOut()) {
                SmallFloatingActionButton(onClick = { scope.launch { list.animateScrollToItem(maxOf(0, list.layoutInfo.totalItemsCount - 1)) } }) {
                    Icon(Icons.Outlined.KeyboardDoubleArrowDown, contentDescription = stringResource(R.string.game_scroll_bottom))
                }
            }
        },
    ) { pad ->
      Row(Modifier.fillMaxSize().padding(pad)) {
        Column(Modifier.weight(1f).fillMaxSize(), horizontalAlignment = Alignment.CenterHorizontally) {
        if (combat != null && !s.loading) {
            CombatHeader(combat, Modifier.widthIn(max = 720.dp).padding(start = 12.dp, end = 12.dp, top = 8.dp))
        }
        Box(Modifier.weight(1f).fillMaxWidth(), contentAlignment = Alignment.TopCenter) {
            when {
                s.loading -> CircularProgressIndicator(Modifier.align(Alignment.Center))
                s.fatal != null -> Column(Modifier.align(Alignment.Center).padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    Text(s.fatal ?: "", style = MaterialTheme.typography.bodyLarge)
                    Spacer(Modifier.size(12.dp))
                    Button(onClick = onRetry) { Text(stringResource(R.string.retry)) }
                }
                else -> LazyColumn(
                    state = list,
                    modifier = Modifier.widthIn(max = 720.dp).fillMaxSize(),
                    contentPadding = PaddingValues(horizontal = 16.dp, vertical = 16.dp),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    if (s.hasEarlier) {
                        item(key = "earlier", contentType = "earlier") {
                            Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                                if (s.loadingEarlier) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                                else androidx.compose.material3.TextButton(onClick = onLoadEarlier) { Text(stringResource(R.string.game_load_earlier)) }
                            }
                        }
                    }
                    itemsIndexed(
                        s.entries,
                        key = { i, e -> if (e.id != 0L) e.id else "i$i" },
                        contentType = { _, e -> e.kind },
                    ) { _, e ->
                        EntryItem(
                            entry = e,
                            animate = e.id in s.freshIds && e.id !in animated,
                            onAnimated = { animated = animated + it },
                            onOption = { onQuick(it.action, it.label) },
                            enabled = s.pending == null,
                            onCard = { dialogs.cardId = it },
                            onRollback = actions.onRollbackTurn,
                        )
                    }
                    s.pending?.let { p ->
                        item(key = "pending") {
                            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                if (p.label.isNotBlank()) PlayerBubble(p.label, sending = true)
                                if (s.streaming.isNotEmpty()) {
                                    StreamingNarration(s.streaming)
                                } else {
                                    Row(
                                        verticalAlignment = Alignment.CenterVertically,
                                        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                                    ) {
                                        CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                                        Spacer(Modifier.size(8.dp))
                                        Text(
                                            stringResource(R.string.game_thinking), style = MaterialTheme.typography.bodyMedium,
                                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                                        )
                                    }
                                }
                            }
                        }
                    }
                }
            }
        }
        }
        if (wide && showWorld) {
            WorldPanelPane(
                world, busy = s.pending != null, onClose = { showWorld = false }, onTab = actions.onWorldTab, onAction = onQuick,
                onRevert = actions.onRevert, modifier = Modifier.width(400.dp),
            )
        }
      }
    }

    if (showWorld && !wide) {
        WorldPanelSheet(world, busy = s.pending != null, onDismiss = { showWorld = false }, onTab = actions.onWorldTab, onAction = { qa, label ->
            if (qa.kind != "manage") showWorld = false
            onQuick(qa, label)
        }, onRevert = actions.onRevert)
    }

    if (decision != null && showDecisionSheet && s.pending == null) {
        DecisionSheet(
            decision, busy = s.pending != null,
            onAccept = { n -> actions.onDecision(true, n) },
            onRollback = { n -> actions.onDecision(false, n) },
            onSensitivity = actions.onSensitivity,
        )
    }

    timeline?.let { t -> TimelineDialog(t, s.scene.saveName.ifBlank { s.scene.playerName }, s.scene.packName, actions.timeline) }

    prompts?.let { p -> SensitivityDialog(p, onDismiss = { actions.onSavePrompts(null) }, onSave = { actions.onSavePrompts(it) }) }

    if (showPanels) {
        PanelsSheet(
            panels = panels,
            busy = s.pending != null,
            onDismiss = { showPanels = false },
            onAction = { qa, label ->
                // 管理类操作（加点 / 装备 / 改装）留在面板里，其它行动关闭面板回到叙事。
                if (qa.kind != "manage") showPanels = false
                onQuick(qa, label)
            },
            refreshKey = s.version,
        )
    }

    RpgDialogHost(dialogs, onQuick, refreshKey = s.version)

}

private fun combat0(scene: SceneV1) = scene.combat != null

@Composable
private fun SceneTitle(scene: SceneV1) {
    Column {
        val combat = scene.combat
        Text(
            if (combat != null) "战斗 · 第 ${combat.round} 轮" else scene.locationName,
            style = MaterialTheme.typography.titleLarge, maxLines = 1, overflow = TextOverflow.Ellipsis,
            color = if (combat != null) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
        )
        val sub = buildString {
            if (combat0(scene)) {
                append(scene.locationName)
            } else if (scene.packName.isNotBlank()) {
                append(scene.packName).append(" · ")
            }
            if (scene.timeText.isNotBlank()) append(if (combat0(scene)) " · " else "").append(scene.timeText) else append(stringResource(R.string.saves_turn, scene.turn))
            if (scene.branch.isNotBlank() && scene.branch != "主干") append(" · ").append(scene.branch)
        }
        Text(
            sub,
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

@Composable
private fun InputArea(
    combat: @Composable () -> Unit,
    input: String,
    onInput: (String) -> Unit,
    onSend: () -> Unit,
    suggestions: List<SuggestionV1>,
    busy: Boolean,
    onSuggestion: (SuggestionV1) -> Unit,
    aiSuggestions: List<String> = emptyList(),
    onAiSuggestion: (String) -> Unit = {},
    inCombat: Boolean = false,
    pendingTurn: Int = 0,
    onCancelRollback: () -> Unit = {},
    onWait: ((String, String) -> Unit)? = null,
) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, tonalElevation = 0.dp) {
        Column(Modifier.windowInsetsPadding(WindowInsets.navigationBars.union(WindowInsets.ime))) {
            if (pendingTurn != 0) {
                Row(
                    Modifier.fillMaxWidth().padding(start = 16.dp, end = 8.dp, top = 6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text(
                        "已回到回合 ${maxOf(pendingTurn, 0)}：下一次行动会开出新分支，原来的进展会保留。",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.tertiary, modifier = Modifier.weight(1f),
                    )
                    TextButton(onClick = onCancelRollback, enabled = !busy) { Text("取消") }
                }
            }
            combat()
            val label = stringResource(R.string.game_suggestions)
            var waitMenu by remember { mutableStateOf(false) }
            if (suggestions.isNotEmpty() || aiSuggestions.isNotEmpty() || onWait != null) {
                LazyRow(
                    contentPadding = PaddingValues(horizontal = 12.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.padding(top = 8.dp).semantics { contentDescription = label },
                ) {
                    items(aiSuggestions, key = { "ai:$it" }) { text ->
                        AssistChip(
                            onClick = { onAiSuggestion(text) },
                            enabled = !busy,
                            label = { Text(text) },
                            leadingIcon = { Icon(Icons.Outlined.AutoAwesome, contentDescription = "AI 建议", modifier = Modifier.size(AssistChipDefaults.IconSize)) },
                            border = AssistChipDefaults.assistChipBorder(enabled = !busy, borderColor = MaterialTheme.colorScheme.primary),
                        )
                    }
                    items(suggestions, key = { it.label + it.action.kind + (it.action.action ?: "") + (it.action.target ?: "") + (it.action.destination ?: "") }) { sg ->
                        AssistChip(
                            onClick = { onSuggestion(sg) },
                            enabled = !busy,
                            label = { Text(sg.label) },
                            leadingIcon = {
                                Icon(iconFor(sg.icon), contentDescription = null, modifier = Modifier.size(AssistChipDefaults.IconSize))
                            },
                        )
                    }
                    if (onWait != null) {
                        item(key = "wait") {
                            Box {
                                AssistChip(
                                    onClick = { waitMenu = true },
                                    enabled = !busy,
                                    label = { Text("等待…") },
                                    leadingIcon = { Icon(Icons.Outlined.Schedule, contentDescription = null, modifier = Modifier.size(AssistChipDefaults.IconSize)) },
                                )
                                DropdownMenu(expanded = waitMenu, onDismissRequest = { waitMenu = false }) {
                                    listOf("10m" to "等 10 分钟", "1h" to "等 1 小时", "dawn" to "等到天亮").forEach { (t, l) ->
                                        DropdownMenuItem(text = { Text(l) }, onClick = { waitMenu = false; onWait(t, l) })
                                    }
                                }
                            }
                        }
                    }
                }
            }
            Row(
                Modifier.fillMaxWidth().padding(start = 12.dp, end = 8.dp, top = 6.dp, bottom = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    value = input,
                    onValueChange = { if (it.length <= 200) onInput(it) },
                    placeholder = { Text(if (inCombat) "描述你的行动……" else stringResource(R.string.game_input_hint)) },
                    modifier = Modifier.weight(1f).heightIn(min = 52.dp),
                    maxLines = 4,
                    shape = MaterialTheme.shapes.extraLarge,
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Send),
                    keyboardActions = KeyboardActions(onSend = { if (!busy) onSend() }),
                )
                Spacer(Modifier.size(8.dp))
                FilledIconButton(
                    onClick = onSend,
                    enabled = !busy && input.isNotBlank(),
                    modifier = Modifier.size(52.dp),
                ) {
                    if (busy) {
                        CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                    } else {
                        Icon(Icons.AutoMirrored.Outlined.Send, contentDescription = stringResource(R.string.game_send))
                    }
                }
            }
        }
    }
}
