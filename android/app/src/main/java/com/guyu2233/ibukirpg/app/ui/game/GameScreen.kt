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
import androidx.compose.material.icons.outlined.AltRoute
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
import com.guyu2233.ibukirpg.app.data.MainlineV1
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
    ) }
}

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
) {
    val snackbar = remember { SnackbarHostState() }
    val combat = s.scene.combat
    val mainline = s.scene.mainline
    var deviationDismissedAt by rememberSaveable { mutableIntStateOf(-1) }

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
    var lastCount by remember { mutableIntStateOf(-1) }
    LaunchedEffect(s.entries.size, s.loading) {
        if (s.loading) return@LaunchedEffect
        val n = s.entries.size
        if (lastCount < 0) {
            if (n > 0) list.scrollToItem(n - 1)
        } else if (n > lastCount) {
            list.animateScrollToItem(lastCount.coerceAtMost(n - 1))
        }
        lastCount = n
    }
    LaunchedEffect(s.pending) {
        if (s.pending != null) list.animateScrollToItem(s.entries.size)
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
                        IconButton(onClick = { showPanels = true; onOpenPanels() }) {
                            Icon(Icons.Outlined.PersonOutline, contentDescription = stringResource(R.string.game_panels))
                        }
                    },
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surfaceContainer),
                )
                if (!s.loading) HudBar(s.scene.hud, s.scene.story, initiallyExpanded = initialHudExpanded)
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
                busy = s.pending != null || s.loading,
                onSuggestion = { onQuick(it.action, it.label) },
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
        Box(Modifier.fillMaxSize().padding(pad), contentAlignment = Alignment.TopCenter) {
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
                    itemsIndexed(s.entries, key = { i, e -> if (e.id != 0L) e.id else "i$i" }) { _, e ->
                        EntryItem(
                            entry = e,
                            animate = e.id in s.freshIds && e.id !in animated,
                            onAnimated = { animated = animated + it },
                            onOption = { onQuick(it.action, it.label) },
                            enabled = s.pending == null,
                            onCard = { dialogs.cardId = it },
                        )
                    }
                    s.pending?.let { p ->
                        item(key = "pending") {
                            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                                PlayerBubble(p.label, sending = true)
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
            mainline = mainline,
            refreshKey = s.version,
        )
    }

    RpgDialogHost(dialogs, onQuick, refreshKey = s.version)

    if (mainline != null && mainline.pending && s.pending == null && combat == null && deviationDismissedAt != s.scene.turn) {
        DeviationDialog(
            mainline = mainline,
            onReturn = { onQuick(QuickActionV1(kind = "mainline", action = "return", label = "回到主线"), "回到主线") },
            onFree = { onQuick(QuickActionV1(kind = "mainline", action = "free", label = "进入自由推演"), if (mainline.freeOnline) "进入自由推演" else "进入沙盒模式") },
            onLater = { deviationDismissedAt = s.scene.turn },
        )
    }
}

@Composable
private fun DeviationDialog(mainline: MainlineV1, onReturn: () -> Unit, onFree: () -> Unit, onLater: () -> Unit) {
    AlertDialog(
        onDismissRequest = onLater,
        icon = { Icon(Icons.Outlined.AltRoute, contentDescription = null) },
        title = { Text(stringResource(R.string.mainline_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.mainline_body, mainline.deviation))
                mainline.anchor?.let { Text("当前锚点：$it", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                if (!mainline.freeOnline) Text(stringResource(R.string.mainline_offline_note), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.tertiary)
            }
        },
        confirmButton = { Button(onClick = onReturn) { Text(stringResource(R.string.mainline_return)) } },
        dismissButton = {
            OutlinedButton(onClick = onFree) {
                Text(stringResource(if (mainline.freeOnline) R.string.mainline_free else R.string.mainline_sandbox))
            }
        },
    )
}

@Composable
private fun SceneTitle(scene: SceneV1) {
    Column {
        Text(scene.locationName, style = MaterialTheme.typography.titleLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
        val sub = buildString {
            if (scene.packName.isNotBlank()) append("《").append(scene.packName).append("》 · ")
            append(stringResource(R.string.saves_turn, scene.turn))
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
) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainer, tonalElevation = 0.dp) {
        Column(Modifier.windowInsetsPadding(WindowInsets.navigationBars.union(WindowInsets.ime))) {
            combat()
            if (suggestions.isNotEmpty()) {
                val label = stringResource(R.string.game_suggestions)
                LazyRow(
                    contentPadding = PaddingValues(horizontal = 12.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.padding(top = 8.dp).semantics { contentDescription = label },
                ) {
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
                }
            }
            Row(
                Modifier.fillMaxWidth().padding(start = 12.dp, end = 8.dp, top = 6.dp, bottom = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    value = input,
                    onValueChange = { if (it.length <= 200) onInput(it) },
                    placeholder = { Text(stringResource(R.string.game_input_hint)) },
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
